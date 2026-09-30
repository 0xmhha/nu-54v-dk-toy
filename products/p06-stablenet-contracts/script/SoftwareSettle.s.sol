// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Script, console2} from "forge-std/Script.sol";
import {PaymentSettlement} from "../src/PaymentSettlement.sol";
import {MerchantRegistry} from "../src/MerchantRegistry.sol";
import {TestUSDC} from "../src/test-token/TestUSDC.sol";
import {PaymentTypes} from "../src/PaymentTypes.sol";
import {IPaymentSettlement} from "../src/IPaymentSettlement.sol";

/// Software-signer settlement against an existing deployment (the week-6 contract gate path).
/// It registers a merchant, funds a device account and settles one payment signed by a
/// software device key instead of the board, then resubmits the same signature to show
/// that it ends in OrderAlreadyPaid.
///
/// Signers are chosen by address; the keys are loaded by forge, never read by this script:
///   testnet: script/testnet-forge.sh script/SoftwareSettle.s.sol registry-admin token-owner operator kiosk device
///   sandbox: forge script ... --private-keys <anvil keys for the same roles>
///
/// Environment (public values; `source ~/.nu54/testnet-accounts.env` provides the NU54_ADDR_* ones):
///   NU54_SETTLEMENT                     deployed PaymentSettlement
///   NU54_ADDR_REGISTRY_ADMIN, NU54_ADDR_TOKEN_OWNER, NU54_ADDR_OPERATOR, NU54_ADDR_KIOSK, NU54_ADDR_DEVICE
///   NU54_MERCHANT, NU54_PAYOUT          merchant and payout addresses (default: the kiosk and the operator)
///   NU54_WITHDRAW                       device account's withdraw address (default: the operator)
contract SoftwareSettle is Script {
    uint256 internal constant AMOUNT = 45e5; // 4.5 test dollars
    uint256 internal constant DEPOSIT = 50e6;

    function run() external {
        PaymentSettlement settlement = PaymentSettlement(vm.envAddress("NU54_SETTLEMENT"));
        address kiosk = vm.envAddress("NU54_ADDR_KIOSK");
        address operator = vm.envAddress("NU54_ADDR_OPERATOR");
        address device = vm.envAddress("NU54_ADDR_DEVICE");
        address merchant = vm.envOr("NU54_MERCHANT", kiosk);
        address payout = vm.envOr("NU54_PAYOUT", operator);

        // Payouts from an untrusted settlement contract would wait as held transfers.
        require(TestUSDC(settlement.token()).trustedSender(address(settlement)), "run TrustSettlement.s.sol first");
        _registerMerchant(settlement, merchant, payout);
        _fund(settlement, operator, device);

        PaymentTypes.PaymentAuthorization memory a = _authorization(settlement, merchant, payout, device);
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(device, _digest(settlement, a)); // device wallet loaded by forge
        bytes memory sig = abi.encodePacked(r, s, v);

        vm.broadcast(kiosk);
        settlement.settle(a, sig);

        // Resubmitting the settled signature must end in OrderAlreadyPaid (checked locally, not broadcast).
        vm.prank(kiosk);
        try settlement.settle(a, sig) {
            revert("resubmission was accepted");
        } catch (bytes memory err) {
            require(bytes4(err) == IPaymentSettlement.OrderAlreadyPaid.selector, "resubmission reverted differently");
        }

        console2.log("device", device);
        console2.log("merchant", merchant);
        console2.log("amount", AMOUNT);
        console2.log("nonce", a.nonce);
        console2.logBytes32(a.orderId);
    }

    function _registerMerchant(PaymentSettlement settlement, address merchant, address payout) internal {
        MerchantRegistry registry = MerchantRegistry(settlement.registry());
        if (registry.isActive(merchant) && registry.payoutOf(merchant) == payout) return;
        vm.broadcast(vm.envAddress("NU54_ADDR_REGISTRY_ADMIN"));
        registry.registerMerchant(merchant, payout);
    }

    function _fund(PaymentSettlement settlement, address operator, address device) internal {
        if (settlement.balanceOf(device) >= AMOUNT) return;
        TestUSDC token = TestUSDC(settlement.token());
        vm.broadcast(vm.envAddress("NU54_ADDR_TOKEN_OWNER"));
        token.mint(operator, DEPOSIT);
        vm.startBroadcast(operator);
        token.approve(address(settlement), DEPOSIT);
        settlement.depositFor(device, DEPOSIT, vm.envOr("NU54_WITHDRAW", operator));
        vm.stopBroadcast();
    }

    function _authorization(PaymentSettlement settlement, address merchant, address payout, address device)
        internal
        view
        returns (PaymentTypes.PaymentAuthorization memory)
    {
        return PaymentTypes.PaymentAuthorization({
            chainId: block.chainid,
            contractAddress: address(settlement),
            merchant: merchant,
            payout: payout,
            token: settlement.token(),
            amount: AMOUNT,
            orderId: keccak256(abi.encode("nu54-software-settle", block.timestamp)),
            nonce: _nextNonce(settlement, device),
            expiry: uint64(block.timestamp + 100)
        });
    }

    /// @dev Sequential nonces from a per-device start that is a multiple of 256, as the device
    /// does (payment-protocol 2): consecutive nonces share one bitmap slot, which saves gas.
    function _nextNonce(PaymentSettlement settlement, address device) internal view returns (uint256 n) {
        n = uint256(keccak256(abi.encode("nu54-nonce-start", device))) & ~uint256(0xff);
        while (settlement.isNonceUsed(device, n)) n++;
    }

    function _digest(PaymentSettlement settlement, PaymentTypes.PaymentAuthorization memory a)
        internal
        view
        returns (bytes32)
    {
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
        return keccak256(abi.encodePacked("\x19\x01", settlement.domainSeparator(), structHash));
    }
}
