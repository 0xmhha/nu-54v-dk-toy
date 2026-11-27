// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {ChargeTypes} from "./ChargeTypes.sol";

/// @title The keys of a top-up contract and the only ways to change them.
/// @notice No owner and no upgrade. Seven different keys:
/// - relay: sends the transactions and pays the gas,
/// - quoter: signs rate quotes,
/// - witness: the deposit-confirmation key, never the relay or the quoter,
/// - three signers: any two of them repair, refund and approve changes,
/// - pauser: one key that only stops or narrows.
///
/// Whatever widens (a higher limit, a wider rate range, a new key, money out of the pool) needs two signers and
/// then a day of waiting, and lapses three days later. Whatever narrows or stops needs one key and no waiting.
abstract contract ChargeGovernance {
    enum Key {
        Relay,
        Quoter,
        Witness,
        Pauser
    }

    enum ProposalState {
        None,
        Pending,
        Applied,
        Cancelled
    }

    struct Keys {
        address relay;
        address quoter;
        address witness;
        address pauser;
        address[3] signers;
    }

    struct Proposal {
        uint64 proposedAt;
        ProposalState state;
        uint256 generation;
    }

    /// @notice A proposed change waits this long.
    uint64 public constant CHANGE_DELAY = 24 hours;
    /// @notice After the wait, a change can be applied for this long.
    uint64 public constant CHANGE_WINDOW = 3 days;
    /// @notice No signature may be kept for longer than this.
    uint64 public constant LONGEST_SIGNATURE = 7 days;

    bytes32 internal constant PROPOSE = keccak256("propose");
    bytes32 internal constant CANCEL = keccak256("cancel");
    bytes32 internal constant SET_KEY = keccak256("setKey");
    bytes32 internal constant SET_SIGNER = keccak256("setSigner");
    bytes32 internal constant SET_RATES = keccak256("setRates");

    address public relay;
    address public quoter;
    address public witness;
    address public pauser;
    address[3] public signers;
    /// @notice Grows when a signer changes or a key is switched off; every earlier proposal is then void.
    uint256 public generation;
    /// @notice The quoted rate must lie in this range, in 0.01 won for one whole USDC.
    uint256 public minRate;
    uint256 public maxRate;
    mapping(bytes32 => Proposal) public proposals;
    bool private entered;

    event KeyChanged(Key indexed which, address account);
    event SignerChanged(uint8 indexed index, address account);
    event ChangeProposed(bytes32 indexed action, uint64 readyAt, uint64 lapsesAt);
    event ChangeCancelled(bytes32 indexed action);
    event ChangeApplied(bytes32 indexed action);
    event RatesChanged(uint256 minRate, uint256 maxRate);

    error Unauthorized();
    error InvalidParameters();
    error InvalidState();
    error InvalidTime();
    error SignersNotDistinct();
    error TokenTransferFailed();
    error TokenBalanceMismatch();
    error Reentered();

    constructor(Keys memory keys, uint256 minRate_, uint256 maxRate_) {
        address[7] memory all =
            [keys.relay, keys.quoter, keys.witness, keys.pauser, keys.signers[0], keys.signers[1], keys.signers[2]];
        for (uint256 i; i < all.length; ++i) {
            if (all[i] == address(0)) revert InvalidParameters();
            for (uint256 j; j < i; ++j) {
                if (all[i] == all[j]) revert InvalidParameters();
            }
        }
        // A fixed rate is the range with both ends equal.
        if (minRate_ == 0 || minRate_ > maxRate_) revert InvalidParameters();
        relay = keys.relay;
        quoter = keys.quoter;
        witness = keys.witness;
        pauser = keys.pauser;
        signers = keys.signers;
        minRate = minRate_;
        maxRate = maxRate_;
    }

    modifier guarded() {
        if (entered) revert Reentered();
        entered = true;
        _;
        entered = false;
    }

    modifier onlyPauser() {
        if (msg.sender != pauser) revert Unauthorized();
        _;
    }

    // ---- digests ----

    function domainSeparator() public view returns (bytes32) {
        return ChargeTypes.domain(block.chainid, address(this));
    }

    /// @notice What signers sign: the kind of action, what it is about, the round it is valid in and its end.
    function actionDigest(bytes32 kind, bytes32 subject, uint256 round, uint256 until) public view returns (bytes32) {
        return ChargeTypes.digest(
            domainSeparator(), keccak256(abi.encode(ChargeTypes.ACTION_TYPEHASH, kind, subject, round, until))
        );
    }

    // ---- changes that widen: two signers, then a day ----

    /// @notice Starts the wait of one change. `action` is the hash the changing function computes from its
    /// arguments and a salt. Anyone may send it; the two signatures are what counts.
    function propose(bytes32 action, uint256 until, bytes[2] calldata signatures) external {
        Proposal storage proposal = proposals[action];
        // Once per action and generation: a lapsed or cancelled change needs a new salt and new signatures.
        if (proposal.state != ProposalState.None && proposal.generation == generation) revert InvalidState();
        _checkTwo(PROPOSE, action, generation, until, signatures);
        uint64 readyAt = uint64(block.timestamp) + CHANGE_DELAY;
        proposals[action] =
            Proposal({proposedAt: uint64(block.timestamp), state: ProposalState.Pending, generation: generation});
        emit ChangeProposed(action, readyAt, readyAt + CHANGE_WINDOW);
    }

    /// @notice The pauser, or one signer by signature, stops a waiting change for good.
    function cancelProposal(bytes32 action, uint256 until, bytes calldata signature) external {
        Proposal storage proposal = proposals[action];
        if (proposal.state != ProposalState.Pending || proposal.generation != generation) revert InvalidState();
        if (msg.sender != pauser) {
            _checkFresh(until);
            address signer = ChargeTypes.recover(actionDigest(CANCEL, action, generation, until), signature);
            if (!_isSigner(signer)) revert Unauthorized();
        }
        proposal.state = ProposalState.Cancelled;
        emit ChangeCancelled(action);
    }

    function setKey(Key which, address to, bytes32 salt) external {
        _apply(keccak256(abi.encode(SET_KEY, which, to, salt)));
        _checkUnused(to);
        if (which == Key.Relay) relay = to;
        else if (which == Key.Quoter) quoter = to;
        else if (which == Key.Witness) witness = to;
        else pauser = to;
        emit KeyChanged(which, to);
    }

    function setSigner(uint8 index, address to, bytes32 salt) external {
        _apply(keccak256(abi.encode(SET_SIGNER, index, to, salt)));
        if (index > 2) revert InvalidParameters();
        _checkUnused(to);
        signers[index] = to;
        generation += 1;
        emit SignerChanged(index, to);
    }

    function setRates(uint256 minRate_, uint256 maxRate_, bytes32 salt) external {
        _apply(keccak256(abi.encode(SET_RATES, minRate_, maxRate_, salt)));
        if (minRate_ == 0 || minRate_ > maxRate_) revert InvalidParameters();
        minRate = minRate_;
        maxRate = maxRate_;
        emit RatesChanged(minRate_, maxRate_);
    }

    // ---- changes that narrow: the pauser alone ----

    /// @notice Switches off the relay, the quoter or the witness. Only two signers and a day bring a key back.
    function disableKey(Key which) external onlyPauser {
        if (which == Key.Relay) relay = address(0);
        else if (which == Key.Quoter) quoter = address(0);
        else if (which == Key.Witness) witness = address(0);
        else revert InvalidParameters();
        generation += 1;
        emit KeyChanged(which, address(0));
    }

    function narrowRates(uint256 minRate_, uint256 maxRate_) external onlyPauser {
        if (minRate_ < minRate || maxRate_ > maxRate || minRate_ > maxRate_) revert InvalidParameters();
        minRate = minRate_;
        maxRate = maxRate_;
        emit RatesChanged(minRate_, maxRate_);
    }

    // ---- internals ----

    function _isSigner(address account) internal view returns (bool) {
        return account == signers[0] || account == signers[1] || account == signers[2];
    }

    function _checkFresh(uint256 until) internal view {
        if (block.timestamp >= until || until > block.timestamp + LONGEST_SIGNATURE) revert InvalidTime();
    }

    function _checkTwo(bytes32 kind, bytes32 subject, uint256 round, uint256 until, bytes[2] calldata signatures)
        internal
        view
    {
        _checkFresh(until);
        bytes32 hash = actionDigest(kind, subject, round, until);
        address first = ChargeTypes.recover(hash, signatures[0]);
        address second = ChargeTypes.recover(hash, signatures[1]);
        if (first == second) revert SignersNotDistinct();
        if (!_isSigner(first) || !_isSigner(second)) revert Unauthorized();
    }

    function _apply(bytes32 action) internal {
        Proposal storage proposal = proposals[action];
        if (proposal.state != ProposalState.Pending || proposal.generation != generation) revert InvalidState();
        uint256 readyAt = uint256(proposal.proposedAt) + CHANGE_DELAY;
        if (block.timestamp < readyAt || block.timestamp > readyAt + CHANGE_WINDOW) revert InvalidTime();
        proposal.state = ProposalState.Applied;
        emit ChangeApplied(action);
    }

    function _checkUnused(address to) private view {
        if (to == address(0) || to == relay || to == quoter || to == witness || to == pauser || _isSigner(to)) {
            revert InvalidParameters();
        }
    }

    /// @dev A token that answers false, answers nothing useful or moves another amount reverts the whole
    /// call, so no record ever says money moved when it did not. The token's own refusal comes back unchanged.
    function _send(address token, address to, uint256 amount) internal {
        uint256 beforeBalance = _balanceOf(token, to);
        (bool ok, bytes memory answer) = token.call(abi.encodeWithSignature("transfer(address,uint256)", to, amount));
        if (!ok) {
            if (answer.length == 0) revert TokenTransferFailed();
            assembly ("memory-safe") {
                revert(add(answer, 32), mload(answer))
            }
        }
        if (answer.length != 0 && (answer.length != 32 || abi.decode(answer, (uint256)) != 1)) {
            revert TokenTransferFailed();
        }
        if (_balanceOf(token, to) != beforeBalance + amount) revert TokenBalanceMismatch();
    }

    function _balanceOf(address token, address account) internal view returns (uint256) {
        (bool ok, bytes memory answer) = token.staticcall(abi.encodeWithSignature("balanceOf(address)", account));
        if (!ok || answer.length != 32) revert TokenTransferFailed();
        return abi.decode(answer, (uint256));
    }
}
