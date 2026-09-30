// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {SettlementBase} from "./Base.t.sol";
import {IPaymentSettlement} from "../src/IPaymentSettlement.sol";
import {IPaymentSettlementExtensions} from "../src/IPaymentSettlementExtensions.sol";
import {PaymentSettlement} from "../src/PaymentSettlement.sol";
import {PaymentTypes} from "../src/PaymentTypes.sol";
import {TestUSDC} from "../src/test-token/TestUSDC.sol";

/// Limits, withdrawals, closing, cash-out on a merchant's behalf and the edge cases found in
/// the pre-deployment review. Test names follow docs/content/products/p06/srs.md.
contract ExtensionsTest is SettlementBase {
    bytes32 internal constant ORDER = bytes32(uint256(0x0101));

    // P06-FR-13
    function test_limitChange_withinCap() public {
        PaymentTypes.LimitChange memory c = _limitChange(10e6, 30e6, 5);
        vm.expectEmit(true, false, false, true);
        emit IPaymentSettlementExtensions.LimitsChanged(device, 10e6, 30e6);
        vm.prank(kiosk);
        settlement.setLimits(c, _signLimits(deviceKey, c));
        IPaymentSettlementExtensions.AccountView memory v = settlement.accountOf(device);
        assertEq(v.perPaymentLimit, 10e6);
        assertEq(v.dailyLimit, 30e6);

        PaymentTypes.PaymentAuthorization memory a = _auth(10e6 + 1, ORDER, 1);
        bytes memory sig = _sign(deviceKey, a);
        vm.expectRevert(IPaymentSettlement.OverCap.selector);
        _settle(a, sig);
        for (uint256 i = 0; i < 3; i++) {
            a = _auth(10e6, bytes32(i + 1), i + 10);
            _settle(a, _sign(deviceKey, a));
        }
        a = _auth(1, ORDER, 20);
        sig = _sign(deviceKey, a);
        vm.expectRevert(IPaymentSettlement.OverCap.selector); // 30e6 daily limit reached
        _settle(a, sig);
    }

    function test_limitChange_aboveCapRejected() public {
        PaymentTypes.LimitChange memory c = _limitChange(PER_PAYMENT_CAP + 1, 0, 5);
        bytes memory sig = _signLimits(deviceKey, c);
        vm.expectRevert(IPaymentSettlement.OverCap.selector);
        settlement.setLimits(c, sig);
        c = _limitChange(0, DAILY_CAP + 1, 6);
        sig = _signLimits(deviceKey, c);
        vm.expectRevert(IPaymentSettlement.OverCap.selector);
        settlement.setLimits(c, sig);
    }

    function test_limitChange_expired() public {
        PaymentTypes.LimitChange memory c = _limitChange(10e6, 0, 5);
        bytes memory sig = _signLimits(deviceKey, c);
        vm.warp(c.expiry + 1);
        vm.expectRevert(IPaymentSettlement.Expired.selector);
        settlement.setLimits(c, sig);
        c = _limitChange(10e6, 0, 6);
        c.expiry = uint64(block.timestamp + AUTH_EXPIRY + 1);
        sig = _signLimits(deviceKey, c);
        vm.expectRevert(IPaymentSettlement.Expired.selector);
        settlement.setLimits(c, sig);
    }

    // P06-FR-08: payments and limit changes share one nonce space
    function test_limitChange_replay() public {
        PaymentTypes.LimitChange memory c = _limitChange(10e6, 0, 5);
        bytes memory sig = _signLimits(deviceKey, c);
        settlement.setLimits(c, sig);
        vm.expectRevert(IPaymentSettlement.NonceReplayed.selector);
        settlement.setLimits(c, sig);
        PaymentTypes.PaymentAuthorization memory a = _auth(1e6, ORDER, 5);
        bytes memory psig = _sign(deviceKey, a);
        vm.expectRevert(IPaymentSettlement.NonceReplayed.selector);
        _settle(a, psig);
    }

    function test_limitChange_wrongDomainAndSigner() public {
        PaymentTypes.LimitChange memory c = _limitChange(10e6, 0, 5);
        c.contractAddress = address(0xBEEF);
        bytes memory sig = _signLimits(deviceKey, c);
        vm.expectRevert(IPaymentSettlement.WrongDomain.selector);
        settlement.setLimits(c, sig);
        c = _limitChange(10e6, 0, 5);
        sig = _signLimits(0xBAD, c);
        vm.expectRevert(IPaymentSettlement.AccountInactive.selector);
        settlement.setLimits(c, sig);
    }

    // P06-FR-07
    function test_limits_zeroMeansCap() public {
        PaymentTypes.LimitChange memory c = _limitChange(10e6, 0, 5);
        settlement.setLimits(c, _signLimits(deviceKey, c));
        c = _limitChange(0, 0, 6);
        settlement.setLimits(c, _signLimits(deviceKey, c));
        PaymentTypes.PaymentAuthorization memory a = _auth(PER_PAYMENT_CAP, ORDER, 1);
        _settle(a, _sign(deviceKey, a));
        assertEq(settlement.merchantBalance(merchant), PER_PAYMENT_CAP);
    }

    // P06-FR-07: the fixed window allows at most 2 x dailyCap across one boundary, never more
    function testFuzz_dailyWindow(uint256 gap) public {
        gap = bound(gap, 1, 3 days);
        _deposit(device, 1000e6);
        uint256 n;
        for (uint256 i = 0; i < 4; i++) {
            PaymentTypes.PaymentAuthorization memory a = _auth(50e6, bytes32(++n), n);
            _settle(a, _sign(deviceKey, a));
        }
        vm.warp(block.timestamp + gap);
        PaymentTypes.PaymentAuthorization memory b = _auth(50e6, bytes32(++n), n);
        bytes memory sig = _sign(deviceKey, b);
        if (gap < 1 days) {
            vm.expectRevert(IPaymentSettlement.OverCap.selector);
            _settle(b, sig);
        } else {
            _settle(b, sig);
            assertEq(settlement.accountOf(device).windowStart, block.timestamp);
            assertEq(settlement.accountOf(device).windowSpent, 50e6);
        }
    }

    // P06-FR-14
    function test_withdrawal_operatorOnly() public {
        vm.expectRevert(IPaymentSettlement.NotOperator.selector);
        settlement.requestWithdrawal(device, 1e6);
        vm.expectRevert(IPaymentSettlement.NotOperator.selector);
        settlement.cancelWithdrawal(device);
        vm.expectRevert(IPaymentSettlement.NotOperator.selector);
        settlement.closeAccount(device);
    }

    function test_withdrawal_delayed() public {
        vm.prank(operator);
        settlement.requestWithdrawal(device, 40e6);
        vm.expectRevert(IPaymentSettlementExtensions.WithdrawalNotReady.selector);
        settlement.executeWithdrawal(device);
        vm.warp(block.timestamp + WITHDRAWAL_DELAY - 1);
        vm.expectRevert(IPaymentSettlementExtensions.WithdrawalNotReady.selector);
        settlement.executeWithdrawal(device);
        vm.warp(block.timestamp + 1);
        settlement.executeWithdrawal(device);
        assertEq(token.balanceOf(renter), 40e6);
        assertEq(settlement.balanceOf(device), 110e6);
        vm.expectRevert(IPaymentSettlementExtensions.NoPendingWithdrawal.selector);
        settlement.executeWithdrawal(device);
    }

    function test_withdrawal_fixedDestination() public {
        vm.prank(operator);
        settlement.requestWithdrawal(device, 40e6);
        vm.warp(block.timestamp + WITHDRAWAL_DELAY);
        vm.prank(makeAddr("stranger"));
        settlement.executeWithdrawal(device);
        assertEq(token.balanceOf(renter), 40e6, "paid to the withdraw address only");
        assertEq(token.balanceOf(operator), 0);
    }

    function test_withdrawal_settleDuringDelay() public {
        vm.prank(operator);
        settlement.requestWithdrawal(device, 150e6);
        PaymentTypes.PaymentAuthorization memory a = _auth(45e5, ORDER, 1);
        _settle(a, _sign(deviceKey, a)); // the full balance still backs signed payments
        vm.warp(block.timestamp + WITHDRAWAL_DELAY);
        settlement.executeWithdrawal(device);
        assertEq(token.balanceOf(renter), 150e6 - 45e5, "min(requested, balance)");
        assertEq(settlement.balanceOf(device), 0);
    }

    function test_withdrawal_cancel() public {
        vm.startPrank(operator);
        settlement.requestWithdrawal(device, 40e6);
        settlement.cancelWithdrawal(device);
        vm.expectRevert(IPaymentSettlementExtensions.NoPendingWithdrawal.selector);
        settlement.cancelWithdrawal(device);
        vm.stopPrank();
        vm.warp(block.timestamp + WITHDRAWAL_DELAY);
        vm.expectRevert(IPaymentSettlementExtensions.NoPendingWithdrawal.selector);
        settlement.executeWithdrawal(device);
    }

    function test_withdrawal_unknownAccount() public {
        vm.prank(operator);
        vm.expectRevert(IPaymentSettlement.AccountInactive.selector);
        settlement.requestWithdrawal(makeAddr("nobody"), 1);
        vm.prank(operator);
        vm.expectRevert(PaymentSettlement.InvalidParameters.selector);
        settlement.requestWithdrawal(device, 0);
    }

    // P06-FR-15
    function test_closeAccount_blocksSettle() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(1e6, ORDER, 1);
        bytes memory sig = _sign(deviceKey, a); // signed before the close
        vm.prank(operator);
        settlement.closeAccount(device);
        vm.expectRevert(IPaymentSettlement.AccountInactive.selector);
        _settle(a, sig);
        PaymentTypes.LimitChange memory c = _limitChange(1e6, 0, 2);
        bytes memory lsig = _signLimits(deviceKey, c);
        vm.expectRevert(IPaymentSettlement.AccountInactive.selector);
        settlement.setLimits(c, lsig);
        vm.startPrank(operator);
        vm.expectRevert(IPaymentSettlement.AccountInactive.selector);
        settlement.closeAccount(device);
        vm.expectRevert(IPaymentSettlement.AccountInactive.selector);
        settlement.cancelWithdrawal(device); // a closed account's payout cannot be cancelled
        vm.stopPrank();
    }

    function test_closeAccount_paysAfterDelay() public {
        vm.prank(operator);
        settlement.closeAccount(device);
        vm.expectRevert(IPaymentSettlementExtensions.WithdrawalNotReady.selector);
        settlement.executeWithdrawal(device);
        vm.warp(block.timestamp + WITHDRAWAL_DELAY);
        settlement.executeWithdrawal(device);
        assertEq(token.balanceOf(renter), 150e6);
        assertEq(settlement.balanceOf(device), 0);
        assertEq(token.balanceOf(address(settlement)), 0);
    }

    function test_depositFor_closedReverts() public {
        vm.prank(operator);
        settlement.closeAccount(device);
        vm.startPrank(operator);
        token.mint(operator, 1e6);
        token.approve(address(settlement), 1e6);
        vm.expectRevert(IPaymentSettlement.AccountInactive.selector);
        settlement.depositFor(device, 1e6, renter);
        vm.stopPrank();
    }

    // P06-FR-16
    function test_execute_permissionlessFixedDestination() public {
        vm.prank(operator);
        settlement.closeAccount(device);
        vm.warp(block.timestamp + WITHDRAWAL_DELAY);
        vm.prank(kiosk);
        settlement.executeWithdrawal(device);
        assertEq(token.balanceOf(renter), 150e6);
        assertEq(token.balanceOf(kiosk), 0);
    }

    // P06-FR-12: a merchant that lost its key is still paid, and only to its payout
    function test_cashOutFor_permissionless() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(45e5, ORDER, 1);
        _settle(a, _sign(deviceKey, a));
        vm.prank(makeAddr("stranger"));
        settlement.cashOutFor(merchant);
        assertEq(token.balanceOf(payout), 45e5);
        assertEq(settlement.merchantBalance(merchant), 0);
    }

    // P06-FR-12, P06-FR-17: cash-out follows the payout in effect at cash-out time
    function test_cashOut_followsDelayedPayoutChange() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(45e5, ORDER, 1);
        _settle(a, _sign(deviceKey, a));
        address newPayout = makeAddr("newPayout");
        vm.prank(registryAdmin);
        registry.requestPayoutChange(merchant, newPayout);
        settlement.cashOutFor(merchant); // before the delay: the old payout
        assertEq(token.balanceOf(payout), 45e5);
        a = _auth(1e6, bytes32(uint256(2)), 2);
        _settle(a, _sign(deviceKey, a));
        vm.warp(block.timestamp + PAYOUT_CHANGE_DELAY);
        settlement.cashOutFor(merchant);
        assertEq(token.balanceOf(newPayout), 1e6);
    }

    // Right to refuse: a trusted settlement contract pays straight through; an opted-out
    // payout receives a held transfer it can accept.
    function test_cashOut_payoutInRefusalMode() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(45e5, ORDER, 1);
        _settle(a, _sign(deviceKey, a));
        vm.prank(payout);
        token.setRefusalMode(true);
        vm.prank(operator); // the token owner in this suite
        token.setTrustedSender(address(settlement), true);
        settlement.cashOutFor(merchant);
        assertEq(token.balanceOf(payout), 45e5, "trusted settlement payouts are not held");

        a = _auth(1e6, bytes32(uint256(2)), 2);
        _settle(a, _sign(deviceKey, a));
        vm.prank(payout);
        token.setTrustOptOut(address(settlement), true);
        uint256 id = token.nextHeldId();
        settlement.cashOutFor(merchant);
        assertEq(token.balanceOf(payout), 45e5, "held after opting out");
        assertEq(settlement.merchantBalance(merchant), 0);
        vm.prank(payout);
        token.acceptTransfer(id);
        assertEq(token.balanceOf(payout), 55e5);
    }

    // Malformed signatures end in AccountInactive, never in a wrong signer.
    function test_settle_malformedSignature() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(1e6, ORDER, 1);
        bytes memory sig = _sign(deviceKey, a);
        bytes memory short = new bytes(64);
        bytes memory long = abi.encodePacked(sig, uint8(0));
        bytes memory badV = abi.encodePacked(sig);
        badV[64] = bytes1(uint8(1));
        bytes memory zero = new bytes(65);
        bytes[4] memory bad = [short, long, badV, zero];
        for (uint256 i = 0; i < 4; i++) {
            vm.expectRevert(IPaymentSettlement.AccountInactive.selector);
            _settle(a, bad[i]);
        }
        _settle(a, sig);
    }

    // P06-FR-19: a rejected payout comes back as surplus and only the surplus is recoverable.
    function test_recoverSurplus_rejectedPayout() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(45e5, ORDER, 1);
        _settle(a, _sign(deviceKey, a));
        vm.startPrank(payout);
        token.setRefusalMode(true); // settlement not trusted in this suite
        vm.stopPrank();
        uint256 id = token.nextHeldId();
        settlement.cashOutFor(merchant);
        vm.prank(payout);
        token.rejectTransfer(id); // back to the settlement contract
        assertEq(settlement.totalOwed(), 150e6 - 45e5, "owed: the device balance only");
        assertEq(settlement.surplus(), 45e5);

        address treasury = makeAddr("treasury");
        vm.prank(operator);
        vm.expectRevert(PaymentSettlement.InvalidParameters.selector);
        settlement.recoverSurplus(treasury, 45e5 + 1); // never into owed balances
        vm.expectEmit(true, false, false, true);
        emit IPaymentSettlementExtensions.SurplusRecovered(treasury, 45e5);
        vm.prank(operator);
        settlement.recoverSurplus(treasury, 45e5);
        assertEq(token.balanceOf(treasury), 45e5);
        assertEq(settlement.surplus(), 0);
        assertEq(token.balanceOf(address(settlement)), settlement.totalOwed());
    }

    function test_recoverSurplus_operatorOnlyAndDirectTransfers() public {
        vm.prank(operator);
        token.mint(address(this), 1e6);
        token.transfer(address(settlement), 1e6); // sent here directly by mistake
        assertEq(settlement.surplus(), 1e6);
        vm.expectRevert(IPaymentSettlement.NotOperator.selector);
        settlement.recoverSurplus(address(this), 1e6);
        vm.prank(operator);
        settlement.recoverSurplus(address(this), 1e6);
        assertEq(token.balanceOf(address(this)), 1e6);
    }

    function test_totalOwed_tracksEveryMove() public {
        assertEq(settlement.totalOwed(), 150e6);
        PaymentTypes.PaymentAuthorization memory a = _auth(45e5, ORDER, 1);
        _settle(a, _sign(deviceKey, a));
        assertEq(settlement.totalOwed(), 150e6, "a payment moves between balances");
        settlement.cashOutFor(merchant);
        assertEq(settlement.totalOwed(), 150e6 - 45e5);
        vm.prank(operator);
        settlement.closeAccount(device);
        vm.warp(block.timestamp + WITHDRAWAL_DELAY);
        settlement.executeWithdrawal(device);
        assertEq(settlement.totalOwed(), 0);
        assertEq(settlement.surplus(), 0);
    }

    // Review finding: a free zero payment must not claim an order id.
    function test_settle_zeroAmountRejected() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(0, ORDER, 1);
        bytes memory sig = _sign(deviceKey, a);
        vm.expectRevert(PaymentSettlement.InvalidParameters.selector);
        _settle(a, sig);
        assertFalse(settlement.isPaid(merchant, ORDER));
    }

    // Review finding: a withdraw address that can never receive would lock the account.
    function test_depositFor_unpayableWithdrawAddressRejected() public {
        address fresh = vm.addr(0xF00D);
        vm.startPrank(operator);
        token.mint(operator, 2);
        token.approve(address(settlement), 2);
        vm.expectRevert(PaymentSettlement.InvalidParameters.selector);
        settlement.depositFor(fresh, 1, address(token));
        vm.expectRevert(PaymentSettlement.InvalidParameters.selector);
        settlement.depositFor(fresh, 1, address(settlement));
        vm.stopPrank();
    }

    // Review finding: a payout that can never receive keeps the balance with the merchant.
    function test_cashOut_unpayablePayoutKeepsBalance() public {
        address m2 = makeAddr("m2");
        vm.prank(registryAdmin);
        registry.registerMerchant(m2, address(settlement));
        PaymentTypes.PaymentAuthorization memory a = _auth(1e6, ORDER, 1);
        a.merchant = m2;
        a.payout = address(settlement);
        _settle(a, _sign(deviceKey, a));
        vm.expectRevert(PaymentSettlement.InvalidParameters.selector);
        settlement.cashOutFor(m2);
        assertEq(settlement.merchantBalance(m2), 1e6);
    }

    // Review finding: an empty cash-out creates no transfer (and no held-transfer notice).
    function test_cashOut_zeroBalanceNoTransfer() public {
        vm.prank(payout);
        token.setRefusalMode(true);
        uint256 before = token.nextHeldId();
        settlement.cashOutFor(merchant);
        assertEq(token.nextHeldId(), before);
    }

    // Review finding: the operator cancels only during the delay (P06-FR-14).
    function test_withdrawal_cancelOnlyDuringDelay() public {
        vm.prank(operator);
        settlement.requestWithdrawal(device, 40e6);
        vm.warp(block.timestamp + WITHDRAWAL_DELAY);
        vm.prank(operator);
        vm.expectRevert(IPaymentSettlementExtensions.WithdrawalMatured.selector);
        settlement.cancelWithdrawal(device);
        settlement.executeWithdrawal(device);
        assertEq(token.balanceOf(renter), 40e6);
    }

    function test_constructor_rejectsAddressesWithoutCode() public {
        vm.expectRevert(PaymentSettlement.InvalidParameters.selector);
        new PaymentSettlement(
            makeAddr("noToken"), address(registry), operator, PER_PAYMENT_CAP, DAILY_CAP, WITHDRAWAL_DELAY, AUTH_EXPIRY
        );
        vm.expectRevert(PaymentSettlement.InvalidParameters.selector);
        new PaymentSettlement(
            address(token), makeAddr("noRegistry"), operator, PER_PAYMENT_CAP, DAILY_CAP, WITHDRAWAL_DELAY, AUTH_EXPIRY
        );
    }

    // Packed layout: windowSpent is uint88, so dailyCap must fit it.
    function test_constructor_rejectsDailyCapAboveUint88() public {
        vm.expectRevert(PaymentSettlement.InvalidParameters.selector);
        new PaymentSettlement(
            address(token),
            address(registry),
            operator,
            PER_PAYMENT_CAP,
            uint256(type(uint88).max) + 1,
            WITHDRAWAL_DELAY,
            AUTH_EXPIRY
        );
    }

    function test_testToken_rejectsZeroOwner() public {
        vm.expectRevert(TestUSDC.ZeroAddress.selector);
        new TestUSDC(address(0));
    }
}

