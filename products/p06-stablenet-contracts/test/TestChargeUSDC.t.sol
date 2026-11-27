// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Test} from "forge-std/Test.sol";
import {TestChargeUSDC} from "../src/test-token/TestChargeUSDC.sol";

/// @notice The test dollar a top-up is paid in: it tells its signing domain, it always arrives, and a signed
/// transfer works once, for the one recipient and time it names.
contract TestChargeUSDCTest is Test {
    uint256 internal constant HOLDER_KEY = 0xB0A2D;
    TestChargeUSDC internal token;
    address internal holder;
    address internal collector = address(0xC011EC7);
    address internal other = address(0x07);

    function setUp() public {
        vm.warp(1_800_000_000);
        token = new TestChargeUSDC(address(this));
        holder = vm.addr(HOLDER_KEY);
        token.mint(holder, 100e6);
    }

    function signed(bytes32 kind, address to, uint256 value, uint256 validBefore, bytes32 nonce)
        internal
        view
        returns (uint8 v, bytes32 r, bytes32 s)
    {
        bytes32 content = keccak256(abi.encode(kind, holder, to, value, uint256(0), validBefore, nonce));
        return vm.sign(HOLDER_KEY, keccak256(abi.encodePacked(hex"1901", token.DOMAIN_SEPARATOR(), content)));
    }

    function testItTellsItsSigningDomain() public view {
        assertEq(token.decimals(), 6);
        assertEq(token.version(), "2");
        bytes32 expected = keccak256(
            abi.encode(
                keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"),
                keccak256(bytes(token.name())),
                keccak256(bytes(token.version())),
                block.chainid,
                address(token)
            )
        );
        assertEq(token.DOMAIN_SEPARATOR(), expected);
    }

    function testOnlyTheOwnerMints() public {
        vm.prank(other);
        vm.expectRevert(TestChargeUSDC.NotOwner.selector);
        token.mint(other, 1);
        vm.expectRevert(TestChargeUSDC.InvalidRecipient.selector);
        token.mint(address(token), 1);
        vm.expectRevert(TestChargeUSDC.ZeroAddress.selector);
        new TestChargeUSDC(address(0));
        assertEq(token.totalSupply(), 100e6);
    }

    function testPlainTransfersAndAllowances() public {
        vm.startPrank(holder);
        token.transfer(other, 10e6);
        token.approve(collector, 5e6);
        vm.expectRevert(TestChargeUSDC.InsufficientBalance.selector);
        token.transfer(other, 91e6);
        vm.expectRevert(TestChargeUSDC.InvalidRecipient.selector);
        token.transfer(address(token), 1);
        vm.stopPrank();
        vm.startPrank(collector);
        token.transferFrom(holder, collector, 4e6);
        vm.expectRevert(TestChargeUSDC.InsufficientAllowance.selector);
        token.transferFrom(holder, collector, 2e6);
        vm.stopPrank();
        assertEq(token.balanceOf(other), 10e6);
        assertEq(token.balanceOf(collector), 4e6);
        assertEq(token.allowance(holder, collector), 1e6);
    }

    function testOnlyTheNamedRecipientCollects() public {
        bytes32 kind = token.RECEIVE_WITH_AUTHORIZATION_TYPEHASH();
        (uint8 v, bytes32 r, bytes32 s) = signed(kind, collector, 30e6, block.timestamp + 120, "nonce");
        vm.prank(other);
        vm.expectRevert(TestChargeUSDC.CallerNotRecipient.selector);
        token.receiveWithAuthorization(holder, collector, 30e6, 0, block.timestamp + 120, "nonce", v, r, s);
        vm.startPrank(collector);
        vm.expectRevert(TestChargeUSDC.InvalidAuthorization.selector);
        token.receiveWithAuthorization(holder, collector, 31e6, 0, block.timestamp + 120, "nonce", v, r, s);
        token.receiveWithAuthorization(holder, collector, 30e6, 0, block.timestamp + 120, "nonce", v, r, s);
        vm.expectRevert(TestChargeUSDC.AuthorizationAlreadyUsed.selector);
        token.receiveWithAuthorization(holder, collector, 30e6, 0, block.timestamp + 120, "nonce", v, r, s);
        vm.stopPrank();
        assertEq(token.balanceOf(collector), 30e6);
        assertTrue(token.authorizationState(holder, "nonce"));
    }

    function testASignedTransferEndsAndIsNotAReceive() public {
        bytes32 kind = token.TRANSFER_WITH_AUTHORIZATION_TYPEHASH();
        uint256 end = block.timestamp + 120;
        (uint8 v, bytes32 r, bytes32 s) = signed(kind, other, 30e6, end, "nonce");
        vm.prank(other);
        vm.expectRevert(TestChargeUSDC.InvalidAuthorization.selector);
        token.receiveWithAuthorization(holder, other, 30e6, 0, end, "nonce", v, r, s);
        vm.expectRevert(TestChargeUSDC.AuthorizationNotYetValid.selector);
        token.transferWithAuthorization(holder, other, 30e6, block.timestamp, end, "nonce", v, r, s);
        vm.warp(end);
        vm.expectRevert(TestChargeUSDC.AuthorizationExpired.selector);
        token.transferWithAuthorization(holder, other, 30e6, 0, end, "nonce", v, r, s);
        vm.warp(end - 1);
        token.transferWithAuthorization(holder, other, 30e6, 0, end, "nonce", v, r, s);
        assertEq(token.balanceOf(other), 30e6);
    }

    function testAHighSSignatureIsRefused() public {
        bytes32 kind = token.TRANSFER_WITH_AUTHORIZATION_TYPEHASH();
        (uint8 v, bytes32 r, bytes32 s) = signed(kind, other, 30e6, block.timestamp + 120, "nonce");
        uint256 order = 0xFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141;
        bytes32 high = bytes32(order - uint256(s));
        uint8 flipped = v == 27 ? 28 : 27;
        vm.expectRevert(TestChargeUSDC.InvalidAuthorization.selector);
        token.transferWithAuthorization(holder, other, 30e6, 0, block.timestamp + 120, "nonce", flipped, r, high);
        vm.expectRevert(TestChargeUSDC.InvalidAuthorization.selector);
        token.transferWithAuthorization(holder, other, 30e6, 0, block.timestamp + 120, "nonce", 29, r, s);
    }
}
