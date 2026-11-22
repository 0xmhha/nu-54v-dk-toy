// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Test} from "forge-std/Test.sol";
import {Market} from "../src/Market.sol";

contract IsolationTest is Test {
    address constant ADMIN = address(10);
    address constant PUBLISHER_A = address(11);
    address constant PUBLISHER_B = address(12);
    address constant KEEPER = address(13);
    uint256 constant SIGNER_KEY = 0x1234;
    uint256 constant PRICE = 1_200_000;
    Market market;

    function setUp() public {
        vm.chainId(8283);
        vm.warp(7205);
        market = new Market(ADMIN, vm.addr(SIGNER_KEY), PUBLISHER_A, PUBLISHER_B, KEEPER);
    }

    function testChainGuard() public {
        vm.chainId(1);
        address signer = vm.addr(SIGNER_KEY);
        vm.expectRevert(Market.WrongChain.selector);
        new Market(ADMIN, signer, PUBLISHER_A, PUBLISHER_B, KEEPER);
        vm.prank(PUBLISHER_A);
        vm.expectRevert(Market.WrongChain.selector);
        market.observe(PRICE, block.timestamp);
    }

    function testTwoIndependentFreshPricesAndFaultLatch() public {
        vm.expectRevert(Market.InvalidPrice.selector);
        market.validPrice();
        vm.prank(PUBLISHER_A);
        market.observe(PRICE, block.timestamp);
        vm.expectRevert(Market.InvalidPrice.selector);
        market.validPrice();
        vm.prank(PUBLISHER_B);
        market.observe(PRICE + 1000, block.timestamp);
        (uint256 version, uint256 price) = market.validPrice();
        assertEq(version, 2);
        assertEq(price, PRICE + 500);

        vm.warp(7206);
        vm.prank(PUBLISHER_A);
        market.observe(PRICE * 2, block.timestamp);
        vm.expectRevert(Market.InvalidPrice.selector);
        market.validPrice();

        vm.prank(PUBLISHER_B);
        market.observe(PRICE * 2, block.timestamp);
        (, price) = market.validPrice();
        assertEq(price, PRICE * 2);

        vm.warp(7327);
        vm.expectRevert(Market.InvalidPrice.selector);
        market.validPrice();
    }

    function testObservationRejectsUnauthorizedFutureAndDuplicate() public {
        vm.expectRevert(Market.Forbidden.selector);
        market.observe(PRICE, block.timestamp);
        vm.startPrank(PUBLISHER_A);
        vm.expectRevert(Market.InvalidObservation.selector);
        market.observe(PRICE, block.timestamp + 1);
        market.observe(PRICE, block.timestamp);
        vm.expectRevert(Market.InvalidObservation.selector);
        market.observe(PRICE, block.timestamp);
    }
}
