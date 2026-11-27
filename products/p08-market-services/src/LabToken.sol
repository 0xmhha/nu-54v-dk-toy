// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

/// @notice Fixed-behavior, faucet-only units tied to the market deployment.
contract LabToken {
    error InvalidTransfer();
    error Forbidden();

    string public name;
    string public symbol;
    uint8 public immutable decimals;
    address public immutable faucet;
    uint256 public totalSupply;
    mapping(address => uint256) public balanceOf;
    mapping(address => mapping(address => uint256)) public allowance;

    event Transfer(address indexed from, address indexed to, uint256 amount);
    event Approval(address indexed owner, address indexed spender, uint256 amount);

    constructor(string memory name_, string memory symbol_, uint8 decimals_, address faucet_) {
        if (block.chainid != 8283 || faucet_ == address(0)) revert Forbidden();
        name = name_;
        symbol = symbol_;
        decimals = decimals_;
        faucet = faucet_;
    }

    function mint(address to, uint256 amount) external {
        if (block.chainid != 8283 || msg.sender != faucet) revert Forbidden();
        if (to == address(0)) revert InvalidTransfer();
        totalSupply += amount;
        balanceOf[to] += amount;
        emit Transfer(address(0), to, amount);
    }

    function approve(address spender, uint256 amount) external returns (bool) {
        if (block.chainid != 8283) revert Forbidden();
        allowance[msg.sender][spender] = amount;
        emit Approval(msg.sender, spender, amount);
        return true;
    }

    function transfer(address to, uint256 amount) external returns (bool) {
        _transfer(msg.sender, to, amount);
        return true;
    }

    function transferFrom(address from, address to, uint256 amount) external returns (bool) {
        uint256 authorized = allowance[from][msg.sender];
        if (authorized != type(uint256).max) allowance[from][msg.sender] = authorized - amount;
        _transfer(from, to, amount);
        return true;
    }

    function _transfer(address from, address to, uint256 amount) private {
        if (block.chainid != 8283) revert Forbidden();
        if (to == address(0)) revert InvalidTransfer();
        balanceOf[from] -= amount;
        balanceOf[to] += amount;
        emit Transfer(from, to, amount);
    }
}
