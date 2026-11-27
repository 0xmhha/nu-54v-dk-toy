/* Exercises the production approval gateway with host substitutes for its hardware services. */
#include "device_evm.h"
#include "device_install.h"
#include "nu54_sig.h"
#include "psa/crypto.h"
#include "secp256k1.h"

#include <assert.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static nu54_install_policy installed;
static uint8_t authority_statement[896], authority_signature[64], host_key[32], display_body[1024];
static size_t authority_size, display_size;
static unsigned host_signature_count;

static size_t hex(const char *input, uint8_t *output, size_t capacity)
{
	size_t length = strlen(input);
	assert(length % 2 == 0 && length / 2 <= capacity);
	for (size_t i = 0; i < length / 2; i++) {
		unsigned value;
		assert(sscanf(input + i * 2, "%2x", &value) == 1);
		output[i] = value;
	}
	return length / 2;
}

const nu54_install_policy *device_install_policy(void) { return &installed; }

int device_install_verify_authority(const uint8_t *statement, size_t size, const uint8_t signature[64])
{
	return size == authority_size && memcmp(statement, authority_statement, size) == 0 &&
	       memcmp(signature, authority_signature, 64) == 0 ? 0 : -1;
}

int device_install_verify_display(const uint8_t *statement, size_t size, const uint8_t signature[64])
{
	return size == display_size && memcmp(statement, display_body, size) == 0 && signature[0] == 0x56 ? 0 : -1;
}

int device_hd_sign(const uint8_t address[20], const uint8_t digest[32], uint8_t signature[64])
{
	(void)address;
	secp256k1_context *context = secp256k1_context_create(SECP256K1_CONTEXT_NONE);
	secp256k1_ecdsa_signature signed_digest;
	assert(secp256k1_ecdsa_sign(context, &signed_digest, digest, host_key, NULL, NULL));
	assert(secp256k1_ecdsa_signature_serialize_compact(context, signature, &signed_digest));
	secp256k1_context_destroy(context);
	host_signature_count++;
	return 0;
}

int device_hd_market_sign(const uint8_t address[20], const uint8_t digest[32], uint8_t signature[64])
{
	return device_hd_sign(address,digest,signature);
}

psa_status_t psa_generate_random(uint8_t *output, size_t size)
{
	memset(output, 0x42, size);
	return PSA_SUCCESS;
}

int main(int argc, char **argv)
{
	assert(argc == 9);
	uint8_t wire[896], board[32], address[20], raw[LAB_TX_LENGTH], proof[64] = {0x56};
	size_t wire_size = hex(argv[1], wire, sizeof(wire)), raw_size = 0;
	authority_size = hex(argv[2], authority_statement, sizeof(authority_statement));
	assert(hex(argv[3], authority_signature, sizeof(authority_signature)) == 64);
	assert(hex(argv[4], board, sizeof(board)) == 32);
	assert(hex(argv[5], address, sizeof(address)) == 20);
	assert(hex(argv[6], host_key, sizeof(host_key)) == 32);
	uint64_t now = strtoull(argv[7], NULL, 10), generation = strtoull(argv[8], NULL, 10);
	installed.not_after = now + 180;
	installed.revision = 1;
	bool market_scope = wire[0] == NU54_EVM_MARKET_OP;
	bool revoke_scope = wire[0] == NU54_EVM_REVOKE_OP;
	bool allowance_scope = wire[0] == NU54_EVM_DAPP_ALLOWANCE_OP;
	bool login_scope = wire[0] == NU54_EVM_SIWE_OP;
	assert(market_scope || revoke_scope || allowance_scope || login_scope || wire[0] == NU54_EVM_TRANSFER_OP || wire[0] == NU54_EVM_EXTERNAL_OP);
	memcpy(display_body, market_scope ? "N54Z" : login_scope ? "N54I" : "N54X", 4);
	memset(display_body + 4, 0x42, 32);
	memcpy(display_body + 36, wire, wire_size);
	display_size = 36 + wire_size;
	nu54_v2_link link = {.profile = NU54_V2_MANAGEMENT, .local = NU54_V2_BOARD, .peer = NU54_V2_OWNER,
			    .epoch = {0xa0}, .pending_id = {0xb0}};
	nu54_v2_request request = {.payload = wire, .payload_length = wire_size, .expires = now + 120, .id = {0xb0}};
	device_evm state = {0};
	int (*stage)(device_evm *, const nu54_v2_request *, const nu54_v2_link *, uint64_t, uint64_t, uint32_t,
		     const uint8_t [32], const uint8_t [20]) = market_scope ? device_evm_stage_market :
		     login_scope ? device_evm_stage_login : device_evm_stage;
	for (size_t field = 165; field <= 261 && field < wire_size; field += 32) {
		wire[field] ^= 1;
		assert(stage(&state, &request, &link, now, generation, 7, board, address));
		wire[field] ^= 1;
	}
	if (market_scope) {
		for (size_t field = 230; field <= 670; field += 100) {
			wire[field] ^= 1;
			authority_statement[64+field] ^= 1;
			assert(stage(&state,&request,&link,now,generation,7,board,address));
			authority_statement[64+field] ^= 1;
			wire[field] ^= 1;
		}
	}
	assert(stage(&state, &request, &link, now + 120, generation, 7, board, address));
	assert(!stage(&state, &request, &link, now, generation, 7, board, address));
	if (revoke_scope) {
		assert(state.policy.revoke_allowance);
		assert(!memcmp(state.intent.token, wire + 33, 20));
		assert(memcmp(state.intent.token, (uint8_t[20]){0}, 20));
		assert(memcmp(state.intent.recipient, (uint8_t[20]){0}, 20));
		assert(!memcmp(state.intent.amount, (uint8_t[32]){0}, 32));
	}
	if (allowance_scope) {
		assert(!state.policy.revoke_allowance && !state.market && !state.login);
		assert(!memcmp(state.intent.token, wire + 33, 20));
		assert(!memcmp(state.intent.recipient, wire + 166, 20));
		assert(memcmp(state.intent.amount, (uint8_t[32]){0}, 32));
		assert(memcmp(state.intent.amount, wire + 186, 32) <= 0);
	}
	assert(stage(&state, &request, &link, now, generation, 8, board, address));
	assert(device_evm_complete(&state, &link, now, generation, 7, address, raw, &raw_size));
	assert(host_signature_count == 0);
	uint8_t invalid_proof[64] = {0};
	assert(device_evm_display(&state, now, invalid_proof));
	assert(!device_evm_display(&state, now, proof));
	assert(device_evm_complete(&state, &link, now, generation + 1, 7, address, raw, &raw_size));
	assert(device_evm_complete(&state, &link, now, generation, 8, address, raw, &raw_size));
	link.epoch[0]++;
	assert(device_evm_complete(&state, &link, now, generation, 7, address, raw, &raw_size));
	link.epoch[0]--;
	assert(!device_evm_complete(&state, &link, now, generation, 7, address, raw, &raw_size));
	if (login_scope) assert(raw_size == 65);
	assert(device_evm_complete(&state, &link, now, generation, 7, address, raw, &raw_size));
	assert(host_signature_count == 1);
	for (size_t i = 0; i < raw_size; i++) printf("%02x", raw[i]);
	puts("");
	return 0;
}
