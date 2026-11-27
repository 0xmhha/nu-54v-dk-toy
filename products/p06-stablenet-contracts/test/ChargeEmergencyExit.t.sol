// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {ChargeBase} from "./ChargeBase.t.sol";
import {ChargeTypes} from "../src/charge/ChargeTypes.sol";
import {ChargeGovernance} from "../src/charge/ChargeGovernance.sol";
import {ChargeDeposit} from "../src/charge/ChargeDeposit.sol";
import {WonPool} from "../src/charge/WonPool.sol";
import {CoinView} from "./ChargeMocks.sol";
import {Spender} from "./ChargeDeposit.t.sol";

/// @notice The emergency exit of both contracts: two signers propose it, it waits seven days, and then anyone
/// sends the money to the treasury that was fixed at deployment. One key cannot stop it, and neither can the
/// pauser together with one signer. On the deposit contract it takes only money no traveller can still claim.
contract ChargeEmergencyExitTest is ChargeBase {
    bytes32 internal constant PROPOSE_EXIT = keccak256("proposeEmergencyExit");
    bytes32 internal constant CANCEL_EXIT = keccak256("cancelEmergencyExit");
    uint8 internal constant NONE = uint8(ChargeGovernance.Status.None);
    uint8 internal constant WAITING = uint8(ChargeGovernance.Status.Waiting);
    uint8 internal constant READY = uint8(ChargeGovernance.Status.Ready);
    uint8 internal constant VOID = uint8(ChargeGovernance.Status.Void);
    address internal pauser = vm.addr(PAUSER_KEY);

    function confirmIt(bytes32 id) internal {
        toSource();
        uint256 until = block.timestamp + 1 days;
        deposit.confirmPaid(id, until, two(deposit, "confirmPaid", id, deposit.depositOf(id).round, until));
    }

    // ---- who starts it, and when it opens ----

    function testTheExitIsProposedByTwoSignersAndWaitsSevenDays() public {
        toWon();
        uint256 until = block.timestamp + 1 days;
        bytes32 digest = pool.actionDigest(PROPOSE_EXIT, exitSubject(pool), 0, until);
        bytes memory first = sign(SIGNER1_KEY, digest);
        vm.expectRevert(ChargeGovernance.SignersNotDistinct.selector);
        pool.proposeEmergencyExit(until, [first, first]);
        uint256[4] memory others = [RELAY_KEY, QUOTER_KEY, WITNESS_KEY, PAUSER_KEY];
        for (uint256 i; i < others.length; ++i) {
            vm.expectRevert(ChargeGovernance.Unauthorized.selector);
            pool.proposeEmergencyExit(until, [first, sign(others[i], digest)]);
        }
        // Signed for the treasury of this contract and for nothing else.
        bytes32 other = pool.actionDigest(PROPOSE_EXIT, bytes32(uint256(uint160(stranger))), 0, until);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        pool.proposeEmergencyExit(until, [sign(SIGNER1_KEY, other), sign(SIGNER2_KEY, other)]);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        pool.proposeEmergencyExit(block.timestamp + 7 days + 1, [first, sign(SIGNER3_KEY, digest)]);

        assertEq(exitStatus(pool), NONE);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        pool.emergencyExit(); // nothing was proposed

        uint256 proposedAt = block.timestamp;
        vm.expectEmit(address(pool));
        emit ChargeGovernance.EmergencyExitProposed(0, uint64(proposedAt + 7 days), uint64(proposedAt + 14 days));
        vm.prank(stranger); // the two signatures count, not the sender
        pool.proposeEmergencyExit(until, [first, sign(SIGNER3_KEY, digest)]);
        assertEq(pool.exitProposedAt(), proposedAt);
        assertEq(pool.exitRound(), 1);
        assertEq(exitStatus(pool), WAITING);

        // One at a time.
        bytes[2] memory again = exitSignatures(pool, "proposeEmergencyExit", SIGNER1_KEY, SIGNER2_KEY, until);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        pool.proposeEmergencyExit(until, again);

        // It is never the quicker way: when a change proposed at the same time is ready, the exit still waits.
        vm.warp(proposedAt + 24 hours);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        pool.emergencyExit();
        vm.warp(proposedAt + 7 days - 1);
        assertEq(exitStatus(pool), WAITING);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        pool.emergencyExit();
        vm.warp(proposedAt + 7 days);
        assertEq(exitStatus(pool), READY);
        vm.warp(proposedAt + 14 days);
        assertEq(exitStatus(pool), READY);
        uint256 open = vm.snapshotState();
        vm.prank(stranger); // anyone
        pool.emergencyExit();
        assertEq(assetBalance(treasury), POOL_SIZE * scale());
        vm.revertToState(open);

        // It lapses. The next one needs two new signatures and waits its seven days again.
        vm.warp(proposedAt + 14 days + 1);
        assertEq(exitStatus(pool), VOID);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        pool.emergencyExit();
        proposeExitOn(pool);
        assertEq(pool.exitRound(), 2);
        assertEq(exitStatus(pool), WAITING);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        pool.emergencyExit();
        assertEq(assetBalance(address(pool)), POOL_SIZE * scale());
    }

    // ---- who stops it ----

    /// Two signers together cancel the exit. The pauser cannot, one signer cannot, and the two of them
    /// together cannot. A cancelled exit is not started again by the signatures that started it.
    function testOnlyTwoSignersTogetherCancelTheExit() public {
        toWon();
        uint256 until = block.timestamp + 6 days;
        bytes[2] memory start = exitSignatures(pool, "proposeEmergencyExit", SIGNER1_KEY, SIGNER2_KEY, until);
        bytes[2] memory early = exitSignatures(pool, "cancelEmergencyExit", SIGNER1_KEY, SIGNER2_KEY, until);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        pool.cancelEmergencyExit(until, early); // nothing waits
        pool.proposeEmergencyExit(until, start);

        bytes32 digest = pool.actionDigest(CANCEL_EXIT, exitSubject(pool), 1, until);
        bytes memory leaked = sign(SIGNER3_KEY, digest);
        vm.startPrank(pauser);
        vm.expectRevert(ChargeTypes.InvalidSignature.selector);
        pool.cancelEmergencyExit(until, [bytes(""), bytes("")]);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        pool.cancelEmergencyExit(until, [sign(PAUSER_KEY, digest), leaked]);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        pool.cancelEmergencyExit(until, [leaked, sign(PAUSER_KEY, digest)]);
        vm.expectRevert(ChargeGovernance.SignersNotDistinct.selector);
        pool.cancelEmergencyExit(until, [leaked, leaked]);
        // It is no change: there is nothing for the pauser or one signer to cancel as they cancel a change.
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        pool.cancelProposal(bytes32(uint256(uint160(treasury))), 0, "");
        vm.stopPrank();
        // Signatures for another kind or another round do not cancel it.
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        pool.cancelEmergencyExit(until, start);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        pool.cancelEmergencyExit(until, early);
        assertEq(exitStatus(pool), WAITING);

        uint256 waiting = vm.snapshotState();
        bytes[2] memory stop = [leaked, sign(SIGNER2_KEY, digest)]; // any two of the three
        vm.expectEmit(address(pool));
        emit ChargeGovernance.EmergencyExitCancelled(1);
        vm.prank(stranger);
        pool.cancelEmergencyExit(until, stop);
        assertEq(exitStatus(pool), NONE);
        assertEq(pool.exitProposedAt(), 0);
        assertEq(pool.exitRound(), 2);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        pool.cancelEmergencyExit(until, stop);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        pool.proposeEmergencyExit(until, start); // the round has moved
        vm.warp(block.timestamp + 7 days);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        pool.emergencyExit();
        // The next exit is another round, so the old cancel does not stop it.
        proposeExitOn(pool);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        pool.cancelEmergencyExit(until, stop); // and the old signatures have ended
        assertEq(exitStatus(pool), WAITING);

        // It is cancelled the same way while it is open.
        vm.revertToState(waiting);
        vm.warp(block.timestamp + 5 days);
        until = block.timestamp + 6 days;
        stop = exitSignatures(pool, "cancelEmergencyExit", SIGNER1_KEY, SIGNER3_KEY, until);
        vm.warp(block.timestamp + 2 days);
        assertEq(exitStatus(pool), READY);
        pool.cancelEmergencyExit(until, stop);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        pool.emergencyExit();
        assertEq(assetBalance(address(pool)), POOL_SIZE * scale());
    }

    /// A cancel that was signed for one exit does not stop the next one, even inside its own time.
    function testACancelSignedForOneExitDoesNotStopTheNext() public {
        toWon();
        proposeExitOn(pool);
        uint256 until = block.timestamp + 6 days;
        bytes[2] memory stop = exitSignatures(pool, "cancelEmergencyExit", SIGNER1_KEY, SIGNER2_KEY, until);
        uint256 kept = vm.snapshotState();
        pool.cancelEmergencyExit(until, stop);
        proposeExitOn(pool);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        pool.cancelEmergencyExit(until, stop);
        // Unused, it is still for that one round only.
        vm.revertToState(kept);
        assertEq(pool.exitRound(), 1);
        pool.cancelEmergencyExit(until, stop);
        assertEq(pool.exitRound(), 2);
    }

    /// Nothing the pauser does touches the exit, and neither does a change of signers or of the pauser. After
    /// a change of signers the two signers of the day cancel it; the replaced key no longer counts.
    function testSwitchingKeysOffAndChangingKeysLeavesTheExitWaiting() public {
        proposeExitOn(deposit);
        uint256 proposedAt = block.timestamp;
        vm.startPrank(pauser);
        deposit.pauseDeposits();
        deposit.disableKey(ChargeGovernance.Key.Relay);
        deposit.disableKey(ChargeGovernance.Key.Quoter);
        deposit.disableKey(ChargeGovernance.Key.Witness);
        deposit.clearSpender();
        deposit.lowerLimit(MIN_WON);
        vm.stopPrank();
        uint256 until = block.timestamp + 1 days;
        bytes32 digest = deposit.actionDigest(keccak256("voidPauserReplacements"), 0, 0, until);
        deposit.voidPauserReplacements(until, sign(SIGNER3_KEY, digest));
        assertEq(deposit.generation(), 3);
        assertEq(deposit.pauserRound(), 1);

        uint256 newKey = 0xA008;
        proposeOn(deposit, "setSigner", abi.encode(uint8(0), vm.addr(newKey)), "salt");
        proposeOn(deposit, "setKey", abi.encode(ChargeGovernance.Key.Pauser, address(0xE0A)), "salt");
        vm.warp(block.timestamp + 24 hours);
        deposit.setKey(ChargeGovernance.Key.Pauser, address(0xE0A), "salt");
        deposit.setSigner(0, vm.addr(newKey), "salt");
        assertEq(deposit.generation(), 4);
        assertEq(deposit.pauserRound(), 3);

        assertEq(deposit.exitRound(), 1);
        assertEq(deposit.exitProposedAt(), proposedAt);
        assertEq(exitStatus(deposit), WAITING);
        assertTrue(deposit.depositsPaused()); // and the exit undid nothing

        until = block.timestamp + 1 days;
        bytes[2] memory old = exitSignatures(deposit, "cancelEmergencyExit", SIGNER1_KEY, SIGNER2_KEY, until);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.cancelEmergencyExit(until, old);
        uint256 kept = vm.snapshotState();
        deposit.cancelEmergencyExit(until, exitSignatures(deposit, "cancelEmergencyExit", newKey, SIGNER2_KEY, until));
        assertEq(exitStatus(deposit), NONE);
        vm.revertToState(kept);
        vm.warp(proposedAt + 7 days);
        assertEq(exitStatus(deposit), READY);
    }

    // ---- where the money goes ----

    /// The treasury is fixed at deployment on both contracts and is never one of the seven keys, at
    /// deployment or later: money that leaves through the exit does not go to a key that may have leaked.
    function testTheTreasuryIsFixedAndIsNoneOfTheKeys() public {
        assertEq(deposit.treasury(), treasury);
        ChargeDeposit.Config memory config = depositConfig(address(pool));
        config.treasury = address(0);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        new ChargeDeposit(keys(), config);
        config.treasury = vm.addr(SIGNER2_KEY);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        new ChargeDeposit(keys(), config);
        config.treasury = vm.addr(PAUSER_KEY);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        new ChargeDeposit(keys(), config);
        config.treasury = vm.computeCreateAddress(address(this), vm.getNonce(address(this)));
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        new ChargeDeposit(keys(), config); // the contract itself

        proposeOn(deposit, "setKey", abi.encode(ChargeGovernance.Key.Relay, treasury), "salt");
        proposeOn(deposit, "setSigner", abi.encode(uint8(1), treasury), "salt");
        vm.warp(block.timestamp + 24 hours);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.setKey(ChargeGovernance.Key.Relay, treasury, "salt");
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.setSigner(1, treasury, "salt");

        toWon();
        assertEq(pool.treasury(), treasury);
        WonPool.Config memory poolValues = poolConfig(address(deposit));
        poolValues.treasury = vm.addr(RELAY_KEY);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        new WonPool(keys(), poolValues);
        poolValues.treasury = vm.computeCreateAddress(address(this), vm.getNonce(address(this)));
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        new WonPool(keys(), poolValues);
    }

    // ---- the pool ----

    /// The pool's exit sends all it holds to the treasury. The service runs while the exit waits; a pause
    /// does not close it and it undoes no pause. After it the pool pays nothing and deposits are refunded.
    function testThePoolExitSendsEverythingToTheTreasuryAndStopsTheService() public {
        toWon();
        proposeExitOn(pool);
        uint256 proposedAt = block.timestamp;

        // The service goes on while the exit waits, and the normal way out is what it was: two signers, a
        // day, and the pauser can stop it.
        vm.warp(proposedAt + 3 days);
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        payIt(quote, false);
        completeIt(id);
        toWon();
        bytes32 stopped = proposeOn(pool, "withdraw", abi.encode(uint256(7)), "salt");
        proposeOn(pool, "withdraw", abi.encode(uint256(5)), "salt");
        vm.prank(pauser);
        pool.cancelProposal(stopped, 0, "");
        vm.warp(proposedAt + 4 days);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        pool.withdraw(7, "salt");
        pool.withdraw(5, "salt");
        assertEq(assetBalance(treasury), 5);

        vm.prank(pauser);
        pool.pausePays();
        vm.warp(proposedAt + 7 days);
        uint256 held = assetBalance(address(pool));
        assertEq(held, (POOL_SIZE - quote.wonAmount) * scale() - 5);
        vm.expectEmit(address(pool));
        emit ChargeGovernance.EmergencyExitTaken(bytes32(0), treasury, held);
        vm.prank(stranger);
        pool.emergencyExit();
        assertEq(assetBalance(treasury), held + 5);
        assertEq(assetBalance(address(pool)), 0);
        assertEq(assetBalance(stranger), 0);
        assertTrue(pool.paysPaused());
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        pool.emergencyExit(); // nothing is left

        // Money put in while the exit is open leaves the same way.
        fundPool(MAX_WON);
        pool.emergencyExit();
        assertEq(assetBalance(treasury), held + 5 + MAX_WON * scale());

        // The service has stopped: a payout fails whole and the deposit behind it goes back to its wallet.
        uint256 until = block.timestamp + 1 days;
        pool.unpausePays(until, two(pool, "unpausePays", 0, pool.pauseRound(), until));
        quote = quoteFor(50e6, board);
        id = depositIt(quote);
        vm.expectRevert();
        this.payItExternal(quote, false);
        assertEq(pool.payoutOf(id).paidAt, 0);
        vm.warp(quote.payBy + 2 hours);
        refundBySigners(id);
        assertEq(usdc.balanceOf(payer), 1_000_000e6 - 100e6);
    }

    // ---- the deposit contract ----

    /// One deposit for every state a deposit can be in when the exit opens.
    struct Each {
        bytes32 old;
        bytes32 settled;
        bytes32 before;
        bytes32 during;
        bytes32 open;
        bytes32 rejected;
        bytes32 contested;
        bytes32 fresh;
    }

    /// One deposit in every state while the exit is open. It takes what is left of a completed deposit that
    /// is settled or past its contest window, and of nothing else: not of a deposit that is open, was
    /// rejected, is contested, is still inside its contest window or was refunded. Every refund works
    /// before the exit is proposed, while it waits, while it is open and after it lapsed.
    function testTheDepositExitTakesOnlyMoneyNoTravellerCanStillClaim() public {
        Each memory d;
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        d.old = depositIt(quote);
        payIt(quote, false);
        completeIt(d.old);
        d.settled = depositIt(quoteFor(50e6, board));
        completeIt(d.settled);
        contestIt(d.settled);
        confirmIt(d.settled);
        d.before = depositIt(quoteFor(30e6, board));
        usdc.mint(address(deposit), 5e6); // stray coins, which are nobody's deposit
        vm.warp(block.timestamp + 5 hours);
        refundBySigners(d.before); // a refund before any exit
        assertEq(usdc.balanceOf(payer), 1_000_000e6 - 150e6);

        proposeExitOn(deposit);
        uint256 proposedAt = block.timestamp;
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.emergencyExit(listOf(d.old));
        d.during = depositIt(quoteFor(20e6, board));
        vm.warp(proposedAt + 5 hours);
        refundBySigners(d.during); // a refund while the exit waits
        assertEq(usdc.balanceOf(payer), 1_000_000e6 - 150e6);

        vm.warp(proposedAt + 6 days);
        d.open = depositIt(quoteFor(10e6, board));
        d.rejected = depositIt(quoteFor(11e6, board));
        completeIt(d.rejected);
        rejectIt(d.rejected);
        d.contested = depositIt(quoteFor(12e6, board));
        completeIt(d.contested);
        contestIt(d.contested);
        d.fresh = depositIt(quoteFor(13e6, board));
        completeIt(d.fresh);
        vm.prank(pauser);
        deposit.pauseDeposits(); // a pause does not close the exit

        vm.warp(proposedAt + 7 days);
        assertEq(exitStatus(deposit), READY);
        bytes32[5] memory claimable = [d.open, d.rejected, d.contested, d.before, bytes32("no deposit")];
        for (uint256 i; i < claimable.length; ++i) {
            vm.expectRevert(ChargeGovernance.InvalidState.selector);
            deposit.emergencyExit(listOf(claimable[i]));
        }
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.emergencyExit(listOf(d.fresh)); // its payer can still contest it
        // One deposit that is not final undoes the whole call, wherever it stands in the list.
        bytes32[] memory ids = new bytes32[](3);
        (ids[0], ids[1], ids[2]) = (d.old, d.open, d.settled);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.emergencyExit(ids);
        (ids[0], ids[1], ids[2]) = (d.old, d.settled, d.old); // named twice
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.emergencyExit(ids);
        assertEq(usdc.balanceOf(treasury), 0);
        assertEq(deposit.totalHeld(), 196e6);

        ids = new bytes32[](2);
        (ids[0], ids[1]) = (d.old, d.settled);
        vm.expectEmit(address(deposit));
        emit ChargeGovernance.EmergencyExitTaken(d.old, treasury, 100e6);
        vm.expectEmit(address(deposit));
        emit ChargeGovernance.EmergencyExitTaken(d.settled, treasury, 50e6);
        vm.prank(stranger);
        deposit.emergencyExit(ids);
        assertEq(usdc.balanceOf(treasury), 150e6);
        assertEq(usdc.balanceOf(stranger), 0);
        assertEq(deposit.totalHeld(), 46e6);
        assertEq(usdc.balanceOf(address(deposit)), 46e6 + 5e6);
        assertEq(deposit.depositOf(d.old).left, 0);
        assertEq(uint8(state(d.old)), uint8(ChargeDeposit.State.Completed));
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.emergencyExit(listOf(d.old)); // nothing is left of it
        assertTrue(deposit.depositsPaused());
        // As after spending, a completion whose money has left is not taken back.
        uint256 until = block.timestamp + 1 days;
        bytes[2] memory reject = two(deposit, "reject", d.old, deposit.depositOf(d.old).round, until);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.reject(d.old, until, reject);

        // Refunds while the exit is open: by two signers, and the contest that leads to one.
        refundBySigners(d.open);
        contestIt(d.fresh);
        assertEq(usdc.balanceOf(payer), 1_000_000e6 - 150e6 - 36e6);

        // And after it lapsed: by anyone, for the rejected deposit and the two unanswered contests.
        vm.warp(proposedAt + 14 days + 1);
        assertEq(exitStatus(deposit), VOID);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.emergencyExit(listOf(d.fresh));
        vm.prank(stranger);
        deposit.refundSelf(d.rejected);
        vm.warp(proposedAt + 7 days + 14 days);
        vm.startPrank(stranger);
        deposit.refundSelf(d.contested);
        deposit.refundSelf(d.fresh);
        vm.stopPrank();
        assertEq(usdc.balanceOf(payer), 1_000_000e6 - 150e6);
        assertEq(usdc.balanceOf(treasury), 150e6);
        assertEq(deposit.totalHeld(), 0);
        assertEq(usdc.balanceOf(address(deposit)), 5e6); // the stray coins were never touched
    }

    /// A completion becomes final while the exit is open: by the end of its contest window, or at once when
    /// two signers answer a contest. The exit then takes it; what was completed before the exit opened is
    /// final before it closes.
    function testWhatBecomesFinalWhileTheExitIsOpenIsTaken() public {
        proposeExitOn(deposit);
        uint256 proposedAt = block.timestamp;
        vm.warp(proposedAt + 7 days - 3 minutes);
        bytes32 last = depositIt(quoteFor(40e6, board));
        bytes32 answered = depositIt(quoteFor(60e6, board));
        vm.warp(proposedAt + 7 days);
        completeIt(last); // completed in the very second the exit opens
        completeIt(answered);
        contestIt(answered);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.emergencyExit(listOf(answered));
        confirmIt(answered);
        deposit.emergencyExit(listOf(answered));
        assertEq(usdc.balanceOf(treasury), 60e6);

        vm.warp(proposedAt + 14 days - 1);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.emergencyExit(listOf(last));
        vm.warp(proposedAt + 14 days);
        deposit.emergencyExit(listOf(last)); // the last second of the exit is the first of its finality
        assertEq(usdc.balanceOf(treasury), 100e6);
        assertEq(deposit.totalHeld(), 0);
    }

    /// The exit takes what the spender left, and leaves the spender nothing to take.
    function testTheExitTakesWhatTheSpenderLeft() public {
        bytes32 id = depositIt(quoteFor(100e6, board));
        completeIt(id);
        Spender later = new Spender(deposit);
        proposeOn(deposit, "setSpender", spenderArgs(address(later)), "salt");
        proposeExitOn(deposit);
        vm.warp(block.timestamp + 24 hours);
        deposit.setSpender(address(later), address(later).codehash, "salt");
        vm.warp(block.timestamp + 6 days);
        later.spend(id, 60e6, stranger);
        deposit.emergencyExit(listOf(id));
        assertEq(usdc.balanceOf(treasury), 40e6);
        assertEq(usdc.balanceOf(stranger), 60e6);
        assertEq(deposit.totalHeld(), 0);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        later.spend(id, 1, stranger);
    }

    /// Without a proposal that waited seven days nobody takes anything, whatever state a deposit is in.
    function testNothingLeavesTheDepositContractWithoutAnOpenExit() public {
        bytes32 id = depositIt(quoteFor(100e6, board));
        completeIt(id);
        vm.warp(block.timestamp + 30 days);
        uint256[5] memory callers = [PAUSER_KEY, SIGNER1_KEY, RELAY_KEY, WITNESS_KEY, QUOTER_KEY];
        for (uint256 i; i < callers.length; ++i) {
            vm.prank(vm.addr(callers[i]));
            vm.expectRevert(ChargeGovernance.InvalidState.selector);
            deposit.emergencyExit(listOf(id));
        }
        assertEq(deposit.totalHeld(), 100e6);
    }
}

/// @notice The same exit with StableNet's coin as the pool's asset.
contract ChargeEmergencyExitCoinTest is ChargeEmergencyExitTest {
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

    function fundPool(uint256 wonAmount) internal override {
        vm.deal(address(this), wonAmount * 1e16);
        (bool ok,) = address(pool).call{value: wonAmount * 1e16}("");
        assertTrue(ok);
    }
}
