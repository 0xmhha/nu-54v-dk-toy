// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {IERC20Minimal} from "./interfaces/IERC20Minimal.sol";
import {PaymentTypesV2 as Types} from "./PaymentTypesV2.sol";

interface IERC20AllowanceV2 {
    function allowance(address owner, address spender) external view returns (uint256);
}

/// @notice Test-only settlement with separate escrow and refund liabilities.
contract SettlementV2 {
    uint256 public constant CHAIN_ID = 8283;
    uint256 public constant MAX_PENDING_ROTATION = 256;
    uint8 public constant ESCROW = 1;
    uint8 public constant DIRECT = 2;
    uint8 public constant OWNER = 1;
    uint8 public constant RENTER = 2;
    uint256 private constant HALF_ORDER = 0x7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0;

    struct Merchant {
        uint64 keyVersion;
        uint64 beneficiaryVersion;
        uint64 refundKeyVersion;
        address refundSigner;
        bool active;
    }

    struct VersionedAddress {
        address account;
        uint64 validFrom;
        uint64 validUntil;
        bool revoked;
    }

    struct Payer {
        uint64 generation;
        uint8 role;
        bool active;
    }

    struct Payment {
        address payer;
        uint256 amount;
        uint256 refunded;
        uint64 payerGeneration;
        uint8 mode;
    }

    address public immutable asset;
    address public immutable governor;
    bool public immutable directEnabled;
    bool private entered;
    uint256 public totalEscrow;
    uint256 public totalRefundReserve;

    mapping(bytes32 => Merchant) public merchants;
    mapping(bytes32 => mapping(uint64 => VersionedAddress)) public merchantKeys;
    mapping(bytes32 => mapping(uint64 => VersionedAddress)) public beneficiaries;
    mapping(bytes32 => mapping(bytes32 => bool)) public grandfathered;
    mapping(address => Payer) public payers;
    mapping(address => mapping(uint64 => address)) public escrowWithdrawalAddress;
    mapping(address => mapping(uint64 => uint256)) public escrowBalance;
    mapping(bytes32 => uint256) public refundReserve;
    mapping(bytes32 => mapping(bytes32 => Payment)) public payments;
    mapping(bytes32 => mapping(uint256 => bool)) public merchantNonceUsed;
    mapping(address => mapping(uint256 => bool)) public payerNonceUsed;
    mapping(bytes32 => mapping(bytes32 => mapping(bytes32 => bool))) public refundIdUsed;

    error WrongDomain();
    error Unauthorized();
    error InvalidParameters();
    error InvalidMerchant();
    error InvalidPayer();
    error InvalidQuote();
    error InvalidSignature();
    error InvalidTime();
    error OrderAlreadyPaid();
    error NonceUsed();
    error InsufficientFunds();
    error RefundAlreadyUsed();
    error RefundAbovePayment();
    error TokenTransferFailed();
    error TokenBalanceMismatch();
    error Reentered();

    event MerchantConfigured(bytes32 indexed merchantId, uint64 keyVersion, uint64 beneficiaryVersion);
    event MerchantKeyRevoked(bytes32 indexed merchantId, uint64 indexed keyVersion);
    event Paid(
        bytes32 indexed merchantId,
        bytes32 indexed orderId,
        address indexed payer,
        address beneficiary,
        address asset,
        uint256 amount,
        uint256 nonce,
        bytes32 quoteDigest,
        uint8 mode,
        uint64 payerGeneration
    );
    event Refunded(
        bytes32 indexed merchantId,
        bytes32 indexed orderId,
        bytes32 indexed refundId,
        address payer,
        address asset,
        uint256 amount,
        uint8 mode
    );

    constructor(address asset_, address governor_, bool directEnabled_) {
        if (block.chainid != CHAIN_ID) revert WrongDomain();
        if (asset_.code.length == 0 || governor_ == address(0) || governor_ == asset_) revert InvalidParameters();
        asset = asset_;
        governor = governor_;
        directEnabled = directEnabled_;
    }

    modifier onlyGovernor() {
        if (msg.sender != governor) revert Unauthorized();
        _;
    }

    modifier guarded() {
        if (entered) revert Reentered();
        entered = true;
        _;
        entered = false;
    }

    function domainSeparator() public view returns (bytes32) {
        return Types.domain(block.chainid, address(this));
    }

    function quoteDigest(Types.MerchantQuote calldata quote) public view returns (bytes32) {
        return Types.quoteDigest(domainSeparator(), quote);
    }

    function approvalDigest(bytes32 authorizedQuoteDigest) public view returns (bytes32) {
        return Types.approvalDigest(domainSeparator(), authorizedQuoteDigest);
    }

    function refundDigest(Types.RefundAuthorization calldata authorization) public view returns (bytes32) {
        return Types.refundDigest(domainSeparator(), authorization);
    }

    function registerMerchant(bytes32 merchantId, address signer, address beneficiary, address refundSigner)
        external
        onlyGovernor
    {
        if (
            merchantId == 0 || merchants[merchantId].keyVersion != 0 || !_validRecipient(beneficiary)
                || signer == address(0) || refundSigner == address(0) || signer == refundSigner
        ) {
            revert InvalidParameters();
        }
        merchants[merchantId] = Merchant(1, 1, 1, refundSigner, true);
        uint64 now_ = _timestamp();
        merchantKeys[merchantId][1] = VersionedAddress(signer, now_, 0, false);
        beneficiaries[merchantId][1] = VersionedAddress(beneficiary, now_, 0, false);
        emit MerchantConfigured(merchantId, 1, 1);
    }

    /// @notice P05 freezes issuance, snapshots committed open digests and supplies that exact set at cutover.
    /// @dev Governor authentication protects the snapshot. No extra transaction is needed for ordinary issuance.
    function rotateMerchant(
        bytes32 merchantId,
        address nextSigner,
        address nextBeneficiary,
        bytes32[] calldata openDigests
    ) external onlyGovernor {
        Merchant storage merchant = merchants[merchantId];
        if (
            !merchant.active || nextSigner == address(0) || nextSigner == merchant.refundSigner
                || !_validRecipient(nextBeneficiary) || openDigests.length > MAX_PENDING_ROTATION
        ) revert InvalidParameters();

        VersionedAddress storage previousKey = merchantKeys[merchantId][merchant.keyVersion];
        VersionedAddress storage previousBeneficiary = beneficiaries[merchantId][merchant.beneficiaryVersion];
        bool changeKey = previousKey.account != nextSigner;
        bool changeBeneficiary = previousBeneficiary.account != nextBeneficiary;
        if (!changeKey && !changeBeneficiary) revert InvalidParameters();

        uint64 now_ = _timestamp();
        for (uint256 i; i < openDigests.length; ++i) {
            if (openDigests[i] == 0) revert InvalidParameters();
            grandfathered[merchantId][openDigests[i]] = true;
        }
        if (changeKey) {
            previousKey.validUntil = now_;
            ++merchant.keyVersion;
            merchantKeys[merchantId][merchant.keyVersion] = VersionedAddress(nextSigner, now_, 0, false);
        }
        if (changeBeneficiary) {
            previousBeneficiary.validUntil = now_;
            ++merchant.beneficiaryVersion;
            beneficiaries[merchantId][merchant.beneficiaryVersion] = VersionedAddress(nextBeneficiary, now_, 0, false);
        }
        emit MerchantConfigured(merchantId, merchant.keyVersion, merchant.beneficiaryVersion);
    }

    function revokeMerchantKey(bytes32 merchantId, uint64 version) external onlyGovernor {
        VersionedAddress storage key = merchantKeys[merchantId][version];
        if (key.account == address(0) || key.revoked) revert InvalidMerchant();
        key.revoked = true;
        emit MerchantKeyRevoked(merchantId, version);
    }

    function setMerchantActive(bytes32 merchantId, bool active) external onlyGovernor {
        if (merchants[merchantId].keyVersion == 0) revert InvalidMerchant();
        merchants[merchantId].active = active;
    }

    function rotateRefundSigner(bytes32 merchantId, address signer) external onlyGovernor {
        Merchant storage merchant = merchants[merchantId];
        if (
            merchant.keyVersion == 0 || signer == address(0) || signer == merchant.refundSigner
                || signer == merchantKeys[merchantId][merchant.keyVersion].account
        ) revert InvalidMerchant();
        merchant.refundSigner = signer;
        ++merchant.refundKeyVersion;
    }

    /// @notice A new authority generation starts off with its own balance and withdrawal destination.
    function registerPayer(address payer, uint64 generation, uint8 role, address withdrawalAddress)
        external
        onlyGovernor
    {
        if (
            !_validRecipient(payer) || generation <= payers[payer].generation || (role != OWNER && role != RENTER)
                || !_validRecipient(withdrawalAddress)
        ) revert InvalidParameters();
        payers[payer] = Payer(generation, role, true);
        escrowWithdrawalAddress[payer][generation] = withdrawalAddress;
    }

    function deactivatePayer(address payer) external onlyGovernor {
        if (!payers[payer].active) revert InvalidPayer();
        payers[payer].active = false;
    }

    function depositEscrow(address payer, uint64 generation, uint256 amount) external onlyGovernor guarded {
        Payer memory current = payers[payer];
        if (!current.active || current.generation != generation) revert InvalidPayer();
        _transferExact(msg.sender, address(this), amount);
        escrowBalance[payer][generation] += amount;
        totalEscrow += amount;
    }

    function withdrawEscrow(address payer, uint64 generation, uint256 amount) external guarded {
        if (msg.sender != escrowWithdrawalAddress[payer][generation]) revert Unauthorized();
        if (escrowBalance[payer][generation] < amount) revert InsufficientFunds();
        escrowBalance[payer][generation] -= amount;
        totalEscrow -= amount;
        _transferExact(address(this), msg.sender, amount);
    }

    function fundRefundReserve(bytes32 merchantId, uint256 amount) external guarded {
        if (merchants[merchantId].keyVersion == 0) revert InvalidMerchant();
        _transferExact(msg.sender, address(this), amount);
        refundReserve[merchantId] += amount;
        totalRefundReserve += amount;
    }

    function settle(Types.MerchantQuote calldata quote, bytes calldata merchantSignature, bytes calldata payerSignature)
        external
        guarded
    {
        _checkChain();
        _checkQuote(quote);
        bytes32 hash = quoteDigest(quote);
        _checkMerchant(quote, hash, merchantSignature);
        if (_recover(approvalDigest(hash), payerSignature) != quote.payer) revert InvalidSignature();
        if (payments[quote.merchantId][quote.orderId].amount != 0) revert OrderAlreadyPaid();
        if (merchantNonceUsed[quote.merchantId][quote.nonce] || payerNonceUsed[quote.payer][quote.nonce]) {
            revert NonceUsed();
        }
        merchantNonceUsed[quote.merchantId][quote.nonce] = true;
        payerNonceUsed[quote.payer][quote.nonce] = true;
        payments[quote.merchantId][quote.orderId] =
            Payment(quote.payer, quote.amount, 0, quote.payerGeneration, quote.mode);

        if (quote.mode == ESCROW) {
            if (escrowBalance[quote.payer][quote.payerGeneration] < quote.amount) revert InsufficientFunds();
            escrowBalance[quote.payer][quote.payerGeneration] -= quote.amount;
            totalEscrow -= quote.amount;
            _transferExact(address(this), quote.beneficiary, quote.amount);
        } else {
            _transferExact(quote.payer, quote.beneficiary, quote.amount);
        }
        emit Paid(
            quote.merchantId,
            quote.orderId,
            quote.payer,
            quote.beneficiary,
            asset,
            quote.amount,
            quote.nonce,
            hash,
            quote.mode,
            quote.payerGeneration
        );
    }

    function refund(Types.RefundAuthorization calldata authorization, bytes calldata signature) external guarded {
        _checkChain();
        if (authorization.amount == 0 || authorization.refundId == 0) revert InvalidParameters();
        if (block.timestamp >= authorization.expiresAt) revert InvalidTime();
        Merchant memory merchant = merchants[authorization.merchantId];
        if (
            merchant.refundKeyVersion == 0 || merchant.refundKeyVersion != authorization.keyVersion
                || _recover(refundDigest(authorization), signature) != merchant.refundSigner
        ) revert InvalidSignature();

        Payment storage payment = payments[authorization.merchantId][authorization.orderId];
        if (authorization.amount > payment.amount - payment.refunded) revert RefundAbovePayment();
        if (refundIdUsed[authorization.merchantId][authorization.orderId][authorization.refundId]) {
            revert RefundAlreadyUsed();
        }
        if (refundReserve[authorization.merchantId] < authorization.amount) revert InsufficientFunds();
        refundIdUsed[authorization.merchantId][authorization.orderId][authorization.refundId] = true;
        payment.refunded += authorization.amount;
        refundReserve[authorization.merchantId] -= authorization.amount;
        totalRefundReserve -= authorization.amount;
        if (payment.mode == ESCROW) {
            escrowBalance[payment.payer][payment.payerGeneration] += authorization.amount;
            totalEscrow += authorization.amount;
        } else {
            _transferExact(address(this), payment.payer, authorization.amount);
        }
        emit Refunded(
            authorization.merchantId,
            authorization.orderId,
            authorization.refundId,
            payment.payer,
            asset,
            authorization.amount,
            payment.mode
        );
    }

    function _checkChain() private view {
        if (block.chainid != CHAIN_ID) revert WrongDomain();
    }

    function _checkQuote(Types.MerchantQuote calldata quote) private view {
        if (quote.asset != asset) revert WrongDomain();
        if (
            quote.quoteId == 0 || quote.tenantCommitment == 0 || quote.branchCommitment == 0 || quote.orderId == 0
                || quote.merchantId == 0 || quote.claimCommitment == 0 || quote.intentCommitment == 0
                || quote.amount == 0 || quote.nonce == 0 || quote.issuedAt == 0
        ) revert InvalidQuote();
        if (block.timestamp < quote.issuedAt || block.timestamp >= quote.expiresAt) revert InvalidTime();
        Payer memory payer = payers[quote.payer];
        if (!payer.active || payer.generation != quote.payerGeneration || payer.role != quote.payerRole) {
            revert InvalidPayer();
        }
        if (quote.mode == DIRECT) {
            if (!directEnabled || payer.role != OWNER || msg.sender != quote.payer) revert InvalidPayer();
            if (IERC20AllowanceV2(asset).allowance(quote.payer, address(this)) != quote.amount) {
                revert InvalidParameters();
            }
        } else if (quote.mode != ESCROW) {
            revert InvalidQuote();
        }
    }

    function _checkMerchant(Types.MerchantQuote calldata quote, bytes32 hash, bytes calldata signature) private view {
        Merchant memory merchant = merchants[quote.merchantId];
        VersionedAddress storage key = merchantKeys[quote.merchantId][quote.merchantKeyVersion];
        VersionedAddress storage beneficiary = beneficiaries[quote.merchantId][quote.beneficiaryVersion];
        bool historical =
            merchant.keyVersion != quote.merchantKeyVersion || merchant.beneficiaryVersion != quote.beneficiaryVersion;
        // Same-second older quotes cannot be ordered by their timestamps. Only the exact
        // attested pre-cutover digests enter the historical path, including that boundary.
        if (historical && !grandfathered[quote.merchantId][hash]) revert InvalidMerchant();
        if (
            !merchant.active || key.revoked || !_validAtIssue(key, quote.issuedAt)
                || !_validAtIssue(beneficiary, quote.issuedAt) || beneficiary.account != quote.beneficiary
        ) revert InvalidMerchant();
        if (key.account == quote.payer || _recover(hash, signature) != key.account) revert InvalidSignature();
    }

    function _validAtIssue(VersionedAddress storage record, uint64 issuedAt) private view returns (bool) {
        return record.account != address(0) && record.validFrom <= issuedAt
            && (record.validUntil == 0 || issuedAt <= record.validUntil);
    }

    function _recover(bytes32 hash, bytes calldata sig) private pure returns (address signer) {
        if (sig.length != 65) revert InvalidSignature();
        (bytes32 r, bytes32 s, uint8 v) = (bytes32(sig[0:32]), bytes32(sig[32:64]), uint8(sig[64]));
        if ((v != 27 && v != 28) || uint256(s) > HALF_ORDER) revert InvalidSignature();
        signer = ecrecover(hash, v, r, s);
        if (signer == address(0)) revert InvalidSignature();
    }

    function _transferExact(address from, address to, uint256 amount) private {
        if (from == to || amount == 0) revert InvalidParameters();
        IERC20Minimal token = IERC20Minimal(asset);
        uint256 fromBefore = token.balanceOf(from);
        uint256 toBefore = token.balanceOf(to);
        bool success = from == address(this) ? token.transfer(to, amount) : token.transferFrom(from, to, amount);
        if (!success) revert TokenTransferFailed();
        uint256 fromAfter = token.balanceOf(from);
        uint256 toAfter = token.balanceOf(to);
        if (
            fromAfter > fromBefore || toAfter < toBefore || fromBefore - fromAfter != amount
                || toAfter - toBefore != amount
        ) revert TokenBalanceMismatch();
    }

    function _validRecipient(address account) private view returns (bool) {
        return account != address(0) && account != asset && account != address(this);
    }

    function _timestamp() private view returns (uint64) {
        if (block.timestamp == 0 || block.timestamp > type(uint64).max) revert InvalidTime();
        return uint64(block.timestamp);
    }
}
