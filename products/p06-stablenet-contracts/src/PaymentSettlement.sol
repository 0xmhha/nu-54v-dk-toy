// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {IPaymentSettlement} from "./IPaymentSettlement.sol";
import {IMerchantRegistry} from "./IMerchantRegistry.sol";
import {IERC20Minimal} from "./interfaces/IERC20Minimal.sol";
import {PaymentTypes} from "./PaymentTypes.sol";

/// @title Pre-deposit settlement for device-signed payments ([N07][N08]).
/// @notice The operator deposits for a rented device; the device signs an EIP-712
/// PaymentAuthorization after a button press; anyone (the kiosk) submits it here.
/// No upgrade proxy and no pause (P06-NFR-01). Roles and parameters are fixed at
/// deployment; changing them means a new deployment.
contract PaymentSettlement is IPaymentSettlement {
    struct Account {
        uint128 balance;
        uint64 closedAt; // 0 = active
        uint64 withdrawAfter;
        address withdrawAddress; // set on the first deposit; non-zero means the account exists
        uint128 pendingWithdrawal;
        uint128 perPaymentLimit; // 0 = use perPaymentCap
        uint128 dailyLimit; // 0 = use dailyCap
        uint64 windowStart;
        uint128 windowSpent;
    }

    /// @dev secp256k1n / 2; signatures with a larger s are refused (low-s only).
    uint256 internal constant HALF_ORDER = 0x7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0;

    address public immutable override token;
    address public immutable override registry;
    address public immutable override operator;
    uint256 public immutable perPaymentCap;
    uint256 public immutable dailyCap;
    uint256 public immutable withdrawalDelay;
    uint256 public immutable authorizationExpiry;
    bytes32 internal immutable domainSep;

    mapping(address => Account) internal accounts;
    mapping(address => mapping(uint256 => uint256)) internal nonceBitmap;
    mapping(address => mapping(bytes32 => bool)) internal paid;
    mapping(address => uint256) public override merchantBalance;

    error InvalidParameters();

    /// @param token_ the deployment's test token (6 decimals)
    /// @param registry_ merchant registry that settle reads
    /// @param operator_ role key for deposits (and, later, withdrawals and closing)
    /// @param perPaymentCap_ per-payment cap in token base units
    /// @param dailyCap_ cap per fixed 24-hour window in token base units
    /// @param withdrawalDelay_ seconds between a withdrawal request and its execution
    /// @param authorizationExpiry_ the longest allowed lifetime of a signed authorization, in seconds
    constructor(
        address token_,
        address registry_,
        address operator_,
        uint256 perPaymentCap_,
        uint256 dailyCap_,
        uint256 withdrawalDelay_,
        uint256 authorizationExpiry_
    ) {
        if (token_ == address(0) || registry_ == address(0) || operator_ == address(0)) {
            revert ZeroAddress();
        }
        // Ranges from the register parameters (design-freeze-checkpoint-02).
        if (perPaymentCap_ == 0 || perPaymentCap_ > dailyCap_) revert InvalidParameters();
        if (authorizationExpiry_ < 30 || authorizationExpiry_ > 300) revert InvalidParameters();
        if (withdrawalDelay_ < authorizationExpiry_) revert InvalidParameters();
        token = token_;
        registry = registry_;
        operator = operator_;
        perPaymentCap = perPaymentCap_;
        dailyCap = dailyCap_;
        withdrawalDelay = withdrawalDelay_;
        authorizationExpiry = authorizationExpiry_;
        domainSep = PaymentTypes.domainSeparator(block.chainid, address(this));
    }

    function domainSeparator() external view override returns (bytes32) {
        return domainSep;
    }

    /// @notice Operator funds a device account. The first deposit fixes the withdraw address.
    function depositFor(address device, uint256 amount, address withdrawAddress) external override {
        if (msg.sender != operator) revert NotOperator();
        if (device == address(0)) revert ZeroAddress();
        Account storage acct = accounts[device];
        if (acct.closedAt != 0) revert AccountInactive();
        if (acct.withdrawAddress == address(0)) {
            if (withdrawAddress == address(0)) revert ZeroAddress();
            acct.withdrawAddress = withdrawAddress;
        }
        uint256 newBalance = uint256(acct.balance) + amount;
        if (newBalance > type(uint128).max) revert InvalidParameters(); // never truncate a balance
        // forge-lint: disable-next-line(unsafe-typecast)
        acct.balance = uint128(newBalance);
        emit Deposited(device, amount);
        if (!IERC20Minimal(token).transferFrom(msg.sender, address(this), amount)) revert TransferFailed();
    }

    /// @notice Settles one device-signed payment. Checks run in the order of design 5.
    function settle(PaymentTypes.PaymentAuthorization calldata auth, bytes calldata sig) external override {
        // 1. domain
        if (auth.chainId != block.chainid || auth.contractAddress != address(this) || auth.token != token) {
            revert WrongDomain();
        }
        // 2. order uniqueness first, so resubmitting a settled signature always ends here
        if (paid[auth.merchant][auth.orderId]) revert OrderAlreadyPaid();
        // 3. expiry window
        if (block.timestamp > auth.expiry || auth.expiry > block.timestamp + authorizationExpiry) revert Expired();
        // 4. signer must own an active account
        address device = _recover(PaymentTypes.digest(domainSep, PaymentTypes.hashAuthorization(auth)), sig);
        Account storage acct = accounts[device];
        if (device == address(0) || acct.withdrawAddress == address(0) || acct.closedAt != 0) {
            revert AccountInactive();
        }
        // 5-6. merchant authority and payout
        IMerchantRegistry reg = IMerchantRegistry(registry);
        if (!reg.isActive(auth.merchant)) revert MerchantRevoked();
        if (auth.payout != reg.payoutOf(auth.merchant)) revert MerchantForged();
        // 7. unordered nonce
        uint256 word = auth.nonce >> 8;
        uint256 bit = 1 << (auth.nonce & 0xff);
        if (nonceBitmap[device][word] & bit != 0) revert NonceReplayed();
        // 8. caps (fixed 24-hour window from its first payment)
        if (auth.amount > _cap(perPaymentCap, acct.perPaymentLimit)) revert OverCap();
        uint64 windowStart = acct.windowStart;
        uint256 spent = acct.windowSpent;
        if (block.timestamp >= uint256(windowStart) + 1 days) {
            windowStart = uint64(block.timestamp);
            spent = 0;
        }
        if (spent + auth.amount > _cap(dailyCap, acct.dailyLimit)) revert OverCap();
        // 9. balance
        if (auth.amount > acct.balance) revert InsufficientBalance();
        // 10. effects
        nonceBitmap[device][word] |= bit;
        paid[auth.merchant][auth.orderId] = true;
        // amount <= balance (uint128) and spent + amount <= dailyCap, so both casts are exact.
        // forge-lint: disable-next-line(unsafe-typecast)
        acct.balance -= uint128(auth.amount);
        acct.windowStart = windowStart;
        // forge-lint: disable-next-line(unsafe-typecast)
        acct.windowSpent = uint128(spent + auth.amount);
        merchantBalance[auth.merchant] += auth.amount;
        // 11.
        emit PaymentSettled(auth.merchant, auth.orderId, device, auth.amount, auth.nonce);
    }

    /// @notice A merchant withdraws its settled balance to its current payout address.
    function cashOut() external override {
        uint256 amount = merchantBalance[msg.sender];
        address payout = IMerchantRegistry(registry).payoutOf(msg.sender);
        if (payout == address(0)) revert ZeroAddress();
        merchantBalance[msg.sender] = 0; // state first, then transfer
        emit CashedOut(msg.sender, payout, amount);
        if (!IERC20Minimal(token).transfer(payout, amount)) revert TransferFailed();
    }

    function balanceOf(address device) external view override returns (uint256) {
        return accounts[device].balance;
    }

    function isPaid(address merchant, bytes32 orderId) external view override returns (bool) {
        return paid[merchant][orderId];
    }

    function isNonceUsed(address device, uint256 nonce) external view override returns (bool) {
        return nonceBitmap[device][nonce >> 8] & (1 << (nonce & 0xff)) != 0;
    }

    function _cap(uint256 cap, uint256 limit) internal pure returns (uint256) {
        return limit == 0 || limit > cap ? cap : limit;
    }

    /// @dev 65-byte r || s || v signature, low-s only. Returns address(0) on any malformed input.
    function _recover(bytes32 hash, bytes calldata sig) internal pure returns (address) {
        if (sig.length != 65) return address(0);
        bytes32 r = bytes32(sig[0:32]);
        bytes32 s = bytes32(sig[32:64]);
        uint8 v = uint8(sig[64]);
        if (uint256(s) > HALF_ORDER || (v != 27 && v != 28)) return address(0);
        return ecrecover(hash, v, r, s);
    }
}
