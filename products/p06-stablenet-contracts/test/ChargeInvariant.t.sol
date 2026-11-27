// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Test} from "forge-std/Test.sol";
import {ChargeBase} from "./ChargeBase.t.sol";
import {ChargeTypes} from "../src/charge/ChargeTypes.sol";
import {ChargeDeposit} from "../src/charge/ChargeDeposit.sol";
import {WonPool} from "../src/charge/WonPool.sol";

/// Drives both contracts with every key in play: deposits, payouts with and without a deposit behind them,
/// completions, contests, answers, refunds, spending, stray coins and time. A call that the contracts refuse
/// is simply skipped; what is recorded here is only what went through.
contract ChargeHandler is ChargeBase {
    uint256[3] internal payerKeys = [uint256(0xB001), 0xB002, 0xB003];
    address[3] internal boards = [address(0xB0A2D1), address(0xB0A2D2), address(0xB0A2D3)];

    bytes32[] public ids;
    mapping(bytes32 => ChargeTypes.Quote) internal quotes;
    mapping(bytes32 => uint256) public payCount;
    uint256 public minted;
    uint256 public stray;
    uint256 public spent;
    uint256 public paidWon;
    bool public limitBroken;

    struct Pay {
        uint256 at;
        uint256 wonAmount;
        address board;
        address payer;
        bool fast;
    }

    Pay[] internal pays;

    constructor() {
        setUp();
    }

    /// The place for the later re-exchange contract is filled with this handler, so spending is driven too.
    function installSpender() external {
        toSource();
        proposeOn(deposit, keccak256(abi.encode(keccak256("setSpender"), address(this), bytes32(0))));
        vm.warp(block.timestamp + 24 hours);
        deposit.setSpender(address(this), 0);
    }

    function count() external view returns (uint256) {
        return ids.length;
    }

    function boardAt(uint256 i) external view returns (address) {
        return boards[i];
    }

    function payerAt(uint256 i) public view returns (address) {
        return vm.addr(payerKeys[i]);
    }

    function newQuote(uint256 amountSeed, uint256 boardSeed, uint256 payerSeed)
        internal
        returns (ChargeTypes.Quote memory quote, uint256 key)
    {
        key = payerKeys[payerSeed % 3];
        quote = quoteFor(bound(amountSeed, usdcFor(MIN_WON), usdcFor(MAX_WON)), boards[boardSeed % 3]);
        quote.payer = vm.addr(key);
    }

    // ---- actions ----

    function depositOne(uint256 amountSeed, uint256 boardSeed, uint256 payerSeed) external {
        toSource();
        (ChargeTypes.Quote memory quote, uint256 key) = newQuote(amountSeed, boardSeed, payerSeed);
        usdc.mint(quote.payer, quote.usdcAmount);
        minted += quote.usdcAmount;
        bytes32 id = deposit.quoteDigest(quote);
        bytes memory quoteSig = sign(QUOTER_KEY, id);
        bytes memory auth = authorizationFor(quote, key, address(deposit), id);
        try deposit.depositWithAuthorization(quote, quoteSig, auth) {
            ids.push(id);
            quotes[id] = quote;
        } catch {}
    }

    function payOne(uint256 seed, bool fast) external {
        if (ids.length == 0) return;
        pay(quotes[ids[seed % ids.length]], fast);
    }

    /// Relay, quote and confirmation keys used together for a quote nobody deposited for.
    function payUnbacked(uint256 amountSeed, uint256 boardSeed, uint256 payerSeed, bool fast) external {
        (ChargeTypes.Quote memory quote,) = newQuote(amountSeed, boardSeed, payerSeed);
        pay(quote, fast);
    }

    function pay(ChargeTypes.Quote memory quote, bool fast) internal {
        PayArgs memory args = payArgs(quote, fast);
        toWon();
        vm.prank(vm.addr(RELAY_KEY));
        try pool.pay(args.origin, args.quote, args.quoteSignature, args.confirmation, args.witnessSignature) {
            payCount[idOf(quote)] += 1;
            paidWon += quote.wonAmount;
            pays.push(Pay(block.timestamp, quote.wonAmount, quote.board, quote.payer, fast));
            checkLimits(quote, fast);
        } catch {}
    }

    /// Adds up what went out in the 24 hours that end now, for the board, the wallet, the fast path and all.
    function checkLimits(ChargeTypes.Quote memory quote, bool fast) internal {
        uint256 total;
        uint256 toBoard;
        uint256 fromPayer;
        uint256 fastTotal;
        for (uint256 i = pays.length; i > 0; --i) {
            Pay memory p = pays[i - 1];
            if (p.at + 24 hours <= block.timestamp) break;
            total += p.wonAmount;
            if (p.board == quote.board) toBoard += p.wonAmount;
            if (p.payer == quote.payer) fromPayer += p.wonAmount;
            if (p.fast) fastTotal += p.wonAmount;
        }
        if (
            total > TOTAL_DAILY || toBoard > ACCOUNT_DAILY || fromPayer > ACCOUNT_DAILY || fastTotal > FAST_DAILY
                || quote.wonAmount > MAX_WON || (fast && quote.wonAmount > FAST_EACH)
        ) limitBroken = true;
    }

    function completeOne(uint256 seed, bool bySigners) external {
        if (ids.length == 0) return;
        bytes32 id = ids[seed % ids.length];
        toSource();
        uint256 until = block.timestamp + 1 hours;
        if (bySigners) {
            bytes[2] memory signatures = two(deposit, "complete", id, deposit.depositOf(id).round, until);
            try deposit.completeBySigners(id, until, signatures) {} catch {}
        } else {
            bytes memory signature = sign(WITNESS_KEY, deposit.paidDigest(id, WON_BLOCK, until));
            vm.prank(vm.addr(RELAY_KEY));
            try deposit.complete(id, WON_BLOCK, until, signature) {} catch {}
        }
    }

    function contestOne(uint256 seed) external {
        if (ids.length == 0) return;
        bytes32 id = ids[seed % ids.length];
        toSource();
        vm.prank(deposit.depositOf(id).payer);
        try deposit.contest(id, 0, "") {} catch {}
    }

    function answerOne(uint256 seed, bool paid) external {
        if (ids.length == 0) return;
        bytes32 id = ids[seed % ids.length];
        toSource();
        uint256 until = block.timestamp + 1 hours;
        uint256 round = deposit.depositOf(id).round;
        if (paid) {
            try deposit.confirmPaid(id, until, two(deposit, "confirmPaid", id, round, until)) {} catch {}
        } else {
            try deposit.reject(id, until, two(deposit, "reject", id, round, until)) {} catch {}
        }
    }

    function refundOne(uint256 seed, bool bySigners) external {
        if (ids.length == 0) return;
        bytes32 id = ids[seed % ids.length];
        toSource();
        if (bySigners) {
            uint256 until = block.timestamp + 1 hours;
            bytes[2] memory signatures = two(deposit, "refund", refundSubject(id), deposit.depositOf(id).round, until);
            try deposit.refundBySigners(id, WON_BLOCK, until, signatures) {} catch {}
        } else {
            try deposit.refundSelf(id) {} catch {}
        }
    }

    function spendOne(uint256 seed, uint256 amount) external {
        if (ids.length == 0) return;
        bytes32 id = ids[seed % ids.length];
        toSource();
        uint256 left = deposit.depositOf(id).left;
        if (left == 0) return;
        amount = bound(amount, 1, left);
        try deposit.spend(id, amount, address(0x5E17)) {
            spent += amount;
        } catch {}
    }

    function sendStray(uint256 amount) external {
        amount = bound(amount, 1, 10e6);
        usdc.mint(address(deposit), amount);
        stray += amount;
    }

    function wait(uint256 time) external {
        vm.warp(block.timestamp + bound(time, 1 minutes, 3 days));
    }

    // ---- what the invariants read ----

    function usdcOf(address account) external view returns (uint256) {
        return usdc.balanceOf(account);
    }

    function wonOf(address account) external view returns (uint256) {
        return assetBalance(account);
    }

    function contracts() external view returns (ChargeDeposit, WonPool) {
        return (deposit, pool);
    }

    function poolSize() external pure returns (uint256) {
        return POOL_SIZE;
    }

    function dailyLimit() external pure returns (uint256) {
        return TOTAL_DAILY;
    }
}

