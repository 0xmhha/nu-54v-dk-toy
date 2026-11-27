// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {ChargeTypes} from "./ChargeTypes.sol";
import {ChargeGovernance} from "./ChargeGovernance.sol";

/// @title Pays the won of a top-up from money put here in advance.
/// @notice Nothing is minted. A payout needs three keys at once: the relay sends it, the quote key signed the
/// quote and the deposit-confirmation key signed that the deposit is in a block of the source chain. This
/// contract cannot see that chain, so it trusts the confirmation; what it enforces itself is one payout per
/// deposit number, the payout deadline, the rate range and the limits, which hold even when all three keys
/// are in the wrong hands.
///
/// The asset is a deployment value. Quotes and limits count 0.01 won; the amount sent is that number times
/// `unitScale`: 1 for a token with 2 decimals, 10^16 for the token view of StableNet's coin (18 decimals).
/// Both are paid by the same call, `transfer(board, amount)` on the asset, and the payout only counts when
/// the board's balance grew by exactly that amount.
contract WonPool is ChargeGovernance {
    struct Origin {
        uint256 chainId;
        address deposit;
    }

    /// @dev All in 0.01 won. `accountDaily` is counted for the board and for the paying wallet separately.
    struct Limits {
        uint256 perPayout;
        uint256 accountDaily;
        uint256 totalDaily;
        uint256 fastPerPayout;
        uint256 fastDaily;
    }

    struct Config {
        address asset;
        uint8 assetDecimals; // must equal what the asset answers
        // The asset is the token view of the chain's coin, so plain coin transfers are accepted. Must be what
        // the asset shows for the deployer's balance.
        bool assetIsCoin;
        address treasury; // the only place pool money can be withdrawn to
        uint256 chainId; // this chain; a deployment anywhere else fails
        uint256 minWon;
        uint256 minRate;
        uint256 maxRate;
        Limits limits;
        Origin origin; // the first deposit contract whose quotes are paid
    }

    /// @dev `fast`: paid on a source block that was safe but not yet final.
    struct Confirmation {
        bool fast;
        bytes32 sourceBlockHash;
        uint256 until;
    }

    struct Payout {
        address board;
        address payer;
        uint256 wonAmount;
        uint64 paidAt; // 0 = this deposit number was never paid
        bool fast;
    }

    struct Bucket {
        uint64 hour;
        uint128 amount;
    }

    /// @notice The current hour and the 24 before it are counted, so no 24 hours ever exceed a limit. A
    /// payout can therefore be held back for up to 25 hours.
    uint256 public constant BUCKETS = 25;

    bytes32 private constant UNPAUSE = keccak256("unpausePays");
    bytes32 private constant WITHDRAW = keccak256("withdraw");
    bytes32 private constant RAISE_LIMITS = keccak256("raiseLimits");
    bytes32 private constant ADD_ORIGIN = keccak256("addOrigin");
    bytes32 private constant RETURN_TOKEN = keccak256("returnToken");
    bytes32 private constant TOTAL = keccak256("total");
    bytes32 private constant FAST = keccak256("fast");
    bytes32 private constant BOARD = keccak256("board");
    bytes32 private constant PAYER = keccak256("payer");

    address public immutable asset;
    uint8 public immutable assetDecimals;
    bool public immutable assetIsCoin;
    /// @notice One 0.01 won in the units of the asset.
    uint256 public immutable unitScale;
    address public immutable treasury;
    uint256 public immutable chainId;
    uint256 public immutable minWon;
    Limits public limits;
    bool public paysPaused;
    uint256 public pauseRound;

    mapping(bytes32 => bool) private origins;
    mapping(bytes32 => Payout) private payouts;
    mapping(bytes32 => Bucket[25]) private windows;

    event Paid(
        bytes32 indexed id,
        address indexed board,
        address indexed payer,
        uint256 wonAmount,
        uint256 assetAmount,
        uint256 rate,
        bool fast,
        uint256 sourceChainId,
        address sourceDeposit,
        bytes32 sourceBlockHash
    );
    event OriginSet(uint256 indexed chainId, address indexed deposit, bool allowed);
    event LimitsChanged(Limits limits);
    event PaysPausedSet(bool paused);
    event Withdrawn(address indexed treasury, uint256 assetAmount);
    event TokenReturned(address indexed token, address indexed treasury, uint256 amount);
    event CoinReceived(address indexed from, uint256 amount);

    error WrongChain();
    error PaysPaused();
    error UnknownOrigin();
    error AlreadyPaid();
    error InvalidQuote();
    error InvalidAmount();
    error InvalidRate();
    error LimitExceeded();
    error CoinNotAccepted();

    constructor(Keys memory keys, Config memory config) ChargeGovernance(keys, config.minRate, config.maxRate) {
        if (block.chainid != config.chainId) revert WrongChain();
        if (
            config.asset.code.length == 0 || config.treasury == address(0) || config.treasury == address(this)
                || config.minWon == 0 || !_validLimits(config.limits, config.minWon)
                || config.assetDecimals < ChargeTypes.WON_DECIMALS || config.assetDecimals > 36
                || config.origin.chainId == 0 || config.origin.deposit == address(0)
        ) revert InvalidParameters();
        (bool ok, bytes memory answer) = config.asset.staticcall(abi.encodeWithSignature("decimals()"));
        if (!ok || answer.length != 32 || abi.decode(answer, (uint256)) != config.assetDecimals) {
            revert InvalidParameters();
        }
        // The token view of the chain's coin answers an account's coin balance; a token of its own does not.
        // The deployer holds coins, so its two balances tell which of the two the asset is. A pool for the
        // coin must therefore be deployed by an account that holds some. This catches a wrong `assetIsCoin`
        // in an honest deployment and nothing more: a deployer without coins passes with the coin as a
        // token, and so does any token that answers the deployer's coin balance. The deployment checklist
        // reads the asset on the chain before deploying.
        bool coin = msg.sender.balance != 0 && _balanceOf(config.asset, msg.sender) == msg.sender.balance;
        if (coin != config.assetIsCoin) revert InvalidParameters();
        asset = config.asset;
        assetDecimals = config.assetDecimals;
        assetIsCoin = config.assetIsCoin;
        unitScale = 10 ** (config.assetDecimals - ChargeTypes.WON_DECIMALS);
        treasury = config.treasury;
        chainId = config.chainId;
        minWon = config.minWon;
        limits = config.limits;
        origins[_originKey(config.origin.chainId, config.origin.deposit)] = true;
        emit OriginSet(config.origin.chainId, config.origin.deposit, true);
        emit LimitsChanged(config.limits);
    }

    /// @notice Coins sent plainly are pool money when the asset is the coin, and refused otherwise: with a
    /// token as the asset they could never leave.
    receive() external payable {
        if (!assetIsCoin) revert CoinNotAccepted();
        emit CoinReceived(msg.sender, msg.value);
    }

    // ---- reading ----

    function isOrigin(uint256 sourceChainId, address deposit) public view returns (bool) {
        return origins[_originKey(sourceChainId, deposit)];
    }

    /// @notice The payout number of a quote: the deposit number the deposit contract of `origin` gives it.
    function payoutId(Origin calldata origin, ChargeTypes.Quote calldata quote) public pure returns (bytes32) {
        return ChargeTypes.quoteDigest(ChargeTypes.domain(origin.chainId, origin.deposit), quote);
    }

    /// @notice What the deposit-confirmation key signs for this pool.
    function confirmationDigest(bytes32 id, bytes32 sourceBlockHash, bool fast, uint256 until)
        public
        view
        returns (bytes32)
    {
        return ChargeTypes.digest(
            domainSeparator(),
            keccak256(abi.encode(ChargeTypes.CONFIRMATION_TYPEHASH, id, sourceBlockHash, fast, until))
        );
    }

    function payoutOf(bytes32 id) external view returns (Payout memory) {
        return payouts[id];
    }

    /// @notice What counts against the daily limit of a board right now, in 0.01 won.
    function usedByBoard(address board) external view returns (uint256) {
        return _used(keccak256(abi.encode(BOARD, board)));
    }

    function usedByPayer(address payer) external view returns (uint256) {
        return _used(keccak256(abi.encode(PAYER, payer)));
    }

    function usedTotal() external view returns (uint256) {
        return _used(TOTAL);
    }

    function usedFast() external view returns (uint256) {
        return _used(FAST);
    }

    // ---- payout ----

    /// @notice Pays the won of one deposit to its board. Only the relay may call, and only with the quote
    /// key's and the deposit-confirmation key's signatures. Each deposit number is paid once.
    function pay(
        Origin calldata origin,
        ChargeTypes.Quote calldata quote,
        bytes calldata quoteSignature,
        Confirmation calldata confirmation,
        bytes calldata witnessSignature
    ) external guarded {
        if (block.chainid != chainId) revert WrongChain();
        if (relay == address(0) || msg.sender != relay) revert Unauthorized();
        if (paysPaused) revert PaysPaused();
        if (!isOrigin(origin.chainId, origin.deposit)) revert UnknownOrigin();
        bytes32 id = payoutId(origin, quote);
        if (payouts[id].paidAt != 0) revert AlreadyPaid();
        if (quoter == address(0) || ChargeTypes.recover(id, quoteSignature) != quoter) revert Unauthorized();
        _checkQuote(quote);
        _checkFresh(confirmation.until);
        bytes32 confirmed = confirmationDigest(id, confirmation.sourceBlockHash, confirmation.fast, confirmation.until);
        if (witness == address(0) || ChargeTypes.recover(confirmed, witnessSignature) != witness) {
            revert Unauthorized();
        }
        _count(quote.board, quote.payer, quote.wonAmount, confirmation.fast);
        _pay(id, origin, quote, confirmation);
    }

    function _pay(
        bytes32 id,
        Origin calldata origin,
        ChargeTypes.Quote calldata quote,
        Confirmation calldata confirmation
    ) private {
        payouts[id] = Payout({
            board: quote.board,
            payer: quote.payer,
            wonAmount: quote.wonAmount,
            paidAt: uint64(block.timestamp),
            fast: confirmation.fast
        });
        uint256 assetAmount = quote.wonAmount * unitScale;
        emit Paid(
            id,
            quote.board,
            quote.payer,
            quote.wonAmount,
            assetAmount,
            quote.rate,
            confirmation.fast,
            origin.chainId,
            origin.deposit,
            confirmation.sourceBlockHash
        );
        // A board that cannot receive (a blocked account of the coin, a token that holds the transfer) or a
        // pool that is short makes the whole call fail: the deposit number stays unpaid, no limit is used and
        // the deposit is refunded on its own chain.
        _send(asset, quote.board, assetAmount);
    }

    // ---- stopping and limits ----

    /// @notice Stops every payout. Deposits that are in then end as refunds on their own chain.
    function pausePays() external onlyPauser {
        paysPaused = true;
        pauseRound += 1;
        emit PaysPausedSet(true);
    }

    function unpausePays(uint256 until, bytes[2] calldata signatures) external {
        _checkTwo(UNPAUSE, bytes32(0), pauseRound, until, signatures);
        paysPaused = false;
        pauseRound += 1;
        emit PaysPausedSet(false);
    }

    function lowerLimits(Limits calldata next) external onlyPauser {
        Limits memory now_ = limits;
        if (
            next.perPayout > now_.perPayout || next.accountDaily > now_.accountDaily
                || next.totalDaily > now_.totalDaily || next.fastPerPayout > now_.fastPerPayout
                || next.fastDaily > now_.fastDaily
        ) revert InvalidParameters();
        limits = next;
        emit LimitsChanged(next);
    }

    function raiseLimits(Limits calldata next, bytes32 salt) external {
        _apply(RAISE_LIMITS, abi.encode(next), salt);
        if (!_validLimits(next, minWon)) revert InvalidParameters();
        limits = next;
        emit LimitsChanged(next);
    }

    /// @notice A further deposit contract, for another source chain or deposit method.
    function addOrigin(uint256 sourceChainId, address deposit, bytes32 salt) external {
        _apply(ADD_ORIGIN, abi.encode(sourceChainId, deposit), salt);
        if (sourceChainId == 0 || deposit == address(0)) revert InvalidParameters();
        origins[_originKey(sourceChainId, deposit)] = true;
        emit OriginSet(sourceChainId, deposit, true);
    }

    function removeOrigin(uint256 sourceChainId, address deposit) external onlyPauser {
        origins[_originKey(sourceChainId, deposit)] = false;
        emit OriginSet(sourceChainId, deposit, false);
    }

    /// @notice Pool money leaves, other than as a payout, only to the treasury and only after two signers and
    /// a day. Putting money in needs nothing: send the asset to this address.
    function withdraw(uint256 assetAmount, bytes32 salt) external guarded {
        _apply(WITHDRAW, abi.encode(assetAmount), salt);
        emit Withdrawn(treasury, assetAmount);
        _send(asset, treasury, assetAmount);
    }

    /// @notice Another token that came here by mistake goes to the treasury, after two signers and a day.
    /// The asset itself leaves only by `pay` and `withdraw`: it is refused here, and a token that moves the
    /// pool's asset under another address undoes the whole call.
    function returnToken(address token, uint256 amount, bytes32 salt) external guarded {
        _apply(RETURN_TOKEN, abi.encode(token, amount), salt);
        if (token == asset || token.code.length == 0 || amount == 0) revert InvalidParameters();
        uint256 held = _balanceOf(asset, address(this));
        emit TokenReturned(token, treasury, amount);
        _send(token, treasury, amount);
        if (_balanceOf(asset, address(this)) != held) revert TokenBalanceMismatch();
    }

    // ---- internals ----

    function _shape(bytes32 kind) internal pure override returns (uint256, uint256) {
        if (kind == WITHDRAW) return (32, 0);
        if (kind == ADD_ORIGIN) return (64, 2);
        if (kind == RETURN_TOKEN) return (64, 1);
        if (kind == RAISE_LIMITS) return (160, 0);
        return super._shape(kind);
    }

    /// @dev How the USDC arrived (`quote.method`) is the business of the deposit contract that took it; a
    /// deposit contract for another method is another origin.
    function _checkQuote(ChargeTypes.Quote calldata quote) private view {
        if (
            quote.wonChainId != chainId || quote.pool != address(this) || quote.won != asset
                || quote.board == address(0) || quote.board == address(this) || quote.board == asset
                || quote.feeWon != 0
        ) revert InvalidQuote();
        if (quote.payBy != quote.expiresAt + ChargeTypes.PAY_WINDOW || block.timestamp >= quote.payBy) {
            revert InvalidTime();
        }
        if (quote.rate < minRate || quote.rate > maxRate) revert InvalidRate();
        if (quote.wonAmount != ChargeTypes.wonFor(quote.usdcAmount, quote.rate)) revert InvalidQuote();
        if (quote.wonAmount < minWon || quote.wonAmount > limits.perPayout) revert InvalidAmount();
    }

    function _count(address board, address payer, uint256 wonAmount, bool fast) private {
        Limits memory limit = limits;
        if (fast) {
            if (wonAmount > limit.fastPerPayout) revert LimitExceeded();
            _take(FAST, wonAmount, limit.fastDaily);
        }
        _take(keccak256(abi.encode(BOARD, board)), wonAmount, limit.accountDaily);
        _take(keccak256(abi.encode(PAYER, payer)), wonAmount, limit.accountDaily);
        _take(TOTAL, wonAmount, limit.totalDaily);
    }

    function _take(bytes32 key, uint256 wonAmount, uint256 limit) private {
        if (_used(key) + wonAmount > limit) revert LimitExceeded();
        uint64 hour = uint64(block.timestamp / 1 hours);
        Bucket storage bucket = windows[key][hour % BUCKETS];
        if (bucket.hour != hour) {
            bucket.hour = hour;
            bucket.amount = 0;
        }
        // The sum stays within the limit checked above, and no limit is larger than 128 bits.
        // forge-lint: disable-next-line(unsafe-typecast)
        bucket.amount += uint128(wonAmount);
    }

    function _used(bytes32 key) private view returns (uint256 sum) {
        uint64 hour = uint64(block.timestamp / 1 hours);
        Bucket[25] storage buckets = windows[key];
        for (uint256 i; i < BUCKETS; ++i) {
            Bucket memory bucket = buckets[i];
            if (bucket.hour <= hour && bucket.hour + 24 >= hour) sum += bucket.amount;
        }
    }

    /// @dev The hourly sums are kept in 128 bits, so no limit may be larger.
    function _validLimits(Limits memory next, uint256 least) private pure returns (bool) {
        uint256 most = type(uint128).max;
        return next.perPayout >= least && next.perPayout <= most && next.accountDaily <= most && next.totalDaily <= most
            && next.fastPerPayout <= most && next.fastDaily <= most;
    }

    function _originKey(uint256 sourceChainId, address deposit) private pure returns (bytes32) {
        return keccak256(abi.encode(sourceChainId, deposit));
    }
}
