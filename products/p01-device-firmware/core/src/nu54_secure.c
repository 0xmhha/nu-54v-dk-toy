#include "nu54_secure.h"

#include <string.h>

#include "secp256k1.h"
#include "secp256k1_ecdh.h"
#include "secp256k1_preallocated.h"

/* Public key creation needs a context with the generator tables; the static context only
 * parses. Preallocated, so the device needs no heap. */
static const secp256k1_context *context(void)
{
	static uint64_t mem[64]; /* secp256k1_context_preallocated_size(NONE) is far smaller */
	static secp256k1_context *ctx;
	if (!ctx && secp256k1_context_preallocated_size(SECP256K1_CONTEXT_NONE) <= sizeof(mem)) {
		ctx = secp256k1_context_preallocated_create(mem, SECP256K1_CONTEXT_NONE);
	}
	return ctx;
}

static int lift(const uint8_t x[32], secp256k1_pubkey *pub)
{
	uint8_t c[33];
	c[0] = 0x02;
	memcpy(&c[1], x, 32);
	return secp256k1_ec_pubkey_parse(secp256k1_context_static, pub, c, sizeof(c));
}

int nu54_secure_public_x(const uint8_t priv[32], uint8_t x[32])
{
	const secp256k1_context *ctx = context();
	secp256k1_pubkey pub;
	uint8_t c[33];
	size_t len = sizeof(c);

	if (!ctx || !secp256k1_ec_seckey_verify(ctx, priv) || !secp256k1_ec_pubkey_create(ctx, &pub, priv) ||
	    !secp256k1_ec_pubkey_serialize(ctx, c, &len, &pub, SECP256K1_EC_COMPRESSED)) {
		return -1;
	}
	memcpy(x, &c[1], 32);
	return 0;
}

int nu54_secure_is_point(const uint8_t x[32])
{
	secp256k1_pubkey pub;
	return lift(x, &pub);
}

/* The shared secret is the bare x coordinate (no hashing); HKDF comes after. */
static int x_only(unsigned char *out, const unsigned char *x32, const unsigned char *y32, void *data)
{
	(void)y32;
	(void)data;
	memcpy(out, x32, 32);
	return 1;
}

int nu54_secure_shared_x(const uint8_t priv[32], const uint8_t peer_x[32], uint8_t shared[32])
{
	const secp256k1_context *ctx = context();
	secp256k1_pubkey pub;
	return ctx && lift(peer_x, &pub) && secp256k1_ecdh(ctx, shared, &pub, priv, x_only, NULL) ? 0 : -1;
}
