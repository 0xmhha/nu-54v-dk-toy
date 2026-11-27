// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {ChargeBase} from "./ChargeBase.t.sol";
import {ChargeTypes} from "../src/charge/ChargeTypes.sol";
import {ChargeGovernance} from "../src/charge/ChargeGovernance.sol";
import {ChargeDeposit} from "../src/charge/ChargeDeposit.sol";
import {WonPool} from "../src/charge/WonPool.sol";
import {OddToken, TwoViewToken} from "./ChargeMocks.sol";

/// @notice Tokens that answer false, move nothing, keep a part, revert or call back. Whatever a token does,
/// a record never says more money moved than did, and nothing moves twice.
contract ChargeOddTokensTest is ChargeBase {
    OddToken internal oddUsdc;
    OddToken internal oddWon;
    ChargeDeposit internal oddDeposit;
    WonPool internal oddPool;
    bytes internal anySignature = new bytes(65);

    function setUp() public override {
        super.setUp();
        oddUsdc = new OddToken(6);
        oddWon = new OddToken(2);
        ChargeDeposit.Config memory depositValues = depositConfig(address(0));
        depositValues.usdc = address(oddUsdc);
        depositValues.won = address(oddWon);
        depositValues.pool = vm.computeCreateAddress(address(this), vm.getNonce(address(this)) + 1);
        oddDeposit = new ChargeDeposit(keys(), depositValues);
        toWon();
        WonPool.Config memory poolValues = poolConfig(address(oddDeposit));
        poolValues.asset = address(oddWon);
        poolValues.assetDecimals = 2;
        poolValues.assetIsCoin = false;
        oddPool = new WonPool(keys(), poolValues);
        assertEq(address(oddPool), depositValues.pool);
        oddWon.mint(address(oddPool), POOL_SIZE);
        oddUsdc.mint(payer, 1_000e6);
        toSource();
    }

    function oddQuote(uint256 usdcAmount) internal returns (ChargeTypes.Quote memory quote) {
        quote = quoteFor(usdcAmount, board);
        quote.usdc = address(oddUsdc);
        quote.won = address(oddWon);
        quote.pool = address(oddPool);
    }

    function oddDepositIt(ChargeTypes.Quote memory quote) internal returns (bytes32) {
        toSource();
        bytes memory quoteSig = sign(QUOTER_KEY, oddDeposit.quoteDigest(quote));
        return oddDeposit.depositWithAuthorization(quote, quoteSig, anySignature);
    }

    function oddPayArgs(ChargeTypes.Quote memory quote) internal returns (PayArgs memory args) {
        toSource();
        args.origin = WonPool.Origin({chainId: SOURCE_CHAIN, deposit: address(oddDeposit)});
        args.quote = quote;
        args.quoteSignature = sign(QUOTER_KEY, oddDeposit.quoteDigest(quote));
        args.confirmation =
            WonPool.Confirmation({fast: false, sourceBlockHash: SOURCE_BLOCK, until: block.timestamp + 1 hours});
        toWon();
        args.witnessSignature = sign(
            WITNESS_KEY,
            oddPool.confirmationDigest(
                oddPool.payoutId(args.origin, quote), SOURCE_BLOCK, false, args.confirmation.until
            )
        );
    }

    function oddPay(PayArgs memory args) external {
        toWon();
        vm.prank(vm.addr(RELAY_KEY));
        oddPool.pay(args.origin, args.quote, args.quoteSignature, args.confirmation, args.witnessSignature);
    }

    function tryDeposit(ChargeTypes.Quote memory quote) external {
        oddDepositIt(quote);
    }

    // ---- the deposit side ----

    function testADepositThatDoesNotArriveInFullIsNotRecorded() public {
        ChargeTypes.Quote memory quote = oddQuote(100e6);
        bytes32 id = oddDeposit.quoteDigest(quote);

        oddUsdc.setMode(OddToken.Mode.Fee); // keeps a part of every transfer
        vm.expectRevert(ChargeGovernance.TokenBalanceMismatch.selector);
        this.tryDeposit(quote);
        oddUsdc.setMode(OddToken.Mode.Silent); // says yes and moves nothing
        vm.expectRevert(ChargeGovernance.TokenBalanceMismatch.selector);
        this.tryDeposit(quote);
        oddUsdc.setMode(OddToken.Mode.Reverts);
        vm.expectRevert("odd token: refused");
        this.tryDeposit(quote);

        assertEq(uint8(oddDeposit.depositOf(id).state), uint8(ChargeDeposit.State.None));
        assertEq(oddDeposit.totalHeld(), 0);
        oddUsdc.setMode(OddToken.Mode.Normal);
        assertEq(oddDepositIt(quote), id);
        assertEq(oddDeposit.totalHeld(), 100e6);
    }

    /// The refund reverts and the deposit stays open, to be refunded once the token delivers.
    function testARefundThatDoesNotArriveLeavesTheDepositOpen() public {
        bytes32 id = oddDepositIt(oddQuote(100e6));
        vm.warp(block.timestamp + 7 days);
        uint256 before = oddUsdc.balanceOf(payer);

        oddUsdc.setMode(OddToken.Mode.ReturnsFalse);
        vm.expectRevert(ChargeGovernance.TokenTransferFailed.selector);
        oddDeposit.refundSelf(id);
        oddUsdc.setMode(OddToken.Mode.Silent);
        vm.expectRevert(ChargeGovernance.TokenBalanceMismatch.selector);
        oddDeposit.refundSelf(id);
        oddUsdc.setMode(OddToken.Mode.Fee);
        vm.expectRevert(ChargeGovernance.TokenBalanceMismatch.selector);
        oddDeposit.refundSelf(id);
        oddUsdc.setMode(OddToken.Mode.Reverts);
        vm.expectRevert("odd token: refused");
        oddDeposit.refundSelf(id);

        assertEq(uint8(oddDeposit.depositOf(id).state), uint8(ChargeDeposit.State.Deposited));
        assertEq(oddDeposit.totalHeld(), 100e6);
        assertEq(oddUsdc.balanceOf(payer), before);
        oddUsdc.setMode(OddToken.Mode.NoAnswer); // an old token that answers nothing still pays
        oddDeposit.refundSelf(id);
        assertEq(oddUsdc.balanceOf(payer), before + 100e6);
        assertEq(oddDeposit.totalHeld(), 0);
    }

    function testCallingBackDuringADepositChangesNothing() public {
        ChargeTypes.Quote memory quote = oddQuote(100e6);
        bytes memory quoteSig = sign(QUOTER_KEY, oddDeposit.quoteDigest(quote));
        oddUsdc.setMode(OddToken.Mode.Reenters);
        oddUsdc.setCallback(
            address(oddDeposit), abi.encodeCall(oddDeposit.depositWithAuthorization, (quote, quoteSig, anySignature))
        );
        oddDeposit.depositWithAuthorization(quote, quoteSig, anySignature);
        assertEq(oddUsdc.reentries(), 1);
        assertEq(bytes4(oddUsdc.reentryAnswer()), ChargeGovernance.Reentered.selector);
        assertEq(oddUsdc.balanceOf(address(oddDeposit)), 100e6);
        assertEq(oddDeposit.totalHeld(), 100e6);
    }

    function testCallingBackDuringARefundPaysOnce() public {
        bytes32 first = oddDepositIt(oddQuote(100e6));
        bytes32 second = oddDepositIt(oddQuote(50e6));
        vm.warp(block.timestamp + 7 days);
        uint256 before = oddUsdc.balanceOf(payer);
        oddUsdc.setMode(OddToken.Mode.Reenters);
        // While the first refund moves, the token asks for the second one and for the first again.
        oddUsdc.setCallback(address(oddDeposit), abi.encodeCall(oddDeposit.refundSelf, (second)));
        oddDeposit.refundSelf(first);
        assertEq(oddUsdc.reentries(), 1);
        assertEq(bytes4(oddUsdc.reentryAnswer()), ChargeGovernance.Reentered.selector);
        assertEq(oddUsdc.balanceOf(payer), before + 100e6);
        assertEq(oddDeposit.totalHeld(), 50e6);
        assertEq(uint8(oddDeposit.depositOf(second).state), uint8(ChargeDeposit.State.Deposited));
    }

    // ---- the pool side ----

    function testAPayoutThatDoesNotArriveInFullIsNotCounted() public {
        ChargeTypes.Quote memory quote = oddQuote(100e6);
        PayArgs memory args = oddPayArgs(quote);
        bytes32 id = oddPool.payoutId(args.origin, quote);

        oddWon.setMode(OddToken.Mode.ReturnsFalse);
        vm.expectRevert(ChargeGovernance.TokenTransferFailed.selector);
        this.oddPay(args);
        oddWon.setMode(OddToken.Mode.Silent);
        vm.expectRevert(ChargeGovernance.TokenBalanceMismatch.selector);
        this.oddPay(args);
        oddWon.setMode(OddToken.Mode.Fee);
        vm.expectRevert(ChargeGovernance.TokenBalanceMismatch.selector);
        this.oddPay(args);
        oddWon.setMode(OddToken.Mode.Reverts);
        vm.expectRevert("odd token: refused");
        this.oddPay(args);

        assertEq(oddPool.payoutOf(id).paidAt, 0);
        assertEq(oddPool.usedTotal(), 0);
        assertEq(oddPool.usedByBoard(board), 0);
        assertEq(oddWon.balanceOf(address(oddPool)), POOL_SIZE);
        oddWon.setMode(OddToken.Mode.NoAnswer);
        this.oddPay(args);
        assertEq(oddWon.balanceOf(board), 134_500 * WON);
        assertEq(oddPool.payoutOf(id).paidAt, block.timestamp);
    }

    /// The only call open to anyone on the pool is applying a change two signers proposed. Even that cannot
    /// run inside a payout.
    function testCallingBackDuringAPayoutMovesNothingElse() public {
        toWon();
        uint256 amount = 1_000_000 * WON;
        proposeOn(oddPool, "withdraw", abi.encode(amount), "salt");
        vm.warp(block.timestamp + 24 hours);
        ChargeTypes.Quote memory quote = oddQuote(100e6);
        PayArgs memory args = oddPayArgs(quote);
        oddWon.setMode(OddToken.Mode.Reenters);
        oddWon.setCallback(address(oddPool), abi.encodeCall(oddPool.withdraw, (amount, bytes32("salt"))));
        this.oddPay(args);
        assertEq(oddWon.reentries(), 1);
        assertEq(bytes4(oddWon.reentryAnswer()), ChargeGovernance.Reentered.selector);
        assertEq(oddWon.balanceOf(treasury), 0);
        assertEq(oddWon.balanceOf(board), 134_500 * WON);

        // Outside a payout the proposed withdrawal works, and a paid deposit is not paid again.
        ChargeTypes.Quote memory again = oddQuote(50e6);
        PayArgs memory againArgs = oddPayArgs(again);
        oddWon.setMode(OddToken.Mode.Normal);
        oddPool.withdraw(amount, "salt");
        assertEq(oddWon.balanceOf(treasury), amount);
        this.oddPay(againArgs);
        vm.expectRevert(WonPool.AlreadyPaid.selector);
        this.oddPay(againArgs);
    }

    /// The pool's own money has one way out besides a payout. Should the asset ever answer at a second
    /// address, returning that "other token" would move the pool's money, and the whole call is undone.
    function testASecondViewOfTheAssetIsNotAWayOut() public {
        toWon();
        TwoViewToken twoViews = new TwoViewToken();
        WonPool.Config memory config = poolConfig(address(deposit));
        config.asset = address(twoViews);
        WonPool twoViewPool = new WonPool(keys(), config);
        twoViews.mint(address(twoViewPool), POOL_SIZE);
        address second = twoViews.second();
        proposeOn(twoViewPool, "returnToken", abi.encode(second, POOL_SIZE), "salt");
        vm.warp(block.timestamp + 24 hours);
        vm.expectRevert(ChargeGovernance.TokenBalanceMismatch.selector);
        twoViewPool.returnToken(second, POOL_SIZE, "salt");
        assertEq(twoViews.balanceOf(address(twoViewPool)), POOL_SIZE);
        assertEq(twoViews.balanceOf(treasury), 0);
    }

    function testAnAssetWithFewerThanTwoDecimalsIsRefused() public {
        toWon();
        for (uint8 decimals; decimals < 2; ++decimals) {
            WonPool.Config memory config = poolConfig(address(deposit));
            config.asset = address(new OddToken(decimals));
            config.assetDecimals = decimals;
            vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
            new WonPool(keys(), config);
        }
    }

    // ---- the emergency exit ----

    /// A completed deposit past nothing but two signers' word: completed, contested and answered at once.
    function oddSettled(uint256 usdcAmount) internal returns (bytes32 id) {
        id = oddDepositIt(oddQuote(usdcAmount));
        uint256 until = block.timestamp + 1 days;
        oddDeposit.completeBySigners(id, until, two(oddDeposit, "complete", id, 0, until));
        vm.prank(payer);
        oddDeposit.contest(id, 0, "");
        oddDeposit.confirmPaid(id, until, two(oddDeposit, "confirmPaid", id, 2, until));
    }

    /// An exit whose money does not reach the treasury in full is undone, and the money waits for the next
    /// try. Nothing is recorded as taken.
    function testAnExitThatDoesNotArriveInFullIsUndone() public {
        bytes32 id = oddSettled(100e6);
        proposeExitOn(oddDeposit);
        toWon();
        proposeExitOn(oddPool);
        vm.warp(block.timestamp + 7 days);
        OddToken[2] memory tokens = [oddUsdc, oddWon];
        for (uint256 which; which < 2; ++which) {
            OddToken token = tokens[which];
            token.setMode(OddToken.Mode.ReturnsFalse);
            vm.expectRevert(ChargeGovernance.TokenTransferFailed.selector);
            this.oddExit(which, id);
            token.setMode(OddToken.Mode.Silent);
            vm.expectRevert(ChargeGovernance.TokenBalanceMismatch.selector);
            this.oddExit(which, id);
            token.setMode(OddToken.Mode.Fee);
            vm.expectRevert(ChargeGovernance.TokenBalanceMismatch.selector);
            this.oddExit(which, id);
            token.setMode(OddToken.Mode.Reverts);
            vm.expectRevert("odd token: refused");
            this.oddExit(which, id);
        }
        assertEq(oddDeposit.depositOf(id).left, 100e6);
        assertEq(oddDeposit.totalHeld(), 100e6);
        assertEq(oddUsdc.balanceOf(treasury), 0);
        assertEq(oddWon.balanceOf(address(oddPool)), POOL_SIZE);

        oddUsdc.setMode(OddToken.Mode.NoAnswer);
        oddWon.setMode(OddToken.Mode.NoAnswer);
        this.oddExit(0, id);
        this.oddExit(1, id);
        assertEq(oddUsdc.balanceOf(treasury), 100e6);
        assertEq(oddDeposit.totalHeld(), 0);
        assertEq(oddWon.balanceOf(treasury), POOL_SIZE);
    }

    function oddExit(uint256 which, bytes32 id) external {
        if (which == 0) oddDeposit.emergencyExit(listOf(id));
        else oddPool.emergencyExit();
    }

    /// A token that calls back while the exit moves money starts nothing else: not a refund, not a second
    /// exit of the same deposit, and on the pool not a second exit either.
    function testCallingBackDuringAnExitMovesNothingElse() public {
        bytes32 id = oddSettled(100e6);
        bytes32 open = oddDepositIt(oddQuote(50e6));
        proposeExitOn(oddDeposit);
        toWon();
        proposeExitOn(oddPool);
        vm.warp(block.timestamp + 7 days);

        oddUsdc.setMode(OddToken.Mode.Reenters);
        oddUsdc.setCallback(address(oddDeposit), abi.encodeCall(oddDeposit.refundSelf, (open)));
        uint256 before = vm.snapshotState();
        oddDeposit.emergencyExit(listOf(id));
        assertEq(oddUsdc.reentries(), 1);
        assertEq(bytes4(oddUsdc.reentryAnswer()), ChargeGovernance.Reentered.selector);
        assertEq(oddUsdc.balanceOf(treasury), 100e6);
        assertEq(oddDeposit.totalHeld(), 50e6);
        assertEq(uint8(oddDeposit.depositOf(open).state), uint8(ChargeDeposit.State.Deposited));
        vm.revertToState(before);
        oddUsdc.setCallback(address(oddDeposit), abi.encodeCall(oddDeposit.emergencyExit, (listOf(id))));
        oddDeposit.emergencyExit(listOf(id));
        assertEq(bytes4(oddUsdc.reentryAnswer()), ChargeGovernance.Reentered.selector);
        assertEq(oddUsdc.balanceOf(treasury), 100e6);

        oddWon.setMode(OddToken.Mode.Reenters);
        oddWon.setCallback(address(oddPool), abi.encodeCall(oddPool.emergencyExit, ()));
        oddPool.emergencyExit();
        assertEq(oddWon.reentries(), 1);
        assertEq(bytes4(oddWon.reentryAnswer()), ChargeGovernance.Reentered.selector);
        assertEq(oddWon.balanceOf(treasury), POOL_SIZE);
        assertEq(oddWon.balanceOf(address(oddPool)), 0);
    }
}
