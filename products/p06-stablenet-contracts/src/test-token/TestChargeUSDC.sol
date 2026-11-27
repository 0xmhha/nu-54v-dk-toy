// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

/// @title Test-only dollar token for top-ups: moves on a signature (EIP-3009) and always arrives.
/// @notice 6 decimals. Only the owner mints. Not a real asset. Unlike TestUSDC3009 it has no refusal mode, so a
/// refund always reaches the wallet it is sent to, and it answers `version()` so a signer reads the signing
/// domain from the token instead of assuming it. The version is "2" on purpose: a signer that assumes "1" fails.
contract TestChargeUSDC {
    string public constant name = "NU54 Charge Test USDC";
    string public constant symbol = "tUSDC";
    string public constant version = "2";
    uint8 public constant decimals = 6;
    bytes32 public constant TRANSFER_WITH_AUTHORIZATION_TYPEHASH = keccak256(
        "TransferWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"
    );
    bytes32 public constant RECEIVE_WITH_AUTHORIZATION_TYPEHASH = keccak256(
        "ReceiveWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"
    );
    bytes32 private constant DOMAIN_TYPEHASH =
        keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)");
    uint256 private constant HALF_ORDER = 0x7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0;

    address public immutable owner;
    uint256 public totalSupply;
    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => uint256)) public allowance;
    mapping(address => mapping(bytes32 => bool)) public authorizationState;

    event Transfer(address indexed from, address indexed to, uint256 value);
    event Approval(address indexed owner, address indexed spender, uint256 value);
    event AuthorizationUsed(address indexed authorizer, bytes32 indexed nonce);

    error NotOwner();
    error ZeroAddress();
    error InvalidRecipient();
    error InsufficientBalance();
    error InsufficientAllowance();
    error AuthorizationNotYetValid();
    error AuthorizationExpired();
    error AuthorizationAlreadyUsed();
    error CallerNotRecipient();
    error InvalidAuthorization();

    constructor(address owner_) {
        if (owner_ == address(0)) revert ZeroAddress();
        owner = owner_;
    }

    function DOMAIN_SEPARATOR() public view returns (bytes32) {
        return keccak256(
            abi.encode(DOMAIN_TYPEHASH, keccak256(bytes(name)), keccak256(bytes(version)), block.chainid, address(this))
        );
    }

    function mint(address to, uint256 amount) external {
        if (msg.sender != owner) revert NotOwner();
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
        _move(msg.sender, to, amount);
        return true;
    }

    function transferFrom(address from, address to, uint256 amount) external returns (bool) {
        uint256 allowed = allowance[from][msg.sender];
        if (allowed != type(uint256).max) {
            if (allowed < amount) revert InsufficientAllowance();
            allowance[from][msg.sender] = allowed - amount;
        }
        _move(from, to, amount);
        return true;
    }

    /// @notice Anyone may send the holder's signed transfer.
    function transferWithAuthorization(
        address from,
        address to,
        uint256 value,
        uint256 validAfter,
        uint256 validBefore,
        bytes32 nonce,
        uint8 v,
        bytes32 r,
        bytes32 s
    ) external {
        _authorized(TRANSFER_WITH_AUTHORIZATION_TYPEHASH, from, to, value, validAfter, validBefore, nonce, v, r, s);
    }

    /// @notice Only the named recipient can collect, so nobody can send it ahead of the recipient's own call.
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
    ) external {
        if (msg.sender != to) revert CallerNotRecipient();
        _authorized(RECEIVE_WITH_AUTHORIZATION_TYPEHASH, from, to, value, validAfter, validBefore, nonce, v, r, s);
    }

    function _authorized(
        bytes32 kind,
        address from,
        address to,
        uint256 value,
        uint256 validAfter,
        uint256 validBefore,
        bytes32 nonce,
        uint8 v,
        bytes32 r,
        bytes32 s
    ) private {
        if (block.timestamp <= validAfter) revert AuthorizationNotYetValid();
        if (block.timestamp >= validBefore) revert AuthorizationExpired();
        if (authorizationState[from][nonce]) revert AuthorizationAlreadyUsed();
        if ((v != 27 && v != 28) || uint256(s) > HALF_ORDER) revert InvalidAuthorization();
        bytes32 content = keccak256(abi.encode(kind, from, to, value, validAfter, validBefore, nonce));
        address signer = ecrecover(keccak256(abi.encodePacked(hex"1901", DOMAIN_SEPARATOR(), content)), v, r, s);
        if (signer == address(0) || signer != from) revert InvalidAuthorization();
        authorizationState[from][nonce] = true;
        emit AuthorizationUsed(from, nonce);
        _move(from, to, value);
    }

    function _move(address from, address to, uint256 amount) private {
        if (to == address(0) || to == address(this)) revert InvalidRecipient();
        if (balanceOf[from] < amount) revert InsufficientBalance();
        balanceOf[from] -= amount;
        balanceOf[to] += amount;
        emit Transfer(from, to, amount);
    }
}
