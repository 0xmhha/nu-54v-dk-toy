// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Test} from "forge-std/Test.sol";
import {MerchantRegistry} from "../src/MerchantRegistry.sol";
import {IMerchantRegistry} from "../src/IMerchantRegistry.sol";
import {IMerchantRegistryExtensions} from "../src/IMerchantRegistryExtensions.sol";

// P06-FR-17
contract MerchantRegistryTest is Test {
    MerchantRegistry internal registry;
    address internal admin = makeAddr("admin");
    address internal merchant = makeAddr("merchant");
    address internal payout = makeAddr("payout");

    function setUp() public {
        registry = new MerchantRegistry(admin, 86400);
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

    function test_payoutChange_delayed() public {
        vm.startPrank(admin);
        registry.registerMerchant(merchant, payout);
        address next = makeAddr("next");
        vm.expectEmit(true, false, false, true);
        emit IMerchantRegistryExtensions.PayoutChangeQueued(merchant, next, block.timestamp + 86400);
        registry.requestPayoutChange(merchant, next);
        vm.stopPrank();
        (address queued, uint256 at) = registry.pendingPayoutOf(merchant);
        assertEq(queued, next);
        assertEq(at, block.timestamp + 86400);
        vm.warp(block.timestamp + 86400 - 1);
        assertEq(registry.payoutOf(merchant), payout, "not before the delay");
        vm.warp(block.timestamp + 1);
        assertEq(registry.payoutOf(merchant), next);
        (queued,) = registry.pendingPayoutOf(merchant);
        assertEq(queued, address(0));
    }

    function test_payoutChange_cancel() public {
        vm.startPrank(admin);
        registry.registerMerchant(merchant, payout);
        registry.requestPayoutChange(merchant, makeAddr("next"));
        registry.cancelPayoutChange(merchant);
        vm.expectRevert(IMerchantRegistryExtensions.NoPendingPayoutChange.selector);
        registry.cancelPayoutChange(merchant);
        vm.stopPrank();
        vm.warp(block.timestamp + 86400);
        assertEq(registry.payoutOf(merchant), payout);
    }

    function test_payoutChange_cannotCancelAfterEffect() public {
        vm.startPrank(admin);
        registry.registerMerchant(merchant, payout);
        registry.requestPayoutChange(merchant, makeAddr("next"));
        vm.warp(block.timestamp + 86400);
        vm.expectRevert(IMerchantRegistryExtensions.NoPendingPayoutChange.selector);
        registry.cancelPayoutChange(merchant);
        vm.stopPrank();
    }

    function test_payoutChange_queueAfterEffectKeepsEffectivePayout() public {
        address second = makeAddr("second");
        vm.startPrank(admin);
        registry.registerMerchant(merchant, payout);
        registry.requestPayoutChange(merchant, second);
        vm.warp(block.timestamp + 86400);
        registry.requestPayoutChange(merchant, makeAddr("third"));
        vm.stopPrank();
        assertEq(registry.payoutOf(merchant), second, "the change in effect is kept while the next one waits");
    }

    // The review finding: re-registering must not change a payout without the delay.
    function test_register_cannotChangePayoutImmediately() public {
        vm.startPrank(admin);
        registry.registerMerchant(merchant, payout);
        vm.expectRevert(IMerchantRegistryExtensions.PayoutChangeRequired.selector);
        registry.registerMerchant(merchant, makeAddr("attacker"));
        registry.revokeMerchant(merchant);
        vm.expectRevert(IMerchantRegistryExtensions.PayoutChangeRequired.selector);
        registry.registerMerchant(merchant, makeAddr("attacker"));
        registry.registerMerchant(merchant, payout); // re-activation with the same payout
        vm.stopPrank();
        assertTrue(registry.isActive(merchant));
        assertEq(registry.payoutOf(merchant), payout);
    }

    function test_registry_unknownMerchant() public {
        vm.startPrank(admin);
        vm.expectRevert(IMerchantRegistryExtensions.UnknownMerchant.selector);
        registry.revokeMerchant(merchant);
        vm.expectRevert(IMerchantRegistryExtensions.UnknownMerchant.selector);
        registry.requestPayoutChange(merchant, payout);
        vm.stopPrank();
    }

    function test_registry_payoutChangeOnlyAdmin() public {
        vm.prank(admin);
        registry.registerMerchant(merchant, payout);
        vm.expectRevert(IMerchantRegistry.NotRegistryAdmin.selector);
        registry.requestPayoutChange(merchant, makeAddr("next"));
        vm.expectRevert(IMerchantRegistryExtensions.NotAdminOrMerchant.selector);
        registry.cancelPayoutChange(merchant);
    }

    // Residual risk closed: a stolen admin key alone cannot redirect a payout.
    function test_payoutChange_merchantVeto() public {
        vm.startPrank(admin);
        registry.registerMerchant(merchant, payout);
        registry.requestPayoutChange(merchant, makeAddr("attacker"));
        vm.stopPrank();
        vm.prank(merchant);
        registry.cancelPayoutChange(merchant);
        vm.warp(block.timestamp + 86400);
        assertEq(registry.payoutOf(merchant), payout);
    }

    function test_registry_constructorChecks() public {
        vm.expectRevert(IMerchantRegistryExtensions.InvalidParameters.selector);
        new MerchantRegistry(admin, 0);
        vm.expectRevert(IMerchantRegistry.ZeroAddress.selector);
        new MerchantRegistry(address(0), 86400);
    }

    function test_merchantStatus_matchesSeparateReads() public {
        vm.startPrank(admin);
        registry.registerMerchant(merchant, payout);
        registry.requestPayoutChange(merchant, makeAddr("next"));
        vm.stopPrank();
        for (uint256 i = 0; i < 2; i++) {
            (bool active, address p) = registry.merchantStatus(merchant);
            assertEq(active, registry.isActive(merchant));
            assertEq(p, registry.payoutOf(merchant));
            vm.warp(block.timestamp + 86400);
        }
        vm.prank(admin);
        registry.revokeMerchant(merchant);
        (bool stillActive,) = registry.merchantStatus(merchant);
        assertFalse(stillActive);
    }
}
