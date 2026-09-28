// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Test} from "forge-std/Test.sol";
import {PaymentSettlement} from "../src/PaymentSettlement.sol";
import {MerchantRegistry} from "../src/MerchantRegistry.sol";
import {TestUSDC} from "../src/test-token/TestUSDC.sol";
import {PaymentTypes} from "../src/PaymentTypes.sol";

/// Shared deployment and signing helpers. Parameters are the register values in base units.
abstract contract SettlementBase is Test {
    uint256 internal constant PER_PAYMENT_CAP = 50e6;
    uint256 internal constant DAILY_CAP = 200e6;
    uint256 internal constant WITHDRAWAL_DELAY = 3600;
    uint256 internal constant AUTH_EXPIRY = 120;

    TestUSDC internal token;
    MerchantRegistry internal registry;
    PaymentSettlement internal settlement;

    address internal operator = makeAddr("operator");
    address internal registryAdmin = makeAddr("registryAdmin");
    address internal merchant = makeAddr("merchant");
    address internal payout = makeAddr("payout");
    address internal renter = makeAddr("renter");
    address internal kiosk = makeAddr("kiosk");
    uint256 internal deviceKey = 0xD1CE;
    address internal device;

    function setUp() public virtual {
        vm.warp(1_790_000_000);
        device = vm.addr(deviceKey);
        token = new TestUSDC(operator);
        registry = new MerchantRegistry(registryAdmin);
        settlement = new PaymentSettlement(
            address(token), address(registry), operator, PER_PAYMENT_CAP, DAILY_CAP, WITHDRAWAL_DELAY, AUTH_EXPIRY
        );
        vm.prank(registryAdmin);
        registry.registerMerchant(merchant, payout);
        _deposit(device, 150e6);
    }

    function _deposit(address dev, uint256 amount) internal {
        vm.startPrank(operator);
        token.mint(operator, amount);
        token.approve(address(settlement), amount);
        settlement.depositFor(dev, amount, renter);
        vm.stopPrank();
    }

    function _auth(uint256 amount, bytes32 orderId, uint256 nonce)
        internal
        view
        returns (PaymentTypes.PaymentAuthorization memory a)
    {
        a = PaymentTypes.PaymentAuthorization({
            chainId: block.chainid,
            contractAddress: address(settlement),
            merchant: merchant,
            payout: payout,
            token: address(token),
            amount: amount,
            orderId: orderId,
            nonce: nonce,
            expiry: uint64(block.timestamp + 60)
        });
    }

    function _structHash(PaymentTypes.PaymentAuthorization memory a) internal pure returns (bytes32) {
        return keccak256(
            abi.encode(
                PaymentTypes.PAYMENT_AUTHORIZATION_TYPEHASH,
                a.chainId,
                a.contractAddress,
                a.merchant,
                a.payout,
                a.token,
                a.amount,
                a.orderId,
                a.nonce,
                a.expiry
            )
        );
    }

    function _sign(uint256 key, PaymentTypes.PaymentAuthorization memory a) internal view returns (bytes memory) {
        bytes32 d = keccak256(abi.encodePacked("\x19\x01", settlement.domainSeparator(), _structHash(a)));
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(key, d);
        return abi.encodePacked(r, s, v);
    }

    function _settle(PaymentTypes.PaymentAuthorization memory a, bytes memory sig) internal {
        vm.prank(kiosk);
        settlement.settle(a, sig);
    }
}
