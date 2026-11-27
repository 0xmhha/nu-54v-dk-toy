// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {ChargeBase} from "./ChargeBase.t.sol";
import {ChargeTypes} from "../src/charge/ChargeTypes.sol";
import {ChargeGovernance} from "../src/charge/ChargeGovernance.sol";
import {WonPool} from "../src/charge/WonPool.sol";
import {Spender} from "./ChargeDeposit.t.sol";

/// @notice Keys in the wrong hands against the changes that wait a day. One key alone, the pauser or a signer,
/// can delay but cannot stay: the key whose place a change takes cannot stop that change. No key alone forces
/// a change through. Two keys together are named with what they can and cannot do, and what the emergency
/// exit changes for each pair.
contract ChargeLeakedKeysTest is ChargeBase {
    bytes32 internal constant SET_KEY = keccak256("setKey");
    uint256 internal constant NEW_KEY = 0xA008;
    uint256[3] internal signerKeys = [SIGNER1_KEY, SIGNER2_KEY, SIGNER3_KEY];
    address internal pauser = vm.addr(PAUSER_KEY);
    address internal next = vm.addr(NEW_KEY);
    /// A relay, a quote key and a confirmation key of the attackers' own.
    uint256[3] internal theirKeys = [uint256(0xD001), 0xD002, 0xD003];

    function proposePauser(ChargeGovernance target) internal returns (bytes32) {
        return proposeOn(target, "setKey", abi.encode(ChargeGovernance.Key.Pauser, next), "salt");
    }

    function proposeSigner(ChargeGovernance target, uint8 index) internal returns (bytes32) {
        return proposeOn(target, "setSigner", abi.encode(index, next), "salt");
    }

    /// One signer's cancel, sent by anyone. `refusal` is the expected error, or 0 when it must work.
    function cancelBy(ChargeGovernance target, uint256 key, bytes32 action, bytes4 refusal) internal {
        uint256 until = block.timestamp + 1 days;
        bytes memory signature = cancelSignature(target, key, action, until);
        vm.prank(stranger);
        if (refusal != 0) vm.expectRevert(refusal);
        target.cancelProposal(action, until, signature);
    }

    function cancelByPauser(ChargeGovernance target, bytes32 action, bytes4 refusal) internal {
        vm.prank(pauser);
        if (refusal != 0) vm.expectRevert(refusal);
        target.cancelProposal(action, 0, "");
    }

    function isWaiting(ChargeGovernance target, bytes32 action) internal view returns (bool) {
        (, ChargeGovernance.ProposalState state_,,) = target.proposals(action);
        return state_ == ChargeGovernance.ProposalState.Pending;
    }

    // ---- the pauser key alone ----

    /// While it is the pauser the key stops payouts, switches keys off, cancels every other change and
    /// front-runs the undoing of its pause. It cannot cancel or void its own replacement, and a day later it
    /// has no power left: the pause is undone and the pool's money leaves to the treasury.
    function testALeakedPauserDelaysButCannotLockThePool() public {
        toWon();
        uint256 amount = POOL_SIZE * scale();
        bytes32 withdrawal = proposeOn(pool, "withdraw", abi.encode(amount), "salt");
        bytes32 replacement = proposePauser(pool);
        vm.prank(pauser);
        pool.pausePays();
        cancelByPauser(pool, withdrawal, 0);
        cancelByPauser(pool, replacement, ChargeGovernance.Unauthorized.selector);
        cancelBy(pool, PAUSER_KEY, replacement, ChargeGovernance.Unauthorized.selector); // it is no signer either

        // Switching keys off voids the other changes, once per key, and never the replacement.
        bytes32 second = proposeOn(pool, "withdraw", abi.encode(amount), "salt 2");
        vm.startPrank(pauser);
        pool.disableKey(ChargeGovernance.Key.Relay);
        pool.disableKey(ChargeGovernance.Key.Quoter);
        pool.disableKey(ChargeGovernance.Key.Witness);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        pool.disableKey(ChargeGovernance.Key.Relay);
        vm.stopPrank();
        assertEq(pool.generation(), 3);
        assertEq(pool.pauserRound(), 0);
        cancelByPauser(pool, second, ChargeGovernance.InvalidState.selector); // void already

        // For as long as it is the pauser it makes the signatures that undo its pause useless.
        uint256 until = block.timestamp + 1 days;
        bytes[2] memory undo = two(pool, "unpausePays", 0, pool.pauseRound(), until);
        vm.prank(pauser);
        pool.pausePays();
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        pool.unpausePays(until, undo);

        vm.warp(block.timestamp + 24 hours - 1);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        pool.setKey(ChargeGovernance.Key.Pauser, next, "salt"); // the day of waiting holds for it too
        vm.warp(block.timestamp + 1);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        pool.withdraw(amount, "salt 2");
        vm.prank(stranger);
        pool.setKey(ChargeGovernance.Key.Pauser, next, "salt");
        assertEq(pool.pauser(), next);

        // After the replacement the old key does nothing.
        vm.startPrank(pauser);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        pool.pausePays();
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        pool.disableKey(ChargeGovernance.Key.Relay);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        pool.lowerLimits(limits());
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        pool.removeOrigin(SOURCE_CHAIN, address(deposit));
        vm.stopPrank();
        until = block.timestamp + 1 days;
        undo = two(pool, "unpausePays", 0, pool.pauseRound(), until);
        pool.unpausePays(until, undo);
        assertFalse(pool.paysPaused());
        bytes32 third = proposeOn(pool, "withdraw", abi.encode(amount), "salt 3");
        cancelByPauser(pool, third, ChargeGovernance.InvalidTime.selector); // judged as a signature now
        cancelBy(pool, PAUSER_KEY, third, ChargeGovernance.Unauthorized.selector);
        vm.warp(block.timestamp + 24 hours);
        pool.withdraw(amount, "salt 3");
        assertEq(assetBalance(treasury), amount);
        assertEq(assetBalance(address(pool)), 0);
    }

    /// The same on the deposit contract: completed USDC waits for a spender, and the pauser can clear the
    /// spender and cancel a new one only until its replacement has gone through.
    function testALeakedPauserCannotLockCompletedMoney() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        payIt(quote, false);
        completeIt(id);
        bytes32 spenderChange = proposeOn(deposit, "setSpender", spenderArgs(address(this)), "salt");
        bytes32 replacement = proposePauser(deposit);
        cancelByPauser(deposit, spenderChange, 0);
        cancelByPauser(deposit, replacement, ChargeGovernance.Unauthorized.selector);
        vm.startPrank(pauser);
        deposit.pauseDeposits();
        deposit.clearSpender();
        deposit.disableKey(ChargeGovernance.Key.Witness);
        vm.stopPrank();
        assertTrue(isWaiting(deposit, replacement));

        vm.warp(block.timestamp + 24 hours);
        deposit.setKey(ChargeGovernance.Key.Pauser, next, "salt");
        proposeOn(deposit, "setSpender", spenderArgs(address(this)), "salt 2");
        vm.prank(pauser);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.clearSpender();
        vm.warp(block.timestamp + 24 hours);
        deposit.setSpender(address(this), address(this).codehash, "salt 2");
        vm.warp(block.timestamp + 7 days);
        deposit.spend(id, 100e6, treasury);
        assertEq(usdc.balanceOf(treasury), 100e6);
    }

    // ---- one signer key alone ----

    /// A signer cancels any change by its signature, except the one that takes its own place. Every other
    /// key that could cancel before still can: the pauser and each of the two other signers.
    function testALeakedSignerCannotStopItsOwnReplacement() public {
        toWon();
        for (uint8 i; i < 3; ++i) {
            uint256 start = vm.snapshotState();
            bytes32 withdrawal = proposeWithout(i, "withdraw", abi.encode(uint256(1)), "salt");
            bytes32 replacement = proposeWithout(i, "setSigner", abi.encode(i, next), "salt");
            cancelBy(pool, signerKeys[i], withdrawal, 0);
            cancelBy(pool, signerKeys[i], replacement, ChargeGovernance.Unauthorized.selector);
            for (uint256 j; j < 3; ++j) {
                if (j == i) continue;
                uint256 before = vm.snapshotState();
                cancelBy(pool, signerKeys[j], replacement, 0);
                vm.revertToState(before);
            }
            uint256 beforePauser = vm.snapshotState();
            cancelByPauser(pool, replacement, 0);
            vm.revertToState(beforePauser);

            vm.warp(block.timestamp + 24 hours);
            pool.setSigner(i, next, "salt");
            assertEq(pool.signers(i), next);
            // The old key is no signer any more: it cancels nothing, and the new one does.
            bytes32 later = proposeWithout(i, "withdraw", abi.encode(uint256(1)), "salt 2");
            cancelBy(pool, signerKeys[i], later, ChargeGovernance.Unauthorized.selector);
            cancelBy(pool, NEW_KEY, later, 0);
            vm.revertToState(start);
        }
    }

    /// A proposal on the pool signed by the two signers other than signer `gone`.
    function proposeWithout(uint8 gone, string memory kind, bytes memory arguments, bytes32 salt)
        internal
        returns (bytes32 action)
    {
        uint256 until = block.timestamp + 1 days;
        bytes32 kindHash = keccak256(bytes(kind));
        action = pool.actionOf(kindHash, arguments, salt);
        bytes32 digest = pool.actionDigest(keccak256("propose"), action, pool.proposalRound(kindHash, arguments), until);
        bytes[2] memory signatures =
            [sign(signerKeys[(gone + 1) % 3], digest), sign(signerKeys[(gone + 2) % 3], digest)];
        pool.propose(kindHash, arguments, salt, until, signatures);
    }

    // ---- no key alone forces a change ----

    function testNoSingleKeyForcesAChange() public {
        bytes memory arguments = abi.encode(ChargeGovernance.Key.Pauser, next);
        bytes32 action = deposit.actionOf(SET_KEY, arguments, "salt");
        uint256 until = block.timestamp + 6 days;
        bytes32 digest = deposit.actionDigest(keccak256("propose"), action, 0, until);
        uint256[5] memory single = [RELAY_KEY, QUOTER_KEY, WITNESS_KEY, PAUSER_KEY, SIGNER3_KEY];
        for (uint256 i; i < single.length; ++i) {
            bytes memory one = sign(single[i], digest);
            vm.expectRevert(ChargeGovernance.SignersNotDistinct.selector);
            deposit.propose(SET_KEY, arguments, "salt", until, [one, one]);
            vm.expectRevert(ChargeGovernance.Unauthorized.selector);
            deposit.propose(SET_KEY, arguments, "salt", until, [one, sign(NEW_KEY, digest)]);
        }

        // The change the pauser cannot stop is stopped by any one signer, and then stays stopped.
        bytes[2] memory signatures = two(deposit, "propose", action, 0, until);
        deposit.propose(SET_KEY, arguments, "salt", until, signatures);
        for (uint256 i; i < 3; ++i) {
            uint256 before = vm.snapshotState();
            cancelBy(deposit, signerKeys[i], action, 0);
            vm.expectRevert(ChargeGovernance.InvalidState.selector);
            deposit.propose(SET_KEY, arguments, "salt", until, signatures);
            vm.warp(block.timestamp + 24 hours);
            vm.expectRevert(ChargeGovernance.InvalidState.selector);
            deposit.setKey(ChargeGovernance.Key.Pauser, next, "salt");
            vm.revertToState(before);
        }

        // Switching a key off does not void it; a change of signers voids it like every other change, and
        // the signatures made before do not start it again.
        vm.prank(pauser);
        deposit.disableKey(ChargeGovernance.Key.Relay);
        assertEq(deposit.proposalRound(SET_KEY, arguments), 0);
        uint256 moved = vm.snapshotState();
        cancelBy(deposit, SIGNER1_KEY, action, 0); // one signer still stops it, in the round it was proposed in
        vm.revertToState(moved);
        proposeOn(deposit, "setSigner", abi.encode(uint8(2), vm.addr(0xA009)), "salt");
        vm.warp(block.timestamp + 24 hours);
        deposit.setSigner(2, vm.addr(0xA009), "salt");
        assertEq(deposit.proposalRound(SET_KEY, arguments), 1);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.setKey(ChargeGovernance.Key.Pauser, next, "salt");
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.propose(SET_KEY, arguments, "salt", until, signatures);
        assertEq(deposit.pauser(), pauser);
    }

    // ---- two keys together ----

    /// Two signers in the wrong hands start any change. The honest pauser and the honest third signer each
    /// stop the change aimed at the other, and neither can stop the one aimed at itself.
    function testTwoLeakedSignersAreStoppedByThePauserAndTheThirdSigner() public {
        toWon();
        bytes32 takeThird = proposeSigner(pool, 2); // `two` signs with signers 1 and 2: the leaked ones
        bytes32 takePauser = proposePauser(pool);
        bytes32 withdrawal = proposeOn(pool, "withdraw", abi.encode(POOL_SIZE * scale()), "salt");
        cancelBy(pool, SIGNER3_KEY, takeThird, ChargeGovernance.Unauthorized.selector);
        cancelByPauser(pool, takePauser, ChargeGovernance.Unauthorized.selector);

        // Known cost of the rule: if the one key that can stop a change does not act within the day, the
        // change goes through.
        uint256 before = vm.snapshotState();
        vm.warp(block.timestamp + 24 hours);
        pool.setSigner(2, next, "salt");
        assertEq(pool.signers(2), next);
        vm.revertToState(before);

        cancelBy(pool, SIGNER3_KEY, takePauser, 0);
        cancelByPauser(pool, takeThird, 0);
        cancelByPauser(pool, withdrawal, 0);
        vm.warp(block.timestamp + 24 hours);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        pool.setSigner(2, next, "salt");
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        pool.setKey(ChargeGovernance.Key.Pauser, next, "salt");
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        pool.withdraw(POOL_SIZE * scale(), "salt");
        assertEq(pool.signers(2), vm.addr(SIGNER3_KEY));
        assertEq(pool.pauser(), pauser);
    }

    // ---- the pauser and one signer together ----

    function onChainOf(ChargeGovernance target) internal {
        if (address(target) == address(pool)) toWon();
        else toSource();
    }

    /// Every change that would end the lock, proposed by the two honest signers: a new pauser, a new third
    /// signer, and the normal way out for the money of that contract.
    function proposeEveryWayOut(ChargeGovernance target, bytes32 salt) internal returns (bytes32[3] memory actions) {
        onChainOf(target);
        actions[0] = proposeOn(target, "setKey", abi.encode(ChargeGovernance.Key.Pauser, next), salt);
        actions[1] = proposeOn(target, "setSigner", abi.encode(uint8(2), next), salt);
        actions[2] = address(target) == address(pool)
            ? proposeOn(target, "withdraw", abi.encode(assetBalance(address(pool))), salt)
            : proposeOn(target, "setSpender", spenderArgs(address(this)), salt);
    }

    /// The two leaked keys stop them all: signer 3 the replacement of the pauser, by a cancel or by voiding
    /// every such replacement at once, and the pauser the two others.
    function stopEveryWayOut(ChargeGovernance target, bytes32[3] memory actions, bool atOnce) internal {
        if (atOnce) {
            uint256 until = block.timestamp + 1 days;
            bytes32 digest = target.actionDigest(keccak256("voidPauserReplacements"), 0, target.pauserRound(), until);
            target.voidPauserReplacements(until, sign(SIGNER3_KEY, digest));
        } else {
            cancelBy(target, SIGNER3_KEY, actions[0], 0);
        }
        cancelByPauser(target, actions[1], 0);
        cancelByPauser(target, actions[2], 0);
    }

    /// A day later none of those changes can be applied.
    function assertNoWayOutApplies(ChargeGovernance target, bytes32 salt) internal {
        onChainOf(target);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        target.setKey(ChargeGovernance.Key.Pauser, next, salt);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        target.setSigner(2, next, salt);
        if (address(target) == address(pool)) {
            uint256 amount = assetBalance(address(pool));
            vm.expectRevert(ChargeGovernance.InvalidState.selector);
            pool.withdraw(amount, salt);
        } else {
            bytes32 codeHash = address(this).codehash;
            vm.expectRevert(ChargeGovernance.InvalidState.selector);
            deposit.setSpender(address(this), codeHash, salt);
        }
    }

    /// Everything else the leaked pauser can do, a little more each day.
    function stopAndNarrow(uint256 day) internal {
        toSource();
        vm.startPrank(pauser);
        deposit.pauseDeposits();
        deposit.clearSpender();
        if (day < 3) deposit.disableKey(ChargeGovernance.Key(day));
        if (day == 3) deposit.lowerLimit(MIN_WON);
        vm.stopPrank();
        toWon();
        vm.startPrank(pauser);
        pool.pausePays();
        if (day < 3) pool.disableKey(ChargeGovernance.Key(day));
        if (day == 3) pool.removeOrigin(SOURCE_CHAIN, address(deposit));
        if (day == 4) pool.lowerLimits(WonPool.Limits(MIN_WON, 0, 0, 0, 0));
        vm.stopPrank();
    }

    /// The pauser and signer 3 together are not two signers: the exit waits on, in the round it was proposed.
    function assertTheExitStillWaits(ChargeGovernance target) internal {
        onChainOf(target);
        uint256 until = block.timestamp + 1 days;
        bytes[2] memory stop = exitSignatures(target, "cancelEmergencyExit", PAUSER_KEY, SIGNER3_KEY, until);
        vm.prank(pauser);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        target.cancelEmergencyExit(until, stop);
        assertEq(exitStatus(target), uint8(ChargeGovernance.Status.Waiting));
        assertEq(target.exitRound(), 1);
    }

    /// The pauser and one signer in the wrong hands together stop each other's replacement, and with it
    /// every change on both contracts: the pool's won is not withdrawn and no spender is ever named for the
    /// completed USDC, however long it waits. A traveller's deposit is not caught in this: the two honest
    /// signers refund it, and after seven days anyone does. The way out is the exit, in the next test.
    function testALeakedPauserWithALeakedSignerBlocksEveryChangeOnBothContracts() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 completed = depositIt(quote);
        payIt(quote, false);
        completeIt(completed);
        bytes32 id = depositIt(quoteFor(60e6, board));
        bytes32 other = depositIt(quoteFor(50e6, board));
        uint256 start = block.timestamp;
        toWon();
        uint256 poolMoney = assetBalance(address(pool));

        stopEveryWayOut(pool, proposeEveryWayOut(pool, "salt"), false);
        stopEveryWayOut(deposit, proposeEveryWayOut(deposit, "salt"), true);
        vm.warp(start + 24 hours);
        assertNoWayOutApplies(pool, "salt");
        assertNoWayOutApplies(deposit, "salt");
        assertEq(assetBalance(address(pool)), poolMoney);

        refundBySigners(id); // signers 1 and 2
        vm.warp(start + 7 days);
        vm.prank(stranger);
        deposit.refundSelf(other);
        assertEq(usdc.balanceOf(payer), 1_000_000e6 - 100e6);

        // The lock on the deposit side: with no spender and no exit, completed USDC stays for ever.
        vm.warp(start + 365 days);
        assertEq(deposit.spender(), address(0));
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.spend(completed, 100e6, treasury);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.emergencyExit(listOf(completed)); // nobody proposed the exit
        assertEq(deposit.depositOf(completed).left, 100e6);
        assertEq(usdc.balanceOf(address(deposit)), 100e6);
        assertEq(assetBalance(address(pool)), poolMoney);
    }

    /// The whole sequence with the pauser and signer 3 in the wrong hands. The two honest signers propose
    /// the exit on both contracts. For seven days they also try every normal way out again, and every day
    /// the two leaked keys cancel it, stop and narrow what they can, and fail to stop the exit. On the
    /// seventh day anyone sends the pool's won and the completed USDC to the treasury, and the traveller who
    /// was never paid has the USDC back.
    function testTheExitEndsALeakedPauserWithALeakedSigner() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 completed = depositIt(quote);
        payIt(quote, false);
        completeIt(completed);
        bytes32 unpaid = depositIt(quoteFor(50e6, board));
        uint256 start = block.timestamp;
        toWon();
        uint256 poolMoney = assetBalance(address(pool));
        proposeExitOn(pool);
        toSource();
        proposeExitOn(deposit);

        for (uint256 day; day < 7; ++day) {
            vm.warp(start + day * 1 days);
            if (day > 0) {
                assertNoWayOutApplies(pool, bytes32(day - 1));
                assertNoWayOutApplies(deposit, bytes32(day - 1));
            }
            stopEveryWayOut(pool, proposeEveryWayOut(pool, bytes32(day)), day % 2 == 0);
            stopEveryWayOut(deposit, proposeEveryWayOut(deposit, bytes32(day)), day % 2 == 1);
            stopAndNarrow(day);
            assertTheExitStillWaits(pool);
            assertTheExitStillWaits(deposit);
            vm.expectRevert(ChargeGovernance.InvalidTime.selector);
            pool.emergencyExit();
            vm.expectRevert(ChargeGovernance.InvalidTime.selector);
            deposit.emergencyExit(listOf(completed));
        }
        vm.warp(start + 7 days - 1);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        pool.emergencyExit();

        vm.warp(start + 7 days);
        assertNoWayOutApplies(pool, bytes32(uint256(6)));
        assertNoWayOutApplies(deposit, bytes32(uint256(6)));
        toWon();
        vm.prank(stranger);
        pool.emergencyExit();
        assertEq(assetBalance(treasury), poolMoney);
        assertEq(assetBalance(address(pool)), 0);
        toSource();
        vm.prank(stranger);
        deposit.emergencyExit(listOf(completed));
        assertEq(usdc.balanceOf(treasury), 100e6);
        vm.prank(stranger);
        deposit.refundSelf(unpaid);
        assertEq(usdc.balanceOf(payer), 1_000_000e6 - 100e6);
        assertEq(deposit.totalHeld(), 0);
        assertEq(usdc.balanceOf(address(deposit)), 0);

        // The two keys still hold their seats and their pauses. The contracts are empty and are left behind.
        assertEq(pool.pauser(), pauser);
        assertEq(pool.signers(2), vm.addr(SIGNER3_KEY));
        assertEq(deposit.pauser(), pauser);
        assertEq(deposit.signers(2), vm.addr(SIGNER3_KEY));
        assertTrue(pool.paysPaused());
        assertTrue(deposit.depositsPaused());
    }

    // ---- two signers together, with and without the pauser ----

    /// Two signers in the wrong hands start the exit as well, and the honest pauser and third signer cannot
    /// stop it. What that costs is the service, not the money: seven days later the pool's won is in the
    /// treasury. The two signers can also record a payout that never happened and answer the traveller's
    /// contest themselves; the exit then sends that traveller's USDC to the treasury, which owes it back.
    function testTwoLeakedSignersForceTheExitButOnlyToTheTreasury() public {
        bytes32 unpaid = depositIt(quoteFor(50e6, board));
        uint256 start = block.timestamp;
        toWon();
        proposeExitOn(pool); // signed by signers 1 and 2: the leaked ones
        toSource();
        proposeExitOn(deposit);

        uint256 until = start + 1 days;
        bytes[2] memory stop = exitSignatures(deposit, "cancelEmergencyExit", PAUSER_KEY, SIGNER3_KEY, until);
        vm.prank(pauser);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.cancelEmergencyExit(until, stop);
        vm.expectRevert(ChargeGovernance.SignersNotDistinct.selector);
        deposit.cancelEmergencyExit(until, [stop[1], stop[1]]);
        vm.startPrank(pauser);
        deposit.pauseDeposits();
        deposit.disableKey(ChargeGovernance.Key.Relay);
        vm.stopPrank();
        assertEq(exitStatus(deposit), uint8(ChargeGovernance.Status.Waiting));

        deposit.completeBySigners(unpaid, until, two(deposit, "complete", unpaid, 0, until));
        contestIt(unpaid);
        deposit.confirmPaid(unpaid, until, two(deposit, "confirmPaid", unpaid, 2, until));

        vm.warp(start + 7 days);
        deposit.emergencyExit(listOf(unpaid));
        toWon();
        pool.emergencyExit();
        assertEq(assetBalance(treasury), POOL_SIZE * scale());
        assertEq(usdc.balanceOf(treasury), 50e6);
        assertEq(usdc.balanceOf(payer), 1_000_000e6 - 50e6);
        assertEq(usdc.balanceOf(vm.addr(SIGNER1_KEY)) + usdc.balanceOf(vm.addr(SIGNER2_KEY)), 0);
        assertEq(assetBalance(board), 0);
    }

    function theirLimits() internal pure returns (WonPool.Limits memory) {
        return WonPool.Limits(POOL_SIZE, POOL_SIZE, POOL_SIZE, 0, 0);
    }

    /// The changes that let the attackers pay themselves: their own relay, quote key and confirmation key,
    /// and limits as large as the pool.
    function proposeTheirOwn(bytes32 salt) internal returns (bytes32[4] memory actions) {
        for (uint256 k; k < 3; ++k) {
            actions[k] = proposeOn(pool, "setKey", abi.encode(k, vm.addr(theirKeys[k])), salt);
        }
        actions[3] = proposeOn(pool, "raiseLimits", abi.encode(theirLimits()), salt);
    }

    /// Two signers and the pauser in the wrong hands. The honest third signer cancels every change but the
    /// one that takes its own seat. A day later that seat is gone, a day after that every other change is
    /// applied, and the attackers pay the whole pool to themselves with keys and limits of their own. The
    /// exit does not help here: one honest signer cannot propose it.
    function testTwoLeakedSignersWithThePauserTakeThePoolInTwoDays() public {
        toWon();
        uint256 start = block.timestamp;
        bytes32 takeThird = proposeSigner(pool, 2);
        bytes32[4] memory rest = proposeTheirOwn("salt");
        cancelBy(pool, SIGNER3_KEY, takeThird, ChargeGovernance.Unauthorized.selector);
        for (uint256 i; i < rest.length; ++i) {
            cancelBy(pool, SIGNER3_KEY, rest[i], 0);
        }
        uint256 until = start + 1 days;
        bytes[2] memory alone = exitSignatures(pool, "proposeEmergencyExit", SIGNER3_KEY, SIGNER3_KEY, until);
        vm.expectRevert(ChargeGovernance.SignersNotDistinct.selector);
        pool.proposeEmergencyExit(until, alone);

        vm.warp(start + 24 hours);
        pool.setSigner(2, next, "salt");
        rest = proposeTheirOwn("salt 2");
        for (uint256 i; i < rest.length; ++i) {
            cancelBy(pool, SIGNER3_KEY, rest[i], ChargeGovernance.Unauthorized.selector); // no signer any more
        }

        vm.warp(start + 48 hours);
        for (uint256 k; k < 3; ++k) {
            pool.setKey(ChargeGovernance.Key(k), vm.addr(theirKeys[k]), "salt 2");
        }
        pool.raiseLimits(theirLimits(), "salt 2");
        ChargeTypes.Quote memory quote = quoteForWon(POOL_SIZE, stranger);
        bytes32 id = idOf(quote);
        toWon();
        WonPool.Confirmation memory confirmation =
            WonPool.Confirmation({fast: false, sourceBlockHash: SOURCE_BLOCK, until: block.timestamp + 1 hours});
        bytes memory confirmed =
            sign(theirKeys[2], pool.confirmationDigest(id, SOURCE_BLOCK, false, confirmation.until));
        bytes memory quoted = sign(theirKeys[1], id);
        vm.prank(vm.addr(theirKeys[0]));
        pool.pay(origin(), quote, quoted, confirmation, confirmed);
        assertEq(assetBalance(stranger), POOL_SIZE * scale());
        assertEq(assetBalance(address(pool)), 0);
        assertEq(assetBalance(treasury), 0);
    }

    // ---- many replacements of the pauser at once ----

    /// Two signers in the wrong hands flood the pauser's seat with twenty replacements. The honest pauser can
    /// stop none of them. One signature of the third signer stops them all; nothing else is touched.
    function testOneSignatureStopsTwentyPauserReplacements() public {
        toWon();
        for (uint256 i; i < 20; ++i) {
            proposeOn(pool, "setKey", abi.encode(ChargeGovernance.Key.Pauser, vm.addr(0xC000 + i)), bytes32(i));
        }
        bytes32 seventh =
            pool.actionOf(SET_KEY, abi.encode(ChargeGovernance.Key.Pauser, vm.addr(0xC007)), bytes32(uint256(7)));
        proposeOn(pool, "withdraw", abi.encode(uint256(1)), "salt");
        cancelByPauser(pool, seventh, ChargeGovernance.Unauthorized.selector);

        // Only a signer: not the pauser, by transaction or by signature, and no other key.
        uint256 until = block.timestamp + 6 days;
        bytes32 digest = pool.actionDigest(keccak256("voidPauserReplacements"), 0, 0, until);
        vm.prank(pauser);
        vm.expectRevert(ChargeTypes.InvalidSignature.selector);
        pool.voidPauserReplacements(until, "");
        uint256[4] memory others = [PAUSER_KEY, RELAY_KEY, QUOTER_KEY, WITNESS_KEY];
        for (uint256 i; i < others.length; ++i) {
            vm.prank(pauser);
            vm.expectRevert(ChargeGovernance.Unauthorized.selector);
            pool.voidPauserReplacements(until, sign(others[i], digest));
        }
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        pool.voidPauserReplacements(block.timestamp, sign(SIGNER3_KEY, digest));
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        pool.voidPauserReplacements(block.timestamp + 7 days + 1, sign(SIGNER3_KEY, digest));

        // Each of the three signers can, as each can cancel one such change.
        for (uint256 i; i < 3; ++i) {
            uint256 before = vm.snapshotState();
            vm.expectEmit(address(pool));
            emit ChargeGovernance.PauserReplacementsVoided(0, ChargeGovernance.Seat(i + 2));
            vm.prank(stranger);
            pool.voidPauserReplacements(until, sign(signerKeys[i], digest));
            assertEq(pool.pauserRound(), 1);
            assertEq(pool.generation(), 0);
            vm.revertToState(before);
        }
        pool.voidPauserReplacements(until, sign(SIGNER3_KEY, digest));

        vm.warp(block.timestamp + 24 hours);
        for (uint256 i; i < 20; ++i) {
            vm.expectRevert(ChargeGovernance.InvalidState.selector);
            pool.setKey(ChargeGovernance.Key.Pauser, vm.addr(0xC000 + i), bytes32(i));
        }
        assertEq(pool.pauser(), pauser);
        pool.withdraw(1, "salt"); // a change for no seat is not touched
        assertEq(assetBalance(treasury), 1);

        // The next flood is counted in the next round and needs a second signature; the first is used up.
        proposeOn(pool, "setKey", abi.encode(ChargeGovernance.Key.Pauser, next), "again");
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        pool.voidPauserReplacements(until, sign(SIGNER3_KEY, digest));
        until = block.timestamp + 1 days;
        pool.voidPauserReplacements(
            until, sign(SIGNER3_KEY, pool.actionDigest(keccak256("voidPauserReplacements"), 0, 1, until))
        );
        vm.warp(block.timestamp + 24 hours);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        pool.setKey(ChargeGovernance.Key.Pauser, next, "again");
    }

    /// Without that signature the third signer has to cancel twenty changes one by one, and one miss is
    /// enough. A day later the two leaked signers hold the pauser, the only key that could stop the
    /// replacement of the third signer. A day after that they hold all three seats, and on the third day a
    /// spender of theirs takes the completed USDC.
    function testOneMissedCancelOutOfTwentyEndsInATakeoverInThreeDays() public {
        bytes32 id = depositIt(quoteFor(100e6, board));
        completeIt(id);
        vm.warp(block.timestamp + 7 days); // past its contest window
        uint256 start = block.timestamp;
        bytes32[] memory flood = new bytes32[](20);
        for (uint256 i; i < 20; ++i) {
            flood[i] =
                proposeOn(deposit, "setKey", abi.encode(ChargeGovernance.Key.Pauser, vm.addr(0xC000 + i)), bytes32(i));
        }
        bytes32 takeThird = proposeSigner(deposit, 2);
        cancelByPauser(deposit, takeThird, 0); // the honest pauser still stops this one
        for (uint256 i; i < 20; ++i) {
            if (i != 13) cancelBy(deposit, SIGNER3_KEY, flood[i], 0);
        }

        vm.warp(start + 24 hours);
        deposit.setKey(ChargeGovernance.Key.Pauser, vm.addr(0xC00D), bytes32(uint256(13)));
        assertEq(deposit.pauser(), vm.addr(0xC00D));
        takeThird = proposeOn(deposit, "setSigner", abi.encode(uint8(2), next), "salt 2");
        cancelByPauser(deposit, takeThird, ChargeGovernance.InvalidTime.selector); // the old pauser is nobody
        cancelBy(deposit, SIGNER3_KEY, takeThird, ChargeGovernance.Unauthorized.selector);

        vm.warp(start + 48 hours);
        deposit.setSigner(2, next, "salt 2");
        Spender theirs = new Spender(deposit);
        bytes32 spenderChange = proposeOn(deposit, "setSpender", spenderArgs(address(theirs)), "theirs");
        cancelBy(deposit, SIGNER3_KEY, spenderChange, ChargeGovernance.Unauthorized.selector); // nobody is left

        vm.warp(start + 72 hours);
        deposit.setSpender(address(theirs), address(theirs).codehash, "theirs");
        theirs.spend(id, 100e6, stranger);
        assertEq(usdc.balanceOf(stranger), 100e6);
        assertEq(usdc.balanceOf(treasury), 0);
    }

    // ---- what needs no key ----

    /// With every hot key off, deposits stopped, the spender cleared and the limit at its lowest, a deposit
    /// still goes back to its wallet after seven days, sent by anyone.
    function testTheSelfRefundNeedsNoKey() public {
        bytes32 id = depositIt(quoteFor(100e6, board));
        vm.startPrank(pauser);
        deposit.pauseDeposits();
        deposit.disableKey(ChargeGovernance.Key.Relay);
        deposit.disableKey(ChargeGovernance.Key.Quoter);
        deposit.disableKey(ChargeGovernance.Key.Witness);
        deposit.clearSpender();
        deposit.lowerLimit(MIN_WON);
        vm.stopPrank();
        vm.warp(block.timestamp + 7 days);
        vm.prank(stranger);
        deposit.refundSelf(id);
        assertEq(usdc.balanceOf(payer), 1_000_000e6);
        assertEq(deposit.totalHeld(), 0);
    }
}
