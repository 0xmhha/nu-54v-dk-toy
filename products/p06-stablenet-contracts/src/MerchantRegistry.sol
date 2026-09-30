// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {IMerchantRegistry} from "./IMerchantRegistry.sol";
import {IMerchantRegistryExtensions} from "./IMerchantRegistryExtensions.sol";

/// @title Minimal merchant registry ([N05]).
/// @notice The admin and the payout change delay are fixed at deployment; key rotation is a
/// new deployment (design 6). Revocation is immediate; a payout change waits payoutChangeDelay.
contract MerchantRegistry is IMerchantRegistry, IMerchantRegistryExtensions {
    struct Merchant {
        bool active;
        bool hasPending; // same slot as payout, so settle reads one slot when nothing is queued
        address payout;
        address pendingPayout;
        uint64 payoutEffectiveAt;
    }

    address public immutable override admin;
    uint256 public immutable override payoutChangeDelay;
    mapping(address => Merchant) internal merchants;

    /// @param payoutChangeDelay_ seconds before a queued payout takes effect; the deployment
    /// checks it against attestationValidity from the register.
    constructor(address admin_, uint256 payoutChangeDelay_) {
        if (admin_ == address(0)) revert ZeroAddress();
        if (payoutChangeDelay_ == 0) revert InvalidParameters();
        admin = admin_;
        payoutChangeDelay = payoutChangeDelay_;
    }

    modifier onlyAdmin() {
        if (msg.sender != admin) revert NotRegistryAdmin();
        _;
    }

    /// @notice Registers a new merchant, or re-activates a revoked one with its current payout.
    function registerMerchant(address merchant, address payout) external override onlyAdmin {
        if (merchant == address(0) || payout == address(0)) revert ZeroAddress();
        Merchant storage m = merchants[merchant];
        address current = _effectivePayout(m);
        if (current != address(0) && current != payout) revert PayoutChangeRequired();
        m.active = true;
        m.payout = payout;
        emit MerchantRegistered(merchant, payout);
    }

    /// @notice Revokes a merchant immediately; settle refuses it with MerchantRevoked.
    function revokeMerchant(address merchant) external override onlyAdmin {
        Merchant storage m = merchants[merchant];
        if (m.payout == address(0)) revert UnknownMerchant();
        m.active = false;
        emit MerchantRevoked(merchant);
    }

    /// @notice Queues a payout change. It replaces any change that has not taken effect yet.
    function requestPayoutChange(address merchant, address payout) external override onlyAdmin {
        if (payout == address(0)) revert ZeroAddress();
        Merchant storage m = merchants[merchant];
        if (m.payout == address(0)) revert UnknownMerchant();
        m.payout = _effectivePayout(m); // settle a change that already took effect
        // forge-lint: disable-next-line(unsafe-typecast)
        uint64 effectiveAt = uint64(block.timestamp + payoutChangeDelay); // register value in seconds
        m.hasPending = true;
        m.pendingPayout = payout;
        m.payoutEffectiveAt = effectiveAt;
        emit PayoutChangeQueued(merchant, payout, effectiveAt);
    }

    /// @notice Drops a queued payout change before it takes effect. The merchant can do this
    /// too, so a stolen admin key cannot move a payout while the merchant is watching.
    function cancelPayoutChange(address merchant) external override {
        if (msg.sender != admin && msg.sender != merchant) revert NotAdminOrMerchant();
        Merchant storage m = merchants[merchant];
        if (!m.hasPending || block.timestamp >= m.payoutEffectiveAt) revert NoPendingPayoutChange();
        m.hasPending = false;
        m.pendingPayout = address(0);
        m.payoutEffectiveAt = 0;
        emit PayoutChangeCancelled(merchant);
    }

    function merchantStatus(address merchant) external view override returns (bool active, address payout) {
        Merchant storage m = merchants[merchant];
        return (m.active, _effectivePayout(m));
    }

    function isActive(address merchant) external view override returns (bool) {
        return merchants[merchant].active;
    }

    function payoutOf(address merchant) external view override returns (address) {
        return _effectivePayout(merchants[merchant]);
    }

    function pendingPayoutOf(address merchant) external view override returns (address payout, uint256 effectiveAt) {
        Merchant storage m = merchants[merchant];
        if (!m.hasPending || block.timestamp >= m.payoutEffectiveAt) return (address(0), 0);
        return (m.pendingPayout, m.payoutEffectiveAt);
    }

    function _effectivePayout(Merchant storage m) internal view returns (address) {
        if (m.hasPending && block.timestamp >= m.payoutEffectiveAt) return m.pendingPayout;
        return m.payout;
    }
}
