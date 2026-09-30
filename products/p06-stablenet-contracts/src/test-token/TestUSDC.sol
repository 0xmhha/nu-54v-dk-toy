// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

/// @title Test-only dollar token for the testnet and the sandbox ([N07]).
/// @notice 6 decimals like USDC. Only the owner mints. Not a real asset.
///
/// Right to refuse: an account can turn on refusal mode. While it is on, a transfer to the
/// account from an unknown sender does not reach the account's balance. The tokens move to
/// this contract and are recorded as a held transfer (step 1, TransferHeld). The recipient
/// then accepts it, rejects it (the tokens go straight back to the sender), or ignores it
/// (step 2). The sender cannot cancel a held transfer. An ignored transfer can be sent back
/// to its sender by the token team, only after RETURN_DELAY, once the team has checked it.
///
/// Known senders skip the hold: the team keeps a trusted-sender list (for example the
/// settlement contract, whose payouts only go to addresses the recipient registered) that
/// applies to every account by default, and each account can opt out of any entry.
///
/// Note for integrators: with refusal mode on, `transfer` returns true while the recipient's
/// balance does not change until the recipient accepts.
contract TestUSDC {
    /// @dev Two slots: (from, heldAt) and (to, amount).
    struct HeldTransfer {
        address from;
        uint64 heldAt; // 0 = no held transfer under this id
        address to;
        uint96 amount; // about 7.9e22 dollars at 6 decimals
    }

    string public constant name = "NU54 Test USD";
    string public constant symbol = "tUSDC";
    uint8 public constant decimals = 6;
    /// @notice The recipient has this long to accept or reject before the team may send back.
    uint256 public constant RETURN_DELAY = 3 days;

    address public immutable owner;
    uint256 public totalSupply;
    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => uint256)) public allowance;

    mapping(address => bool) public refusalMode;
    mapping(address => bool) public trustedSender; // team list, applies to every account
    mapping(address => mapping(address => bool)) public trustOptOut; // account => sender => opted out
    mapping(uint256 => HeldTransfer) internal held;
    uint256 public nextHeldId = 1;

    event Transfer(address indexed from, address indexed to, uint256 value);
    event Approval(address indexed owner, address indexed spender, uint256 value);
    event RefusalModeSet(address indexed account, bool enabled);
    event TrustedSenderSet(address indexed sender, bool trusted);
    event TrustOptOutSet(address indexed account, address indexed sender, bool optedOut);
    /// @notice Step 1: tokens for `to` are held; a notification service watches this event.
    event TransferHeld(uint256 indexed id, address indexed from, address indexed to, uint256 amount);
    event TransferAccepted(uint256 indexed id, address indexed from, address indexed to, uint256 amount);
    /// @notice The recipient refused; the sender has the tokens back.
    event TransferRejected(uint256 indexed id, address indexed from, address indexed to, uint256 amount);
    /// @notice The team sent an ignored transfer back to its sender after RETURN_DELAY.
    event TransferReturned(uint256 indexed id, address indexed from, address indexed to, uint256 amount);

    error NotOwner();
    error InsufficientBalance();
    error InsufficientAllowance();
    error ZeroAddress();
    error InvalidRecipient();
    error AmountTooLarge();
    error UnknownHeldTransfer();
    error NotRecipient();
    error ReturnNotYetAllowed();
    /// @notice Minting needs a recipient that can always receive: refusal mode must be off.
    error RecipientRefusing();

    constructor(address owner_) {
        if (owner_ == address(0)) revert ZeroAddress(); // no owner would mean no token ever minted
        owner = owner_;
    }

    modifier onlyOwner() {
        if (msg.sender != owner) revert NotOwner();
        _;
    }

    /// @dev "Always receives": fails early when `account` has refusal mode on, so the caller
    /// learns the account must turn it off first. Transfers never use this; they hold instead.
    modifier mustReceive(address account) {
        if (refusalMode[account]) revert RecipientRefusing();
        _;
    }

    /// @notice Mints only to an account that always receives (refusal mode off); otherwise
    /// RecipientRefusing, and the account must turn refusal mode off to be minted to.
    function mint(address to, uint256 amount) external onlyOwner mustReceive(to) {
        if (to == address(0) || to == address(this)) revert InvalidRecipient();
        totalSupply += amount;
        balanceOf[to] += amount;
        emit Transfer(address(0), to, amount);
    }

    function approve(address spender, uint256 amount) external returns (bool) {
        allowance[msg.sender][spender] = amount;
        emit Approval(msg.sender, spender, amount);
        return true;
    }

    function transfer(address to, uint256 amount) external returns (bool) {
        _send(msg.sender, to, amount);
        return true;
    }

    function transferFrom(address from, address to, uint256 amount) external returns (bool) {
        uint256 allowed = allowance[from][msg.sender];
        if (allowed != type(uint256).max) {
            if (allowed < amount) revert InsufficientAllowance();
            allowance[from][msg.sender] = allowed - amount;
        }
        _send(from, to, amount);
        return true;
    }

    // ---- right to refuse ----

    /// @notice Turns refusal mode on or off for the caller. Off by default. Transfers already
    /// held stay held either way.
    function setRefusalMode(bool enabled) external {
        refusalMode[msg.sender] = enabled;
        emit RefusalModeSet(msg.sender, enabled);
    }

    /// @notice Team list of known senders that skip the hold for every account by default.
    function setTrustedSender(address sender, bool trusted) external onlyOwner {
        if (sender == address(0)) revert ZeroAddress();
        trustedSender[sender] = trusted;
        emit TrustedSenderSet(sender, trusted);
    }

    /// @notice The caller stops (or resumes) trusting one entry of the team list.
    function setTrustOptOut(address sender, bool optedOut) external {
        trustOptOut[msg.sender][sender] = optedOut;
        emit TrustOptOutSet(msg.sender, sender, optedOut);
    }

    /// @notice True when a transfer from `sender` to `account` would be held.
    function wouldHold(address sender, address account) public view returns (bool) {
        if (!refusalMode[account] || sender == account) return false;
        return !(trustedSender[sender] && !trustOptOut[account][sender]);
    }

    /// @notice Step 2: the recipient takes the tokens.
    function acceptTransfer(uint256 id) external {
        HeldTransfer memory h = _takeHeld(id, true);
        _move(address(this), h.to, h.amount);
        emit TransferAccepted(id, h.from, h.to, h.amount);
    }

    /// @notice Step 2: the recipient refuses; the tokens go straight back to the sender.
    function rejectTransfer(uint256 id) external {
        HeldTransfer memory h = _takeHeld(id, true);
        _move(address(this), h.from, h.amount); // back to the sender, whatever its own mode
        emit TransferRejected(id, h.from, h.to, h.amount);
    }

    /// @notice An ignored transfer, reported as a mistaken transfer and checked by the team,
    /// goes back to its sender. Only after RETURN_DELAY, and only to the original sender.
    function returnToSender(uint256 id) external onlyOwner {
        HeldTransfer memory h = _takeHeld(id, false);
        if (block.timestamp < uint256(h.heldAt) + RETURN_DELAY) revert ReturnNotYetAllowed();
        _move(address(this), h.from, h.amount);
        emit TransferReturned(id, h.from, h.to, h.amount);
    }

    function heldTransfer(uint256 id) external view returns (address from, address to, uint256 amount, uint256 heldAt) {
        HeldTransfer memory h = held[id];
        return (h.from, h.to, h.amount, h.heldAt);
    }

    // ---- internals ----

    function _send(address from, address to, uint256 amount) internal {
        // Held tokens sit in this contract's balance; a direct transfer there would be lost among them.
        if (to == address(0) || to == address(this)) revert InvalidRecipient();
        // A zero transfer delivers nothing, so it is never held (and cannot spam notifications).
        if (amount == 0 || !wouldHold(from, to)) {
            _move(from, to, amount);
            return;
        }
        if (amount > type(uint96).max) revert AmountTooLarge();
        _move(from, address(this), amount); // step 1: the tokens leave the sender now
        uint256 id = nextHeldId++;
        // forge-lint: disable-next-line(unsafe-typecast)
        held[id] = HeldTransfer({from: from, heldAt: uint64(block.timestamp), to: to, amount: uint96(amount)});
        emit TransferHeld(id, from, to, amount);
    }

    function _takeHeld(uint256 id, bool byRecipient) internal returns (HeldTransfer memory h) {
        h = held[id];
        if (h.heldAt == 0) revert UnknownHeldTransfer();
        if (byRecipient && msg.sender != h.to) revert NotRecipient();
        delete held[id]; // state first, so each held transfer resolves once
    }

    function _move(address from, address to, uint256 amount) internal {
        if (balanceOf[from] < amount) revert InsufficientBalance();
        balanceOf[from] -= amount;
        balanceOf[to] += amount;
        emit Transfer(from, to, amount);
    }
}
