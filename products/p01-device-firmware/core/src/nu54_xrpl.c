#include "nu54_xrpl.h"

#include <string.h>

typedef struct { const uint8_t *p; size_t n; } cursor;

static const uint8_t *take(cursor *input, size_t length)
{
	if (length > input->n) return NULL;
	const uint8_t *start = input->p;
	input->p += length;
	input->n -= length;
	return start;
}

static bool field(cursor *input, uint8_t id)
{
	const uint8_t *tag = take(input, 1);
	return tag && *tag == id;
}

static bool next_is(const cursor *input, uint8_t id)
{
	return input->n && *input->p == id;
}

static uint64_t big(const uint8_t *p, size_t size)
{
	uint64_t value = 0;
	while (size--) value = (value << 8) | *p++;
	return value;
}

static bool uint32(cursor *input, uint32_t *result)
{
	const uint8_t *p = take(input, 4);
	if (!p) return false;
	*result = (uint32_t)big(p, 4);
	return true;
}

static bool read_xrp(uint64_t value, uint64_t *drops)
{
	if (value >> 62 != 1) return false;
	*drops = value & UINT64_C(0x3fffffffffffffff);
	return *drops && *drops <= UINT64_C(100000000000000000);
}

static bool read_amount(cursor *input, const nu54_xrpl_policy *policy, nu54_xrpl_intent *out)
{
	const uint8_t *amount = take(input, 8);
	if (!amount) return false;
	uint64_t value = big(amount, 8);
	if (!(value >> 63)) return read_xrp(value, &out->coefficient);
	if (value >> 62 != 3 || !policy->allow_issued) return false;
	out->coefficient = value & UINT64_C(0x003fffffffffffff);
	out->exponent = (int16_t)((value >> 54) & 0xff) - 97;
	if (out->coefficient < UINT64_C(1000000000000000) ||
	    out->coefficient > UINT64_C(9999999999999999) ||
	    out->exponent < -96 || out->exponent > 80) return false;
	const uint8_t *currency = take(input, 20);
	const uint8_t *issuer = take(input, 20);
	static const uint8_t zero[20];
	static const uint8_t xrp[20] = {[12] = 'X', [13] = 'R', [14] = 'P'};
	if (!issuer || !currency || !memcmp(issuer, zero, 20) || !memcmp(currency, zero, 20) ||
	    !memcmp(currency, xrp, 20) || memcmp(issuer, policy->issuer, 20) ||
	    memcmp(currency, policy->currency, 20)) return false;
	memcpy(out->issuer, issuer, 20);
	memcpy(out->currency, currency, 20);
	out->issued = true;
	return true;
}

static const uint8_t *variable(cursor *input, uint8_t id, uint8_t length)
{
	return field(input, id) && field(input, length) ? take(input, length) : NULL;
}

