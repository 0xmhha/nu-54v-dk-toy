// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {TestToken3009} from "./TestToken3009.sol";

/// @title Test-only won token that a store's order contract can collect (EIP-3009).
/// @notice 2 decimals: 100 units are 1 won. Not a real asset. Adds the recipient-only transfer an
/// order contract needs to TestToken3009; both kinds share one nonce space per holder.
contract TestKRW3009 is TestToken3009 {
    bytes32 public constant RECEIVE_WITH_AUTHORIZATION_TYPEHASH = keccak256(
        "ReceiveWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"
    );
    uint256 private constant HALF_ORDER = 0x7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0;

    /// @notice The version of the signing domain.
    string public constant version = "1";

    error CallerNotRecipient();

    constructor(address owner_) TestToken3009(owner_, "NU54 Test KRW", "tKRW", 2) {}

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
        if (block.timestamp <= validAfter) revert AuthorizationNotYetValid();
        if (block.timestamp >= validBefore) revert AuthorizationExpired();
        if (authorizationState[from][nonce]) revert AuthorizationAlreadyUsed();
        if ((v != 27 && v != 28) || uint256(s) > HALF_ORDER) revert InvalidAuthorization();
        bytes32 structHash =
            keccak256(abi.encode(RECEIVE_WITH_AUTHORIZATION_TYPEHASH, from, to, value, validAfter, validBefore, nonce));
        address signer = ecrecover(keccak256(abi.encodePacked(hex"1901", DOMAIN_SEPARATOR(), structHash)), v, r, s);
        if (signer == address(0) || signer != from) revert InvalidAuthorization();
        if (to == address(0) || to == address(this)) revert InvalidRecipient();
        if (balanceOf[from] < value) revert InsufficientBalance();
        authorizationState[from][nonce] = true;
        emit AuthorizationUsed(from, nonce);
        // TestToken3009 keeps its own move private, so the same two steps are written here.
        balanceOf[from] -= value;
        balanceOf[to] += value;
        emit Transfer(from, to, value);
    }
}
