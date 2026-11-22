#include "nu54_btc48.h"

#include <secp256k1.h>
#include <string.h>

#define HARD UINT32_C(0x80000000)
#define MAX_MONEY UINT64_C(2100000000000000)
#define WITNESS_SIZE 105
#define PATH_SIZE 28

typedef struct { const uint8_t *p; size_t n; } bytes;
typedef struct { bytes key, value; } entry;
typedef struct { entry entries[15]; size_t length; } fields;

static bool take(bytes *in, size_t n, bytes *out)
{
	if (n > in->n) return false;
	*out = (bytes){in->p, n};
	in->p += n;
	in->n -= n;
	return true;
}

static uint64_t le(const uint8_t *raw, size_t n)
{
	uint64_t result = 0;
	for (size_t i = 0; i < n; ++i) result |= (uint64_t)raw[i] << (8 * i);
	return result;
}

static void put(uint8_t *out, uint64_t n, size_t length)
{
	for (size_t i = 0; i < length; ++i) out[i] = (uint8_t)(n >> (i * 8));
}

static bool var(bytes *in, bytes *out)
{
	bytes prefix, length;
	if (!take(in, 1, &prefix)) return false;
	size_t n = *prefix.p;
	if (n < 253) return take(in, n, out);
	if (n != 253 || !take(in, 2, &length) || le(length.p, 2) < 253) return false;
	return take(in, (size_t)le(length.p, 2), out);
}

static bool same(bytes a, bytes b) { return a.n == b.n && (!a.n || !memcmp(a.p, b.p, a.n)); }

static bool map(bytes *in, fields *out)
{
	out->length = 0;
	bytes key, value;
	while (var(in, &key)) {
		if (!key.n) return true;
		if (out->length == sizeof(out->entries) / sizeof(out->entries[0]) || !var(in, &value))
			return false;
		for (size_t i = 0; i < out->length; ++i) {
			if (same(key, out->entries[i].key)) return false;
		}
		out->entries[out->length++] = (entry){key, value};
	}
	return false;
}

static bytes field(const fields *m, uint8_t type)
{
	for (size_t i = 0; i < m->length; ++i) {
		bytes key = m->entries[i].key;
		if (key.n == 1 && key.p[0] == type) return m->entries[i].value;
	}
	return (bytes){0};
}

int nu54_btc48_scope_valid(const nu54_btc48_scope *scope)
{
	if (!scope || (scope->network != 1 && scope->network != 2) || scope->account > 99 || scope->owner >= 3 ||
	    !scope->max_fee || scope->max_fee > UINT64_C(1000000)) return -1;
	for (size_t i = 0; i < 3; ++i) {
		const nu54_btc48_xpub *root = &scope->roots[i];
		secp256k1_pubkey point;
		if ((root->public_key[0] != 2 && root->public_key[0] != 3) ||
		    !secp256k1_ec_pubkey_parse(secp256k1_context_static, &point, root->public_key, 33)) return -1;
		for (size_t j = 0; j < i; ++j) {
			if (!memcmp(root->public_key, scope->roots[j].public_key, 33) ||
			    !memcmp(root->fingerprint, scope->roots[j].fingerprint, 4)) return -1;
		}
	}
	return 0;
}

static int child(const nu54_btc48_xpub *root, uint32_t branch, uint32_t index, nu54_hd_hmac hmac, uint8_t out[33])
{
	const secp256k1_context *context = secp256k1_context_static;
	secp256k1_pubkey point;
	uint8_t data[37], code[32], result[64];
	const uint32_t paths[] = {branch, index};
	memcpy(data, root->public_key, 33);
	memcpy(code, root->chain_code, 32);
	if (!secp256k1_ec_pubkey_parse(context, &point, data, 33)) return -1;
	for (size_t i = 0; i < 2; ++i) {
		for (size_t b = 0; b < 4; ++b) data[33 + b] = (uint8_t)(paths[i] >> (24 - 8 * b));
		size_t n = 33;
		if (hmac(code, sizeof(code), data, sizeof(data), result) ||
		    !secp256k1_ec_pubkey_tweak_add(context, &point, result) ||
		    !secp256k1_ec_pubkey_serialize(context, data, &n, &point, SECP256K1_EC_COMPRESSED)) return -1;
		memcpy(code, result + 32, 32);
	}
	memcpy(out, data, 33);
	return 0;
}

