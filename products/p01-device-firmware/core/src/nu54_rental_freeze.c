#include "nu54_rental_freeze.h"
#include "nu54_session_int.h"

#include "nu54_keccak.h"

static void word_uint64(uint8_t word[32], uint64_t value)
{
	memset(word, 0, 24);
	for (unsigned i = 0; i < 8; ++i) word[31 - i] = (uint8_t)(value >> (8 * i));
}

static void digest_words(const nu54_rental_binding_t *binding, const uint8_t *words, size_t length, uint8_t digest[32])
{
	uint8_t body[32], domain[32];
	nu54_keccak256(words, length, body);
	nu54_eip712_domain_separator(binding->chain, binding->contract, domain);
	nu54_eip712_digest(domain, body, digest);
}

void nu54_rental_freeze_request_digest(const nu54_rental_binding_t *binding, const nu54_rental_freeze_request_t *request,
				      uint8_t digest[32])
{
	static const char type[] = NU54_ENCODE_TYPE_RENTAL_FREEZE_REQUEST_V2;
	uint8_t words[5][32];
	nu54_keccak256((const uint8_t *)type, sizeof(type) - 1, words[0]);
	nu54_keccak256((const uint8_t *)binding, sizeof(*binding), words[1]);
	memcpy(words[2], request->challenge, 32);
	memcpy(words[3], request->channel_nonce, 32);
	word_uint64(words[4], request->expires_at);
	digest_words(binding, (const uint8_t *)words, sizeof(words), digest);
}

void nu54_rental_freeze_receipt_digest(const nu54_rental_binding_t *binding, const nu54_rental_freeze_receipt_t *receipt,
				      uint8_t digest[32])
{
	static const char type[] = "RentalFreezeReceipt(bytes32 binding,bytes32 challenge,bytes32 projectionRoot,"
				   "uint256 firstNonce,uint256 nextNonce,uint64 entryCount,uint64 frozenAt,uint64 expiresAt)";
	uint8_t words[9][32];
	nu54_keccak256((const uint8_t *)type, sizeof(type) - 1, words[0]);
	memcpy(words[1], receipt->binding, 32);
	memcpy(words[2], receipt->challenge, 32);
	memcpy(words[3], receipt->projection_root, 32);
	memcpy(words[4], receipt->first_nonce, 32);
	memcpy(words[5], receipt->next_nonce, 32);
	word_uint64(words[6], receipt->entry_count);
	word_uint64(words[7], receipt->frozen_at);
	word_uint64(words[8], receipt->expires_at);
	digest_words(binding, (const uint8_t *)words, sizeof(words), digest);
}

int nu54_device_attest_rental_freeze(nu54_device_t *d, const nu54_rental_freeze_request_t *request,
				     const uint8_t operator_signature[65], const uint8_t board_identity[20],
				     nu54_rental_freeze_receipt_t *receipt, uint8_t board_signature[65])
{
	static const uint8_t zero[32] = {0};
	if (!d || !request || !operator_signature || !board_identity || !receipt || !board_signature) return -1;
	nu54_rental_journal_t *journal = d->rental_journal;
	uint64_t now;
	if (!journal || !d->cfg.platform.sign_rental_attestation || ss_rental_time(d, &now) != 0 ||
	    request->expires_at <= now || request->expires_at - now > NU54_RENTAL_FREEZE_LIFETIME ||
	    !memcmp(request->challenge, zero, sizeof(request->challenge)) ||
	    !memcmp(request->channel_nonce, zero, sizeof(request->channel_nonce)) || !memcmp(board_identity, zero, 20) ||
	    !memcmp(board_identity, d->address, 20) || !memcmp(board_identity, d->operator_address, 20)) return -1;

	const nu54_rental_binding_t *binding = &journal->expected_binding;
	uint8_t digest[32], signer[20];
	nu54_rental_freeze_request_digest(binding, request, digest);
	if (nu54_sig_recover(digest, operator_signature, 65, signer) != NU54_SIG_OK ||
	    memcmp(signer, d->operator_address, sizeof(signer)) != 0) return -1;

	nu54_rental_freeze_receipt_t confirmed = {0};
	if (nu54_device_freeze_rental_journal(d, confirmed.projection_root) != 0) return -1;
	for (unsigned i = 0; i < 8; ++i)
		confirmed.frozen_at = (confirmed.frozen_at << 8) | journal->image.frozen_at[i];
	if (confirmed.frozen_at > now || confirmed.frozen_at > request->expires_at) return -1;
	nu54_keccak256((const uint8_t *)binding, sizeof(*binding), confirmed.binding);
	memcpy(confirmed.challenge, request->challenge, 32);
	memcpy(confirmed.first_nonce, journal->image.first, 32);
	memcpy(confirmed.next_nonce, journal->image.next, 32);
	confirmed.entry_count = journal->image.count;
	confirmed.expires_at = request->expires_at;

	uint8_t raw[64], signature[65];
	nu54_rental_freeze_receipt_digest(binding, &confirmed, digest);
	if (d->cfg.platform.sign_rental_attestation(d->cfg.platform.ctx, digest, raw) != 0 ||
	    nu54_sig_finish(digest, raw, board_identity, signature) != NU54_SIG_OK) return -1;
	*receipt = confirmed;
	memcpy(board_signature, signature, sizeof(signature));
	return 0;
}
