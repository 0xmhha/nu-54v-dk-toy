// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {ChargeBase} from "./ChargeBase.t.sol";
import {ChargeTypes} from "../src/charge/ChargeTypes.sol";
import {ChargeGovernance} from "../src/charge/ChargeGovernance.sol";
import {WonPool} from "../src/charge/WonPool.sol";
import {CoinView} from "./ChargeMocks.sol";
import {NativeCoinStandIn} from "./NativeCoinStandIn.sol";

/// @notice The StableNet side of a top-up with a 2-decimal won token: who can make the pool pay, how often,
/// and how much in a day. The pool cannot see the source chain, so most quotes here have no deposit behind
/// them: that is exactly what the three keys together could do, and what the limits must bound.
contract WonPoolTest is ChargeBase {
    event Paid(
        bytes32 indexed id,
        address indexed board,
        address indexed payer,
        uint256 wonAmount,
        uint256 assetAmount,
        uint256 rate,
        bool fast,
        uint256 sourceChainId,
        address sourceDeposit,
        bytes32 sourceBlockHash
    );

    function boardNo(uint256 i) internal pure returns (address) {
        return address(uint160(0xB0A2D000 + i));
    }

    function payerNo(uint256 i) internal pure returns (address) {
        return address(uint160(0xFA1E2000 + i));
    }

    function quoteOf(uint256 wonAmount, uint256 boardIndex, uint256 payerIndex)
        internal
        returns (ChargeTypes.Quote memory quote)
    {
        quote = quoteForWon(wonAmount, boardNo(boardIndex));
        quote.payer = payerNo(payerIndex);
    }

    function expectPayRevert(PayArgs memory args, bytes4 reason) internal {
        toWon();
        vm.prank(vm.addr(RELAY_KEY));
        vm.expectRevert(reason);
        pool.pay(args.origin, args.quote, args.quoteSignature, args.confirmation, args.witnessSignature);
    }

    function expectQuoteRevert(ChargeTypes.Quote memory quote, bytes4 reason) internal {
        expectPayRevert(payArgs(quote, false), reason);
    }

    /// The witness signs again, for a changed confirmation.
    function reconfirm(PayArgs memory args) internal {
        toWon();
        args.witnessSignature = sign(
            WITNESS_KEY,
            pool.confirmationDigest(
                pool.payoutId(args.origin, args.quote),
                args.confirmation.sourceBlockHash,
                args.confirmation.fast,
                args.confirmation.until
            )
        );
    }

    // ---- one payout ----

    function testPayoutReachesTheBoardAndIsRecorded() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        PayArgs memory args = payArgs(quote, false);
        toWon();
        vm.expectEmit(address(pool));
        emit Paid(
            id,
            board,
            payer,
            134_500 * WON,
            134_500 * WON * scale(),
            RATE,
            false,
            SOURCE_CHAIN,
            address(deposit),
            SOURCE_BLOCK
        );
        payWith(args);
        assertEq(assetBalance(board), 134_500 * WON * scale());
        assertEq(assetBalance(address(pool)), (POOL_SIZE - 134_500 * WON) * scale());
        WonPool.Payout memory payout = pool.payoutOf(id);
        assertEq(payout.board, board);
        assertEq(payout.payer, payer);
        assertEq(payout.wonAmount, 134_500 * WON);
        assertEq(payout.paidAt, block.timestamp);
        assertFalse(payout.fast);
        assertEq(pool.usedByBoard(board), 134_500 * WON);
        assertEq(pool.usedByPayer(payer), 134_500 * WON);
        assertEq(pool.usedTotal(), 134_500 * WON);
        assertEq(pool.usedFast(), 0);
    }

    function testOneDepositIsPaidOnce() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        PayArgs memory args = payArgs(quote, false);
        payWith(args);
        expectPayRevert(args, WonPool.AlreadyPaid.selector);
        // A new confirmation, another block, the other path: still the same deposit number.
        args.confirmation =
            WonPool.Confirmation({fast: true, sourceBlockHash: bytes32("later"), until: block.timestamp + 2 hours});
        reconfirm(args);
        expectPayRevert(args, WonPool.AlreadyPaid.selector);
        assertEq(assetBalance(board), 134_500 * WON * scale());
    }

    function testTheRelayKeyAloneCannotPay() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        PayArgs memory args = payArgs(quote, false);
        bytes32 confirmed = pool.confirmationDigest(idOf(quote), SOURCE_BLOCK, false, args.confirmation.until);
        args.witnessSignature = sign(RELAY_KEY, confirmed);
        expectPayRevert(args, ChargeGovernance.Unauthorized.selector);
        args.witnessSignature = sign(QUOTER_KEY, confirmed);
        expectPayRevert(args, ChargeGovernance.Unauthorized.selector);
        args.witnessSignature = "";
        expectPayRevert(args, ChargeTypes.InvalidSignature.selector);
        assertEq(assetBalance(board), 0);
    }

    function testTheConfirmationKeyAloneCannotPay() public {
        PayArgs memory args = payArgs(quoteFor(100e6, board), false);
        address[3] memory callers = [vm.addr(WITNESS_KEY), vm.addr(QUOTER_KEY), stranger];
        toWon();
        for (uint256 i; i < callers.length; ++i) {
            vm.prank(callers[i]);
            vm.expectRevert(ChargeGovernance.Unauthorized.selector);
            pool.pay(args.origin, args.quote, args.quoteSignature, args.confirmation, args.witnessSignature);
        }
        assertEq(assetBalance(board), 0);
    }

    function testRelayAndConfirmationKeysCannotPayWithoutAQuote() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        PayArgs memory args = payArgs(quote, false);
        args.quoteSignature = sign(RELAY_KEY, idOf(quote));
        expectPayRevert(args, ChargeGovernance.Unauthorized.selector);
        args.quoteSignature = sign(WITNESS_KEY, idOf(quote));
        expectPayRevert(args, ChargeGovernance.Unauthorized.selector);
    }

    /// The confirmation names the deposit, the source block and the path; none can be changed after signing.
    function testTheConfirmationCannotBeChanged() public {
        ChargeTypes.Quote memory quote = quoteOf(40_000 * WON, 1, 1);
        PayArgs memory args = payArgs(quote, false);
        args.confirmation.fast = true;
        expectPayRevert(args, ChargeGovernance.Unauthorized.selector);
        args.confirmation.fast = false;
        args.confirmation.sourceBlockHash = bytes32("another block");
        expectPayRevert(args, ChargeGovernance.Unauthorized.selector);
        args.confirmation.sourceBlockHash = SOURCE_BLOCK;
        args.confirmation.until += 1;
        expectPayRevert(args, ChargeGovernance.Unauthorized.selector);

        PayArgs memory other = payArgs(quoteOf(40_000 * WON, 2, 2), false);
        other.witnessSignature = args.witnessSignature; // a confirmation of another deposit
        expectPayRevert(other, ChargeGovernance.Unauthorized.selector);
    }

    function testAConfirmationEndsAndCannotBeLong() public {
        PayArgs memory args = payArgs(quoteFor(100e6, board), false);
        args.confirmation.until = block.timestamp + 7 days + 1;
        reconfirm(args);
        expectPayRevert(args, ChargeGovernance.InvalidTime.selector);
        args.confirmation.until = block.timestamp + 10 minutes;
        reconfirm(args);
        vm.warp(block.timestamp + 10 minutes);
        expectPayRevert(args, ChargeGovernance.InvalidTime.selector);
        vm.warp(block.timestamp - 1);
        payWith(args);
    }

    function testKeysThatAreOffPayNothing() public {
        PayArgs memory args = payArgs(quoteFor(100e6, board), false);
        uint256 snapshot = vm.snapshotState();
        ChargeGovernance.Key[3] memory hot =
            [ChargeGovernance.Key.Relay, ChargeGovernance.Key.Quoter, ChargeGovernance.Key.Witness];
        for (uint256 i; i < hot.length; ++i) {
            toWon();
            vm.prank(vm.addr(PAUSER_KEY));
            pool.disableKey(hot[i]);
            expectPayRevert(args, ChargeGovernance.Unauthorized.selector);
            vm.revertToState(snapshot);
        }
        // With the relay off, the zero address is not a relay either.
        toWon();
        vm.prank(vm.addr(PAUSER_KEY));
        pool.disableKey(ChargeGovernance.Key.Relay);
        vm.prank(address(0));
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        pool.pay(args.origin, args.quote, args.quoteSignature, args.confirmation, args.witnessSignature);
    }

    // ---- what the quote must say ----

    function testAQuoteForAnotherPlaceIsRefused() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        quote.wonChainId = SOURCE_CHAIN;
        expectQuoteRevert(quote, WonPool.InvalidQuote.selector);

        quote = quoteFor(100e6, board);
        quote.pool = address(0xDEAD);
        expectQuoteRevert(quote, WonPool.InvalidQuote.selector);

        quote = quoteFor(100e6, board);
        quote.won = address(usdc);
        expectQuoteRevert(quote, WonPool.InvalidQuote.selector);

        expectQuoteRevert(quoteFor(100e6, address(0)), WonPool.InvalidQuote.selector);
        expectQuoteRevert(quoteFor(100e6, address(pool)), WonPool.InvalidQuote.selector);
        expectQuoteRevert(quoteFor(100e6, asset), WonPool.InvalidQuote.selector);

        quote = quoteFor(100e6, board);
        quote.feeWon = 1;
        expectQuoteRevert(quote, WonPool.InvalidQuote.selector);
    }

    function testAmountAndRateAreChecked() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        quote.wonAmount += 1; // more won than the USDC buys
        expectQuoteRevert(quote, WonPool.InvalidQuote.selector);

        quote = quoteFor(100e6, board);
        quote.rate = RATE + 1;
        quote.wonAmount = quote.usdcAmount * quote.rate / 1e6;
        expectQuoteRevert(quote, WonPool.InvalidRate.selector);
        quote.rate = RATE * 10; // a leaked quote key cannot sell won cheaply
        quote.wonAmount = quote.usdcAmount * quote.rate / 1e6;
        expectQuoteRevert(quote, WonPool.InvalidRate.selector);

        expectQuoteRevert(quoteFor(usdcFor(MIN_WON) - 1, board), WonPool.InvalidAmount.selector);
        expectQuoteRevert(quoteFor(usdcFor(MAX_WON + 1), board), WonPool.InvalidAmount.selector);
        payIt(quoteOf(MIN_WON, 1, 1), false);
        payIt(quoteOf(MAX_WON, 2, 2), false);
        assertEq(assetBalance(boardNo(1)), MIN_WON * scale());
        assertEq(assetBalance(boardNo(2)), MAX_WON * scale());
    }

    /// What is below 0.01 won never leaves the pool, whatever the decimals of the asset.
    function testWonBelowOneHundredthIsNeverPaid() public {
        ChargeTypes.Quote memory quote = quoteFor(1_000_007, board); // 1,345.0094 won
        assertEq(quote.wonAmount, 1_345 * WON);
        payIt(quote, false);
        assertEq(assetBalance(board), 1_345 * WON * scale());
        assertEq(assetBalance(board) % scale(), 0);
    }

    function testPayoutDeadline() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        quote.payBy = quote.expiresAt + 30 days; // the deadline is not the quote server's choice
        expectQuoteRevert(quote, ChargeGovernance.InvalidTime.selector);

        quote = quoteFor(100e6, board);
        PayArgs memory args = payArgs(quote, false);
        args.confirmation.until = quote.payBy + 1 hours;
        reconfirm(args);
        vm.warp(quote.payBy);
        expectPayRevert(args, ChargeGovernance.InvalidTime.selector);
        vm.warp(quote.payBy - 1);
        payWith(args);
        assertEq(assetBalance(board), 134_500 * WON * scale());
    }

    // ---- where deposits may come from ----

    function testOnlyKnownDepositContractsArePaid() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        PayArgs memory args = payArgs(quote, false);
        args.origin.deposit = address(0xDEAD);
        expectPayRevert(args, WonPool.UnknownOrigin.selector);
        args.origin = WonPool.Origin({chainId: SOURCE_CHAIN + 1, deposit: address(deposit)});
        expectPayRevert(args, WonPool.UnknownOrigin.selector);
        assertTrue(pool.isOrigin(SOURCE_CHAIN, address(deposit)));
    }

    function testASecondOriginNeedsTwoSignersAndADayAndHasItsOwnNumbers() public {
        address second = address(0x5EC0);
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        PayArgs memory args = payArgs(quote, false);
        toWon();
        bytes32 action = keccak256(abi.encode(keccak256("addOrigin"), uint256(10), second, bytes32("salt")));
        assertEq(proposeOn(pool, "addOrigin", abi.encode(uint256(10), second), "salt"), action);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        pool.addOrigin(10, second, "salt");
        vm.warp(block.timestamp + 24 hours);
        pool.addOrigin(10, second, "salt");
        assertTrue(pool.isOrigin(10, second));

        // The same quote under the other origin is another number, which the quote key never signed.
        quote = quoteFor(100e6, board);
        args = payArgs(quote, false);
        args.origin = WonPool.Origin({chainId: 10, deposit: second});
        assertTrue(pool.payoutId(args.origin, quote) != idOf(quote));
        reconfirm(args);
        expectPayRevert(args, ChargeGovernance.Unauthorized.selector);

        vm.prank(stranger);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        pool.removeOrigin(10, second);
        vm.prank(vm.addr(PAUSER_KEY));
        pool.removeOrigin(SOURCE_CHAIN, address(deposit));
        expectPayRevert(payArgs(quote, false), WonPool.UnknownOrigin.selector);
    }

    function testDeploymentNamesItsChainAndAsset() public {
        toWon();
        WonPool.Config memory config = poolConfig(address(deposit));
        config.chainId = SOURCE_CHAIN;
        vm.expectRevert(WonPool.WrongChain.selector);
        new WonPool(keys(), config);

        config = poolConfig(address(deposit));
        config.assetDecimals = assetDecimals() + 1; // not what the asset answers
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        new WonPool(keys(), config);

        config = poolConfig(address(deposit));
        config.asset = address(0xDEAD);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        new WonPool(keys(), config);

        config = poolConfig(address(deposit));
        config.treasury = address(0);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        new WonPool(keys(), config);

        config = poolConfig(address(deposit));
        config.minRate = RATE + 1;
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        new WonPool(keys(), config);

        config = poolConfig(address(deposit));
        config.limits.perPayout = MIN_WON - 1; // nothing could ever be paid
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        new WonPool(keys(), config);

        config = poolConfig(address(deposit));
        config.limits.totalDaily = uint256(type(uint128).max) + 1; // more than the hourly sums can hold
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        new WonPool(keys(), config);
    }

    /// Whether the asset is the chain's coin is read from the asset: the token view of the coin answers the
    /// deployer's coin balance, a token of its own does not. A pool that says otherwise is not deployed, so
    /// no token pool takes plain coins that could never leave.
    function testDeploymentChecksWhetherTheAssetIsTheCoin() public {
        toWon();
        vm.deal(address(this), 1 ether);
        WonPool.Config memory config = poolConfig(address(deposit));
        config.assetIsCoin = !assetIsCoin();
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        new WonPool(keys(), config);

        // A deployer without coins cannot show that the asset is the coin.
        config = poolConfig(address(deposit));
        vm.deal(address(this), 0);
        if (assetIsCoin()) {
            vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
            new WonPool(keys(), config);
        } else {
            assertFalse(new WonPool(keys(), config).assetIsCoin());
        }
    }

    function testAForkWithAnotherChainIdPaysNothing() public {
        PayArgs memory args = payArgs(quoteFor(100e6, board), false);
        vm.chainId(WON_CHAIN + 1);
        vm.prank(vm.addr(RELAY_KEY));
        vm.expectRevert(WonPool.WrongChain.selector);
        pool.pay(args.origin, args.quote, args.quoteSignature, args.confirmation, args.witnessSignature);
    }

    // ---- limits ----

    function testABoardReceivesAtMostTheDailyLimit() public {
        for (uint256 i = 1; i <= 3; ++i) {
            payIt(quoteOf(300_000 * WON, 1, i), false);
        }
        payIt(quoteOf(100_000 * WON, 1, 4), false);
        assertEq(pool.usedByBoard(boardNo(1)), ACCOUNT_DAILY);
        expectQuoteRevert(quoteOf(MIN_WON, 1, 5), WonPool.LimitExceeded.selector);
        payIt(quoteOf(MIN_WON, 2, 5), false); // another board is not affected
        assertEq(assetBalance(boardNo(1)), ACCOUNT_DAILY * scale());
    }

    function testAWalletBuysAtMostTheDailyLimit() public {
        for (uint256 i = 1; i <= 3; ++i) {
            payIt(quoteOf(300_000 * WON, i, 1), false);
        }
        payIt(quoteOf(100_000 * WON, 4, 1), false);
        assertEq(pool.usedByPayer(payerNo(1)), ACCOUNT_DAILY);
        expectQuoteRevert(quoteOf(MIN_WON, 5, 1), WonPool.LimitExceeded.selector);
        payIt(quoteOf(MIN_WON, 5, 2), false);
    }

    function testThePoolPaysAtMostItsDailyLimit() public {
        for (uint256 i = 1; i <= 10; ++i) {
            payIt(quoteOf(300_000 * WON, i, i), false);
        }
        assertEq(pool.usedTotal(), TOTAL_DAILY);
        expectQuoteRevert(quoteOf(MIN_WON, 11, 11), WonPool.LimitExceeded.selector);
        assertEq(assetBalance(address(pool)), (POOL_SIZE - TOTAL_DAILY) * scale());
    }

    function testTheFastPathHasItsOwnSmallerLimits() public {
        expectPayRevert(payArgs(quoteOf(FAST_EACH + 1, 1, 1), true), WonPool.LimitExceeded.selector);
        for (uint256 i = 1; i <= 10; ++i) {
            payIt(quoteOf(FAST_EACH, i, i), true);
        }
        assertEq(pool.usedFast(), FAST_DAILY);
        expectPayRevert(payArgs(quoteOf(MIN_WON, 11, 11), true), WonPool.LimitExceeded.selector);
        payIt(quoteOf(MAX_WON, 11, 11), false); // a final deposit is still paid
        assertEq(pool.usedTotal(), FAST_DAILY + MAX_WON);
    }

    /// A day is counted in hours: the hour of a payout and the 24 after it. No 24 hours ever hold more than
    /// the limit, and a payout frees its room at most 25 hours later.
    function testTheDailyWindowEdges() public {
        vm.warp(block.timestamp + 30 minutes); // the middle of an hour
        uint256 start = block.timestamp;
        for (uint256 i = 1; i <= 3; ++i) {
            payIt(quoteOf(300_000 * WON, 1, i), false);
        }
        payIt(quoteOf(100_000 * WON, 1, 4), false);

        vm.warp(start + 24 hours - 1); // still inside 24 hours
        expectQuoteRevert(quoteOf(MIN_WON, 1, 5), WonPool.LimitExceeded.selector);
        vm.warp(start + 24 hours + 29 minutes + 59 seconds); // the last second of the 25th hour
        expectQuoteRevert(quoteOf(MIN_WON, 1, 5), WonPool.LimitExceeded.selector);
        assertEq(pool.usedByBoard(boardNo(1)), ACCOUNT_DAILY);
        vm.warp(start + 24 hours + 30 minutes); // the next hour begins
        assertEq(pool.usedByBoard(boardNo(1)), 0);
        payIt(quoteOf(300_000 * WON, 1, 5), false);
        assertEq(pool.usedByBoard(boardNo(1)), 300_000 * WON);
    }

    function testPayoutsSpreadOverHoursAreAddedUp() public {
        for (uint256 i = 1; i <= 9; ++i) {
            payIt(quoteOf(100_000 * WON, 1, i), false);
            vm.warp(block.timestamp + 2 hours);
        }
        // 18 hours later, 900,000 won are counted.
        assertEq(pool.usedByBoard(boardNo(1)), 900_000 * WON);
        expectQuoteRevert(quoteOf(100_000 * WON + 1, 1, 10), WonPool.LimitExceeded.selector);
        payIt(quoteOf(100_000 * WON, 1, 10), false);
        vm.warp(block.timestamp + 7 hours); // 25 hours after the first payout: only that one is free
        assertEq(pool.usedByBoard(boardNo(1)), 900_000 * WON);
        expectQuoteRevert(quoteOf(100_000 * WON + 1, 1, 11), WonPool.LimitExceeded.selector);
        payIt(quoteOf(100_000 * WON, 1, 11), false);
    }

    /// Relay, quote and confirmation keys together, with no deposit at all: a day still costs the pool no
    /// more than its daily limit, and the pool never pays what it does not hold.
    function testAllThreeKeysTogetherStayInsideTheLimitsAndThePool() public {
        uint256 paid;
        for (uint256 i = 1; i <= 20; ++i) {
            PayArgs memory args = payArgs(quoteOf(MAX_WON, i, i), false);
            toWon();
            vm.prank(vm.addr(RELAY_KEY));
            try pool.pay(args.origin, args.quote, args.quoteSignature, args.confirmation, args.witnessSignature) {
                paid += MAX_WON;
            } catch {}
        }
        assertEq(paid, TOTAL_DAILY);
        assertEq(assetBalance(address(pool)), (POOL_SIZE - TOTAL_DAILY) * scale());

        vm.warp(block.timestamp + 25 hours);
        for (uint256 i = 21; i <= 26; ++i) {
            payIt(quoteOf(MAX_WON, i, i), false);
        }
        // 200,000 won are left. A payout of 300,000 fails as a whole: no record and no limit used.
        ChargeTypes.Quote memory short = quoteOf(MAX_WON, 27, 27);
        PayArgs memory shortArgs = payArgs(short, false);
        toWon();
        vm.prank(vm.addr(RELAY_KEY));
        vm.expectRevert();
        pool.pay(
            shortArgs.origin,
            shortArgs.quote,
            shortArgs.quoteSignature,
            shortArgs.confirmation,
            shortArgs.witnessSignature
        );
        assertEq(pool.payoutOf(idOf(short)).paidAt, 0);
        assertEq(pool.usedTotal(), 6 * MAX_WON);
        assertEq(pool.usedByBoard(boardNo(27)), 0);
        assertEq(assetBalance(boardNo(27)), 0);
        payIt(quoteOf(200_000 * WON, 27, 27), false);
        assertEq(assetBalance(address(pool)), 0);
        fundPool(MAX_WON);
        payWith(shortArgs); // the same payout works once the pool is filled again
        assertEq(assetBalance(boardNo(27)), MAX_WON * scale() + 200_000 * WON * scale());
    }

    function testOneKeyLowersLimitsAndTwoWithADayRaiseThem() public {
        toWon();
        WonPool.Limits memory lower = WonPool.Limits({
            perPayout: 100_000 * WON,
            accountDaily: 100_000 * WON,
            totalDaily: 500_000 * WON,
            fastPerPayout: 10_000 * WON,
            fastDaily: 50_000 * WON
        });
        vm.prank(stranger);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        pool.lowerLimits(lower);
        vm.startPrank(vm.addr(PAUSER_KEY));
        pool.lowerLimits(lower);
        // One value up is a raise, whichever value it is.
        uint256[5] memory values =
            [lower.perPayout, lower.accountDaily, lower.totalDaily, lower.fastPerPayout, lower.fastDaily];
        for (uint256 i; i < 5; ++i) {
            values[i] += 1;
            vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
            pool.lowerLimits(WonPool.Limits(values[0], values[1], values[2], values[3], values[4]));
            values[i] -= 1;
        }
        vm.stopPrank();
        expectQuoteRevert(quoteOf(100_000 * WON + 1, 1, 1), WonPool.InvalidAmount.selector);

        toWon();
        bytes32 action = keccak256(abi.encode(keccak256("raiseLimits"), limits(), bytes32("salt")));
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        pool.raiseLimits(limits(), "salt");
        assertEq(proposeOn(pool, "raiseLimits", abi.encode(limits()), "salt"), action);
        vm.warp(block.timestamp + 24 hours - 1);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        pool.raiseLimits(limits(), "salt");
        vm.warp(block.timestamp + 1);
        vm.prank(stranger); // the two signatures count, not the sender
        pool.raiseLimits(limits(), "salt");
        (uint256 perPayout,, uint256 totalDaily,,) = pool.limits();
        assertEq(perPayout, MAX_WON);
        assertEq(totalDaily, TOTAL_DAILY);
        payIt(quoteOf(MAX_WON, 1, 1), false);
    }

    // ---- the rate ----

    /// The rate is one fixed value: both ends of the range are equal. Another value needs two signers and a day.
    function testTheFixedRateChangesOnlyByTwoSignersAndADay() public {
        toWon();
        assertEq(pool.minRate(), RATE);
        assertEq(pool.maxRate(), RATE);
        vm.prank(vm.addr(PAUSER_KEY));
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        pool.narrowRates(RATE, RATE + 1);
        vm.prank(vm.addr(RELAY_KEY));
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        pool.narrowRates(RATE, RATE);

        uint256 next = 140_000;
        bytes32 action = keccak256(abi.encode(keccak256("setRates"), next, next, bytes32("salt")));
        assertEq(proposeOn(pool, "setRates", abi.encode(next, next), "salt"), action);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        pool.setRates(next, next, "salt");
        vm.warp(block.timestamp + 24 hours);
        pool.setRates(next, next, "salt");
        expectQuoteRevert(quoteOf(MIN_WON, 1, 1), WonPool.InvalidRate.selector); // quoted at the old rate
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        quote.rate = next;
        quote.wonAmount = 140_000 * WON;
        payIt(quote, false);
        assertEq(assetBalance(board), 140_000 * WON * scale());
    }

    function testAWiderRangeCanBeNarrowedByOneKey() public {
        toWon();
        proposeOn(pool, "setRates", abi.encode(uint256(130_000), uint256(140_000)), 0);
        vm.warp(block.timestamp + 24 hours);
        pool.setRates(130_000, 140_000, 0);
        vm.startPrank(vm.addr(PAUSER_KEY));
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        pool.narrowRates(129_999, 140_000);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        pool.narrowRates(136_000, 135_000);
        pool.narrowRates(134_000, 135_000);
        vm.stopPrank();
        assertEq(pool.minRate(), 134_000);
        assertEq(pool.maxRate(), 135_000);
    }

    // ---- stopping, and the pool's own money ----

    function testPauseStopsPayoutsUntilTwoSignersUndoIt() public {
        PayArgs memory args = payArgs(quoteFor(100e6, board), false);
        toWon();
        vm.prank(vm.addr(RELAY_KEY));
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        pool.pausePays();
        vm.prank(vm.addr(PAUSER_KEY));
        pool.pausePays();
        expectPayRevert(args, WonPool.PaysPaused.selector);

        uint256 until = block.timestamp + 1 days;
        bytes memory one = sign(SIGNER1_KEY, pool.actionDigest(keccak256("unpausePays"), 0, pool.pauseRound(), until));
        vm.expectRevert(ChargeGovernance.SignersNotDistinct.selector);
        pool.unpausePays(until, [one, one]);
        bytes[2] memory signatures = two(pool, "unpausePays", 0, pool.pauseRound(), until);
        pool.unpausePays(until, signatures);
        assertEq(pool.pauseRound(), 2); // every pause and every undo is a new round
        payWith(args);
        vm.prank(vm.addr(PAUSER_KEY));
        pool.pausePays();
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        pool.unpausePays(until, signatures); // works once
    }

    function testPoolMoneyLeavesOnlyToTheTreasuryAfterTwoSignersAndADay() public {
        toWon();
        uint256 amount = 1_000_000 * WON * scale();
        bytes32 action = keccak256(abi.encode(keccak256("withdraw"), amount, bytes32("salt")));
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        pool.withdraw(amount, "salt");
        assertEq(proposeOn(pool, "withdraw", abi.encode(amount), "salt"), action);
        vm.warp(block.timestamp + 24 hours - 1);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        pool.withdraw(amount, "salt");
        vm.warp(block.timestamp + 1);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        pool.withdraw(amount + 1, "salt"); // another amount was never proposed
        vm.prank(stranger);
        pool.withdraw(amount, "salt");
        assertEq(assetBalance(treasury), amount);
        assertEq(assetBalance(stranger), 0);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        pool.withdraw(amount, "salt");
    }

    /// A paused pool is not a locked pool: two signers undo the pause, and its money can be taken out.
    function testAPausedPoolCanStillBeEmptied() public {
        toWon();
        vm.prank(vm.addr(PAUSER_KEY));
        pool.pausePays();
        uint256 amount = POOL_SIZE * scale();
        proposeOn(pool, "withdraw", abi.encode(amount), "salt");
        vm.warp(block.timestamp + 24 hours);
        pool.withdraw(amount, "salt");
        assertEq(assetBalance(treasury), amount);
        assertEq(assetBalance(address(pool)), 0);
    }

    function testPlainCoinsAreOnlyTakenWhenTheAssetIsTheCoin() public virtual {
        toWon();
        vm.deal(stranger, 1 ether);
        vm.prank(stranger);
        (bool ok,) = address(pool).call{value: 1 ether}("");
        assertFalse(ok);
        assertEq(address(pool).balance, 0);
    }
}