/// @notice The two sums a top-up must never break, under random use of every key.
/// The contract name ends in SettlementInvariantTest so the gas snapshot leaves it out, as the Makefile does
/// for the settlement invariants.
contract ChargeSettlementInvariantTest is Test {
    ChargeHandler internal handler;
    ChargeDeposit internal deposit;
    WonPool internal pool;

    function setUp() public {
        handler = new ChargeHandler();
        handler.installSpender();
        (deposit, pool) = handler.contracts();
        bytes4[] memory selectors = new bytes4[](10);
        selectors[0] = handler.depositOne.selector;
        selectors[1] = handler.payOne.selector;
        selectors[2] = handler.payUnbacked.selector;
        selectors[3] = handler.completeOne.selector;
        selectors[4] = handler.contestOne.selector;
        selectors[5] = handler.answerOne.selector;
        selectors[6] = handler.refundOne.selector;
        selectors[7] = handler.spendOne.selector;
        selectors[8] = handler.sendStray.selector;
        selectors[9] = handler.wait.selector;
        targetContract(address(handler));
        targetSelector(FuzzSelector({addr: address(handler), selectors: selectors}));
    }

    /// Every deposited USDC is in exactly one place: refunded to its wallet, spent from a completed deposit,
    /// still held for a completed deposit, or still held for an open one.
    function invariant_depositsEqualRefundedPlusCompletedPlusOpen() public view {
        uint256 deposited;
        uint256 refunded;
        uint256 completed;
        uint256 open;
        for (uint256 i; i < handler.count(); ++i) {
            ChargeDeposit.Deposit memory record = deposit.depositOf(handler.ids(i));
            deposited += record.usdcAmount;
            if (record.state == ChargeDeposit.State.Refunded) {
                assertEq(record.left, 0);
                refunded += record.usdcAmount;
            } else if (record.state == ChargeDeposit.State.Deposited) {
                assertEq(record.left, record.usdcAmount);
                open += record.left;
            } else {
                completed += record.left; // completed or contested
            }
        }
        assertEq(deposited, refunded + completed + handler.spent() + open);
        assertEq(deposit.totalHeld(), completed + open);
        assertEq(handler.usdcOf(address(deposit)), deposit.totalHeld() + handler.stray());
        assertEq(handler.usdcOf(address(0x5E17)), handler.spent());
        uint256 inWallets;
        for (uint256 i; i < 3; ++i) {
            inWallets += handler.usdcOf(handler.payerAt(i));
        }
        assertEq(inWallets, handler.minted() - deposited + refunded, "a refund returns exactly what was paid");
    }

    /// The pool pays each deposit number at most once, never more than it holds and never more than its
    /// limits in any 24 hours, even for quotes that have no deposit behind them.
    function invariant_poolPaysOncePerDepositAndWithinLimits() public view {
        for (uint256 i; i < handler.count(); ++i) {
            assertLe(handler.payCount(handler.ids(i)), 1);
        }
        assertFalse(handler.limitBroken());
        assertLe(pool.usedTotal(), handler.dailyLimit());
        uint256 onBoards;
        for (uint256 i; i < 3; ++i) {
            onBoards += handler.wonOf(handler.boardAt(i));
        }
        assertEq(onBoards, handler.paidWon());
        assertEq(handler.wonOf(address(pool)), handler.poolSize() - handler.paidWon());
    }
}
