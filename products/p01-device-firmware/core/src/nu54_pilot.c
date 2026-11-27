#include "nu54_pilot.h"
#include "nu54_keccak.h"
#include "nu54_sig.h"

#include <string.h>

static bool recover(const uint8_t digest[32], const uint8_t signature[65], uint8_t address[20])
{
	return nu54_sig_recover(digest, signature, 65, address) == NU54_SIG_OK;
}

/* This approval path is limited to the enrolled settlement and asset on StableNet 8283. */
#define PILOT_CHAIN UINT64_C(8283)

static const nu54_v2_quote_crypto crypto = {nu54_keccak256, recover};

static void hash_text(const char *text, uint8_t out[32])
{
	nu54_keccak256((const uint8_t *)text, strlen(text), out);
}

bool nu54_pilot_approval_preimage(const uint8_t quote[NU54_V2_UNSIGNED_SIZE], uint8_t out[NU54_LOCAL_PREIMAGE_SIZE])
{
	uint8_t fields[160], quote_digest[32];
	if (!nu54_v2_quote_digest(quote, NU54_V2_UNSIGNED_SIZE, &crypto, quote_digest)) return false;
	hash_text(NU54_V2_DOMAIN_TYPE, fields);
	hash_text(NU54_V2_DOMAIN_NAME, fields + 32);
	hash_text(NU54_V2_DOMAIN_VERSION, fields + 64);
	memcpy(fields + 96, quote, 64);
	out[0] = 0x19;
	out[1] = 1;
	nu54_keccak256(fields, sizeof(fields), out + 2);
	hash_text("PaymentApproval(bytes32 quoteDigest)", fields);
	memcpy(fields + 32, quote_digest, sizeof(quote_digest));
	nu54_keccak256(fields, 64, out + 34);
	return true;
}

static bool endpoint(const nu54_v2_link *channel)
{
	return channel && !channel->closed && channel->local == NU54_V2_BOARD &&
	       ((channel->profile == NU54_V2_MANAGEMENT && channel->peer == NU54_V2_OWNER) ||
		(channel->profile == NU54_V2_PAYMENT && channel->peer == NU54_V2_TERMINAL));
}

int nu54_pilot_review_bind(nu54_pilot_review_binding *binding, const nu54_v2_link *channel,
			  const uint8_t request[16], const uint8_t quote[NU54_V2_SIGNED_SIZE], nu54_pilot_hash hash)
{
	const uint8_t zero[16] = {0};
	memset(binding, 0, sizeof(*binding));
	if (!endpoint(channel) || channel->profile != NU54_V2_MANAGEMENT ||
	    !memcmp(zero, request, sizeof(zero)) || hash(quote, NU54_V2_SIGNED_SIZE, binding->digest)) return -1;
	memcpy(binding->request, request, sizeof(binding->request));
	memcpy(binding->session, channel->epoch, sizeof(binding->session));
	binding->ready = true;
	return 0;
}

int nu54_pilot_review_consume(nu54_pilot_review_binding *binding, const nu54_v2_link *channel,
			     const nu54_v2_request *request, nu54_pilot_hash hash)
{
	uint8_t digest[32];
	bool match = binding->ready && endpoint(channel) && channel->profile == NU54_V2_MANAGEMENT &&
		     request->command == NU54_V2_PAY_PREPARE && request->payload_length == NU54_V2_SIGNED_SIZE &&
		     !memcmp(binding->session, channel->epoch, sizeof(binding->session)) &&
		     !memcmp(binding->request, request->id, sizeof(binding->request)) &&
		     !hash(request->payload, request->payload_length, digest) &&
		     !memcmp(binding->digest, digest, sizeof(digest));
	memset(binding, 0, sizeof(*binding));
	return match ? 0 : -1;
}

static uint64_t word_u64(const uint8_t *word)
{
	uint64_t value = 0;
	for (size_t i = 24; i < 32; ++i) value = (value << 8) | word[i];
	return value;
}

