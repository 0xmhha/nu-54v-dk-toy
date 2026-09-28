// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

/// @title Merchant registry interface frozen at the W6 contract gate ([N05][N07]).
/// @notice The registry is the final merchant authority that settle reads.
/// Payout-change functions are added in a later, additive interface (WBS2-P06-03).
interface IMerchantRegistry {
    event MerchantRegistered(address indexed merchant, address payout);
    event MerchantRevoked(address indexed merchant);

    error NotRegistryAdmin();
    error ZeroAddress();

    function registerMerchant(address merchant, address payout) external;
    function revokeMerchant(address merchant) external;

    function admin() external view returns (address);
    function isActive(address merchant) external view returns (bool);
    /// @notice Payout currently in effect for a merchant (zero address if never registered).
    function payoutOf(address merchant) external view returns (address);
}
