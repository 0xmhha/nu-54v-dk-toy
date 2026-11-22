#include "nu54_btc.h"

#include <string.h>

#define MAX_MONEY UINT64_C(2100000000000000)
#define HARDENED UINT32_C(0x80000000)

typedef struct { const uint8_t *p; size_t n; } span;
typedef struct { span outpoint, sequence; } tx_input;
typedef struct { span wire, script; uint64_t amount; } tx_output;
typedef struct {
	span version, locktime;
	uint32_t ni, no;
	tx_input in[NU54_BTC_INPUT_MAX];
	tx_output out[NU54_BTC_OUTPUT_MAX];
} transaction;

static bool take(span *source, size_t n, span *out)
{
	if (n > source->n) return false;
	*out = (span){source->p, n};
	source->p += n;
	source->n -= n;
	return true;
}

static uint64_t little(span field)
{
	uint64_t value = 0;
	for (size_t i = 0; i < field.n; i++) value |= (uint64_t)field.p[i] << (i * 8);
	return value;
}

static uint32_t u32(const uint8_t *p) { return (uint32_t)little((span){p, 4}); }

static bool compact(span *source, uint32_t *size)
{
	span first, tail;
	if (!take(source, 1, &first)) return false;
	*size = first.p[0];
	if (*size < 253) return true;
	/* All supported structures fit within uint16_t. */
	if (*size != 253 || !take(source, 2, &tail)) return false;
	*size = (uint32_t)little(tail);
	return *size >= 253;
}

static bool varbytes(span *source, span *out)
{
	uint32_t length;
	return compact(source, &length) && take(source, length, out);
}

static bool output(span *source, tx_output *out)
{
	span amount;
	const uint8_t *start = source->p;
	if (!take(source, 8, &amount) || !varbytes(source, &out->script)) return false;
	out->amount = little(amount);
	out->wire = (span){start, (size_t)(source->p - start)};
	return out->amount <= MAX_MONEY;
}

/* Legacy encoding is the txid preimage, even for SegWit parents. Require the
 * complete stripped previous transaction so its outpoint commits to every
 * amount reviewed. The spending transaction must have empty scriptSigs. */
static bool parse_tx(span source, bool unsigned_tx, transaction *tx)
{
	if (!take(&source, 4, &tx->version) ||
	    !compact(&source, &tx->ni) || tx->ni == 0 || tx->ni > NU54_BTC_INPUT_MAX) return false;
	for (uint32_t i = 0; i < tx->ni; i++) {
		span script;
		if (!take(&source, 36, &tx->in[i].outpoint) || !varbytes(&source, &script) ||
		    (unsigned_tx && script.n != 0) || !take(&source, 4, &tx->in[i].sequence)) return false;
	}
	if (!compact(&source, &tx->no) || tx->no == 0 || tx->no > NU54_BTC_OUTPUT_MAX) return false;
	for (uint32_t i = 0; i < tx->no; i++) {
		if (!output(&source, &tx->out[i])) return false;
	}
	return take(&source, 4, &tx->locktime) && source.n == 0;
}

typedef struct { span values[8], pubkey; } map;

static bool read_map(span *source, unsigned mask, uint8_t derivation_key, map *fields)
{
	unsigned seen = 0;
	span key, value;
	memset(fields, 0, sizeof(*fields));
	while (varbytes(source, &key)) {
		if (!key.n) return true;
		uint8_t type = key.p[0];
		if (type >= 8 || !(mask & (1u << type)) || (seen & (1u << type)) ||
		    key.n != (type == derivation_key ? 34u : 1u) || !varbytes(source, &value)) return false;
		seen |= 1u << type;
		fields->values[type] = value;
		if (type == derivation_key) fields->pubkey = (span){key.p + 1, 33};
	}
	return false;
}

static bool owned(span script, span pubkey, span path, const nu54_btc_policy *policy,
		  nu54_btc_input *key)
{
	uint8_t hash[20];
	if (script.n != 22 || script.p[0] != 0 || script.p[1] != 20 ||
	    pubkey.n != 33 || path.n != 24 ||
	    u32(path.p) != policy->fingerprint ||
	    u32(path.p + 4) != (HARDENED | 84u) ||
	    u32(path.p + 8) != (HARDENED | policy->coin) ||
	    u32(path.p + 12) != (HARDENED | policy->account)) return false;
	key->branch = u32(path.p + 16);
	key->index = u32(path.p + 20);
	return key->branch <= 1 && key->index < HARDENED &&
	       !policy->derive(policy->wallet, key->branch, key->index, key->pubkey, hash) &&
	       !memcmp(key->pubkey, pubkey.p, 33) && !memcmp(hash, script.p + 2, 20);
}

