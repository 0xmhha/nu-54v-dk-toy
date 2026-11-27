// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Test} from "forge-std/Test.sol";
import {OrderEscrow} from "../src/OrderEscrow.sol";
import {PaymentTypesV2 as Types} from "../src/PaymentTypesV2.sol";
import {NativeCoinStandIn} from "./NativeCoinStandIn.sol";

/// @notice StableNet can block an account of its own coin. An order's money then waits in the order contract: no state
/// is lost, and the same call works once the account is free again. Nothing here can move it anywhere else.
contract OrderEscrowCoinTest is Test {
    uint256 internal constant MERCHANT_KEY = 0xA11CE;
    uint256 internal constant WALLET_KEY = 0xB0A2D;
    bytes32 internal constant MERCHANT = bytes32(uint256(7));
    bytes32 internal constant ORDER = bytes32(uint256(5));
    address internal constant STORE = address(0x1003);
    address internal constant PHONE = address(0x1004);
    uint256 internal constant PRICE = 6_500 ether;
    NativeCoinStandIn internal constant COIN = NativeCoinStandIn(address(0x1000));
    OrderEscrow internal escrow;
    address internal wallet;

    function setUp() public {
        vm.chainId(8283);
        vm.warp(1_800_000_000);
        vm.etch(address(COIN), type(NativeCoinStandIn).runtimeCode);
        escrow = new OrderEscrow(address(COIN), address(this), address(this));
        wallet = vm.addr(WALLET_KEY);
        escrow.configureMerchant(MERCHANT, vm.addr(MERCHANT_KEY), STORE, 2 days, 7 days);
        COIN.mint(wallet, 10_000 ether);
    }

    function testABlockedPayerCannotLockAnOrder() public {
        COIN.setBlacklisted(wallet, true);
        vm.expectRevert("NativeCoinAdapter: account is blacklisted");
        this.lock();
        assertEq(escrow.totalHeld(), 0);
        COIN.setBlacklisted(wallet, false);
        this.lock();
        assertEq(COIN.balanceOf(address(escrow)), PRICE);
    }

    function testARefundToABlockedPayerWaits() public {
        this.lock();
        vm.warp(block.timestamp + 8 days); // past the time to deliver
        COIN.setBlacklisted(wallet, true);
        vm.expectRevert("NativeCoinAdapter: account is blacklisted");
        escrow.refundUndelivered(MERCHANT, ORDER);
        assertEq(escrow.totalHeld(), PRICE);
        assertEq(COIN.balanceOf(address(escrow)), PRICE);
        COIN.setBlacklisted(wallet, false);
        escrow.refundUndelivered(MERCHANT, ORDER);
        assertEq(COIN.balanceOf(wallet), 10_000 ether);
        assertEq(escrow.totalHeld(), 0);
    }

    /// One payment covers every order of a batch, so a blocked store receives none of them until it is free.
    function testABatchToABlockedStoreWaits() public {
        this.lock();
        bytes32 delivered = escrow.actionDigest(escrow.DELIVERY_TYPEHASH(), MERCHANT, ORDER);
        escrow.markDelivered(MERCHANT, ORDER, sign(MERCHANT_KEY, delivered));
        vm.warp(block.timestamp + 8 days); // past the time to answer
        bytes32[] memory orders = new bytes32[](1);
        orders[0] = ORDER;
        COIN.setBlacklisted(STORE, true);
        vm.expectRevert("NativeCoinAdapter: account is blacklisted");
        escrow.claimBatch(MERCHANT, STORE, orders);
        assertEq(escrow.totalHeld(), PRICE);
        COIN.setBlacklisted(STORE, false);
        assertEq(escrow.claimBatch(MERCHANT, STORE, orders), PRICE);
        assertEq(COIN.balanceOf(STORE), PRICE);
    }

    function testABlockedOrderContractStopsEveryPayment() public {
        COIN.setBlacklisted(address(escrow), true);
        vm.expectRevert("NativeCoinAdapter: account is blacklisted");
        this.lock();
        assertEq(COIN.balanceOf(wallet), 10_000 ether);
    }

    function testASignatureForAnotherNameAtTheCoinsAddressIsRefused() public {
        string[4] memory names = ["KRWC", "NU54 Test KRW", "NU54 Test USD", "WKRC "];
        for (uint256 i; i < names.length; ++i) {
            vm.expectRevert("NativeCoinAdapter: invalid signature");
            this.lockAs(names[i]);
        }
        assertEq(COIN.balanceOf(wallet), 10_000 ether);
        this.lockAs("WKRC");
        assertEq(escrow.totalHeld(), PRICE);
    }

    function lock() external {
        this.lockAs("WKRC");
    }

    function lockAs(string calldata name) external {
        Types.MerchantQuote memory q = Types.MerchantQuote({
            quoteId: bytes32(uint256(2)),
            tenantCommitment: bytes32(uint256(3)),
            branchCommitment: bytes32(uint256(4)),
            orderId: ORDER,
            merchantId: MERCHANT,
            claimCommitment: bytes32(uint256(8)),
            intentCommitment: bytes32(uint256(9)),
            payer: wallet,
            asset: address(COIN),
            beneficiary: STORE,
            amount: PRICE,
            nonce: 11,
            merchantKeyVersion: 1,
            beneficiaryVersion: 1,
            payerGeneration: 1,
            issuedAt: uint64(block.timestamp - 10),
            expiresAt: uint64(block.timestamp + 110),
            mode: 1,
            payerRole: 1
        });
        bytes32 content = keccak256(
            abi.encode(
                COIN.RECEIVE_WITH_AUTHORIZATION_TYPEHASH(),
                wallet,
                address(escrow),
                q.amount,
                uint256(0),
                uint256(q.expiresAt),
                escrow.lockNonce(q, PHONE)
            )
        );
        bytes32 domain = keccak256(
            abi.encode(
                keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"),
                keccak256(bytes(name)),
                keccak256("1"),
                block.chainid,
                address(COIN)
            )
        );
        bytes memory payer = sign(WALLET_KEY, keccak256(abi.encodePacked(hex"1901", domain, content)));
        escrow.lock(q, PHONE, sign(MERCHANT_KEY, escrow.quoteDigest(q)), payer);
    }

    function sign(uint256 key, bytes32 digest) internal pure returns (bytes memory) {
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(key, digest);
        return abi.encodePacked(r, s, v);
    }
}
