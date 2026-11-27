// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Test} from "forge-std/Test.sol";
import {ChargeBase} from "./ChargeBase.t.sol";
import {ChargeTypes} from "../src/charge/ChargeTypes.sol";
import {ChargeGovernance} from "../src/charge/ChargeGovernance.sol";
import {ChargeDeposit} from "../src/charge/ChargeDeposit.sol";
import {WonPool} from "../src/charge/WonPool.sol";
import {TestKRW3009} from "../src/test-token/TestKRW3009.sol";

/// Drives both contracts with every key in play: deposits, payouts with and without a deposit behind them,
/// completions, contests, answers, refunds, spending, stray coins and time; and between them the pauser
/// stopping and narrowing, two signers undoing it, and pool withdrawals that go through or are cancelled.
/// Every seat is given to new keys, with the key in the seat trying to stop it; another token is returned
/// from the pool; and the emergency exit is proposed, cancelled and used on both contracts.
/// A call that the contracts refuse is simply skipped; what is recorded here is only what went through.
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
    uint256 public withdrawn;
    /// What was put into the pool: at the start, and again after an exit emptied it.
    uint256 public funded = POOL_SIZE;
    uint256 public exitedWon;
    uint256 public exitedUsdc;
    uint256 public strayOther;
    uint256 public returnedOther;
    bool public limitBroken;
    /// Something went through that a pause, a key that is off or a lowered limit forbids.
    bool public stopBroken;
    /// A cancelled change was applied.
    bool public cancelBroken;
    /// The key in a seat stopped the change that takes its seat, or the pauser with one signer stopped an exit.
    bool public seatBroken;
    /// An exit moved money while it was not open.
    bool public exitBroken;
    /// Money left a deposit other than as a refund while its payer could still claim it.
    bool public tookRefundable;
    /// The pool's own asset left as if it were another token.
    bool public assetReturned;
    uint256 internal salts;
    /// The keys in the seven seats now: relay, quote, confirmation, pauser and signers 1 to 3. A seat is
    /// given to a new key on both contracts together, as one operator would.
    uint256[7] internal seats = [RELAY_KEY, QUOTER_KEY, WITNESS_KEY, PAUSER_KEY, SIGNER1_KEY, SIGNER2_KEY, SIGNER3_KEY];
    uint256 internal newKeys = 0xC000;
    TestKRW3009 internal other;
    /// What two signers did to the exit of the deposit contract (0) and of the pool (1): when it was
    /// proposed and how often it was proposed or cancelled. Nothing else may move either.
    uint256[2] public exitAt;
    uint256[2] public exitRounds;

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
        toWon();
        other = new TestKRW3009(address(this));
        toSource();
    }

    function relayKey() internal view override returns (uint256) {
        return seats[0];
    }

    function quoterKey() internal view override returns (uint256) {
        return seats[1];
    }

    function witnessKey() internal view override returns (uint256) {
        return seats[2];
    }

    function pauserKey() internal view override returns (uint256) {
        return seats[3];
    }

    function signerKey(uint256 index) internal view override returns (uint256) {
        return seats[4 + index];
    }

    function onContract(uint256 which) internal returns (ChargeGovernance) {
        if (which == 0) {
            toSource();
            return deposit;
        }
        toWon();
        return pool;
    }

    /// Completed money its payer can no longer contest, as the handler reads the record itself.
    function isFinal(bytes32 id) internal view returns (bool) {
        ChargeDeposit.Deposit memory record = deposit.depositOf(id);
        return record.state == ChargeDeposit.State.Completed
            && (record.settled || block.timestamp >= uint256(record.completedAt) + 7 days);
    }

    /// The place for the later re-exchange contract is filled with this handler, so spending is driven too.
    function installSpender() external {
        toSource();
        proposeOn(deposit, "setSpender", spenderArgs(address(this)), 0);
        vm.warp(block.timestamp + 24 hours);
        deposit.setSpender(address(this), address(this).codehash, 0);
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
        bytes memory quoteSig = sign(quoterKey(), id);
        bytes memory auth = authorizationFor(quote, key, address(deposit), id);
        bool stopped = deposit.depositsPaused() || deposit.quoter() == address(0) || quote.wonAmount > deposit.maxWon();
        try deposit.depositWithAuthorization(quote, quoteSig, auth) {
            ids.push(id);
            quotes[id] = quote;
            if (stopped) stopBroken = true;
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
        bool stopped = payStopped(quote, fast);
        vm.prank(vm.addr(relayKey()));
        try pool.pay(args.origin, args.quote, args.quoteSignature, args.confirmation, args.witnessSignature) {
            payCount[idOf(quote)] += 1;
            paidWon += quote.wonAmount;
            pays.push(Pay(block.timestamp, quote.wonAmount, quote.board, quote.payer, fast));
            checkLimits(quote, fast);
            if (stopped || overLimitsNow(quote, fast)) stopBroken = true;
        } catch {}
    }

    /// A payout must not go through while payouts are paused, a hot key is off, or over the limits as they
    /// are now for one payout.
    function payStopped(ChargeTypes.Quote memory quote, bool fast) internal view returns (bool) {
        (uint256 perPayout,,, uint256 fastPerPayout,) = pool.limits();
        return pool.paysPaused() || pool.relay() == address(0) || pool.quoter() == address(0)
            || pool.witness() == address(0) || quote.wonAmount > perPayout || (fast && quote.wonAmount > fastPerPayout);
    }

    /// Right after a payout, what is counted for the day is within the limits as they are now.
    function overLimitsNow(ChargeTypes.Quote memory quote, bool fast) internal view returns (bool) {
        (, uint256 accountDaily, uint256 totalDaily,, uint256 fastDaily) = pool.limits();
        return pool.usedTotal() > totalDaily || pool.usedByBoard(quote.board) > accountDaily
            || pool.usedByPayer(quote.payer) > accountDaily || (fast && pool.usedFast() > fastDaily);
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
            bytes memory signature = sign(witnessKey(), deposit.paidDigest(id, WON_BLOCK, until));
            bool stopped = deposit.relay() == address(0) || deposit.witness() == address(0);
            vm.prank(vm.addr(relayKey()));
            try deposit.complete(id, WON_BLOCK, until, signature) {
                if (stopped) stopBroken = true;
            } catch {}
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
        bool final_ = isFinal(id);
        try deposit.spend(id, amount, address(0x5E17)) {
            spent += amount;
            if (!final_) tookRefundable = true;
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

    // ---- stopping, narrowing and the changes that wait a day ----

    /// The pauser stops or narrows one thing on one of the two contracts.
    function narrow(uint256 seed) external {
        uint256 what = seed % 7;
        uint256 rest = seed / 7;
        if (what < 4) toSource();
        else toWon();
        vm.startPrank(vm.addr(pauserKey()));
        if (what == 0) {
            deposit.pauseDeposits();
        } else if (what == 1) {
            try deposit.disableKey(ChargeGovernance.Key(rest % 3)) {} catch {}
        } else if (what == 2) {
            deposit.lowerLimit(bound(rest, MIN_WON, deposit.maxWon()));
        } else if (what == 3) {
            deposit.clearSpender();
        } else if (what == 4) {
            pool.pausePays();
        } else if (what == 5) {
            try pool.disableKey(ChargeGovernance.Key(rest % 3)) {} catch {}
        } else {
            (uint256 a, uint256 b, uint256 c, uint256 d, uint256 e) = pool.limits();
            pool.lowerLimits(WonPool.Limits(a / 2, b / 2, c / 2, d / 2, e / 2));
        }
        vm.stopPrank();
    }

    /// Two signers undo every pause at once and, after a day, bring back every key that is off, the first
    /// limits and the spender.
    function restore() external {
        uint256 until = block.timestamp + 1 hours;
        toSource();
        if (deposit.depositsPaused()) {
            deposit.unpauseDeposits(until, two(deposit, "unpauseDeposits", 0, deposit.pauseRound(), until));
        }
        toWon();
        if (pool.paysPaused()) pool.unpausePays(until, two(pool, "unpausePays", 0, pool.pauseRound(), until));

        bytes32 salt = bytes32(++salts);
        bool[3] memory offDeposit = keysOff(deposit);
        bool[3] memory offPool = keysOff(pool);
        bool limitLow = deposit.maxWon() != MAX_WON;
        bool spenderGone = deposit.spender() == address(0);
        (uint256 perPayout,, uint256 totalDaily,,) = pool.limits();
        bool limitsLow = perPayout != MAX_WON || totalDaily != TOTAL_DAILY;
        toSource();
        for (uint256 k; k < 3; ++k) {
            if (offDeposit[k]) proposeOn(deposit, "setKey", abi.encode(k, hotKey(k)), salt);
        }
        if (limitLow) proposeOn(deposit, "raiseLimit", abi.encode(MAX_WON), salt);
        if (spenderGone) proposeOn(deposit, "setSpender", spenderArgs(address(this)), salt);
        toWon();
        for (uint256 k; k < 3; ++k) {
            if (offPool[k]) proposeOn(pool, "setKey", abi.encode(k, hotKey(k)), salt);
        }
        if (limitsLow) proposeOn(pool, "raiseLimits", abi.encode(limits()), salt);

        vm.warp(block.timestamp + 24 hours);
        for (uint256 k; k < 3; ++k) {
            if (offDeposit[k]) deposit.setKey(ChargeGovernance.Key(k), hotKey(k), salt);
            if (offPool[k]) pool.setKey(ChargeGovernance.Key(k), hotKey(k), salt);
        }
        if (limitLow) deposit.raiseLimit(MAX_WON, salt);
        if (spenderGone) deposit.setSpender(address(this), address(this).codehash, salt);
        if (limitsLow) pool.raiseLimits(limits(), salt);
    }

    function hotKey(uint256 k) internal view returns (address) {
        return vm.addr(seats[k]);
    }

    function keysOff(ChargeGovernance target) internal view returns (bool[3] memory) {
        return [target.relay() == address(0), target.quoter() == address(0), target.witness() == address(0)];
    }

    /// Two signers take pool money out to the treasury. Or the pauser, or one signer, cancels the change
    /// while it waits, and then nothing may leave.
    function withdrawSome(uint256 amountSeed, uint256 cancelSeed) external {
        toWon();
        uint256 amount = bound(amountSeed, 1, 1_000 * WON);
        bytes32 salt = bytes32(++salts);
        bytes32 action = proposeOn(pool, "withdraw", abi.encode(amount), salt);
        uint256 how = cancelSeed % 3;
        if (how == 1) {
            vm.prank(vm.addr(pauserKey()));
            pool.cancelProposal(action, 0, "");
        } else if (how == 2) {
            uint256 until = block.timestamp + 1 hours;
            pool.cancelProposal(action, until, cancelSignature(pool, signerKey(2), action, until));
        }
        vm.warp(block.timestamp + 24 hours);
        try pool.withdraw(amount, salt) {
            withdrawn += amount;
            if (how != 0) cancelBroken = true;
        } catch {}
    }

    // ---- new keys in the seats ----

    /// Two signers give one of the seven seats to a new key, on both contracts. The key in a seat that can
    /// cancel, the pauser or a signer, tries to stop its own replacement and must fail. Then the change goes
    /// through; or another key stops it, and then it must not.
    function replaceSeat(uint256 seed) external {
        uint256 seat = seed % 7;
        uint256 how = (seed / 7) % 4; // 0 and 1: nobody stops it; 2 and 3: another key does
        address to = vm.addr(++newKeys);
        bytes32 salt = bytes32(++salts);
        for (uint256 which; which < 2; ++which) {
            ChargeGovernance target = onContract(which);
            bytes32 action = seat < 4
                ? proposeOn(target, "setKey", abi.encode(seat, to), salt)
                : proposeOn(target, "setSigner", abi.encode(seat - 4, to), salt);
            if (seat >= 3) tryOwnCancel(target, action, seat);
            if (how >= 2) cancelAs(target, action, seat, how);
        }
        vm.warp(block.timestamp + 24 hours);
        for (uint256 which; which < 2; ++which) {
            ChargeGovernance target = onContract(which);
            if (how < 2) {
                if (seat < 4) target.setKey(ChargeGovernance.Key(seat), to, salt);
                // The seat is 4, 5 or 6 here.
                // forge-lint: disable-next-line(unsafe-typecast)
                else target.setSigner(uint8(seat - 4), to, salt);
            } else if (applies(target, seat, to, salt)) {
                cancelBroken = true;
            }
        }
        if (how < 2) seats[seat] = newKeys;
    }

    function applies(ChargeGovernance target, uint256 seat, address to, bytes32 salt) internal returns (bool) {
        if (seat < 4) {
            try target.setKey(ChargeGovernance.Key(seat), to, salt) {
                return true;
            } catch {}
        } else {
            // The seat is 4, 5 or 6 here.
            // forge-lint: disable-next-line(unsafe-typecast)
            try target.setSigner(uint8(seat - 4), to, salt) {
                return true;
            } catch {}
        }
        return false;
    }

    /// The key in a seat tries to stop the change that takes that seat.
    function tryOwnCancel(ChargeGovernance target, bytes32 action, uint256 seat) internal {
        if (seat == 3) {
            vm.prank(vm.addr(seats[3]));
            try target.cancelProposal(action, 0, "") {
                seatBroken = true;
            } catch {}
        } else {
            uint256 until = block.timestamp + 1 hours;
            bytes memory signature = cancelSignature(target, seats[seat], action, until);
            try target.cancelProposal(action, until, signature) {
                seatBroken = true;
            } catch {}
        }
    }

    /// A key that may stop it does: the pauser, or a signer other than the one whose seat it is. For the
    /// pauser's seat it is a signer either way: by one cancel, or by voiding every such replacement at once.
    function cancelAs(ChargeGovernance target, bytes32 action, uint256 seat, uint256 how) internal {
        uint256 until = block.timestamp + 1 hours;
        if (seat == 3 && how == 2) {
            bytes32 digest = target.actionDigest(keccak256("voidPauserReplacements"), 0, target.pauserRound(), until);
            target.voidPauserReplacements(until, sign(seats[4], digest));
        } else if (how == 2) {
            vm.prank(vm.addr(seats[3]));
            target.cancelProposal(action, 0, "");
        } else {
            uint256 by = seat == 6 ? 4 : 6;
            target.cancelProposal(action, until, cancelSignature(target, seats[by], action, until));
        }
    }

    // ---- another token in the pool ----

    /// A token that is not the pool's asset arrives by mistake and two signers send it to the treasury, or
    /// the change is cancelled and it stays. The same change for the pool's own asset must never apply.
    function returnOther(uint256 amountSeed, uint256 cancelSeed) external {
        toWon();
        uint256 amount = bound(amountSeed, 1, 1_000);
        other.mint(address(pool), amount);
        strayOther += amount;
        bytes32 salt = bytes32(++salts);
        bytes32 action = proposeOn(pool, "returnToken", abi.encode(address(other), amount), salt);
        proposeOn(pool, "returnToken", abi.encode(asset, amount), salt);
        uint256 how = cancelSeed % 3;
        if (how == 1) {
            vm.prank(vm.addr(pauserKey()));
            pool.cancelProposal(action, 0, "");
        } else if (how == 2) {
            uint256 until = block.timestamp + 1 hours;
            pool.cancelProposal(action, until, cancelSignature(pool, signerKey(2), action, until));
        }
        vm.warp(block.timestamp + 24 hours);
        try pool.returnToken(asset, amount, salt) {
            assetReturned = true;
        } catch {}
        try pool.returnToken(address(other), amount, salt) {
            returnedOther += amount;
            if (how != 0) cancelBroken = true;
        } catch {}
    }

    // ---- the emergency exit ----

    /// Two signers propose the exit of one of the two contracts.
    function exitPropose(uint256 seed) external {
        uint256 which = seed % 2;
        ChargeGovernance target = onContract(which);
        uint256 until = block.timestamp + 1 hours;
        bytes[2] memory signatures = exitSignatures(target, "proposeEmergencyExit", signerKey(0), signerKey(1), until);
        try target.proposeEmergencyExit(until, signatures) {
            exitAt[which] = block.timestamp;
            exitRounds[which] += 1;
        } catch {}
    }

    /// The pauser with one signer tries to cancel the exit and must fail. Then two signers do, when the
    /// seed says so.
    function exitCancel(uint256 seed) external {
        uint256 which = seed % 2;
        ChargeGovernance target = onContract(which);
        uint256 until = block.timestamp + 1 hours;
        bytes[2] memory wrong = exitSignatures(target, "cancelEmergencyExit", pauserKey(), signerKey(2), until);
        vm.prank(vm.addr(pauserKey()));
        try target.cancelEmergencyExit(until, wrong) {
            seatBroken = true;
        } catch {}
        if ((seed / 2) % 4 != 0) return;
        bytes[2] memory signatures = exitSignatures(target, "cancelEmergencyExit", signerKey(1), signerKey(2), until);
        try target.cancelEmergencyExit(until, signatures) {
            exitAt[which] = 0;
            exitRounds[which] += 1;
        } catch {}
    }

    /// Anyone tries the exit of one contract: all of the pool, or every deposit one by one. Sometimes the
    /// clock is first moved to the moment a waiting exit opens.
    function exitTake(uint256 seed) external {
        uint256 which = seed % 2;
        ChargeGovernance target = onContract(which);
        if ((seed / 2) % 2 == 0 && target.emergencyExitStatus() == ChargeGovernance.Status.Waiting) {
            vm.warp(exitAt[which] + 7 days);
        }
        bool open = target.emergencyExitStatus() == ChargeGovernance.Status.Ready;
        vm.startPrank(stranger);
        if (which == 1) {
            uint256 held = assetBalance(address(pool));
            try pool.emergencyExit() {
                exitedWon += held;
                if (!open) exitBroken = true;
            } catch {}
        } else {
            for (uint256 i; i < ids.length; ++i) {
                bool final_ = isFinal(ids[i]);
                uint256 left = deposit.depositOf(ids[i]).left;
                try deposit.emergencyExit(listOf(ids[i])) {
                    exitedUsdc += left;
                    if (!open) exitBroken = true;
                    if (!final_) tookRefundable = true;
                } catch {}
            }
        }
        vm.stopPrank();
    }

    /// The pool is filled up again, as after an exit that emptied it.
    function refill() external {
        toWon();
        uint256 held = assetBalance(address(pool));
        if (held >= POOL_SIZE) return;
        fundPool(POOL_SIZE - held);
        funded += POOL_SIZE - held;
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

    function otherOf(address account) external view returns (uint256) {
        return other.balanceOf(account);
    }

    function treasuryUsdc() external view returns (uint256) {
        return usdc.balanceOf(treasury);
    }

    function treasuryWon() external view returns (uint256) {
        return assetBalance(treasury);
    }

    function treasuryAddress() external view returns (address) {
        return treasury;
    }

    function dailyLimit() external pure returns (uint256) {
        return TOTAL_DAILY;
    }
}

/// @notice The sums a top-up must never break, under random use of every key, of every change of keys and
/// of the emergency exit.
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
        bytes4[] memory selectors = new bytes4[](19);
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
        selectors[10] = handler.narrow.selector;
        selectors[11] = handler.restore.selector;
        selectors[12] = handler.withdrawSome.selector;
        selectors[13] = handler.replaceSeat.selector;
        selectors[14] = handler.returnOther.selector;
        selectors[15] = handler.exitPropose.selector;
        selectors[16] = handler.exitCancel.selector;
        selectors[17] = handler.exitTake.selector;
        selectors[18] = handler.refill.selector;
        targetContract(address(handler));
        targetSelector(FuzzSelector({addr: address(handler), selectors: selectors}));
    }

    /// Every deposited USDC is in exactly one place: refunded to its wallet, spent from a completed deposit,
    /// sent to the treasury by an exit, still held for a completed deposit, or still held for an open one.
    /// Money a traveller can still claim is never taken by any path the keys control: a deposit that is open
    /// or contested holds all of its USDC, and the spender and the exit only ever took from final deposits.
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
                if (record.state == ChargeDeposit.State.Contested) assertEq(record.left, record.usdcAmount);
                completed += record.left; // completed or contested
            }
        }
        assertFalse(handler.tookRefundable());
        assertEq(deposited, refunded + completed + handler.spent() + handler.exitedUsdc() + open);
        assertEq(handler.treasuryUsdc(), handler.exitedUsdc());
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
    /// limits in any 24 hours, even for quotes that have no deposit behind them. Nothing goes through a
    /// pause, a key that is off or a lowered limit. Its money leaves only as payouts to boards and to the
    /// treasury: by a change that was not cancelled, or by an exit that was open. Another token leaves only
    /// to the treasury, and the pool's asset never leaves as another token.
    function invariant_poolPaysOncePerDepositAndWithinLimits() public view {
        for (uint256 i; i < handler.count(); ++i) {
            assertLe(handler.payCount(handler.ids(i)), 1);
        }
        assertFalse(handler.limitBroken());
        assertFalse(handler.stopBroken());
        assertFalse(handler.cancelBroken());
        assertLe(pool.usedTotal(), handler.dailyLimit());
        uint256 onBoards;
        for (uint256 i; i < 3; ++i) {
            onBoards += handler.wonOf(handler.boardAt(i));
        }
        assertEq(onBoards, handler.paidWon());
        assertEq(
            handler.wonOf(address(pool)),
            handler.funded() - handler.paidWon() - handler.withdrawn() - handler.exitedWon()
        );
        assertEq(handler.treasuryWon(), handler.withdrawn() + handler.exitedWon());
        assertFalse(handler.assetReturned());
        assertEq(handler.otherOf(handler.treasuryAddress()), handler.returnedOther());
        assertEq(handler.otherOf(address(pool)), handler.strayOther() - handler.returnedOther());
    }

    /// Only two signers move an emergency exit. After any sequence, each contract's exit is where the two
    /// signers' own calls left it: no pauser, single signer, key switched off, pause or change of keys
    /// started, stopped or reset one. No key stopped the change that takes its own seat, and no exit moved
    /// money while it was not open.
    function invariant_onlyTwoSignersMoveTheExitAndNoKeyKeepsItsSeat() public view {
        assertEq(deposit.exitProposedAt(), handler.exitAt(0));
        assertEq(deposit.exitRound(), handler.exitRounds(0));
        assertEq(pool.exitProposedAt(), handler.exitAt(1));
        assertEq(pool.exitRound(), handler.exitRounds(1));
        assertFalse(handler.seatBroken());
        assertFalse(handler.exitBroken());
    }
}
