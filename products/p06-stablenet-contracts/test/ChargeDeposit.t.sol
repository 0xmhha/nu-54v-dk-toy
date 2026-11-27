// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {ChargeBase} from "./ChargeBase.t.sol";
import {ChargeTypes} from "../src/charge/ChargeTypes.sol";
import {ChargeGovernance} from "../src/charge/ChargeGovernance.sol";
import {ChargeDeposit} from "../src/charge/ChargeDeposit.sol";
import {TestChargeUSDC} from "../src/test-token/TestChargeUSDC.sol";

/// @notice The source-chain side of a top-up: what a deposit needs, and every way the USDC can leave again.
contract ChargeDepositTest is ChargeBase {
    event SpenderChanged(address indexed spender);
    event Deposited(
        bytes32 indexed id,
        address indexed payer,
        address indexed board,
        uint256 usdcAmount,
        uint256 wonAmount,
        uint256 rate,
        uint8 method,
        uint64 expiresAt,
        uint64 payBy,
        bytes32 number
    );

    function expectDepositRevert(ChargeTypes.Quote memory quote, bytes4 reason) internal {
        bytes memory quoteSig = quoteSignature(quote);
        bytes memory auth = authorization(quote, PAYER_KEY);
        toSource();
        vm.expectRevert(reason);
        deposit.depositWithAuthorization(quote, quoteSig, auth);
    }

    // ---- deposit ----

    function testDepositRecordsWhoHowMuchAtWhatRateForWhichBoard() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = idOf(quote);
        bytes memory quoteSig = quoteSignature(quote);
        bytes memory auth = authorization(quote, PAYER_KEY);
        vm.expectEmit(address(deposit));
        emit Deposited(id, payer, board, 100e6, 134_500 * WON, RATE, 1, quote.expiresAt, quote.payBy, quote.number);
        vm.prank(stranger); // anyone may send it and pay the gas
        assertEq(deposit.depositWithAuthorization(quote, quoteSig, auth), id);

        ChargeDeposit.Deposit memory record = deposit.depositOf(id);
        assertEq(record.payer, payer);
        assertEq(record.board, board);
        assertEq(record.usdcAmount, 100e6);
        assertEq(record.left, 100e6);
        assertEq(record.wonAmount, 134_500 * WON);
        assertEq(record.rate, RATE);
        assertEq(record.depositedAt, block.timestamp);
        assertEq(record.payBy, quote.payBy);
        assertEq(uint8(record.state), uint8(ChargeDeposit.State.Deposited));
        assertEq(usdc.balanceOf(address(deposit)), 100e6);
        assertEq(deposit.totalHeld(), 100e6);
        assertTrue(usdc.authorizationState(payer, id), "the quote digest is the authorization nonce");
        assertEq(usdc.allowance(payer, address(deposit)), 0, "no standing allowance");
    }

    function testSameQuoteDepositsOnce() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        depositIt(quote);
        expectDepositRevert(quote, ChargeDeposit.AlreadyDeposited.selector);
        assertEq(usdc.balanceOf(address(deposit)), 100e6);
    }

    /// Whoever sends the transaction first, the same deposit for the same payer and board is recorded.
    function testSendingItAheadOfTheRelayChangesNothing() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes memory quoteSig = quoteSignature(quote);
        bytes memory auth = authorization(quote, PAYER_KEY);
        vm.prank(stranger);
        bytes32 id = deposit.depositWithAuthorization(quote, quoteSig, auth);
        assertEq(deposit.depositOf(id).payer, payer);
        assertEq(deposit.depositOf(id).board, board);
        assertEq(usdc.balanceOf(stranger), 0);
        vm.prank(vm.addr(RELAY_KEY));
        vm.expectRevert(ChargeDeposit.AlreadyDeposited.selector);
        deposit.depositWithAuthorization(quote, quoteSig, auth);
    }

    /// The authorization only works when the deposit contract itself collects it.
    function testNobodyElseCanUseTheAuthorization() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = idOf(quote);
        bytes memory auth = authorization(quote, PAYER_KEY);
        (bytes32 r, bytes32 s, uint8 v) = split(auth);
        vm.startPrank(stranger);
        vm.expectRevert(TestChargeUSDC.CallerNotRecipient.selector);
        usdc.receiveWithAuthorization(payer, address(deposit), 100e6, 0, quote.expiresAt, id, v, r, s);
        vm.expectRevert(TestChargeUSDC.InvalidAuthorization.selector);
        usdc.transferWithAuthorization(payer, address(deposit), 100e6, 0, quote.expiresAt, id, v, r, s);
        vm.expectRevert(TestChargeUSDC.InvalidAuthorization.selector);
        usdc.transferWithAuthorization(payer, stranger, 100e6, 0, quote.expiresAt, id, v, r, s);
        vm.stopPrank();
        assertEq(usdc.balanceOf(payer), 1_000_000e6);
    }

    /// A relay holding the quote key still cannot move the money to another board, amount or rate: the wallet
    /// signed the digest of the one quote it saw.
    function testAuthorizationBindsBoardAmountRateAndQuote() public {
        ChargeTypes.Quote memory signed = quoteFor(100e6, board);
        bytes memory auth = authorization(signed, PAYER_KEY);

        ChargeTypes.Quote memory other = copy(signed);
        other.board = stranger;
        expectForged(other, auth);

        other = copy(signed);
        other.number = bytes32("another quote");
        expectForged(other, auth);

        other = copy(signed);
        other.usdcAmount = 200e6;
        other.wonAmount = 269_000 * WON;
        expectForged(other, auth);

        other = copy(signed);
        other.expiresAt += 1;
        other.payBy += 1;
        expectForged(other, auth);

        assertEq(usdc.balanceOf(payer), 1_000_000e6);
        assertEq(deposit.totalHeld(), 0);
    }

    function expectForged(ChargeTypes.Quote memory quote, bytes memory auth) internal {
        bytes memory quoteSig = quoteSignature(quote);
        vm.expectRevert(TestChargeUSDC.InvalidAuthorization.selector);
        deposit.depositWithAuthorization(quote, quoteSig, auth);
    }

    function testOnlyTheQuoteKeySignsQuotes() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes memory auth = authorization(quote, PAYER_KEY);
        bytes memory byRelay = sign(RELAY_KEY, idOf(quote));
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.depositWithAuthorization(quote, byRelay, auth);

        bytes memory byPayer = sign(PAYER_KEY, idOf(quote));
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.depositWithAuthorization(quote, byPayer, auth);

        vm.prank(vm.addr(PAUSER_KEY));
        deposit.disableKey(ChargeGovernance.Key.Quoter);
        expectDepositRevert(quote, ChargeGovernance.Unauthorized.selector);
    }

    function testHighSAndMalformedSignaturesAreRefused() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes memory auth = authorization(quote, PAYER_KEY);
        (bytes32 r, bytes32 s, uint8 v) = split(quoteSignature(quote));
        uint256 order = 0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141;
        bytes memory flipped = abi.encodePacked(r, bytes32(order - uint256(s)), v == 27 ? uint8(28) : uint8(27));
        vm.expectRevert(ChargeTypes.InvalidSignature.selector);
        deposit.depositWithAuthorization(quote, flipped, auth);
        vm.expectRevert(ChargeTypes.InvalidSignature.selector);
        deposit.depositWithAuthorization(quote, abi.encodePacked(r, s), auth);
        bytes memory quoteSig = quoteSignature(quote);
        vm.expectRevert(ChargeTypes.InvalidSignature.selector);
        deposit.depositWithAuthorization(quote, quoteSig, abi.encodePacked(r, s));
    }

    function testAQuoteForAnotherPlaceIsRefused() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        quote.usdc = address(0xDEAD);
        expectDepositRevert(quote, ChargeDeposit.InvalidQuote.selector);

        quote = quoteFor(100e6, board);
        quote.pool = address(0xDEAD);
        expectDepositRevert(quote, ChargeDeposit.InvalidQuote.selector);

        quote = quoteFor(100e6, board);
        quote.won = address(0xDEAD);
        expectDepositRevert(quote, ChargeDeposit.InvalidQuote.selector);

        quote = quoteFor(100e6, board);
        quote.wonChainId = SOURCE_CHAIN;
        expectDepositRevert(quote, ChargeDeposit.InvalidQuote.selector);

        expectDepositRevert(quoteFor(100e6, address(0)), ChargeDeposit.InvalidQuote.selector);
        expectDepositRevert(quoteFor(100e6, address(pool)), ChargeDeposit.InvalidQuote.selector);
    }

    /// Only the built method is taken; the number is in the quote so a second method can be told apart later.
    /// The pool pays no board that is the pool itself or its asset, so no deposit is taken for one.
    function testABoardThePoolRefusesIsRefusedAtTheDeposit() public {
        expectDepositRevert(quoteFor(100e6, address(pool)), ChargeDeposit.InvalidQuote.selector);
        expectDepositRevert(quoteFor(100e6, asset), ChargeDeposit.InvalidQuote.selector);
        expectDepositRevert(quoteFor(100e6, address(0)), ChargeDeposit.InvalidQuote.selector);
        assertEq(usdc.balanceOf(address(deposit)), 0);
    }

    function testOnlyTheAuthorizationMethodIsTaken() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        quote.method = 2;
        expectDepositRevert(quote, ChargeDeposit.InvalidQuote.selector);
        quote.method = 0;
        expectDepositRevert(quote, ChargeDeposit.InvalidQuote.selector);
    }

    function testFeeMustBeZero() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        quote.feeWon = 1;
        expectDepositRevert(quote, ChargeDeposit.InvalidQuote.selector);
        quote.wonAmount -= 1;
        expectDepositRevert(quote, ChargeDeposit.InvalidQuote.selector);
    }

    function testAmountRange() public {
        // 1,000 won is the least and 300,000 won the most, counted in 0.01 won after dropping the rest.
        uint256 least = usdcFor(MIN_WON);
        expectDepositRevert(quoteFor(least - 1, board), ChargeDeposit.InvalidAmount.selector); // 999.99 won
        depositIt(quoteForWon(MIN_WON, board));
        depositIt(quoteForWon(MAX_WON, board));
        expectDepositRevert(quoteFor(usdcFor(MAX_WON + 1), board), ChargeDeposit.InvalidAmount.selector);
        // The quote server cannot name more won than the USDC buys to reach the range.
        ChargeTypes.Quote memory padded = quoteFor(least - 1, board);
        padded.wonAmount = MIN_WON;
        expectDepositRevert(padded, ChargeDeposit.InvalidQuote.selector);
    }

    function testRateOutsideTheRangeIsRefused() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        quote.rate = RATE + 1;
        quote.wonAmount = quote.usdcAmount * quote.rate / 1e6;
        expectDepositRevert(quote, ChargeDeposit.InvalidRate.selector);
        quote.rate = RATE - 1;
        quote.wonAmount = quote.usdcAmount * quote.rate / 1e6;
        expectDepositRevert(quote, ChargeDeposit.InvalidRate.selector);
    }

    /// Anything below 0.01 won is dropped, never rounded up.
    function testWonBelowOneHundredthIsDropped() public {
        ChargeTypes.Quote memory quote = quoteFor(1_000_001, board); // 1,345.001345 won
        assertEq(quote.wonAmount, 1_345 * WON);
        quote.wonAmount += 1;
        expectDepositRevert(quote, ChargeDeposit.InvalidQuote.selector);
        quote.wonAmount -= 2;
        expectDepositRevert(quote, ChargeDeposit.InvalidQuote.selector);
        quote.wonAmount += 1;
        depositIt(quote);

        ChargeTypes.Quote memory almost = quoteFor(1_000_007, board); // 1,345.0094 won
        assertEq(almost.wonAmount, 1_345 * WON);
        depositIt(almost);
    }

    function testQuoteTimes() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        quote.payBy += 1; // the payout deadline is not the quote server's choice
        expectDepositRevert(quote, ChargeGovernance.InvalidTime.selector);

        quote = quoteFor(100e6, board);
        quote.expiresAt = uint64(block.timestamp + 10 minutes + 1); // a far end would outlive a refund
        quote.payBy = quote.expiresAt + 2 hours;
        expectDepositRevert(quote, ChargeGovernance.InvalidTime.selector);

        quote = quoteFor(100e6, board);
        bytes memory quoteSig = quoteSignature(quote);
        bytes memory auth = authorization(quote, PAYER_KEY);
        vm.warp(quote.expiresAt); // two minutes after it was issued
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.depositWithAuthorization(quote, quoteSig, auth);
        vm.warp(quote.expiresAt - 1);
        deposit.depositWithAuthorization(quote, quoteSig, auth);
        assertEq(usdc.balanceOf(payer), 1_000_000e6 - 100e6);
    }

    function testAWalletWithoutTheMoneyDepositsNothing() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        vm.prank(payer);
        usdc.transfer(stranger, 1_000_000e6);
        expectDepositRevert(quote, TestChargeUSDC.InsufficientBalance.selector);
        assertEq(uint8(state(idOf(quote))), uint8(ChargeDeposit.State.None));
        assertEq(deposit.totalHeld(), 0);
    }

    function testDeploymentNamesItsChainAndCounterparts() public {
        ChargeDeposit.Config memory config = depositConfig(address(pool));
        config.chainId = WON_CHAIN;
        vm.expectRevert(ChargeDeposit.WrongChain.selector);
        new ChargeDeposit(keys(), config);

        config = depositConfig(address(0));
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        new ChargeDeposit(keys(), config);

        config = depositConfig(address(pool));
        config.usdc = address(0xDEAD); // not a contract
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        new ChargeDeposit(keys(), config);

        config = depositConfig(address(pool));
        config.minWon = config.maxWon + 1;
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        new ChargeDeposit(keys(), config);
    }

    function testAForkWithAnotherChainIdTakesNoDeposits() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes memory quoteSig = quoteSignature(quote);
        bytes memory auth = authorization(quote, PAYER_KEY);
        vm.chainId(SOURCE_CHAIN + 1);
        vm.expectRevert(ChargeDeposit.WrongChain.selector);
        deposit.depositWithAuthorization(quote, quoteSig, auth);
    }

    // ---- pause and limits ----

    function testPauseStopsNewDepositsAndNothingElse() public {
        bytes32 id = depositIt(quoteFor(100e6, board));
        vm.prank(stranger);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.pauseDeposits();
        vm.prank(vm.addr(PAUSER_KEY));
        deposit.pauseDeposits();
        expectDepositRevert(quoteFor(50e6, board), ChargeDeposit.DepositsPaused.selector);
        completeIt(id); // a deposit that is in can still be finished
        bytes32 second = unpauseDepositAndPauseAgain();
        vm.warp(block.timestamp + 7 days);
        deposit.refundSelf(second);
    }

    function unpauseDepositAndPauseAgain() internal returns (bytes32) {
        uint256 until = block.timestamp + 1 days;
        bytes[2] memory signatures = two(deposit, "unpauseDeposits", 0, deposit.pauseRound(), until);
        deposit.unpauseDeposits(until, signatures);
        assertEq(deposit.pauseRound(), 2); // every pause and every undo is a new round
        bytes32 id = depositIt(quoteFor(50e6, board));
        // The same two signatures do not undo the next pause.
        vm.prank(vm.addr(PAUSER_KEY));
        deposit.pauseDeposits();
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.unpauseDeposits(until, signatures);
        return id;
    }

    function testOneKeyLowersTheLimitAndTwoWithADayRaiseIt() public {
        vm.prank(stranger);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.lowerLimit(100_000 * WON);
        vm.startPrank(vm.addr(PAUSER_KEY));
        deposit.lowerLimit(100_000 * WON);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.lowerLimit(200_000 * WON); // the pauser cannot raise
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.lowerLimit(MIN_WON - 1);
        vm.stopPrank();
        expectDepositRevert(quoteFor(100e6, board), ChargeDeposit.InvalidAmount.selector); // 134,500 won

        bytes32 action = keccak256(abi.encode(keccak256("raiseLimit"), MAX_WON, bytes32("salt")));
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.raiseLimit(MAX_WON, "salt");
        assertEq(proposeOn(deposit, "raiseLimit", abi.encode(MAX_WON), "salt"), action);
        vm.warp(block.timestamp + 24 hours - 1);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.raiseLimit(MAX_WON, "salt");
        vm.warp(block.timestamp + 1);
        deposit.raiseLimit(MAX_WON, "salt");
        assertEq(deposit.maxWon(), MAX_WON);
        depositIt(quoteFor(100e6, board));
    }

    // ---- completion ----

    function testCompletionNeedsTheRelayAndTheWitness() public {
        bytes32 id = depositIt(quoteFor(100e6, board));
        uint256 until = block.timestamp + 1 hours;
        bytes memory byWitness = paidSignature(id, until);
        bytes memory byRelay = sign(RELAY_KEY, deposit.paidDigest(id, WON_BLOCK, until));

        vm.prank(vm.addr(WITNESS_KEY)); // the witness key alone
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.complete(id, WON_BLOCK, until, byWitness);
        vm.prank(stranger);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.complete(id, WON_BLOCK, until, byWitness);

        vm.startPrank(vm.addr(RELAY_KEY)); // the relay key alone
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.complete(id, WON_BLOCK, until, byRelay);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.complete(id, bytes32("another block"), until, byWitness);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.complete(id, WON_BLOCK, until + 1, byWitness);
        deposit.complete(id, WON_BLOCK, until, byWitness);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.complete(id, WON_BLOCK, until, byWitness);
        vm.stopPrank();

        ChargeDeposit.Deposit memory record = deposit.depositOf(id);
        assertEq(uint8(record.state), uint8(ChargeDeposit.State.Completed));
        assertEq(record.completedAt, block.timestamp);
        assertEq(usdc.balanceOf(address(deposit)), 100e6, "completed money stays");
    }

    function testAPaidRecordOfOneDepositDoesNotCompleteAnother() public {
        bytes32 first = depositIt(quoteFor(100e6, board));
        bytes32 second = depositIt(quoteFor(100e6, board));
        uint256 until = block.timestamp + 1 hours;
        bytes memory signature = paidSignature(first, until);
        vm.prank(vm.addr(RELAY_KEY));
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.complete(second, WON_BLOCK, until, signature);
    }

    function testAPaidRecordEndsAndCannotBeKeptForLong() public {
        bytes32 id = depositIt(quoteFor(100e6, board));
        uint256 tooLong = block.timestamp + 7 days + 1;
        bytes memory longSignature = paidSignature(id, tooLong);
        uint256 until = block.timestamp + 1 hours;
        bytes memory signature = paidSignature(id, until);
        vm.startPrank(vm.addr(RELAY_KEY));
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.complete(id, WON_BLOCK, tooLong, longSignature);
        vm.warp(until);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.complete(id, WON_BLOCK, until, signature);
        vm.stopPrank();
    }

    function testTwoSignersCompleteWhenTheWitnessKeyIsOff() public {
        bytes32 id = depositIt(quoteFor(100e6, board));
        vm.prank(vm.addr(PAUSER_KEY));
        deposit.disableKey(ChargeGovernance.Key.Witness);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        this.completeItExternal(id);
        uint256 until = block.timestamp + 1 days;
        bytes[2] memory signatures = two(deposit, "complete", id, 0, until);
        deposit.completeBySigners(id, until, signatures);
        assertEq(uint8(state(id)), uint8(ChargeDeposit.State.Completed));
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.completeBySigners(id, until, signatures);
    }

    // ---- refunds ----

    function testSignedRefundOpensTwoHoursAfterThePayoutDeadline() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        uint256 until = block.timestamp + 1 days;
        bytes[2] memory signatures = two(deposit, "refund", refundSubject(id), 0, until);
        vm.warp(quote.payBy + 2 hours - 1);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.refundBySigners(id, WON_BLOCK, until, signatures);
        vm.warp(quote.payBy + 2 hours);
        vm.prank(stranger); // whoever sends it, the money goes to the paying wallet
        deposit.refundBySigners(id, WON_BLOCK, until, signatures);
        assertEq(usdc.balanceOf(payer), 1_000_000e6);
        assertEq(usdc.balanceOf(stranger), 0);
        assertEq(deposit.totalHeld(), 0);
        assertEq(uint8(state(id)), uint8(ChargeDeposit.State.Refunded));
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.refundBySigners(id, WON_BLOCK, until, signatures);
        vm.warp(block.timestamp + 30 days);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.refundSelf(id);
    }

    function testSignedRefundNeedsTwoDifferentSigners() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        bytes32 other = depositIt(quoteFor(50e6, board));
        vm.warp(quote.payBy + 2 hours);
        uint256 until = block.timestamp + 1 days;
        bytes32 digest = deposit.actionDigest(keccak256("refund"), refundSubject(id), 0, until);
        bytes memory one = sign(SIGNER1_KEY, digest);

        vm.expectRevert(ChargeGovernance.SignersNotDistinct.selector);
        deposit.refundBySigners(id, WON_BLOCK, until, [one, one]);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.refundBySigners(id, WON_BLOCK, until, [one, sign(RELAY_KEY, digest)]);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.refundBySigners(id, WON_BLOCK, until, [one, sign(PAUSER_KEY, digest)]);
        // Signatures for one deposit, or for another kind of action, do nothing for this one.
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.refundBySigners(other, WON_BLOCK, until, [one, sign(SIGNER3_KEY, digest)]);
        bytes[2] memory complete = two(deposit, "complete", id, 0, until);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.refundBySigners(id, WON_BLOCK, until, complete);
        deposit.refundBySigners(id, WON_BLOCK, until, [sign(SIGNER3_KEY, digest), one]);
        assertEq(usdc.balanceOf(payer), 1_000_000e6 - 50e6);
    }

    function testSelfRefundOpensAfterSevenDays() public {
        bytes32 id = depositIt(quoteFor(100e6, board));
        assertEq(deposit.selfRefundAt(id), block.timestamp + 7 days);
        vm.warp(block.timestamp + 7 days - 1);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.refundSelf(id);
        vm.warp(block.timestamp + 1);
        vm.prank(stranger);
        deposit.refundSelf(id);
        assertEq(usdc.balanceOf(payer), 1_000_000e6);
        assertEq(usdc.balanceOf(stranger), 0);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.refundSelf(id);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.refundSelf(bytes32("unknown"));
    }

    function testACompletedDepositIsNotRefunded() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        payIt(quote, false);
        completeIt(id);
        vm.warp(block.timestamp + 30 days);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.refundSelf(id);
        uint256 until = block.timestamp + 1 days;
        bytes[2] memory signatures = two(deposit, "refund", refundSubject(id), deposit.depositOf(id).round, until);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.refundBySigners(id, WON_BLOCK, until, signatures);
        assertEq(usdc.balanceOf(address(deposit)), 100e6);
    }

    // ---- contest, answer, reject ----

    function testThePayerContestsDirectlyOrBySignature() public {
        bytes32 first = depositIt(quoteFor(100e6, board));
        bytes32 second = depositIt(quoteFor(50e6, board));
        completeIt(first);
        completeIt(second);
        uint256 until = block.timestamp + 1 days;
        bytes memory bySigner = sign(SIGNER1_KEY, deposit.contestDigest(second, until));
        bytes memory byPayer = sign(PAYER_KEY, deposit.contestDigest(second, until));

        vm.prank(stranger);
        vm.expectRevert(ChargeTypes.InvalidSignature.selector);
        deposit.contest(first, until, "");
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.contest(second, until, bySigner);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.contest(first, until, byPayer); // signed for the other deposit

        vm.prank(payer);
        deposit.contest(first, 0, "");
        assertEq(uint8(state(first)), uint8(ChargeDeposit.State.Contested));
        vm.prank(stranger);
        deposit.contest(second, until, byPayer);
        assertEq(uint8(state(second)), uint8(ChargeDeposit.State.Contested));
        assertEq(deposit.depositOf(second).contestedAt, block.timestamp);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.contest(second, until, byPayer);
    }

    function testAContestSignatureEndsAndCannotBeLong() public {
        bytes32 id = depositIt(quoteFor(100e6, board));
        completeIt(id);
        uint256 tooLong = block.timestamp + 7 days + 1;
        bytes memory long = sign(PAYER_KEY, deposit.contestDigest(id, tooLong));
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.contest(id, tooLong, long);
        uint256 until = block.timestamp + 1 days;
        bytes memory signature = sign(PAYER_KEY, deposit.contestDigest(id, until));
        vm.warp(until); // one day after the completion: the contest itself is still open
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.contest(id, until, signature);
        assertEq(uint8(state(id)), uint8(ChargeDeposit.State.Completed));
        // The payer's own transaction needs no signature and no end.
        vm.prank(payer);
        deposit.contest(id, 0, "");
        assertEq(uint8(state(id)), uint8(ChargeDeposit.State.Contested));
    }

    function testContestClosesSevenDaysAfterCompletion() public {
        bytes32 id = depositIt(quoteFor(100e6, board));
        vm.prank(payer);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.contest(id, 0, ""); // nothing to contest before a completion
        completeIt(id);
        vm.warp(block.timestamp + 7 days);
        vm.prank(payer);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.contest(id, 0, "");
        vm.warp(block.timestamp - 1);
        vm.prank(payer);
        deposit.contest(id, 0, "");
    }

    /// A completion written without a payout does not hold the money forever.
    function testAnUnansweredContestRefundsAfterFourteenDays() public {
        bytes32 id = depositIt(quoteFor(100e6, board));
        completeIt(id); // relay and witness both wrong: nothing was paid
        contestIt(id);
        assertEq(deposit.selfRefundAt(id), block.timestamp + 14 days);
        vm.warp(block.timestamp + 14 days - 1);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.refundSelf(id);
        vm.warp(block.timestamp + 1);
        deposit.refundSelf(id);
        assertEq(usdc.balanceOf(payer), 1_000_000e6);
        assertEq(uint8(state(id)), uint8(ChargeDeposit.State.Refunded));
    }

    function testAnAnsweredContestIsFinal() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        payIt(quote, false);
        completeIt(id);
        contestIt(id);
        uint256 until = block.timestamp + 1 days;
        uint256 round = deposit.depositOf(id).round;
        bytes[2] memory confirm = two(deposit, "confirmPaid", id, round, until);
        deposit.confirmPaid(id, until, confirm);
        assertTrue(deposit.depositOf(id).settled);
        assertEq(uint8(state(id)), uint8(ChargeDeposit.State.Completed));

        vm.prank(payer);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.contest(id, 0, "");
        bytes[2] memory reject = two(deposit, "reject", id, round + 1, until);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.reject(id, until, reject);
        vm.warp(block.timestamp + 60 days);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.refundSelf(id);
    }

    function testAnswersAfterFourteenDaysAreTooLate() public {
        bytes32 id = depositIt(quoteFor(100e6, board));
        completeIt(id);
        contestIt(id);
        vm.warp(block.timestamp + 14 days);
        uint256 until = block.timestamp + 1 days;
        uint256 round = deposit.depositOf(id).round;
        bytes[2] memory confirm = two(deposit, "confirmPaid", id, round, until);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.confirmPaid(id, until, confirm);
        bytes[2] memory reject = two(deposit, "reject", id, round, until);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.reject(id, until, reject);
        deposit.refundSelf(id);
    }

    function testARejectedCompletionNeedsTwoSignersToCompleteAgain() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        completeIt(id);
        uint256 until = block.timestamp + 1 days;
        bytes[2] memory reject = two(deposit, "reject", id, deposit.depositOf(id).round, until);
        deposit.reject(id, until, reject);
        assertEq(uint8(state(id)), uint8(ChargeDeposit.State.Deposited));
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        this.completeItExternal(id); // relay and witness are no longer believed for this deposit

        payIt(quote, false);
        toSource();
        bytes[2] memory complete = two(deposit, "complete", id, deposit.depositOf(id).round, until);
        deposit.completeBySigners(id, until, complete);
        // The earlier reject signatures belong to an earlier round.
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.reject(id, until, reject);
    }

    function testALateRejectWaitsThreeDaysBeforeTheSelfRefund() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        payIt(quote, false);
        completeIt(id);
        vm.warp(block.timestamp + 6 days);
        rejectIt(id);
        assertEq(deposit.selfRefundAt(id), block.timestamp + 3 days);
        vm.warp(block.timestamp + 3 days - 1); // nine days after the deposit
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.refundSelf(id);
        vm.warp(block.timestamp + 1);
        deposit.refundSelf(id);
    }

    function testSignaturesOfSignersEndAndCannotBeLong() public {
        bytes32 id = depositIt(quoteFor(100e6, board));
        uint256 tooLong = block.timestamp + 7 days + 1;
        bytes[2] memory long = two(deposit, "complete", id, 0, tooLong);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.completeBySigners(id, tooLong, long);
        uint256 until = block.timestamp + 1 hours;
        bytes[2] memory short = two(deposit, "complete", id, 0, until);
        vm.warp(until);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.completeBySigners(id, until, short);
    }

    // ---- the way out for completed money: one empty place for a later contract ----

    function testCompletedMoneyLeavesOnlyThroughTheSpender() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        payIt(quote, false);
        completeIt(id);
        vm.warp(block.timestamp + 8 days);
        assertEq(deposit.spender(), address(0));
        vm.prank(vm.addr(RELAY_KEY));
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.spend(id, 1, stranger);
        vm.prank(address(0));
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.spend(id, 1, stranger);

        Spender later = new Spender(deposit);
        proposeOn(deposit, "setSpender", abi.encode(vm.addr(0xE0A)), "salt");
        bytes32 action = keccak256(abi.encode(keccak256("setSpender"), address(later), bytes32("salt")));
        assertEq(proposeOn(deposit, "setSpender", abi.encode(address(later)), "salt"), action);
        vm.warp(block.timestamp + 24 hours);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.setSpender(vm.addr(0xE0A), "salt"); // a plain key cannot be the spender
        deposit.setSpender(address(later), "salt");

        later.spend(id, 60e6, stranger);
        assertEq(usdc.balanceOf(stranger), 60e6);
        assertEq(deposit.depositOf(id).left, 40e6);
        assertEq(deposit.totalHeld(), 40e6);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        later.spend(id, 41e6, stranger);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        later.spend(id, 0, stranger);
        // Money that left cannot be claimed back by a contest or a reject.
        uint256 until = block.timestamp + 1 days;
        bytes[2] memory reject = two(deposit, "reject", id, deposit.depositOf(id).round, until);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.reject(id, until, reject);
    }

    function testTheSpenderWaitsForTheContestWindowAndTakesNoOpenDeposit() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        bytes32 open = depositIt(quoteFor(50e6, board));
        Spender later = new Spender(deposit);
        proposeOn(deposit, "setSpender", abi.encode(address(later)), "salt");
        vm.warp(block.timestamp + 24 hours);
        deposit.setSpender(address(later), "salt");

        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        later.spend(open, 1, stranger);
        completeIt(id);
        vm.warp(block.timestamp + 7 days - 1);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        later.spend(id, 1, stranger);
        contestIt(id);
        vm.warp(block.timestamp + 1);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        later.spend(id, 1, stranger); // a contested deposit is not spendable
        vm.warp(block.timestamp + 14 days);
        deposit.refundSelf(id);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        later.spend(id, 1, stranger);
        assertEq(usdc.balanceOf(stranger), 0);
    }

    /// Stopping is a one-key action: the pauser empties the place of the spender at once, and only two
    /// signers and a day fill it again.
    function testThePauserStopsSpendingAtOnce() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        payIt(quote, false);
        completeIt(id);
        Spender later = new Spender(deposit);
        proposeOn(deposit, "setSpender", abi.encode(address(later)), "salt");
        vm.warp(block.timestamp + 8 days);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.setSpender(address(later), "salt"); // lapsed
        proposeOn(deposit, "setSpender", abi.encode(address(later)), "salt 2");
        vm.warp(block.timestamp + 24 hours);
        deposit.setSpender(address(later), "salt 2");
        later.spend(id, 10e6, stranger);

        vm.prank(stranger);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.clearSpender();
        vm.prank(address(later));
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.clearSpender();
        vm.expectEmit(address(deposit));
        emit SpenderChanged(address(0));
        vm.prank(vm.addr(PAUSER_KEY));
        deposit.clearSpender();
        assertEq(deposit.spender(), address(0));
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        later.spend(id, 10e6, stranger);
        assertEq(usdc.balanceOf(stranger), 10e6);
        assertEq(deposit.depositOf(id).left, 90e6);

        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.setSpender(address(later), "salt 2"); // the old change does not bring it back
        proposeOn(deposit, "setSpender", abi.encode(address(later)), "salt 3");
        vm.warp(block.timestamp + 24 hours);
        deposit.setSpender(address(later), "salt 3");
        later.spend(id, 90e6, stranger);
        assertEq(usdc.balanceOf(stranger), 100e6);
    }

    /// An account whose code is `0xef0100` and an address is a key that lent its account to that contract
    /// (EIP-7702). The key can lend it elsewhere at any time, so it is no more a spender than a plain key.
    function testAKeyThatLentItsAccountIsNotASpender() public {
        Spender later = new Spender(deposit);
        address lent = vm.addr(0xE0A);
        vm.etch(lent, abi.encodePacked(hex"ef0100", address(later)));
        assertEq(lent.code.length, 23);
        address shortCode = address(0xC0DE);
        vm.etch(shortCode, hex"00"); // a contract, however short: only the lent mark is refused
        proposeOn(deposit, "setSpender", abi.encode(lent), "salt");
        proposeOn(deposit, "setSpender", abi.encode(shortCode), "salt");
        proposeOn(deposit, "setSpender", abi.encode(address(deposit)), "salt");
        proposeOn(deposit, "setSpender", abi.encode(address(0)), "salt");
        vm.warp(block.timestamp + 24 hours);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.setSpender(lent, "salt");
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.setSpender(address(deposit), "salt");
        deposit.setSpender(shortCode, "salt");
        assertEq(deposit.spender(), shortCode);
        deposit.setSpender(address(0), "salt"); // two signers may also empty the place
        assertEq(deposit.spender(), address(0));
    }

    // ---- coins sent by mistake ----

    function testStrayCoinsGoBackOnceAndNeverTouchDeposits() public {
        depositIt(quoteFor(100e6, board));
        usdc.mint(address(deposit), 50e6); // somebody sent USDC straight to the contract
        bytes32 txHash = bytes32("transaction");
        uint256 until = block.timestamp + 1 days;
        bytes[2] memory signatures = straySignatures(address(usdc), stranger, 50e6, txHash, 3, until);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.returnStray(address(usdc), stranger, 51e6, txHash, 3, until, signatures);
        deposit.returnStray(address(usdc), stranger, 50e6, txHash, 3, until, signatures);
        assertEq(usdc.balanceOf(stranger), 50e6);
        vm.expectRevert(ChargeDeposit.AlreadyReturned.selector);
        deposit.returnStray(address(usdc), stranger, 50e6, txHash, 3, until, signatures);

        // Two signers together still cannot take a deposit this way.
        signatures = straySignatures(address(usdc), stranger, 1, txHash, 4, until);
        vm.expectRevert(ChargeDeposit.NothingStray.selector);
        deposit.returnStray(address(usdc), stranger, 1, txHash, 4, until, signatures);
        assertEq(usdc.balanceOf(address(deposit)), 100e6);
    }

    function testAnotherStrayTokenGoesBackTheSameWay() public {
        TestChargeUSDC otherToken = new TestChargeUSDC(address(this));
        otherToken.mint(address(deposit), 7e6);
        uint256 until = block.timestamp + 1 days;
        bytes[2] memory signatures = straySignatures(address(otherToken), stranger, 7e6, bytes32("tx"), 0, until);
        deposit.returnStray(address(otherToken), stranger, 7e6, bytes32("tx"), 0, until, signatures);
        assertEq(otherToken.balanceOf(stranger), 7e6);
    }

    function straySignatures(address token, address to, uint256 amount, bytes32 txHash, uint256 logIndex, uint256 until)
        internal
        view
        returns (bytes[2] memory)
    {
        return two(deposit, "returnStray", keccak256(abi.encode(token, to, amount, txHash, logIndex)), 0, until);
    }

    function split(bytes memory signature) internal pure returns (bytes32 r, bytes32 s, uint8 v) {
        assembly ("memory-safe") {
            r := mload(add(signature, 32))
            s := mload(add(signature, 64))
            v := byte(0, mload(add(signature, 96)))
        }
    }
}

/// Stands for the later contract that turns won back into USDC. It only forwards.
contract Spender {
    ChargeDeposit internal immutable deposit;

    constructor(ChargeDeposit deposit_) {
        deposit = deposit_;
    }

    function spend(bytes32 id, uint256 amount, address to) external {
        deposit.spend(id, amount, to);
    }
}