int nu54_pilot_verify_quote(const uint8_t *quote, size_t length, const nu54_v2_quote_policy *order,
                            const nu54_pilot_policy *allowlist, const uint8_t payer[20], uint64_t generation)
{
	if (!order || !allowlist || !payer ||
	    nu54_v2_quote_verify(quote, length, order, &crypto) != NU54_V2_STATUS_OK) return -1;
	const uint8_t zero[24] = {0};
	uint64_t cap = allowlist->commercial_testnet ? NU54_PILOT_TESTUSDC2_CAP : NU54_PILOT_LEGACY_CAP;
	if (memcmp(quote + NU54_V2_CHAIN_ID_OFFSET, zero, sizeof(zero)) ||
	    word_u64(quote + NU54_V2_CHAIN_ID_OFFSET) != PILOT_CHAIN ||
	    allowlist->max_amount == 0 || allowlist->max_amount > cap ||
	    memcmp(quote + NU54_V2_VERIFYING_CONTRACT_OFFSET + 12, allowlist->contract, 20) ||
	    memcmp(quote + NU54_V2_ASSET_OFFSET + 12, allowlist->token, 20) ||
	    memcmp(quote + NU54_V2_BENEFICIARY_OFFSET + 12, allowlist->beneficiary, 20) ||
	    memcmp(quote + NU54_V2_PAYER_OFFSET + 12, payer, 20) ||
	    memcmp(quote + NU54_V2_AMOUNT_OFFSET, zero, sizeof(zero)) ||
	    word_u64(quote + NU54_V2_AMOUNT_OFFSET) > allowlist->max_amount ||
	    word_u64(quote + NU54_V2_PAYER_GENERATION_OFFSET) != generation) return -1;
	return 0;
}

int nu54_pilot_stage(nu54_local_approval *approval, const nu54_v2_link *channel, const nu54_v2_request *request,
		      const nu54_v2_quote_policy *order, const nu54_pilot_policy *allowlist,
		      const uint8_t payer[20], uint64_t generation, uint32_t physical_prompt,
		      int (*present)(void *, const uint8_t *, size_t), void *context)
{
	if (!endpoint(channel) || !request || request->command != NU54_V2_PAY_PREPARE ||
	    !channel->pending_expiry || channel->pending_expiry != request->expires ||
	    memcmp(channel->pending_id, request->id, sizeof(request->id)) ||
	    nu54_pilot_verify_quote(request->payload, request->payload_length, order, allowlist, payer, generation))
		return -1;
	const uint8_t *quote = request->payload;
	uint64_t expiry = word_u64(quote + NU54_V2_EXPIRES_AT_OFFSET);
	nu54_local_intent intent = {.generation = generation, .expires = expiry < request->expires ? expiry : request->expires,
				    .physical_prompt = physical_prompt};
	memcpy(intent.session, channel->epoch, sizeof(intent.session));
	memcpy(intent.request, request->id, sizeof(intent.request));
	if (!nu54_pilot_approval_preimage(quote, intent.preimage)) return -1;
	return nu54_local_approval_stage(approval, &intent, order->now, present, context);
}

static void write_u64(uint8_t *out, uint64_t value)
{
	for (size_t i = 8; i; --i) {
		out[i - 1] = (uint8_t)value;
		value >>= 8;
	}
}

static int p256_canonical_signature(uint8_t signature[64])
{
	static const uint8_t zero[32];
	static const uint8_t order[32] = {
		0xff, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xbc, 0xe6, 0xfa, 0xad, 0xa7, 0x17, 0x9e, 0x84, 0xf3, 0xb9, 0xca, 0xc2, 0xfc, 0x63, 0x25, 0x51,
	};
	static const uint8_t half_order[32] = {
		0x7f, 0xff, 0xff, 0xff, 0x80, 0x00, 0x00, 0x00, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xde, 0x73, 0x7d, 0x56, 0xd3, 0x8b, 0xcf, 0x42, 0x79, 0xdc, 0xe5, 0x61, 0x7e, 0x31, 0x92, 0xa8,
	};
	uint8_t *s = signature + 32;
	if (!memcmp(signature, zero, 32) || !memcmp(s, zero, 32) ||
	    memcmp(signature, order, 32) >= 0 || memcmp(s, order, 32) >= 0)
		return -1;
	if (memcmp(s, half_order, 32) > 0) {
		unsigned borrow = 0;
		for (size_t i = 32; i; --i) {
			unsigned subtrahend = s[i - 1] + borrow;
			s[i - 1] = (uint8_t)(order[i - 1] - subtrahend);
			borrow = order[i - 1] < subtrahend;
		}
	}
	return 0;
}