static int derive(const nu54_btc48_scope *scope, uint32_t branch, uint32_t index, nu54_hd_hmac hmac,
		  uint8_t keys[3][33], uint8_t script[WITNESS_SIZE])
{
	if (!hmac || branch > 1 || index > 9999 || nu54_btc48_scope_valid(scope)) return -1;
	size_t sorted[] = {0, 1, 2};
	for (size_t i = 0; i < 3; ++i) {
		if (child(&scope->roots[i], branch, index, hmac, keys[i])) return -1;
	}
	for (size_t i = 0; i < 3; ++i) {
		for (size_t j = i + 1; j < 3; ++j) {
			int order = memcmp(keys[sorted[i]], keys[sorted[j]], 33);
			if (!order) return -1;
			if (order > 0) {
				size_t swap = sorted[i];
				sorted[i] = sorted[j];
				sorted[j] = swap;
			}
		}
	}
	script[0] = 0x52;
	for (size_t i = 0; i < 3; ++i) {
		script[1 + i * 34] = 33;
		memcpy(script + 2 + i * 34, keys[sorted[i]], 33);
	}
	script[103] = 0x53;
	script[104] = 0xae;
	return 0;
}

int nu54_btc48_script(const nu54_btc48_scope *scope, uint32_t branch, uint32_t index, nu54_hd_hmac hmac,
		       uint8_t script[105])
{
	uint8_t keys[3][33];
	return script ? derive(scope, branch, index, hmac, keys, script) : -1;
}

static bool parse_path(bytes path, const nu54_btc48_scope *scope, uint32_t *branch, uint32_t *index)
{
	if (path.n != PATH_SIZE || le(path.p + 4, 4) != (HARD | 48) ||
	    le(path.p + 8, 4) != (HARD | 1U) || le(path.p + 12, 4) != (HARD | scope->account) ||
	    le(path.p + 16, 4) != (HARD | 2)) return false;
	*branch = (uint32_t)le(path.p + 20, 4);
	*index = (uint32_t)le(path.p + 24, 4);
	return *branch <= 1 && *index <= 9999;
}

static bool descriptor(const fields *m, uint8_t type, bytes witness, bytes program, const nu54_btc48_scope *scope,
		       nu54_hd_hmac hmac, nu54_btc48_hash hash, uint32_t *branch, uint32_t *index,
		       uint8_t keys[3][33])
{
	uint8_t matched = 0, expected[WITNESS_SIZE], digest[32];
	for (size_t i = 0; i < m->length; ++i) {
		entry item = m->entries[i];
		if (item.key.p[0] != type) continue;
		uint32_t b, n;
		if (item.key.n != 34 || !parse_path(item.value, scope, &b, &n)) return false;
		if (!matched) {
			*branch = b;
			*index = n;
			if (derive(scope, b, n, hmac, keys, expected)) return false;
		}
		if (*branch != b || *index != n) return false;
		size_t slot = 0;
		while (slot < 3 && memcmp(item.value.p, scope->roots[slot].fingerprint, 4)) ++slot;
		if (slot == 3 || (matched & (1u << slot)) || memcmp(item.key.p + 1, keys[slot], 33)) return false;
		matched |= (uint8_t)(1u << slot);
	}
	return matched == 7 && same(witness, (bytes){expected, sizeof(expected)}) &&
	       program.n == 34 && program.p[0] == 0 && program.p[1] == 32 &&
	       !hash(expected, sizeof(expected), digest) && !memcmp(program.p + 2, digest, 32);
}

static bool txout(bytes *raw, uint64_t *value, bytes *script)
{
	bytes satoshis;
	if (!take(raw, 8, &satoshis) || !var(raw, script)) return false;
	*value = le(satoshis.p, 8);
	return *value <= MAX_MONEY;
}

