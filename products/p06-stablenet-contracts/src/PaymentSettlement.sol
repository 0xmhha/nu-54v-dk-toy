// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {IPaymentSettlement} from "./IPaymentSettlement.sol";
import {IPaymentSettlementExtensions} from "./IPaymentSettlementExtensions.sol";
import {IMerchantRegistry} from "./IMerchantRegistry.sol";
import {IMerchantRegistryExtensions} from "./IMerchantRegistryExtensions.sol";
import {IERC20Minimal} from "./interfaces/IERC20Minimal.sol";
import {PaymentTypes} from "./PaymentTypes.sol";

/// @title Pre-deposit settlement for device-signed payments ([N07][N08]).
/// @notice The operator deposits for a rented device; the device signs an EIP-712
/// PaymentAuthorization after a button press; anyone (the kiosk) submits it here.
/// No upgrade proxy and no pause (P06-NFR-01). Roles and parameters are fixed at
/// deployment; changing them means a new deployment.
contract PaymentSettlement is IPaymentSettlement, IPaymentSettlementExtensions {
    /// @dev Packed so that settle reads three slots and writes one (slot B).
    struct Account {
        address withdrawAddress; // slot A: set on the first deposit; non-zero means the account exists
        uint64 closedAt; // 0 = active
        uint128 balance; // slot B
        uint88 windowSpent; // <= dailyCap <= uint88 max (constructor)
        uint40 windowStart; // seconds; uint40 lasts about 35,000 years
        uint128 perPaymentLimit; // slot C: 0 = use perPaymentCap
        uint128 dailyLimit; // 0 = use dailyCap
        uint128 pendingWithdrawal; // slot D: requested amount; balance is not moved until execute
        uint64 withdrawAfter;
    }

    /// @dev secp256k1n / 2; signatures with a larger s are refused (low-s only).
    uint256 internal constant HALF_ORDER = 0x7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0;

    address public immutable override token;
    address public immutable override registry;
    address public immutable override operator;
    uint256 public immutable perPaymentCap;
    uint256 public immutable dailyCap;
    uint256 public immutable override withdrawalDelay;
    uint256 public immutable authorizationExpiry;
    bytes32 internal immutable domainSep;

    mapping(address => Account) internal accounts;
    mapping(address => mapping(uint256 => uint256)) internal nonceBitmap;
    mapping(address => mapping(bytes32 => bool)) internal paid;
    mapping(address => uint256) public override merchantBalance;
    /// @notice Sum of every device balance and merchant balance: what this contract owes.
    uint256 public override totalOwed;

    error InvalidParameters();

    /// @param token_ the deployment's test token (6 decimals)
    /// @param registry_ merchant registry that settle reads
    /// @param operator_ role key for deposits, withdrawals and closing
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
        // Both are fixed forever, so a typo must fail here rather than at the first call.
        if (token_.code.length == 0 || registry_.code.length == 0) revert InvalidParameters();
        // Ranges from the register parameters (design-freeze-checkpoint-02).
        if (perPaymentCap_ == 0 || perPaymentCap_ > dailyCap_ || dailyCap_ > type(uint88).max) {
            revert InvalidParameters();
        }
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

    modifier onlyOperator() {
        if (msg.sender != operator) revert NotOperator();
        _;
    }

    function domainSeparator() external view override returns (bytes32) {
        return domainSep;
    }

    /// @notice Operator funds a device account. The first deposit fixes the withdraw address;
    /// later deposits pass the same address or zero.
    function depositFor(address device, uint256 amount, address withdrawAddress) external override onlyOperator {
        if (device == address(0)) revert ZeroAddress();
        Account storage acct = accounts[device];
        if (acct.closedAt != 0) revert AccountInactive();
        if (acct.withdrawAddress == address(0)) {
            if (withdrawAddress == address(0)) revert ZeroAddress();
            // Fixed forever: the token and this contract could never receive a payout.
            if (withdrawAddress == token || withdrawAddress == address(this)) revert InvalidParameters();
            acct.withdrawAddress = withdrawAddress;
        } else if (withdrawAddress != address(0) && withdrawAddress != acct.withdrawAddress) {
            revert WithdrawAddressMismatch();
        }
        uint256 newBalance = uint256(acct.balance) + amount;
        if (newBalance > type(uint128).max) revert InvalidParameters(); // never truncate a balance
        // forge-lint: disable-next-line(unsafe-typecast)
        acct.balance = uint128(newBalance);
        totalOwed += amount;
        emit Deposited(device, amount);
        _pull(msg.sender, amount);
    }

    /// @notice Settles one device-signed payment. Checks run in the order of design 5.
    function settle(PaymentTypes.PaymentAuthorization calldata auth, bytes calldata sig) external override {
        // 1. domain
        if (auth.chainId != block.chainid || auth.contractAddress != address(this) || auth.token != token) {
            revert WrongDomain();
        }
        // A zero payment would claim (merchant, orderId) for free and block the real order.
        if (auth.amount == 0) revert InvalidParameters();
        // 2. order uniqueness first, so resubmitting a settled signature always ends here
        if (paid[auth.merchant][auth.orderId]) revert OrderAlreadyPaid();
        // 3. expiry window
        _checkExpiry(auth.expiry);
        // 4. signer must own an active account
        (address device, Account storage acct) =
            _activeSigner(PaymentTypes.digest(domainSep, PaymentTypes.hashAuthorization(auth)), sig);
        // 5-6. merchant authority and payout
        (bool active, address currentPayout) = IMerchantRegistryExtensions(registry).merchantStatus(auth.merchant);
        if (!active) revert MerchantRevoked();
        if (auth.payout != currentPayout) revert MerchantForged();
        // 7. unordered nonce, shared with setLimits
        _useNonce(device, auth.nonce);
        // 8. caps (fixed 24-hour window from its first payment)
        if (auth.amount > _cap(perPaymentCap, acct.perPaymentLimit)) revert OverCap();
        uint40 windowStart = acct.windowStart;
        uint256 spent = acct.windowSpent;
        if (block.timestamp >= uint256(windowStart) + 1 days) {
            windowStart = uint40(block.timestamp);
            spent = 0;
        }
        if (spent + auth.amount > _cap(dailyCap, acct.dailyLimit)) revert OverCap();
        // 9. balance
        if (auth.amount > acct.balance) revert InsufficientBalance();
        // 10. effects (the nonce bit is already set; a revert above undoes it)
        paid[auth.merchant][auth.orderId] = true;
        // amount <= balance (uint128) and spent + amount <= dailyCap <= uint88 max, so both casts are exact.
        // forge-lint: disable-next-line(unsafe-typecast)
        acct.balance -= uint128(auth.amount);
        acct.windowStart = windowStart;
        // forge-lint: disable-next-line(unsafe-typecast)
        acct.windowSpent = uint88(spent + auth.amount);
        merchantBalance[auth.merchant] += auth.amount;
        // 11.
        emit PaymentSettled(auth.merchant, auth.orderId, device, auth.amount, auth.nonce);
    }

    /// @notice Device-signed limit change. Limits stay at or below the register caps; 0 means the cap.
    function setLimits(PaymentTypes.LimitChange calldata change, bytes calldata sig) external override {
        if (change.chainId != block.chainid || change.contractAddress != address(this)) revert WrongDomain();
        _checkExpiry(change.expiry);
        (address device, Account storage acct) =
            _activeSigner(PaymentTypes.digest(domainSep, PaymentTypes.hashLimitChange(change)), sig);
        _useNonce(device, change.nonce);
        if (change.perPaymentLimit > perPaymentCap || change.dailyLimit > dailyCap) revert OverCap();
        // Both are at most a cap, and the caps fit uint128 (constructor).
        // forge-lint: disable-next-line(unsafe-typecast)
        acct.perPaymentLimit = uint128(change.perPaymentLimit);
        // forge-lint: disable-next-line(unsafe-typecast)
        acct.dailyLimit = uint128(change.dailyLimit);
        emit LimitsChanged(device, change.perPaymentLimit, change.dailyLimit);
    }

    /// @notice Records a withdrawal of `amount`. The balance stays, so payments signed before
    /// the request keep settling during the delay; execution pays min(amount, balance).
    function requestWithdrawal(address device, uint256 amount) external override onlyOperator {
        Account storage acct = _openAccount(device);
        if (amount == 0 || amount > type(uint128).max) revert InvalidParameters();
        // forge-lint: disable-next-line(unsafe-typecast)
        acct.pendingWithdrawal = uint128(amount);
        // Timestamps fit uint64 for billions of years; withdrawalDelay is a register value in seconds.
        // forge-lint: disable-next-line(unsafe-typecast)
        acct.withdrawAfter = uint64(block.timestamp + withdrawalDelay);
        emit WithdrawalRequested(device, amount);
    }

    /// @notice Undoes a mistaken request during the delay. A closed account's payout cannot be cancelled.
    function cancelWithdrawal(address device) external override onlyOperator {
        Account storage acct = _openAccount(device);
        if (acct.pendingWithdrawal == 0) revert NoPendingWithdrawal();
        if (block.timestamp >= acct.withdrawAfter) revert WithdrawalMatured(); // only during the delay
        acct.pendingWithdrawal = 0;
        acct.withdrawAfter = 0;
        emit WithdrawalCancelled(device);
    }

    function executeWithdrawal(address device) external override {
        Account storage acct = accounts[device];
        uint256 pending = acct.pendingWithdrawal;
        if (pending == 0) revert NoPendingWithdrawal();
        if (block.timestamp < acct.withdrawAfter) revert WithdrawalNotReady();
        // pendingWithdrawal is itself a uint128, so the cast is exact.
        // forge-lint: disable-next-line(unsafe-typecast)
        uint128 amount = pending < acct.balance ? uint128(pending) : acct.balance;
        acct.balance -= amount; // state first, then transfer
        totalOwed -= amount;
        acct.pendingWithdrawal = 0;
        acct.withdrawAfter = 0;
        emit WithdrawalExecuted(device, amount);
        if (amount != 0) _push(acct.withdrawAddress, amount);
    }

    /// @notice Stops the account at once (settle refuses it from now on) and queues its whole
    /// balance for the withdraw address after withdrawalDelay.
    function closeAccount(address device) external override onlyOperator {
        Account storage acct = _openAccount(device);
        acct.closedAt = uint64(block.timestamp);
        acct.pendingWithdrawal = acct.balance;
        // Timestamps fit uint64 for billions of years; withdrawalDelay is a register value in seconds.
        // forge-lint: disable-next-line(unsafe-typecast)
        acct.withdrawAfter = uint64(block.timestamp + withdrawalDelay);
        emit AccountClosed(device);
        emit WithdrawalRequested(device, acct.balance);
    }

    /// @notice A merchant withdraws its settled balance to its current payout address.
    function cashOut() external override {
        _cashOut(msg.sender);
    }

    /// @notice Same as cashOut, for a merchant that lost its key: the payout is still the registry's.
    function cashOutFor(address merchant) external override {
        _cashOut(merchant);
    }

    /// @notice Recovers tokens this contract holds beyond what it owes: a payout that a
    /// recipient rejected and that came back here, or tokens sent here directly. Owed
    /// balances stay untouchable; the recovery is public through SurplusRecovered.
    function recoverSurplus(address to, uint256 amount) external override onlyOperator {
        if (to == address(0)) revert ZeroAddress();
        if (to == token || to == address(this)) revert InvalidParameters();
        if (amount == 0 || amount > surplus()) revert InvalidParameters();
        emit SurplusRecovered(to, amount);
        _push(to, amount);
    }

    /// @notice Tokens held beyond totalOwed.
    function surplus() public view override returns (uint256) {
        uint256 held = IERC20Minimal(token).balanceOf(address(this));
        return held > totalOwed ? held - totalOwed : 0;
    }

    function balanceOf(address device) external view override returns (uint256) {
        return accounts[device].balance;
    }

    function accountOf(address device) external view override returns (AccountView memory v) {
        Account storage a = accounts[device];
        v = AccountView({
            balance: a.balance,
            closedAt: a.closedAt,
            withdrawAddress: a.withdrawAddress,
            pendingWithdrawal: a.pendingWithdrawal,
            withdrawAfter: a.withdrawAfter,
            perPaymentLimit: a.perPaymentLimit,
            dailyLimit: a.dailyLimit,
            windowStart: a.windowStart,
            windowSpent: a.windowSpent
        });
    }

    function isPaid(address merchant, bytes32 orderId) external view override returns (bool) {
        return paid[merchant][orderId];
    }

    function isNonceUsed(address device, uint256 nonce) external view override returns (bool) {
        return nonceBitmap[device][nonce >> 8] & (1 << (nonce & 0xff)) != 0;
    }

    function _cashOut(address merchant) internal {
        uint256 amount = merchantBalance[merchant];
        if (amount == 0) return; // nothing to pay; no empty transfer for anyone to spam
        address payout = IMerchantRegistry(registry).payoutOf(merchant);
        if (payout == address(0)) revert ZeroAddress();
        // The balance stays with the merchant until the registry names a payout that can receive it.
        if (payout == token || payout == address(this)) revert InvalidParameters();
        merchantBalance[merchant] = 0; // state first, then transfer
        totalOwed -= amount;
        emit CashedOut(merchant, payout, amount);
        _push(payout, amount);
    }

    function _checkExpiry(uint64 expiry) internal view {
        if (block.timestamp > expiry || expiry > block.timestamp + authorizationExpiry) revert Expired();
    }

    function _activeSigner(bytes32 hash, bytes calldata sig)
        internal
        view
        returns (address device, Account storage acct)
    {
        device = _recover(hash, sig);
        acct = accounts[device];
        if (device == address(0) || acct.withdrawAddress == address(0) || acct.closedAt != 0) {
            revert AccountInactive();
        }
    }

    function _openAccount(address device) internal view returns (Account storage acct) {
        acct = accounts[device];
        if (acct.withdrawAddress == address(0) || acct.closedAt != 0) revert AccountInactive();
    }

    function _useNonce(address device, uint256 nonce) internal {
        uint256 word = nonce >> 8;
        uint256 bit = 1 << (nonce & 0xff);
        uint256 bits = nonceBitmap[device][word];
        if (bits & bit != 0) revert NonceReplayed();
        nonceBitmap[device][word] = bits | bit;
    }

    function _cap(uint256 cap, uint256 limit) internal pure returns (uint256) {
        return limit == 0 || limit > cap ? cap : limit;
    }

    /// @dev Accepts tokens that return true or nothing; anything else is TransferFailed.
    function _push(address to, uint256 amount) internal {
        _call(abi.encodeCall(IERC20Minimal.transfer, (to, amount)));
    }

    function _pull(address from, uint256 amount) internal {
        _call(abi.encodeCall(IERC20Minimal.transferFrom, (from, address(this), amount)));
    }

    function _call(bytes memory data) internal {
        (bool ok, bytes memory ret) = token.call(data);
        if (!ok || (ret.length != 0 && (ret.length != 32 || abi.decode(ret, (uint256)) != 1))) {
            revert TransferFailed();
        }
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
