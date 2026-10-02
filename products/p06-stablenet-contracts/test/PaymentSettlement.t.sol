// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {SettlementBase} from "./Base.t.sol";
import {IPaymentSettlement} from "../src/IPaymentSettlement.sol";
import {IPaymentSettlementExtensions} from "../src/IPaymentSettlementExtensions.sol";
import {PaymentTypes} from "../src/PaymentTypes.sol";

/// Test names follow docs/content/products/p06/srs.md (P06-FR-*).
contract PaymentSettlementTest is SettlementBase {
    bytes32 internal constant ORDER = bytes32(uint256(0x0101));

    // P06-FR-01: later deposits pass the same address or zero; a different one is refused
    function test_depositFor_withdrawAddressImmutable() public {
        assertEq(settlement.balanceOf(device), 150e6);
        vm.startPrank(operator);
        token.mint(operator, 2e6);
        token.approve(address(settlement), 2e6);
        settlement.depositFor(device, 1e6, renter);
        settlement.depositFor(device, 1e6, address(0));
        vm.expectRevert(IPaymentSettlementExtensions.WithdrawAddressMismatch.selector);
        settlement.depositFor(device, 1e6, makeAddr("other"));
        vm.stopPrank();
        assertEq(settlement.balanceOf(device), 152e6);
        assertEq(settlement.accountOf(device).withdrawAddress, renter);
    }

    function test_depositFor_onlyOperator() public {
        vm.expectRevert(IPaymentSettlement.NotOperator.selector);
        settlement.depositFor(device, 1, renter);
    }

    // P06-FR-02, P06-FR-11
    function test_settle_emitsPaymentSettled() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(45e5, ORDER, 1);
        bytes memory sig = _sign(deviceKey, a);
        vm.expectEmit(true, true, true, true);
        emit IPaymentSettlement.PaymentSettled(merchant, ORDER, device, 45e5, 1);
        _settle(a, sig);
        assertEq(settlement.balanceOf(device), 150e6 - 45e5);
        assertEq(settlement.merchantBalance(merchant), 45e5);
        assertTrue(settlement.isPaid(merchant, ORDER));
        assertTrue(settlement.isNonceUsed(device, 1));
    }

    function test_settle_rejectsUnknownSigner() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(1e6, ORDER, 1);
        bytes memory sig = _sign(0xBAD, a);
        vm.expectRevert(IPaymentSettlement.AccountInactive.selector);
        _settle(a, sig);
    }

    function test_settle_highSIsRejected() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(1e6, ORDER, 1);
        bytes memory sig = _sign(deviceKey, a);
        bytes32 r;
        bytes32 s;
        assembly {
            r := mload(add(sig, 32))
            s := mload(add(sig, 64))
        }
        uint256 n = 0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141;
        uint8 v = uint8(sig[64]) == 27 ? 28 : 27; // (r, n - s, flipped v) is the same signature, high-s form
        bytes memory malleable = abi.encodePacked(r, bytes32(n - uint256(s)), v);
        vm.expectRevert(IPaymentSettlement.AccountInactive.selector);
        _settle(a, malleable);
    }

    // P06-FR-04
    function test_settle_rejectsWrongDomain() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(1e6, ORDER, 1);
        a.chainId = 1;
        bytes memory sig = _sign(deviceKey, a);
        vm.expectRevert(IPaymentSettlement.WrongDomain.selector);
        _settle(a, sig);
    }

    function test_settle_rejectsWrongToken() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(1e6, ORDER, 1);
        a.token = makeAddr("otherToken");
        bytes memory sig = _sign(deviceKey, a);
        vm.expectRevert(IPaymentSettlement.WrongDomain.selector);
        _settle(a, sig);
    }

    // P06-FR-05
    function test_settle_revokedMerchant() public {
        vm.prank(registryAdmin);
        registry.revokeMerchant(merchant);
        PaymentTypes.PaymentAuthorization memory a = _auth(1e6, ORDER, 1);
        bytes memory sig = _sign(deviceKey, a);
        vm.expectRevert(IPaymentSettlement.MerchantRevoked.selector);
        _settle(a, sig);
    }

    // P06-FR-17: revocation takes effect for the very next settle
    function test_registry_revokeImmediate() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(1e6, ORDER, 1);
        _settle(a, _sign(deviceKey, a));
        vm.prank(registryAdmin);
        registry.revokeMerchant(merchant);
        PaymentTypes.PaymentAuthorization memory b = _auth(1e6, bytes32(uint256(0x0202)), 2);
        bytes memory sig = _sign(deviceKey, b);
        vm.expectRevert(IPaymentSettlement.MerchantRevoked.selector);
        _settle(b, sig);
    }

    // P06-FR-06
    function test_settle_payoutMismatch() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(1e6, ORDER, 1);
        a.payout = makeAddr("attacker");
        bytes memory sig = _sign(deviceKey, a);
        vm.expectRevert(IPaymentSettlement.MerchantForged.selector);
        _settle(a, sig);
    }

    // P06-FR-07
    function test_settle_overPerPayment() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(PER_PAYMENT_CAP + 1, ORDER, 1);
        bytes memory sig = _sign(deviceKey, a);
        vm.expectRevert(IPaymentSettlement.OverCap.selector);
        _settle(a, sig);
    }

    function test_settle_overDaily() public {
        _deposit(device, 200e6);
        for (uint256 i = 0; i < 4; i++) {
            PaymentTypes.PaymentAuthorization memory ok = _auth(50e6, bytes32(i + 1), i + 1);
            _settle(ok, _sign(deviceKey, ok));
        }
        PaymentTypes.PaymentAuthorization memory a = _auth(1, bytes32(uint256(99)), 99);
        bytes memory sig = _sign(deviceKey, a);
        vm.expectRevert(IPaymentSettlement.OverCap.selector);
        _settle(a, sig);
        vm.warp(block.timestamp + 1 days); // new fixed window
        a = _auth(1, bytes32(uint256(99)), 99);
        _settle(a, _sign(deviceKey, a));
    }

    // P06-FR-08
    function test_settle_nonceReplay() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(1e6, ORDER, 7);
        _settle(a, _sign(deviceKey, a));
        PaymentTypes.PaymentAuthorization memory b = _auth(1e6, bytes32(uint256(0x0202)), 7);
        bytes memory sig = _sign(deviceKey, b);
        vm.expectRevert(IPaymentSettlement.NonceReplayed.selector);
        _settle(b, sig);
    }

    function testFuzz_nonce_anyOrder(uint256 n1, uint256 n2, uint256 n3) public {
        vm.assume(n1 != n2 && n2 != n3 && n1 != n3);
        uint256[3] memory nonces = [n1, n2, n3];
        for (uint256 i = 0; i < 3; i++) {
            PaymentTypes.PaymentAuthorization memory a = _auth(1e6, bytes32(i + 10), nonces[i]);
            _settle(a, _sign(deviceKey, a));
            assertTrue(settlement.isNonceUsed(device, nonces[i]));
        }
    }

    // P06-FR-09
    function test_settle_expired() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(1e6, ORDER, 1);
        bytes memory sig = _sign(deviceKey, a);
        vm.warp(a.expiry + 1);
        vm.expectRevert(IPaymentSettlement.Expired.selector);
        _settle(a, sig);
    }

    function test_settle_expiryTooFar() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(1e6, ORDER, 1);
        a.expiry = uint64(block.timestamp + AUTH_EXPIRY + 1);
        bytes memory sig = _sign(deviceKey, a);
        vm.expectRevert(IPaymentSettlement.Expired.selector);
        _settle(a, sig);
    }

    // P06-FR-10: resubmitting a settled signature ends in OrderAlreadyPaid, even after expiry
    function test_settle_resubmitAfterExpiryIsAlreadyPaid() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(1e6, ORDER, 1);
        bytes memory sig = _sign(deviceKey, a);
        _settle(a, sig);
        vm.expectRevert(IPaymentSettlement.OrderAlreadyPaid.selector);
        _settle(a, sig);
        vm.warp(a.expiry + 1000);
        vm.expectRevert(IPaymentSettlement.OrderAlreadyPaid.selector);
        _settle(a, sig);
    }

    // P06-FR-11
    function test_settle_insufficientBalance() public {
        address poor = vm.addr(0xB00);
        _deposit(poor, 1e6);
        PaymentTypes.PaymentAuthorization memory a = _auth(2e6, ORDER, 1);
        bytes memory sig = _sign(0xB00, a);
        vm.expectRevert(IPaymentSettlement.InsufficientBalance.selector);
        _settle(a, sig);
    }

    // P06-FR-12
    function test_cashOut_toPayoutOnly() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(45e5, ORDER, 1);
        _settle(a, _sign(deviceKey, a));
        vm.prank(merchant);
        settlement.cashOut();
        assertEq(token.balanceOf(payout), 45e5);
        assertEq(settlement.merchantBalance(merchant), 0);
    }

    // P06-NFR-02: a whole settle transaction stays within settleGasEstimate (130,000),
    // which kioskMinGasBalance is derived from. Sandbox first payment: 165,399 before the
    // storage packing, 120,188 after it (2026-10-01); testnet first payment 120,888.
    function test_settle_txGasRecorded() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(45e5, ORDER, 1);
        bytes memory sig = _sign(deviceKey, a);
        bytes memory data = abi.encodeCall(settlement.settle, (a, sig));
        uint256 calldataGas;
        for (uint256 i = 0; i < data.length; i++) {
            calldataGas += data[i] == 0 ? 4 : 16;
        }
        vm.prank(kiosk);
        uint256 before = gasleft();
        settlement.settle(a, sig);
        uint256 execution = before - gasleft();
        uint256 txGas = 21_000 + calldataGas + execution;
        emit log_named_uint("settle execution gas", execution);
        emit log_named_uint("settle transaction gas (estimate)", txGas);
        assertLe(txGas, 130_000, "settle exceeds settleGasEstimate");
    }

    // P06-FR-01
    function test_depositFor_createsAccount() public {
        address fresh = vm.addr(0xF00D);
        assertEq(settlement.balanceOf(fresh), 0);
        _deposit(fresh, 10e6);
        assertEq(settlement.balanceOf(fresh), 10e6);
        assertEq(token.balanceOf(address(settlement)), 160e6);
    }

    // P06-FR-02
    function test_settle_recoversDeviceSigner() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(1e6, ORDER, 1);
        _settle(a, _sign(deviceKey, a));
        assertEq(settlement.balanceOf(device), 149e6); // debited from the signer's account
    }

    // P06-FR-10
    function test_settle_orderAlreadyPaid() public {
        PaymentTypes.PaymentAuthorization memory a = _auth(1e6, ORDER, 1);
        _settle(a, _sign(deviceKey, a));
        PaymentTypes.PaymentAuthorization memory b = _auth(2e6, ORDER, 2); // same order, new signature
        bytes memory sig = _sign(deviceKey, b);
        vm.expectRevert(IPaymentSettlement.OrderAlreadyPaid.selector);
        _settle(b, sig);
    }

    // P06-NFR-01: no role can move a device balance anywhere except through a device-signed payment
    function test_noAdminDrain() public {
        address attacker = makeAddr("attacker");
        uint256 held = token.balanceOf(address(settlement));
        vm.prank(registryAdmin);
        registry.registerMerchant(attacker, attacker);
        vm.prank(attacker);
        settlement.cashOut(); // nothing owed to the attacker
        PaymentTypes.PaymentAuthorization memory a = _auth(1e6, ORDER, 1);
        a.merchant = attacker;
        a.payout = attacker;
        bytes memory forged = _sign(0xA11CE, a); // not the device key
        vm.expectRevert(IPaymentSettlement.AccountInactive.selector);
        _settle(a, forged);
        vm.prank(operator);
        settlement.cashOut(); // the operator is not a merchant: nothing owed, nothing moves
        assertEq(settlement.balanceOf(device), 150e6);
        assertEq(token.balanceOf(address(settlement)), held);
        assertEq(token.balanceOf(attacker), 0);
    }

    function testFuzz_settle_amountWithinCaps(uint256 amount) public {
        amount = bound(amount, 1, PER_PAYMENT_CAP);
        PaymentTypes.PaymentAuthorization memory a = _auth(amount, ORDER, 1);
        _settle(a, _sign(deviceKey, a));
        assertEq(settlement.balanceOf(device) + settlement.merchantBalance(merchant), 150e6);
    }
}
