// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Test} from "forge-std/Test.sol";
import {PaymentTypes} from "../src/PaymentTypes.sol";

/// Typehashes must be derived from the same encodeType strings as the shared vectors.
contract PaymentTypesTest is Test {
    string internal constant VECTORS = "../../docs/content/specifications/protocol/eip712-vectors.json";

    function test_typehashesMatchVectors() public view {
        string memory json = vm.readFile(VECTORS);
        string memory pa = vm.parseJsonString(json, ".vectors[0].encodeType");
        string memory lc = vm.parseJsonString(json, ".vectors[2].encodeType");
        assertEq(PaymentTypes.PAYMENT_AUTHORIZATION_TYPEHASH, keccak256(bytes(pa)));
        assertEq(PaymentTypes.LIMIT_CHANGE_TYPEHASH, keccak256(bytes(lc)));
    }
}
