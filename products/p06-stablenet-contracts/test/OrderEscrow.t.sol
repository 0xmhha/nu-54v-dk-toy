// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Test} from "forge-std/Test.sol";
import {OrderEscrow} from "../src/OrderEscrow.sol";
import {PaymentTypesV2 as Types} from "../src/PaymentTypesV2.sol";
import {TestUSDC3009} from "../src/test-token/TestUSDC3009.sol";

contract OrderEscrowTest is Test {
    uint256 internal constant MERCHANT_KEY = 201;
    uint256 internal constant WALLET_KEY = 202; // the board account
    uint256 internal constant PHONE_KEY = 203;
    uint256 internal constant STRANGER_KEY = 204;
    bytes32 internal constant MERCHANT = bytes32(uint256(50));
    uint256 internal constant PRICE = 6_500_000;
    uint256 internal constant DEPOSIT = 1_000_000_000;

    TestUSDC3009 internal token;
    OrderEscrow internal escrow;
    address internal store = makeAddr("store payout");
    address internal arbiter = makeAddr("arbiter");
    address internal relay = makeAddr("relay");
    address internal wallet;
    address internal phone;
    uint256 internal start = 1_800_000_000;

    event ClaimSkipped(bytes32 indexed merchantId, bytes32 indexed orderId, OrderEscrow.SkipReason reason);

    function setUp() public {
        vm.chainId(8283);
        vm.warp(start);
        wallet = vm.addr(WALLET_KEY);
        phone = vm.addr(PHONE_KEY);
        token = new TestUSDC3009(address(this));
        escrow = new OrderEscrow(address(token), address(this), arbiter);
        escrow.configureMerchant(MERCHANT, vm.addr(MERCHANT_KEY), store, 2 days, 7 days);
        token.mint(wallet, DEPOSIT);
    }

    // ---- helpers ----

    function quoteFor(uint256 order) internal view returns (Types.MerchantQuote memory q) {
        q.quoteId = bytes32(order + 1000);
        q.tenantCommitment = bytes32(uint256(2));
        q.branchCommitment = bytes32(uint256(3));
        q.orderId = bytes32(order);
        q.merchantId = MERCHANT;
        q.claimCommitment = bytes32(uint256(4));
        q.intentCommitment = bytes32(uint256(5));
        q.payer = wallet;
        q.asset = address(token);
        q.beneficiary = store;
        q.amount = PRICE;
        q.nonce = order;
        q.merchantKeyVersion = 1;
        q.beneficiaryVersion = 1;
        q.payerGeneration = 1;
        q.issuedAt = uint64(vm.getBlockTimestamp());
        q.expiresAt = uint64(vm.getBlockTimestamp() + 120);
        q.mode = 1;
        q.payerRole = 1;
    }

    function sig(uint256 key, bytes32 digest) internal pure returns (bytes memory) {
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(key, digest);
        return abi.encodePacked(r, s, v);
    }

    function walletSig(uint256 key, Types.MerchantQuote memory q, address confirmer)
        internal
        view
        returns (bytes memory)
    {
        bytes32 structHash = keccak256(
            abi.encode(
                token.RECEIVE_WITH_AUTHORIZATION_TYPEHASH(),
                q.payer,
                address(escrow),
                q.amount,
                uint256(0),
                uint256(q.expiresAt),
                escrow.lockNonce(q, confirmer)
            )
        );
        return sig(key, keccak256(abi.encodePacked(hex"1901", token.DOMAIN_SEPARATOR(), structHash)));
    }

    function lockOrder(uint256 order) internal returns (bytes32) {
        Types.MerchantQuote memory q = quoteFor(order);
        bytes memory m = sig(MERCHANT_KEY, escrow.quoteDigest(q));
        bytes memory w = walletSig(WALLET_KEY, q, phone);
        vm.prank(relay);
        escrow.lock(q, phone, m, w);
        return q.orderId;
    }

    function action(uint256 key, bytes32 typehash, bytes32 order) internal view returns (bytes memory) {
        return sig(key, escrow.actionDigest(typehash, MERCHANT, order));
    }

    function deliver(bytes32 order) internal {
        escrow.markDelivered(MERCHANT, order, action(MERCHANT_KEY, escrow.DELIVERY_TYPEHASH(), order));
    }

    function state(bytes32 order) internal view returns (OrderEscrow.State s) {
        (,,,,,,, s) = escrow.orders(MERCHANT, order);
    }

    function ids(bytes32 a, bytes32 b, bytes32 c) internal pure returns (bytes32[] memory list) {
        list = new bytes32[](3);
        (list[0], list[1], list[2]) = (a, b, c);
    }

    function assertBacked() internal view {
        assertEq(token.balanceOf(address(escrow)), escrow.totalHeld(), "held amount differs from the balance");
    }

    // ---- paying ----

    function testDepositIsAPlainTransferAndOnlyTheOrderAmountIsHeld() public {
        assertEq(token.balanceOf(address(escrow)), 0, "a deposit must not touch the escrow");
        lockOrder(1);
        assertEq(token.balanceOf(wallet), DEPOSIT - PRICE);
        assertEq(token.balanceOf(address(escrow)), PRICE);
        assertEq(token.balanceOf(store), 0, "the store must not be paid yet");
        assertEq(token.allowance(wallet, address(escrow)), 0, "no standing allowance");
        assertEq(wallet.balance, 0, "the wallet account needs no gas");
        assertBacked();
    }

    function testOneOrderIsPaidOnce() public {
        Types.MerchantQuote memory q = quoteFor(1);
        bytes memory m = sig(MERCHANT_KEY, escrow.quoteDigest(q));
        bytes memory w = walletSig(WALLET_KEY, q, phone);
        escrow.lock(q, phone, m, w);
        vm.expectRevert(OrderEscrow.OrderAlreadyPaid.selector);
        escrow.lock(q, phone, m, w);
    }

    function testRelayCannotSwapThePhoneKeyOrTheQuote() public {
        Types.MerchantQuote memory q = quoteFor(1);
        bytes memory m = sig(MERCHANT_KEY, escrow.quoteDigest(q));
        bytes memory w = walletSig(WALLET_KEY, q, phone);
        vm.expectRevert(TestUSDC3009.InvalidAuthorization.selector);
        escrow.lock(q, vm.addr(STRANGER_KEY), m, w);

        q.amount = PRICE + 1;
        m = sig(MERCHANT_KEY, escrow.quoteDigest(q));
        vm.expectRevert(TestUSDC3009.InvalidAuthorization.selector);
        escrow.lock(q, phone, m, w);
    }

    function testOnlyTheMerchantQuoteAndTheWalletSignatureCount() public {
        Types.MerchantQuote memory q = quoteFor(1);
        bytes memory m = sig(STRANGER_KEY, escrow.quoteDigest(q));
        bytes memory w = walletSig(WALLET_KEY, q, phone);
        vm.expectRevert(OrderEscrow.InvalidSignature.selector);
        escrow.lock(q, phone, m, w);

        m = sig(MERCHANT_KEY, escrow.quoteDigest(q));
        w = walletSig(STRANGER_KEY, q, phone);
        vm.expectRevert(TestUSDC3009.InvalidAuthorization.selector);
        escrow.lock(q, phone, m, w);
    }

    function testExpiredQuoteAndInactiveMerchantAreRefused() public {
        Types.MerchantQuote memory q = quoteFor(1);
        bytes memory m = sig(MERCHANT_KEY, escrow.quoteDigest(q));
        bytes memory w = walletSig(WALLET_KEY, q, phone);
        vm.warp(q.expiresAt);
        vm.expectRevert(OrderEscrow.InvalidTime.selector);
        escrow.lock(q, phone, m, w);

        vm.warp(start);
        escrow.setMerchantActive(MERCHANT, false);
        vm.expectRevert(OrderEscrow.InvalidMerchant.selector);
        escrow.lock(q, phone, m, w);
    }

    function testTheStoreCannotBeItsOwnBuyer() public {
        Types.MerchantQuote memory q = quoteFor(1);
        bytes memory m = sig(MERCHANT_KEY, escrow.quoteDigest(q));
        address signer = vm.addr(MERCHANT_KEY);
        bytes memory w = walletSig(WALLET_KEY, q, signer);
        vm.expectRevert(OrderEscrow.InvalidParameters.selector);
        escrow.lock(q, signer, m, w);
        w = walletSig(WALLET_KEY, q, store);
        vm.expectRevert(OrderEscrow.InvalidParameters.selector);
        escrow.lock(q, store, m, w);
    }

    function testNobodyElseCanUseTheWalletAuthorization() public {
        Types.MerchantQuote memory q = quoteFor(1);
        bytes memory w = walletSig(WALLET_KEY, q, phone);
        (bytes32 r, bytes32 s) = abi.decode(w, (bytes32, bytes32));
        bytes32 nonce = escrow.lockNonce(q, phone);
        vm.prank(relay);
        vm.expectRevert(TestUSDC3009.CallerNotRecipient.selector);
        token.receiveWithAuthorization(wallet, address(escrow), PRICE, 0, q.expiresAt, nonce, uint8(w[64]), r, s);
    }

    // ---- receipt ----

    function testReceiptPaysTheStoreAndIsFinal() public {
        bytes32 order = lockOrder(1);
        deliver(order);
        escrow.release(MERCHANT, order, action(PHONE_KEY, escrow.RECEIPT_TYPEHASH(), order));
        assertEq(token.balanceOf(store), PRICE);
        assertEq(uint8(state(order)), uint8(OrderEscrow.State.Paid));
        assertBacked();

        bytes memory c = action(MERCHANT_KEY, escrow.CANCEL_TYPEHASH(), order);
        vm.expectRevert(OrderEscrow.InvalidState.selector);
        escrow.cancel(MERCHANT, order, c);
        bytes memory d = action(PHONE_KEY, escrow.DISPUTE_TYPEHASH(), order);
        vm.expectRevert(OrderEscrow.InvalidState.selector);
        escrow.dispute(MERCHANT, order, d);
    }

    function testPhoneAloneCannotPayBeforeTheStoreMarksDelivery() public {
        bytes32 order = lockOrder(1);
        bytes memory r = action(PHONE_KEY, escrow.RECEIPT_TYPEHASH(), order);
        vm.expectRevert(OrderEscrow.InvalidState.selector);
        escrow.release(MERCHANT, order, r);
    }

    function testStoreAloneCannotConfirmReceipt() public {
        bytes32 order = lockOrder(1);
        deliver(order);
        bytes memory r = action(MERCHANT_KEY, escrow.RECEIPT_TYPEHASH(), order);
        vm.expectRevert(OrderEscrow.InvalidSignature.selector);
        escrow.release(MERCHANT, order, r);
    }

    function testASignatureForOneActionOrOrderDoesNotWorkForAnother() public {
        bytes32 order = lockOrder(1);
        bytes32 other = lockOrder(2);
        deliver(order);
        deliver(other);
        bytes memory r = action(PHONE_KEY, escrow.RECEIPT_TYPEHASH(), order);
        vm.expectRevert(OrderEscrow.InvalidSignature.selector);
        escrow.release(MERCHANT, other, r);
        vm.expectRevert(OrderEscrow.InvalidSignature.selector);
        escrow.dispute(MERCHANT, order, r);
    }

    // ---- store claim ----

    function testClaimWaitsFromDeliveryNotFromPayment() public {
        bytes32 order = lockOrder(1);
        vm.warp(start + 3 days); // longer than the window, but not delivered yet
        vm.expectRevert(OrderEscrow.InvalidState.selector);
        escrow.claim(MERCHANT, order);

        deliver(order);
        vm.warp(start + 5 days - 1);
        vm.expectRevert(OrderEscrow.InvalidState.selector);
        escrow.claim(MERCHANT, order);

        vm.warp(start + 5 days);
        vm.prank(relay); // anyone may send it; only the store is paid
        escrow.claim(MERCHANT, order);
        assertEq(token.balanceOf(store), PRICE);
        assertEq(token.balanceOf(relay), 0);
        assertBacked();
    }

    function testNothingIsPaidUntilSomebodyClaims() public {
        bytes32 order = lockOrder(1);
        deliver(order);
        vm.warp(start + 365 days);
        assertEq(token.balanceOf(store), 0);
        assertEq(uint8(escrow.claimStatus(MERCHANT, order)), uint8(OrderEscrow.SkipReason.None));
    }

    function testBatchPaysEligibleOrdersAndSkipsTheRest() public {
        bytes32 a = lockOrder(1);
        bytes32 b = lockOrder(2);
        bytes32 disputed = lockOrder(3);
        bytes32 undelivered = lockOrder(4);
        bytes32 late = lockOrder(5);
        deliver(a);
        deliver(b);
        deliver(disputed);
        escrow.dispute(MERCHANT, disputed, action(PHONE_KEY, escrow.DISPUTE_TYPEHASH(), disputed));
        vm.warp(start + 1 days);
        deliver(late);
        vm.warp(start + 2 days);

        bytes32[] memory list = new bytes32[](7);
        (list[0], list[1], list[2], list[3]) = (a, disputed, undelivered, late);
        (list[4], list[5], list[6]) = (bytes32(uint256(999)), a, b);
        vm.expectEmit(address(escrow));
        emit ClaimSkipped(MERCHANT, disputed, OrderEscrow.SkipReason.Disputed);
        vm.expectEmit(address(escrow));
        emit ClaimSkipped(MERCHANT, undelivered, OrderEscrow.SkipReason.NotDelivered);
        vm.expectEmit(address(escrow));
        emit ClaimSkipped(MERCHANT, late, OrderEscrow.SkipReason.WindowOpen);
        vm.expectEmit(address(escrow));
        emit ClaimSkipped(MERCHANT, bytes32(uint256(999)), OrderEscrow.SkipReason.Unknown);
        vm.expectEmit(address(escrow));
        emit ClaimSkipped(MERCHANT, a, OrderEscrow.SkipReason.Settled);
        vm.prank(relay);
        assertEq(escrow.claimBatch(MERCHANT, store, list), 2 * PRICE);

        assertEq(token.balanceOf(store), 2 * PRICE, "a repeated order must be paid once");
        assertEq(uint8(state(disputed)), uint8(OrderEscrow.State.Disputed));
        assertBacked();

        vm.expectRevert(OrderEscrow.NothingToClaim.selector); // a retry pays nothing
        escrow.claimBatch(MERCHANT, store, ids(a, b, disputed));
    }

    function testBatchCannotRedirectThePayout() public {
        bytes32 a = lockOrder(1);
        deliver(a);
        vm.warp(start + 2 days);
        bytes32[] memory list = new bytes32[](1);
        list[0] = a;
        vm.expectRevert(OrderEscrow.NothingToClaim.selector);
        escrow.claimBatch(MERCHANT, relay, list);
    }

    function testBatchSizeIsBounded() public {
        vm.expectRevert(OrderEscrow.InvalidParameters.selector);
        escrow.claimBatch(MERCHANT, store, new bytes32[](0));
        vm.expectRevert(OrderEscrow.InvalidParameters.selector);
        escrow.claimBatch(MERCHANT, store, new bytes32[](101));
    }

    function testFullBatchOfOneHundred() public {
        bytes32[] memory list = new bytes32[](100);
        for (uint256 i = 0; i < 100; ++i) {
            list[i] = lockOrder(i + 1);
            deliver(list[i]);
        }
        vm.warp(start + 2 days);
        vm.cool(address(escrow)); // a real transaction starts with cold storage
        vm.cool(address(token));
        uint256 gasBefore = gasleft();
        escrow.claimBatch(MERCHANT, store, list);
        emit log_named_uint("claimBatch(100) execution gas", gasBefore - gasleft());
        assertEq(token.balanceOf(store), 100 * PRICE);
        assertEq(escrow.totalHeld(), 0);
    }

    // ---- dispute, cancel, not delivered ----

    function testDisputeStopsTheClaimAndTheArbiterDecides() public {
        bytes32 order = lockOrder(1);
        deliver(order);
        escrow.dispute(MERCHANT, order, action(PHONE_KEY, escrow.DISPUTE_TYPEHASH(), order));
        vm.warp(start + 30 days);
        vm.expectRevert(OrderEscrow.InvalidState.selector);
        escrow.claim(MERCHANT, order);
        vm.expectRevert(OrderEscrow.Unauthorized.selector);
        escrow.resolve(MERCHANT, order, true);
        vm.prank(arbiter);
        escrow.resolve(MERCHANT, order, false);
        assertEq(token.balanceOf(wallet), DEPOSIT);
        assertBacked();
    }

    function testArbiterCanPayTheStoreAndOnlyDisputedOrders() public {
        bytes32 order = lockOrder(1);
        deliver(order);
        vm.prank(arbiter);
        vm.expectRevert(OrderEscrow.InvalidState.selector);
        escrow.resolve(MERCHANT, order, true);
        escrow.dispute(MERCHANT, order, action(PHONE_KEY, escrow.DISPUTE_TYPEHASH(), order));
        vm.prank(arbiter);
        escrow.resolve(MERCHANT, order, true);
        assertEq(token.balanceOf(store), PRICE);
    }

    function testDisputeClosesWhenTheStoreMayClaim() public {
        bytes32 order = lockOrder(1);
        deliver(order);
        vm.warp(start + 2 days);
        bytes memory d = action(PHONE_KEY, escrow.DISPUTE_TYPEHASH(), order);
        vm.expectRevert(OrderEscrow.InvalidTime.selector);
        escrow.dispute(MERCHANT, order, d);
    }

    function testStoreCancelReturnsTheMoneyToTheWalletAccount() public {
        bytes32 order = lockOrder(1);
        bytes memory byBuyer = action(PHONE_KEY, escrow.CANCEL_TYPEHASH(), order);
        vm.expectRevert(OrderEscrow.InvalidSignature.selector); // the buyer cannot cancel
        escrow.cancel(MERCHANT, order, byBuyer);
        escrow.cancel(MERCHANT, order, action(MERCHANT_KEY, escrow.CANCEL_TYPEHASH(), order));
        assertEq(token.balanceOf(wallet), DEPOSIT);
        assertBacked();
    }

    function testUndeliveredOrderIsRefundedAndCannotBeDeliveredLate() public {
        bytes32 order = lockOrder(1);
        vm.warp(start + 7 days - 1);
        vm.expectRevert(OrderEscrow.InvalidTime.selector);
        escrow.refundUndelivered(MERCHANT, order);
        vm.warp(start + 7 days);
        bytes memory m = action(MERCHANT_KEY, escrow.DELIVERY_TYPEHASH(), order);
        vm.expectRevert(OrderEscrow.InvalidTime.selector);
        escrow.markDelivered(MERCHANT, order, m);
        vm.prank(relay);
        escrow.refundUndelivered(MERCHANT, order);
        assertEq(token.balanceOf(wallet), DEPOSIT);
        assertBacked();
    }

    // ---- settings and the token ----

    function testMerchantChangesDoNotMoveExistingOrders() public {
        bytes32 order = lockOrder(1);
        address nextStore = makeAddr("next payout");
        escrow.configureMerchant(MERCHANT, vm.addr(MERCHANT_KEY), nextStore, 30 days, 30 days);
        escrow.setMerchantActive(MERCHANT, false);
        deliver(order);
        vm.warp(start + 2 days);
        escrow.claim(MERCHANT, order);
        assertEq(token.balanceOf(store), PRICE);
        assertEq(token.balanceOf(nextStore), 0);
    }

    function testOnlyTheGovernorChangesSettings() public {
        vm.startPrank(relay);
        vm.expectRevert(OrderEscrow.Unauthorized.selector);
        escrow.configureMerchant(MERCHANT, relay, relay, 2 days, 7 days);
        vm.expectRevert(OrderEscrow.Unauthorized.selector);
        escrow.setMerchantActive(MERCHANT, false);
        vm.expectRevert(OrderEscrow.Unauthorized.selector);
        escrow.setArbiter(relay);
        vm.stopPrank();
        vm.expectRevert(OrderEscrow.InvalidParameters.selector);
        escrow.configureMerchant(MERCHANT, vm.addr(MERCHANT_KEY), store, 1 hours - 1, 7 days);
    }

    function testAHeldPayoutIsNotRecordedAsPaid() public {
        bytes32 order = lockOrder(1);
        deliver(order);
        vm.warp(start + 2 days);
        vm.prank(store);
        token.setRefusalMode(true);
        vm.expectRevert(OrderEscrow.TokenBalanceMismatch.selector);
        escrow.claim(MERCHANT, order);
        assertEq(uint8(state(order)), uint8(OrderEscrow.State.Delivered));

        token.setTrustedSender(address(escrow), true);
        escrow.claim(MERCHANT, order);
        assertEq(token.balanceOf(store), PRICE);
    }

    function testReturnToTheOriginalWalletIsASignedPlainTransfer() public {
        address home = makeAddr("original wallet");
        bytes32 nonce = bytes32(uint256(77));
        uint256 until = start + 120;
        bytes32 structHash = keccak256(
            abi.encode(token.TRANSFER_WITH_AUTHORIZATION_TYPEHASH(), wallet, home, DEPOSIT, uint256(0), until, nonce)
        );
        (uint8 v, bytes32 r, bytes32 s) =
            vm.sign(WALLET_KEY, keccak256(abi.encodePacked(hex"1901", token.DOMAIN_SEPARATOR(), structHash)));
        vm.prank(relay);
        token.transferWithAuthorization(wallet, home, DEPOSIT, 0, until, nonce, v, r, s);
        assertEq(token.balanceOf(home), DEPOSIT);
        vm.expectRevert(TestUSDC3009.AuthorizationAlreadyUsed.selector);
        token.transferWithAuthorization(wallet, home, DEPOSIT, 0, until, nonce, v, r, s);
    }

    function testAuthorizationCannotGoToAnotherAddress() public {
        address home = makeAddr("original wallet");
        uint256 until = start + 120;
        bytes32 structHash = keccak256(
            abi.encode(
                token.TRANSFER_WITH_AUTHORIZATION_TYPEHASH(),
                wallet,
                home,
                PRICE,
                uint256(0),
                until,
                bytes32(uint256(1))
            )
        );
        (uint8 v, bytes32 r, bytes32 s) =
            vm.sign(WALLET_KEY, keccak256(abi.encodePacked(hex"1901", token.DOMAIN_SEPARATOR(), structHash)));
        vm.expectRevert(TestUSDC3009.InvalidAuthorization.selector);
        token.transferWithAuthorization(wallet, relay, PRICE, 0, until, bytes32(uint256(1)), v, r, s);
        vm.warp(until);
        vm.expectRevert(TestUSDC3009.AuthorizationExpired.selector);
        token.transferWithAuthorization(wallet, home, PRICE, 0, until, bytes32(uint256(1)), v, r, s);
    }

    /// @dev Any mix of outcomes leaves exactly the open orders' money in the contract.
    function testFuzzHeldMoneyAlwaysMatchesOpenOrders(uint8[12] calldata outcomes) public {
        uint256 open;
        for (uint256 i = 0; i < outcomes.length; ++i) {
            bytes32 order = lockOrder(i + 1);
            uint256 outcome = outcomes[i] % 6;
            if (outcome == 1) escrow.cancel(MERCHANT, order, action(MERCHANT_KEY, escrow.CANCEL_TYPEHASH(), order));
            if (outcome >= 2) deliver(order);
            if (outcome == 3) escrow.release(MERCHANT, order, action(PHONE_KEY, escrow.RECEIPT_TYPEHASH(), order));
            if (outcome >= 4) escrow.dispute(MERCHANT, order, action(PHONE_KEY, escrow.DISPUTE_TYPEHASH(), order));
            if (outcome == 5) {
                vm.prank(arbiter);
                escrow.resolve(MERCHANT, order, i % 2 == 0);
            }
            if (outcome == 0 || outcome == 2 || outcome == 4) open += PRICE;
        }
        assertEq(escrow.totalHeld(), open);
        assertBacked();
        assertEq(token.balanceOf(wallet) + token.balanceOf(store) + open, DEPOSIT, "no money appears or vanishes");
    }
}
