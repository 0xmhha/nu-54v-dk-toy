// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Test} from "forge-std/Test.sol";
import {ChargeTypes} from "../src/charge/ChargeTypes.sol";
import {ChargeGovernance} from "../src/charge/ChargeGovernance.sol";
import {ChargeDeposit} from "../src/charge/ChargeDeposit.sol";
import {WonPool} from "../src/charge/WonPool.sol";
import {TestChargeUSDC} from "../src/test-token/TestChargeUSDC.sol";
import {TestKRW3009} from "../src/test-token/TestKRW3009.sol";

/// Two contracts on two chains in one test: every call first switches to the chain id of the contract it
/// talks to, so a digest that forgets the chain or the contract fails here. Amounts are the chosen limits.
abstract contract ChargeBase is Test {
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
    /// The fixed test rate as of 2026-11-27: 1 USDC = 1,345.00 won, in 0.01 won. A deployment value.
    uint256 internal constant RATE = 134_500;
    /// One won in the unit of quotes and limits.
    uint256 internal constant WON = 100;
    uint256 internal constant MIN_WON = 1_000 * WON;
    uint256 internal constant MAX_WON = 300_000 * WON;
    uint256 internal constant ACCOUNT_DAILY = 1_000_000 * WON;
    uint256 internal constant TOTAL_DAILY = 3_000_000 * WON;
    uint256 internal constant FAST_EACH = 50_000 * WON;
    uint256 internal constant FAST_DAILY = 500_000 * WON;
    uint256 internal constant POOL_SIZE = 5_000_000 * WON;
    bytes32 internal constant SOURCE_BLOCK = bytes32("source block");
    bytes32 internal constant WON_BLOCK = bytes32("won block");

    TestChargeUSDC internal usdc;
    address internal asset;
    ChargeDeposit internal deposit;
    WonPool internal pool;
    address internal payer;
    address internal board = address(0xB0A2D0);
    address internal treasury = address(0x7EA5);
    address internal stranger = address(0x5712A);
    uint256 internal quoteCount;

    struct PayArgs {
        WonPool.Origin origin;
        ChargeTypes.Quote quote;
        bytes quoteSignature;
        WonPool.Confirmation confirmation;
        bytes witnessSignature;
    }

    function setUp() public virtual {
        vm.warp(1_800_000_000);
        payer = vm.addr(PAYER_KEY);
        toSource();
        usdc = new TestChargeUSDC(address(this));
        usdc.mint(payer, 1_000_000e6);
        toWon();
        asset = deployAsset();
        address nextPool = vm.computeCreateAddress(address(this), vm.getNonce(address(this)) + 1);
        toSource();
        deposit = new ChargeDeposit(keys(), depositConfig(nextPool));
        toWon();
        pool = new WonPool(keys(), poolConfig(address(deposit)));
        assertEq(address(pool), nextPool);
        fundPool(POOL_SIZE);
        toSource();
    }

    // ---- what a variant changes ----

    function deployAsset() internal virtual returns (address) {
        return address(new TestKRW3009(address(this)));
    }

    function assetDecimals() internal view virtual returns (uint8) {
        return 2;
    }

    function assetIsCoin() internal view virtual returns (bool) {
        return false;
    }

    function fundPool(uint256 wonAmount) internal virtual {
        TestKRW3009(asset).mint(address(pool), wonAmount);
    }

    function assetBalance(address account) internal view returns (uint256) {
        return TestKRW3009(asset).balanceOf(account);
    }

    /// One 0.01 won in the units of the asset.
    function scale() internal view returns (uint256) {
        return 10 ** (assetDecimals() - 2);
    }

    // ---- deployment values ----

    function keys() internal pure returns (ChargeGovernance.Keys memory) {
        return ChargeGovernance.Keys({
            relay: vm.addr(RELAY_KEY),
            quoter: vm.addr(QUOTER_KEY),
            witness: vm.addr(WITNESS_KEY),
            pauser: vm.addr(PAUSER_KEY),
            signers: [vm.addr(SIGNER1_KEY), vm.addr(SIGNER2_KEY), vm.addr(SIGNER3_KEY)]
        });
    }

    function depositConfig(address pool_) internal view returns (ChargeDeposit.Config memory) {
        return ChargeDeposit.Config({
            usdc: address(usdc),
            chainId: SOURCE_CHAIN,
            wonChainId: WON_CHAIN,
            pool: pool_,
            won: asset,
            minWon: MIN_WON,
            maxWon: MAX_WON,
            minRate: RATE,
            maxRate: RATE
        });
    }

    function limits() internal pure returns (WonPool.Limits memory) {
        return WonPool.Limits({
            perPayout: MAX_WON,
            accountDaily: ACCOUNT_DAILY,
            totalDaily: TOTAL_DAILY,
            fastPerPayout: FAST_EACH,
            fastDaily: FAST_DAILY
        });
    }

    function poolConfig(address deposit_) internal view returns (WonPool.Config memory) {
        return WonPool.Config({
            asset: asset,
            assetDecimals: assetDecimals(),
            assetIsCoin: assetIsCoin(),
            treasury: treasury,
            chainId: WON_CHAIN,
            minWon: MIN_WON,
            minRate: RATE,
            maxRate: RATE,
            limits: limits(),
            origin: WonPool.Origin({chainId: SOURCE_CHAIN, deposit: deposit_})
        });
    }

    // ---- chains ----

    function toSource() internal {
        vm.chainId(SOURCE_CHAIN);
    }

    function toWon() internal {
        vm.chainId(WON_CHAIN);
    }

    // ---- signatures ----

    function sign(uint256 key, bytes32 digest) internal pure returns (bytes memory) {
        (uint8 v, bytes32 r, bytes32 s) = vm.sign(key, digest);
        return abi.encodePacked(r, s, v);
    }

    function origin() internal view returns (WonPool.Origin memory) {
        return WonPool.Origin({chainId: SOURCE_CHAIN, deposit: address(deposit)});
    }

    /// A quote for a USDC amount, issued now and valid for two minutes.
    function quoteFor(uint256 usdcAmount, address board_) internal returns (ChargeTypes.Quote memory) {
        return ChargeTypes.Quote({
            method: 1,
            usdc: address(usdc),
            payer: payer,
            usdcAmount: usdcAmount,
            wonChainId: WON_CHAIN,
            pool: address(pool),
            won: asset,
            board: board_,
            wonAmount: usdcAmount * RATE / 1e6,
            rate: RATE,
            feeWon: 0,
            expiresAt: uint64(block.timestamp + 2 minutes),
            payBy: uint64(block.timestamp + 2 minutes + 2 hours),
            number: bytes32(++quoteCount)
        });
    }

    /// The least USDC that buys an amount of 0.01 won.
    function usdcFor(uint256 wonAmount) internal pure returns (uint256) {
        return (wonAmount * 1e6 + RATE - 1) / RATE;
    }

    /// A quote that buys exactly an amount of 0.01 won.
    function quoteForWon(uint256 wonAmount, address board_) internal returns (ChargeTypes.Quote memory quote) {
        quote = quoteFor(usdcFor(wonAmount), board_);
        assertEq(quote.wonAmount, wonAmount);
    }

    /// Memory structs alias on assignment; a changed copy must not change the original.
    function copy(ChargeTypes.Quote memory quote) internal pure returns (ChargeTypes.Quote memory) {
        return abi.decode(abi.encode(quote), (ChargeTypes.Quote));
    }

    function idOf(ChargeTypes.Quote memory quote) internal returns (bytes32 id) {
        uint256 chain = block.chainid;
        toSource();
        id = deposit.quoteDigest(quote);
        vm.chainId(chain);
    }

    function quoteSignature(ChargeTypes.Quote memory quote) internal returns (bytes memory) {
        return sign(QUOTER_KEY, idOf(quote));
    }

    /// The one signature the traveller's wallet gives: an EIP-3009 authorization whose nonce is the quote.
    function authorization(ChargeTypes.Quote memory quote, uint256 key) internal returns (bytes memory) {
        return authorizationFor(quote, key, address(deposit), idOf(quote));
    }

    function authorizationFor(ChargeTypes.Quote memory quote, uint256 key, address to, bytes32 nonce)
        internal
        returns (bytes memory signature)
    {
        uint256 chain = block.chainid;
        toSource();
        bytes32 content = keccak256(
            abi.encode(
                usdc.RECEIVE_WITH_AUTHORIZATION_TYPEHASH(),
                quote.payer,
                to,
                quote.usdcAmount,
                uint256(0),
                uint256(quote.expiresAt),
                nonce
            )
        );
        signature = sign(key, keccak256(abi.encodePacked(hex"1901", usdc.DOMAIN_SEPARATOR(), content)));
        vm.chainId(chain);
    }

    function two(ChargeGovernance target, string memory kind, bytes32 subject, uint256 round, uint256 until)
        internal
        view
        returns (bytes[2] memory signatures)
    {
        bytes32 digest = target.actionDigest(keccak256(bytes(kind)), subject, round, until);
        signatures = [sign(SIGNER1_KEY, digest), sign(SIGNER2_KEY, digest)];
    }

    // ---- steps of a top-up ----

    function depositIt(ChargeTypes.Quote memory quote) internal returns (bytes32 id) {
        bytes memory quoteSig = quoteSignature(quote);
        bytes memory auth = authorization(quote, PAYER_KEY);
        toSource();
        vm.prank(vm.addr(RELAY_KEY));
        id = deposit.depositWithAuthorization(quote, quoteSig, auth);
    }

    function payArgs(ChargeTypes.Quote memory quote, bool fast) internal returns (PayArgs memory args) {
        uint256 chain = block.chainid;
        args.origin = origin();
        args.quote = quote;
        args.quoteSignature = quoteSignature(quote);
        args.confirmation =
            WonPool.Confirmation({fast: fast, sourceBlockHash: SOURCE_BLOCK, until: block.timestamp + 1 hours});
        toWon();
        args.witnessSignature = sign(
            WITNESS_KEY,
            pool.confirmationDigest(pool.payoutId(args.origin, quote), SOURCE_BLOCK, fast, args.confirmation.until)
        );
        vm.chainId(chain);
    }

    function payWith(PayArgs memory args) internal {
        toWon();
        vm.prank(vm.addr(RELAY_KEY));
        pool.pay(args.origin, args.quote, args.quoteSignature, args.confirmation, args.witnessSignature);
    }

    function payIt(ChargeTypes.Quote memory quote, bool fast) internal {
        payWith(payArgs(quote, fast));
    }

    /// An external form, so a test can expect its revert after the signatures were made.
    function payItExternal(ChargeTypes.Quote memory quote, bool fast) external {
        payIt(quote, fast);
    }

    function paidSignature(bytes32 id, uint256 until) internal returns (bytes memory) {
        toSource();
        return sign(WITNESS_KEY, deposit.paidDigest(id, WON_BLOCK, until));
    }

    function completeIt(bytes32 id) internal {
        uint256 until = block.timestamp + 1 hours;
        bytes memory signature = paidSignature(id, until);
        vm.prank(vm.addr(RELAY_KEY));
        deposit.complete(id, WON_BLOCK, until, signature);
    }

    function completeItExternal(bytes32 id) external {
        completeIt(id);
    }

    function contestIt(bytes32 id) internal {
        toSource();
        vm.prank(payer);
        deposit.contest(id, 0, "");
    }

    function rejectIt(bytes32 id) internal {
        toSource();
        uint256 until = block.timestamp + 1 days;
        deposit.reject(id, until, two(deposit, "reject", id, deposit.depositOf(id).round, until));
    }

    function refundSubject(bytes32 id) internal pure returns (bytes32) {
        return keccak256(abi.encode(id, WON_BLOCK));
    }

    function refundBySigners(bytes32 id) internal {
        toSource();
        uint256 until = block.timestamp + 1 days;
        bytes[2] memory signatures = two(deposit, "refund", refundSubject(id), deposit.depositOf(id).round, until);
        deposit.refundBySigners(id, WON_BLOCK, until, signatures);
    }

    /// Two signers propose a change on a contract; the caller then waits and applies it. `kind` is the name
    /// of the changing function and `arguments` are its arguments without the salt.
    function proposeOn(ChargeGovernance target, string memory kind, bytes memory arguments, bytes32 salt)
        internal
        returns (bytes32 action)
    {
        uint256 until = block.timestamp + 1 days;
        bytes32 kindHash = keccak256(bytes(kind));
        action = target.actionOf(kindHash, arguments, salt);
        uint256 round = target.proposalRound(kindHash, arguments);
        target.propose(kindHash, arguments, salt, until, two(target, "propose", action, round, until));
    }

    /// The arguments of `setSpender`: a contract with the hash of its code as it is now, or nothing.
    function spenderArgs(address to) internal view returns (bytes memory) {
        return abi.encode(to, to == address(0) ? bytes32(0) : to.codehash);
    }

    /// A change for no seat that the contract refuses when it is proposed, with two good signatures.
    function expectProposeRevert(ChargeGovernance target, string memory kind, bytes memory arguments, bytes4 refusal)
        internal
    {
        uint256 until = block.timestamp + 1 days;
        bytes32 kindHash = keccak256(bytes(kind));
        bytes32 action = target.actionOf(kindHash, arguments, "salt");
        bytes[2] memory signatures = two(target, "propose", action, target.generation(), until);
        vm.expectRevert(refusal);
        target.propose(kindHash, arguments, "salt", until, signatures);
    }

    /// One signer's signature that stops a waiting change.
    function cancelSignature(ChargeGovernance target, uint256 key, bytes32 action, uint256 until)
        internal
        view
        returns (bytes memory)
    {
        (,,, uint256 round) = target.proposals(action);
        return sign(key, target.actionDigest(keccak256("cancel"), action, round, until));
    }

    function state(bytes32 id) internal view returns (ChargeDeposit.State) {
        return deposit.depositOf(id).state;
    }
}
