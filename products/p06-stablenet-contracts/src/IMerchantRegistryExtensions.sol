// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

/// @title Delayed payout changes for the merchant registry (P06-FR-17).
/// @notice Only adds to the IMerchantRegistry ABI, which stays as frozen. A payout change
/// takes effect payoutChangeDelay after it is queued, so the merchant can see it and object.
interface IMerchantRegistryExtensions {
    event PayoutChangeQueued(address indexed merchant, address payout, uint256 effectiveAt);
    event PayoutChangeCancelled(address indexed merchant);

    error InvalidParameters();
    error UnknownMerchant();
    /// @notice registerMerchant cannot change the payout of a known merchant; use requestPayoutChange.
    error PayoutChangeRequired();
    error NoPendingPayoutChange();
    error NotAdminOrMerchant();

    function requestPayoutChange(address merchant, address payout) external;
    /// @notice The admin or the merchant itself drops a queued change before it takes effect.
    /// The merchant's veto means a stolen admin key alone cannot redirect a payout.
    function cancelPayoutChange(address merchant) external;

    function payoutChangeDelay() external view returns (uint256);
    /// @notice isActive and payoutOf in one call; settle reads this.
    function merchantStatus(address merchant) external view returns (bool active, address payout);
    /// @notice Queued payout and the time it takes effect; zeros when nothing is queued.
    function pendingPayoutOf(address merchant) external view returns (address payout, uint256 effectiveAt);
}
