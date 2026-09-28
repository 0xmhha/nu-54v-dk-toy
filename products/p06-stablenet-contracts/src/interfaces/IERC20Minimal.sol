// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

/// @notice The ERC-20 calls the settlement contract makes.
interface IERC20Minimal {
    function transfer(address to, uint256 amount) external returns (bool);
    function transferFrom(address from, address to, uint256 amount) external returns (bool);
    function balanceOf(address account) external view returns (uint256);
}
