// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {ChargeBase} from "./ChargeBase.t.sol";
import {ChargeTypes} from "../src/charge/ChargeTypes.sol";
import {ChargeGovernance} from "../src/charge/ChargeGovernance.sol";
import {ChargeDeposit} from "../src/charge/ChargeDeposit.sol";
import {WonPool} from "../src/charge/WonPool.sol";

/// @notice The keys of both top-up contracts: one key narrows or stops, two signers and a day widen.
contract ChargeGovernanceTest is ChargeBase {
    address internal next = address(0xE0A);

    function keyAction(ChargeGovernance.Key which, address to, bytes32 salt) internal pure returns (bytes32) {
        return keccak256(abi.encode(keccak256("setKey"), which, to, salt));
    }

    function signerAction(uint8 index, address to, bytes32 salt) internal pure returns (bytes32) {
        return keccak256(abi.encode(keccak256("setSigner"), index, to, salt));
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
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.setKey(ChargeGovernance.Key.Relay, next, "salt"); // nobody proposed it

        uint256 until = block.timestamp + 1 days;
        bytes32 digest = deposit.actionDigest(keccak256("propose"), action, 0, until);
        bytes memory one = sign(SIGNER1_KEY, digest);
        vm.expectRevert(ChargeGovernance.SignersNotDistinct.selector);
        deposit.propose(action, until, [one, one]);
        uint256[4] memory others = [RELAY_KEY, QUOTER_KEY, WITNESS_KEY, PAUSER_KEY];
        for (uint256 i; i < others.length; ++i) {
            vm.expectRevert(ChargeGovernance.Unauthorized.selector);
            deposit.propose(action, until, [one, sign(others[i], digest)]);
        }
        deposit.propose(action, until, [sign(SIGNER3_KEY, digest), one]);
    }

    function testAChangeWaitsADayAndLapsesThreeDaysLater() public {
        bytes32 action = keyAction(ChargeGovernance.Key.Relay, next, "salt");
        proposeOn(deposit, action);
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
        deposit.propose(action, until, signatures);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.propose(action, until, signatures); // already waiting

        vm.prank(stranger);
        vm.expectRevert(ChargeTypes.InvalidSignature.selector);
        deposit.cancelProposal(action, until, "");
        vm.prank(vm.addr(PAUSER_KEY));
        deposit.cancelProposal(action, 0, "");
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.propose(action, until, signatures);
        vm.warp(block.timestamp + 24 hours);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.setKey(ChargeGovernance.Key.Relay, next, "salt");

        // A change that lapsed needs a new salt and new signatures too.
        bytes32 second = keyAction(ChargeGovernance.Key.Relay, next, "salt 2");
        bytes[2] memory secondSignatures = two(deposit, "propose", second, 0, until);
        deposit.propose(second, until, secondSignatures);
        vm.warp(block.timestamp + 4 days + 1);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.propose(second, until, secondSignatures);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.setKey(ChargeGovernance.Key.Relay, next, "salt 2");
    }

    function testOneSignerCancelsBySignature() public {
        bytes32 action = keyAction(ChargeGovernance.Key.Relay, next, "salt");
        proposeOn(deposit, action);
        uint256 until = block.timestamp + 1 days;
        bytes32 digest = deposit.actionDigest(keccak256("cancel"), action, 0, until);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.cancelProposal(action, until, sign(RELAY_KEY, digest));
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.cancelProposal(action, block.timestamp, sign(SIGNER3_KEY, digest));
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
        deposit.propose(action, until, signatures);
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

        vm.warp(block.timestamp + 24 hours);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.setKey(ChargeGovernance.Key.Quoter, next, "salt");
        // The same change can be proposed again, but only with signatures made after the switch.
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.propose(action, until, signatures);
        proposeOn(deposit, action);
        vm.warp(block.timestamp + 24 hours);
        deposit.setKey(ChargeGovernance.Key.Quoter, next, "salt");
        assertEq(deposit.quoter(), next);
    }

    function testAKeyThatIsOffComesBackOnlyByTwoSignersAndStaysDistinct() public {
        toWon();
        vm.prank(vm.addr(PAUSER_KEY));
        pool.disableKey(ChargeGovernance.Key.Witness);
        address relayKey = vm.addr(RELAY_KEY);
        // The confirmation key can never become the relay's own key, nor any other key in use.
        proposeOn(pool, keyAction(ChargeGovernance.Key.Witness, relayKey, "salt"));
        proposeOn(pool, keyAction(ChargeGovernance.Key.Witness, vm.addr(SIGNER2_KEY), "salt"));
        proposeOn(pool, keyAction(ChargeGovernance.Key.Witness, address(0), "salt"));
        proposeOn(pool, keyAction(ChargeGovernance.Key.Witness, next, "salt"));
        proposeOn(pool, keyAction(ChargeGovernance.Key.Relay, next, "salt"));
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
        proposeOn(deposit, keyAction(ChargeGovernance.Key.Pauser, next, "salt"));
        vm.warp(block.timestamp + 24 hours);
        deposit.setKey(ChargeGovernance.Key.Pauser, next, "salt");
        vm.prank(vm.addr(PAUSER_KEY));
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.pauseDeposits();
        vm.prank(next);
        deposit.pauseDeposits();
        assertTrue(deposit.depositsPaused());
    }

    function testAReplacedSignerNoLongerCounts() public {
        uint256 newKey = 0xA008;
        proposeOn(deposit, signerAction(0, vm.addr(newKey), "salt"));
        proposeOn(deposit, signerAction(3, next, "salt"));
        proposeOn(deposit, signerAction(1, vm.addr(SIGNER3_KEY), "salt"));
        vm.warp(block.timestamp + 24 hours);
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.setSigner(3, next, "salt"); // there are three signers
        vm.expectRevert(ChargeGovernance.InvalidParameters.selector);
        deposit.setSigner(1, vm.addr(SIGNER3_KEY), "salt"); // one person cannot hold two places
        deposit.setSigner(0, vm.addr(newKey), "salt");
        assertEq(deposit.signers(0), vm.addr(newKey));
        assertEq(deposit.generation(), 1);

        bytes32 action = keyAction(ChargeGovernance.Key.Relay, next, "salt");
        uint256 until = block.timestamp + 1 days;
        bytes32 digest = deposit.actionDigest(keccak256("propose"), action, 1, until);
        vm.expectRevert(ChargeGovernance.Unauthorized.selector);
        deposit.propose(action, until, [sign(SIGNER1_KEY, digest), sign(SIGNER2_KEY, digest)]);
        deposit.propose(action, until, [sign(newKey, digest), sign(SIGNER2_KEY, digest)]);
    }

    function testProposalSignaturesEndAndCannotBeLong() public {
        bytes32 action = keyAction(ChargeGovernance.Key.Relay, next, "salt");
        uint256 tooLong = block.timestamp + 7 days + 1;
        bytes[2] memory long = two(deposit, "propose", action, 0, tooLong);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.propose(action, tooLong, long);
        uint256 until = block.timestamp + 1 days;
        bytes[2] memory signatures = two(deposit, "propose", action, 0, until);
        vm.warp(until);
        vm.expectRevert(ChargeGovernance.InvalidTime.selector);
        deposit.propose(action, until, signatures);
    }
}
