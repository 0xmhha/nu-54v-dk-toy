// SPDX-License-Identifier: MIT
pragma solidity ^0.8.30;

import {Test} from "forge-std/Test.sol";

/// Independent P06 consumption of the v2 publishable byte fixture; no v2 funds path.
contract V2QuoteContractBoundary is Test {
    bytes32 private constant DOMAIN_TYPEHASH =
        keccak256("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)");
    bytes32 private constant QUOTE_TYPEHASH = keccak256(
        "MerchantQuote(bytes32 quoteId,bytes32 tenantCommitment,bytes32 branchCommitment,bytes32 orderId,bytes32 merchantId,bytes32 claimCommitment,bytes32 intentCommitment,address payer,address asset,address beneficiary,uint256 amount,uint256 nonce,uint64 merchantKeyVersion,uint64 beneficiaryVersion,uint64 payerGeneration,uint64 issuedAt,uint64 expiresAt,uint8 mode,uint8 payerRole)"
    );

    function test_v2SdkQuoteFixtureAtContractBoundary() external view {
        string memory path = "../../packages/protocol/v2/testdata/quote.vectors";
        vm.readLine(path);
        string[] memory kiosk = vm.split(vm.readLine(path), "|");
        string[] memory phone = vm.split(vm.readLine(path), "|");
        bytes memory raw = vm.parseBytes(string.concat("0x", kiosk[3]));
        bytes32 expected = vm.parseBytes32(string.concat("0x", kiosk[7]));
        assertEq(raw, vm.parseBytes(string.concat("0x", phone[3])));
        assertEq(raw.length, 21 * 32);
        assertEq(hashQuote(raw), expected);
        raw[11 * 32 + 31] = bytes1(uint8(raw[11 * 32 + 31]) ^ 1);
        assertNotEq(hashQuote(raw), expected);
    }

    function hashQuote(bytes memory raw) private pure returns (bytes32) {
        bytes32 chain;
        bytes32 contract_;
        bytes memory fields = new bytes(raw.length - 64);
        assembly ("memory-safe") {
            chain := mload(add(raw, 32))
            contract_ := mload(add(raw, 64))
            mcopy(add(fields, 32), add(raw, 96), mload(fields))
        }
        bytes32 domain = keccak256(
            abi.encode(DOMAIN_TYPEHASH, keccak256("NU54 Payment Settlement"), keccak256("2"), chain, contract_)
        );
        return keccak256(abi.encodePacked(hex"1901", domain, keccak256(abi.encodePacked(QUOTE_TYPEHASH, fields))));
    }
}
