// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Test} from "forge-std/Test.sol";
import {PaymentSettlement} from "../src/PaymentSettlement.sol";
import {MerchantRegistry} from "../src/MerchantRegistry.sol";
import {TestUSDC} from "../src/test-token/TestUSDC.sol";
import {PaymentTypes} from "../src/PaymentTypes.sol";

/// Drives deposits, payments (including resubmissions), withdrawals, closing and cash-outs with random inputs.
contract SettlementHandler is Test {
    PaymentSettlement public settlement;
    TestUSDC public token;
    address public operator;
    address public merchant;
    uint256[3] internal keys = [uint256(0xA1), 0xA2, 0xA3];
    PaymentTypes.PaymentAuthorization[] internal settledAuths;
    bytes[] internal settledSigs;
    mapping(bytes32 => uint256) public settledCount;
    bytes32[] public orders;
    uint256 public settledOnClosed; // must stay 0

    constructor(PaymentSettlement s, TestUSDC t, address op, address m) {
        settlement = s;
        token = t;
        operator = op;
        merchant = m;
    }

    function devices(uint256 i) public view returns (address) {
        return vm.addr(keys[i]);
    }

    function deposit(uint256 who, uint256 amount) external {
        amount = bound(amount, 1, 100e6);
        address dev = devices(who % 3);
        vm.startPrank(operator);
        token.mint(operator, amount);
        token.approve(address(settlement), amount);
        try settlement.depositFor(dev, amount, operator) {} catch {} // closed accounts refuse
        vm.stopPrank();
    }

    function pay(uint256 who, uint256 amount, uint256 order, uint256 nonce) external {
        uint256 key = keys[who % 3];
        PaymentTypes.PaymentAuthorization memory a = PaymentTypes.PaymentAuthorization({
            chainId: block.chainid,
            contractAddress: address(settlement),
            merchant: merchant,
            payout: merchant,
            token: address(token),
            amount: bound(amount, 1, 50e6),
            orderId: bytes32(order % 16), // few order ids, so duplicates happen
            nonce: nonce,
            expiry: uint64(block.timestamp + 60)
        });
        bytes memory sig = _sign(key, a);
        bool wasClosed = settlement.accountOf(vm.addr(key)).closedAt != 0;
        try settlement.settle(a, sig) {
            if (wasClosed) settledOnClosed++;
            if (settledCount[a.orderId] == 0) orders.push(a.orderId);
            settledCount[a.orderId]++;
            settledAuths.push(a);
            settledSigs.push(sig);
        } catch {}
    }

    function resubmit(uint256 i) external {
        if (settledAuths.length == 0) return;
        i = i % settledAuths.length;
        try settlement.settle(settledAuths[i], settledSigs[i]) {
            settledCount[settledAuths[i].orderId]++; // must never happen
        } catch (bytes memory err) {
            assertEq(bytes4(err), bytes4(keccak256("OrderAlreadyPaid()")), "resubmission must be OrderAlreadyPaid");
        }
    }

    function cashOut() external {
        vm.prank(merchant);
        settlement.cashOut();
    }

    function requestWithdrawal(uint256 who, uint256 amount) external {
        vm.prank(operator);
        try settlement.requestWithdrawal(devices(who % 3), bound(amount, 1, 200e6)) {} catch {}
    }

    function cancelWithdrawal(uint256 who) external {
        vm.prank(operator);
        try settlement.cancelWithdrawal(devices(who % 3)) {} catch {}
    }

    function closeAccount(uint256 who) external {
        vm.prank(operator);
        try settlement.closeAccount(devices(who % 3)) {} catch {}
    }

    function donateAndRecover(uint256 amount) external {
        amount = bound(amount, 1, 10e6);
        vm.prank(operator);
        token.mint(address(settlement), amount); // tokens arriving outside any account
        uint256 spare = settlement.surplus();
        vm.prank(operator);
        settlement.recoverSurplus(operator, spare);
    }

    function executeWithdrawal(uint256 who) external {
        try settlement.executeWithdrawal(devices(who % 3)) {} catch {}
    }

    function warp(uint256 secs) external {
        vm.warp(block.timestamp + bound(secs, 1, 2 days));
    }

    function orderCount() external view returns (uint256) {
        return orders.length;
    }

    function _sign(uint256 key, PaymentTypes.PaymentAuthorization memory a) internal view returns (bytes memory) {
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
        (uint8 v, bytes32 r, bytes32 s) =
            vm.sign(key, keccak256(abi.encodePacked("\x19\x01", settlement.domainSeparator(), structHash)));
        return abi.encodePacked(r, s, v);
    }
}

/// Design 8 invariants: the contract always holds what it owes, and an order settles at most once.
contract SettlementInvariantTest is Test {
    SettlementHandler internal handler;
    PaymentSettlement internal settlement;
    TestUSDC internal token;
    address internal merchant = makeAddr("merchant");

    function setUp() public {
        vm.warp(1_790_000_000);
        address operator = makeAddr("operator");
        address admin = makeAddr("admin");
        MerchantRegistry registry = new MerchantRegistry(admin, 86400);
        token = new TestUSDC(operator); // the handler mints as the operator
        settlement = new PaymentSettlement(address(token), address(registry), operator, 50e6, 200e6, 3600, 120);
        vm.prank(admin);
        registry.registerMerchant(merchant, merchant);
        handler = new SettlementHandler(settlement, token, operator, merchant);
        targetContract(address(handler));
    }

    function invariant_holdsWhatItOwes() public view {
        uint256 owed = settlement.merchantBalance(merchant);
        for (uint256 i = 0; i < 3; i++) {
            owed += settlement.balanceOf(handler.devices(i));
        }
        assertLe(owed, token.balanceOf(address(settlement)));
    }

    function invariant_totalOwedIsTheSumOfBalances() public view {
        uint256 owed = settlement.merchantBalance(merchant);
        for (uint256 i = 0; i < 3; i++) {
            owed += settlement.balanceOf(handler.devices(i));
        }
        assertEq(settlement.totalOwed(), owed);
        assertLe(settlement.totalOwed(), token.balanceOf(address(settlement)));
    }

    function invariant_closedAccountNeverSettles() public view {
        assertEq(handler.settledOnClosed(), 0);
    }

    function invariant_orderSettlesAtMostOnce() public view {
        for (uint256 i = 0; i < handler.orderCount(); i++) {
            assertLe(handler.settledCount(handler.orders(i)), 1);
        }
    }
}
