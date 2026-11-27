// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Test} from "forge-std/Test.sol";
import {ChargeTypes} from "../src/charge/ChargeTypes.sol";
import {ChargeGovernance} from "../src/charge/ChargeGovernance.sol";
import {ChargeDeposit} from "../src/charge/ChargeDeposit.sol";
import {WonPool} from "../src/charge/WonPool.sol";
import {TestChargeUSDC} from "../src/test-token/TestChargeUSDC.sol";
import {TestKRW3009} from "../src/test-token/TestKRW3009.sol";

/// @notice Pins what every off-chain signer of a top-up must sign against the real contracts: the quote
/// server, the traveller's wallet, the deposit-confirmation key, two signers and the contesting payer. The
/// file is what Go and TypeScript signers are checked against; each line is `name=hex`. The same signatures
/// then run a whole top-up here, so a vector that the contracts would refuse cannot be written down.
contract ChargeVectorsTest is Test {
    string internal constant FILE = "test/vectors/charge.vectors";
    uint256 internal constant SOURCE_CHAIN = 84532;
    uint256 internal constant WON_CHAIN = 8283;
    uint256 internal constant RELAY_KEY = 0xA001;
    uint256 internal constant QUOTER_KEY = 0xA002;
    uint256 internal constant WITNESS_KEY = 0xA003;
    uint256 internal constant PAUSER_KEY = 0xA004;
    uint256 internal constant SIGNER1_KEY = 0xA005;
    uint256 internal constant SIGNER2_KEY = 0xA006;
    uint256 internal constant SIGNER3_KEY = 0xA007;
    uint256 internal constant PAYER_KEY = 0xB0A2D;
    address internal constant USDC = address(0x2001);
    address internal constant DEPOSIT = address(0x2002);
    address internal constant WON = address(0x2003);
    address internal constant POOL = address(0x2004);
    address internal constant BOARD = address(0x2005);
    address internal constant TREASURY = address(0x2006);
    /// The fixed test rate as of 2026-11-27: 1 USDC = 1,345.00 won.
    uint256 internal constant RATE = 134_500;
    uint256 internal constant NOW = 1_800_000_000;
    uint256 internal constant UNTIL = NOW + 1 days;
    bytes32 internal constant SOURCE_BLOCK = keccak256("source block");
    bytes32 internal constant WON_BLOCK = keccak256("won block");

    ChargeDeposit internal deposit = ChargeDeposit(DEPOSIT);
    WonPool internal pool = WonPool(payable(POOL));
    address internal payer;
    bytes32 internal id;

    struct Signed {
        bytes message; // the values of the signed struct, ABI-encoded in order
        bytes preimage; // 0x1901, the domain separator, the struct hash: its keccak256 is what is signed
        bytes signature; // r, s, v
    }

    function setUp() public {
        vm.warp(NOW);
        payer = vm.addr(PAYER_KEY);
        ChargeGovernance.Keys memory keys = ChargeGovernance.Keys({
            relay: vm.addr(RELAY_KEY),
            quoter: vm.addr(QUOTER_KEY),
            witness: vm.addr(WITNESS_KEY),
            pauser: vm.addr(PAUSER_KEY),
            signers: [vm.addr(SIGNER1_KEY), vm.addr(SIGNER2_KEY), vm.addr(SIGNER3_KEY)]
        });
        vm.chainId(SOURCE_CHAIN);
        deployCodeTo("TestChargeUSDC.sol:TestChargeUSDC", abi.encode(address(this)), USDC);
        TestChargeUSDC(USDC).mint(payer, 1_000e6);
        ChargeDeposit.Config memory depositValues = ChargeDeposit.Config({
            usdc: USDC,
            chainId: SOURCE_CHAIN,
            wonChainId: WON_CHAIN,
            pool: POOL,
            won: WON,
            minWon: 100_000,
            maxWon: 30_000_000,
            minRate: RATE,
            maxRate: RATE
        });
        deployCodeTo("ChargeDeposit.sol:ChargeDeposit", abi.encode(keys, depositValues), DEPOSIT);

        vm.chainId(WON_CHAIN);
        deployCodeTo("TestKRW3009.sol:TestKRW3009", abi.encode(address(this)), WON);
        WonPool.Config memory poolValues = WonPool.Config({
            asset: WON,
            assetDecimals: 2,
            assetIsCoin: false,
            treasury: TREASURY,
            chainId: WON_CHAIN,
            minWon: 100_000,
            minRate: RATE,
            maxRate: RATE,
            limits: WonPool.Limits({
                perPayout: 30_000_000,
                accountDaily: 100_000_000,
                totalDaily: 300_000_000,
                fastPerPayout: 5_000_000,
                fastDaily: 50_000_000
            }),
            origin: WonPool.Origin({chainId: SOURCE_CHAIN, deposit: DEPOSIT})
        });
        deployCodeTo("WonPool.sol:WonPool", abi.encode(keys, poolValues), POOL);
        TestKRW3009(WON).mint(POOL, 500_000_000);

        vm.chainId(SOURCE_CHAIN);
        id = deposit.quoteDigest(theQuote());
    }

    /// 30 USDC buy 40,350.00 won, small enough for the fast path.
    function theQuote() internal view returns (ChargeTypes.Quote memory) {
        return ChargeTypes.Quote({
            method: 1,
            usdc: USDC,
            payer: payer,
            usdcAmount: 30e6,
            wonChainId: WON_CHAIN,
            pool: POOL,
            won: WON,
            board: BOARD,
            wonAmount: 4_035_000,
            rate: RATE,
            feeWon: 0,
            expiresAt: uint64(NOW + 2 minutes),
            payBy: uint64(NOW + 2 minutes + 2 hours),
            number: bytes32(uint256(0x51))
        });
    }

    Signed internal quoted;
    Signed internal authorized;
    Signed internal confirmed;
    Signed internal paid;
    Signed internal refundOne;
    Signed internal refundTwo;
    Signed internal contested;

    function signOnTheSourceChain() internal {
        vm.chainId(SOURCE_CHAIN);
        bytes32 domain = deposit.domainSeparator();
        bytes memory place = abi.encode(SOURCE_CHAIN, DEPOSIT);
        ChargeTypes.Quote memory quote = theQuote();
        quoted = signedBy(QUOTER_KEY, domain, abi.encode(ChargeTypes.QUOTE_TYPEHASH, quote), place);
        authorized = signedBy(
            PAYER_KEY,
            TestChargeUSDC(USDC).DOMAIN_SEPARATOR(),
            abi.encode(
                TestChargeUSDC(USDC).RECEIVE_WITH_AUTHORIZATION_TYPEHASH(),
                payer,
                DEPOSIT,
                quote.usdcAmount,
                uint256(0),
                uint256(quote.expiresAt),
                id
            ),
            abi.encode(SOURCE_CHAIN, USDC)
        );
        paid = signedBy(
            WITNESS_KEY, domain, abi.encode(ChargeTypes.PAID_TYPEHASH, id, WON_BLOCK, uint256(0), UNTIL), place
        );
        bytes memory refund = abi.encode(
            ChargeTypes.ACTION_TYPEHASH, keccak256("refund"), keccak256(abi.encode(id, WON_BLOCK)), uint256(0), UNTIL
        );
        refundOne = signedBy(SIGNER1_KEY, domain, refund, place);
        refundTwo = signedBy(SIGNER2_KEY, domain, refund, place);
        // A contest answers a completion, so it is signed for the round after it.
        contested = signedBy(PAYER_KEY, domain, abi.encode(ChargeTypes.CONTEST_TYPEHASH, id, uint256(1), UNTIL), place);
    }

    function signOnThePoolChain() internal {
        vm.chainId(WON_CHAIN);
        confirmed = signedBy(
            WITNESS_KEY,
            pool.domainSeparator(),
            abi.encode(ChargeTypes.CONFIRMATION_TYPEHASH, id, SOURCE_BLOCK, true, UNTIL),
            abi.encode(WON_CHAIN, POOL)
        );
    }

    function file() internal view returns (string memory) {
        return
            string.concat(quoteLines(), authorizationLines(), poolLines(), paidLines(), refundLines(), contestLines());
    }

    function quoteLines() internal view returns (string memory) {
        return string.concat(
            line("quote", quoted.message),
            line("quotePreimage", quoted.preimage),
            line("depositId", abi.encodePacked(id)),
            line("quoteSigner", abi.encodePacked(vm.addr(QUOTER_KEY))),
            line("quoteSignature", quoted.signature)
        );
    }

    function authorizationLines() internal view returns (string memory) {
        return string.concat(
            line("authorization", authorized.message),
            line("authorizationPreimage", authorized.preimage),
            line("payer", abi.encodePacked(payer)),
            line("authorizationSignature", authorized.signature)
        );
    }

    function poolLines() internal view returns (string memory) {
        return string.concat(
            line("confirmation", confirmed.message),
            line("confirmationPreimage", confirmed.preimage),
            line("confirmationSigner", abi.encodePacked(vm.addr(WITNESS_KEY))),
            line("confirmationSignature", confirmed.signature)
        );
    }

    function paidLines() internal view returns (string memory) {
        return string.concat(
            line("paid", paid.message), line("paidPreimage", paid.preimage), line("paidSignature", paid.signature)
        );
    }

    function refundLines() internal view returns (string memory) {
        return string.concat(
            line("refund", refundOne.message),
            line("refundPreimage", refundOne.preimage),
            line("refundSigners", abi.encodePacked(vm.addr(SIGNER1_KEY), vm.addr(SIGNER2_KEY))),
            line("refundSignatures", abi.encodePacked(refundOne.signature, refundTwo.signature))
        );
    }

    function contestLines() internal view returns (string memory) {
        return string.concat(
            line("contest", contested.message),
            line("contestPreimage", contested.preimage),
            line("contestSignature", contested.signature)
        );
    }

    function testChargeVectorsMatchTheFileAndRunATopUp() public {
        signOnTheSourceChain();
        signOnThePoolChain();
        assertEq(vm.readFile(FILE), file(), "regenerate the vector");
        ChargeTypes.Quote memory quote = theQuote();

        // The deposit: anyone sends the quote with the two signatures.
        vm.chainId(SOURCE_CHAIN);
        assertEq(deposit.depositWithAuthorization(quote, quoted.signature, authorized.signature), id);
        assertEq(TestChargeUSDC(USDC).balanceOf(DEPOSIT), 30e6);

        // One way on: nothing is paid, and two signers refund after the deadline.
        uint256 snapshot = vm.snapshotState();
        vm.warp(quote.payBy + 2 hours);
        deposit.refundBySigners(id, WON_BLOCK, UNTIL, [refundOne.signature, refundTwo.signature]);
        assertEq(TestChargeUSDC(USDC).balanceOf(payer), 1_000e6);
        vm.revertToState(snapshot);

        // The other way on: the pool pays, the payout is recorded, and the payer contests it.
        vm.chainId(WON_CHAIN);
        vm.prank(vm.addr(RELAY_KEY));
        pool.pay(
            WonPool.Origin({chainId: SOURCE_CHAIN, deposit: DEPOSIT}),
            quote,
            quoted.signature,
            WonPool.Confirmation({fast: true, sourceBlockHash: SOURCE_BLOCK, until: UNTIL}),
            confirmed.signature
        );
        assertEq(TestKRW3009(WON).balanceOf(BOARD), 4_035_000);
        vm.chainId(SOURCE_CHAIN);
        vm.prank(vm.addr(RELAY_KEY));
        deposit.complete(id, WON_BLOCK, UNTIL, paid.signature);
        deposit.contest(id, UNTIL, contested.signature);
        assertEq(uint8(deposit.depositOf(id).state), uint8(ChargeDeposit.State.Contested));
    }

    /// The typehash is the first word of `content`; the vector's message starts with where it is valid.
    function signedBy(uint256 key, bytes32 domain, bytes memory content, bytes memory place)
        internal
        pure
        returns (Signed memory out)
    {
        out.message = abi.encodePacked(place, content);
        out.preimage = abi.encodePacked(hex"1901", domain, keccak256(content));
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(key, keccak256(out.preimage));
        out.signature = abi.encodePacked(r, s, v);
    }

    function line(string memory name, bytes memory value) internal pure returns (string memory) {
        return string.concat(name, "=", vm.toString(value), "\n");
    }
}