/// @notice The same pool with StableNet's coin as the asset: 18 decimals behind the token view at 0x…1000.
/// Every test above runs again here; quotes and limits are the same numbers and only the amount sent differs.
contract WonPoolCoinTest is WonPoolTest {
    address internal constant COIN = address(0x1000);

    function deployAsset() internal override returns (address) {
        vm.etch(COIN, type(CoinView).runtimeCode);
        return COIN;
    }

    function assetDecimals() internal pure override returns (uint8) {
        return 18;
    }

    function assetIsCoin() internal pure override returns (bool) {
        return true;
    }

    /// The pool is filled by a plain coin transfer.
    function fundPool(uint256 wonAmount) internal override {
        vm.deal(address(this), wonAmount * 1e16);
        (bool ok,) = address(pool).call{value: wonAmount * 1e16}("");
        assertTrue(ok);
    }

    function testPlainCoinsAreOnlyTakenWhenTheAssetIsTheCoin() public override {
        toWon();
        assertEq(address(pool).balance, POOL_SIZE * 1e16);
        assertEq(pool.unitScale(), 1e16);
    }

    function testTheBoardReceivesCoins() public {
        payIt(quoteForWon(300_000 * WON, board), false);
        assertEq(board.balance, 300_000 ether);
        assertEq(pool.usedByBoard(board), 300_000 * WON);
    }

    /// StableNet can block an account of its coin. The payout then fails whole and the deposit is refunded
    /// on its own chain; once the account is free the same payout works, if its deadline has not passed.
    function testABlockedBoardOrPoolIsNotPaidAndNothingIsRecorded() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        PayArgs memory args = payArgs(quote, false);
        CoinView(COIN).setBlacklisted(board, true);
        toWon();
        vm.prank(vm.addr(RELAY_KEY));
        vm.expectRevert("NativeCoinAdapter: account is blacklisted");
        pool.pay(args.origin, args.quote, args.quoteSignature, args.confirmation, args.witnessSignature);
        assertEq(pool.payoutOf(id).paidAt, 0);
        assertEq(pool.usedTotal(), 0);

        CoinView(COIN).setBlacklisted(board, false);
        CoinView(COIN).setBlacklisted(address(pool), true);
        vm.prank(vm.addr(RELAY_KEY));
        vm.expectRevert("NativeCoinAdapter: account is blacklisted");
        pool.pay(args.origin, args.quote, args.quoteSignature, args.confirmation, args.witnessSignature);

        CoinView(COIN).setBlacklisted(address(pool), false);
        payWith(args);
        assertEq(board.balance, 134_500 ether);
    }
}

