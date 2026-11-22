// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

/// @notice The static quote words are the SDK's approved MerchantQuote v2 schema.
library PaymentTypesV2 {
    struct MerchantQuote {
        bytes32 quoteId;
        bytes32 tenantCommitment;
        bytes32 branchCommitment;
        bytes32 orderId;
        bytes32 merchantId;
        bytes32 claimCommitment;
        bytes32 intentCommitment;
        address payer;
        address asset;
        address beneficiary;
        uint256 amount;
        uint256 nonce;
        uint64 merchantKeyVersion;
        uint64 beneficiaryVersion;
        uint64 payerGeneration;
        uint64 issuedAt;
        uint64 expiresAt;
        uint8 mode;
        uint8 payerRole;
    }

    struct RefundAuthorization {
        bytes32 merchantId;
        bytes32 orderId;
        bytes32 refundId;
        uint256 amount;
        uint64 keyVersion;
        uint64 expiresAt;
    }

    bytes32 internal constant DOMAIN_TYPEHASH =
        keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)");
    bytes32 internal constant QUOTE_TYPEHASH = keccak256(
        "MerchantQuote(bytes32 quoteId,bytes32 tenantCommitment,bytes32 branchCommitment,bytes32 orderId,bytes32 merchantId,bytes32 claimCommitment,bytes32 intentCommitment,address payer,address asset,address beneficiary,uint256 amount,uint256 nonce,uint64 merchantKeyVersion,uint64 beneficiaryVersion,uint64 payerGeneration,uint64 issuedAt,uint64 expiresAt,uint8 mode,uint8 payerRole)"
    );
    bytes32 internal constant APPROVAL_TYPEHASH = keccak256("PaymentApproval(bytes32 quoteDigest)");
    bytes32 internal constant REFUND_TYPEHASH = keccak256(
        "RefundAuthorization(bytes32 merchantId,bytes32 orderId,bytes32 refundId,uint256 amount,uint64 keyVersion,uint64 expiresAt)"
    );

    function domain(uint256 chainId, address settlement) internal pure returns (bytes32) {
        return keccak256(
            abi.encode(DOMAIN_TYPEHASH, keccak256("NU54 Payment Settlement"), keccak256("2"), chainId, settlement)
        );
    }

    function digest(bytes32 separator, bytes32 structHash) internal pure returns (bytes32) {
        return keccak256(abi.encodePacked(hex"1901", separator, structHash));
    }

    function quoteDigest(bytes32 separator, MerchantQuote calldata quote) internal pure returns (bytes32) {
        return digest(separator, keccak256(abi.encode(QUOTE_TYPEHASH, quote)));
    }

    function approvalDigest(bytes32 separator, bytes32 authorizedQuoteDigest) internal pure returns (bytes32) {
        return digest(separator, keccak256(abi.encode(APPROVAL_TYPEHASH, authorizedQuoteDigest)));
    }

    function refundDigest(bytes32 separator, RefundAuthorization calldata authorization)
        internal
        pure
        returns (bytes32)
    {
        return digest(separator, keccak256(abi.encode(REFUND_TYPEHASH, authorization)));
    }
}
