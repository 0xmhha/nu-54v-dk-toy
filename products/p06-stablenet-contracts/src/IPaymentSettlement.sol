// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {PaymentTypes} from "./PaymentTypes.sol";

/// @title Settlement interface frozen at the W6 contract gate ([N07][N08]).
/// @notice Payment, deposit and merchant cash-out. Limits, withdrawals and account
/// closing are an additive extension (IPaymentSettlementExtensions, WBS2-P06-03/04),
/// so this ABI does not change when they land.
interface IPaymentSettlement {
    event Deposited(address indexed device, uint256 amount);
    event PaymentSettled(
        address indexed merchant, bytes32 indexed orderId, address indexed device, uint256 amount, uint256 nonce
    );
    event CashedOut(address indexed merchant, address indexed payout, uint256 amount);

    // settle reverts, in the order settle checks them ([N07]).
    error WrongDomain();
    error OrderAlreadyPaid();
    error Expired();
    error AccountInactive();
    error MerchantRevoked();
    error MerchantForged();
    error NonceReplayed();
    error OverCap();
    error InsufficientBalance();
    // Other functions.
    error NotOperator();
    error ZeroAddress();
    error TransferFailed();

    function depositFor(address device, uint256 amount, address withdrawAddress) external;
    function settle(PaymentTypes.PaymentAuthorization calldata auth, bytes calldata sig) external;
    function cashOut() external;

    function token() external view returns (address);
    function registry() external view returns (address);
    function operator() external view returns (address);
    function domainSeparator() external view returns (bytes32);
    function balanceOf(address device) external view returns (uint256);
    function merchantBalance(address merchant) external view returns (uint256);
    function isPaid(address merchant, bytes32 orderId) external view returns (bool);
    function isNonceUsed(address device, uint256 nonce) external view returns (bool);
}
