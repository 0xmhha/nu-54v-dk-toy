// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {ChargeBase} from "./ChargeBase.t.sol";
import {ChargeTypes} from "../src/charge/ChargeTypes.sol";
import {ChargeGovernance} from "../src/charge/ChargeGovernance.sol";
import {ChargeDeposit} from "../src/charge/ChargeDeposit.sol";
import {WonPool} from "../src/charge/WonPool.sol";

/// @notice The keys of both top-up contracts: one key narrows or stops, two signers and a day widen.
contract ChargeGovernanceTest is ChargeBase {
    event ChangeProposed(
        bytes32 indexed action,
        bytes32 indexed kind,
        bytes arguments,
        bytes32 salt,
        uint256 round,
        uint64 readyAt,
        uint64 lapsesAt
    );

    bytes32 internal constant SET_KEY = keccak256("setKey");
    bytes32 internal constant SET_SIGNER = keccak256("setSigner");
    address internal next = address(0xE0A);

    function keyArgs(ChargeGovernance.Key which, address to) internal pure returns (bytes memory) {
        return abi.encode(which, to);
    }

    function signerArgs(uint8 index, address to) internal pure returns (bytes memory) {
        return abi.encode(index, to);
    }

    /// The number of a change is the hash of its kind, its arguments and its salt, in one ABI encoding.
    function keyAction(ChargeGovernance.Key which, address to, bytes32 salt) internal pure returns (bytes32) {
        return keccak256(abi.encode(SET_KEY, which, to, salt));
    }

    function signerAction(uint8 index, address to, bytes32 salt) internal pure returns (bytes32) {
        return keccak256(abi.encode(SET_SIGNER, index, to, salt));
    }

    function proposeKey(ChargeGovernance.Key which, address to, bytes32 salt, uint256 until, bytes[2] memory signatures)
        internal
    {
        deposit.propose(SET_KEY, keyArgs(which, to), salt, until, signatures);
    }

    /// The relay and the deposit-confirmation key are never the same address, at deployment or later.
    function testEveryKeyIsADifferentAddress() public {
        ChargeGovernance.Keys memory same = keys();
        same.witness = same.relay;
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        new ChargeDeposit(same, depositConfig(address(pool)));
        same = keys();
        same.quoter = same.witness;
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        new ChargeDeposit(same, depositConfig(address(pool)));
        same = keys();
        same.signers[2] = same.signers[0];
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        new ChargeDeposit(same, depositConfig(address(pool)));
        same = keys();
        same.signers[1] = same.pauser;
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        new ChargeDeposit(same, depositConfig(address(pool)));
        same = keys();
        same.pauser = address(0);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        new ChargeDeposit(same, depositConfig(address(pool)));

        toWon();
        same = keys();
        same.witness = same.relay;
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        new WonPool(same, poolConfig(address(deposit)));
    }

    function testAChangeNeedsTwoDifferentSigners() public {
        bytes32 action = keyAction(ChargeGovernance.Key.Relay, next, "salt");
        assertEq(deposit.actionOf(SET_KEY, keyArgs(ChargeGovernance.Key.Relay, next), "salt"), action);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.setKey(ChargeGovernance.Key.Relay, next, "salt"); // nobody proposed it

        uint256 until = block.timestamp + 1 days;
        bytes32 digest = deposit.actionDigest(keccak256("propose"), action, 0, until);
        bytes memory one = sign(SIGNER1_KEY, digest);
        vm.expectRevert(ChargeGovernance.SignersNotDistinct.selector);
        proposeKey(ChargeGovernance.Key.Relay, next, "salt", until, [one, one]);
        uint256[4] memory others = [RELAY_KEY, QUOTER_KEY, WITNESS_KEY, PAUSER_KEY];
        for (uint256 i; i < others.length; ++i) {
            vm.expectRevert(ChargeGovernance.Unauthorized.selector);
            proposeKey(ChargeGovernance.Key.Relay, next, "salt", until, [one, sign(others[i], digest)]);
        }
        // The signatures are for one change: not for another key, another address or another salt.
        bytes[2] memory signatures = [sign(SIGNER3_KEY, digest), one];
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        proposeKey(ChargeGovernance.Key.Quoter, next, "salt", until, signatures);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        proposeKey(ChargeGovernance.Key.Relay, stranger, "salt", until, signatures);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        proposeKey(ChargeGovernance.Key.Relay, next, "salt 2", until, signatures);
        proposeKey(ChargeGovernance.Key.Relay, next, "salt", until, signatures);
    }

    /// What waits for a day is readable: the event names the kind of change and carries its arguments.
    function testAProposalSaysWhatItChanges() public {
        bytes memory arguments = keyArgs(ChargeGovernance.Key.Relay, next);
        bytes32 action = keyAction(ChargeGovernance.Key.Relay, next, "salt");
        uint256 until = block.timestamp + 1 days;
        bytes[2] memory signatures = two(deposit, "propose", action, 0, until);
        vm.expectEmit(address(deposit));
        emit ChangeProposed(
            action,
            SET_KEY,
            arguments,
            "salt",
            0,
            uint64(block.timestamp + 24 hours),
            uint64(block.timestamp + 24 hours + 3 days)
        );
        assertEq(deposit.propose(SET_KEY, arguments, "salt", until, signatures), action);
        (uint64 proposedAt, ChargeGovernance.ProposalState state_, ChargeGovernance.Seat replaces, uint256 round) =
            deposit.proposals(action);
        assertEq(proposedAt, block.timestamp);
        assertEq(uint8(state_), uint8(ChargeGovernance.ProposalState.Pending));
        assertEq(uint8(replaces), uint8(ChargeGovernance.Seat.None));
        assertEq(round, 0);

        // A change that takes the place of a key says whose.
        bytes32 pauserAction = proposeOn(deposit, "setKey", keyArgs(ChargeGovernance.Key.Pauser, next), "salt");
        (,, replaces,) = deposit.proposals(pauserAction);
        assertEq(uint8(replaces), uint8(ChargeGovernance.Seat.Pauser));
        for (uint8 i; i < 3; ++i) {
            bytes32 signerAction_ = proposeOn(deposit, "setSigner", signerArgs(i, next), "salt");
            assertEq(signerAction_, signerAction(i, next, "salt"));
            (,, replaces,) = deposit.proposals(signerAction_);
            assertEq(uint8(replaces), uint8(ChargeGovernance.Seat.Signer0) + i);
        }
    }

    /// An address in the arguments has nothing above its 20 bytes. The changing function encodes a clean
    /// address, so a proposal with anything there would get a number that no call ever reaches: it would
    /// wait and never be applied.
    function testArgumentsWithAnUncleanAddressAreNotProposed() public {
        uint256 until = block.timestamp + 1 days;
        bytes[2] memory none;
        uint256 unclean = uint256(uint160(next)) | (1 << 160);
        uint256 topBit = uint256(uint160(next)) | (1 << 255);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.propose(SET_KEY, abi.encode(ChargeGovernance.Key.Relay, unclean), "salt", until, none);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.propose(SET_KEY, abi.encode(ChargeGovernance.Key.Pauser, topBit), "salt", until, none);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.propose(SET_SIGNER, abi.encode(uint8(1), unclean), "salt", until, none);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.proposalRound(SET_SIGNER, abi.encode(uint8(1), topBit));
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.propose(keccak256("setSpender"), abi.encode(unclean), "salt", until, none);
        toWon();
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        pool.propose(keccak256("addOrigin"), abi.encode(uint256(10), unclean), "salt", until, none);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        pool.propose(keccak256("returnToken"), abi.encode(topBit, uint256(1)), "salt", until, none);
        // A number is no address: every bit of it counts.
        proposeOn(pool, "addOrigin", abi.encode(type(uint256).max, next), "salt");
        proposeOn(pool, "returnToken", abi.encode(next, type(uint256).max), "salt");
        proposeOn(pool, "withdraw", abi.encode(type(uint256).max), "salt");
        toSource();
        proposeOn(deposit, "setRates", abi.encode(type(uint256).max, type(uint256).max), "salt");
        // The largest clean address is taken.
        proposeOn(deposit, "setKey", abi.encode(ChargeGovernance.Key.Relay, type(uint160).max), "salt");
    }

    /// What is let in names one call that exists. Whether the call then succeeds is checked when it is
    /// applied: a change with a value the function refuses waits its day and fails then.
    function testAChangeWithARefusedValueWaitsAndFailsWhenApplied() public {
        bytes32 zeroRate = proposeOn(deposit, "setRates", abi.encode(uint256(0), RATE), "salt");
        bytes32 keyInUse =
            proposeOn(deposit, "setKey", keyArgs(ChargeGovernance.Key.Relay, vm.addr(QUOTER_KEY)), "salt");
        assertEq(uint8(deposit.proposalStatus(zeroRate)), uint8(ChargeGovernance.Status.Waiting));
        vm.warp(block.timestamp + 24 hours);
        assertEq(uint8(deposit.proposalStatus(keyInUse)), uint8(ChargeGovernance.Status.Ready));
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.setRates(0, RATE, "salt");
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.setKey(ChargeGovernance.Key.Relay, vm.addr(QUOTER_KEY), "salt");
        assertEq(uint8(deposit.proposalStatus(zeroRate)), uint8(ChargeGovernance.Status.Ready));
    }

    /// The end of a signature binds only the moment it is used. A change proposed one second before the end
    /// of its signatures still waits its day and is applied days after that end.
    function testTheEndOfASignatureBindsOnlyTheMomentOfUse() public {
        bytes32 action = keyAction(ChargeGovernance.Key.Relay, next, "salt");
        uint256 until = block.timestamp + 30 days;
        bytes[2] memory signatures = two(deposit, "propose", action, 0, until);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        proposeKey(ChargeGovernance.Key.Relay, next, "salt", until, signatures); // more than 7 days ahead
        // The same signatures are taken once their end is within 7 days, however long ago they were made.
        vm.warp(until - 1);
        proposeKey(ChargeGovernance.Key.Relay, next, "salt", until, signatures);
        vm.warp(until + 24 hours + 3 days - 1);
        deposit.setKey(ChargeGovernance.Key.Relay, next, "salt");
        assertEq(deposit.relay(), next);
    }

    /// A proposal must be a change this contract has, with arguments of the right size that name a key or
    /// a signer that exists.
    function testOnlyAChangeTheContractCanApplyIsProposed() public {
        uint256 until = block.timestamp + 1 days;
        bytes[2] memory none;
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.propose(keccak256("mint"), abi.encode(next), "salt", until, none);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.propose(keccak256("mint"), "", "salt", until, none); // unknown, with or without arguments
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.propose(SET_KEY, abi.encode(ChargeGovernance.Key.Relay), "salt", until, none);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.propose(SET_KEY, abi.encode(ChargeGovernance.Key.Relay, next, uint256(0)), "salt", until, none);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.propose(SET_KEY, abi.encode(uint256(4), next), "salt", until, none); // there are four keys
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.propose(SET_SIGNER, signerArgs(3, next), "salt", until, none); // there are three signers
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.proposalRound(SET_SIGNER, signerArgs(3, next));
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.propose(keccak256("setRates"), abi.encode(RATE), "salt", until, none);

        // Each contract takes its own kinds and not those of the other.
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.propose(keccak256("withdraw"), abi.encode(uint256(1)), "salt", until, none);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.propose(keccak256("setSpender"), abi.encode(next, next), "salt", until, none);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.propose(keccak256("raiseLimit"), "", "salt", until, none);
        proposeOn(deposit, "setSpender", abi.encode(next), "salt");
        proposeOn(deposit, "raiseLimit", abi.encode(MAX_WON), "salt");
        proposeOn(deposit, "setRates", abi.encode(RATE, RATE), "salt");
        toWon();
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        pool.propose(keccak256("setSpender"), abi.encode(next), "salt", until, none);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        pool.propose(keccak256("raiseLimits"), abi.encode(MAX_WON), "salt", until, none);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        pool.propose(keccak256("withdraw"), abi.encode(uint256(1), uint256(2)), "salt", until, none);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        pool.propose(keccak256("addOrigin"), abi.encode(uint256(1)), "salt", until, none);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        pool.propose(keccak256("returnToken"), abi.encode(next), "salt", until, none);
        proposeOn(pool, "withdraw", abi.encode(uint256(1)), "salt");
        proposeOn(pool, "raiseLimits", abi.encode(limits()), "salt");
        proposeOn(pool, "addOrigin", abi.encode(uint256(10), next), "salt");
        proposeOn(pool, "returnToken", abi.encode(next, uint256(1)), "salt");
    }

    function testAChangeWaitsADayAndLapsesThreeDaysLater() public {
        proposeOn(deposit, "setKey", keyArgs(ChargeGovernance.Key.Relay, next), "salt");
        uint256 proposedAt = block.timestamp;
        vm.warp(proposedAt + 24 hours - 1);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.setKey(ChargeGovernance.Key.Relay, next, "salt");
        vm.warp(proposedAt + 24 hours + 3 days + 1);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.setKey(ChargeGovernance.Key.Relay, next, "salt");
        vm.warp(proposedAt + 24 hours + 3 days);
        vm.prank(stranger); // anyone may apply what two signers proposed
        deposit.setKey(ChargeGovernance.Key.Relay, next, "salt");
        assertEq(deposit.relay(), next);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.setKey(ChargeGovernance.Key.Relay, next, "salt");
    }

    function testACancelledOrLapsedChangeIsNotRevivedByTheSameSignatures() public {
        bytes32 action = keyAction(ChargeGovernance.Key.Relay, next, "salt");
        uint256 until = block.timestamp + 6 days;
        bytes[2] memory signatures = two(deposit, "propose", action, 0, until);
        proposeKey(ChargeGovernance.Key.Relay, next, "salt", until, signatures);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        proposeKey(ChargeGovernance.Key.Relay, next, "salt", until, signatures); // already waiting

        vm.prank(stranger);
        vm.expectRevert(ChargeTypes.InvalidSignature.selector);
        deposit.cancelProposal(action, until, "");
        vm.prank(vm.addr(PAUSER_KEY));
        deposit.cancelProposal(action, 0, "");
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        proposeKey(ChargeGovernance.Key.Relay, next, "salt", until, signatures);
        vm.prank(vm.addr(PAUSER_KEY));
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.cancelProposal(action, 0, ""); // stopped once
        vm.warp(block.timestamp + 24 hours);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.setKey(ChargeGovernance.Key.Relay, next, "salt");

        // A change that lapsed needs a new salt and new signatures too.
        bytes32 second = keyAction(ChargeGovernance.Key.Relay, next, "salt 2");
        bytes[2] memory secondSignatures = two(deposit, "propose", second, 0, until);
        proposeKey(ChargeGovernance.Key.Relay, next, "salt 2", until, secondSignatures);
        vm.warp(block.timestamp + 4 days + 1);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        proposeKey(ChargeGovernance.Key.Relay, next, "salt 2", until, secondSignatures);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.setKey(ChargeGovernance.Key.Relay, next, "salt 2");
    }

    function testOneSignerCancelsBySignature() public {
        bytes32 action = proposeOn(deposit, "setKey", keyArgs(ChargeGovernance.Key.Relay, next), "salt");
        uint256 until = block.timestamp + 1 days;
        bytes32 digest = deposit.actionDigest(keccak256("cancel"), action, 0, until);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.cancelProposal(action, until, sign(RELAY_KEY, digest));
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.cancelProposal(action, block.timestamp, sign(SIGNER3_KEY, digest));
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.cancelProposal(action, block.timestamp + 7 days + 1, sign(SIGNER3_KEY, digest));
        // The pauser's own signature is no signer's: with a signature the call is judged by the signature.
        vm.prank(vm.addr(PAUSER_KEY));
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.cancelProposal(action, until, sign(PAUSER_KEY, digest));
        vm.prank(stranger);
        deposit.cancelProposal(action, until, sign(SIGNER3_KEY, digest));
        vm.warp(block.timestamp + 24 hours);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.setKey(ChargeGovernance.Key.Relay, next, "salt");
        assertEq(deposit.relay(), vm.addr(RELAY_KEY));
    }

    function testSwitchingAKeyOffVoidsEveryEarlierChange() public {
        bytes32 action = keyAction(ChargeGovernance.Key.Quoter, next, "salt");
        uint256 until = block.timestamp + 6 days;
        bytes[2] memory signatures = two(deposit, "propose", action, 0, until);
        proposeKey(ChargeGovernance.Key.Quoter, next, "salt", until, signatures);
        vm.prank(stranger);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.disableKey(ChargeGovernance.Key.Relay);
        vm.startPrank(vm.addr(PAUSER_KEY));
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.disableKey(ChargeGovernance.Key.Pauser); // the pauser cannot remove the last brake
        deposit.disableKey(ChargeGovernance.Key.Relay);
        vm.stopPrank();
        assertEq(deposit.relay(), address(0));
        assertEq(deposit.generation(), 1);
        assertEq(deposit.pauserRound(), 0);

        vm.warp(block.timestamp + 24 hours);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.setKey(ChargeGovernance.Key.Quoter, next, "salt");
        bytes memory cancel = sign(SIGNER1_KEY, deposit.actionDigest(keccak256("cancel"), action, 0, until));
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.cancelProposal(action, until, cancel); // nothing waits any more
        // The same change can be proposed again, but only with signatures made after the switch.
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        proposeKey(ChargeGovernance.Key.Quoter, next, "salt", until, signatures);
        proposeOn(deposit, "setKey", keyArgs(ChargeGovernance.Key.Quoter, next), "salt");
        vm.warp(block.timestamp + 24 hours);
        deposit.setKey(ChargeGovernance.Key.Quoter, next, "salt");
        assertEq(deposit.quoter(), next);
    }

    /// Switching a key off voids every waiting change, so it works once per key: a key that is off cannot be
    /// switched off again until two signers brought it back.
    function testAKeyThatIsOffIsNotSwitchedOffAgain() public {
        ChargeGovernance.Key[3] memory hot =
            [ChargeGovernance.Key.Relay, ChargeGovernance.Key.Quoter, ChargeGovernance.Key.Witness];
        vm.startPrank(vm.addr(PAUSER_KEY));
        for (uint256 i; i < hot.length; ++i) {
            deposit.disableKey(hot[i]);
            assertEq(deposit.generation(), i + 1);
            vm.expectRevert(ChargeGovernance.InvalidState.selector);
            deposit.disableKey(hot[i]);
        }
        vm.stopPrank();
        assertEq(deposit.generation(), 3);

        bytes32 action = proposeOn(deposit, "setSpender", abi.encode(address(pool)), "salt");
        for (uint256 i; i < hot.length; ++i) {
            vm.prank(vm.addr(PAUSER_KEY));
            vm.expectRevert(ChargeGovernance.InvalidState.selector);
            deposit.disableKey(hot[i]);
        }
        (, ChargeGovernance.ProposalState state_,, uint256 round) = deposit.proposals(action);
        assertEq(uint8(state_), uint8(ChargeGovernance.ProposalState.Pending));
        assertEq(round, deposit.generation());

        // Once two signers brought the relay back, it can be switched off again.
        proposeOn(deposit, "setKey", keyArgs(ChargeGovernance.Key.Relay, next), "salt");
        vm.warp(block.timestamp + 24 hours);
        deposit.setKey(ChargeGovernance.Key.Relay, next, "salt");
        vm.prank(vm.addr(PAUSER_KEY));
        deposit.disableKey(ChargeGovernance.Key.Relay);
        assertEq(deposit.generation(), 4);
    }

    function testAKeyThatIsOffComesBackOnlyByTwoSignersAndStaysDistinct() public {
        toWon();
        vm.prank(vm.addr(PAUSER_KEY));
        pool.disableKey(ChargeGovernance.Key.Witness);
        address relayKey = vm.addr(RELAY_KEY);
        // The confirmation key can never become the relay's own key, nor any other key in use.
        proposeOn(pool, "setKey", keyArgs(ChargeGovernance.Key.Witness, relayKey), "salt");
        proposeOn(pool, "setKey", keyArgs(ChargeGovernance.Key.Witness, vm.addr(SIGNER2_KEY)), "salt");
        proposeOn(pool, "setKey", keyArgs(ChargeGovernance.Key.Witness, address(0)), "salt");
        proposeOn(pool, "setKey", keyArgs(ChargeGovernance.Key.Witness, next), "salt");
        proposeOn(pool, "setKey", keyArgs(ChargeGovernance.Key.Relay, next), "salt");
        vm.warp(block.timestamp + 24 hours);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        pool.setKey(ChargeGovernance.Key.Witness, relayKey, "salt");
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        pool.setKey(ChargeGovernance.Key.Witness, vm.addr(SIGNER2_KEY), "salt");
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        pool.setKey(ChargeGovernance.Key.Witness, address(0), "salt");
        pool.setKey(ChargeGovernance.Key.Witness, next, "salt");
        assertEq(pool.witness(), next);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        pool.setKey(ChargeGovernance.Key.Relay, next, "salt"); // now the witness
    }

    function testThePauserKeyIsReplaced() public {
        proposeOn(deposit, "setKey", keyArgs(ChargeGovernance.Key.Pauser, next), "salt");
        vm.warp(block.timestamp + 24 hours);
        deposit.setKey(ChargeGovernance.Key.Pauser, next, "salt");
        vm.prank(vm.addr(PAUSER_KEY));
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.pauseDeposits();
        vm.prank(next);
        deposit.pauseDeposits();
        assertTrue(deposit.depositsPaused());
    }

    /// The new pauser cannot stop a change that takes its place. So when one replacement is applied, every
    /// other one that was waiting for that seat is void, and only those: a change of another kind stays.
    function testApplyingAPauserReplacementVoidsTheOthersForThatSeat() public {
        address other = address(0xE0B);
        proposeOn(deposit, "setKey", keyArgs(ChargeGovernance.Key.Pauser, next), "salt");
        bytes32 second = proposeOn(deposit, "setKey", keyArgs(ChargeGovernance.Key.Pauser, other), "salt");
        proposeOn(deposit, "setKey", keyArgs(ChargeGovernance.Key.Relay, other), "salt");
        vm.warp(block.timestamp + 24 hours);
        deposit.setKey(ChargeGovernance.Key.Pauser, next, "salt");
        assertEq(deposit.pauserRound(), 1);
        assertEq(deposit.generation(), 0);
        assertEq(deposit.proposalRound(SET_KEY, keyArgs(ChargeGovernance.Key.Pauser, other)), 1);

        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.setKey(ChargeGovernance.Key.Pauser, other, "salt");
        uint256 until = block.timestamp + 1 days;
        bytes memory cancel = sign(SIGNER3_KEY, deposit.actionDigest(keccak256("cancel"), second, 0, until));
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.cancelProposal(second, until, cancel); // nothing waits for that seat any more
        assertEq(deposit.pauser(), next);
        deposit.setKey(ChargeGovernance.Key.Relay, other, "salt");
        assertEq(deposit.relay(), other);

        // A replacement of another key leaves the round of the pauser's seat alone.
        assertEq(deposit.pauserRound(), 1);
    }

    function testAReplacedSignerNoLongerCounts() public {
        uint256 newKey = 0xA008;
        proposeOn(deposit, "setSigner", signerArgs(0, vm.addr(newKey)), "salt");
        proposeOn(deposit, "setSigner", signerArgs(1, vm.addr(SIGNER3_KEY)), "salt");
        vm.warp(block.timestamp + 24 hours);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.setSigner(3, next, "salt"); // there are three signers, so this was never proposed
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.setSigner(1, vm.addr(SIGNER3_KEY), "salt"); // one person cannot hold two places
        deposit.setSigner(0, vm.addr(newKey), "salt");
        assertEq(deposit.signers(0), vm.addr(newKey));
        assertEq(deposit.generation(), 1);
        assertEq(deposit.pauserRound(), 1);

        bytes32 action = keyAction(ChargeGovernance.Key.Relay, next, "salt");
        uint256 until = block.timestamp + 1 days;
        bytes32 digest = deposit.actionDigest(keccak256("propose"), action, 1, until);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        proposeKey(
            ChargeGovernance.Key.Relay, next, "salt", until, [sign(SIGNER1_KEY, digest), sign(SIGNER2_KEY, digest)]
        );
        proposeKey(ChargeGovernance.Key.Relay, next, "salt", until, [sign(newKey, digest), sign(SIGNER2_KEY, digest)]);
    }

    function testProposalSignaturesEndAndCannotBeLong() public {
        bytes32 action = keyAction(ChargeGovernance.Key.Relay, next, "salt");
        uint256 tooLong = block.timestamp + 7 days + 1;
        bytes[2] memory long = two(deposit, "propose", action, 0, tooLong);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        proposeKey(ChargeGovernance.Key.Relay, next, "salt", tooLong, long);
        uint256 until = block.timestamp + 1 days;
        bytes[2] memory signatures = two(deposit, "propose", action, 0, until);
        vm.warp(until);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        proposeKey(ChargeGovernance.Key.Relay, next, "salt", until, signatures);
    }

    // ---- what the server reads ----

    function statusOf(bytes32 action) internal view returns (ChargeGovernance.Status) {
        return deposit.proposalStatus(action);
    }

    function assertStatus(bytes32 action, ChargeGovernance.Status expected) internal view {
        assertEq(uint8(statusOf(action)), uint8(expected));
    }

    function assertStillPending(bytes32 action) internal view {
        (, ChargeGovernance.ProposalState stored,,) = deposit.proposals(action);
        assertEq(uint8(stored), uint8(ChargeGovernance.ProposalState.Pending));
    }

    /// The stored state says `Pending` for a change that lapsed or was voided. The status reads the clock
    /// and the rounds, so it says what applying would say.
    function testTheStatusSaysWhereAChangeStands() public {
        bytes32 action = keyAction(ChargeGovernance.Key.Relay, next, "salt");
        assertStatus(action, ChargeGovernance.Status.None);
        proposeOn(deposit, "setKey", keyArgs(ChargeGovernance.Key.Relay, next), "salt");
        uint256 proposedAt = block.timestamp;
        assertStatus(action, ChargeGovernance.Status.Waiting);
        vm.warp(proposedAt + 24 hours - 1);
        assertStatus(action, ChargeGovernance.Status.Waiting);
        vm.warp(proposedAt + 24 hours);
        assertStatus(action, ChargeGovernance.Status.Ready);
        vm.warp(proposedAt + 24 hours + 3 days);
        assertStatus(action, ChargeGovernance.Status.Ready);
        uint256 ready = vm.snapshotState();
        deposit.setKey(ChargeGovernance.Key.Relay, next, "salt");
        assertStatus(action, ChargeGovernance.Status.Applied);
        vm.revertToState(ready);
        vm.prank(vm.addr(PAUSER_KEY));
        deposit.cancelProposal(action, 0, "");
        assertStatus(action, ChargeGovernance.Status.Cancelled);
        vm.revertToState(ready);
        vm.warp(proposedAt + 24 hours + 3 days + 1);
        assertStatus(action, ChargeGovernance.Status.Void); // lapsed
        assertStillPending(action);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.setKey(ChargeGovernance.Key.Relay, next, "salt");
    }

    function testTheStatusOfAVoidedChangeIsVoidWhileItsStoredStateIsPending() public {
        bytes32 relayChange = proposeOn(deposit, "setKey", keyArgs(ChargeGovernance.Key.Relay, next), "salt");
        bytes32 pauserChange = proposeOn(deposit, "setKey", keyArgs(ChargeGovernance.Key.Pauser, next), "salt");
        vm.prank(vm.addr(PAUSER_KEY));
        deposit.disableKey(ChargeGovernance.Key.Quoter);
        assertStatus(relayChange, ChargeGovernance.Status.Void);
        assertStillPending(relayChange);
        assertStatus(pauserChange, ChargeGovernance.Status.Waiting); // the pauser does not void its replacement

        uint256 until = block.timestamp + 1 days;
        bytes32 digest = deposit.actionDigest(keccak256("voidPauserReplacements"), 0, 0, until);
        deposit.voidPauserReplacements(until, sign(SIGNER1_KEY, digest));
        assertStatus(pauserChange, ChargeGovernance.Status.Void);
        assertStillPending(pauserChange);
        vm.warp(block.timestamp + 24 hours);
        assertStatus(pauserChange, ChargeGovernance.Status.Void);
        assertStatus(relayChange, ChargeGovernance.Status.Void);
    }

    /// The round a change is counted in is what a cancel signs and what the server stores with the change:
    /// the round of the pauser's seat for a replacement of the pauser, the generation for every other.
    function testAProposalIsAnnouncedWithTheRoundItCountsIn() public {
        vm.startPrank(vm.addr(PAUSER_KEY));
        deposit.disableKey(ChargeGovernance.Key.Relay);
        deposit.disableKey(ChargeGovernance.Key.Quoter);
        vm.stopPrank();
        uint256 until = block.timestamp + 1 days;
        bytes32 digest = deposit.actionDigest(keccak256("voidPauserReplacements"), 0, 0, until);
        deposit.voidPauserReplacements(until, sign(SIGNER1_KEY, digest));
        assertEq(deposit.generation(), 2);
        assertEq(deposit.pauserRound(), 1);

        bytes memory arguments = keyArgs(ChargeGovernance.Key.Relay, next);
        bytes32 action = keyAction(ChargeGovernance.Key.Relay, next, "salt");
        uint64 readyAt = uint64(block.timestamp + 24 hours);
        bytes[2] memory signatures = two(deposit, "propose", action, 2, until);
        vm.expectEmit(address(deposit));
        emit ChangeProposed(action, SET_KEY, arguments, "salt", 2, readyAt, readyAt + 3 days);
        deposit.propose(SET_KEY, arguments, "salt", until, signatures);
        (,,, uint256 round) = deposit.proposals(action);
        assertEq(round, 2);

        arguments = keyArgs(ChargeGovernance.Key.Pauser, next);
        action = keyAction(ChargeGovernance.Key.Pauser, next, "salt");
        signatures = two(deposit, "propose", action, 1, until);
        vm.expectEmit(address(deposit));
        emit ChangeProposed(action, SET_KEY, arguments, "salt", 1, readyAt, readyAt + 3 days);
        deposit.propose(SET_KEY, arguments, "salt", until, signatures);
        (,,, round) = deposit.proposals(action);
        assertEq(round, 1);

        arguments = signerArgs(1, next);
        action = signerAction(1, next, "salt");
        signatures = two(deposit, "propose", action, 2, until);
        vm.expectEmit(address(deposit));
        emit ChangeProposed(action, SET_SIGNER, arguments, "salt", 2, readyAt, readyAt + 3 days);
        deposit.propose(SET_SIGNER, arguments, "salt", until, signatures);
    }

    function testACancelIsAnnouncedWithTheSeatThatCancelled() public {
        bytes32 action = proposeOn(deposit, "setKey", keyArgs(ChargeGovernance.Key.Relay, next), "salt");
        uint256 before = vm.snapshotState();
        vm.expectEmit(address(deposit));
        emit ChargeGovernance.ChangeCancelled(action, ChargeGovernance.Seat.Pauser);
        vm.prank(vm.addr(PAUSER_KEY));
        deposit.cancelProposal(action, 0, "");

        uint256[3] memory signerKeys = [SIGNER1_KEY, SIGNER2_KEY, SIGNER3_KEY];
        uint256 until = block.timestamp + 1 days;
        for (uint256 i; i < 3; ++i) {
            vm.revertToState(before);
            vm.expectEmit(address(deposit));
            emit ChargeGovernance.ChangeCancelled(action, ChargeGovernance.Seat(i + 2));
            deposit.cancelProposal(action, until, cancelSignature(deposit, signerKeys[i], action, until));
        }
    }

    function testEveryChangeOfAKeyIsAnnounced() public {
        vm.expectEmit(address(deposit));
        emit ChargeGovernance.KeyChanged(ChargeGovernance.Key.Witness, address(0));
        vm.prank(vm.addr(PAUSER_KEY));
        deposit.disableKey(ChargeGovernance.Key.Witness);

        bytes32 action = proposeOn(deposit, "setKey", keyArgs(ChargeGovernance.Key.Witness, next), "salt");
        proposeOn(deposit, "setSigner", signerArgs(2, address(0xE0B)), "salt");
        vm.warp(block.timestamp + 24 hours);
        vm.expectEmit(address(deposit));
        emit ChargeGovernance.ChangeApplied(action);
        vm.expectEmit(address(deposit));
        emit ChargeGovernance.KeyChanged(ChargeGovernance.Key.Witness, next);
        deposit.setKey(ChargeGovernance.Key.Witness, next, "salt");
        vm.expectEmit(address(deposit));
        emit ChargeGovernance.SignerChanged(2, address(0xE0B));
        deposit.setSigner(2, address(0xE0B), "salt");
    }
}
