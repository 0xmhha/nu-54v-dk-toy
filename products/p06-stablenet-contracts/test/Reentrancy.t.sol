// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Test} from "forge-std/Test.sol";
import {PaymentSettlement} from "../src/PaymentSettlement.sol";
import {MerchantRegistry} from "../src/MerchantRegistry.sol";
import {PaymentTypes} from "../src/PaymentTypes.sol";

/// A token whose transfer calls back into cashOut, to show the balance is zeroed first.
contract ReentrantToken {
    mapping(address => uint256) public balanceOf;
    PaymentSettlement public target;
    address public reenterAs;
    uint256 public reentries;

    function setTarget(PaymentSettlement t, address merchant) external {
        target = t;
        reenterAs = merchant;
    }

    function mint(address to, uint256 amount) external {
        balanceOf[to] += amount;
    }

    function transferFrom(address from, address to, uint256 amount) external returns (bool) {
        balanceOf[from] -= amount;
        balanceOf[to] += amount;
        return true;
    }

    function transfer(address to, uint256 amount) external returns (bool) {
        balanceOf[msg.sender] -= amount;
        balanceOf[to] += amount;
        if (reentries == 0 && address(target) != address(0)) {
            reentries++;
            // The merchant contract re-enters cashOut while the first transfer runs.
            ReentrantMerchant(reenterAs).again();
        }
        return true;
    }
}

contract ReentrantMerchant {
    PaymentSettlement internal settlement;

    constructor(PaymentSettlement s) {
        settlement = s;
    }

    function cashOut() external {
        settlement.cashOut();
    }

    function again() external {
        settlement.cashOut();
    }
}

// P06-NFR-04
contract ReentrancyTest is Test {
    function test_reentrancy_cashOut() public {
        vm.warp(1_790_000_000);
        address operator = makeAddr("operator");
        address admin = makeAddr("admin");
        address payout = makeAddr("payout");
        uint256 deviceKey = 0xD1CE;
        address device = vm.addr(deviceKey);

        ReentrantToken token = new ReentrantToken();
        MerchantRegistry registry = new MerchantRegistry(admin);
        PaymentSettlement settlement =
            new PaymentSettlement(address(token), address(registry), operator, 50e6, 200e6, 3600, 120);
        ReentrantMerchant merchant = new ReentrantMerchant(settlement);
        vm.prank(admin);
        registry.registerMerchant(address(merchant), payout);

        token.mint(operator, 10e6);
        vm.prank(operator);
        settlement.depositFor(device, 10e6, operator);

        PaymentTypes.PaymentAuthorization memory a = PaymentTypes.PaymentAuthorization({
            chainId: block.chainid,
            contractAddress: address(settlement),
            merchant: address(merchant),
            payout: payout,
            token: address(token),
            amount: 5e6,
            orderId: bytes32(uint256(1)),
            nonce: 1,
            expiry: uint64(block.timestamp + 60)
        });
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
            vm.sign(deviceKey, keccak256(abi.encodePacked("\x19\x01", settlement.domainSeparator(), structHash)));
        settlement.settle(a, abi.encodePacked(r, s, v));

        token.setTarget(settlement, address(merchant));
        merchant.cashOut();

        assertEq(token.reentries(), 1, "the transfer re-entered cashOut");
        assertEq(token.balanceOf(payout), 5e6, "paid exactly once");
        assertEq(settlement.merchantBalance(address(merchant)), 0);
    }
}
