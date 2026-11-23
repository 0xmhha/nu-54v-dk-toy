// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {PaymentTypesV2 as Types} from "./PaymentTypesV2.sol";

interface IAuthorizedToken {
    function balanceOf(address account) external view returns (uint256);
    function transfer(address to, uint256 amount) external returns (bool);
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
    ) external;
}

/// @title Purchase protection for one order at a time.
/// @notice An order's amount enters this contract only when the order is paid. It leaves to the
/// merchant after the buyer confirms receipt, or when the merchant claims it after the
/// confirmation window. It goes back to the paying account when the order is cancelled or never
/// delivered. A buyer's deposits into their own wallet account never touch this contract.
///
/// One wallet signature pays: the token authorization's nonce commits to the merchant quote and
/// to the phone key that may later confirm or dispute, so neither can be swapped by a relay.
contract OrderEscrow {
    enum State {
        None,
        Locked,
        Delivered,
        Disputed,
        Paid,
        Refunded
    }

    enum SkipReason {
        None,
        Unknown,
        NotDelivered,
        WindowOpen,
        Disputed,
        Settled,
        OtherBeneficiary
    }

    enum Payout {
        Receipt,
        Claim,
        Arbiter,
        DisputeWithdrawn
    }

    enum Refund {
        Store,
        Undelivered,
        Arbiter
    }

    struct Merchant {
        address signer;
        address beneficiary;
        uint64 keyVersion;
        uint64 beneficiaryVersion;
        uint32 confirmWindow;
        uint32 deliveryWindow;
        bool active;
    }

    /// @dev The windows and the beneficiary are fixed when the order is paid, so a later change
    /// of the merchant settings never moves the terms of an order that already exists.
    struct Order {
        address payer;
        address beneficiary;
        address confirmer;
        uint256 amount;
        uint64 deliverBy;
        uint64 claimableAt; // 0 until delivered
        uint32 confirmWindow;
        State state;
    }

    uint256 public constant CHAIN_ID = 8283;
    uint32 public constant MIN_WINDOW = 1 hours;
    uint32 public constant MAX_WINDOW = 60 days;
    uint256 public constant MAX_BATCH = 100;
    uint8 private constant ESCROW = 1;
    uint256 private constant HALF_ORDER = 0x7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0;

    bytes32 public constant LOCK_TYPEHASH = keccak256("OrderLock(bytes32 quoteDigest,address confirmer)");
    bytes32 public constant DELIVERY_TYPEHASH = keccak256("OrderDelivered(bytes32 merchantId,bytes32 orderId)");
    bytes32 public constant CANCEL_TYPEHASH = keccak256("OrderCancel(bytes32 merchantId,bytes32 orderId)");
    // The buyer's actions name the delivery they answer, so nothing signed before the delivered mark,
    // or for another one, can be used. Giving up a dispute is its own action.
    bytes32 public constant RECEIPT_TYPEHASH =
        keccak256("OrderReceipt(bytes32 merchantId,bytes32 orderId,uint64 claimableAt)");
    bytes32 public constant DISPUTE_TYPEHASH =
        keccak256("OrderDispute(bytes32 merchantId,bytes32 orderId,uint64 claimableAt)");
    bytes32 public constant WITHDRAW_DISPUTE_TYPEHASH =
        keccak256("OrderDisputeWithdraw(bytes32 merchantId,bytes32 orderId,uint64 claimableAt)");

    address public immutable asset;
    address public immutable governor;
    address public arbiter;
    uint256 public totalHeld;
    bool private entered;

    mapping(bytes32 => Merchant) public merchants;
    mapping(bytes32 => mapping(bytes32 => Order)) public orders;

    event MerchantConfigured(
        bytes32 indexed merchantId,
        address signer,
        address beneficiary,
        uint64 keyVersion,
        uint64 beneficiaryVersion,
        uint32 confirmWindow,
        uint32 deliveryWindow
    );
    event MerchantActive(bytes32 indexed merchantId, bool active);
    event ArbiterChanged(address indexed arbiter);
    event OrderLocked(
        bytes32 indexed merchantId,
        bytes32 indexed orderId,
        address indexed payer,
        address beneficiary,
        address confirmer,
        uint256 amount,
        uint64 deliverBy,
        uint32 confirmWindow,
        bytes32 quoteDigest
    );
    event OrderDelivered(bytes32 indexed merchantId, bytes32 indexed orderId, uint64 claimableAt);
    event OrderDisputed(bytes32 indexed merchantId, bytes32 indexed orderId);
    event OrderReleased(
        bytes32 indexed merchantId, bytes32 indexed orderId, address beneficiary, uint256 amount, Payout reason
    );
    event OrderCancelled(
        bytes32 indexed merchantId, bytes32 indexed orderId, address payer, uint256 amount, Refund reason
    );
    event ClaimSkipped(bytes32 indexed merchantId, bytes32 indexed orderId, SkipReason reason);

    error WrongDomain();
    error Unauthorized();
    error InvalidParameters();
    error InvalidMerchant();
    error InvalidQuote();
    error InvalidSignature();
    error InvalidTime();
    error InvalidState();
    error OrderAlreadyPaid();
    error TokenTransferFailed();
    error TokenBalanceMismatch();
    error Reentered();

    constructor(address asset_, address governor_, address arbiter_) {
        if (block.chainid != CHAIN_ID) revert WrongDomain();
        if (asset_.code.length == 0 || governor_ == address(0) || arbiter_ == address(0)) revert InvalidParameters();
        asset = asset_;
        governor = governor_;
        arbiter = arbiter_;
        emit ArbiterChanged(arbiter_);
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

    // ---- settings ----

    function setArbiter(address next) external onlyGovernor {
        if (next == address(0)) revert InvalidParameters();
        arbiter = next;
        emit ArbiterChanged(next);
    }

    /// @notice Registers a merchant or rotates its keys and windows. Existing orders keep their terms.
    function configureMerchant(
        bytes32 merchantId,
        address signer,
        address beneficiary,
        uint32 confirmWindow,
        uint32 deliveryWindow
    ) external onlyGovernor {
        if (
            merchantId == 0 || signer == address(0) || beneficiary == address(0) || beneficiary == address(this)
                || beneficiary == asset || confirmWindow < MIN_WINDOW || confirmWindow > MAX_WINDOW
                || deliveryWindow < MIN_WINDOW || deliveryWindow > MAX_WINDOW
        ) revert InvalidParameters();
        Merchant storage merchant = merchants[merchantId];
        if (merchant.signer == address(0)) merchant.active = true;
        if (merchant.signer != signer) merchant.keyVersion += 1;
        if (merchant.beneficiary != beneficiary) merchant.beneficiaryVersion += 1;
        merchant.signer = signer;
        merchant.beneficiary = beneficiary;
        merchant.confirmWindow = confirmWindow;
        merchant.deliveryWindow = deliveryWindow;
        emit MerchantConfigured(
            merchantId,
            signer,
            beneficiary,
            merchant.keyVersion,
            merchant.beneficiaryVersion,
            confirmWindow,
            deliveryWindow
        );
    }

    /// @notice Stops new orders only. Orders already paid can still be delivered, claimed or refunded.
    function setMerchantActive(bytes32 merchantId, bool active) external onlyGovernor {
        if (merchants[merchantId].signer == address(0)) revert InvalidMerchant();
        merchants[merchantId].active = active;
        emit MerchantActive(merchantId, active);
    }

    // ---- digests ----

    function domainSeparator() public view returns (bytes32) {
        return Types.domain(block.chainid, address(this));
    }

    function quoteDigest(Types.MerchantQuote calldata quote) public view returns (bytes32) {
        return Types.quoteDigest(domainSeparator(), quote);
    }

    /// @notice The token authorization nonce the wallet signs for this quote and phone key.
    function lockNonce(Types.MerchantQuote calldata quote, address confirmer) public view returns (bytes32) {
        return keccak256(abi.encode(LOCK_TYPEHASH, quoteDigest(quote), confirmer));
    }

    function actionDigest(bytes32 typehash, bytes32 merchantId, bytes32 orderId) public view returns (bytes32) {
        return Types.digest(domainSeparator(), keccak256(abi.encode(typehash, merchantId, orderId)));
    }

    function buyerDigest(bytes32 typehash, bytes32 merchantId, bytes32 orderId) public view returns (bytes32) {
        uint64 claimableAt = orders[merchantId][orderId].claimableAt;
        return Types.digest(domainSeparator(), keccak256(abi.encode(typehash, merchantId, orderId, claimableAt)));
    }

    // ---- order life ----

    /// @notice Pays an order into escrow. Anyone may submit it and pay the gas.
    function lock(
        Types.MerchantQuote calldata quote,
        address confirmer,
        bytes calldata merchantSignature,
        bytes calldata payerSignature
    ) external guarded {
        bytes32 hash = _checkQuote(quote, confirmer, merchantSignature);
        Merchant storage merchant = merchants[quote.merchantId];
        uint64 deliverBy = _now() + merchant.deliveryWindow;
        orders[quote.merchantId][quote.orderId] = Order({
            payer: quote.payer,
            beneficiary: quote.beneficiary,
            confirmer: confirmer,
            amount: quote.amount,
            deliverBy: deliverBy,
            claimableAt: 0,
            confirmWindow: merchant.confirmWindow,
            state: State.Locked
        });
        totalHeld += quote.amount;
        _pull(quote, keccak256(abi.encode(LOCK_TYPEHASH, hash, confirmer)), payerSignature);
        emit OrderLocked(
            quote.merchantId,
            quote.orderId,
            quote.payer,
            quote.beneficiary,
            confirmer,
            quote.amount,
            deliverBy,
            merchant.confirmWindow,
            hash
        );
    }

    /// @notice The store hands over the goods. The confirmation window starts here, not at payment.
    function markDelivered(bytes32 merchantId, bytes32 orderId, bytes calldata signature) external guarded {
        Order storage order = orders[merchantId][orderId];
        if (order.state != State.Locked) revert InvalidState();
        if (_now() >= order.deliverBy) revert InvalidTime();
        _checkMerchantAction(DELIVERY_TYPEHASH, merchantId, orderId, signature);
        order.state = State.Delivered;
        order.claimableAt = _now() + order.confirmWindow;
        emit OrderDelivered(merchantId, orderId, order.claimableAt);
    }

    /// @notice The buyer confirms receipt. Final: the merchant is paid and nothing can be cancelled.
    function release(bytes32 merchantId, bytes32 orderId, bytes calldata signature) external guarded {
        Order storage order = orders[merchantId][orderId];
        if (order.state != State.Delivered) revert InvalidState();
        _checkBuyerAction(order, RECEIPT_TYPEHASH, merchantId, orderId, signature);
        _pay(order, merchantId, orderId, Payout.Receipt);
    }

    /// @notice The buyer gives up a dispute and pays. A receipt signed earlier does not do this.
    function withdrawDispute(bytes32 merchantId, bytes32 orderId, bytes calldata signature) external guarded {
        Order storage order = orders[merchantId][orderId];
        if (order.state != State.Disputed) revert InvalidState();
        _checkBuyerAction(order, WITHDRAW_DISPUTE_TYPEHASH, merchantId, orderId, signature);
        _pay(order, merchantId, orderId, Payout.DisputeWithdrawn);
    }

    /// @notice The buyer says the goods did not arrive. Only before the merchant may claim.
    function dispute(bytes32 merchantId, bytes32 orderId, bytes calldata signature) external guarded {
        Order storage order = orders[merchantId][orderId];
        if (order.state != State.Delivered) revert InvalidState();
        if (_now() >= order.claimableAt) revert InvalidTime();
        _checkBuyerAction(order, DISPUTE_TYPEHASH, merchantId, orderId, signature);
        order.state = State.Disputed;
        emit OrderDisputed(merchantId, orderId);
    }

    /// @notice The operator settles a disputed order.
    function resolve(bytes32 merchantId, bytes32 orderId, bool payMerchant) external guarded {
        if (msg.sender != arbiter) revert Unauthorized();
        Order storage order = orders[merchantId][orderId];
        if (order.state != State.Disputed) revert InvalidState();
        if (payMerchant) _pay(order, merchantId, orderId, Payout.Arbiter);
        else _refund(order, merchantId, orderId, Refund.Arbiter);
    }

    /// @notice The store gives the money back before the buyer confirmed.
    function cancel(bytes32 merchantId, bytes32 orderId, bytes calldata signature) external guarded {
        Order storage order = orders[merchantId][orderId];
        if (order.state != State.Locked && order.state != State.Delivered && order.state != State.Disputed) {
            revert InvalidState();
        }
        _checkMerchantAction(CANCEL_TYPEHASH, merchantId, orderId, signature);
        _refund(order, merchantId, orderId, Refund.Store);
    }

    /// @notice Returns the money of an order the store never marked as delivered.
    function refundUndelivered(bytes32 merchantId, bytes32 orderId) external guarded {
        Order storage order = orders[merchantId][orderId];
        if (order.state != State.Locked) revert InvalidState();
        if (_now() < order.deliverBy) revert InvalidTime();
        _refund(order, merchantId, orderId, Refund.Undelivered);
    }

    /// @notice The store collects one order the buyer did not confirm in time. A contract never
    /// runs by itself, so somebody must send this; the money can only go to the fixed beneficiary.
    function claim(bytes32 merchantId, bytes32 orderId) external guarded {
        Order storage order = orders[merchantId][orderId];
        if (claimStatus(merchantId, orderId) != SkipReason.None) revert InvalidState();
        _pay(order, merchantId, orderId, Payout.Claim);
    }

    /// @notice Collects many orders in one transaction with one token transfer. An order that cannot
    /// be claimed is skipped with its reason and does not stop the others. A batch with nothing to claim
    /// succeeds with zero, so its reasons stay on record.
    function claimBatch(bytes32 merchantId, address beneficiary, bytes32[] calldata orderIds)
        external
        guarded
        returns (uint256 total)
    {
        if (orderIds.length == 0 || orderIds.length > MAX_BATCH) revert InvalidParameters();
        for (uint256 i = 0; i < orderIds.length; ++i) {
            bytes32 orderId = orderIds[i];
            SkipReason reason = claimStatus(merchantId, orderId);
            Order storage order = orders[merchantId][orderId];
            if (reason == SkipReason.None && order.beneficiary != beneficiary) reason = SkipReason.OtherBeneficiary;
            if (reason != SkipReason.None) {
                emit ClaimSkipped(merchantId, orderId, reason);
                continue;
            }
            order.state = State.Paid; // a repeated id in the same batch is skipped as settled
            total += order.amount;
            emit OrderReleased(merchantId, orderId, beneficiary, order.amount, Payout.Claim);
        }
        if (total == 0) return 0;
        totalHeld -= total;
        _push(beneficiary, total);
    }

    /// @notice Why an order cannot be claimed now; None means it can.
    function claimStatus(bytes32 merchantId, bytes32 orderId) public view returns (SkipReason) {
        Order storage order = orders[merchantId][orderId];
        if (order.state == State.None) return SkipReason.Unknown;
        if (order.state == State.Locked) return SkipReason.NotDelivered;
        if (order.state == State.Disputed) return SkipReason.Disputed;
        if (order.state != State.Delivered) return SkipReason.Settled;
        return _now() < order.claimableAt ? SkipReason.WindowOpen : SkipReason.None;
    }

    // ---- internals ----

    function _checkQuote(Types.MerchantQuote calldata quote, address confirmer, bytes calldata merchantSignature)
        private
        view
        returns (bytes32 hash)
    {
        if (block.chainid != CHAIN_ID || quote.asset != asset) revert WrongDomain();
        if (
            quote.quoteId == 0 || quote.tenantCommitment == 0 || quote.branchCommitment == 0 || quote.orderId == 0
                || quote.merchantId == 0 || quote.claimCommitment == 0 || quote.intentCommitment == 0
                || quote.amount == 0 || quote.nonce == 0 || quote.issuedAt == 0 || quote.mode != ESCROW
        ) revert InvalidQuote();
        if (block.timestamp < quote.issuedAt || block.timestamp >= quote.expiresAt) revert InvalidTime();
        if (orders[quote.merchantId][quote.orderId].state != State.None) revert OrderAlreadyPaid();
        Merchant memory merchant = merchants[quote.merchantId];
        if (
            !merchant.active || merchant.keyVersion != quote.merchantKeyVersion
                || merchant.beneficiaryVersion != quote.beneficiaryVersion || merchant.beneficiary != quote.beneficiary
        ) revert InvalidMerchant();
        // The store must not be able to confirm receipt for the buyer.
        if (
            confirmer == address(0) || confirmer == merchant.signer || confirmer == quote.beneficiary
                || quote.payer == merchant.signer || quote.payer == quote.beneficiary || quote.payer == address(this)
        ) revert InvalidParameters();
        hash = quoteDigest(quote);
        if (_recover(hash, merchantSignature) != merchant.signer) revert InvalidSignature();
    }

    function _checkMerchantAction(bytes32 typehash, bytes32 merchantId, bytes32 orderId, bytes calldata signature)
        private
        view
    {
        address signer = merchants[merchantId].signer;
        if (_recover(actionDigest(typehash, merchantId, orderId), signature) != signer) revert InvalidSignature();
    }

    function _checkBuyerAction(
        Order storage order,
        bytes32 typehash,
        bytes32 merchantId,
        bytes32 orderId,
        bytes calldata signature
    ) private view {
        if (_recover(buyerDigest(typehash, merchantId, orderId), signature) != order.confirmer) {
            revert InvalidSignature();
        }
    }

    function _pay(Order storage order, bytes32 merchantId, bytes32 orderId, Payout reason) private {
        order.state = State.Paid;
        totalHeld -= order.amount;
        emit OrderReleased(merchantId, orderId, order.beneficiary, order.amount, reason);
        _push(order.beneficiary, order.amount);
    }

    function _refund(Order storage order, bytes32 merchantId, bytes32 orderId, Refund reason) private {
        order.state = State.Refunded;
        totalHeld -= order.amount;
        emit OrderCancelled(merchantId, orderId, order.payer, order.amount, reason);
        _push(order.payer, order.amount);
    }

    function _pull(Types.MerchantQuote calldata quote, bytes32 nonce, bytes calldata signature) private {
        if (signature.length != 65) revert InvalidSignature();
        uint256 beforeBalance = IAuthorizedToken(asset).balanceOf(address(this));
        IAuthorizedToken(asset)
            .receiveWithAuthorization(
                quote.payer,
                address(this),
                quote.amount,
                0,
                quote.expiresAt,
                nonce,
                uint8(signature[64]),
                bytes32(signature[0:32]),
                bytes32(signature[32:64])
            );
        if (IAuthorizedToken(asset).balanceOf(address(this)) != beforeBalance + quote.amount) {
            revert TokenBalanceMismatch();
        }
    }

    function _push(address to, uint256 amount) private {
        uint256 beforeBalance = IAuthorizedToken(asset).balanceOf(to);
        if (!IAuthorizedToken(asset).transfer(to, amount)) revert TokenTransferFailed();
        // A held or short transfer reverts the whole call, so an order is never marked paid without money.
        if (IAuthorizedToken(asset).balanceOf(to) != beforeBalance + amount) revert TokenBalanceMismatch();
    }

    function _recover(bytes32 hash, bytes calldata sig) private pure returns (address signer) {
        if (sig.length != 65) revert InvalidSignature();
        (bytes32 r, bytes32 s, uint8 v) = (bytes32(sig[0:32]), bytes32(sig[32:64]), uint8(sig[64]));
        if ((v != 27 && v != 28) || uint256(s) > HALF_ORDER) revert InvalidSignature();
        signer = ecrecover(hash, v, r, s);
        if (signer == address(0)) revert InvalidSignature();
    }

    function _now() private view returns (uint64) {
        if (block.timestamp > type(uint64).max - MAX_WINDOW) revert InvalidTime();
        return uint64(block.timestamp);
    }
}
