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
///
/// Two keys in the wrong hands, the pauser and one signer, still stop each other's replacement and with it
/// every change. For that case there is the emergency exit: two signers propose it, it waits seven days, and
/// then anyone sends the money to the treasury, an address fixed at deployment. The pauser, a single signer,
/// a pause and a key switched off do not stop it. Only two signers together cancel it.
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

    /// @notice What `proposalStatus` answers. `Void`: lapsed, or its round has passed.
    enum Status {
        None,
        Waiting,
        Ready,
        Void,
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
    /// @notice The `until` of a signature must lie within this time of the block that uses it. This binds
    /// only the moment of use: it says nothing about when the signature was made, and a change proposed
    /// before `until` still waits its day and is applied after `until`.
    uint64 public constant LONGEST_SIGNATURE = 7 days;
    /// @notice A proposed emergency exit waits this long: seven times the wait of a change, so it is never the
    /// quicker way to do what a change does.
    uint64 public constant EMERGENCY_DELAY = 7 days;
    /// @notice After the wait, the exit is open for this long. On the deposit contract that is the contest
    /// window, so whatever was completed before the exit opened can be taken before it closes.
    uint64 public constant EMERGENCY_WINDOW = 7 days;

    bytes32 internal constant PROPOSE = keccak256("propose");
    bytes32 internal constant CANCEL = keccak256("cancel");
    bytes32 internal constant VOID_PAUSER = keccak256("voidPauserReplacements");
    bytes32 internal constant PROPOSE_EXIT = keccak256("proposeEmergencyExit");
    bytes32 internal constant CANCEL_EXIT = keccak256("cancelEmergencyExit");
    bytes32 internal constant SET_KEY = keccak256("setKey");
    bytes32 internal constant SET_SIGNER = keccak256("setSigner");
    bytes32 internal constant SET_RATES = keccak256("setRates");

    /// @notice The one address money goes to other than as a payout, a refund or through the spender. Fixed
    /// at deployment and never one of the seven keys.
    address public immutable treasury;
    address public relay;
    address public quoter;
    address public witness;
    address public pauser;
    address[3] public signers;
    /// @notice When the emergency exit that waits or is open was proposed.
    uint64 public exitProposedAt;
    /// @notice Grows with every proposal and every cancel of an emergency exit, so each signature works once.
    /// Nothing else moves it: not the pauser, not a key switched off, not a change of signers.
    uint256 public exitRound;
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
    /// @notice `by` is the seat that stopped it: the pauser or one of the three signers.
    event ChangeCancelled(bytes32 indexed action, Seat by);
    event ChangeApplied(bytes32 indexed action);
    /// @notice Every proposal that replaces the pauser and was counted in `round` is void.
    event PauserReplacementsVoided(uint256 round, Seat by);
    event EmergencyExitProposed(uint256 round, uint64 readyAt, uint64 lapsesAt);
    event EmergencyExitCancelled(uint256 round);
    /// @notice Money that left through the open exit. `id` is the deposit it was taken from; zero on the pool.
    event EmergencyExitTaken(bytes32 indexed id, address indexed treasury, uint256 amount);
    event RatesChanged(uint256 minRate, uint256 maxRate);

    error Unauthorized();
    error InvalidParameters();
    error InvalidState();
    error InvalidTime();
    error SignersNotDistinct();
    error TokenTransferFailed();
    error TokenBalanceMismatch();
    error Reentered();

    constructor(Keys memory keys, address treasury_, uint256 minRate_, uint256 maxRate_) {
        address[8] memory all = [
            keys.relay,
            keys.quoter,
            keys.witness,
            keys.pauser,
            keys.signers[0],
            keys.signers[1],
            keys.signers[2],
            treasury_
        ];
        for (uint256 i; i < all.length; ++i) {
            if (all[i] == address(0)) revert InvalidParameters();
            for (uint256 j; j < i; ++j) {
                if (all[i] == all[j]) revert InvalidParameters();
            }
        }
        // A fixed rate is the range with both ends equal.
        if (treasury_ == address(this) || minRate_ == 0 || minRate_ > maxRate_) revert InvalidParameters();
        treasury = treasury_;
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

    /// @notice What signers sign: the kind of action, what it is about, the round it is valid in and the time
    /// before which it must be used.
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
        _checkProposed(kind, arguments);
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
        Seat by = Seat.Pauser;
        if (msg.sender != pauser || signature.length != 0) {
            _checkFresh(until);
            bytes32 hash = actionDigest(CANCEL, action, proposal.round, until);
            by = _seatOfSigner(ChargeTypes.recover(hash, signature));
            if (by == Seat.None) revert Unauthorized();
        }
        if (by == proposal.replaces) revert Unauthorized();
        proposal.state = ProposalState.Cancelled;
        emit ChangeCancelled(action, by);
    }

    /// @notice Where a change stands now. `proposals(action)` keeps answering `Pending` for a change that
    /// lapsed or was voided; this reads the clock and the rounds as applying and cancelling do.
    function proposalStatus(bytes32 action) external view returns (Status) {
        Proposal storage proposal = proposals[action];
        if (proposal.state == ProposalState.None) return Status.None;
        if (proposal.state == ProposalState.Applied) return Status.Applied;
        if (proposal.state == ProposalState.Cancelled) return Status.Cancelled;
        if (proposal.round != _roundOf(proposal.replaces)) return Status.Void;
        return _statusAt(proposal.proposedAt, CHANGE_DELAY, CHANGE_WINDOW);
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
    /// be switched off again, so the pauser voids three times with the three keys of the deployment and once
    /// more for every key two signers bring back. The emergency exit is no change and is not touched.
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

    // ---- the emergency exit: two signers, then seven days ----

    /// @notice Starts the wait of the emergency exit. Two signers sign
    /// `actionDigest(keccak256("proposeEmergencyExit"), treasury, exitRound, until)`, with the treasury as a
    /// 32-byte word, and anyone may send it. One exit at a time: the next can be proposed once this one was
    /// cancelled or has lapsed. It is not a change: `cancelProposal`, a key switched off, a change of signers
    /// and a pause leave it as it is, and it leaves them as they are.
    function proposeEmergencyExit(uint256 until, bytes[2] calldata signatures) external {
        if (_exitWaitsOrIsOpen()) revert InvalidState();
        uint256 round = exitRound;
        _checkTwo(PROPOSE_EXIT, _exitSubject(), round, until, signatures);
        exitProposedAt = uint64(block.timestamp);
        exitRound = round + 1;
        uint64 readyAt = uint64(block.timestamp) + EMERGENCY_DELAY;
        emit EmergencyExitProposed(round, readyAt, readyAt + EMERGENCY_WINDOW);
    }

    /// @notice Two signers together stop the exit, while it waits or while it is open. Nobody else can: not
    /// the pauser, not one signer, not the two of them. They sign
    /// `actionDigest(keccak256("cancelEmergencyExit"), treasury, exitRound, until)`.
    function cancelEmergencyExit(uint256 until, bytes[2] calldata signatures) external {
        if (!_exitWaitsOrIsOpen()) revert InvalidState();
        uint256 round = exitRound;
        _checkTwo(CANCEL_EXIT, _exitSubject(), round, until, signatures);
        exitProposedAt = 0;
        exitRound = round + 1;
        emit EmergencyExitCancelled(round);
    }

    /// @notice `None`, `Waiting` for seven days, `Ready` while it is open, `Void` once it has lapsed.
    function emergencyExitStatus() public view returns (Status) {
        uint256 proposedAt = exitProposedAt;
        return proposedAt == 0 ? Status.None : _statusAt(proposedAt, EMERGENCY_DELAY, EMERGENCY_WINDOW);
    }

    function _exitWaitsOrIsOpen() private view returns (bool) {
        Status status = emergencyExitStatus();
        return status == Status.Waiting || status == Status.Ready;
    }

    function _exitSubject() private view returns (bytes32) {
        return bytes32(uint256(uint160(treasury)));
    }

    /// @dev Each contract sends its own money through the open exit. Nothing else is checked here on
    /// purpose: no pause and no key that is off closes it.
    function _checkExitOpen() internal view {
        Status status = emergencyExitStatus();
        if (status == Status.Waiting) revert InvalidTime();
        if (status != Status.Ready) revert InvalidState();
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

    /// @dev The shape of the ABI-encoded arguments of a kind of change: how many bytes they have, and which
    /// of their words is an address (1 is the first word, 0 is none). A length of 0 is a kind this contract
    /// does not have. Each contract adds its own kinds.
    function _shape(bytes32 kind) internal pure virtual returns (uint256 length, uint256 addressWord) {
        if (kind == SET_KEY || kind == SET_SIGNER) return (64, 2);
        if (kind == SET_RATES) return (64, 0);
    }

    /// @dev What a contract checks of a change already when it is proposed, beyond the shape of its arguments.
    function _checkProposed(bytes32 kind, bytes calldata arguments) internal view virtual {}

    /// @dev Whose place a change takes, read from its arguments when it is proposed. Only arguments that the
    /// changing function would encode itself are let in: a kind this contract has, the right length, a key
    /// or signer that exists and an address with nothing above its 20 bytes. So every waiting change names
    /// one call that exists. Whether that call then succeeds (a new key that is free, a rate above zero) is
    /// checked when it is applied: such a change waits and fails then.
    function _seatOf(bytes32 kind, bytes calldata arguments) private pure returns (Seat) {
        (uint256 length, uint256 addressWord) = _shape(kind);
        if (length == 0 || arguments.length != length) revert InvalidParameters();
        if (addressWord != 0 && uint256(bytes32(arguments[addressWord * 32 - 32:addressWord * 32])) >> 160 != 0) {
            revert InvalidParameters();
        }
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

    function _statusAt(uint256 startedAt, uint256 delay, uint256 window) internal view returns (Status) {
        if (block.timestamp < startedAt + delay) return Status.Waiting;
        return block.timestamp > startedAt + delay + window ? Status.Void : Status.Ready;
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
        if (
            to == address(0) || to == relay || to == quoter || to == witness || to == pauser || to == treasury
                || _isSigner(to)
        ) {
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
