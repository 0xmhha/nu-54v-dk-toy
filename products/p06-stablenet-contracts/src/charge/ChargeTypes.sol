// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

/// @title The quote of one top-up and everything both chains must compute the same way.
/// @notice A traveller pays USDC on a source chain and a board receives won on StableNet. One signed quote
/// names both sides. Its digest is the deposit number, the nonce of the traveller's token authorization and
/// the payout number, so none of the three can be exchanged for another.
library ChargeTypes {
    /// @dev All members are static, so `abi.encode(typehash, quote)` is the EIP-712 encoding.
    struct Quote {
        uint8 method; // how the USDC arrives; see METHOD_AUTHORIZATION
        address usdc; // the token on the source chain
        address payer; // the traveller's own wallet; refunds only go back here
        uint256 usdcAmount; // 6 decimals
        uint256 wonChainId; // the chain of the pool
        address pool;
        address won; // the pool's asset: a token, or the token view of the chain's coin
        address board; // receives the won on StableNet
        uint256 wonAmount; // in 0.01 won, whatever the decimals of the asset
        uint256 rate; // 0.01 won for one whole USDC
        uint256 feeWon; // must be 0
        uint64 expiresAt; // the deposit must land before this
        uint64 payBy; // the pool pays before this: expiresAt + PAY_WINDOW
        bytes32 number; // the quote server's own number
    }

    /// @notice One EIP-3009 `receiveWithAuthorization` signed by the payer. The only method built.
    uint8 internal constant METHOD_AUTHORIZATION = 1;
    /// @notice The pool pays until this long after the quote ends.
    uint64 internal constant PAY_WINDOW = 2 hours;
    /// @notice Won amounts and limits count 0.01 won.
    uint8 internal constant WON_DECIMALS = 2;
    uint256 internal constant USDC_UNIT = 1e6;
    uint256 private constant HALF_ORDER = 0x7fffffffffffffffffffffffffffffff5d576e7357a4501ddfe92f46681b20a0;

    bytes32 internal constant DOMAIN_TYPEHASH =
        keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)");
    bytes32 internal constant QUOTE_TYPEHASH = keccak256(
        "ChargeQuote(uint8 method,address usdc,address payer,uint256 usdcAmount,uint256 wonChainId,address pool,address won,address board,uint256 wonAmount,uint256 rate,uint256 feeWon,uint64 expiresAt,uint64 payBy,bytes32 number)"
    );
    /// @notice What two signers, or one signer cancelling, sign. `round` makes a signature work once.
    bytes32 internal constant ACTION_TYPEHASH =
        keccak256("Action(bytes32 kind,bytes32 subject,uint256 round,uint256 until)");
    /// @notice The deposit-confirmation key tells the pool that a deposit is in a source-chain block.
    bytes32 internal constant CONFIRMATION_TYPEHASH =
        keccak256("DepositConfirmation(bytes32 depositId,bytes32 sourceBlockHash,bool fast,uint256 until)");
    /// @notice The deposit-confirmation key tells the deposit contract that the pool paid.
    bytes32 internal constant PAID_TYPEHASH =
        keccak256("PaidRecord(bytes32 depositId,bytes32 wonBlockHash,uint256 round,uint256 until)");
    /// @notice The payer says a deposit marked as paid never arrived.
    bytes32 internal constant CONTEST_TYPEHASH = keccak256("Contest(bytes32 depositId,uint256 round,uint256 until)");

    error InvalidSignature();

    /// @notice Every digest names a chain and a contract, so nothing signed for one deployment works on another.
    function domain(uint256 chainId, address verifyingContract) internal pure returns (bytes32) {
        return
            keccak256(abi.encode(DOMAIN_TYPEHASH, keccak256("NU54 Charge"), keccak256("1"), chainId, verifyingContract));
    }

    function digest(bytes32 separator, bytes32 structHash) internal pure returns (bytes32) {
        return keccak256(abi.encodePacked(hex"1901", separator, structHash));
    }

    function quoteDigest(bytes32 separator, Quote calldata quote) internal pure returns (bytes32) {
        return digest(separator, keccak256(abi.encode(QUOTE_TYPEHASH, quote)));
    }

    /// @notice The won a USDC amount buys. Anything below 0.01 won is dropped.
    function wonFor(uint256 usdcAmount, uint256 rate) internal pure returns (uint256) {
        return usdcAmount * rate / USDC_UNIT;
    }

    /// @notice The least USDC that buys a won amount: rounded up, so the pool never gives more than was paid for.
    function usdcFor(uint256 wonAmount, uint256 rate) internal pure returns (uint256) {
        return (wonAmount * USDC_UNIT + rate - 1) / rate;
    }

    function recover(bytes32 hash, bytes calldata signature) internal pure returns (address signer) {
        if (signature.length != 65) revert InvalidSignature();
        (bytes32 r, bytes32 s, uint8 v) = (bytes32(signature[0:32]), bytes32(signature[32:64]), uint8(signature[64]));
        if ((v != 27 && v != 28) || uint256(s) > HALF_ORDER) revert InvalidSignature();
        signer = ecrecover(hash, v, r, s);
        if (signer == address(0)) revert InvalidSignature();
    }
}
