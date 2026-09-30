// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Test} from "forge-std/Test.sol";
import {PaymentTypes} from "../src/PaymentTypes.sol";

/// P06-FR-03: the contract's EIP-712 hashing reproduces the shared vectors ([N21]).
contract Eip712VectorsTest is Test {
    string internal constant VECTORS = "../../docs/content/specifications/protocol/eip712-vectors.json";

    function test_eip712_matchesVectors() public view {
        string memory json = vm.readFile(VECTORS);
        bytes32 domainSep = PaymentTypes.domainSeparator(
            vm.parseJsonUint(json, ".domain.chainId"), vm.parseJsonAddress(json, ".domain.verifyingContract")
        );
        for (uint256 i = 0; i < 2; i++) {
            string memory p = string.concat(".vectors[", vm.toString(i), "]");
            PaymentTypes.PaymentAuthorization memory a = PaymentTypes.PaymentAuthorization({
                chainId: vm.parseUint(vm.parseJsonString(json, string.concat(p, ".message.chainId"))),
                contractAddress: vm.parseJsonAddress(json, string.concat(p, ".message.contract")),
                merchant: vm.parseJsonAddress(json, string.concat(p, ".message.merchant")),
                payout: vm.parseJsonAddress(json, string.concat(p, ".message.payout")),
                token: vm.parseJsonAddress(json, string.concat(p, ".message.token")),
                amount: vm.parseUint(vm.parseJsonString(json, string.concat(p, ".message.amount"))),
                orderId: vm.parseJsonBytes32(json, string.concat(p, ".message.orderId")),
                nonce: vm.parseUint(vm.parseJsonString(json, string.concat(p, ".message.nonce"))),
                expiry: uint64(vm.parseUint(vm.parseJsonString(json, string.concat(p, ".message.expiry"))))
            });
            bytes32 d = keccak256(abi.encodePacked("\x19\x01", domainSep, _hash(a)));
            assertEq(d, vm.parseJsonBytes32(json, string.concat(p, ".digest")), "digest");
            bytes memory sig = vm.parseJsonBytes(json, string.concat(p, ".signature"));
            (bytes32 r, bytes32 s) = (bytes32(_slice(sig, 0)), bytes32(_slice(sig, 32)));
            address signer = ecrecover(d, uint8(sig[64]), r, s);
            assertEq(signer, vm.parseJsonAddress(json, string.concat(p, ".signer")), "signer");
        }
        _checkLimitChange(json, domainSep);
    }

    /// Vector 2 is LimitChange (LC-01), the other type the device signs.
    function _checkLimitChange(string memory json, bytes32 domainSep) internal pure {
        string memory p = ".vectors[2]";
        assertEq(vm.parseJsonString(json, string.concat(p, ".primaryType")), "LimitChange");
        bytes32 structHash = keccak256(
            abi.encode(
                PaymentTypes.LIMIT_CHANGE_TYPEHASH,
                vm.parseUint(vm.parseJsonString(json, string.concat(p, ".message.chainId"))),
                vm.parseJsonAddress(json, string.concat(p, ".message.contract")),
                vm.parseUint(vm.parseJsonString(json, string.concat(p, ".message.perPaymentLimit"))),
                vm.parseUint(vm.parseJsonString(json, string.concat(p, ".message.dailyLimit"))),
                vm.parseUint(vm.parseJsonString(json, string.concat(p, ".message.nonce"))),
                vm.parseUint(vm.parseJsonString(json, string.concat(p, ".message.expiry")))
            )
        );
        bytes32 d = keccak256(abi.encodePacked("\x19\x01", domainSep, structHash));
        assertEq(d, vm.parseJsonBytes32(json, string.concat(p, ".digest")), "LimitChange digest");
        bytes memory sig = vm.parseJsonBytes(json, string.concat(p, ".signature"));
        address signer = ecrecover(d, uint8(sig[64]), bytes32(_slice(sig, 0)), bytes32(_slice(sig, 32)));
        assertEq(signer, vm.parseJsonAddress(json, string.concat(p, ".signer")), "LimitChange signer");
    }

    function _hash(PaymentTypes.PaymentAuthorization memory a) internal pure returns (bytes32) {
        return keccak256(
            abi.encode(
                PaymentTypes.PAYMENT_AUTHORIZATION_TYPEHASH,
                a.chainId,
                a.contractAddress,
                a.merchant,
                a.payout,
                a.token,
                a.amount,
                a.orderId,
                a.nonce,
                a.expiry
            )
        );
    }

    function _slice(bytes memory b, uint256 off) internal pure returns (uint256 x) {
        assembly {
            x := mload(add(add(b, 32), off))
        }
    }
}
