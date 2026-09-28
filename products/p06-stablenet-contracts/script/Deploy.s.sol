// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Script, console2} from "forge-std/Script.sol";
import {PaymentSettlement} from "../src/PaymentSettlement.sol";
import {MerchantRegistry} from "../src/MerchantRegistry.sol";
import {TestUSDC} from "../src/test-token/TestUSDC.sol";

/// Deploys the test token, the merchant registry and the settlement contract.
///
/// Roles come from the environment; the broadcasting key is chosen on the command line
/// (keystore for the testnet, an anvil key only for the local sandbox):
///   NU54_OPERATOR, NU54_REGISTRY_ADMIN, NU54_TOKEN_OWNER  role addresses
///
///   forge script script/Deploy.s.sol --rpc-url sandbox --broadcast --private-key <anvil key>
///   forge script script/Deploy.s.sol --rpc-url stablenet_testnet --broadcast \
///       --keystore $NU54_DEPLOYER_KEYSTORE --password "$(printenv $NU54_DEPLOYER_PASSWORD_ENV)"
///
/// Parameters are read from the design register so the deployment cannot drift from it;
/// caps are converted to token base units (6 decimals).
contract Deploy is Script {
    string internal constant REGISTER = "../../docs/content/planning/design-freeze-checkpoint-02.json";
    uint256 internal constant TOKEN_UNIT = 1e6;

    function run() external returns (TestUSDC token, MerchantRegistry registry, PaymentSettlement settlement) {
        address operator = vm.envAddress("NU54_OPERATOR");
        address registryAdmin = vm.envAddress("NU54_REGISTRY_ADMIN");
        address tokenOwner = vm.envAddress("NU54_TOKEN_OWNER");

        string memory reg = vm.readFile(REGISTER);
        uint256 perPaymentCap = vm.parseJsonUint(reg, ".parameters.perPaymentCap.value") * TOKEN_UNIT;
        uint256 dailyCap = vm.parseJsonUint(reg, ".parameters.dailyCap.value") * TOKEN_UNIT;
        uint256 withdrawalDelay = vm.parseJsonUint(reg, ".parameters.withdrawalDelay.value");
        uint256 authExpiry = vm.parseJsonUint(reg, ".parameters.authorizationExpiry.value");

        vm.startBroadcast();
        token = new TestUSDC(tokenOwner);
        registry = new MerchantRegistry(registryAdmin);
        settlement = new PaymentSettlement(
            address(token), address(registry), operator, perPaymentCap, dailyCap, withdrawalDelay, authExpiry
        );
        vm.stopBroadcast();

        console2.log("chainId", block.chainid);
        console2.log("TestUSDC", address(token));
        console2.log("MerchantRegistry", address(registry));
        console2.log("PaymentSettlement", address(settlement));
        console2.log("perPaymentCap", perPaymentCap);
        console2.log("dailyCap", dailyCap);
    }
}
