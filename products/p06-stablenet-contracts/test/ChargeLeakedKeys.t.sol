// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {ChargeBase} from "./ChargeBase.t.sol";
import {ChargeTypes} from "../src/charge/ChargeTypes.sol";
import {ChargeGovernance} from "../src/charge/ChargeGovernance.sol";

/// @notice Keys in the wrong hands against the changes that wait a day. One key alone, the pauser or a signer,
/// can delay but cannot stay: the key whose place a change takes cannot stop that change. No key alone forces
/// a change through. Two keys together are named with what they can and cannot do.
contract ChargeLeakedKeysTest is ChargeBase {
    bytes32 internal constant SET_KEY = keccak256("setKey");
    uint256 internal constant NEW_KEY = 0xA008;
    uint256[3] internal signerKeys = [SIGNER1_KEY, SIGNER2_KEY, SIGNER3_KEY];
    address internal pauser = vm.addr(PAUSER_KEY);
    address internal next = vm.addr(NEW_KEY);

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
        bytes32 spenderChange = proposeOn(deposit, "setSpender", abi.encode(address(this)), "salt");
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
        proposeOn(deposit, "setSpender", abi.encode(address(this)), "salt 2");
        vm.prank(pauser);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.clearSpender();
        vm.warp(block.timestamp + 24 hours);
        deposit.setSpender(address(this), "salt 2");
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

    /// Known limit, kept by decision: the pauser and one signer in the wrong hands together stop each other's
    /// replacement, so no change goes through and the pool's money stays where it is. A traveller's deposit
    /// is not caught in this: the two honest signers refund it, and after seven days anyone does.
    function testKnownLimitALeakedPauserWithALeakedSignerBlocksEveryChange() public {
        ChargeTypes.Quote memory quote = quoteFor(100e6, board);
        bytes32 id = depositIt(quote);
        bytes32 other = depositIt(quoteFor(50e6, board));
        toWon();
        uint256 amount = POOL_SIZE * scale();
        bytes32 takePauser = proposePauser(pool);
        bytes32 takeThird = proposeSigner(pool, 2);
        bytes32 withdrawal = proposeOn(pool, "withdraw", abi.encode(amount), "salt");
        cancelBy(pool, SIGNER3_KEY, takePauser, 0);
        cancelByPauser(pool, takeThird, 0);
        cancelByPauser(pool, withdrawal, 0);
        vm.prank(pauser);
        pool.pausePays();
        vm.warp(block.timestamp + 24 hours);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        pool.setKey(ChargeGovernance.Key.Pauser, next, "salt");
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        pool.setSigner(2, next, "salt");
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        pool.withdraw(amount, "salt");
        assertEq(assetBalance(address(pool)), amount);

        refundBySigners(id); // signers 1 and 2
        vm.warp(block.timestamp + 7 days);
        vm.prank(stranger);
        deposit.refundSelf(other);
        assertEq(usdc.balanceOf(payer), 1_000_000e6);
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

    /// Without that signature the third signer has to cancel twenty changes one by one, and the one it
    /// misses goes through a day later: the two leaked signers then hold the pauser as well.
    function testOneMissedCancelOutOfTwentyHandsOverThePauser() public {
        toWon();
        bytes32[] memory flood = new bytes32[](20);
        for (uint256 i; i < 20; ++i) {
            flood[i] =
                proposeOn(pool, "setKey", abi.encode(ChargeGovernance.Key.Pauser, vm.addr(0xC000 + i)), bytes32(i));
        }
        for (uint256 i; i < 20; ++i) {
            if (i != 13) cancelBy(pool, SIGNER3_KEY, flood[i], 0);
        }
        vm.warp(block.timestamp + 24 hours);
        pool.setKey(ChargeGovernance.Key.Pauser, vm.addr(0xC00D), bytes32(uint256(13)));
        assertEq(pool.pauser(), vm.addr(0xC00D));
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
