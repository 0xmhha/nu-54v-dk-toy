// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

/// @title EIP-712 types shared with the device, kiosk and operator tools.
/// @notice encodeType strings come from payment-protocol.schema.json ([N04][N21]).
library PaymentTypes {
    struct PaymentAuthorization {
        uint256 chainId;
        address contractAddress;
        address merchant;
        address payout;
        address token;
        uint256 amount;
        bytes32 orderId;
        uint256 nonce;
        uint64 expiry;
    }

    struct LimitChange {
        uint256 chainId;
        address contractAddress;
        uint256 perPaymentLimit;
        uint256 dailyLimit;
        uint256 nonce;
        uint64 expiry;
    }

    bytes32 internal constant PAYMENT_AUTHORIZATION_TYPEHASH = keccak256(
        "PaymentAuthorization(uint256 chainId,address contract,address merchant,address payout,address token,uint256 amount,bytes32 orderId,uint256 nonce,uint64 expiry)"
    );

    bytes32 internal constant LIMIT_CHANGE_TYPEHASH = keccak256(
        "LimitChange(uint256 chainId,address contract,uint256 perPaymentLimit,uint256 dailyLimit,uint256 nonce,uint64 expiry)"
    );

    bytes32 internal constant DOMAIN_TYPEHASH =
        keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)");
    bytes32 internal constant DOMAIN_NAME_HASH = keccak256("NU54 Payment Settlement");
    bytes32 internal constant DOMAIN_VERSION_HASH = keccak256("1");

    /// @notice EIP-712 domain separator for a given chain and settlement contract.
    function domainSeparator(uint256 chainId, address verifyingContract) internal pure returns (bytes32) {
        return keccak256(abi.encode(DOMAIN_TYPEHASH, DOMAIN_NAME_HASH, DOMAIN_VERSION_HASH, chainId, verifyingContract));
    }

    function hashAuthorization(PaymentAuthorization calldata a) internal pure returns (bytes32) {
        return keccak256(
            abi.encode(
                PAYMENT_AUTHORIZATION_TYPEHASH,
                a.chainId,
                a.contractAddress,
                a.merchant,
                a.payout,
                a.token,
                a.amount,
                a.orderId,
                a.nonce,
                a.expiry
            )
        );
    }

    /// @notice `\x19\x01 || domainSeparator || structHash`, hashed.
    function digest(bytes32 domainSep, bytes32 structHash) internal pure returns (bytes32) {
        return keccak256(abi.encodePacked("\x19\x01", domainSep, structHash));
    }
}
