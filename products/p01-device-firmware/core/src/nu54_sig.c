#include "nu54_sig.h"

#include <string.h>

#include "nu54_keccak.h"
#include "secp256k1.h"
#include "secp256k1_recovery.h"

void nu54_address_of_pubkey(const uint8_t pubkey65[65], uint8_t address[20])
{
	uint8_t h[32];
	nu54_keccak256(&pubkey65[1], 64, h);
	memcpy(address, &h[12], 20);
}

static int recover_with_id(const uint8_t digest[32], const uint8_t rs[64], int recid, uint8_t address[20])
{
	/* The static context needs no allocation and is enough for parsing and recovery. */
	const secp256k1_context *ctx = secp256k1_context_static;
	secp256k1_ecdsa_recoverable_signature sig;
	secp256k1_pubkey pub;
	uint8_t raw[65];
	size_t raw_len = sizeof(raw);

	if (!secp256k1_ecdsa_recoverable_signature_parse_compact(ctx, &sig, rs, recid) ||
	    !secp256k1_ecdsa_recover(ctx, &pub, &sig, digest) ||
	    !secp256k1_ec_pubkey_serialize(ctx, raw, &raw_len, &pub, SECP256K1_EC_UNCOMPRESSED)) {
		return 0;
	}
	nu54_address_of_pubkey(raw, address);
	return 1;
}

/* 1 if s is above n / 2 (normalize reports that it had to change the signature). */
static int is_high_s(const uint8_t rs[64], int *ok)
{
	const secp256k1_context *ctx = secp256k1_context_static;
	secp256k1_ecdsa_signature sig;
	*ok = secp256k1_ecdsa_signature_parse_compact(ctx, &sig, rs);
	return *ok ? secp256k1_ecdsa_signature_normalize(ctx, NULL, &sig) : 0;
}

nu54_sig_result_t nu54_sig_recover(const uint8_t digest[32], const uint8_t *sig, size_t sig_len, uint8_t address[20])
{
	int ok;

	if (sig_len != 65) {
		return NU54_SIG_BAD_LENGTH;
	}
	if (sig[64] != 27 && sig[64] != 28) {
		return NU54_SIG_BAD_V;
	}
	if (is_high_s(sig, &ok)) {
		return NU54_SIG_HIGH_S;
	}
	if (!ok || !recover_with_id(digest, sig, sig[64] - 27, address)) {
		return NU54_SIG_INVALID;
	}
	return NU54_SIG_OK;
}

nu54_sig_result_t nu54_sig_finish(const uint8_t digest[32], const uint8_t rs[64], const uint8_t address[20], uint8_t sig[65])
{
	const secp256k1_context *ctx = secp256k1_context_static;
	secp256k1_ecdsa_signature parsed;
	uint8_t got[20];

	if (!secp256k1_ecdsa_signature_parse_compact(ctx, &parsed, rs)) {
		return NU54_SIG_INVALID;
	}
	secp256k1_ecdsa_signature_normalize(ctx, &parsed, &parsed); /* low-s */
	secp256k1_ecdsa_signature_serialize_compact(ctx, sig, &parsed);
	for (int recid = 0; recid < 2; recid++) {
		if (recover_with_id(digest, sig, recid, got) && memcmp(got, address, 20) == 0) {
			sig[64] = (uint8_t)(27 + recid);
			return NU54_SIG_OK;
		}
	}
	return NU54_SIG_INVALID;
}
