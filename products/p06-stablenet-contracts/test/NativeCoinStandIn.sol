// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

/// @notice What StableNet's own coin answers as a token at 0x1000, for tests on a chain that has no such coin.
/// The real one moves native balances and is part of the chain (NativeCoinAdapter of go-stablenet): its names, decimals,
/// signing domain, and the two signed transfers are the same here, and so is its refusal of blocked accounts. It has
/// no constructor, because tests put the code at that address. Anybody may mint or block. Never deployed.
contract NativeCoinStandIn {
    string public constant name = "WKRC";
    string public constant symbol = "WKRC";
    string public constant currency = "KRW";
    string public constant version = "1";
    uint8 public constant decimals = 18;
    bytes32 public constant TRANSFER_WITH_AUTHORIZATION_TYPEHASH = keccak256(
        "TransferWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"
    );
    bytes32 public constant RECEIVE_WITH_AUTHORIZATION_TYPEHASH = keccak256(
        "ReceiveWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"
    );
    bytes32 private constant DOMAIN_TYPEHASH =
        keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)");
    uint256 private constant HALF_ORDER = 0x7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0;

    uint256 public totalSupply;
    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(bytes32 => bool)) public authorizationState;
    mapping(address => bool) public isBlacklisted;

    event Transfer(address indexed from, address indexed to, uint256 value);
    event AuthorizationUsed(address indexed authorizer, bytes32 indexed nonce);

    modifier notBlacklisted(address account) {
        require(!isBlacklisted[account], "NativeCoinAdapter: account is blacklisted");
        _;
    }

    function DOMAIN_SEPARATOR() public view returns (bytes32) {
        return keccak256(
            abi.encode(DOMAIN_TYPEHASH, keccak256(bytes(name)), keccak256(bytes(version)), block.chainid, address(this))
        );
    }

    function mint(address to, uint256 amount) external {
        totalSupply += amount;
        balanceOf[to] += amount;
        emit Transfer(address(0), to, amount);
    }

    function setBlacklisted(address account, bool blocked) external {
        isBlacklisted[account] = blocked;
    }

    function transfer(address to, uint256 value) external notBlacklisted(msg.sender) notBlacklisted(to) returns (bool) {
        _transfer(msg.sender, to, value);
        return true;
    }

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
    ) external notBlacklisted(from) notBlacklisted(to) {
        _authorized(TRANSFER_WITH_AUTHORIZATION_TYPEHASH, from, to, value, validAfter, validBefore, nonce, v, r, s);
    }

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
    ) external notBlacklisted(from) notBlacklisted(to) {
        require(to == msg.sender, "NativeCoinAdapter: caller must be the payee");
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
        require(block.timestamp > validAfter, "NativeCoinAdapter: authorization is not yet valid");
        require(block.timestamp < validBefore, "NativeCoinAdapter: authorization is expired");
        require(!authorizationState[from][nonce], "NativeCoinAdapter: authorization is used or canceled");
        bytes32 content = keccak256(abi.encode(kind, from, to, value, validAfter, validBefore, nonce));
        address signer = ecrecover(keccak256(abi.encodePacked(hex"1901", DOMAIN_SEPARATOR(), content)), v, r, s);
        require(
            signer != address(0) && signer == from && uint256(s) <= HALF_ORDER, "NativeCoinAdapter: invalid signature"
        );
        authorizationState[from][nonce] = true;
        emit AuthorizationUsed(from, nonce);
        _transfer(from, to, value);
    }

    function _transfer(address from, address to, uint256 value) private {
        require(to != address(0), "NativeCoinAdapter: transfer to the zero address");
        require(value <= balanceOf[from], "NativeCoinAdapter: transfer amount exceeds balance");
        balanceOf[from] -= value;
        balanceOf[to] += value;
        emit Transfer(from, to, value);
    }
}