int nu54_pilot_rental_review(const nu54_local_approval *approval, const nu54_v2_link *link,
    const nu54_v2_request *request, const uint8_t challenge[32], const uint8_t board_pin[32],
    uint64_t now, int (*hash)(const uint8_t *, size_t, uint8_t[32]),
    int (*sign)(const uint8_t[32], uint8_t[64]), uint8_t out[NU54_V2_RENTAL_REVIEW_RECEIPT_SIZE])
{
	static const uint8_t domain[] = "NU54 W10 rental payment review v1";
	static const uint8_t zero[32];
	uint8_t statement[sizeof(domain) + 232], preimage[NU54_LOCAL_PREIMAGE_SIZE], digest[32], signature[64];
	uint8_t *body = statement + sizeof(domain);
	if (!approval || !link || !request || !request->payload || !challenge || !board_pin || !hash || !sign || !out)
		return -1;
	const uint8_t *quote = request->payload;
	if (!endpoint(link) || link->profile != NU54_V2_PAYMENT || request->command != NU54_V2_PAY_PREPARE ||
	    !link->pending_rental_review || !link->pending_fresh || link->pending_reported ||
	    !approval->pending || !now || approval->intent.expires <= now || approval->intent.expires - now > 120 ||
	    request->expires != approval->intent.expires || request->expires != link->pending_expiry ||
	    memcmp(link->pending_id, request->id, sizeof(request->id)) ||
	    !memcmp(challenge, zero, 32) || memcmp(challenge, link->pending_review_challenge, 32) ||
	    !memcmp(board_pin, zero, 32) ||
	    !memcmp(board_pin, link->peer_identity, 32) ||
	    memcmp(approval->intent.session, link->epoch, sizeof(link->epoch)) ||
	    memcmp(approval->intent.request, request->id, sizeof(request->id)) ||
	    request->payload_length != NU54_V2_SIGNED_SIZE ||
	    quote[NU54_V2_PAYER_ROLE_OFFSET + 31] != NU54_V2_RENTER_ROLE ||
	    quote[NU54_V2_MODE_OFFSET + 31] != NU54_V2_ESCROW_MODE ||
	    !approval->intent.generation || word_u64(quote + NU54_V2_PAYER_GENERATION_OFFSET) != approval->intent.generation ||
	    word_u64(quote + NU54_V2_EXPIRES_AT_OFFSET) != request->expires ||
	    !nu54_pilot_approval_preimage(quote, preimage) || memcmp(approval->intent.preimage, preimage, sizeof(preimage))) return -1;
	memcpy(statement, domain, sizeof(domain));
	memcpy(body, "W10R", 4);
	memcpy(body + 4, board_pin, 32);
	memcpy(body + 36, link->peer_identity, 32);
	memcpy(body + 68, link->epoch, 16);
	memcpy(body + 84, request->id, 16);
	memcpy(body + 100, challenge, 32);
	memcpy(body + 132, quote + NU54_V2_ORDER_ID_OFFSET, 32);
	if (hash(quote, NU54_V2_SIGNED_SIZE, body + 164) ||
	    memcmp(body + 164, link->pending_review_quote_hash, 32)) return -1;
	memcpy(body + 196, quote + NU54_V2_PAYER_OFFSET + 12, 20);
	write_u64(body + 216, approval->intent.generation);
	write_u64(body + 224, request->expires);
	if (hash(statement, sizeof(statement), digest) || sign(digest, signature) ||
	    p256_canonical_signature(signature)) return -1;
	memcpy(out, body, 232);
	memcpy(out + 232, signature, sizeof(signature));
	return 0;
}

