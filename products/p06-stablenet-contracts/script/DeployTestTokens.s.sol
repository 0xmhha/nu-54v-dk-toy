// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Script, console2} from "forge-std/Script.sol";
import {TestToken3009} from "../src/test-token/TestToken3009.sol";
import {TestUSDC3009} from "../src/test-token/TestUSDC3009.sol";

/// Deploys the wallet's test tokens on one public test network: tUSDC, tUSDT and tKRWC.
///   NU54_TOKEN_OWNER  the only account that mints
/// Refuses every chain that is not a test network of this product or the local sandbox.
contract DeployTestTokens is Script {
    function run() external returns (TestUSDC3009 usdc, TestToken3009 usdt, TestToken3009 krwc) {
        require(
            block.chainid == 8283 || block.chainid == 11155111 || block.chainid == 84532 || block.chainid == 31337,
            "not a test network"
        );
        address owner = vm.envAddress("NU54_TOKEN_OWNER");
        vm.startBroadcast();
        usdc = new TestUSDC3009(owner); // the one OrderEscrow and the board already know
        usdt = new TestToken3009(owner, "NU54 Test USDT", "tUSDT", 6);
        krwc = new TestToken3009(owner, "NU54 Test KRWC", "tKRWC", 18);
        vm.stopBroadcast();
        console2.log("chainId", block.chainid);
        console2.log("tUSDC", address(usdc));
        console2.log("tUSDT", address(usdt));
        console2.log("tKRWC", address(krwc));
    }
}
