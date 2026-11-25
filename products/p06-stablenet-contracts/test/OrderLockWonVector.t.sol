// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Test} from "forge-std/Test.sol";
import {OrderEscrow} from "../src/OrderEscrow.sol";
import {PaymentTypesV2 as Types} from "../src/PaymentTypesV2.sol";
import {TestKRW3009} from "../src/test-token/TestKRW3009.sol";

/// @notice Pins what a board must sign for a won store against the real contracts. The dollar vectors
/// stay in OrderLockVector.t.sol; only the token differs here.
contract OrderLockWonVectorTest is Test {
    uint256 internal constant MERCHANT_KEY = 0xA11CE;
    uint256 internal constant WALLET_KEY = 0xB0A2D;
    // 65,000 won: the same units as the dollar vector, so the firmware payment test runs on both. The return is 34,200 won.
    address internal constant TOKEN = address(0x1001);
    address internal constant ESCROW = address(0x1002);
    address internal constant STORE = address(0x1003);
    address internal constant PHONE = address(0x1004);

    function testWonSignatureVectorPaysTheOrder() public {
        vm.chainId(8283);
        vm.warp(1_800_000_000);
        deployCodeTo("TestKRW3009.sol:TestKRW3009", abi.encode(address(this)), TOKEN);
        deployCodeTo("OrderEscrow.sol:OrderEscrow", abi.encode(TOKEN, address(this), address(this)), ESCROW);
        OrderEscrow escrow = OrderEscrow(ESCROW);
        TestKRW3009 token = TestKRW3009(TOKEN);
        address wallet = vm.addr(WALLET_KEY);
        escrow.configureMerchant(bytes32(uint256(7)), vm.addr(MERCHANT_KEY), STORE, 2 days, 7 days);
        token.mint(wallet, 10_000_000);

        Types.MerchantQuote memory q = Types.MerchantQuote({
            quoteId: bytes32(uint256(2)),
            tenantCommitment: bytes32(uint256(3)),
            branchCommitment: bytes32(uint256(4)),
            orderId: bytes32(uint256(5)),
            merchantId: bytes32(uint256(7)),
            claimCommitment: bytes32(uint256(8)),
            intentCommitment: bytes32(uint256(9)),
            payer: wallet,
            asset: TOKEN,
            beneficiary: STORE,
            amount: 6_500_000,
            nonce: 11,
            merchantKeyVersion: 1,
            beneficiaryVersion: 1,
            payerGeneration: 1,
            issuedAt: 1_799_999_990,
            expiresAt: 1_800_000_110,
            mode: 1,
            payerRole: 1
        });
        bytes memory unsigned = abi.encodePacked(abi.encode(uint256(8283), ESCROW), abi.encode(q));
        bytes memory merchantSig = sign(MERCHANT_KEY, escrow.quoteDigest(q));
        bytes memory preimage = abi.encodePacked(
            hex"1901",
            token.DOMAIN_SEPARATOR(),
            keccak256(
                abi.encode(
                    token.RECEIVE_WITH_AUTHORIZATION_TYPEHASH(),
                    wallet,
                    ESCROW,
                    q.amount,
                    uint256(0),
                    uint256(q.expiresAt),
                    escrow.lockNonce(q, PHONE)
                )
            )
        );
        bytes memory walletSig = sign(WALLET_KEY, keccak256(preimage));

        string memory expected = string.concat(
            "unsigned=",
            vm.toString(unsigned),
            "\nmerchantSignature=",
            vm.toString(merchantSig),
            "\nmerchantSigner=",
            vm.toString(abi.encodePacked(vm.addr(MERCHANT_KEY))),
            "\nconfirmer=",
            vm.toString(abi.encodePacked(PHONE)),
            "\npreimage=",
            vm.toString(preimage),
            "\npayerSignature=",
            vm.toString(walletSig),
            "\n"
        );
        assertEq(
            vm.readFile("../../packages/protocol/v2/testdata/order-lock-won.vectors"), expected, "regenerate the vector"
        );

        escrow.lock(q, PHONE, merchantSig, walletSig);
        assertEq(token.balanceOf(ESCROW), 6_500_000);
    }

    function testWonReturnVectorSendsTheMoneyHome() public {
        vm.chainId(8283);
        vm.warp(1_800_000_000);
        deployCodeTo("TestKRW3009.sol:TestKRW3009", abi.encode(address(this)), TOKEN);
        TestKRW3009 token = TestKRW3009(TOKEN);
        address wallet = vm.addr(WALLET_KEY);
        address home = address(0x1005);
        bytes32 nonce = bytes32(uint256(0x77));
        token.mint(wallet, 10_000_000);
        bytes memory preimage = abi.encodePacked(
            hex"1901",
            token.DOMAIN_SEPARATOR(),
            keccak256(
                abi.encode(
                    token.TRANSFER_WITH_AUTHORIZATION_TYPEHASH(),
                    wallet,
                    home,
                    uint256(3_420_000),
                    uint256(0),
                    uint256(1_800_000_500),
                    nonce
                )
            )
        );
        string memory expected = string.concat(
            "token=",
            vm.toString(abi.encodePacked(TOKEN)),
            "\nfrom=",
            vm.toString(abi.encodePacked(wallet)),
            "\nto=",
            vm.toString(abi.encodePacked(home)),
            "\nnonce=",
            vm.toString(nonce),
            "\npreimage=",
            vm.toString(preimage),
            "\n"
        );
        assertEq(
            vm.readFile("../../packages/protocol/v2/testdata/wallet-return-won.vectors"),
            expected,
            "regenerate the vector"
        );
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(WALLET_KEY, keccak256(preimage));
        token.transferWithAuthorization(wallet, home, 3_420_000, 0, 1_800_000_500, nonce, v, r, s);
        assertEq(token.balanceOf(home), 3_420_000);
    }

    function sign(uint256 key, bytes32 digest) internal pure returns (bytes memory) {
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(key, digest);
        return abi.encodePacked(r, s, v);
    }
}