static bool parent_output(bytes prev, uint32_t selected, uint64_t *amount, bytes *script)
{
	bytes field;
	if (!take(&prev, 4, &field) || !take(&prev, 1, &field) || !*field.p || *field.p > 16) return false;
	size_t count = *field.p;
	for (size_t i = 0; i < count; ++i) {
		if (!take(&prev, 36, &field) || !var(&prev, &field) || !take(&prev, 4, &field)) return false;
	}
	if (!take(&prev, 1, &field) || !*field.p || *field.p > 16 || selected >= *field.p) return false;
	count = *field.p;
	for (size_t i = 0; i < count; ++i) {
		uint64_t value;
		if (!txout(&prev, &value, &field)) return false;
		if (i == selected) {
			*amount = value;
			*script = field;
		}
	}
	return prev.n == 4;
}

static bool double_hash(nu54_btc48_hash hash, bytes raw, uint8_t out[32])
{
	uint8_t first[32];
	return !hash(raw.p, raw.n, first) && !hash(first, sizeof(first), out);
}

static bool allowed(const fields *m, bytes simple, uint8_t derivation, uint8_t partial)
{
	for (size_t i = 0; i < m->length; ++i) {
		bytes key = m->entries[i].key;
		if (key.n == 1 && memchr(simple.p, key.p[0], simple.n)) continue;
		if (key.n == 34 && ((derivation != 0xff && key.p[0] == derivation) ||
		    (partial != 0xff && key.p[0] == partial))) continue;
		return false;
	}
	return true;
}

static bool verify(const uint8_t pubkey[33], bytes sig, const uint8_t digest[32])
{
	const secp256k1_context *secp = secp256k1_context_static;
	secp256k1_pubkey point;
	secp256k1_ecdsa_signature parsed;
	uint8_t canonical[72];
	size_t length = sizeof(canonical);
	return sig.n >= 9 && sig.n <= 73 && sig.p[sig.n - 1] == 1 &&
	       secp256k1_ec_pubkey_parse(secp, &point, pubkey, 33) &&
	       secp256k1_ecdsa_signature_parse_der(secp, &parsed, sig.p, sig.n - 1) &&
	       secp256k1_ecdsa_signature_serialize_der(secp, canonical, &length, &parsed) &&
	       same((bytes){sig.p, sig.n - 1}, (bytes){canonical, length}) &&
	       secp256k1_ecdsa_verify(secp, &parsed, digest, &point);
}

typedef struct {
	bytes witness, partial, partial_key;
	uint64_t value;
	int cosigner;
} input_evidence;

static bool review_input(const fields *map_fields, const nu54_btc48_scope *scope, nu54_hd_hmac hmac,
			 nu54_btc48_hash hash, uint8_t outpoint[36], uint8_t sequence_bytes[4],
			 nu54_btc48_input *input, input_evidence *evidence)
{
	static const uint8_t kinds[] = {0, 1, 3, 5, 0x0e, 0x0f, 0x10};
	if (!allowed(map_fields, (bytes){kinds, sizeof(kinds)}, 6, 2)) return false;
	bytes parent = field(map_fields, 0), prevout = field(map_fields, 1), sighash = field(map_fields, 3);
	bytes txid = field(map_fields, 0x0e), vout = field(map_fields, 0x0f), sequence = field(map_fields, 0x10);
	bytes program;
	evidence->witness = field(map_fields, 5);
	evidence->cosigner = -1;
	uint8_t parent_hash[32], keys[3][33];
	if (txid.n != 32 || vout.n != 4 || sequence.n != 4 || le(sequence.p, 4) < UINT32_C(0xfffffffd) ||
	    (sighash.p && (sighash.n != 4 || le(sighash.p, 4) != 1)) || !parent.p ||
	    !double_hash(hash, parent, parent_hash) || memcmp(txid.p, parent_hash, 32) ||
	    !parent_output(parent, (uint32_t)le(vout.p, 4), &evidence->value, &program) ||
	    !descriptor(map_fields, 6, evidence->witness, program, scope, hmac, hash,
			&input->branch, &input->index, keys)) return false;
	memcpy(outpoint, txid.p, 32);
	memcpy(outpoint + 32, vout.p, 4);
	memcpy(sequence_bytes, sequence.p, 4);
	memcpy(input->owned_key, keys[scope->owner], 33);
	if (prevout.p) {
		uint64_t claimed;
		bytes script;
		if (!txout(&prevout, &claimed, &script) || prevout.n || claimed != evidence->value || !same(script, program))
			return false;
	}
	for (size_t i = 0; i < map_fields->length; ++i) {
		entry partial = map_fields->entries[i];
		if (partial.key.p[0] != 2) continue;
		if (evidence->partial.p || partial.key.n != 34) return false;
		for (size_t slot = 0; slot < NU54_BTC48_SLOTS; ++slot) {
			if (slot != scope->owner && !memcmp(keys[slot], partial.key.p + 1, 33)) evidence->cosigner = (int)slot;
		}
		if (evidence->cosigner < 0) return false;
		evidence->partial = partial.value;
		evidence->partial_key = (bytes){partial.key.p + 1, 33};
	}
	return true;
}