static int review_payment(const uint8_t *raw, size_t length, const nu54_xrpl_policy *policy,
			  nu54_xrpl_intent *intent, size_t *signature_offset)
{
	if (!intent) return -1;
	memset(intent, 0, sizeof(*intent));
	if (!raw || length > NU54_XRPL_UNSIGNED_MAX || !policy || !policy->sha512_half) return -1;
	cursor input = {raw, length};
	nu54_xrpl_intent review = {0};
	/* One canonical field order with explicit allowlist. Unknown fields,
	 * Paths, DeliverMin, multisigners and duplicate fields fail. */
	if (!field(&input, 0x12) || !field(&input, 0) || !field(&input, 0)) return -1;
	if (next_is(&input, 0x22)) {
		uint32_t flags;
		if (!field(&input, 0x22) || !uint32(&input, &flags) ||
		    (flags != 0 && flags != UINT32_C(0x80000000))) return -1;
	}
	if (!field(&input, 0x24) || !uint32(&input, &review.sequence)) return -1;
	if (next_is(&input, 0x2e)) {
		if (!field(&input, 0x2e) || !uint32(&input, &review.destination_tag)) return -1;
		review.has_tag = true;
	}
	if (!field(&input, 0x20) || !field(&input, 27) || !uint32(&input, &review.last_ledger) ||
	    review.last_ledger <= policy->ledger ||
	    review.last_ledger - policy->ledger > policy->max_ledger_delta) return -1;
	if (!review.sequence) {
		if (!policy->ticket || !field(&input, 0x20) || !field(&input, 41) ||
		    !uint32(&input, &review.ticket) || review.ticket != policy->ticket) return -1;
	} else if (review.sequence != policy->sequence) return -1;
	if (!field(&input, 0x61)) return -1;
	const uint8_t *amount = input.p;
	if (!read_amount(&input, policy, &review) || !field(&input, 0x68)) return -1;
	const uint8_t *fee = take(&input, 8);
	if (!fee || !read_xrp(big(fee, 8), &review.fee_drops) || review.fee_drops > policy->max_fee)
		return -1;
	/* For holder-to-holder payments, commit to a same-issuer, same-currency
	 * source with an exact spend cap. Refuse a widened path or fee in token units. */
	const bool holder = review.issued && memcmp(review.issuer, policy->account, 20);
	if (holder) {
		const uint8_t *sendmax = field(&input, 0x69) ? take(&input, 48) : NULL;
		if (!sendmax || memcmp(amount, sendmax, 48)) return -1;
	}
	const uint8_t *public = variable(&input, 0x73, 33);
	if (!public || (public[0] != 2 && public[0] != 3) || memcmp(public, policy->public_key, 33))
		return -1;
	*signature_offset = length - input.n;
	const uint8_t *account = variable(&input, 0x81, 20);
	const uint8_t *destination = variable(&input, 0x83, 20);
	static const uint8_t zero[20];
	if (!account || !destination || input.n || memcmp(account, policy->account, 20) ||
	    !memcmp(destination, zero, 20) || !memcmp(account, destination, 20) ||
	    (review.issued && !memcmp(destination, review.issuer, 20))) return -1;
	memcpy(review.account, account, 20);
	memcpy(review.destination, destination, 20);
	uint8_t preimage[4 + NU54_XRPL_UNSIGNED_MAX] = {'S', 'T', 'X', 0};
	memcpy(preimage + 4, raw, length);
	if (policy->sha512_half(preimage, length + 4, review.digest)) return -1;
	*intent = review;
	return 0;
}

int nu54_xrpl_review(const uint8_t *raw, size_t length, const nu54_xrpl_policy *policy,
		     nu54_xrpl_intent *intent)
{
	size_t offset;
	return review_payment(raw, length, policy, intent, &offset);
}

int nu54_xrpl_finalize(const uint8_t *raw, size_t length, const nu54_xrpl_policy *policy,
		       const uint8_t approved[32], nu54_xrpl_sign sign, void *context,
		       uint8_t *signed_tx, size_t capacity, size_t *written)
{
	nu54_xrpl_intent review;
	uint8_t signature[72];
	size_t offset, siglen = sizeof(signature);
	if (!written) return -1;
	*written = 0;
	if (!approved || !sign || !signed_tx || capacity < NU54_XRPL_SIGNED_MAX ||
	    review_payment(raw, length, policy, &review, &offset) ||
	    memcmp(approved, review.digest, 32) ||
	    sign(context, policy->public_key, review.digest, signature, &siglen) ||
	    siglen < 8 || siglen > sizeof(signature)) return -1;
	memmove(signed_tx + offset + 2 + siglen, raw + offset, length - offset);
	memmove(signed_tx, raw, offset);
	signed_tx[offset] = 0x74;
	signed_tx[offset + 1] = (uint8_t)siglen;
	memcpy(signed_tx + offset + 2, signature, siglen);
	*written = length + 2 + siglen;
	return 0;
}
