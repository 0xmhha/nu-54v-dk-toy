#include "nu54_solana.h"

#include <string.h>

#define HARDENED UINT32_C(0x80000000)

static const uint8_t system_program[32];
static const uint8_t token_program[32] = {
	0x06, 0xdd, 0xf6, 0xe1, 0xd7, 0x65, 0xa1, 0x93, 0xd9, 0xcb, 0xe1, 0x46, 0xce, 0xeb, 0x79, 0xac,
	0x1c, 0xb4, 0x85, 0xed, 0x5f, 0x5b, 0x37, 0x91, 0x3a, 0x8c, 0xf5, 0x85, 0x7e, 0xff, 0x00, 0xa9,
};

typedef struct { const uint8_t *data; size_t size; } cursor;

static const uint8_t *take(cursor *input, size_t size)
{
	if (size > input->size) return NULL;
	const uint8_t *p = input->data;
	input->data += size;
	input->size -= size;
	return p;
}

static int byte(cursor *input)
{
	const uint8_t *p = take(input, 1);
	return p ? *p : -1;
}

static uint64_t le64(const uint8_t *data)
{
	uint64_t result = 0;
	for (size_t i = 0; i < 8; i++) result |= (uint64_t)data[i] << (i * 8);
	return result;
}

static bool keys_unique(const uint8_t *keys, int count)
{
	for (int i = 1; i < count; i++) {
		for (int j = 0; j < i; j++) {
			if (!memcmp(keys + 32 * i, keys + 32 * j, 32)) return false;
		}
	}
	return true;
}

int nu54_sol_review(const uint8_t *wire, size_t length, const nu54_sol_policy *policy,
		    nu54_sol_intent *intent)
{
	if (!intent) return -1;
	memset(intent, 0, sizeof(*intent));
	if (!wire || !length || length > NU54_SOL_MESSAGE_MAX || !policy || !policy->sha256 ||
	    !policy->fee || policy->fee > policy->max_fee ||
	    policy->block_height > policy->last_valid_block_height) return -1;

	cursor input = {wire, length};
	nu54_sol_intent review = {0};
	if (wire[0] == 0x80) {
		review.static_v0 = true;
		(void)byte(&input);
	}
	/* Only a one-signer writable fee payer and one supported transfer. All
	 * compact-u16 counts are below 128 and must use one byte. */
	if (byte(&input) != 1 || byte(&input) != 0) return -1;
	int readonly = byte(&input), count = byte(&input);
	if ((count != 3 && count != 5) || readonly != (count == 3 ? 1 : 2)) return -1;
	const uint8_t *keys = take(&input, (size_t)count * 32);
	const uint8_t *blockhash = take(&input, 32);
	if (!keys || !blockhash || memcmp(keys, policy->payer, 32) ||
	    !memcmp(blockhash, system_program, 32) || memcmp(blockhash, policy->blockhash, 32) ||
	    !keys_unique(keys, count) || byte(&input) != 1) return -1;
	int program = byte(&input), accounts = byte(&input);
	if (program < 0 || program >= count) return -1;
	const uint8_t *program_id = keys + 32 * program;
	if (count == 3) {
		static const uint8_t transfer[] = {2, 0, 0, 0};
		if (program != 2 || accounts != 2 || memcmp(program_id, system_program, 32) ||
		    byte(&input) != 0 || byte(&input) != 1 || byte(&input) != 12) return -1;
		const uint8_t *data = take(&input, 12);
		if (!data || memcmp(data, transfer, sizeof(transfer))) return -1;
		review.amount = le64(data + sizeof(transfer));
		review.decimals = 9;
		memcpy(review.source, keys, 32);
		memcpy(review.recipient, keys + 32, 32);
	} else {
		if (!policy->allow_spl || program < 3 || accounts != 4 ||
		    memcmp(program_id, token_program, 32)) return -1;
		int source = byte(&input), mint = byte(&input), dest = byte(&input);
		if ((source != 1 && source != 2) || dest != 3 - source || mint != 7 - program ||
		    byte(&input) != 0 || byte(&input) != 10) return -1;
		const uint8_t *data = take(&input, 10);
		if (!data || data[0] != 12 || data[9] != policy->decimals ||
		    memcmp(keys + 32 * mint, policy->mint, 32)) return -1;
		review.spl = true;
		review.amount = le64(data + 1);
		review.decimals = data[9];
		memcpy(review.source, keys + 32 * source, 32);
		memcpy(review.recipient, keys + 32 * dest, 32);
		memcpy(review.mint, policy->mint, 32);
	}
	/* v0 with no table lookups is fully described by its static keys. */
	if (!review.amount || (review.static_v0 && byte(&input) != 0) || input.size ||
	    policy->sha256(wire, length, review.commitment) ||
	    memcmp(review.commitment, policy->message_hash, 32)) return -1;
	review.fee = policy->fee;
	review.last_valid_block_height = policy->last_valid_block_height;
	memcpy(review.payer, keys, 32);
	memcpy(review.blockhash, blockhash, 32);
	*intent = review;
	return 0;
}

int nu54_sol_finalize(const uint8_t *wire, size_t length, const nu54_sol_policy *policy,
		      const uint8_t approved[32], nu54_sol_sign sign, void *context,
		      uint8_t *output, size_t capacity, size_t *written)
{
	nu54_sol_intent review;
	uint8_t signature[64];
	if (!written) return -1;
	*written = 0;
	if (!output || capacity < 65 || length > capacity - 65 || !approved || !sign ||
	    nu54_sol_review(wire, length, policy, &review) ||
	    memcmp(approved, review.commitment, 32) ||
	    sign(context, review.payer, wire, length, signature)) return -1;
	memmove(output + 65, wire, length);
	output[0] = 1;
	memcpy(output + 1, signature, sizeof(signature));
	*written = length + 65;
	return 0;
}

static void erase(uint8_t *buf, size_t n)
{
	volatile uint8_t *p = buf;
	while (n--) *p++ = 0;
}

int nu54_sol_child(const uint8_t seed[NU54_HD_SEED_SIZE], uint32_t account, nu54_hd_hmac hmac,
		   nu54_sol_ed25519 operation, void *context)
{
	static const uint8_t master[] = "ed25519 seed";
	const uint32_t path[] = {HARDENED | 44, HARDENED | 501, HARDENED | account, HARDENED};
	uint8_t node[64], child[64], message[37] = {0};
	int rc = -1;
	if (!seed || account >= HARDENED || !hmac || !operation ||
	    hmac(master, sizeof(master) - 1, seed, NU54_HD_SEED_SIZE, node)) goto done;
	for (size_t i = 0; i < sizeof(path) / sizeof(path[0]); i++) {
		memcpy(message + 1, node, 32);
		for (size_t j = 0; j < 4; j++) message[33 + j] = (uint8_t)(path[i] >> (24 - j * 8));
		if (hmac(node + 32, 32, message, sizeof(message), child)) goto done;
		memcpy(node, child, sizeof(node));
	}
	rc = operation(node, context);
done:
	erase(node, sizeof(node));
	erase(child, sizeof(child));
	erase(message, sizeof(message));
	return rc;
}
