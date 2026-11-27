// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {IsolationTest, Test} from "./Isolation.t.sol";
import {LabToken} from "../src/LabToken.sol";
import {Market} from "../src/Market.sol";

contract TradeTest is IsolationTest {
    address constant USER = address(20);
    uint256 constant BASE = 100_000_000;

    function _prepare() internal {
        vm.prank(PUBLISHER_A);
        market.observe(PRICE, block.timestamp);
        vm.prank(PUBLISHER_B);
        market.observe(PRICE, block.timestamp);
        vm.startPrank(ADMIN);
        market.base().mint(USER, 2_000 * BASE);
        market.quoteToken().mint(USER, 2_000 * PRICE);
        vm.stopPrank();
        vm.startPrank(USER);
        market.base().approve(address(market), type(uint256).max);
        market.quoteToken().approve(address(market), type(uint256).max);
        vm.stopPrank();
    }

    function _execute(Market.Quote memory order) internal returns (uint256, uint256) {
        bytes memory signature = _sign(market, order);
        vm.prank(USER);
        return market.execute(order, signature);
    }

    function _sign(Market venue, Market.Quote memory order) internal view returns (bytes memory) {
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(SIGNER_KEY, venue.quoteDigest(order));
        return abi.encodePacked(r, s, v);
    }

    function _order(Market.Action action, uint256 amount, uint256 other, uint256 minA, uint256 minB, uint256 nonce)
        internal
        view
        returns (Market.Quote memory)
    {
        return Market.Quote(USER, action, amount, other, minA, minB, market.priceVersion(), nonce, block.timestamp + 60);
    }

    function testLiquiditySwapAndExitActualBalances() public {
        _prepare();
        uint256 baseBefore = market.base().balanceOf(USER);
        uint256 quoteBefore = market.quoteToken().balanceOf(USER);
        _execute(_order(Market.Action.AddLiquidity, 1_000 * BASE, 1_000 * PRICE, 1_000 * BASE, 0, 1));
        assertEq(market.shares(USER), 1_000 * BASE);
        (uint256 out,) = _execute(_order(Market.Action.SwapBase, 10 * BASE, 0, 11_000_000, 0, 2));
        assertEq(out, 11_845_896);
        assertEq(market.quoteToken().balanceOf(USER), quoteBefore - 1_000 * PRICE + out);
        (uint256 redeemedBase, uint256 redeemedQuote) =
            _execute(_order(Market.Action.RemoveLiquidity, 1_000 * BASE, 0, 1_010 * BASE, 1_000 * PRICE - out, 3));
        assertEq(redeemedBase, 1_010 * BASE);
        assertEq(redeemedQuote, 1_000 * PRICE - out);
        assertEq(market.base().balanceOf(USER), baseBefore);
        assertEq(market.quoteToken().balanceOf(USER), quoteBefore);
        assertEq(market.shares(USER), 0);
    }

    function testFxDirectionPrecisionConsentAndExpiration() public {
        _prepare();
        Market.Quote memory order = _order(Market.Action.FxBase, BASE + 55, 0, PRICE, 0, 1);
        bytes memory signature = _sign(market, order);
        uint256 balance = market.base().balanceOf(USER);
        (uint256 out,) = _execute(order);
        assertEq(out, PRICE);
        assertEq(market.base().balanceOf(USER), balance);
        vm.prank(USER);
        vm.expectRevert(Market.InvalidQuote.selector);
        market.execute(order, signature);
        order.nonce = 2;
        signature = _sign(market, order);
        order.action = Market.Action.FxQuote;
        vm.prank(USER);
        vm.expectRevert(Market.InvalidQuote.selector);
        market.execute(order, signature);
        vm.warp(block.timestamp + 61);
        signature = _sign(market, order);
        vm.prank(USER);
        vm.expectRevert(Market.InvalidQuote.selector);
        market.execute(order, signature);
        (out,) = _execute(_order(Market.Action.FxQuote, PRICE - 1, 0, 99_999_900, 0, 3));
        assertEq(out, 99_999_916);
    }

    function testFundingLiquidationAndManualCloseRace() public {
        _prepare();
        LabToken token = market.quoteToken();
        vm.prank(ADMIN);
        token.mint(ADMIN, 100_000_000);
        vm.startPrank(ADMIN);
        market.quoteToken().approve(address(market), 100_000_000);
        market.seedRisk(100_000_000);
        vm.stopPrank();
        (uint256 position,) = _execute(_order(Market.Action.OpenLong, 9_600_000, 8 * BASE, 0, 0, 10));
        assertEq(position, 1);

        vm.warp(7501);
        _update(PRICE);
        uint256 version = market.priceVersion();
        vm.expectRevert(Market.Forbidden.selector);
        market.fund(position, 1, 25, version);
        vm.prank(KEEPER);
        market.fund(position, 1, 25, version);
        vm.prank(KEEPER);
        vm.expectRevert(Market.PositionChanged.selector);
        market.fund(position, 1, 25, version);

        vm.warp(7502);
        _update(1_000);
        version = market.priceVersion();
        Market.Quote memory close = _order(Market.Action.Close, position, 2, 0, 0, 11);
        bytes memory signature = _sign(market, close);
        vm.prank(KEEPER);
        market.liquidate(position, 2, version);
        vm.prank(USER);
        vm.expectRevert(Market.PositionChanged.selector);
        market.execute(close, signature);
        assertFalse(market.used(USER, close.nonce));
        assertEq(market.lockedRisk(), 0);
        assertEq(market.quoteToken().balanceOf(address(market)), market.riskReserve());

        (position,) = _execute(_order(Market.Action.OpenShort, 10_000_000, 1_000 * BASE, 0, 0, 12));
        _execute(_order(Market.Action.Close, position, 1, 10_000_000, 0, 13));
        vm.prank(KEEPER);
        vm.expectRevert(Market.PositionChanged.selector);
        market.liquidate(position, 1, version);
    }

    function _update(uint256 value) private {
        vm.prank(PUBLISHER_A);
        market.observe(value, block.timestamp);
        vm.prank(PUBLISHER_B);
        market.observe(value, block.timestamp);
    }

    function testNonExecutableBoundsWrongDeploymentAndPaused() public {
        _prepare();
        Market.Quote memory order = _order(Market.Action.FxBase, BASE, 0, PRICE + 1, 0, 1);
        bytes memory signature = _sign(market, order);
        vm.prank(USER);
        vm.expectRevert(Market.Limit.selector);
        market.execute(order, signature);
        assertFalse(market.used(USER, 1));

        Market other = new Market(ADMIN, vm.addr(SIGNER_KEY), PUBLISHER_A, PUBLISHER_B, KEEPER);
        order.minA = PRICE;
        signature = _sign(other, order);
        vm.prank(USER);
        vm.expectRevert(Market.InvalidQuote.selector);
        market.execute(order, signature);
        vm.prank(ADMIN);
        market.setPaused(true);
        signature = _sign(market, order);
        vm.prank(USER);
        vm.expectRevert(Market.InvalidQuote.selector);
        market.execute(order, signature);
    }
}