/// @notice The pool against the repository's stand-in for the same coin, which keeps its own balances.
contract WonPoolStandInTest is ChargeBase {
    NativeCoinStandIn internal constant COIN = NativeCoinStandIn(address(0x1000));

    /// The real coin answers an account's coin balance as its token balance, and the pool checks that for
    /// its deployer. The stand-in keeps its own balances, so the deployer's is set to match.
    function deployAsset() internal override returns (address) {
        vm.etch(address(COIN), type(NativeCoinStandIn).runtimeCode);
        COIN.mint(address(this), address(this).balance);
        return address(COIN);
    }

    function assetDecimals() internal pure override returns (uint8) {
        return 18;
    }

    function assetIsCoin() internal pure override returns (bool) {
        return true;
    }

    function fundPool(uint256 wonAmount) internal override {
        COIN.mint(address(pool), wonAmount * 1e16);
    }

    function testATopUpEndsInCoinsOnTheBoard() public {
        ChargeTypes.Quote memory quote = quoteFor(30e6, board);
        bytes32 id = depositIt(quote);
        payIt(quote, true);
        completeIt(id);
        assertEq(COIN.balanceOf(board), 40_350 ether);
        assertEq(pool.usedFast(), 40_350 * WON);
    }

    function testABlockedBoardLeavesTheDepositToBeRefunded() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        PayArgs memory args = payArgs(quote, false);
        COIN.setBlacklisted(board, true);
        toWon();
        vm.prank(vm.addr(RELAY_KEY));
        vm.expectRevert("NativeCoinAdapter: account is blacklisted");
        pool.pay(args.origin, args.quote, args.quoteSignature, args.confirmation, args.witnessSignature);
        assertEq(pool.payoutOf(id).paidAt, 0);

        vm.warp(quote.payBy + 2 hours);
        refundBySigners(id);
        assertEq(usdc.balanceOf(payer), 1_000_000e6);
        // After the deadline the pool no longer pays this deposit, blocked or not.
        COIN.setBlacklisted(board, false);
        toWon();
        vm.prank(vm.addr(RELAY_KEY));
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        pool.pay(args.origin, args.quote, args.quoteSignature, args.confirmation, args.witnessSignature);
        assertEq(COIN.balanceOf(board), 0);
    }
}
