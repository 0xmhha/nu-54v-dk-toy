// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Test} from "forge-std/Test.sol";
import {TestUSDC} from "../src/test-token/TestUSDC.sol";

/// Right to refuse: held transfers, accept, reject, team return and the trusted-sender list.
contract TestUSDCTest is Test {
    TestUSDC internal token;
    address internal team = makeAddr("team");
    address internal alice = makeAddr("alice"); // sender
    address internal bob = makeAddr("bob"); // recipient
    address internal system = makeAddr("system"); // e.g. the settlement contract

    function setUp() public {
        vm.warp(1_790_000_000);
        token = new TestUSDC(team);
        vm.startPrank(team);
        token.mint(alice, 100e6);
        token.mint(system, 100e6);
        vm.stopPrank();
    }

    function _hold(uint256 amount) internal returns (uint256 id) {
        vm.prank(bob);
        token.setRefusalMode(true);
        id = token.nextHeldId();
        vm.prank(alice);
        token.transfer(bob, amount);
    }

    function test_refusal_offByDefault() public {
        assertFalse(token.refusalMode(bob));
        vm.prank(alice);
        token.transfer(bob, 1e6);
        assertEq(token.balanceOf(bob), 1e6);
    }

    function test_refusal_holdsTransfer() public {
        vm.prank(bob);
        token.setRefusalMode(true);
        vm.expectEmit(true, true, true, true);
        emit TestUSDC.TransferHeld(1, alice, bob, 5e6);
        vm.prank(alice);
        assertTrue(token.transfer(bob, 5e6));
        assertEq(token.balanceOf(bob), 0, "nothing reaches the recipient before acceptance");
        assertEq(token.balanceOf(alice), 95e6, "step 1: the tokens left the sender");
        assertEq(token.balanceOf(address(token)), 5e6);
        (address from, address to, uint256 amount, uint256 heldAt) = token.heldTransfer(1);
        assertEq(from, alice);
        assertEq(to, bob);
        assertEq(amount, 5e6);
        assertEq(heldAt, block.timestamp);
    }

    function test_refusal_accept() public {
        uint256 id = _hold(5e6);
        vm.expectEmit(true, true, true, true);
        emit TestUSDC.TransferAccepted(id, alice, bob, 5e6);
        vm.prank(bob);
        token.acceptTransfer(id);
        assertEq(token.balanceOf(bob), 5e6);
        assertEq(token.balanceOf(address(token)), 0);
        vm.prank(bob);
        vm.expectRevert(TestUSDC.UnknownHeldTransfer.selector);
        token.acceptTransfer(id);
    }

    function test_refusal_rejectReturnsAtOnce() public {
        uint256 id = _hold(5e6);
        vm.expectEmit(true, true, true, true);
        emit TestUSDC.TransferRejected(id, alice, bob, 5e6);
        vm.prank(bob);
        token.rejectTransfer(id);
        assertEq(token.balanceOf(alice), 100e6);
        assertEq(token.balanceOf(bob), 0);
        vm.prank(bob);
        vm.expectRevert(TestUSDC.UnknownHeldTransfer.selector);
        token.acceptTransfer(id);
    }

    function test_refusal_rejectReachesSenderInRefusalMode() public {
        vm.prank(alice);
        token.setRefusalMode(true);
        uint256 id = _hold(5e6);
        vm.prank(bob);
        token.rejectTransfer(id);
        assertEq(token.balanceOf(alice), 100e6, "a returned transfer is not held again");
    }

    function test_refusal_onlyRecipientDecides() public {
        uint256 id = _hold(5e6);
        vm.prank(alice); // the sender cannot cancel
        vm.expectRevert(TestUSDC.NotRecipient.selector);
        token.rejectTransfer(id);
        vm.prank(team);
        vm.expectRevert(TestUSDC.NotRecipient.selector);
        token.acceptTransfer(id);
    }

    function test_refusal_teamReturnAfterDelayOnly() public {
        uint256 id = _hold(5e6);
        vm.prank(team);
        vm.expectRevert(TestUSDC.ReturnNotYetAllowed.selector);
        token.returnToSender(id);
        vm.warp(block.timestamp + token.RETURN_DELAY());
        vm.prank(bob);
        vm.expectRevert(TestUSDC.NotOwner.selector);
        token.returnToSender(id);
        vm.expectEmit(true, true, true, true);
        emit TestUSDC.TransferReturned(id, alice, bob, 5e6);
        vm.prank(team);
        token.returnToSender(id);
        assertEq(token.balanceOf(alice), 100e6, "only to the original sender");
    }

    function test_refusal_heldSurvivesModeOff() public {
        uint256 id = _hold(5e6);
        vm.prank(bob);
        token.setRefusalMode(false);
        vm.prank(alice);
        token.transfer(bob, 1e6); // direct now
        assertEq(token.balanceOf(bob), 1e6);
        vm.prank(bob);
        token.acceptTransfer(id);
        assertEq(token.balanceOf(bob), 6e6);
    }

    function test_refusal_transferFromRecordsOwnerAsSender() public {
        address spender = makeAddr("spender");
        vm.prank(alice);
        token.approve(spender, 5e6);
        vm.prank(bob);
        token.setRefusalMode(true);
        vm.prank(spender);
        token.transferFrom(alice, bob, 5e6);
        (address from,,,) = token.heldTransfer(1);
        assertEq(from, alice, "a rejection returns to the token owner, not the spender");
    }

    function test_refusal_selfTransferNotHeld() public {
        vm.startPrank(alice);
        token.setRefusalMode(true);
        token.transfer(alice, 1e6);
        vm.stopPrank();
        assertEq(token.balanceOf(alice), 100e6);
        assertEq(token.nextHeldId(), 1);
    }

    function test_trustedSender_skipsHoldByDefault() public {
        vm.prank(team);
        token.setTrustedSender(system, true);
        vm.prank(bob);
        token.setRefusalMode(true);
        vm.prank(system);
        token.transfer(bob, 5e6);
        assertEq(token.balanceOf(bob), 5e6);
        assertFalse(token.wouldHold(system, bob));
        assertTrue(token.wouldHold(alice, bob));
    }

    function test_trustedSender_userOptOut() public {
        vm.prank(team);
        token.setTrustedSender(system, true);
        vm.startPrank(bob);
        token.setRefusalMode(true);
        token.setTrustOptOut(system, true);
        vm.stopPrank();
        vm.prank(system);
        token.transfer(bob, 5e6);
        assertEq(token.balanceOf(bob), 0, "opted out: held like any other sender");
        vm.prank(bob);
        token.acceptTransfer(1);
        assertEq(token.balanceOf(bob), 5e6);
    }

    function test_trustedSender_onlyOwner() public {
        vm.prank(alice);
        vm.expectRevert(TestUSDC.NotOwner.selector);
        token.setTrustedSender(alice, true);
    }

    function test_directTransferToTokenRefused() public {
        vm.prank(alice);
        vm.expectRevert(TestUSDC.InvalidRecipient.selector);
        token.transfer(address(token), 1);
        vm.prank(alice);
        vm.expectRevert(TestUSDC.InvalidRecipient.selector);
        token.transfer(address(0), 1);
    }

    /// Held balances are always exactly the token contract's own balance, and supply is conserved.
    function testFuzz_refusal_conservation(uint256 a1, uint256 a2, uint8 choice) public {
        a1 = bound(a1, 1, 40e6);
        a2 = bound(a2, 1, 40e6);
        vm.prank(bob);
        token.setRefusalMode(true);
        vm.startPrank(alice);
        token.transfer(bob, a1);
        token.transfer(bob, a2);
        vm.stopPrank();
        assertEq(token.balanceOf(address(token)), a1 + a2);
        vm.prank(bob);
        if (choice % 2 == 0) token.acceptTransfer(1);
        else token.rejectTransfer(1);
        assertEq(token.balanceOf(address(token)), a2);
        assertEq(
            token.balanceOf(alice) + token.balanceOf(bob) + token.balanceOf(address(token)) + token.balanceOf(system),
            token.totalSupply()
        );
    }

    // Minting needs a recipient that always receives.
    function test_mint_requiresReceivingAccount() public {
        vm.prank(bob);
        token.setRefusalMode(true);
        vm.prank(team);
        vm.expectRevert(TestUSDC.RecipientRefusing.selector);
        token.mint(bob, 1e6);
        vm.prank(bob);
        token.setRefusalMode(false);
        vm.prank(team);
        token.mint(bob, 1e6);
        assertEq(token.balanceOf(bob), 1e6);
    }

    function test_refusal_zeroTransferNotHeld() public {
        vm.prank(bob);
        token.setRefusalMode(true);
        vm.prank(alice);
        token.transfer(bob, 0);
        assertEq(token.nextHeldId(), 1);
    }

    // Packed held record: amounts above uint96 cannot be held.
    function test_refusal_amountAboveUint96Rejected() public {
        uint256 big = uint256(type(uint96).max) + 1;
        vm.prank(team);
        token.mint(alice, big);
        vm.prank(bob);
        token.setRefusalMode(true);
        vm.prank(alice);
        vm.expectRevert(TestUSDC.AmountTooLarge.selector);
        token.transfer(bob, big);
    }
}
