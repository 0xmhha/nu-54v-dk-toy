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
/// then run a whole top-up here, and every action of the signers, so a vector that the contracts would
/// refuse cannot be written down.
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
    address internal constant NEW_PAUSER = address(0x2007);
    address internal constant STRAY_SENDER = address(0x2008);
    uint256 internal constant STRAY_AMOUNT = 5e6;
    bytes32 internal constant STRAY_TX = keccak256("stray transaction");
    uint256 internal constant STRAY_LOG = 3;
    /// 1,000,000.00 won of the pool, in the units of the 2-decimal asset.
    uint256 internal constant WITHDRAWAL = 100_000_000;
    bytes32 internal constant PAUSER_SALT = bytes32(uint256(0x52));
    bytes32 internal constant WITHDRAW_SALT = bytes32(uint256(0x53));
    bytes32 internal constant SET_KEY = keccak256("setKey");
    bytes32 internal constant WITHDRAW = keccak256("withdraw");
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

    /// One `Action` signed by signers 1 and 2.
    struct TwoSigned {
        bytes message;
        bytes preimage;
        bytes one;
        bytes two;
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
    Signed internal confirmedFinal;
    Signed internal cancelled;
    mapping(string => TwoSigned) internal actions;

    function pauserArguments() internal pure returns (bytes memory) {
        return abi.encode(ChargeGovernance.Key.Pauser, NEW_PAUSER);
    }

    /// The number of a change: its kind, its arguments and its salt in one ABI encoding.
    function pauserAction() internal pure returns (bytes32) {
        return keccak256(abi.encode(SET_KEY, ChargeGovernance.Key.Pauser, NEW_PAUSER, PAUSER_SALT));
    }

    function withdrawAction() internal pure returns (bytes32) {
        return keccak256(abi.encode(WITHDRAW, WITHDRAWAL, WITHDRAW_SALT));
    }

    function straySubject() internal pure returns (bytes32) {
        return keccak256(abi.encode(USDC, STRAY_SENDER, STRAY_AMOUNT, STRAY_TX, STRAY_LOG));
    }

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

        // What two signers sign besides a refund. The round of a deposit grows with each of its changes:
        // 0 when deposited, 1 when completed, 2 when contested.
        actions["complete"] = twoSigned(domain, place, action("complete", id, 0));
        actions["reject"] = twoSigned(domain, place, action("reject", id, 1));
        actions["confirmPaid"] = twoSigned(domain, place, action("confirmPaid", id, 2));
        actions["returnStray"] = twoSigned(domain, place, action("returnStray", straySubject(), 0));
        // The undoing of a pause is signed for the round the pause made: the first pause makes round 1.
        actions["unpauseDeposits"] = twoSigned(domain, place, action("unpauseDeposits", 0, 1));
        // A change that waits a day is signed by its number, in the round `proposalRound` answers; one
        // signer other than the one it replaces cancels it in the same round.
        actions["proposePauser"] = twoSigned(domain, place, action("propose", pauserAction(), 0));
        cancelled = signedBy(SIGNER3_KEY, domain, action("cancel", pauserAction(), 0), place);
    }

    function signOnThePoolChain() internal {
        vm.chainId(WON_CHAIN);
        confirmed = signedBy(
            WITNESS_KEY,
            pool.domainSeparator(),
            abi.encode(ChargeTypes.CONFIRMATION_TYPEHASH, id, SOURCE_BLOCK, true, UNTIL),
            abi.encode(WON_CHAIN, POOL)
        );
        confirmedFinal = signedBy(
            WITNESS_KEY,
            pool.domainSeparator(),
            abi.encode(ChargeTypes.CONFIRMATION_TYPEHASH, id, SOURCE_BLOCK, false, UNTIL),
            abi.encode(WON_CHAIN, POOL)
        );
        bytes memory place = abi.encode(WON_CHAIN, POOL);
        actions["unpausePays"] = twoSigned(pool.domainSeparator(), place, action("unpausePays", 0, 1));
        actions["proposeWithdraw"] = twoSigned(pool.domainSeparator(), place, action("propose", withdrawAction(), 0));
    }

    function file() internal view returns (string memory) {
        return string.concat(topUpLines(), finalLines(), signerLines(), pauserLines(), cancelLines(), withdrawLines());
    }

    function topUpLines() internal view returns (string memory) {
        return
            string.concat(quoteLines(), authorizationLines(), poolLines(), paidLines(), refundLines(), contestLines());
    }

    function finalLines() internal view returns (string memory) {
        return string.concat(
            line("confirmationFinal", confirmedFinal.message),
            line("confirmationFinalPreimage", confirmedFinal.preimage),
            line("confirmationFinalSignature", confirmedFinal.signature)
        );
    }

    /// Signed by the two signers of `refundSigners`, in that order.
    function signerLines() internal view returns (string memory) {
        return string.concat(
            actionLines("complete"),
            actionLines("reject"),
            actionLines("confirmPaid"),
            strayLines(),
            actionLines("unpauseDeposits"),
            actionLines("unpausePays")
        );
    }

    /// The subject of `returnStray` is the keccak256 of its arguments.
    function strayLines() internal view returns (string memory) {
        return string.concat(
            line("returnStrayArguments", abi.encode(USDC, STRAY_SENDER, STRAY_AMOUNT, STRAY_TX, STRAY_LOG)),
            actionLines("returnStray")
        );
    }

    /// A change on the deposit contract: the kind, the arguments and the salt give its number, and the
    /// number is the subject two signers sign.
    function pauserLines() internal view returns (string memory) {
        return string.concat(
            line("proposePauserKind", abi.encodePacked(SET_KEY)),
            line("proposePauserArguments", pauserArguments()),
            line("proposePauserSalt", abi.encodePacked(PAUSER_SALT)),
            line("proposePauserAction", abi.encodePacked(pauserAction())),
            actionLines("proposePauser")
        );
    }

    function cancelLines() internal view returns (string memory) {
        return string.concat(
            line("cancelPauser", cancelled.message),
            line("cancelPauserPreimage", cancelled.preimage),
            line("cancelPauserSigner", abi.encodePacked(vm.addr(SIGNER3_KEY))),
            line("cancelPauserSignature", cancelled.signature)
        );
    }

    /// A change on the pool.
    function withdrawLines() internal view returns (string memory) {
        return string.concat(
            line("proposeWithdrawKind", abi.encodePacked(WITHDRAW)),
            line("proposeWithdrawArguments", abi.encode(WITHDRAWAL)),
            line("proposeWithdrawSalt", abi.encodePacked(WITHDRAW_SALT)),
            line("proposeWithdrawAction", abi.encodePacked(withdrawAction())),
            actionLines("proposeWithdraw")
        );
    }

    function actionLines(string memory name) internal view returns (string memory) {
        TwoSigned storage signed = actions[name];
        return string.concat(
            line(name, signed.message),
            line(string.concat(name, "Preimage"), signed.preimage),
            line(string.concat(name, "Signatures"), abi.encodePacked(signed.one, signed.two))
        );
    }

    function both(string memory name) internal view returns (bytes[2] memory) {
        return [actions[name].one, actions[name].two];
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
        uint256 completed = vm.snapshotState();
        deposit.contest(id, UNTIL, contested.signature);
        assertEq(uint8(deposit.depositOf(id).state), uint8(ChargeDeposit.State.Contested));

        // Two signers answer the contest: paid, and final.
        deposit.confirmPaid(id, UNTIL, both("confirmPaid"));
        assertTrue(deposit.depositOf(id).settled);
        // Or, instead of a contest, two signers take the completion back.
        vm.revertToState(completed);
        deposit.reject(id, UNTIL, both("reject"));
        assertTrue(deposit.depositOf(id).rejected);
    }

    /// Every other signature of the file, each used the way the contracts take it.
    function testSignerVectorsMatchTheFileAndAreAccepted() public {
        signOnTheSourceChain();
        signOnThePoolChain();
        assertEq(vm.readFile(FILE), file(), "regenerate the vector");
        ChargeTypes.Quote memory quote = theQuote();
        vm.chainId(SOURCE_CHAIN);
        deposit.depositWithAuthorization(quote, quoted.signature, authorized.signature);

        // The pool pays on a final source block, and two signers record the payout without the relay.
        vm.chainId(WON_CHAIN);
        vm.prank(vm.addr(RELAY_KEY));
        pool.pay(
            WonPool.Origin({chainId: SOURCE_CHAIN, deposit: DEPOSIT}),
            quote,
            quoted.signature,
            WonPool.Confirmation({fast: false, sourceBlockHash: SOURCE_BLOCK, until: UNTIL}),
            confirmedFinal.signature
        );
        assertFalse(pool.payoutOf(id).fast);
        assertEq(pool.usedFast(), 0);
        vm.chainId(SOURCE_CHAIN);
        deposit.completeBySigners(id, UNTIL, both("complete"));
        assertEq(uint8(deposit.depositOf(id).state), uint8(ChargeDeposit.State.Completed));

        // USDC sent without a deposit goes back to its sender.
        TestChargeUSDC(USDC).mint(DEPOSIT, STRAY_AMOUNT);
        deposit.returnStray(USDC, STRAY_SENDER, STRAY_AMOUNT, STRAY_TX, STRAY_LOG, UNTIL, both("returnStray"));
        assertEq(TestChargeUSDC(USDC).balanceOf(STRAY_SENDER), STRAY_AMOUNT);

        // The first pause on each contract, undone.
        vm.prank(vm.addr(PAUSER_KEY));
        deposit.pauseDeposits();
        deposit.unpauseDeposits(UNTIL, both("unpauseDeposits"));
        assertFalse(deposit.depositsPaused());
        vm.chainId(WON_CHAIN);
        vm.prank(vm.addr(PAUSER_KEY));
        pool.pausePays();
        pool.unpausePays(UNTIL, both("unpausePays"));
        assertFalse(pool.paysPaused());

        // A change that waits a day on each contract: proposed by two signers, and applied by anyone.
        assertEq(pool.proposalRound(WITHDRAW, abi.encode(WITHDRAWAL)), 0);
        assertEq(
            pool.propose(WITHDRAW, abi.encode(WITHDRAWAL), WITHDRAW_SALT, UNTIL, both("proposeWithdraw")),
            withdrawAction()
        );
        vm.chainId(SOURCE_CHAIN);
        assertEq(deposit.proposalRound(SET_KEY, pauserArguments()), 0);
        assertEq(deposit.propose(SET_KEY, pauserArguments(), PAUSER_SALT, UNTIL, both("proposePauser")), pauserAction());
        // One signer cancels it; without that it goes through a day later.
        uint256 proposed = vm.snapshotState();
        deposit.cancelProposal(pauserAction(), UNTIL, cancelled.signature);
        vm.warp(NOW + 24 hours);
        vm.expectRevert(ChargeGovernance.InvalidState.selector);
        deposit.setKey(ChargeGovernance.Key.Pauser, NEW_PAUSER, PAUSER_SALT);
        vm.revertToState(proposed);
        vm.warp(NOW + 24 hours);
        deposit.setKey(ChargeGovernance.Key.Pauser, NEW_PAUSER, PAUSER_SALT);
        assertEq(deposit.pauser(), NEW_PAUSER);
        vm.chainId(WON_CHAIN);
        pool.withdraw(WITHDRAWAL, WITHDRAW_SALT);
        assertEq(TestKRW3009(WON).balanceOf(TREASURY), WITHDRAWAL);
    }

    function action(string memory kind, bytes32 subject, uint256 round) internal pure returns (bytes memory) {
        return abi.encode(ChargeTypes.ACTION_TYPEHASH, keccak256(bytes(kind)), subject, round, UNTIL);
    }

    function twoSigned(bytes32 domain, bytes memory place, bytes memory content)
        internal
        pure
        returns (TwoSigned memory out)
    {
        Signed memory first = signedBy(SIGNER1_KEY, domain, content, place);
        out.message = first.message;
        out.preimage = first.preimage;
        out.one = first.signature;
        out.two = signedBy(SIGNER2_KEY, domain, content, place).signature;
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
