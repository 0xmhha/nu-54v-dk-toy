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
///
/// A waiting change is stopped by the pauser or by one signer. The one exception keeps a single key in the
/// wrong hands from staying for ever: the key whose place a change takes cannot stop that change. The pauser
/// cannot cancel or void its own replacement, and a signer cannot cancel its own.
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

    /// @notice Whose place a waiting change takes. That key alone cannot stop the change.
    enum Seat {
        None,
        Pauser,
        Signer0,
        Signer1,
        Signer2
    }

    struct Proposal {
        uint64 proposedAt;
        ProposalState state;
        Seat replaces;
        uint256 round; // the generation it was proposed in; see `proposalRound`
    }

    /// @notice A proposed change waits this long.
    uint64 public constant CHANGE_DELAY = 24 hours;
    /// @notice After the wait, a change can be applied for this long.
    uint64 public constant CHANGE_WINDOW = 3 days;
    /// @notice No signature may be kept for longer than this.
    uint64 public constant LONGEST_SIGNATURE = 7 days;

    bytes32 internal constant PROPOSE = keccak256("propose");
    bytes32 internal constant CANCEL = keccak256("cancel");
    bytes32 internal constant VOID_PAUSER = keccak256("voidPauserReplacements");
    bytes32 internal constant SET_KEY = keccak256("setKey");
    bytes32 internal constant SET_SIGNER = keccak256("setSigner");
    bytes32 internal constant SET_RATES = keccak256("setRates");

    address public relay;
    address public quoter;
    address public witness;
    address public pauser;
    address[3] public signers;
    /// @notice Grows when a signer changes or a key is switched off; every earlier proposal is then void,
    /// except one that replaces the pauser.
    uint256 public generation;
    /// @notice The round of the pauser's seat. A proposal that replaces the pauser is counted in this one, so
    /// the pauser cannot void it by switching keys off. It grows when a signer changes, when a replacement of
    /// the pauser is applied and when one signer voids them all: every proposal for that seat is then void.
    uint256 public pauserRound;
    /// @notice The quoted rate must lie in this range, in 0.01 won for one whole USDC.
    uint256 public minRate;
    uint256 public maxRate;
    mapping(bytes32 => Proposal) public proposals;
    bool private entered;

    event KeyChanged(Key indexed which, address account);
    event SignerChanged(uint8 indexed index, address account);
    /// @notice Everything a reviewer needs to see what waits: `kind` is the keccak256 of the name of the
    /// changing function and `arguments` are its arguments without the salt, ABI-encoded.
    event ChangeProposed(
        bytes32 indexed action,
        bytes32 indexed kind,
        bytes arguments,
        bytes32 salt,
        uint256 round,
        uint64 readyAt,
        uint64 lapsesAt
    );
    event ChangeCancelled(bytes32 indexed action);
    event ChangeApplied(bytes32 indexed action);
    /// @notice Every proposal that replaces the pauser and was counted in `round` is void.
    event PauserReplacementsVoided(uint256 round, Seat by);
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

    /// @notice The number of one change: its kind, its arguments and a salt, which makes two equal changes
    /// two numbers. It equals `keccak256(abi.encode(kind, arguments..., salt))`.
    function actionOf(bytes32 kind, bytes memory arguments, bytes32 salt) public pure returns (bytes32) {
        return keccak256(bytes.concat(kind, arguments, salt));
    }

    /// @notice The round two signers put into the proposal of a change, and one signer into its cancel.
    /// Refuses a kind this contract does not have and arguments that are not of that kind.
    function proposalRound(bytes32 kind, bytes calldata arguments) public view returns (uint256) {
        return _roundOf(_seatOf(kind, arguments));
    }

    // ---- changes that widen: two signers, then a day ----

    /// @notice Starts the wait of one change. `kind` is the keccak256 of the name of the changing function
    /// and `arguments` are that function's arguments without the salt, ABI-encoded. Two signers sign
    /// `actionDigest(keccak256("propose"), actionOf(kind, arguments, salt), proposalRound(kind, arguments),
    /// until)`. Anyone may send it; the two signatures are what counts.
    function propose(bytes32 kind, bytes calldata arguments, bytes32 salt, uint256 until, bytes[2] calldata signatures)
        external
        returns (bytes32 action)
    {
        Seat seat = _seatOf(kind, arguments);
        uint256 round = _roundOf(seat);
        action = actionOf(kind, arguments, salt);
        // Once per action and round: a lapsed or cancelled change needs a new salt and new signatures.
        if (proposals[action].state != ProposalState.None && proposals[action].round == round) revert InvalidState();
        _checkTwo(PROPOSE, action, round, until, signatures);
        uint64 readyAt = uint64(block.timestamp) + CHANGE_DELAY;
        proposals[action] =
            Proposal({proposedAt: uint64(block.timestamp), state: ProposalState.Pending, replaces: seat, round: round});
        emit ChangeProposed(action, kind, arguments, salt, round, readyAt, readyAt + CHANGE_WINDOW);
    }

    /// @notice The pauser, or one signer by signature, stops a waiting change for good. Not the key whose
    /// place the change takes: the pauser cannot stop its own replacement, nor a signer its own. The pauser
    /// calls with an empty signature; a signer signs `actionDigest(keccak256("cancel"), action, round, until)`
    /// with the round of the proposal, and anyone may send it.
    function cancelProposal(bytes32 action, uint256 until, bytes calldata signature) external {
        Proposal storage proposal = proposals[action];
        if (!_waits(proposal)) revert InvalidState();
        if (msg.sender == pauser && signature.length == 0) {
            if (proposal.replaces == Seat.Pauser) revert Unauthorized();
        } else {
            _checkFresh(until);
            bytes32 hash = actionDigest(CANCEL, action, proposal.round, until);
            Seat seat = _seatOfSigner(ChargeTypes.recover(hash, signature));
            if (seat == Seat.None || seat == proposal.replaces) revert Unauthorized();
        }
        proposal.state = ProposalState.Cancelled;
        emit ChangeCancelled(action);
    }

    /// @notice One signer stops every waiting replacement of the pauser at once, however many there are. It
    /// is no new power: a signer already stops each of them one by one, and the pauser can stop none of
    /// them. The signer signs `actionDigest(keccak256("voidPauserReplacements"), 0, pauserRound, until)` and
    /// anyone may send it. The round then moves, so the signature works once.
    function voidPauserReplacements(uint256 until, bytes calldata signature) external {
        _checkFresh(until);
        uint256 round = pauserRound;
        Seat seat = _seatOfSigner(ChargeTypes.recover(actionDigest(VOID_PAUSER, bytes32(0), round, until), signature));
        if (seat == Seat.None) revert Unauthorized();
        pauserRound = round + 1;
        emit PauserReplacementsVoided(round, seat);
    }

    function setKey(Key which, address to, bytes32 salt) external {
        _apply(SET_KEY, abi.encode(which, to), salt);
        _checkUnused(to);
        if (which == Key.Relay) {
            relay = to;
        } else if (which == Key.Quoter) {
            quoter = to;
        } else if (which == Key.Witness) {
            witness = to;
        } else {
            // The new pauser cannot stop a change that takes its place, so none is left waiting behind it.
            pauser = to;
            pauserRound += 1;
        }
        emit KeyChanged(which, to);
    }

    function setSigner(uint8 index, address to, bytes32 salt) external {
        _apply(SET_SIGNER, abi.encode(index, to), salt);
        _checkUnused(to);
        signers[index] = to;
        generation += 1;
        pauserRound += 1;
        emit SignerChanged(index, to);
    }

    function setRates(uint256 minRate_, uint256 maxRate_, bytes32 salt) external {
        _apply(SET_RATES, abi.encode(minRate_, maxRate_), salt);
        if (minRate_ == 0 || minRate_ > maxRate_) revert InvalidParameters();
        minRate = minRate_;
        maxRate = maxRate_;
        emit RatesChanged(minRate_, maxRate_);
    }

    // ---- changes that narrow: the pauser alone ----

    /// @notice Switches off the relay, the quoter or the witness. Only two signers and a day bring a key back.
    /// Every waiting change is void after it, except one that replaces the pauser. A key that is off cannot
    /// be switched off again, so the pauser voids once for every key two signers brought back.
    function disableKey(Key which) external onlyPauser {
        address was;
        if (which == Key.Relay) {
            was = relay;
            relay = address(0);
        } else if (which == Key.Quoter) {
            was = quoter;
            quoter = address(0);
        } else if (which == Key.Witness) {
            was = witness;
            witness = address(0);
        } else {
            revert InvalidParameters();
        }
        if (was == address(0)) revert InvalidState();
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
        return _seatOfSigner(account) != Seat.None;
    }

    function _seatOfSigner(address account) private view returns (Seat) {
        if (account == signers[0]) return Seat.Signer0;
        if (account == signers[1]) return Seat.Signer1;
        if (account == signers[2]) return Seat.Signer2;
        return Seat.None;
    }

    /// @dev How many bytes the ABI-encoded arguments of a kind of change have; 0 for a kind this contract does
    /// not have. Each contract adds its own kinds.
    function _argumentsLength(bytes32 kind) internal pure virtual returns (uint256) {
        return kind == SET_KEY || kind == SET_SIGNER || kind == SET_RATES ? 64 : 0;
    }

    /// @dev Whose place a change takes, read from its arguments when it is proposed. Only a change this
    /// contract can apply is let in, so nothing unreadable ever waits.
    function _seatOf(bytes32 kind, bytes calldata arguments) private pure returns (Seat) {
        uint256 length = _argumentsLength(kind);
        if (length == 0 || arguments.length != length) revert InvalidParameters();
        if (kind != SET_KEY && kind != SET_SIGNER) return Seat.None;
        uint256 first = uint256(bytes32(arguments[0:32]));
        if (kind == SET_KEY) {
            if (first > uint256(Key.Pauser)) revert InvalidParameters();
            return first == uint256(Key.Pauser) ? Seat.Pauser : Seat.None;
        }
        if (first > 2) revert InvalidParameters();
        return Seat(first + uint256(Seat.Signer0));
    }

    /// @dev The pauser moves `generation` by switching a key off. Its own replacement is therefore counted
    /// in a round that the pauser cannot move.
    function _roundOf(Seat seat) private view returns (uint256) {
        return seat == Seat.Pauser ? pauserRound : generation;
    }

    function _waits(Proposal storage proposal) private view returns (bool) {
        return proposal.state == ProposalState.Pending && proposal.round == _roundOf(proposal.replaces);
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

    function _apply(bytes32 kind, bytes memory arguments, bytes32 salt) internal {
        bytes32 action = actionOf(kind, arguments, salt);
        Proposal storage proposal = proposals[action];
        if (!_waits(proposal)) revert InvalidState();
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
