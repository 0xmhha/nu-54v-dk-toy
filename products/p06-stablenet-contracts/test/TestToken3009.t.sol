// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Test} from "forge-std/Test.sol";
import {DeployTestTokens} from "../script/DeployTestTokens.s.sol";
import {TestToken3009} from "../src/test-token/TestToken3009.sol";
import {TestUSDC3009} from "../src/test-token/TestUSDC3009.sol";

contract TestToken3009Test is Test {
    uint256 internal constant BOARD_KEY = 301;
    TestToken3009 internal token;
    address internal board;
    address internal home = makeAddr("home");
    address internal relay = makeAddr("relay");
    uint256 internal start = 1_800_000_000;

    function setUp() public {
        vm.chainId(8283);
        vm.warp(start);
        board = vm.addr(BOARD_KEY);
        token = new TestToken3009(address(this), "NU54 Test USDT", "tUSDT", 6);
        token.mint(board, 100e6);
    }

    function _sign(TestToken3009 target, uint256 value, uint256 validBefore, bytes32 nonce)
        internal
        view
        returns (uint8 v, bytes32 r, bytes32 s)
    {
        bytes32 domain = keccak256(
            abi.encode(
                keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"),
                keccak256(bytes(target.name())),
                keccak256("1"),
                block.chainid,
                address(target)
            )
        );
        bytes32 body = keccak256(
            abi.encode(
                keccak256(
                    "TransferWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"
                ),
                board,
                home,
                value,
                uint256(0),
                validBefore,
                nonce
            )
        );
        return vm.sign(BOARD_KEY, keccak256(abi.encodePacked(hex"1901", domain, body)));
    }

    function test_signedTransferNeedsNoGasOfTheHolder() public {
        (uint8 v, bytes32 r, bytes32 s) = _sign(token, 40e6, start + 60, bytes32(uint256(1)));
        vm.prank(relay);
        token.transferWithAuthorization(board, home, 40e6, 0, start + 60, bytes32(uint256(1)), v, r, s);
        assertEq(token.balanceOf(home), 40e6);
        assertEq(token.balanceOf(board), 60e6);
        vm.expectRevert(TestToken3009.AuthorizationAlreadyUsed.selector);
        token.transferWithAuthorization(board, home, 40e6, 0, start + 60, bytes32(uint256(1)), v, r, s);
    }

    function test_aSignatureIsBoundToEveryField() public {
        bytes32 nonce = bytes32(uint256(2));
        (uint8 v, bytes32 r, bytes32 s) = _sign(token, 40e6, start + 60, nonce);
        vm.expectRevert(TestToken3009.InvalidAuthorization.selector);
        token.transferWithAuthorization(board, home, 41e6, 0, start + 60, nonce, v, r, s);
        vm.expectRevert(TestToken3009.InvalidAuthorization.selector);
        token.transferWithAuthorization(board, relay, 40e6, 0, start + 60, nonce, v, r, s);
        vm.expectRevert(TestToken3009.InvalidAuthorization.selector);
        token.transferWithAuthorization(board, home, 40e6, 0, start + 61, nonce, v, r, s);
        // Not on another token of the same shape, and not on another chain.
        TestToken3009 other = new TestToken3009(address(this), "NU54 Test KRWC", "tKRWC", 18);
        other.mint(board, 100e6);
        vm.expectRevert(TestToken3009.InvalidAuthorization.selector);
        other.transferWithAuthorization(board, home, 40e6, 0, start + 60, nonce, v, r, s);
        vm.chainId(84532);
        vm.expectRevert(TestToken3009.InvalidAuthorization.selector);
        token.transferWithAuthorization(board, home, 40e6, 0, start + 60, nonce, v, r, s);
        vm.chainId(8283);
        // The high-s twin of a valid signature is refused, so a transfer has one signature.
        uint256 order = 0xfffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141;
        vm.expectRevert(TestToken3009.InvalidAuthorization.selector);
        token.transferWithAuthorization(
            board, home, 40e6, 0, start + 60, nonce, v == 27 ? 28 : 27, r, bytes32(order - uint256(s))
        );
        vm.warp(start + 60);
        vm.expectRevert(TestToken3009.AuthorizationExpired.selector);
        token.transferWithAuthorization(board, home, 40e6, 0, start + 60, nonce, v, r, s);
    }

    function test_plainTransfersAndAllowances() public {
        vm.prank(board);
        assertTrue(token.transfer(home, 10e6));
        vm.prank(home);
        token.approve(relay, 4e6);
        vm.prank(relay);
        assertTrue(token.transferFrom(home, board, 4e6));
        assertEq(token.allowance(home, relay), 0);
        vm.prank(relay);
        vm.expectRevert(TestToken3009.InsufficientAllowance.selector);
        token.transferFrom(home, board, 1);
        vm.prank(home);
        vm.expectRevert(TestToken3009.InsufficientBalance.selector);
        token.transfer(board, 100e6);
        vm.prank(home);
        vm.expectRevert(TestToken3009.InvalidRecipient.selector);
        token.transfer(address(token), 1);
        vm.prank(home);
        vm.expectRevert(TestToken3009.InvalidRecipient.selector);
        token.transfer(address(0), 1);
    }

    function test_onlyTheOwnerMints() public {
        vm.prank(board);
        vm.expectRevert(TestToken3009.NotOwner.selector);
        token.mint(board, 1);
        vm.expectRevert(TestToken3009.InvalidRecipient.selector);
        token.mint(address(0), 1);
        vm.expectRevert(TestToken3009.ZeroAddress.selector);
        new TestToken3009(address(0), "a", "b", 6);
        assertEq(token.totalSupply(), 100e6);
    }

    function test_theScriptDeploysThreeTokensOnTestNetworksOnly() public {
        DeployTestTokens script = new DeployTestTokens();
        vm.setEnv("NU54_TOKEN_OWNER", vm.toString(address(this)));
        uint256[4] memory chains = [uint256(8283), 11155111, 84532, 31337];
        for (uint256 i; i < chains.length; i++) {
            vm.chainId(chains[i]);
            (TestUSDC3009 usdc, TestToken3009 usdt, TestToken3009 krwc) = script.run();
            assertEq(usdc.decimals(), 6);
            assertEq(usdt.symbol(), "tUSDT");
            assertEq(krwc.decimals(), 18);
            assertEq(krwc.owner(), address(this));
            assertTrue(usdt.DOMAIN_SEPARATOR() != krwc.DOMAIN_SEPARATOR());
        }
        vm.chainId(1);
        vm.expectRevert(bytes("not a test network"));
        script.run();
        vm.chainId(8282); // the StableNet main network
        vm.expectRevert(bytes("not a test network"));
        script.run();
    }
}