int nu54_pilot_rental_decision(const uint8_t review_hash[32], const uint8_t *payer_signature,
    int (*hash)(const uint8_t *, size_t, uint8_t[32]), int (*sign)(const uint8_t[32], uint8_t[64]),
    uint8_t out[NU54_W10_DECISION_SIZE])
{
	static const uint8_t domain[] = "NU54 W10 rental payment decision v1";
	static const uint8_t zero[32];
	if (!review_hash || !hash || !sign || !out || !memcmp(review_hash, zero, 32) ||
	    (payer_signature && payer_signature[64] != 27 && payer_signature[64] != 28)) return -1;
	uint8_t input[sizeof(domain) + 102] = {0}, digest[32], signature[64];
	uint8_t *body = input + sizeof(domain);
	memcpy(input, domain, sizeof(domain));
	memcpy(body, "W10D", 4);
	memcpy(body + 4, review_hash, 32);
	body[36] = payer_signature ? 1 : 2;
	if (payer_signature) memcpy(body + 37, payer_signature, 65);
	if (hash(input, sizeof(input), digest) || sign(digest, signature) ||
	    p256_canonical_signature(signature)) return -1;
	memcpy(out, body, 102);
	memcpy(out + 102, signature, sizeof(signature));
	return 0;
}

int nu54_pilot_display(const nu54_local_approval *approval, const uint8_t quote[NU54_V2_SIGNED_SIZE],
		       const uint8_t challenge[32], const uint8_t proof[64],
		       int (*verify)(const uint8_t *, size_t, const uint8_t [64]))
{
	uint8_t preimage[NU54_LOCAL_PREIMAGE_SIZE];
	if (!approval->pending || !nu54_pilot_approval_preimage(quote, preimage) ||
	    memcmp(approval->intent.preimage, preimage, sizeof(preimage))) return -1;
	uint8_t message[4 + 32 + NU54_V2_SIGNED_SIZE];
	memcpy(message, "N54D", 4);
	memcpy(message + 4, challenge, 32);
	memcpy(message + 36, quote, NU54_V2_SIGNED_SIZE);
	return verify(message, sizeof(message), proof);
}

int nu54_pilot_complete(nu54_local_approval *approval, const nu54_v2_link *channel,
			 uint64_t generation, uint64_t now, uint32_t physical_prompt, nu54_local_button button,
			 const uint8_t payer[20], int (*sign)(void *, const uint8_t[32], uint8_t[64]),
			 void *context, uint8_t signature[65])
{
	if (!endpoint(channel) || !payer || !sign || channel->pending_expiry <= now) {
		nu54_local_approval_cancel(approval);
		return -1;
	}
	nu54_local_intent current = approval->intent;
	current.generation = generation;
	memcpy(current.session, channel->epoch, sizeof(current.session));
	memcpy(current.request, channel->pending_id, sizeof(current.request));
	uint8_t preimage[NU54_LOCAL_PREIMAGE_SIZE], digest[32], raw[64];
	if (nu54_local_approval_consume(approval, &current, now, physical_prompt, button, preimage) != 0) return -1;
	nu54_keccak256(preimage, sizeof(preimage), digest);
	if (sign(context, digest, raw) != 0 || nu54_sig_finish(digest, raw, payer, signature) != NU54_SIG_OK) return -1;
	return 0;
}


int nu54_pilot_receipt_find(nu54_pilot_receipt_read read_slot, void *context, uint64_t generation,
			    const uint8_t request[16], const uint8_t digest[32],
			    nu54_pilot_receipt *result, unsigned *free_slot)
{
	*free_slot = 0;
	for (unsigned slot = 1; slot <= NU54_PILOT_RECEIPTS; ++slot) {
		nu54_pilot_receipt saved;
		int read = read_slot(context, slot, &saved);
		if (read > 0) {
			if (!*free_slot) *free_slot = slot;
			continue;
		}
		if (read < 0 || saved.complete != 1) return NU54_PILOT_RECEIPT_ERROR;
		bool same_request = !memcmp(saved.request, request, sizeof(saved.request));
		bool same_quote = !memcmp(saved.digest, digest, sizeof(saved.digest));
		if (same_request != same_quote) return NU54_PILOT_RECEIPT_ERROR;
		if (same_request) {
			if (generation != saved.generation) return NU54_PILOT_RECEIPT_ERROR;
			*result = saved;
			return NU54_PILOT_RECEIPT_RECORDED;
		}
	}
	return *free_slot ? NU54_PILOT_RECEIPT_NEW : NU54_PILOT_RECEIPT_FULL;
}


