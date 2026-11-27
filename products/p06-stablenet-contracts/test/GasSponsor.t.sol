// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Test} from "forge-std/Test.sol";
import {DeployGasSponsor} from "../script/DeployGasSponsor.s.sol";
import {GasSponsor} from "../src/GasSponsor.sol";
import {OrderEscrow} from "../src/OrderEscrow.sol";
import {TestUSDC3009} from "../src/test-token/TestUSDC3009.sol";
import {TestToken3009} from "../src/test-token/TestToken3009.sol";

/// A target written for the sponsor: it reads the user after the calldata (ERC-2771).
contract Guestbook {
    address public immutable forwarder;
    mapping(address => uint256) public marks;
    GasSponsor.Request internal again;
    bytes internal againSignature;

    error Refused(uint256 code);

    constructor(address forwarder_) {
        forwarder = forwarder_;
    }

    function _user() internal view returns (address) {
        if (msg.sender != forwarder) return msg.sender;
        return address(bytes20(msg.data[msg.data.length - 20:]));
    }

    function mark(uint256 value) external returns (uint256) {
        marks[_user()] += value;
        return marks[_user()];
    }

    function refuse() external pure {
        revert Refused(7);
    }

    function reenter(address target, bytes calldata data) external {
        GasSponsor(forwarder).sponsor(target, data);
    }

    function burn(uint256 rounds) external returns (uint256 sum) {
        for (uint256 i; i < rounds; i++) {
            sum ^= uint256(keccak256(abi.encode(i)));
        }
        marks[_user()] = 1;
    }
}

