// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {ChargeTypes} from "./ChargeTypes.sol";
import {ChargeGovernance} from "./ChargeGovernance.sol";

interface IAuthorizedUSDC {
    function receiveWithAuthorization(
        address from,
        address to,
        uint256 value,
        uint256 validAfter,
        uint256 validBefore,
        bytes32 nonce,
        uint8 v,
        bytes32 r,
        bytes32 s
    ) external;
}

/// @title Takes a traveller's USDC for one top-up and keeps it until the won was paid on StableNet.
/// @notice The traveller signs one EIP-3009 authorization whose nonce is the digest of the signed quote, so
/// the amount, this contract, the deadline, the rate and the board that receives the won are all inside that
/// one signature. Only this contract can collect it. Anyone may send the transaction and pay the gas.
///
/// A deposit ends in one of two ways. The pool paid: the relay records it with the deposit-confirmation
/// key's signature and the USDC stays here. The pool did not pay: the USDC goes back to the paying wallet and
/// to nobody else, by two signers two hours after the payout deadline, or by anyone after seven days.
/// A completion can be contested by the payer for seven days and must then be answered by two signers within
/// fourteen days, or the money goes back.
///
/// Not built: a second deposit method, and turning won back into USDC. For the first the quote carries a
/// method number and `_accept` is shared. For the second completed money can leave only through `spender`,
/// which is empty.
contract ChargeDeposit is ChargeGovernance {
    enum State {
        None,
        Deposited,
        Completed,
        Contested,
        Refunded
    }

    enum RefundReason {
        Signers,
        Unpaid,
        Unanswered
    }

    struct Config {
        address usdc;
        uint256 chainId; // this chain; a deployment anywhere else fails
        uint256 wonChainId;
        address pool;
        address won;
        uint256 minWon; // per top-up, in 0.01 won
        uint256 maxWon;
        uint256 minRate;
        uint256 maxRate;
    }

    struct Deposit {
        address payer;
        address board;
        uint256 usdcAmount;
        uint256 left; // what is still held of it
        uint256 wonAmount;
        uint256 rate;
        uint64 depositedAt;
        uint64 payBy;
        uint64 completedAt;
        uint64 rejectedAt;
        uint64 contestedAt;
        uint32 round; // grows with every change, so a signature made for one state works once
        bool rejected; // a completion was taken back; only two signers may complete it again
        bool settled; // two signers answered a contest; nothing reopens it
        State state;
    }

    /// @notice A quote must end within this time of the block that takes the deposit.
    uint64 public constant MAX_QUOTE_LIFE = 10 minutes;
    /// @notice Two signers may refund this long after the payout deadline.
    uint64 public constant SIGNED_REFUND_DELAY = 2 hours;
    /// @notice Anyone may send the refund this long after the deposit.
    uint64 public constant SELF_REFUND_DELAY = 7 days;
    /// @notice After a completion was taken back, the self refund waits at least this long.
    uint64 public constant REJECTED_REFUND_DELAY = 3 days;
    /// @notice The payer may contest a completion for this long.
    uint64 public constant CONTEST_WINDOW = 7 days;
    /// @notice Two signers must answer a contest within this time, or the money goes back.
    uint64 public constant ANSWER_WINDOW = 14 days;

    bytes32 private constant COMPLETE = keccak256("complete");
    bytes32 private constant CONFIRM_PAID = keccak256("confirmPaid");
    bytes32 private constant REJECT = keccak256("reject");
    bytes32 private constant REFUND = keccak256("refund");
    bytes32 private constant RETURN_STRAY = keccak256("returnStray");
    bytes32 private constant UNPAUSE = keccak256("unpauseDeposits");
    bytes32 private constant SET_SPENDER = keccak256("setSpender");
    bytes32 private constant RAISE_LIMIT = keccak256("raiseLimit");

    address public immutable usdc;
    uint256 public immutable chainId;
    uint256 public immutable wonChainId;
    address public immutable pool;
    address public immutable won;
    uint256 public immutable minWon;
    uint256 public maxWon;
    /// @notice USDC that belongs to deposits: not refunded and not spent.
    uint256 public totalHeld;
    bool public depositsPaused;
    uint256 public pauseRound;
    /// @notice The one contract that may move completed money. Empty until two signers set it.
    address public spender;

    mapping(bytes32 => Deposit) private deposits;
    mapping(bytes32 => bool) public strayReturned;

    event Deposited(
        bytes32 indexed id,
        address indexed payer,
        address indexed board,
        uint256 usdcAmount,
        uint256 wonAmount,
        uint256 rate,
        uint8 method,
        uint64 expiresAt,
        uint64 payBy,
        bytes32 number
    );
    /// @notice `wonBlockHash` is zero when two signers completed it.
    event Completed(bytes32 indexed id, bytes32 wonBlockHash, bool bySigners);
    event Contested(bytes32 indexed id, uint64 answerBy);
    event ContestAnswered(bytes32 indexed id);
    event CompletionRejected(bytes32 indexed id);
    event Refunded(
        bytes32 indexed id, address indexed payer, uint256 amount, RefundReason reason, bytes32 wonBlockHash
    );
    event Spent(bytes32 indexed id, address indexed to, uint256 amount);
    event StrayReturned(address indexed token, address indexed to, uint256 amount, bytes32 txHash, uint256 logIndex);
    event DepositsPausedSet(bool paused);
    event LimitChanged(uint256 maxWon);
    event SpenderChanged(address indexed spender);

    error WrongChain();
    error DepositsPaused();
    error AlreadyDeposited();
    error InvalidQuote();
    error InvalidAmount();
    error InvalidRate();
    error AlreadyReturned();
    error NothingStray();

    constructor(Keys memory keys, Config memory config) ChargeGovernance(keys, config.minRate, config.maxRate) {
        if (block.chainid != config.chainId) revert WrongChain();
        if (
            config.usdc.code.length == 0 || config.pool == address(0) || config.won == address(0)
                || config.wonChainId == 0 || config.minWon == 0 || config.minWon > config.maxWon
        ) revert InvalidParameters();
        usdc = config.usdc;
        chainId = config.chainId;
        wonChainId = config.wonChainId;
        pool = config.pool;
        won = config.won;
        minWon = config.minWon;
        maxWon = config.maxWon;
    }

    // ---- digests ----

    /// @notice The deposit number of a quote: also the nonce the wallet signs and the payout number.
    function quoteDigest(ChargeTypes.Quote calldata quote) public view returns (bytes32) {
        return ChargeTypes.quoteDigest(domainSeparator(), quote);
    }

    /// @notice What the deposit-confirmation key signs to record a payout, for the deposit as it is now.
    function paidDigest(bytes32 id, bytes32 wonBlockHash, uint256 until) public view returns (bytes32) {
        return ChargeTypes.digest(
            domainSeparator(),
            keccak256(abi.encode(ChargeTypes.PAID_TYPEHASH, id, wonBlockHash, deposits[id].round, until))
        );
    }

    /// @notice What the payer signs to contest a completion, for the deposit as it is now.
    function contestDigest(bytes32 id, uint256 until) public view returns (bytes32) {
        return ChargeTypes.digest(
            domainSeparator(), keccak256(abi.encode(ChargeTypes.CONTEST_TYPEHASH, id, deposits[id].round, until))
        );
    }

    function depositOf(bytes32 id) external view returns (Deposit memory) {
        return deposits[id];
    }

    /// @notice When anyone may send the refund of a deposit; 0 when no such refund is coming.
    function selfRefundAt(bytes32 id) public view returns (uint64) {
        Deposit storage record = deposits[id];
        if (record.state == State.Contested) return record.contestedAt + ANSWER_WINDOW;
        if (record.state != State.Deposited) return 0;
        uint64 opensAt = record.depositedAt + SELF_REFUND_DELAY;
        if (record.rejected && record.rejectedAt + REJECTED_REFUND_DELAY > opensAt) {
            opensAt = record.rejectedAt + REJECTED_REFUND_DELAY;
        }
        return opensAt;
    }

    // ---- deposit ----

    /// @notice Takes the USDC of one quote with the payer's signed authorization. Anyone may send it.
    /// @param authorization the 65-byte signature of `receiveWithAuthorization(payer, this, usdcAmount, 0,
    /// expiresAt, quoteDigest)` in the signing domain of the token
    function depositWithAuthorization(
        ChargeTypes.Quote calldata quote,
        bytes calldata quoteSignature,
        bytes calldata authorization
    ) external guarded returns (bytes32 id) {
        id = _accept(quote, quoteSignature, ChargeTypes.METHOD_AUTHORIZATION);
        if (authorization.length != 65) revert ChargeTypes.InvalidSignature();
        uint256 beforeBalance = _balanceOf(usdc, address(this));
        IAuthorizedUSDC(usdc)
            .receiveWithAuthorization(
                quote.payer,
                address(this),
                quote.usdcAmount,
                0,
                quote.expiresAt,
                id,
                uint8(authorization[64]),
                bytes32(authorization[0:32]),
                bytes32(authorization[32:64])
            );
        // A token that keeps a part of the transfer is refused: the record must equal what arrived.
        if (_balanceOf(usdc, address(this)) != beforeBalance + quote.usdcAmount) revert TokenBalanceMismatch();
    }

    /// @dev Everything a deposit needs except the money. A later deposit method checks the same and then
    /// collects in its own way. The board is checked the way the pool checks it (`won` is the pool's asset),
    /// so no deposit is taken for a board the pool is known to refuse.
    function _accept(ChargeTypes.Quote calldata quote, bytes calldata quoteSignature, uint8 method)
        private
        returns (bytes32 id)
    {
        if (block.chainid != chainId) revert WrongChain();
        if (depositsPaused) revert DepositsPaused();
        id = quoteDigest(quote);
        if (deposits[id].state != State.None) revert AlreadyDeposited();
        if (quoter == address(0) || ChargeTypes.recover(id, quoteSignature) != quoter) revert Unauthorized();
        if (
            quote.method != method || quote.usdc != usdc || quote.pool != pool || quote.won != won
                || quote.wonChainId != wonChainId || quote.board == address(0) || quote.board == pool
                || quote.board == won || quote.payer == address(0) || quote.payer == address(this) || quote.feeWon != 0
        ) revert InvalidQuote();
        if (
            quote.payBy != quote.expiresAt + ChargeTypes.PAY_WINDOW || block.timestamp >= quote.expiresAt
                || quote.expiresAt > block.timestamp + MAX_QUOTE_LIFE
        ) revert InvalidTime();
        if (quote.rate < minRate || quote.rate > maxRate) revert InvalidRate();
        if (quote.wonAmount != ChargeTypes.wonFor(quote.usdcAmount, quote.rate)) revert InvalidQuote();
        if (quote.wonAmount < minWon || quote.wonAmount > maxWon) revert InvalidAmount();

        Deposit storage record = deposits[id];
        record.payer = quote.payer;
        record.board = quote.board;
        record.usdcAmount = quote.usdcAmount;
        record.left = quote.usdcAmount;
        record.wonAmount = quote.wonAmount;
        record.rate = quote.rate;
        record.depositedAt = uint64(block.timestamp);
        record.payBy = quote.payBy;
        record.state = State.Deposited;
        totalHeld += quote.usdcAmount;
        emit Deposited(
            id,
            quote.payer,
            quote.board,
            quote.usdcAmount,
            quote.wonAmount,
            quote.rate,
            method,
            quote.expiresAt,
            quote.payBy,
            quote.number
        );
    }

    // ---- the pool paid ----

    /// @notice The relay records a payout. It needs the deposit-confirmation key's signature, which names the
    /// StableNet block of the payout. A deposit whose completion was once taken back needs two signers.
    function complete(bytes32 id, bytes32 wonBlockHash, uint256 until, bytes calldata witnessSignature) external {
        Deposit storage record = deposits[id];
        if (record.state != State.Deposited) revert InvalidState();
        if (msg.sender != relay || record.rejected) revert Unauthorized();
        _checkFresh(until);
        if (
            witness == address(0)
                || ChargeTypes.recover(paidDigest(id, wonBlockHash, until), witnessSignature) != witness
        ) revert Unauthorized();
        _complete(id, record, wonBlockHash, false);
    }

    function completeBySigners(bytes32 id, uint256 until, bytes[2] calldata signatures) external {
        Deposit storage record = deposits[id];
        if (record.state != State.Deposited) revert InvalidState();
        _checkTwo(COMPLETE, id, record.round, until, signatures);
        _complete(id, record, bytes32(0), true);
    }

    /// @notice The payer says the won never arrived: by its own transaction, or by its signature sent by anyone.
    function contest(bytes32 id, uint256 until, bytes calldata payerSignature) external {
        Deposit storage record = deposits[id];
        if (record.state != State.Completed || record.settled || record.left != record.usdcAmount) {
            revert InvalidState();
        }
        if (block.timestamp >= record.completedAt + CONTEST_WINDOW) revert InvalidTime();
        if (msg.sender != record.payer) {
            _checkFresh(until);
            if (ChargeTypes.recover(contestDigest(id, until), payerSignature) != record.payer) revert Unauthorized();
        }
        record.state = State.Contested;
        record.contestedAt = uint64(block.timestamp);
        record.round += 1;
        emit Contested(id, record.contestedAt + ANSWER_WINDOW);
    }

    /// @notice Two signers answer a contest: the won was paid. Final.
    function confirmPaid(bytes32 id, uint256 until, bytes[2] calldata signatures) external {
        Deposit storage record = deposits[id];
        if (record.state != State.Contested) revert InvalidState();
        if (block.timestamp >= record.contestedAt + ANSWER_WINDOW) revert InvalidTime();
        _checkTwo(CONFIRM_PAID, id, record.round, until, signatures);
        record.state = State.Completed;
        record.settled = true;
        record.completedAt = uint64(block.timestamp);
        record.round += 1;
        emit ContestAnswered(id);
    }

    /// @notice Two signers take a completion back: the won was not paid. The deposit is open again and can be
    /// refunded; the relay can no longer complete it.
    function reject(bytes32 id, uint256 until, bytes[2] calldata signatures) external {
        Deposit storage record = deposits[id];
        if (record.state == State.Contested) {
            if (block.timestamp >= record.contestedAt + ANSWER_WINDOW) revert InvalidTime();
        } else if (record.state != State.Completed || record.settled || record.left != record.usdcAmount) {
            revert InvalidState();
        }
        _checkTwo(REJECT, id, record.round, until, signatures);
        record.state = State.Deposited;
        record.rejected = true;
        record.rejectedAt = uint64(block.timestamp);
        record.round += 1;
        emit CompletionRejected(id);
    }

    // ---- the pool did not pay ----

    /// @notice Two signers send a deposit back. They sign the StableNet block in which they saw, after the
    /// payout deadline, that nothing was paid; the contract cannot check that block itself.
    function refundBySigners(bytes32 id, bytes32 wonBlockHash, uint256 until, bytes[2] calldata signatures)
        external
        guarded
    {
        Deposit storage record = deposits[id];
        if (record.state != State.Deposited) revert InvalidState();
        if (block.timestamp < uint256(record.payBy) + SIGNED_REFUND_DELAY) revert InvalidTime();
        _checkTwo(REFUND, keccak256(abi.encode(id, wonBlockHash)), record.round, until, signatures);
        _refund(id, record, RefundReason.Signers, wonBlockHash);
    }

    /// @notice Sends back a deposit nobody finished, or a contest nobody answered. Anyone may send it; the
    /// money only goes to the paying wallet.
    function refundSelf(bytes32 id) external guarded {
        Deposit storage record = deposits[id];
        uint64 opensAt = selfRefundAt(id);
        if (opensAt == 0) revert InvalidState();
        if (block.timestamp < opensAt) revert InvalidTime();
        RefundReason reason = record.state == State.Contested ? RefundReason.Unanswered : RefundReason.Unpaid;
        _refund(id, record, reason, bytes32(0));
    }

    // ---- completed money ----

    /// @notice The later contract that turns won back into USDC takes completed money here: only from a
    /// deposit that can no longer be contested, and never more than is left of it.
    function spend(bytes32 id, uint256 amount, address to) external guarded {
        if (spender == address(0) || msg.sender != spender) revert Unauthorized();
        Deposit storage record = deposits[id];
        if (record.state != State.Completed) revert InvalidState();
        if (!record.settled && block.timestamp < record.completedAt + CONTEST_WINDOW) revert InvalidTime();
        if (amount == 0 || amount > record.left || to == address(0) || to == address(this)) {
            revert InvalidParameters();
        }
        record.left -= amount;
        totalHeld -= amount;
        emit Spent(id, to, amount);
        _send(usdc, to, amount);
    }

    /// @notice A deployed contract or nothing. An account without code is a key, and so is an account whose
    /// code only names the contract a key lent it to (EIP-7702): that key can lend it elsewhere at any time.
    /// What a deployed contract lets its own keys do is for the two signers to read during the day of waiting.
    function setSpender(address to, bytes32 salt) external {
        _apply(SET_SPENDER, abi.encode(to), salt);
        if (to == address(this) || (to != address(0) && !_isContract(to))) revert InvalidParameters();
        spender = to;
        emit SpenderChanged(to);
    }

    /// @notice Stops the spending of completed money at once. Only two signers and a day name a spender again.
    function clearSpender() external onlyPauser {
        spender = address(0);
        emit SpenderChanged(address(0));
    }

    // ---- coins sent here by mistake ----

    /// @notice Two signers send back coins that arrived without a deposit, to the address they found as the
    /// sender in the named log. Each log is used once, and money of deposits can never leave this way.
    function returnStray(
        address token,
        address to,
        uint256 amount,
        bytes32 txHash,
        uint256 logIndex,
        uint256 until,
        bytes[2] calldata signatures
    ) external guarded {
        bytes32 log = keccak256(abi.encode(txHash, logIndex));
        if (strayReturned[log]) revert AlreadyReturned();
        if (token.code.length == 0 || to == address(0) || to == address(this) || amount == 0) {
            revert InvalidParameters();
        }
        _checkTwo(RETURN_STRAY, keccak256(abi.encode(token, to, amount, txHash, logIndex)), 0, until, signatures);
        if (token == usdc && _balanceOf(usdc, address(this)) < totalHeld + amount) revert NothingStray();
        strayReturned[log] = true;
        emit StrayReturned(token, to, amount, txHash, logIndex);
        _send(token, to, amount);
    }

    // ---- stopping and limits ----

    /// @notice Stops new deposits. Deposits that are in are still completed, contested and refunded.
    function pauseDeposits() external onlyPauser {
        depositsPaused = true;
        pauseRound += 1;
        emit DepositsPausedSet(true);
    }

    function unpauseDeposits(uint256 until, bytes[2] calldata signatures) external {
        _checkTwo(UNPAUSE, bytes32(0), pauseRound, until, signatures);
        depositsPaused = false;
        pauseRound += 1;
        emit DepositsPausedSet(false);
    }

    function lowerLimit(uint256 maxWon_) external onlyPauser {
        if (maxWon_ > maxWon || maxWon_ < minWon) revert InvalidParameters();
        maxWon = maxWon_;
        emit LimitChanged(maxWon_);
    }

    function raiseLimit(uint256 maxWon_, bytes32 salt) external {
        _apply(RAISE_LIMIT, abi.encode(maxWon_), salt);
        if (maxWon_ < minWon) revert InvalidParameters();
        maxWon = maxWon_;
        emit LimitChanged(maxWon_);
    }

    // ---- internals ----

    function _shape(bytes32 kind) internal pure override returns (uint256, uint256) {
        if (kind == SET_SPENDER) return (32, 1);
        if (kind == RAISE_LIMIT) return (32, 0);
        return super._shape(kind);
    }

    /// @dev Code that starts with 0xef0100 is the mark of a lent key account (EIP-7702), not a contract. No
    /// deployed contract starts with 0xef (EIP-3541).
    function _isContract(address account) private view returns (bool) {
        bytes memory code = account.code;
        // Only the first three bytes are looked at.
        // forge-lint: disable-next-line(unsafe-typecast)
        return code.length != 0 && bytes3(code) != bytes3(0xef0100);
    }

    function _complete(bytes32 id, Deposit storage record, bytes32 wonBlockHash, bool bySigners) private {
        record.state = State.Completed;
        record.completedAt = uint64(block.timestamp);
        record.round += 1;
        emit Completed(id, wonBlockHash, bySigners);
    }

    /// @dev If the token does not put the money into the payer's balance the call reverts and the deposit
    /// stays as it was, to be refunded later.
    function _refund(bytes32 id, Deposit storage record, RefundReason reason, bytes32 wonBlockHash) private {
        uint256 amount = record.left;
        record.state = State.Refunded;
        record.left = 0;
        record.round += 1;
        totalHeld -= amount;
        emit Refunded(id, record.payer, amount, reason, wonBlockHash);
        _send(usdc, record.payer, amount);
    }
}
