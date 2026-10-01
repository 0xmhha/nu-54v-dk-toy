/*
 * Firmware entry of the conformance harness (products/p10-platform/harness/README.md):
 * all seven EIP-712 digests, operator and merchant signer recovery, and the device's
 * signature finishing (low-s, v) against the shared vectors.
 */
#include <stdio.h>
#include <string.h>

#include "nu54_eip712.h"
#include "nu54_sig.h"
#include "secp256k1.h"
#include "vectors.h"

static int failures;
#define CHECK(cond, ...)                                  \
	do {                                              \
		if (!(cond)) {                            \
			failures++;                       \
			printf("FAIL %s:%d ", __FILE__, __LINE__); \
			printf(__VA_ARGS__);              \
			printf("\n");                     \
		}                                         \
	} while (0)

/* Foundry public test mnemonic, index 0 (device role). Test-only key, host tests only. */
static const uint8_t DEVICE_TEST_KEY[32] = {
	0xac, 0x09, 0x74, 0xbe, 0xc3, 0x9a, 0x17, 0xe3, 0x6b, 0xa4, 0xa6, 0xb4, 0xd2, 0x38, 0xff, 0x94,
	0x4b, 0xac, 0xb4, 0x78, 0xcb, 0xed, 0x5e, 0xfc, 0xae, 0x78, 0x4d, 0x7b, 0xf4, 0xf2, 0xff, 0x80};
/* Role addresses (indexes 0, 1, 2 of the same mnemonic). */
static const uint8_t ROLE_DEVICE[20] = {0xf3, 0x9f, 0xd6, 0xe5, 0x1a, 0xad, 0x88, 0xf6, 0xf4, 0xce, 0x6a, 0xb8, 0x82, 0x72, 0x79, 0xcf, 0xff, 0xb9, 0x22, 0x66};
static const uint8_t ROLE_OPERATOR[20] = {0x70, 0x99, 0x79, 0x70, 0xc5, 0x18, 0x12, 0xdc, 0x3a, 0x01, 0x0c, 0x7d, 0x01, 0xb5, 0x0e, 0x0d, 0x17, 0xdc, 0x79, 0xc8};
static const uint8_t ROLE_MERCHANT[20] = {0x3c, 0x44, 0xcd, 0xdd, 0xb6, 0xa9, 0x00, 0xfa, 0x2b, 0x58, 0x5d, 0xd2, 0x99, 0xe0, 0x3d, 0x12, 0xfa, 0x42, 0x93, 0xbc};

static void struct_hash(const eip712_vector_t *v, uint8_t out[32])
{
	if (strcmp(v->type, "PaymentAuthorization") == 0) {
		nu54_hash_payment_authorization(v->msg, out);
	} else if (strcmp(v->type, "LimitChange") == 0) {
		nu54_hash_limit_change(v->msg, out);
	} else if (strcmp(v->type, "MerchantAttestation") == 0) {
		nu54_hash_merchant_attestation(v->msg, out);
	} else if (strcmp(v->type, "MerchantOrder") == 0) {
		nu54_hash_merchant_order(v->msg, out);
	} else if (strcmp(v->type, "TimeAnchor") == 0) {
		nu54_hash_time_anchor(v->msg, out);
	} else {
		nu54_hash_device_reset(v->msg, out);
	}
}

static const uint8_t *role_address(const char *role)
{
	return strcmp(role, "device") == 0 ? ROLE_DEVICE : strcmp(role, "operator") == 0 ? ROLE_OPERATOR : ROLE_MERCHANT;
}

/* What the secure partition hands over: a raw (r, s), here deliberately in high-s form. */
static void raw_device_signature(secp256k1_context *ctx, const uint8_t digest[32], uint8_t rs_high[64])
{
	static const uint8_t n[32] = {0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xfe,
				      0xba, 0xae, 0xdc, 0xe6, 0xaf, 0x48, 0xa0, 0x3b, 0xbf, 0xd2, 0x5e, 0x8c, 0xd0, 0x36, 0x41, 0x41};
	secp256k1_ecdsa_signature sig;
	CHECK(secp256k1_ecdsa_sign(ctx, &sig, digest, DEVICE_TEST_KEY, NULL, NULL), "sign");
	secp256k1_ecdsa_signature_serialize_compact(ctx, rs_high, &sig); /* RFC 6979, low-s */
	int borrow = 0; /* s := n - s */
	for (int i = 31; i >= 0; i--) {
		int d = n[i] - rs_high[32 + i] - borrow;
		borrow = d < 0;
		rs_high[32 + i] = (uint8_t)(d + (borrow ? 256 : 0));
	}
}

int main(void)
{
	uint8_t domain[32], h[32], d[32], addr[20], sig[65], rs[64];
	secp256k1_context *ctx = secp256k1_context_create(SECP256K1_CONTEXT_NONE);

	nu54_eip712_domain_separator(VEC_CHAIN_ID, VEC_CONTRACT, domain);
	for (size_t i = 0; i < EIP712_VECTOR_COUNT; i++) {
		const eip712_vector_t *v = &EIP712_VECTORS[i];
		struct_hash(v, h);
		nu54_eip712_digest(domain, h, d);
		CHECK(memcmp(d, v->digest, 32) == 0, "%s digest", v->id);

		CHECK(nu54_sig_recover(d, v->signature, 65, addr) == NU54_SIG_OK, "%s recover", v->id);
		CHECK(memcmp(addr, v->signer, 20) == 0, "%s signer", v->id);
		CHECK(memcmp(addr, role_address(v->role), 20) == 0, "%s role %s", v->id, v->role);

		if (strcmp(v->role, "device") == 0) {
			raw_device_signature(ctx, d, rs);
			CHECK(nu54_sig_finish(d, rs, ROLE_DEVICE, sig) == NU54_SIG_OK, "%s finish", v->id);
			CHECK(memcmp(sig, v->signature, 65) == 0, "%s finished signature equals the vector", v->id);
			CHECK(nu54_sig_finish(d, rs, ROLE_OPERATOR, sig) == NU54_SIG_INVALID, "%s finish for another address", v->id);
		}
	}

	/* Refusals: length, v, high-s. */
	const eip712_vector_t *v0 = &EIP712_VECTORS[0];
	memcpy(sig, v0->signature, 65);
	CHECK(nu54_sig_recover(v0->digest, sig, 64, addr) == NU54_SIG_BAD_LENGTH, "short signature");
	sig[64] = 1;
	CHECK(nu54_sig_recover(v0->digest, sig, 65, addr) == NU54_SIG_BAD_V, "v = 1");
	raw_device_signature(ctx, v0->digest, rs);
	memcpy(sig, rs, 64);
	sig[64] = 27;
	CHECK(nu54_sig_recover(v0->digest, sig, 65, addr) == NU54_SIG_HIGH_S, "high-s");

	secp256k1_context_destroy(ctx);
	printf("%s: %zu EIP-712 vectors, %d failures\n", failures ? "FAIL" : "ok", (size_t)EIP712_VECTOR_COUNT, failures);
	return failures ? 1 : 0;
}