int nu54_btc48_review(const uint8_t *raw, size_t size, const nu54_btc48_scope *scope, nu54_hd_hmac hmac,
		       nu54_btc48_hash hash, nu54_btc48_intent *out)
{
	if (!out) return -1;
	memset(out, 0, sizeof(*out));
	if (!raw || !hash || size > NU54_BTC48_MAX || nu54_btc48_scope_valid(scope)) return -1;
	bytes wire = {raw, size}, magic;
	fields m;
	static const uint8_t globals[] = {2, 3, 4, 5, 0xfb}, outs[] = {1, 3, 4};
	if (!take(&wire, 5, &magic) || memcmp(magic.p, "psbt\xff", 5) ||
	    !map(&wire, &m) || !allowed(&m, (bytes){globals, sizeof(globals)}, 0xff, 0xff)) return -1;
	bytes version = field(&m, 2), locktime = field(&m, 3), ninputs = field(&m, 4),
	      noutputs = field(&m, 5), format = field(&m, 0xfb);
	if (version.n != 4 || le(version.p, 4) != 2 || format.n != 4 || le(format.p, 4) != 2 ||
	    (locktime.p && locktime.n != 4) || ninputs.n != 1 || !*ninputs.p || *ninputs.p > NU54_BTC48_INPUTS ||
	    noutputs.n != 1 || !*noutputs.p || *noutputs.p > NU54_BTC48_OUTPUTS) return -1;
	nu54_btc48_intent review = {.input_count = *ninputs.p, .output_count = *noutputs.p};
	input_evidence evidence[NU54_BTC48_INPUTS] = {0};
	uint8_t outpoints[NU54_BTC48_INPUTS][36], sequences[NU54_BTC48_INPUTS][4];
	uint64_t total_input = 0;
	for (size_t i = 0; i < review.input_count; ++i) {
		if (!map(&wire, &m) || !review_input(&m, scope, hmac, hash, outpoints[i], sequences[i],
						    &review.inputs[i], &evidence[i])) return -1;
		if (i && (!memcmp(outpoints[0], outpoints[i], 36) || evidence[i].cosigner != evidence[0].cosigner))
			return -1;
		review.inputs[i].insertion = (size_t)(wire.p - raw) - 1;
		total_input += evidence[i].value;
		review.replaceable |= le(sequences[i], 4) == UINT32_C(0xfffffffd);
	}
	if (total_input > MAX_MONEY) return -1;
	uint8_t serialized[NU54_BTC48_OUTPUTS * 43], *cursor = serialized;
	uint64_t total = 0;
	for (size_t i = 0; i < review.output_count; ++i) {
		if (!map(&wire, &m) || !allowed(&m, (bytes){outs, sizeof(outs)}, 2, 0xff)) return -1;
		bytes amount = field(&m, 3), script = field(&m, 4), change = field(&m, 1);
		if (amount.n != 8 || (script.n != 22 && script.n != 34) ||
		    script.p[0] != 0 || script.p[1] != script.n - 2) return -1;
		uint64_t sat = le(amount.p, 8);
		if (!sat || sat > MAX_MONEY) return -1;
		if (m.length > 2) {
			uint32_t branch, index;
			uint8_t children[3][33];
			if (!descriptor(&m, 2, change, script, scope, hmac, hash, &branch, &index, children) || branch != 1)
				return -1;
			review.outputs[i].change = true;
		}
		memcpy(review.outputs[i].script, script.p, script.n);
		review.outputs[i].length = (uint8_t)script.n;
		review.outputs[i].satoshis = sat;
		total += sat;
		memcpy(cursor, amount.p, 8); cursor += 8;
		*cursor++ = (uint8_t)script.n;
		memcpy(cursor, script.p, script.n); cursor += script.n;
	}
	if (wire.n || total > total_input || total_input - total > scope->max_fee) return -1;
	review.fee = total_input - total;
	review.locktime = locktime.p ? (uint32_t)le(locktime.p, 4) : 0;
	uint8_t signing[262];
	memcpy(signing, version.p, 4);
	if (!double_hash(hash, (bytes){outpoints[0], 36 * review.input_count}, signing + 4) ||
	    !double_hash(hash, (bytes){sequences[0], 4 * review.input_count}, signing + 36) ||
	    !double_hash(hash, (bytes){serialized, (size_t)(cursor - serialized)}, signing + 222)) return -1;
	signing[104] = WITNESS_SIZE;
	put(signing + 254, review.locktime, 4);
	put(signing + 258, 1, 4);
	for (size_t i = 0; i < review.input_count; ++i) {
		memcpy(signing + 68, outpoints[i], 36);
		memcpy(signing + 105, evidence[i].witness.p, WITNESS_SIZE);
		put(signing + 210, evidence[i].value, 8);
		memcpy(signing + 218, sequences[i], 4);
		if (!double_hash(hash, (bytes){signing, sizeof(signing)}, review.inputs[i].digest) ||
		    (evidence[i].partial.p && !verify(evidence[i].partial_key.p, evidence[i].partial, review.inputs[i].digest)))
			return -1;
	}
	review.signatures = evidence[0].partial.p ? 1 : 0;
	if (hash(raw, size, review.commitment)) return -1;
	*out = review;
	return 0;
}