static bool load_input(const map *fields, const tx_input *input, const nu54_btc_policy *policy,
		       nu54_btc_input *result, span *script)
{
	transaction parent;
	uint8_t txid[32];
	tx_output witness;
	span raw = fields->values[0], witness_utxo = fields->values[1], sighash = fields->values[3];
	uint32_t index = u32(input->outpoint.p + 32);
	if ((sighash.p && (sighash.n != 4 || u32(sighash.p) != 1)) ||
	    !parse_tx(raw, false, &parent) || index >= parent.no ||
	    policy->hash256(raw.p, raw.n, txid) || memcmp(txid, input->outpoint.p, sizeof(txid))) return false;
	const tx_output *prevout = &parent.out[index];
	if (witness_utxo.p && (!output(&witness_utxo, &witness) || witness_utxo.n ||
	    witness.wire.n != prevout->wire.n || memcmp(witness.wire.p, prevout->wire.p, witness.wire.n)))
		return false;
	result->amount = prevout->amount;
	*script = prevout->script;
	return owned(*script, fields->pubkey, fields->values[6], policy, result);
}

static void append(uint8_t **end, span bytes)
{
	memcpy(*end, bytes.p, bytes.n);
	*end += bytes.n;
}

static bool digest(const transaction *tx, const span *scripts, const nu54_btc_policy *policy,
		   nu54_btc_intent *review)
{
	uint8_t buffer[NU54_BTC_OUTPUT_MAX * (9 + NU54_BTC_SCRIPT_MAX) + 16];
	uint8_t prevouts[32], sequences[32], outputs[32], *end = buffer;
	for (uint32_t i = 0; i < tx->ni; i++) append(&end, tx->in[i].outpoint);
	if (policy->hash256(buffer, (size_t)(end - buffer), prevouts)) return false;
	end = buffer;
	for (uint32_t i = 0; i < tx->ni; i++) append(&end, tx->in[i].sequence);
	if (policy->hash256(buffer, (size_t)(end - buffer), sequences)) return false;
	end = buffer;
	for (uint32_t i = 0; i < tx->no; i++) append(&end, tx->out[i].wire);
	if (policy->hash256(buffer, (size_t)(end - buffer), outputs)) return false;
	for (uint32_t i = 0; i < tx->ni; i++) {
		static const uint8_t script_start[] = {25, 0x76, 0xa9, 20};
		static const uint8_t script_end[] = {0x88, 0xac};
		static const uint8_t sighash[] = {1, 0, 0, 0};
		end = buffer;
		append(&end, tx->version);
		append(&end, (span){prevouts, sizeof(prevouts)});
		append(&end, (span){sequences, sizeof(sequences)});
		append(&end, tx->in[i].outpoint);
		append(&end, (span){script_start, sizeof(script_start)});
		append(&end, (span){scripts[i].p + 2, 20});
		append(&end, (span){script_end, sizeof(script_end)});
		for (unsigned j = 0; j < 8; j++) *end++ = (uint8_t)(review->inputs[i].amount >> (j * 8));
		append(&end, tx->in[i].sequence);
		append(&end, (span){outputs, sizeof(outputs)});
		append(&end, tx->locktime);
		append(&end, (span){sighash, sizeof(sighash)});
		if (policy->hash256(buffer, (size_t)(end - buffer), review->inputs[i].digest)) return false;
	}
	return true;
}

static bool supported_script(span script)
{
	/* Only the two displayed native v0 witness recipient forms. */
	return (script.n == 22 || script.n == 34) &&
	       script.p[0] == 0 && script.p[1] == script.n - 2;
}