contract GasSponsorTest is Test {
    uint256 internal constant USER_KEY = 401;
    GasSponsor internal sponsor;
    Guestbook internal book;
    TestToken3009 internal token;
    address internal user;
    address internal relay = makeAddr("store gas account");
    address internal shop = makeAddr("shop");
    uint256 internal start = 1_800_000_000;

    function setUp() public {
        vm.chainId(8283);
        vm.warp(start);
        user = vm.addr(USER_KEY);
        sponsor = new GasSponsor(address(this));
        book = new Guestbook(address(sponsor));
        token = new TestToken3009(address(this), "NU54 Test KRWC", "tKRWC", 18);
        token.mint(user, 100 ether);
        sponsor.setRelay(relay, true);
        _allow(address(token), TestToken3009.transferWithAuthorization.selector, GasSponsor.Kind.SIGNED);
        _allow(address(book), Guestbook.mark.selector, GasSponsor.Kind.USER);
        _allow(address(book), Guestbook.refuse.selector, GasSponsor.Kind.USER);
        _allow(address(book), Guestbook.burn.selector, GasSponsor.Kind.USER);
        _allow(address(book), Guestbook.reenter.selector, GasSponsor.Kind.SIGNED);
    }

    function _allow(address target, bytes4 selector, GasSponsor.Kind kind) internal {
        bytes4[] memory one = new bytes4[](1);
        one[0] = selector;
        sponsor.setActions(target, one, kind);
    }

    function _request(bytes memory data) internal view returns (GasSponsor.Request memory) {
        return GasSponsor.Request({
            user: user,
            target: address(book),
            data: data,
            gas: 100_000,
            validBefore: uint64(start + 120),
            nonce: bytes32(uint256(1))
        });
    }

    function _sign(GasSponsor.Request memory request) internal view returns (bytes memory) {
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(USER_KEY, sponsor.requestDigest(request));
        return abi.encodePacked(r, s, v);
    }

    function _execute(GasSponsor.Request memory request, bytes memory signature) internal returns (bytes memory) {
        vm.prank(relay);
        return sponsor.execute(request, signature);
    }

    function _transfer(uint256 amount) internal view returns (bytes memory) {
        bytes32 nonce = bytes32(uint256(5));
        bytes32 body = keccak256(
            abi.encode(token.TRANSFER_WITH_AUTHORIZATION_TYPEHASH(), user, shop, amount, uint256(0), start + 60, nonce)
        );
        (uint8 v, bytes32 r, bytes32 s) =
            vm.sign(USER_KEY, keccak256(abi.encodePacked(hex"1901", token.DOMAIN_SEPARATOR(), body)));
        return
            abi.encodeCall(TestToken3009.transferWithAuthorization, (user, shop, amount, 0, start + 60, nonce, v, r, s));
    }

    function test_aSignedTransferIsPaidForByTheStore() public {
        bytes memory data = _transfer(3 ether);
        vm.prank(relay);
        sponsor.sponsor(address(token), data);
        assertEq(token.balanceOf(shop), 3 ether);
        assertEq(user.balance, 0);
    }

    function test_aChangedSignedTransferIsRefusedByTheToken() public {
        bytes memory data = _transfer(3 ether);
        data[4 + 64 + 31] = 0x09; // another amount
        vm.prank(relay);
        vm.expectRevert(TestToken3009.InvalidAuthorization.selector);
        sponsor.sponsor(address(token), data);
        assertEq(token.balanceOf(shop), 0);
    }

    function test_aUserRequestRunsForThatUserOnce() public {
        GasSponsor.Request memory request = _request(abi.encodeCall(Guestbook.mark, (11)));
        bytes memory signature = _sign(request);
        assertEq(abi.decode(_execute(request, signature), (uint256)), 11);
        assertEq(book.marks(user), 11);
        assertEq(book.marks(address(sponsor)), 0);
        vm.prank(relay);
        vm.expectRevert(GasSponsor.RequestUsed.selector);
        sponsor.execute(request, signature);
    }

    function test_aRequestIsBoundToEveryFieldTheChainAndThisContract() public {
        GasSponsor.Request memory signedRequest = _request(abi.encodeCall(Guestbook.mark, (11)));
        bytes memory signature = _sign(signedRequest);
        for (uint256 field; field < 6; field++) {
            GasSponsor.Request memory changed = _request(abi.encodeCall(Guestbook.mark, (11)));
            if (field == 0) changed.user = shop;
            if (field == 1) changed.target = address(new Guestbook(address(sponsor)));
            if (field == 2) changed.data = abi.encodeCall(Guestbook.mark, (12));
            if (field == 3) changed.gas = 1;
            if (field == 4) changed.validBefore += 1;
            if (field == 5) changed.nonce = bytes32(uint256(2));
            if (field == 1) _allow(changed.target, Guestbook.mark.selector, GasSponsor.Kind.USER);
            vm.prank(relay);
            vm.expectRevert(GasSponsor.InvalidSignature.selector);
            sponsor.execute(changed, signature);
        }
        vm.chainId(84532);
        vm.prank(relay);
        vm.expectRevert(GasSponsor.InvalidSignature.selector);
        sponsor.execute(signedRequest, signature);
        vm.chainId(8283);
        GasSponsor other = new GasSponsor(address(this));
        other.setRelay(relay, true);
        bytes4[] memory one = new bytes4[](1);
        one[0] = Guestbook.mark.selector;
        other.setActions(address(book), one, GasSponsor.Kind.USER);
        vm.prank(relay);
        vm.expectRevert(GasSponsor.InvalidSignature.selector);
        other.execute(signedRequest, signature);
        // The high-s twin and a short signature are refused.
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(USER_KEY, sponsor.requestDigest(signedRequest));
        uint256 order = 0xfffffffffffffffffffffffffffffffebaaedce6af48a03bbfd25e8cd0364141;
        vm.prank(relay);
        vm.expectRevert(GasSponsor.InvalidSignature.selector);
        sponsor.execute(
            signedRequest, abi.encodePacked(r, bytes32(order - uint256(s)), v == 27 ? uint8(28) : uint8(27))
        );
        vm.prank(relay);
        vm.expectRevert(GasSponsor.InvalidSignature.selector);
        sponsor.execute(signedRequest, abi.encodePacked(r, s));
        assertEq(book.marks(user), 0);
    }

    function test_aRequestEndsAndCannotBeKept() public {
        GasSponsor.Request memory request = _request(abi.encodeCall(Guestbook.mark, (1)));
        bytes memory signature = _sign(request);
        vm.warp(request.validBefore);
        vm.prank(relay);
        vm.expectRevert(GasSponsor.RequestExpired.selector);
        sponsor.execute(request, signature);
        vm.warp(start);
        request.validBefore = uint64(start + 10 minutes + 1);
        signature = _sign(request);
        vm.prank(relay);
        vm.expectRevert(GasSponsor.RequestTooLong.selector);
        sponsor.execute(request, signature);
    }

    function test_onlyListedActionsOfListedRelays() public {
        GasSponsor.Request memory request = _request(abi.encodeCall(Guestbook.mark, (1)));
        bytes memory signature = _sign(request);
        bytes memory data = _transfer(1 ether);
        vm.expectRevert(GasSponsor.NotRelay.selector);
        sponsor.execute(request, signature);
        vm.expectRevert(GasSponsor.NotRelay.selector);
        sponsor.sponsor(address(token), data);
        vm.startPrank(relay);
        // A method that acts for its caller, an unlisted contract, a call too short for a method, and the wrong kind.
        vm.expectRevert(GasSponsor.ActionNotAllowed.selector);
        sponsor.sponsor(address(token), abi.encodeCall(TestToken3009.transfer, (shop, 1)));
        vm.expectRevert(GasSponsor.ActionNotAllowed.selector);
        sponsor.sponsor(address(sponsor), abi.encodeCall(GasSponsor.setRelay, (shop, true)));
        vm.expectRevert(GasSponsor.ActionNotAllowed.selector);
        sponsor.sponsor(address(token), hex"e3ee16");
        vm.expectRevert(GasSponsor.ActionNotAllowed.selector);
        sponsor.sponsor(address(book), abi.encodeCall(Guestbook.mark, (1)));
        vm.stopPrank();
        _allow(address(book), Guestbook.mark.selector, GasSponsor.Kind.NONE);
        vm.prank(relay);
        vm.expectRevert(GasSponsor.ActionNotAllowed.selector);
        sponsor.execute(request, signature);
    }

    function test_theOwnerKeepsTheLists() public {
        bytes4[] memory one = new bytes4[](1);
        vm.startPrank(relay);
        vm.expectRevert(GasSponsor.NotOwner.selector);
        sponsor.setRelay(shop, true);
        vm.expectRevert(GasSponsor.NotOwner.selector);
        sponsor.setActions(address(token), one, GasSponsor.Kind.SIGNED);
        vm.stopPrank();
        vm.expectRevert(GasSponsor.InvalidParameters.selector);
        sponsor.setRelay(address(0), true);
        vm.expectRevert(GasSponsor.InvalidParameters.selector);
        sponsor.setActions(address(sponsor), one, GasSponsor.Kind.USER);
        vm.expectRevert(GasSponsor.InvalidParameters.selector);
        sponsor.setActions(shop, one, GasSponsor.Kind.USER);
        vm.expectRevert(GasSponsor.InvalidParameters.selector);
        new GasSponsor(address(0));
    }

    function test_theTargetsRefusalComesBackAndTheRequestStaysUnused() public {
        GasSponsor.Request memory request = _request(abi.encodeCall(Guestbook.refuse, ()));
        bytes memory signature = _sign(request);
        vm.prank(relay);
        vm.expectRevert(abi.encodeWithSelector(Guestbook.Refused.selector, 7));
        sponsor.execute(request, signature);
        assertFalse(sponsor.used(user, request.nonce));
    }

    function test_aRelayCannotStarveTheAction() public {
        GasSponsor.Request memory request = _request(abi.encodeCall(Guestbook.burn, (400)));
        request.gas = 400_000;
        bytes memory signature = _sign(request);
        vm.prank(relay);
        vm.expectRevert(GasSponsor.NotEnoughGas.selector);
        sponsor.execute{gas: 300_000}(request, signature);
        _execute(request, signature);
        assertEq(book.marks(user), 1);
    }

    function test_theScriptListsWhatTheStoreRelays() public {
        TestUSDC3009 usdc = new TestUSDC3009(address(this));
        OrderEscrow escrow = new OrderEscrow(address(usdc), address(this), makeAddr("arbiter"));
        vm.setEnv("NU54_SPONSOR_OWNER", vm.toString(shop));
        vm.setEnv("NU54_SPONSOR_RELAY", vm.toString(relay));
        vm.setEnv("NU54_ORDER_ESCROW", vm.toString(address(escrow)));
        DeployGasSponsor script = new DeployGasSponsor();
        GasSponsor deployed = script.run();
        bytes4[] memory list = script.escrowActions();
        for (uint256 i; i < list.length; i++) {
            assertTrue(deployed.actions(address(escrow), list[i]) == GasSponsor.Kind.SIGNED);
        }
        assertTrue(
            deployed.actions(address(usdc), TestUSDC3009.transferWithAuthorization.selector) == GasSponsor.Kind.SIGNED
        );
        assertTrue(deployed.actions(address(escrow), OrderEscrow.resolve.selector) == GasSponsor.Kind.NONE);
        assertTrue(deployed.relays(relay) && !deployed.relays(shop));
        vm.chainId(8282);
        vm.expectRevert(bytes("not the StableNet test network"));
        script.run();
    }

    function test_anActionCannotComeBack() public {
        bytes memory data = _transfer(1 ether);
        sponsor.setRelay(address(book), true);
        vm.prank(relay);
        vm.expectRevert(GasSponsor.Reentered.selector);
        sponsor.sponsor(address(book), abi.encodeCall(Guestbook.reenter, (address(token), data)));
    }
}
