// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {PaymentTypes} from "./PaymentTypes.sol";

/// @title Settlement contract interface frozen at the W6 contract gate ([N07][N08]).
/// @notice Function, event and error names follow docs/content/products/p06/design.md.
interface IPaymentSettlement {
    event Deposited(address indexed device, uint256 amount);
    event PaymentSettled(
        address indexed merchant, bytes32 indexed orderId, address indexed device, uint256 amount, uint256 nonce
    );
    event LimitsChanged(address indexed device, uint256 perPayment, uint256 daily);
    event WithdrawalRequested(address indexed device, uint256 amount);
    event WithdrawalCancelled(address indexed device);
    event WithdrawalExecuted(address indexed device, uint256 amount);
    event AccountClosed(address indexed device);

    // Revert reasons in the order settle checks them ([N07]).
    error WrongDomain();
    error OrderAlreadyPaid();
    error Expired();
    error AccountInactive();
    error MerchantRevoked();
    error MerchantForged();
    error NonceReplayed();
    error OverCap();
    error InsufficientBalance();

    function depositFor(address device, uint256 amount, address withdrawAddress) external;
    function settle(PaymentTypes.PaymentAuthorization calldata auth, bytes calldata sig) external;
    function setLimits(PaymentTypes.LimitChange calldata change, bytes calldata sig) external;
    function requestWithdrawal(address device, uint256 amount) external;
    function cancelWithdrawal(address device) external;
    function executeWithdrawal(address device) external;
    function closeAccount(address device) external;
    function cashOut() external;
}
