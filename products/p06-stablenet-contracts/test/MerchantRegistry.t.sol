// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Test} from "forge-std/Test.sol";
import {MerchantRegistry} from "../src/MerchantRegistry.sol";
import {IMerchantRegistry} from "../src/IMerchantRegistry.sol";

// P06-FR-17 (register and revoke; payout changes arrive with WBS2-P06-03)
contract MerchantRegistryTest is Test {
    MerchantRegistry internal registry;
    address internal admin = makeAddr("admin");
    address internal merchant = makeAddr("merchant");
    address internal payout = makeAddr("payout");

    function setUp() public {
        registry = new MerchantRegistry(admin);
    }

    function test_registry_registerAndRevoke() public {
        vm.prank(admin);
        registry.registerMerchant(merchant, payout);
        assertTrue(registry.isActive(merchant));
        assertEq(registry.payoutOf(merchant), payout);
        vm.prank(admin);
        registry.revokeMerchant(merchant);
        assertFalse(registry.isActive(merchant));
    }

    function test_registry_onlyAdmin() public {
        vm.expectRevert(IMerchantRegistry.NotRegistryAdmin.selector);
        registry.registerMerchant(merchant, payout);
    }
}
