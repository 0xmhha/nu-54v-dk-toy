// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Script, console2} from "forge-std/Script.sol";
import {PaymentSettlement} from "../src/PaymentSettlement.sol";
import {MerchantRegistry} from "../src/MerchantRegistry.sol";
import {TestUSDC} from "../src/test-token/TestUSDC.sol";
import {PaymentTypes} from "../src/PaymentTypes.sol";

/// Software-signer settlement against an existing deployment (the W6 contract gate path).
/// It registers a merchant, funds a device account and settles one payment signed by a
/// software key instead of the board.
///
/// Environment:
///   NU54_SETTLEMENT                     deployed PaymentSettlement
///   NU54_MERCHANT, NU54_PAYOUT          merchant and payout addresses
///   NU54_WITHDRAW                       device account's withdraw address
///   NU54_REGISTRY_ADMIN_KEY, NU54_OPERATOR_KEY, NU54_TOKEN_OWNER_KEY, NU54_KIOSK_KEY, NU54_DEVICE_KEY
///       private keys. Use them only for the local sandbox (anvil keys) or with a
///       throwaway software device key; never commit them.
contract SoftwareSettle is Script {
    function run() external {
        PaymentSettlement settlement = PaymentSettlement(vm.envAddress("NU54_SETTLEMENT"));
        MerchantRegistry registry = MerchantRegistry(settlement.registry());
        TestUSDC token = TestUSDC(settlement.token());
        address merchant = vm.envAddress("NU54_MERCHANT");
        address payout = vm.envAddress("NU54_PAYOUT");
        uint256 deviceKey = vm.envUint("NU54_DEVICE_KEY");
        address device = vm.addr(deviceKey);
        uint256 amount = 45e5;

        vm.broadcast(vm.envUint("NU54_REGISTRY_ADMIN_KEY"));
        registry.registerMerchant(merchant, payout);

        vm.startBroadcast(vm.envUint("NU54_TOKEN_OWNER_KEY"));
        token.mint(vm.addr(vm.envUint("NU54_OPERATOR_KEY")), 50e6);
        vm.stopBroadcast();

        vm.startBroadcast(vm.envUint("NU54_OPERATOR_KEY"));
        token.approve(address(settlement), 50e6);
        settlement.depositFor(device, 50e6, vm.envAddress("NU54_WITHDRAW"));
        vm.stopBroadcast();

        PaymentTypes.PaymentAuthorization memory a = PaymentTypes.PaymentAuthorization({
            chainId: block.chainid,
            contractAddress: address(settlement),
            merchant: merchant,
            payout: payout,
            token: address(token),
            amount: amount,
            orderId: keccak256(abi.encode("nu54-software-settle", block.timestamp)),
            nonce: uint256(keccak256(abi.encode(block.timestamp, device))),
            expiry: uint64(block.timestamp + 100)
        });
        bytes32 structHash = keccak256(
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
        bytes32 d = keccak256(abi.encodePacked("\x19\x01", settlement.domainSeparator(), structHash));
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(deviceKey, d);

        vm.broadcast(vm.envUint("NU54_KIOSK_KEY"));
        settlement.settle(a, abi.encodePacked(r, s, v));

        console2.log("device", device);
        console2.logBytes32(a.orderId);
    }
}