int nu54_btc_review(const uint8_t *bytes, size_t length, const nu54_btc_policy *policy,
		    nu54_btc_intent *intent)
{
	static const uint8_t magic[] = {'p', 's', 'b', 't', 0xff};
	span source = {bytes, length}, tag, scripts[NU54_BTC_INPUT_MAX];
	transaction tx;
	map fields;
	nu54_btc_intent review = {0};
	uint64_t inputs = 0, outputs = 0;
	if (!intent) return -1;
	memset(intent, 0, sizeof(*intent));
	if (!bytes || length > NU54_BTC_PSBT_MAX || !policy || !policy->derive || !policy->hash256 ||
	    policy->coin > 1 || policy->account >= HARDENED ||
	    !take(&source, sizeof(magic), &tag) || memcmp(tag.p, magic, sizeof(magic)) ||
	    !read_map(&source, 1u, 0xff, &fields) || !parse_tx(fields.values[0], true, &tx) ||
	    u32(tx.version.p) != 2) return -1;
	review.input_count = (uint8_t)tx.ni;
	review.output_count = (uint8_t)tx.no;
	review.locktime = u32(tx.locktime.p);
	for (uint32_t i = 0; i < tx.ni; i++) {
		for (uint32_t j = 0; j < i; j++) {
			if (!memcmp(tx.in[i].outpoint.p, tx.in[j].outpoint.p, 36)) return -1;
		}
		uint32_t sequence = u32(tx.in[i].sequence.p);
		if (sequence < UINT32_C(0xfffffffd)) return -1;
		review.replaceable |= sequence == UINT32_C(0xfffffffd);
		if (!read_map(&source, 0x4b, 6, &fields) ||
		    !load_input(&fields, &tx.in[i], policy, &review.inputs[i], &scripts[i])) return -1;
		inputs += review.inputs[i].amount;
	}
	for (uint32_t i = 0; i < tx.no; i++) {
		const tx_output *out = &tx.out[i];
		nu54_btc_input change;
		if (!supported_script(out->script) || !out->amount ||
		    !read_map(&source, 4u, 2, &fields)) return -1;
		if (fields.values[2].p) {
			if (!owned(out->script, fields.pubkey, fields.values[2], policy, &change) ||
			    change.branch != 1) return -1;
			review.outputs[i].change = true;
		}
		review.outputs[i].amount = out->amount;
		review.outputs[i].length = (uint8_t)out->script.n;
		memcpy(review.outputs[i].script, out->script.p, out->script.n);
		outputs += out->amount;
	}
	if (source.n || inputs > MAX_MONEY || outputs > inputs ||
	    inputs - outputs > policy->max_fee || !digest(&tx, scripts, policy, &review)) return -1;
	if (policy->hash256(bytes, length, review.commitment)) return -1;
	review.fee = inputs - outputs;
	*intent = review;
	return 0;
}

static uint8_t *der_integer(uint8_t *end, const uint8_t value[32])
{
	size_t skip = 0;
	while (skip < 31 && !value[skip]) skip++;
	bool pad = (value[skip] & 0x80) != 0;
	*end++ = 2;
	*end++ = (uint8_t)(32 - skip + pad);
	if (pad) *end++ = 0;
	append(&end, (span){value + skip, 32 - skip});
	return end;
}

int nu54_btc_finalize(const uint8_t *bytes, size_t length, const nu54_btc_policy *policy,
		      const uint8_t approved[32], nu54_btc_sign sign, uint8_t *signed_tx,
		      size_t capacity, size_t *written)
{
	nu54_btc_intent review;
	map fields;
	span source;
	if (!written) return -1;
	*written = 0;
	if (!signed_tx || capacity < NU54_BTC_SIGNED_MAX || !sign || !approved ||
	    nu54_btc_review(bytes, length, policy, &review) ||
	    memcmp(approved, review.commitment, 32)) return -1;
	source = (span){bytes + 5, length - 5};
	if (!read_map(&source, 1u, 0xff, &fields)) return -1;
	span raw = fields.values[0];
	uint8_t out[NU54_BTC_SIGNED_MAX], *end = out;
	append(&end, (span){raw.p, 4});
	*end++ = 0;
	*end++ = 1;
	append(&end, (span){raw.p + 4, raw.n - 8});
	for (uint8_t i = 0; i < review.input_count; i++) {
		uint8_t rs[64], der[73];
		if (sign(policy->wallet, &review.inputs[i], rs)) return -1;
		uint8_t *der_end = der_integer(der + 2, rs);
		der_end = der_integer(der_end, rs + 32);
		der[0] = 0x30;
		der[1] = (uint8_t)(der_end - der - 2);
		*der_end++ = 1;
		*end++ = 2;
		*end++ = (uint8_t)(der_end - der);
		append(&end, (span){der, (size_t)(der_end - der)});
		*end++ = 33;
		append(&end, (span){review.inputs[i].pubkey, 33});
	}
	append(&end, (span){raw.p + raw.n - 4, 4});
	*written = (size_t)(end - out);
	memcpy(signed_tx, out, *written);
	return 0;
}


int nu54_btc_check_context(const nu54_btc_intent *intent, const uint8_t data[90], uint64_t exact_fee)
{
	uint8_t hash = 0;
	if (!intent || !data || data[0] > 1) return -1;
	for (size_t i = data[0] ? 41 : 1; i < 90; ++i) if (data[i]) return -1;
	if (!data[0]) return 0;
	for (size_t i = 1; i < 33; ++i) hash |= data[i];
	uint64_t original_fee = 0;
	for (size_t i = 33; i < 41; ++i) original_fee = (original_fee << 8) | data[i];
	return hash && intent->replaceable && intent->fee == exact_fee && intent->fee > original_fee &&
	       intent->output_count == 2 && !intent->outputs[0].change && intent->outputs[1].change ? 0 : -1;
}