int nu54_btc48_sign_psbt(const uint8_t *raw, size_t size, const nu54_btc48_scope *scope, nu54_hd_hmac hmac,
			  nu54_btc48_hash hash, const uint8_t approved[32], nu54_btc48_sign sign, void *context,
			  uint8_t *out, size_t capacity, size_t *written)
{
	if (!written) return -1;
	*written = 0;
	nu54_btc48_intent review;
	uint8_t compact[64], der[NU54_BTC48_INPUTS][73];
	size_t lengths[NU54_BTC48_INPUTS], added = 0;
	secp256k1_ecdsa_signature signature;
	if (!out || !sign || !approved || capacity < size ||
	    nu54_btc48_review(raw, size, scope, hmac, hash, &review) ||
	    memcmp(approved, review.commitment, 32) ||
	    capacity - size < NU54_BTC48_SIGNATURE_FIELD_MAX * review.input_count) return -1;
	for (size_t i = 0; i < review.input_count; ++i) {
		const nu54_btc48_input *input = &review.inputs[i];
		size_t length = 72;
		if (sign(context, input->branch, input->index, input->owned_key, input->digest, compact) ||
		    !secp256k1_ecdsa_signature_parse_compact(secp256k1_context_static, &signature, compact) ||
		    !secp256k1_ecdsa_signature_serialize_der(secp256k1_context_static, der[i], &length, &signature)) return -1;
		der[i][length++] = 1;
		if (!verify(input->owned_key, (bytes){der[i], length}, input->digest)) return -1;
		lengths[i] = length;
		added += 36 + length;
	}
	memmove(out, raw, size);
	size_t end = size, displacement = added;
	for (size_t i = review.input_count; i-- > 0;) {
		const nu54_btc48_input *input = &review.inputs[i];
		size_t position = input->insertion;
		memmove(out + position + displacement, out + position, end - position);
		displacement -= 36 + lengths[i];
		uint8_t *p = out + position + displacement;
		*p++ = 34;
		*p++ = 2;
		memcpy(p, input->owned_key, 33); p += 33;
		*p++ = (uint8_t)lengths[i];
		memcpy(p, der[i], lengths[i]);
		end = position;
	}
	*written = size + added;
	return 0;
}
