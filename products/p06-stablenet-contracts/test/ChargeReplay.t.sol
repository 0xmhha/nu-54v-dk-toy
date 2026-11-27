// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {ChargeBase} from "./ChargeBase.t.sol";
import {ChargeTypes} from "../src/charge/ChargeTypes.sol";
import {ChargeGovernance} from "../src/charge/ChargeGovernance.sol";
import {ChargeDeposit} from "../src/charge/ChargeDeposit.sol";
import {WonPool} from "../src/charge/WonPool.sol";
import {TestChargeUSDC} from "../src/test-token/TestChargeUSDC.sol";

/// @notice Nothing signed for one chain, one deployment or one state of a deposit works for another. The twin
/// contracts here are second deployments with the very same keys and settings.
contract ChargeReplayTest is ChargeBase {
    ChargeDeposit internal twinDeposit;
    WonPool internal twinPool;

    function setUp() public override {
        super.setUp();
        toSource();
        twinDeposit = new ChargeDeposit(keys(), depositConfig(address(pool)));
        toWon();
        twinPool = new WonPool(keys(), poolConfig(address(deposit)));
        fundTwin();
        toSource();
    }

    function fundTwin() internal {
        WonPool first = pool;
        pool = twinPool;
        fundPool(POOL_SIZE);
        pool = first;
    }

    // ---- the traveller's authorization ----

    function testAUsedAuthorizationIsNeverTakenAgain() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes memory quoteSig = quoteSignature(quote);
        bytes memory auth = authorization(quote, PAYER_KEY);
        bytes32 id = deposit.depositWithAuthorization(quote, quoteSig, auth);
        vm.warp(block.timestamp + 7 days);
        deposit.refundSelf(id);
        // The refunded money is back in the wallet, and the old signatures cannot take it a second time.
        vm.expectRevert(ChargeDeposit.AlreadyDeposited.selector);
        deposit.depositWithAuthorization(quote, quoteSig, auth);
        assertEq(usdc.balanceOf(payer), 1_000_000e6);
    }

    function testTheAuthorizationIsForOneDepositContract() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes memory auth = authorization(quote, PAYER_KEY);
        // The quote key signs the same quote for the twin; the wallet never did.
        bytes memory twinQuoteSig = sign(QUOTER_KEY, twinDeposit.quoteDigest(quote));
        vm.expectRevert(TestChargeUSDC.InvalidAuthorization.selector);
        twinDeposit.depositWithAuthorization(quote, twinQuoteSig, auth);
        // And a quote signature for the first contract is not one for the twin.
        bytes memory quoteSig = quoteSignature(quote);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        twinDeposit.depositWithAuthorization(quote, quoteSig, auth);
        assertEq(usdc.balanceOf(payer), 1_000_000e6);
    }

    function testTheAuthorizationIsForOneChainAndOneToken() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = idOf(quote);
        bytes memory auth = authorization(quote, PAYER_KEY);
        (bytes32 r, bytes32 s, uint8 v) = split(auth);

        vm.chainId(1); // the same token address on another chain
        vm.prank(address(deposit));
        vm.expectRevert(TestChargeUSDC.InvalidAuthorization.selector);
        usdc.receiveWithAuthorization(payer, address(deposit), 100e6, 0, quote.expiresAt, id, v, r, s);

        toSource(); // another token on the same chain
        TestChargeUSDC other = new TestChargeUSDC(address(this));
        other.mint(payer, 100e6);
        vm.prank(address(deposit));
        vm.expectRevert(TestChargeUSDC.InvalidAuthorization.selector);
        other.receiveWithAuthorization(payer, address(deposit), 100e6, 0, quote.expiresAt, id, v, r, s);
    }

    // ---- chain and contract are inside every digest ----

    function testDigestsNameTheChainAndTheContract() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 here = deposit.quoteDigest(quote);
        bytes32 action = deposit.actionDigest(keccak256("refund"), bytes32("subject"), 0, 1);
        bytes32 paid = deposit.paidDigest(here, WON_BLOCK, 1);
        bytes32 contest = deposit.contestDigest(here, 1);
        assertTrue(here != twinDeposit.quoteDigest(quote));
        assertTrue(action != twinDeposit.actionDigest(keccak256("refund"), bytes32("subject"), 0, 1));
        assertTrue(paid != twinDeposit.paidDigest(here, WON_BLOCK, 1));
        assertTrue(contest != twinDeposit.contestDigest(here, 1));

        vm.chainId(SOURCE_CHAIN + 1); // the same address on another chain
        assertTrue(here != deposit.quoteDigest(quote));
        assertTrue(action != deposit.actionDigest(keccak256("refund"), bytes32("subject"), 0, 1));
        assertTrue(paid != deposit.paidDigest(here, WON_BLOCK, 1));
        assertTrue(contest != deposit.contestDigest(here, 1));

        toWon();
        bytes32 confirmation = pool.confirmationDigest(here, SOURCE_BLOCK, false, 1);
        assertTrue(confirmation != twinPool.confirmationDigest(here, SOURCE_BLOCK, false, 1));
        vm.chainId(WON_CHAIN + 1);
        assertTrue(confirmation != pool.confirmationDigest(here, SOURCE_BLOCK, false, 1));
        // The payout number is the deposit number of the origin, wherever the pool is.
        assertEq(pool.payoutId(origin(), quote), here);
        assertEq(twinPool.payoutId(origin(), quote), here);
    }

    // ---- the pool ----

    /// A quote names its pool. A second pool with the same keys cannot pay it again.
    function testAQuoteIsPaidByItsOwnPoolOnly() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        depositIt(quote);
        PayArgs memory args = payArgs(quote, false);
        payWith(args);
        vm.prank(vm.addr(RELAY_KEY));
        vm.expectRevert(WonPool.InvalidQuote.selector);
        twinPool.pay(args.origin, args.quote, args.quoteSignature, args.confirmation, args.witnessSignature);
        assertEq(assetBalance(board), 134_500 * WON);
    }

    /// A confirmation is signed for one pool: the same deposit number confirmed for the first pool does not
    /// make the twin pay a quote that names the twin.
    function testAConfirmationIsForOnePool() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        quote.pool = address(twinPool);
        PayArgs memory args = payArgs(quote, false); // confirmed in the first pool's domain
        toWon();
        vm.prank(vm.addr(RELAY_KEY));
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        twinPool.pay(args.origin, args.quote, args.quoteSignature, args.confirmation, args.witnessSignature);

        args.witnessSignature =
            sign(WITNESS_KEY, twinPool.confirmationDigest(idOf(quote), SOURCE_BLOCK, false, args.confirmation.until));
        vm.prank(vm.addr(RELAY_KEY));
        twinPool.pay(args.origin, args.quote, args.quoteSignature, args.confirmation, args.witnessSignature);
        assertEq(assetBalance(board), 134_500 * WON);
        // The first pool refuses the quote that names the twin.
        vm.prank(vm.addr(RELAY_KEY));
        vm.expectRevert(WonPool.InvalidQuote.selector);
        pool.pay(args.origin, args.quote, args.quoteSignature, args.confirmation, args.witnessSignature);
    }

    /// The same quote signed for the twin deposit contract is another deposit number, and the pool does not
    /// know that contract.
    function testAQuoteFromAnotherDepositContractIsNotPaid() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        PayArgs memory args = payArgs(quote, false);
        args.origin.deposit = address(twinDeposit);
        args.quoteSignature = sign(QUOTER_KEY, twinDeposit.quoteDigest(quote));
        toWon();
        args.witnessSignature = sign(
            WITNESS_KEY,
            pool.confirmationDigest(pool.payoutId(args.origin, quote), SOURCE_BLOCK, false, args.confirmation.until)
        );
        vm.prank(vm.addr(RELAY_KEY));
        vm.expectRevert(WonPool.UnknownOrigin.selector);
        pool.pay(args.origin, args.quote, args.quoteSignature, args.confirmation, args.witnessSignature);
    }

    // ---- signers ----

    function testSignerSignaturesAreForOneContractAndOneChain() public {
        bytes32 kind = keccak256("setKey");
        bytes memory arguments = abi.encode(ChargeGovernance.Key.Relay, stranger);
        bytes32 action = keccak256(abi.encode(kind, ChargeGovernance.Key.Relay, stranger, bytes32(0)));
        uint256 until = block.timestamp + 1 days;
        bytes[2] memory signatures = two(deposit, "propose", action, 0, until);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        twinDeposit.propose(kind, arguments, 0, until, signatures);
        toWon();
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        pool.propose(kind, arguments, 0, until, signatures);
        vm.chainId(SOURCE_CHAIN + 1);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.propose(kind, arguments, 0, until, signatures);
        toSource();
        assertEq(deposit.propose(kind, arguments, 0, until, signatures), action);
    }

    /// The one signature that voids every replacement of the pauser is for one contract, one chain and one
    /// round of that seat. It is no cancel of a single change, and a cancel is no such signature.
    function testAVoidOfPauserReplacementsIsForOneContractOneChainAndOneRound() public {
        bytes32 kind = keccak256("voidPauserReplacements");
        bytes memory arguments = abi.encode(ChargeGovernance.Key.Pauser, stranger);
        uint256 until = block.timestamp + 6 days;
        bytes memory signature = sign(SIGNER3_KEY, deposit.actionDigest(kind, 0, 0, until));
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        twinDeposit.voidPauserReplacements(until, signature);
        toWon();
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        pool.voidPauserReplacements(until, signature);
        vm.chainId(SOURCE_CHAIN + 1);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.voidPauserReplacements(until, signature);
        toSource();

        bytes32 action = proposeOn(deposit, "setKey", arguments, "salt");
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.cancelProposal(action, until, signature);
        bytes memory cancel = cancelSignature(deposit, SIGNER3_KEY, action, until);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.voidPauserReplacements(until, cancel);
        // A signature made for the next round does not work in this one.
        bytes memory later = sign(SIGNER3_KEY, deposit.actionDigest(kind, 0, 1, until));
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.voidPauserReplacements(until, later);

        deposit.voidPauserReplacements(until, signature);
        assertEq(deposit.pauserRound(), 1);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.cancelProposal(action, until, cancel); // void already
        // Used once: the same signature does not void what is proposed after it.
        bytes32 second = proposeOn(deposit, "setKey", arguments, "salt 2");
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.voidPauserReplacements(until, signature);
        deposit.voidPauserReplacements(until, later);
        assertEq(deposit.pauserRound(), 2);
        vm.warp(block.timestamp + 24 hours);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.setKey(ChargeGovernance.Key.Pauser, stranger, "salt");
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.setKey(ChargeGovernance.Key.Pauser, stranger, "salt 2");
        assertEq(second, deposit.actionOf(keccak256("setKey"), arguments, "salt 2"));
        // The twin and the pool were never touched.
        assertEq(twinDeposit.pauserRound(), 0);
        assertEq(pool.pauserRound(), 0);
    }

    /// The two signatures that start or stop an emergency exit are for one contract and one chain, although
    /// the twin has the same signers and the same treasury. What starts one is no change and stops none.
    function testExitSignaturesAreForOneContractAndOneChain() public {
        uint256 until = block.timestamp + 6 days;
        bytes[2] memory start = exitSignatures(deposit, "proposeEmergencyExit", SIGNER1_KEY, SIGNER2_KEY, until);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        twinDeposit.proposeEmergencyExit(until, start);
        toWon();
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        pool.proposeEmergencyExit(until, start);
        vm.chainId(SOURCE_CHAIN + 1);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.proposeEmergencyExit(until, start);
        toSource();
        // Nor are they the two signatures of a change.
        bytes32 kind = keccak256("setKey");
        bytes memory arguments = abi.encode(ChargeGovernance.Key.Relay, stranger);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.propose(kind, arguments, 0, until, start);
        deposit.proposeEmergencyExit(until, start);

        twinDeposit.proposeEmergencyExit(
            until, exitSignatures(twinDeposit, "proposeEmergencyExit", SIGNER1_KEY, SIGNER2_KEY, until)
        );
        bytes[2] memory stop = exitSignatures(deposit, "cancelEmergencyExit", SIGNER1_KEY, SIGNER2_KEY, until);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        twinDeposit.cancelEmergencyExit(until, stop);
        vm.chainId(SOURCE_CHAIN + 1);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.cancelEmergencyExit(until, stop);
        toSource();
        deposit.cancelEmergencyExit(until, stop);
        assertEq(uint8(deposit.emergencyExitStatus()), uint8(ChargeGovernance.Status.None));
        assertEq(uint8(twinDeposit.emergencyExitStatus()), uint8(ChargeGovernance.Status.Waiting));
        assertEq(twinDeposit.exitRound(), 1);
        assertEq(pool.exitRound(), 0);
    }

    function testARefundSignedForOneDeploymentDoesNotRefundOnTheTwin() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        // The same wallet tops up through the twin with a quote of the same content.
        bytes32 twinId = twinDeposit.quoteDigest(quote);
        bytes memory twinQuoteSig = sign(QUOTER_KEY, twinId);
        bytes memory twinAuth = authorizationFor(quote, PAYER_KEY, address(twinDeposit), twinId);
        twinDeposit.depositWithAuthorization(quote, twinQuoteSig, twinAuth);

        vm.warp(quote.payBy + 2 hours);
        uint256 until = block.timestamp + 1 days;
        bytes[2] memory signatures = two(deposit, "refund", refundSubject(id), 0, until);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        twinDeposit.refundBySigners(id, WON_BLOCK, until, signatures); // unknown number there
        bytes[2] memory wrongSubject = two(deposit, "refund", refundSubject(twinId), 0, until);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        twinDeposit.refundBySigners(twinId, WON_BLOCK, until, wrongSubject); // signed in the first domain
        deposit.refundBySigners(id, WON_BLOCK, until, signatures);
        assertEq(usdc.balanceOf(address(twinDeposit)), 100e6);
    }

    // ---- one state, one signature ----

    function testARefundSignatureDoesNotOutliveItsRound() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        uint256 until = block.timestamp + 6 days;
        bytes[2] memory refund = two(deposit, "refund", refundSubject(id), 0, until);
        payIt(quote, false);
        completeIt(id); // the payout was recorded, so the refund signatures are now stale
        rejectIt(id); // even if the completion is later taken back
        vm.warp(quote.payBy + 2 hours);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.refundBySigners(id, WON_BLOCK, until, refund);
        assertEq(usdc.balanceOf(address(deposit)), 100e6);
    }

    function testAContestSignatureWorksForOneCompletionOnly() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        completeIt(id);
        uint256 until = block.timestamp + 6 days;
        bytes memory contest = sign(PAYER_KEY, deposit.contestDigest(id, until));
        deposit.contest(id, until, contest);
        rejectIt(id);
        payIt(quote, false);
        toSource();
        uint256 round = deposit.depositOf(id).round;
        deposit.completeBySigners(id, until, two(deposit, "complete", id, round, until));
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.contest(id, until, contest);
    }

    /// What is signed for a deposit that is open does not work once it is completed, and the other way round.
    function testASignatureMadeForOneStateDoesNotWorkInTheNext() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        uint256 until = block.timestamp + 6 days;
        bytes memory earlyContest = sign(PAYER_KEY, deposit.contestDigest(id, until));
        bytes[2] memory earlyReject = two(deposit, "reject", id, 0, until);
        completeIt(id);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.contest(id, until, earlyContest);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.reject(id, until, earlyReject);

        bytes[2] memory earlyRefund = two(deposit, "refund", refundSubject(id), 1, until);
        rejectIt(id);
        vm.warp(quote.payBy + 2 hours);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.refundBySigners(id, WON_BLOCK, until, earlyRefund);
        refundBySigners(id);
    }

    function testAPaidRecordWorksForOneRoundOnly() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        bytes32 other = depositIt(quoteFor(100e6, board));
        uint256 until = block.timestamp + 6 days;
        bytes memory paid = paidSignature(id, until);
        vm.startPrank(vm.addr(RELAY_KEY));
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.complete(other, WON_BLOCK, until, paid);
        deposit.complete(id, WON_BLOCK, until, paid);
        vm.stopPrank();
        rejectIt(id);
        vm.prank(vm.addr(RELAY_KEY));
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.complete(id, WON_BLOCK, until, paid);
    }

    function split(bytes memory signature) internal pure returns (bytes32 r, bytes32 s, uint8 v) {
        assembly ("memory-safe") {
            r := mload(add(signature, 32))
            s := mload(add(signature, 64))
            v := byte(0, mload(add(signature, 96)))
        }
    }
}
