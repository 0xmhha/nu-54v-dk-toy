// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Script, console2} from "forge-std/Script.sol";
import {PaymentSettlement} from "../src/PaymentSettlement.sol";
import {TestUSDC} from "../src/test-token/TestUSDC.sol";

/// Adds the settlement contract to the test token's trusted-sender list, so cash-outs and
/// withdrawals reach recipients in refusal mode without a hold. Run once after Deploy.s.sol,
/// as the token owner:
///   NU54_SETTLEMENT=0x... script/testnet-forge.sh script/TrustSettlement.s.sol token-owner
///
/// Why the settlement contract is trusted: it only pays the addresses fixed by contract (the
/// merchant payout set by the registry admin, the renter's withdraw address set by the
/// operator). A payout that is rejected anyway comes back as surplus (recoverSurplus).
contract TrustSettlement is Script {
    function run() external {
        PaymentSettlement settlement = PaymentSettlement(vm.envAddress("NU54_SETTLEMENT"));
        TestUSDC token = TestUSDC(settlement.token());
        if (token.trustedSender(address(settlement))) {
            console2.log("already trusted", address(settlement));
            return;
        }
        vm.broadcast(token.owner());
        token.setTrustedSender(address(settlement), true);
        console2.log("trusted", address(settlement));
    }
}
