// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Test} from "forge-std/Test.sol";
import {OrderEscrow} from "../src/OrderEscrow.sol";
import {PaymentTypesV2 as Types} from "../src/PaymentTypesV2.sol";
import {TestKRW3009} from "../src/test-token/TestKRW3009.sol";
import {TestToken3009} from "../src/test-token/TestToken3009.sol";

contract TestKRW3009Test is Test {
    uint256 internal constant BOARD_KEY = 401;
    uint256 internal constant STORE_KEY = 402;
    uint256 internal constant WON = 100; // 2 decimals
    bytes32 internal constant MERCHANT = bytes32(uint256(7));
    bytes32 internal constant TRANSFER_TYPE = keccak256(
        "TransferWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"
    );
    bytes32 internal constant RECEIVE_TYPE = keccak256(
        "ReceiveWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"
    );

    TestKRW3009 internal token;
    OrderEscrow internal escrow;
    address internal board;
    address internal store = makeAddr("store");
    address internal phone = makeAddr("phone");
    address internal home = makeAddr("home");
    uint256 internal start = 1_800_000_000;

    function setUp() public {
        vm.chainId(8283);
        vm.warp(start);
        board = vm.addr(BOARD_KEY);
        token = new TestKRW3009(address(this));
        token.mint(board, 100_000 * WON);
        escrow = new OrderEscrow(address(token), address(this), address(this));
        escrow.configureMerchant(MERCHANT, vm.addr(STORE_KEY), store, 2 days, 7 days);
    }

    function _domain(string memory name, address target) internal view returns (bytes32) {
        return keccak256(
            abi.encode(
                keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"),
                keccak256(bytes(name)),
                keccak256("1"),
                block.chainid,
                target
            )
        );
    }

    function _sign(bytes32 domain, bytes32 kind, address to, uint256 value, uint256 validBefore, bytes32 nonce)
        internal
        view
        returns (uint8 v, bytes32 r, bytes32 s)
    {
        bytes32 body = keccak256(abi.encode(kind, board, to, value, uint256(0), validBefore, nonce));
        return vm.sign(BOARD_KEY, keccak256(abi.encodePacked(hex"1901", domain, body)));
    }

    function _quote(uint256 amount) internal view returns (Types.MerchantQuote memory q) {
        q = Types.MerchantQuote({
            quoteId: bytes32(uint256(2)),
            tenantCommitment: bytes32(uint256(3)),
            branchCommitment: bytes32(uint256(4)),
            orderId: bytes32(uint256(5)),
            merchantId: MERCHANT,
            claimCommitment: bytes32(uint256(8)),
            intentCommitment: bytes32(uint256(9)),
            payer: board,
            asset: address(token),
            beneficiary: store,
            amount: amount,
            nonce: 11,
            merchantKeyVersion: 1,
            beneficiaryVersion: 1,
            payerGeneration: 1,
            issuedAt: uint64(start - 10),
            expiresAt: uint64(start + 110),
            mode: 1,
            payerRole: 1
        });
    }

    function _lockSignatures(Types.MerchantQuote memory q, string memory name)
        internal
        view
        returns (bytes memory merchant, bytes memory wallet)
    {
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(STORE_KEY, escrow.quoteDigest(q));
        merchant = abi.encodePacked(r, s, v);
        (v, r, s) = _sign(
            _domain(name, address(token)),
            RECEIVE_TYPE,
            address(escrow),
            q.amount,
            q.expiresAt,
            escrow.lockNonce(q, phone)
        );
        wallet = abi.encodePacked(r, s, v);
    }

    function test_metadataIsWholeAndFixed() public view {
        assertEq(token.name(), "NU54 Test KRW");
        assertEq(token.symbol(), "tKRW");
        assertEq(token.decimals(), 2);
        assertEq(token.version(), "1");
        assertEq(token.owner(), address(this));
        assertEq(token.DOMAIN_SEPARATOR(), _domain("NU54 Test KRW", address(token)));
        assertEq(token.RECEIVE_WITH_AUTHORIZATION_TYPEHASH(), RECEIVE_TYPE);
        assertEq(token.TRANSFER_WITH_AUTHORIZATION_TYPEHASH(), TRANSFER_TYPE);
    }

    function test_onlyTheNamedRecipientReceives() public {
        bytes32 nonce = bytes32(uint256(1));
        (uint8 v, bytes32 r, bytes32 s) =
            _sign(token.DOMAIN_SEPARATOR(), RECEIVE_TYPE, home, 40 * WON, start + 60, nonce);
        vm.prank(phone);
        vm.expectRevert(TestKRW3009.CallerNotRecipient.selector);
        token.receiveWithAuthorization(board, home, 40 * WON, 0, start + 60, nonce, v, r, s);
        // A receive signature is not a transfer signature: nobody else can push it.
        vm.expectRevert(TestToken3009.InvalidAuthorization.selector);
        token.transferWithAuthorization(board, home, 40 * WON, 0, start + 60, nonce, v, r, s);
        vm.prank(home);
        token.receiveWithAuthorization(board, home, 40 * WON, 0, start + 60, nonce, v, r, s);
        assertEq(token.balanceOf(home), 40 * WON);
        assertEq(token.balanceOf(board), 99_960 * WON);
        vm.prank(home);
        vm.expectRevert(TestToken3009.AuthorizationAlreadyUsed.selector);
        token.receiveWithAuthorization(board, home, 40 * WON, 0, start + 60, nonce, v, r, s);
    }

    function test_bothKindsShareOneNonce() public {
        bytes32 nonce = bytes32(uint256(2));
        (uint8 v, bytes32 r, bytes32 s) = _sign(token.DOMAIN_SEPARATOR(), RECEIVE_TYPE, home, WON, start + 60, nonce);
        vm.prank(home);
        token.receiveWithAuthorization(board, home, WON, 0, start + 60, nonce, v, r, s);
        (v, r, s) = _sign(token.DOMAIN_SEPARATOR(), TRANSFER_TYPE, home, WON, start + 60, nonce);
        vm.expectRevert(TestToken3009.AuthorizationAlreadyUsed.selector);
        token.transferWithAuthorization(board, home, WON, 0, start + 60, nonce, v, r, s);
    }

    function test_aReceiveSignatureIsBoundToEveryField() public {
        bytes32 nonce = bytes32(uint256(3));
        (uint8 v, bytes32 r, bytes32 s) =
            _sign(token.DOMAIN_SEPARATOR(), RECEIVE_TYPE, home, 40 * WON, start + 60, nonce);
        vm.startPrank(home);
        vm.expectRevert(TestToken3009.InvalidAuthorization.selector);
        token.receiveWithAuthorization(board, home, 40 * WON + 1, 0, start + 60, nonce, v, r, s);
        vm.expectRevert(TestToken3009.InvalidAuthorization.selector);
        token.receiveWithAuthorization(board, home, 40 * WON, 0, start + 61, nonce, v, r, s);
        vm.expectRevert(TestToken3009.InvalidAuthorization.selector);
        token.receiveWithAuthorization(board, home, 40 * WON, 1, start + 60, nonce, v, r, s);
        vm.expectRevert(TestToken3009.InvalidAuthorization.selector);
        token.receiveWithAuthorization(board, home, 40 * WON, 0, start + 60, bytes32(uint256(4)), v, r, s);
        uint256 order = 0xfffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141;
        vm.expectRevert(TestToken3009.InvalidAuthorization.selector);
        token.receiveWithAuthorization(
            board, home, 40 * WON, 0, start + 60, nonce, v == 27 ? 28 : 27, r, bytes32(order - uint256(s))
        );
        vm.chainId(84532);
        vm.expectRevert(TestToken3009.InvalidAuthorization.selector);
        token.receiveWithAuthorization(board, home, 40 * WON, 0, start + 60, nonce, v, r, s);
        vm.chainId(8283);
        vm.warp(start + 60);
        vm.expectRevert(TestToken3009.AuthorizationExpired.selector);
        token.receiveWithAuthorization(board, home, 40 * WON, 0, start + 60, nonce, v, r, s);
        vm.warp(0);
        vm.expectRevert(TestToken3009.AuthorizationNotYetValid.selector);
        token.receiveWithAuthorization(board, home, 40 * WON, 0, start + 60, nonce, v, r, s);
        vm.stopPrank();
        assertEq(token.balanceOf(board), 100_000 * WON);
    }

    function test_receiveRefusesShortBalanceAndTheTokenItself() public {
        bytes32 nonce = bytes32(uint256(5));
        uint256 tooMuch = 100_000 * WON + 1;
        (uint8 v, bytes32 r, bytes32 s) =
            _sign(token.DOMAIN_SEPARATOR(), RECEIVE_TYPE, home, tooMuch, start + 60, nonce);
        vm.prank(home);
        vm.expectRevert(TestToken3009.InsufficientBalance.selector);
        token.receiveWithAuthorization(board, home, tooMuch, 0, start + 60, nonce, v, r, s);
        assertFalse(token.authorizationState(board, nonce));
        (v, r, s) = _sign(token.DOMAIN_SEPARATOR(), RECEIVE_TYPE, address(token), WON, start + 60, nonce);
        vm.prank(address(token));
        vm.expectRevert(TestToken3009.InvalidRecipient.selector);
        token.receiveWithAuthorization(board, address(token), WON, 0, start + 60, nonce, v, r, s);
    }

    function test_receivingOwnMoneyKeepsTheBalance() public {
        bytes32 nonce = bytes32(uint256(6));
        (uint8 v, bytes32 r, bytes32 s) = _sign(token.DOMAIN_SEPARATOR(), RECEIVE_TYPE, board, WON, start + 60, nonce);
        vm.prank(board);
        token.receiveWithAuthorization(board, board, WON, 0, start + 60, nonce, v, r, s);
        assertEq(token.balanceOf(board), 100_000 * WON);
    }

    function test_thereIsNoRightToRefuse() public {
        vm.prank(store);
        (bool ok,) = address(token).call(abi.encodeWithSignature("setRefusalMode(bool)", true));
        assertFalse(ok);
        vm.prank(board);
        assertTrue(token.transfer(store, WON));
        assertEq(token.balanceOf(store), WON);
    }

    function test_anOrderIsPaidInWonAndReturned() public {
        Types.MerchantQuote memory q = _quote(12_500 * WON);
        (bytes memory merchant, bytes memory wallet) = _lockSignatures(q, "NU54 Test KRW");
        escrow.lock(q, phone, merchant, wallet);
        assertEq(token.balanceOf(address(escrow)), 12_500 * WON);
        assertEq(token.balanceOf(board), 87_500 * WON);
        vm.warp(start + 110 + 8 days);
        escrow.refundUndelivered(MERCHANT, q.orderId);
        assertEq(token.balanceOf(address(escrow)), 0);
        assertEq(token.balanceOf(board), 100_000 * WON);
    }

    function test_aSignatureForAnotherNameAtTheSameAddressIsRefused() public {
        Types.MerchantQuote memory q = _quote(12_500 * WON);
        string[3] memory names = ["NU54 Test USD", "NU54 Test KRWC", "NU54 Test KRW "];
        for (uint256 i; i < names.length; ++i) {
            (bytes memory merchant, bytes memory wallet) = _lockSignatures(q, names[i]);
            vm.expectRevert(TestToken3009.InvalidAuthorization.selector);
            escrow.lock(q, phone, merchant, wallet);
        }
        assertEq(token.balanceOf(board), 100_000 * WON);
    }

    function test_onlyTheOwnerMints() public {
        vm.prank(board);
        vm.expectRevert(TestToken3009.NotOwner.selector);
        token.mint(board, 1);
        vm.expectRevert(TestToken3009.ZeroAddress.selector);
        new TestKRW3009(address(0));
    }
}
