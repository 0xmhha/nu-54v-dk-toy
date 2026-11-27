// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

/// @title Runs an action a user signed, with the gas paid by the store.
/// @notice The store's relay sends its transaction here. The user needs no gas and never signs a transaction
/// that anyone could hold back for later: what a user signs always ends at a time of its own.
///
/// Two kinds of action, each named by contract and method in a list the owner keeps:
/// - SIGNED: the target checks the user's signature inside its own call, such as `OrderEscrow.lock` or
///   `transferWithAuthorization`. This contract only passes it on.
/// - USER: the user signs the exact call here (EIP-712). It runs with the user's address after the calldata
///   (ERC-2771), so the target must be written to trust this contract as a forwarder.
///
/// This contract holds no money and takes no approval. A target never sees the user as the caller, so a method
/// that acts for its caller, such as `transfer` or `approve` of a token, must not be listed as either kind.
contract GasSponsor {
    enum Kind {
        NONE,
        SIGNED,
        USER
    }

    struct Request {
        address user;
        address target;
        bytes data;
        uint256 gas; // the least the action gets, so a relay cannot make it fail by starving it
        uint64 validBefore;
        bytes32 nonce; // any unused value; requests of one user do not wait for each other
    }

    bytes32 public constant REQUEST_TYPEHASH =
        keccak256("SponsoredCall(address user,address target,bytes data,uint256 gas,uint64 validBefore,bytes32 nonce)");
    bytes32 private constant DOMAIN_TYPEHASH =
        keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)");
    uint256 private constant HALF_ORDER = 0x7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0;
    /// @notice A user's request cannot be kept for longer than this.
    uint64 public constant MAX_LIFE = 10 minutes;

    address public immutable owner;
    bool private entered;
    mapping(address => bool) public relays;
    mapping(address => mapping(bytes4 => Kind)) public actions;
    mapping(address => mapping(bytes32 => bool)) public used;

    event RelaySet(address indexed relay, bool allowed);
    event ActionSet(address indexed target, bytes4 indexed selector, Kind kind);
    /// @notice `user` is zero for a SIGNED action, whose target names the user itself.
    event Sponsored(
        address indexed relay, address indexed target, bytes4 indexed selector, address user, bytes32 nonce
    );

    error NotOwner();
    error NotRelay();
    error InvalidParameters();
    error ActionNotAllowed();
    error RequestExpired();
    error RequestTooLong();
    error RequestUsed();
    error InvalidSignature();
    error NotEnoughGas();
    error Reentered();

    constructor(address owner_) {
        if (owner_ == address(0)) revert InvalidParameters();
        owner = owner_;
    }

    modifier onlyOwner() {
        if (msg.sender != owner) revert NotOwner();
        _;
    }

    modifier relayed() {
        if (!relays[msg.sender]) revert NotRelay();
        if (entered) revert Reentered();
        entered = true;
        _;
        entered = false;
    }

    function setRelay(address relay, bool allowed) external onlyOwner {
        if (relay == address(0)) revert InvalidParameters();
        relays[relay] = allowed;
        emit RelaySet(relay, allowed);
    }

    function setActions(address target, bytes4[] calldata selectors, Kind kind) external onlyOwner {
        if (target == address(this) || (kind != Kind.NONE && target.code.length == 0)) revert InvalidParameters();
        for (uint256 i; i < selectors.length; i++) {
            actions[target][selectors[i]] = kind;
            emit ActionSet(target, selectors[i], kind);
        }
    }

    function DOMAIN_SEPARATOR() public view returns (bytes32) {
        return keccak256(
            abi.encode(DOMAIN_TYPEHASH, keccak256("NU54GasSponsor"), keccak256("1"), block.chainid, address(this))
        );
    }

    function requestDigest(Request calldata request) public view returns (bytes32) {
        bytes32 body = keccak256(
            abi.encode(
                REQUEST_TYPEHASH,
                request.user,
                request.target,
                keccak256(request.data),
                request.gas,
                request.validBefore,
                request.nonce
            )
        );
        return keccak256(abi.encodePacked(hex"1901", DOMAIN_SEPARATOR(), body));
    }

    /// @notice Passes on a call whose target checks the user's signature itself.
    function sponsor(address target, bytes calldata data) external relayed returns (bytes memory) {
        bytes4 selector = _selector(data);
        if (actions[target][selector] != Kind.SIGNED) revert ActionNotAllowed();
        emit Sponsored(msg.sender, target, selector, address(0), bytes32(0));
        return _call(target, data);
    }

    /// @notice Checks the user's request and runs it for that user, once.
    function execute(Request calldata request, bytes calldata signature) external relayed returns (bytes memory) {
        bytes4 selector = _selector(request.data);
        if (actions[request.target][selector] != Kind.USER) revert ActionNotAllowed();
        if (block.timestamp >= request.validBefore) revert RequestExpired();
        if (request.validBefore > block.timestamp + MAX_LIFE) revert RequestTooLong();
        if (used[request.user][request.nonce]) revert RequestUsed();
        if (_signer(requestDigest(request), signature) != request.user) revert InvalidSignature();
        used[request.user][request.nonce] = true;
        // A call passes on at most 63/64 of the gas.
        if (gasleft() < request.gas + request.gas / 63 + 10_000) revert NotEnoughGas();
        emit Sponsored(msg.sender, request.target, selector, request.user, request.nonce);
        return _call(request.target, abi.encodePacked(request.data, request.user));
    }

    function _selector(bytes calldata data) private pure returns (bytes4) {
        if (data.length < 4) revert ActionNotAllowed();
        return bytes4(data[:4]);
    }

    /// @dev The target's own refusal comes back unchanged, so a relay sees why.
    function _call(address target, bytes memory data) private returns (bytes memory) {
        (bool ok, bytes memory answer) = target.call(data);
        if (!ok) {
            assembly ("memory-safe") {
                revert(add(answer, 32), mload(answer))
            }
        }
        return answer;
    }

    function _signer(bytes32 digest, bytes calldata signature) private pure returns (address signer) {
        if (signature.length != 65) revert InvalidSignature();
        bytes32 r = bytes32(signature[:32]);
        bytes32 s = bytes32(signature[32:64]);
        uint8 v = uint8(signature[64]);
        if ((v != 27 && v != 28) || uint256(s) > HALF_ORDER) revert InvalidSignature();
        signer = ecrecover(digest, v, r, s);
        if (signer == address(0)) revert InvalidSignature();
    }
}
