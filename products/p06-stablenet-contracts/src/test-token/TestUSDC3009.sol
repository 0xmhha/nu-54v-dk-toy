// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {TestUSDC} from "./TestUSDC.sol";

/// @title Test-only dollar token that moves on a signature (EIP-3009).
/// @notice The holder signs one exact transfer and a relay pays the gas, so a payment needs no
/// standing allowance. `receiveWithAuthorization` only works for the named recipient itself.
contract TestUSDC3009 is TestUSDC {
    bytes32 public constant TRANSFER_WITH_AUTHORIZATION_TYPEHASH = keccak256(
        "TransferWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"
    );
    bytes32 public constant RECEIVE_WITH_AUTHORIZATION_TYPEHASH = keccak256(
        "ReceiveWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"
    );
    uint256 private constant HALF_ORDER = 0x7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0;

    mapping(address => mapping(bytes32 => bool)) public authorizationState;

    event AuthorizationUsed(address indexed authorizer, bytes32 indexed nonce);

    error AuthorizationNotYetValid();
    error AuthorizationExpired();
    error AuthorizationAlreadyUsed();
    error CallerNotRecipient();
    error InvalidAuthorization();

    constructor(address owner_) TestUSDC(owner_) {}

    function DOMAIN_SEPARATOR() public view returns (bytes32) {
        return keccak256(
            abi.encode(
                keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"),
                keccak256(bytes(name)),
                keccak256("1"),
                block.chainid,
                address(this)
            )
        );
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
    ) external {
        bytes32 structHash = keccak256(
            abi.encode(TRANSFER_WITH_AUTHORIZATION_TYPEHASH, from, to, value, validAfter, validBefore, nonce)
        );
        _useAuthorization(from, validAfter, validBefore, nonce);
        _checkSigner(from, structHash, v, r, s);
        _send(from, to, value);
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
    ) external {
        if (msg.sender != to) revert CallerNotRecipient();
        bytes32 structHash =
            keccak256(abi.encode(RECEIVE_WITH_AUTHORIZATION_TYPEHASH, from, to, value, validAfter, validBefore, nonce));
        _useAuthorization(from, validAfter, validBefore, nonce);
        _checkSigner(from, structHash, v, r, s);
        _send(from, to, value);
    }

    function _useAuthorization(address from, uint256 validAfter, uint256 validBefore, bytes32 nonce) private {
        if (block.timestamp <= validAfter) revert AuthorizationNotYetValid();
        if (block.timestamp >= validBefore) revert AuthorizationExpired();
        if (authorizationState[from][nonce]) revert AuthorizationAlreadyUsed();
        authorizationState[from][nonce] = true;
        emit AuthorizationUsed(from, nonce);
    }

    function _checkSigner(address from, bytes32 structHash, uint8 v, bytes32 r, bytes32 s) private view {
        if ((v != 27 && v != 28) || uint256(s) > HALF_ORDER) revert InvalidAuthorization();
        bytes32 digest = keccak256(abi.encodePacked(hex"1901", DOMAIN_SEPARATOR(), structHash));
        address signer = ecrecover(digest, v, r, s);
        if (signer == address(0) || signer != from) revert InvalidAuthorization();
    }
}
