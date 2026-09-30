// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {PaymentTypes} from "./PaymentTypes.sol";

/// @title Settlement extensions: limits, delayed withdrawals, account closing, cash-out on
/// a merchant's behalf and surplus recovery (P06-FR-13 to P06-FR-16, P06-FR-19).
/// @notice Everything here only adds to the IPaymentSettlement ABI, which stays as frozen.
interface IPaymentSettlementExtensions {
    /// @notice One device account as stored, for operator tools and indexers.
    struct AccountView {
        uint256 balance;
        uint256 closedAt; // 0 = active
        address withdrawAddress; // zero = no account
        uint256 pendingWithdrawal;
        uint256 withdrawAfter;
        uint256 perPaymentLimit; // 0 = perPaymentCap
        uint256 dailyLimit; // 0 = dailyCap
        uint256 windowStart;
        uint256 windowSpent;
    }

    event LimitsChanged(address indexed device, uint256 perPayment, uint256 daily);
    event WithdrawalRequested(address indexed device, uint256 amount);
    event WithdrawalCancelled(address indexed device);
    event WithdrawalExecuted(address indexed device, uint256 amount);
    event AccountClosed(address indexed device);
    event SurplusRecovered(address indexed to, uint256 amount);

    error NoPendingWithdrawal();
    error WithdrawalNotReady();
    error WithdrawAddressMismatch();
    error WithdrawalMatured();

    /// @notice Device-signed limit change, submitted by anyone (the kiosk).
    function setLimits(PaymentTypes.LimitChange calldata change, bytes calldata sig) external;
    /// @notice Operator records a withdrawal; the balance moves only on execution.
    function requestWithdrawal(address device, uint256 amount) external;
    function cancelWithdrawal(address device) external;
    /// @notice Anyone pays out a matured withdrawal, always to the account's withdraw address.
    function executeWithdrawal(address device) external;
    /// @notice Operator closes an account at once and queues its whole balance for withdrawal.
    function closeAccount(address device) external;
    /// @notice Anyone pays a merchant's balance to the merchant's registry payout.
    function cashOutFor(address merchant) external;
    /// @notice Operator recovers only tokens held beyond totalOwed.
    function recoverSurplus(address to, uint256 amount) external;

    function withdrawalDelay() external view returns (uint256);
    function totalOwed() external view returns (uint256);
    function surplus() external view returns (uint256);
    function accountOf(address device) external view returns (AccountView memory);
}
