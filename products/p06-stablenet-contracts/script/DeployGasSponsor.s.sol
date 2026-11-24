// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Script, console2} from "forge-std/Script.sol";
import {GasSponsor} from "../src/GasSponsor.sol";
import {OrderEscrow} from "../src/OrderEscrow.sol";
import {TestUSDC3009} from "../src/test-token/TestUSDC3009.sol";

/// Deploys the contract that pays the gas of users' signed actions, and lists what it may run.
///   NU54_SPONSOR_OWNER  keeps the lists; must be the broadcasting account
///   NU54_SPONSOR_RELAY  the store's gas account
///   NU54_ORDER_ESCROW   the OrderEscrow of the store
/// The escrow's `resolve` is left out: it asks for the arbiter itself.
contract DeployGasSponsor is Script {
    function run() external returns (GasSponsor sponsor) {
        require(block.chainid == 8283, "not the StableNet test network");
        address owner = vm.envAddress("NU54_SPONSOR_OWNER");
        address relay = vm.envAddress("NU54_SPONSOR_RELAY");
        OrderEscrow escrow = OrderEscrow(vm.envAddress("NU54_ORDER_ESCROW"));
        require(owner != relay, "roles overlap");
        vm.startBroadcast(owner);
        sponsor = new GasSponsor(owner);
        sponsor.setRelay(relay, true);
        sponsor.setActions(address(escrow), escrowActions(), GasSponsor.Kind.SIGNED);
        bytes4[] memory transfer = new bytes4[](1);
        transfer[0] = TestUSDC3009.transferWithAuthorization.selector;
        sponsor.setActions(escrow.asset(), transfer, GasSponsor.Kind.SIGNED);
        vm.stopBroadcast();
        console2.log("GasSponsor", address(sponsor));
    }

    function escrowActions() public pure returns (bytes4[] memory list) {
        list = new bytes4[](9);
        list[0] = OrderEscrow.lock.selector;
        list[1] = OrderEscrow.markDelivered.selector;
        list[2] = OrderEscrow.release.selector;
        list[3] = OrderEscrow.withdrawDispute.selector;
        list[4] = OrderEscrow.dispute.selector;
        list[5] = OrderEscrow.refundExpiredDispute.selector;
        list[6] = OrderEscrow.cancel.selector;
        list[7] = OrderEscrow.refundUndelivered.selector;
        list[8] = OrderEscrow.claimBatch.selector;
    }
}
