// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {PaymentTypes} from "./PaymentTypes.sol";

/// @title Settlement extensions added after the W6 gate (WBS2-P06-03, WBS2-P06-04).
/// @notice Not implemented yet. Listed here so their names are fixed early; they
/// only add to the IPaymentSettlement ABI.
interface IPaymentSettlementExtensions {
    event LimitsChanged(address indexed device, uint256 perPayment, uint256 daily);
    event WithdrawalRequested(address indexed device, uint256 amount);
    event WithdrawalCancelled(address indexed device);
    event WithdrawalExecuted(address indexed device, uint256 amount);
    event AccountClosed(address indexed device);

    function setLimits(PaymentTypes.LimitChange calldata change, bytes calldata sig) external;
    function requestWithdrawal(address device, uint256 amount) external;
    function cancelWithdrawal(address device) external;
    function executeWithdrawal(address device) external;
    function closeAccount(address device) external;
}
