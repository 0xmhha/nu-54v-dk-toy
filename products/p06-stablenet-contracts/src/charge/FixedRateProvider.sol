// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {ChargeTypes} from "./ChargeTypes.sol";

/// @notice Where a top-up rate comes from: a value, the time it was set and the name of its source.
/// A market source can replace the fixed one behind the same three answers.
interface IRateProvider {
    /// @return rate 0.01 won for one whole USDC
    /// @return updatedAt when the source set this value, in seconds
    /// @return name the source, as shown in records
    function latestRate() external view returns (uint256 rate, uint64 updatedAt, string memory name);
}

/// @title One exchange rate that never moves.
/// @notice The quote server reads it; the deposit contract and the pool only accept a quote whose rate lies in
/// their own range, which is set to this one value while the rate is fixed. A new rate is a new deployment.
contract FixedRateProvider is IRateProvider {
    uint256 public immutable rate;
    uint64 public immutable setAt;
    string public name;

    error InvalidRate();

    constructor(uint256 rate_, string memory name_) {
        if (rate_ == 0 || bytes(name_).length == 0) revert InvalidRate();
        rate = rate_;
        setAt = uint64(block.timestamp);
        name = name_;
    }

    function latestRate() external view returns (uint256, uint64, string memory) {
        return (rate, setAt, name);
    }

    /// @notice The won, in 0.01 won, that a USDC amount buys. Anything smaller is dropped.
    function wonFor(uint256 usdcAmount) external view returns (uint256) {
        return ChargeTypes.wonFor(usdcAmount, rate);
    }

    /// @notice The least USDC that buys a won amount, rounded up.
    function usdcFor(uint256 wonAmount) external view returns (uint256) {
        return ChargeTypes.usdcFor(wonAmount, rate);
    }
}
