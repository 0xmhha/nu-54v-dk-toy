#include "nu54_evm_tx.h"
#include "nu54_siwe.h"

#include <assert.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static size_t hex(const char *input, uint8_t *out, size_t capacity)
{
	size_t length = strlen(input);
	assert(!(length % 2) && length / 2 <= capacity);
	for (size_t i = 0; i < length / 2; i++) {
		unsigned byte;
		assert(sscanf(input + 2 * i, "%2x", &byte) == 1);
		out[i] = (uint8_t)byte;
	}
	return length / 2;
}

static void bounded_allowance(void)
{
	uint8_t raw[NU54_EVM_TX_MAX] = {0};
	size_t size = hex("02f86783aa36a703030c82a02894121212121212121212121212121212121212121280b844"
			  "095ea7b30000000000000000000000002323232323232323232323232323232323232323"
			  "0000000000000000000000000000000000000000000000000000000000000005c0", raw, sizeof(raw));
	nu54_evm_intent decoded;
	nu54_evm_policy policy = {.chain = 11155111, .total_fee_limit = 600000};
	memset(policy.token, 0x12, sizeof(policy.token));
	assert(nu54_evm_decode(raw, size, &policy, &decoded));
	/* Approval is a separate operation with an exact spender and enrolled cap. */
	memset(policy.allowance_spender, 0x23, sizeof(policy.allowance_spender));
	policy.allowance_maximum[31] = 5;
	assert(!nu54_evm_decode(raw, size, &policy, &decoded));
	assert(decoded.amount[31] == 5 && decoded.recipient[0] == 0x23);
	policy.allowance_maximum[31] = 4;
	assert(nu54_evm_decode(raw, size, &policy, &decoded));
	policy.allowance_maximum[31] = 5;
	policy.allowance_spender[0] ^= 1;
	assert(nu54_evm_decode(raw, size, &policy, &decoded));
	policy.allowance_spender[0] ^= 1;
	memset(raw + size - 33, 0xff, 32);
	assert(nu54_evm_decode(raw, size, &policy, &decoded));
	memset(raw + size - 33, 0, 32);
	assert(nu54_evm_decode(raw, size, &policy, &decoded));
	raw[size - 2] = 5;
	memset(policy.allowance_maximum, 0xff, sizeof(policy.allowance_maximum));
	assert(nu54_evm_decode(raw, size, &policy, &decoded));
	memset(policy.allowance_maximum, 0, sizeof(policy.allowance_maximum));
	assert(nu54_evm_decode(raw, size, &policy, &decoded));
}

int main(int argc, char **argv)
{
	bounded_allowance();
	if (argc == 3 && !strcmp(argv[1], "siwe")) {
		uint8_t digest[32], expected[32], payer[20];
		memset(payer, 0x11, sizeof(payer));
		assert(hex(argv[2], expected, sizeof(expected)) == 32);
		const char message[] = "demo.example wants you to sign in with your Ethereum account:\n"
			"0x1111111111111111111111111111111111111111\n\n\nURI: https://demo.example/sign-in\n"
			"Version: 1\nChain ID: 11155111\nNonce: 8abc0023\n"
			"Issued At: 1970-01-01T00:16:40.000Z\nExpiration Time: 1970-01-01T00:17:10.000Z";
		assert(!nu54_siwe_digest((const uint8_t *)message, sizeof(message)-1, (const uint8_t *)"demo.example", 12, payer, 11155111, 1001, 1030, digest));
		assert(!memcmp(digest, expected, sizeof(digest)));
		assert(nu54_siwe_digest((const uint8_t *)message, sizeof(message)-1, (const uint8_t *)"evil.example", 12, payer, 11155111, 1001, 1030, digest));
		assert(nu54_siwe_digest((const uint8_t *)message, sizeof(message)-1, (const uint8_t *)"demo.example", 12, payer, 8283, 1001, 1030, digest));
		assert(nu54_siwe_digest((const uint8_t *)message, sizeof(message)-1, (const uint8_t *)"demo.example", 12, payer, 11155111, 1030, 1030, digest));
		return 0;
	}
	if (argc == 7 && !strcmp(argv[1], "unsupported")) {
		uint8_t raw[NU54_EVM_TX_MAX], signature[65] = {0}, serialized[NU54_EVM_SIGNED_MAX];
		nu54_evm_policy policy = {.chain = strtoull(argv[2], NULL, 10),
			.total_fee_limit = strtoull(argv[3], NULL, 10), .l1_fee_bound = strtoull(argv[4], NULL, 10)};
		assert(hex(argv[5], policy.token, sizeof(policy.token)) == 20);
		size_t size = hex(argv[6], raw, sizeof(raw)), serialized_size = 0;
		nu54_evm_intent decoded;
		assert(nu54_evm_decode(raw, size, &policy, &decoded));
		assert(nu54_evm_serialize(raw, size, &policy, signature, serialized, &serialized_size));
		return 0;
	}
	bool allowance = argc == 12 && !strcmp(argv[9], "allowance");
	assert(argc == 9 || (argc == 10 && !strcmp(argv[9], "revoke")) || allowance);
	nu54_evm_policy policy = {.chain = strtoull(argv[1], NULL, 10), .total_fee_limit = strtoull(argv[2], NULL, 10),
				   .revoke_allowance = argc == 10, .l1_fee_bound = strtoull(argv[3], NULL, 10)};
	assert(hex(argv[4], policy.token, sizeof(policy.token)) == 20);
	if (allowance) {
		assert(hex(argv[10], policy.allowance_spender, sizeof(policy.allowance_spender)) == 20);
		policy.allowance_maximum[31] = 9;
	}
	uint8_t raw[NU54_EVM_TX_MAX], expected_digest[32], signature[65], expected[NU54_EVM_SIGNED_MAX], actual[NU54_EVM_SIGNED_MAX];
	size_t size = hex(argv[5], raw, sizeof(raw)), actual_length;
	assert(hex(argv[6], expected_digest, sizeof(expected_digest)) == 32);
	assert(hex(argv[7], signature, sizeof(signature)) == 65);
	size_t expected_length = hex(argv[8], expected, sizeof(expected));
	nu54_evm_intent decoded;
	assert(!nu54_evm_decode(raw, size, &policy, &decoded));
	assert(decoded.chain == policy.chain && decoded.nonce == (allowance ? strtoull(argv[11], NULL, 10) : 3) &&
	       decoded.amount[31] == (policy.revoke_allowance ? 0 : 9));
	assert(!memcmp(decoded.digest, expected_digest, sizeof(expected_digest)));
	assert(!nu54_evm_serialize(raw, size, &policy, signature, actual, &actual_length));
	assert(actual_length == expected_length && !memcmp(actual, expected, actual_length));
	nu54_evm_policy wrong = policy;
	wrong.revoke_allowance = !policy.revoke_allowance;
	assert(nu54_evm_decode(raw, size, &wrong, &decoded));
	wrong = policy;
	wrong.chain = 1;
	assert(nu54_evm_decode(raw, size, &wrong, &decoded));
	wrong = policy;
	wrong.total_fee_limit = 20;
	assert(nu54_evm_decode(raw, size, &wrong, &decoded));
	if (policy.chain == 84532) {
		wrong = policy;
		wrong.l1_fee_bound = 0;
		assert(nu54_evm_decode(raw, size, &wrong, &decoded));
	}
	raw[size - 1] = 0xc1;
	assert(nu54_evm_decode(raw, size, &policy, &decoded));
	return 0;
}
