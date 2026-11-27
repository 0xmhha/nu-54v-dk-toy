// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Vm} from "forge-std/Vm.sol";

/// @notice A token that misbehaves in one chosen way, for both sides of a top-up. Its signed transfer checks
/// no signature: only how the money moves matters here. Never deployed.
contract OddToken {
    enum Mode {
        Normal,
        ReturnsFalse, // answers false and moves nothing
        Silent, // answers true and moves nothing
        Fee, // keeps one unit of every transfer
        Reverts,
        NoAnswer, // moves the money and answers nothing, like some old tokens
        Reenters // calls back into `target` while the money moves
    }

    uint8 public immutable decimals;
    Mode public mode;
    address public target;
    bytes public callback;
    uint256 public reentries;
    bytes public reentryAnswer;
    mapping(address => uint256) public balanceOf;

    constructor(uint8 decimals_) {
        decimals = decimals_;
    }

    function mint(address to, uint256 amount) external {
        balanceOf[to] += amount;
    }

    function setMode(Mode mode_) external {
        mode = mode_;
    }

    function setCallback(address target_, bytes calldata callback_) external {
        target = target_;
        callback = callback_;
    }

    function transfer(address to, uint256 amount) external returns (bool) {
        if (mode == Mode.ReturnsFalse) return false;
        _move(msg.sender, to, amount);
        if (mode == Mode.NoAnswer) {
            assembly ("memory-safe") {
                return(0, 0)
            }
        }
        return true;
    }

    function receiveWithAuthorization(
        address from,
        address to,
        uint256 value,
        uint256,
        uint256,
        bytes32,
        uint8,
        bytes32,
        bytes32
    ) external {
        _move(from, to, value);
    }

    function _move(address from, address to, uint256 amount) private {
        if (mode == Mode.Reverts) revert("odd token: refused");
        if (mode == Mode.Silent) return;
        if (mode == Mode.Reenters && reentries == 0) {
            reentries += 1;
            (, reentryAnswer) = target.call(callback);
        }
        balanceOf[from] -= amount;
        balanceOf[to] += mode == Mode.Fee ? amount - 1 : amount;
    }
}

/// @notice The chain's coin seen as a token, the way StableNet shows KRWC at one fixed address: the token
/// balance of an account is its coin balance, and a token transfer moves coins. The test puts this code at
/// that address. Never deployed.
contract CoinView {
    Vm private constant vm = Vm(address(uint160(uint256(keccak256("hevm cheat code")))));

    string public constant name = "WKRC";
    string public constant symbol = "WKRC";
    uint8 public constant decimals = 18;
    mapping(address => bool) public isBlacklisted;

    function setBlacklisted(address account, bool blocked) external {
        isBlacklisted[account] = blocked;
    }

    function balanceOf(address account) external view returns (uint256) {
        return account.balance;
    }

    function transfer(address to, uint256 value) external returns (bool) {
        require(!isBlacklisted[msg.sender] && !isBlacklisted[to], "NativeCoinAdapter: account is blacklisted");
        require(value <= msg.sender.balance, "NativeCoinAdapter: transfer amount exceeds balance");
        vm.deal(msg.sender, msg.sender.balance - value);
        vm.deal(to, to.balance + value);
        return true;
    }
}

/// @notice A token that shows and moves the same balances at a second address, the way a chain could show
/// its coin as a token in two places. Never deployed.
contract TwoViewToken {
    uint8 public constant decimals = 2;
    address public immutable second;
    mapping(address => uint256) public balanceOf;

    constructor() {
        second = address(new SecondView(this));
    }

    function mint(address to, uint256 amount) external {
        balanceOf[to] += amount;
    }

    function transfer(address to, uint256 amount) external returns (bool) {
        _move(msg.sender, to, amount);
        return true;
    }

    function moveFor(address from, address to, uint256 amount) external {
        require(msg.sender == second, "two views: not the second view");
        _move(from, to, amount);
    }

    function _move(address from, address to, uint256 amount) private {
        balanceOf[from] -= amount;
        balanceOf[to] += amount;
    }
}

contract SecondView {
    TwoViewToken internal immutable first;

    constructor(TwoViewToken first_) {
        first = first_;
    }

    function balanceOf(address account) external view returns (uint256) {
        return first.balanceOf(account);
    }

    function transfer(address to, uint256 amount) external returns (bool) {
        first.moveFor(msg.sender, to, amount);
        return true;
    }
}