int nu54_pilot_refusal_find(nu54_pilot_refusal_read read_slot, void *context, uint64_t earliest,
                            const uint8_t request[16], const uint8_t digest[32], unsigned *free_slot)
{
	*free_slot = 0;
	for (unsigned slot = 1; slot <= NU54_PILOT_REFUSALS; ++slot) {
		nu54_pilot_refusal saved;
		int read = read_slot(context, slot, &saved);
		if (read < 0 || (!read && !saved.expires)) return NU54_PILOT_RECEIPT_ERROR;
		if (read > 0 || saved.expires <= earliest) {
			if (!*free_slot) *free_slot = slot;
			continue;
		}
		bool same_request = !memcmp(saved.request, request, sizeof(saved.request));
		bool same_quote = !memcmp(saved.digest, digest, sizeof(saved.digest));
		if (same_request != same_quote) return NU54_PILOT_RECEIPT_ERROR;
		if (same_request) return NU54_PILOT_RECEIPT_RECORDED;
	}
	return *free_slot ? NU54_PILOT_RECEIPT_NEW : NU54_PILOT_RECEIPT_FULL;
}

int nu54_pilot_refuse(nu54_local_approval *approval, const nu54_v2_link *channel,
                      const uint8_t quote[NU54_V2_SIGNED_SIZE], uint64_t now,
                      uint32_t prompt, nu54_pilot_hash hash,
                      nu54_pilot_refusal *record)
{
	uint8_t preimage[NU54_LOCAL_PREIMAGE_SIZE];
	const nu54_local_intent *intent = &approval->intent;
	if (!endpoint(channel) || !approval->pending || intent->physical_prompt != prompt ||
	    intent->expires <= now || channel->pending_expiry <= now ||
	    memcmp(intent->session, channel->epoch, sizeof(intent->session)) ||
	    memcmp(intent->request, channel->pending_id, sizeof(intent->request)) ||
	    !nu54_pilot_approval_preimage(quote, preimage) ||
	    memcmp(intent->preimage, preimage, sizeof(preimage)) ||
	    hash(preimage, sizeof(preimage), record->digest)) return -1;
	record->expires = word_u64(quote + NU54_V2_EXPIRES_AT_OFFSET);
	memcpy(record->request, intent->request, sizeof(record->request));
	nu54_local_approval_cancel(approval);
	return 0;
}


int nu54_pilot_reclaim_body(const uint8_t board[32], const nu54_pilot_receipt *receipt,
			    const uint8_t quote[NU54_V2_UNSIGNED_SIZE], uint64_t earliest_now, uint8_t disposition,
			    int (*hash)(const uint8_t *, size_t, uint8_t [32]),
			    uint8_t out[NU54_PILOT_RECLAIM_BODY_SIZE])
{
	uint8_t preimage[NU54_LOCAL_PREIMAGE_SIZE], digest[32];
	const uint8_t *expiry = quote + NU54_V2_EXPIRES_AT_OFFSET + 24;
	uint64_t expires = word_u64(quote + NU54_V2_EXPIRES_AT_OFFSET);
	if (receipt->complete != 1 || disposition < NU54_PILOT_RECLAIM_FINAL ||
	    disposition > NU54_PILOT_RECLAIM_NO_EFFECT || earliest_now <= expires ||
	    earliest_now - expires < NU54_PILOT_RECLAIM_HOLD_SECONDS ||
	    word_u64(quote + NU54_V2_PAYER_GENERATION_OFFSET) != receipt->generation ||
	    !nu54_pilot_approval_preimage(quote, preimage) || hash(preimage, sizeof(preimage), digest) ||
	    memcmp(digest, receipt->digest, sizeof(digest))) return -1;
	memcpy(out, "N54C", 4);
	memcpy(out + 4, board, 32);
	memcpy(out + 36, receipt->request, 16);
	memcpy(out + 52, digest, 32);
	memcpy(out + 84, quote + NU54_V2_PAYER_GENERATION_OFFSET + 24, 8);
	memcpy(out + 92, expiry, 8);
	out[100] = disposition;
	return 0;
}
