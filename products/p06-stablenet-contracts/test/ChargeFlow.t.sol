// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {ChargeBase} from "./ChargeBase.t.sol";
import {ChargeTypes} from "../src/charge/ChargeTypes.sol";
import {ChargeGovernance} from "../src/charge/ChargeGovernance.sol";
import {ChargeDeposit} from "../src/charge/ChargeDeposit.sol";
import {WonPool} from "../src/charge/WonPool.sol";
import {FixedRateProvider, IRateProvider} from "../src/charge/FixedRateProvider.sol";

/// @notice Whole top-ups across the two chains, and the order of payout and refund around the deadlines.
contract ChargeFlowTest is ChargeBase {
    function testATopUpFromOneSignatureToWonOnTheBoard() public {
        ChargeTypes.Quote memory small = quoteFor(30e6, board); // 40,350 won: paid on a safe block
        ChargeTypes.Quote memory large = quoteFor(200e6, board); // 269,000 won: paid on a final block
        bytes32 smallId = depositIt(small);
        bytes32 largeId = depositIt(large);
        payIt(small, true);
        payIt(large, false);
        completeIt(smallId);
        completeIt(largeId);

        assertEq(assetBalance(board), 309_350 * WON);
        assertEq(usdc.balanceOf(payer), 1_000_000e6 - 230e6);
        assertEq(usdc.balanceOf(address(deposit)), 230e6);
        assertEq(deposit.totalHeld(), 230e6);
        assertEq(uint8(state(smallId)), uint8(ChargeDeposit.State.Completed));
        toWon();
        assertTrue(pool.payoutOf(smallId).fast);
        assertFalse(pool.payoutOf(largeId).fast);
        assertEq(pool.usedByBoard(board), 309_350 * WON);
        assertEq(pool.usedFast(), 40_350 * WON);
    }

    /// The payout window ends where the refund windows begin, with two hours between them.
    function testARefundedDepositIsNeverPaid() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        PayArgs memory args = payArgs(quote, false);
        args.confirmation.until = block.timestamp + 6 days;
        toWon();
        args.witnessSignature =
            sign(WITNESS_KEY, pool.confirmationDigest(id, SOURCE_BLOCK, false, args.confirmation.until));

        vm.warp(quote.payBy + 2 hours);
        refundBySigners(id);
        toWon();
        vm.prank(vm.addr(RELAY_KEY));
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        pool.pay(args.origin, args.quote, args.quoteSignature, args.confirmation, args.witnessSignature);
        assertEq(assetBalance(board), 0);
        assertEq(usdc.balanceOf(payer), 1_000_000e6);
    }

    function testPaidJustBeforeTheDeadlineThenRecordedIsNotRefunded() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        PayArgs memory args = payArgs(quote, false);
        args.confirmation.until = quote.payBy + 1 hours;
        toWon();
        args.witnessSignature =
            sign(WITNESS_KEY, pool.confirmationDigest(id, SOURCE_BLOCK, false, args.confirmation.until));
        toSource();
        uint256 until = block.timestamp + 1 days;
        bytes[2] memory refund = two(deposit, "refund", refundSubject(id), 0, until);

        vm.warp(quote.payBy - 1);
        payWith(args);
        completeIt(id);
        vm.warp(quote.payBy + 2 hours);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.refundBySigners(id, WON_BLOCK, until, refund);
        vm.warp(block.timestamp + 7 days);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.refundSelf(id);
        assertEq(assetBalance(board), 134_500 * WON);
        assertEq(usdc.balanceOf(address(deposit)), 100e6);
    }

    function testADepositThePoolCannotPayEndsAsARefund() public {
        toWon();
        vm.prank(vm.addr(PAUSER_KEY));
        pool.pausePays();
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote); // pausing payouts does not stop deposits; the server must stop quoting
        vm.expectRevert(WonPool.PaysPaused.selector);
        this.payItExternal(quote, false);
        vm.warp(quote.payBy + 2 hours);
        refundBySigners(id);
        assertEq(usdc.balanceOf(payer), 1_000_000e6);
        assertEq(assetBalance(board), 0);
    }

    function testADepositOverThePoolLimitEndsAsARefund() public {
        // The deposit contract knows no daily limit: three deposits to one board are taken, the pool pays
        // what fits its limit and the rest is refunded.
        bytes32[4] memory ids;
        ChargeTypes.Quote[4] memory quotes;
        for (uint256 i; i < 4; ++i) {
            quotes[i] = quoteForWon(MAX_WON, board);
            ids[i] = depositIt(quotes[i]);
        }
        for (uint256 i; i < 3; ++i) {
            payIt(quotes[i], false);
            completeIt(ids[i]);
        }
        vm.expectRevert(WonPool.LimitExceeded.selector);
        this.payItExternal(quotes[3], false);
        vm.warp(quotes[3].payBy + 2 hours);
        refundBySigners(ids[3]);
        assertEq(assetBalance(board), 900_000 * WON);
        assertEq(deposit.totalHeld(), 3 * quotes[0].usdcAmount);
    }

    /// Known loss, kept on purpose: a payout that nobody records for seven days is refunded as well, and the
    /// pool has paid for nothing. The operator must record payouts; the contracts cannot see each other.
    function testAPayoutNeverRecordedIsAlsoRefunded() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        payIt(quote, false);
        toSource();
        vm.warp(block.timestamp + 7 days);
        deposit.refundSelf(id);
        assertEq(assetBalance(board), 134_500 * WON);
        assertEq(usdc.balanceOf(payer), 1_000_000e6);
    }

    /// The other known loss: a false contest that two signers do not answer in fourteen days.
    function testAFalseContestNobodyAnswersIsAlsoRefunded() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        payIt(quote, false);
        completeIt(id);
        contestIt(id);
        vm.warp(block.timestamp + 14 days);
        deposit.refundSelf(id);
        assertEq(assetBalance(board), 134_500 * WON);
        assertEq(usdc.balanceOf(payer), 1_000_000e6);
    }

    // ---- the rate source ----

    function testTheFixedRateSourceAnswersValueTimeAndName() public {
        toSource();
        FixedRateProvider provider = new FixedRateProvider(RATE, "fixed 2026-11-27");
        (uint256 rate, uint64 updatedAt, string memory name) = IRateProvider(address(provider)).latestRate();
        assertEq(rate, RATE);
        assertEq(updatedAt, block.timestamp);
        assertEq(name, "fixed 2026-11-27");
        // The contracts accept exactly this rate, and count the same won for the same USDC.
        assertEq(deposit.minRate(), rate);
        assertEq(deposit.maxRate(), rate);
        assertEq(pool.minRate(), rate);
        assertEq(pool.maxRate(), rate);
        assertEq(provider.wonFor(100e6), 134_500 * WON);
        assertEq(provider.usdcFor(134_500 * WON), 100e6);
        assertEq(provider.usdcFor(MIN_WON), 743_495); // rounded up: 1,000 won cost 0.743495 USDC
        assertEq(provider.wonFor(743_495), MIN_WON);
        assertEq(provider.wonFor(743_494), MIN_WON - 1);
        depositIt(quoteFor(provider.usdcFor(MIN_WON), board));

        vm.expectRevert(FixedRateProvider.InvalidRate.selector);
        new FixedRateProvider(0, "fixed");
        vm.expectRevert(FixedRateProvider.InvalidRate.selector);
        new FixedRateProvider(RATE, "");
    }

    function testFuzz_wonIsDroppedNeverRoundedUp(uint256 usdcAmount, uint256 rate) public {
        usdcAmount = bound(usdcAmount, 1, 1e15);
        rate = bound(rate, 1, 1e9);
        FixedRateProvider provider = new FixedRateProvider(rate, "fuzz");
        uint256 wonAmount = provider.wonFor(usdcAmount);
        assertLe(wonAmount * 1e6, usdcAmount * rate, "never more won than was paid for");
        assertGt((wonAmount + 1) * 1e6, usdcAmount * rate, "never a whole unit short");
        if (wonAmount == 0) return;
        uint256 least = provider.usdcFor(wonAmount);
        assertLe(least, usdcAmount, "the least USDC for that won is not more than was paid");
        assertGe(provider.wonFor(least), wonAmount, "and it still buys that won");
        if (least > 1) assertLt(provider.wonFor(least - 1), wonAmount, "one unit less does not");
    }

    function testFuzz_onlyTheExactWonOfAQuoteIsAccepted(uint256 usdcAmount, uint256 wonAmount) public {
        usdcAmount = bound(usdcAmount, usdcFor(MIN_WON), usdcFor(MAX_WON));
        uint256 exact = usdcAmount * RATE / 1e6;
        wonAmount = bound(wonAmount, exact - 3, exact + 3);
        ChargeTypes.Quote memory quote = quoteFor(usdcAmount, board);
        quote.wonAmount = wonAmount;
        bytes memory quoteSig = quoteSignature(quote);
        bytes memory auth = authorization(quote, PAYER_KEY);
        PayArgs memory args = payArgs(quote, false);
        if (wonAmount == exact) {
            deposit.depositWithAuthorization(quote, quoteSig, auth);
            payWith(args);
            assertEq(assetBalance(board), exact);
        } else {
            vm.expectRevert(ChargeDeposit.InvalidQuote.selector);
            deposit.depositWithAuthorization(quote, quoteSig, auth);
            toWon();
            vm.prank(vm.addr(RELAY_KEY));
            vm.expectRevert(WonPool.InvalidQuote.selector);
            pool.pay(args.origin, args.quote, args.quoteSignature, args.confirmation, args.witnessSignature);
        }
    }

    /// Payouts to one board at random times: no 24 hours ever hold more than the limit.
    function testFuzz_noDayOfOneBoardExceedsTheLimit(uint256 seed) public {
        uint256[] memory times = new uint256[](40);
        uint256[] memory amounts = new uint256[](40);
        uint256 count;
        for (uint256 i; i < 40; ++i) {
            seed = uint256(keccak256(abi.encode(seed)));
            vm.warp(block.timestamp + seed % 3 hours);
            uint256 wonAmount = MIN_WON + (seed >> 64) % (MAX_WON - MIN_WON);
            ChargeTypes.Quote memory quote = quoteForWon(wonAmount, board);
            quote.payer = address(uint160(0xFA1E2000 + i)); // the wallet limit is not what is tested here
            PayArgs memory args = payArgs(quote, false);
            toWon();
            vm.prank(vm.addr(RELAY_KEY));
            try pool.pay(args.origin, args.quote, args.quoteSignature, args.confirmation, args.witnessSignature) {
                times[count] = block.timestamp;
                amounts[count] = wonAmount;
                count += 1;
            } catch {}
            uint256 inDay;
            for (uint256 j; j < count; ++j) {
                if (times[j] + 24 hours > block.timestamp) inDay += amounts[j];
            }
            assertLe(inDay, ACCOUNT_DAILY);
        }
        assertGt(count, 3);
    }
}
