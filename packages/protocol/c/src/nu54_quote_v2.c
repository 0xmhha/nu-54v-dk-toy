#include "nu54_quote_v2.h"

#include <string.h>

static const uint8_t widths[] = NU54_V2_WIDTHS;

static bool zero(const uint8_t *raw, size_t length)
{
	for (size_t i = 0; i < length; i++) {
		if (raw[i] != 0) return false;
	}
	return true;
}

static uint64_t uint64_word(const uint8_t *word)
{
	uint64_t value = 0;
	for (size_t i = 24; i < 32; i++) {
		value = (value << 8) | word[i];
	}
	return value;
}

static bool valid(const uint8_t *raw, size_t length)
{
	if (raw == NULL || length != NU54_V2_UNSIGNED_SIZE) return false;
	for (size_t i = 0; i < NU54_V2_FIELD_COUNT; i++) {
		const uint8_t *word = raw + 32 * i;
		size_t pad = 32 - widths[i];
		if (!zero(word, pad) || zero(word + pad, widths[i])) return false;
	}
	uint8_t mode = raw[NU54_V2_MODE_OFFSET + 31], role = raw[NU54_V2_PAYER_ROLE_OFFSET + 31];
	return (mode == NU54_V2_ESCROW_MODE || mode == NU54_V2_DIRECT_MODE) &&
		(role == NU54_V2_OWNER_ROLE || role == NU54_V2_RENTER_ROLE) &&
		(mode != NU54_V2_DIRECT_MODE || role == NU54_V2_OWNER_ROLE) &&
		uint64_word(raw + NU54_V2_EXPIRES_AT_OFFSET) > uint64_word(raw + NU54_V2_ISSUED_AT_OFFSET);
}

bool nu54_v2_quote_encode(const nu54_v2_quote_field fields[NU54_V2_FIELD_COUNT],
	size_t field_count, uint8_t out[NU54_V2_UNSIGNED_SIZE])
{
	if (!fields || field_count != NU54_V2_FIELD_COUNT || !out) return false;
	for (size_t i = 0; i < field_count; i++) {
		if (!fields[i].bytes || fields[i].length != widths[i]) return false;
	}
	memset(out, 0, NU54_V2_UNSIGNED_SIZE);
	for (size_t i = 0; i < field_count; i++) {
		memcpy(out + (i + 1) * 32 - widths[i], fields[i].bytes, widths[i]);
	}
	return valid(out, NU54_V2_UNSIGNED_SIZE);
}

static void hash_text(const nu54_v2_quote_crypto *crypto, const char *text, uint8_t out[32])
{
	crypto->keccak256((const uint8_t *)text, strlen(text), out);
}

bool nu54_v2_quote_digest(const uint8_t *raw, size_t length,
	const nu54_v2_quote_crypto *crypto, uint8_t out[32])
{
	if (!crypto || !crypto->keccak256 || !out || !valid(raw, length)) return false;
	uint8_t buffer[32 + NU54_V2_UNSIGNED_SIZE - NU54_V2_QUOTE_ID_OFFSET];
	uint8_t envelope[66] = {0x19, 0x01};

	hash_text(crypto, NU54_V2_DOMAIN_TYPE, buffer);
	hash_text(crypto, NU54_V2_DOMAIN_NAME, buffer + 32);
	hash_text(crypto, NU54_V2_DOMAIN_VERSION, buffer + 64);
	memcpy(buffer + 96, raw, NU54_V2_QUOTE_ID_OFFSET);
	crypto->keccak256(buffer, 96 + NU54_V2_QUOTE_ID_OFFSET, &envelope[2]);

	hash_text(crypto, NU54_V2_QUOTE_TYPE, buffer);
	memcpy(buffer + 32, raw + NU54_V2_QUOTE_ID_OFFSET, length - NU54_V2_QUOTE_ID_OFFSET);
	crypto->keccak256(buffer, sizeof buffer, &envelope[34]);
	crypto->keccak256(envelope, sizeof envelope, out);
	return true;
}

static bool signature_valid(const uint8_t sig[65])
{
	static const uint8_t order[32] = {
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfe,
		0xba, 0xae, 0xdc, 0xe6, 0xaf, 0x48, 0xa0, 0x3b, 0xbf, 0xd2, 0x5e, 0x8c, 0xd0, 0x36, 0x41, 0x41
	};
	static const uint8_t half_order[32] = {
		0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0x5d, 0x57, 0x6e, 0x73, 0x57, 0xa4, 0x50, 0x1d, 0xdf, 0xe9, 0x2f, 0x46, 0x68, 0x1b, 0x20, 0xa0
	};
	return (sig[64] == 27 || sig[64] == 28) &&
		!zero(sig, 32) && !zero(sig + 32, 32) &&
		memcmp(sig, order, 32) < 0 && memcmp(sig + 32, half_order, 32) <= 0;
}

int nu54_v2_quote_verify(const uint8_t *signed_quote, size_t length, const nu54_v2_quote_policy *policy,
	const nu54_v2_quote_crypto *crypto)
{
	if (length != NU54_V2_SIGNED_SIZE || !policy || !crypto || !crypto->keccak256 ||
		!crypto->recover_address || !valid(signed_quote, NU54_V2_UNSIGNED_SIZE) ||
		!valid(policy->expected, NU54_V2_UNSIGNED_SIZE) || zero(policy->merchant_signer, 20)) {
		return NU54_V2_STATUS_MALFORMED;
	}
	if (!zero(signed_quote, 24) || uint64_word(signed_quote) != NU54_V2_PILOT_CHAIN ||
		signed_quote[NU54_V2_MODE_OFFSET + 31] != NU54_V2_ESCROW_MODE) {
		return NU54_V2_STATUS_UNSUPPORTED_PROFILE;
	}
	if (memcmp(signed_quote, policy->expected, NU54_V2_UNSIGNED_SIZE) != 0) {
		return NU54_V2_STATUS_WRONG_BINDING;
	}
	if (policy->now < uint64_word(signed_quote + NU54_V2_ISSUED_AT_OFFSET) ||
		policy->now >= uint64_word(signed_quote + NU54_V2_EXPIRES_AT_OFFSET)) {
		return NU54_V2_STATUS_EXPIRED;
	}

	const uint8_t *sig = signed_quote + NU54_V2_UNSIGNED_SIZE;
	uint8_t digest[32], signer[20];
	if (!signature_valid(sig) || !nu54_v2_quote_digest(signed_quote, NU54_V2_UNSIGNED_SIZE, crypto, digest) ||
		!crypto->recover_address(digest, sig, signer) ||
		memcmp(signer, policy->merchant_signer, 20) != 0) {
		return NU54_V2_STATUS_UNTRUSTED_SIGNER;
	}
	return policy->consumed ? NU54_V2_STATUS_CONSUMED : NU54_V2_STATUS_OK;
}
