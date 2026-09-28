// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {IMerchantRegistry} from "./IMerchantRegistry.sol";

/// @title Minimal merchant registry ([N05]).
/// @notice The admin is fixed at deployment; key rotation is a new deployment (design 6).
contract MerchantRegistry is IMerchantRegistry {
    struct Merchant {
        bool active;
        address payout;
        // Reserved for the delayed payout change added in WBS2-P06-03, so that
        // the storage layout stays additive.
        address pendingPayout;
        uint64 payoutEffectiveAt;
    }

    address public immutable override admin;
    mapping(address => Merchant) internal merchants;

    constructor(address admin_) {
        if (admin_ == address(0)) revert ZeroAddress();
        admin = admin_;
    }

    modifier onlyAdmin() {
        if (msg.sender != admin) revert NotRegistryAdmin();
        _;
    }

    /// @notice Registers or re-activates a merchant with its payout address.
    function registerMerchant(address merchant, address payout) external override onlyAdmin {
        if (merchant == address(0) || payout == address(0)) revert ZeroAddress();
        Merchant storage m = merchants[merchant];
        m.active = true;
        m.payout = payout;
        emit MerchantRegistered(merchant, payout);
    }

    /// @notice Revokes a merchant immediately; settle refuses it with MerchantRevoked.
    function revokeMerchant(address merchant) external override onlyAdmin {
        merchants[merchant].active = false;
        emit MerchantRevoked(merchant);
    }

    function isActive(address merchant) external view override returns (bool) {
        return merchants[merchant].active;
    }

    function payoutOf(address merchant) external view override returns (address) {
        return merchants[merchant].payout;
    }
}
