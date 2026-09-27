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
}