/// A token that returns nothing from transfer, like some mainnet tokens.
contract NoReturnToken {
    mapping(address => uint256) public balanceOf;

    function mint(address to, uint256 amount) external {
        balanceOf[to] += amount;
    }

    function transfer(address to, uint256 amount) external {
        balanceOf[msg.sender] -= amount;
        balanceOf[to] += amount;
    }

    function transferFrom(address from, address to, uint256 amount) external {
        balanceOf[from] -= amount;
        balanceOf[to] += amount;
    }
}

/// A token that returns false instead of reverting.
contract FalseToken {
    function transfer(address, uint256) external pure returns (bool) {
        return false;
    }

    function transferFrom(address, address, uint256) external pure returns (bool) {
        return false;
    }
}

contract TokenReturnValueTest is SettlementBase {
    function test_token_noReturnValueAccepted() public {
        NoReturnToken t = new NoReturnToken();
        PaymentSettlement s =
            new PaymentSettlement(address(t), address(registry), operator, 50e6, 200e6, WITHDRAWAL_DELAY, AUTH_EXPIRY);
        t.mint(operator, 5e6);
        vm.startPrank(operator);
        s.depositFor(device, 5e6, renter);
        s.closeAccount(device);
        vm.stopPrank();
        vm.warp(block.timestamp + WITHDRAWAL_DELAY);
        s.executeWithdrawal(device);
        assertEq(t.balanceOf(renter), 5e6);
    }

    function test_token_falseReturnRejected() public {
        FalseToken t = new FalseToken();
        PaymentSettlement s =
            new PaymentSettlement(address(t), address(registry), operator, 50e6, 200e6, WITHDRAWAL_DELAY, AUTH_EXPIRY);
        vm.prank(operator);
        vm.expectRevert(IPaymentSettlement.TransferFailed.selector);
        s.depositFor(device, 1, renter);
    }
}
