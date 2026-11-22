#include "nu54_evm_tx.h"
#include "nu54_keccak.h"

#include <stdbool.h>
#include <string.h>

struct item { const uint8_t *bytes; size_t length; bool list; };

static int rlp(const uint8_t **cursor, size_t *remaining, struct item *out)
{
	if (!*remaining) return -1;
	const uint8_t *bytes = *cursor;
	uint8_t tag = bytes[0];
	size_t prefix = 1, length;
	bool list = tag >= 0xc0;
	if (tag < 0x80) {
		prefix = 0;
		length = 1;
	} else if (tag <= 0xb7) {
		length = tag - 0x80;
	} else if (tag >= 0xc0 && tag <= 0xf7) {
		length = tag - 0xc0;
	} else {
		size_t length_size = tag - (list ? 0xf7u : 0xb7u);
		if (length_size != 1 || *remaining < 2 || bytes[1] < 56) return -1;
		prefix = 2;
		length = bytes[1];
	}
	if (length > *remaining - prefix || (!list && prefix == 1 && length == 1 && bytes[1] < 0x80)) return -1;
	*out = (struct item){bytes + prefix, length, list};
	*cursor += prefix + length;
	*remaining -= prefix + length;
	return 0;
}

static int fields(const uint8_t *raw, size_t length, struct item *list, struct item parts[9])
{
	if (!raw || length < 2 || length > NU54_EVM_TX_MAX || *raw++ != 2) return -1;
	--length;
	if (rlp(&raw, &length, list) || !list->list || length) return -1;
	const uint8_t *bytes = list->bytes;
	size_t left = list->length;
	for (size_t i = 0; i < 9; ++i)
		if (rlp(&bytes, &left, &parts[i]) || parts[i].list != (i == 8)) return -1;
	return left || parts[8].length ? -1 : 0;
}

static int number(const struct item *item, uint64_t *out)
{
	if (item->length > 8 || (item->length && !item->bytes[0])) return -1;
	*out = 0;
	for (size_t i = 0; i < item->length; ++i) *out = (*out << 8) | item->bytes[i];
	return 0;
}

static bool empty(const uint8_t *raw, size_t length)
{
	uint8_t mask = 0;
	for (size_t i = 0; i < length; ++i) mask |= raw[i];
	return mask == 0;
}

int nu54_evm_decode(const uint8_t *raw, size_t length, const nu54_evm_policy *policy, nu54_evm_intent *intent)
{
	if (!policy || !intent) return -1;
	bool allowance = !empty(policy->allowance_spender, sizeof(policy->allowance_spender));
	static const uint8_t unlimited[32] = {
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
	};
	if (allowance != !empty(policy->allowance_maximum, sizeof(policy->allowance_maximum)) ||
	    (allowance && (policy->revoke_allowance || !memcmp(policy->allowance_maximum, unlimited, sizeof(unlimited)))))
		return -1;
	struct item list, p[9];
	nu54_evm_intent next = {0};
	if (fields(raw, length, &list, p) || number(&p[0], &next.chain) || number(&p[1], &next.nonce) ||
	    number(&p[2], &next.priority_fee) || number(&p[3], &next.max_fee) || number(&p[4], &next.gas_limit) ||
	    next.chain != policy->chain || (next.chain != 11155111 && next.chain != 84532 && next.chain != 8283) ||
	    next.max_fee < next.priority_fee || !next.max_fee || next.gas_limit < 21000 ||
	    p[5].length != 20 || empty(p[5].bytes, 20) || p[6].length > 32 ||
	    (p[6].length && !p[6].bytes[0]) || (!!policy->l1_fee_bound != (next.chain == 84532)) ||
	    policy->l1_fee_bound > policy->total_fee_limit ||
	    next.gas_limit > (policy->total_fee_limit - policy->l1_fee_bound) / next.max_fee) return -1;
	next.l1_fee_bound = policy->l1_fee_bound;
	next.total_fee_bound = next.gas_limit * next.max_fee + next.l1_fee_bound;
	if (!p[7].length) {
		if (allowance || policy->revoke_allowance || !p[6].length || next.gas_limit != 21000 || !empty(policy->token, 20)) return -1;
		memcpy(next.recipient, p[5].bytes, 20);
		memcpy(next.amount + 32 - p[6].length, p[6].bytes, p[6].length);
	} else {
		static const uint8_t transfer[] = {0xa9, 0x05, 0x9c, 0xbb};
		static const uint8_t approve[] = {0x09, 0x5e, 0xa7, 0xb3};
		const uint8_t *data = p[7].bytes;
		if (p[6].length || p[7].length != 68 || memcmp(data, (allowance || policy->revoke_allowance) ? approve : transfer, sizeof(transfer)) ||
		    memcmp(policy->token, p[5].bytes, 20) || !empty(data + 4, 12) ||
		    empty(data + 16, 20) || empty(data + 36, 32) != policy->revoke_allowance || next.gas_limit > 150000) return -1;
		if (allowance && (memcmp(policy->allowance_spender, data + 16, 20) ||
				  memcmp(data + 36, policy->allowance_maximum, 32) > 0)) return -1;
		memcpy(next.token, p[5].bytes, 20);
		memcpy(next.recipient, data + 16, 20);
		memcpy(next.amount, data + 36, 32);
	}
	nu54_keccak256(raw, length, next.digest);
	*intent = next;
	return 0;
}

static size_t append_integer(uint8_t *out, const uint8_t value[32])
{
	size_t offset = 0;
	while (offset < 32 && !value[offset]) ++offset;
	size_t length = 32 - offset;
	if (length == 1 && value[offset] < 0x80) {
		out[0] = value[offset];
		return 1;
	}
	out[0] = (uint8_t)(0x80 + length);
	memcpy(out + 1, value + offset, length);
	return length + 1;
}

int nu54_evm_serialize(const uint8_t *raw, size_t length, const nu54_evm_policy *policy,
			const uint8_t signature[65], uint8_t out[NU54_EVM_SIGNED_MAX], size_t *size)
{
	struct item list, p[9];
	nu54_evm_intent intent;
	static const uint8_t half_order[32] = {
		0x7f,0xff,0xff,0xff,0xff,0xff,0xff,0xff,0xff,0xff,0xff,0xff,0xff,0xff,0xff,0xff,
		0x5d,0x57,0x6e,0x73,0x57,0xa4,0x50,0x1d,0xdf,0xe9,0x2f,0x46,0x68,0x1b,0x20,0xa0,
	};
	if (!signature || !out || !size || (signature[64] != 27 && signature[64] != 28) ||
	    empty(signature, 32) || empty(signature + 32, 32) || memcmp(signature + 32, half_order, 32) > 0 ||
	    nu54_evm_decode(raw, length, policy, &intent) || fields(raw, length, &list, p)) return -1;
	uint8_t tail[67] = {signature[64] == 28 ? 1 : 0x80};
	size_t tail_length = 1 + append_integer(tail + 1, signature);
	tail_length += append_integer(tail + tail_length, signature + 32);
	size_t payload = list.length + tail_length, at = 0;
	if (payload > UINT8_MAX) return -1;
	out[at++] = 2;
	if (payload <= 55) out[at++] = (uint8_t)(0xc0 + payload);
	else {
		out[at++] = 0xf8;
		out[at++] = (uint8_t)payload;
	}
	memcpy(out + at, list.bytes, list.length);
	memcpy(out + at + list.length, tail, tail_length);
	*size = at + payload;
	return 0;
}
