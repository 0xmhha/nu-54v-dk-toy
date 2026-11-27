#include "nu54_hd.h"
#include "nu54_sig.h"

#include <secp256k1.h>
#include <string.h>

static void erase(void *buffer, size_t length)
{
	volatile uint8_t *bytes = buffer;
	while (length--) *bytes++ = 0;
}

typedef int (*bip32_operation)(const secp256k1_context *, const uint8_t [64], void *);

static int bip32(const uint8_t seed[NU54_HD_SEED_SIZE], const uint8_t blinding[32], nu54_hd_hmac hmac,
		  const uint32_t *path, size_t depth, bip32_operation operate, void *args)
{
	static const uint8_t master_key[] = "Bitcoin seed";
	secp256k1_context *context = NULL;
	uint8_t node[64] = {0}, child[64] = {0}, data[37] = {0};
	secp256k1_pubkey point;
	int result = -1;
	if (!seed || !blinding || !hmac || !path || depth > NU54_HD_BIP32_MAX_DEPTH ||
	    !(context = secp256k1_context_create(SECP256K1_CONTEXT_NONE)) ||
	    !secp256k1_context_randomize(context, blinding) ||
	    hmac(master_key, sizeof(master_key) - 1, seed, NU54_HD_SEED_SIZE, node) ||
	    !secp256k1_ec_seckey_verify(context, node)) goto done;
	for (size_t i = 0; i < depth; ++i) {
		uint32_t index = path[i];
		if (index & UINT32_C(0x80000000)) {
			data[0] = 0;
			memcpy(data + 1, node, 32);
		} else {
			size_t length = 33;
			if (!secp256k1_ec_pubkey_create(context, &point, node) ||
			    !secp256k1_ec_pubkey_serialize(context, data, &length, &point, SECP256K1_EC_COMPRESSED))
				goto done;
		}
		for (size_t byte = 0; byte < 4; ++byte) data[33 + byte] = (uint8_t)(index >> (24 - 8 * byte));
		if (hmac(node + 32, 32, data, sizeof(data), child) ||
		    !secp256k1_ec_seckey_tweak_add(context, node, child)) goto done;
		memcpy(node + 32, child + 32, 32);
	}
	result = operate(context, node, args);
done:
	if (context) secp256k1_context_destroy(context);
	erase(node, sizeof(node));
	erase(child, sizeof(child));
	erase(data, sizeof(data));
	return result;
}

typedef struct {
	uint8_t *public_key, *address;
	const uint8_t *digest;
	uint8_t *signature;
} evm_operation;

static int evm_key(const secp256k1_context *context, const uint8_t node[64], void *opaque)
{
	evm_operation *args = opaque;
	secp256k1_pubkey point;
	size_t length = NU54_HD_PUBLIC_SIZE;
	if (!secp256k1_ec_pubkey_create(context, &point, node) ||
	    !secp256k1_ec_pubkey_serialize(context, args->public_key, &length, &point, SECP256K1_EC_UNCOMPRESSED))
		return -1;
	uint8_t actual[20];
	nu54_address_of_pubkey(args->public_key, actual);
	if (!args->digest) {
		memcpy(args->address, actual, sizeof(actual));
		return 0;
	}
	secp256k1_ecdsa_signature signature;
	return memcmp(actual, args->address, sizeof(actual)) ||
	       !secp256k1_ecdsa_sign(context, &signature, args->digest, node, NULL, NULL) ||
	       !secp256k1_ecdsa_signature_serialize_compact(context, args->signature, &signature) ? -1 : 0;
}

static int first_evm(const uint8_t seed[NU54_HD_SEED_SIZE], const uint8_t blinding[32], nu54_hd_hmac hmac,
		     uint8_t public_key[NU54_HD_PUBLIC_SIZE], uint8_t address[20], const uint8_t *digest, uint8_t *signature)
{
	static const uint32_t path[] = {0x8000002c, 0x8000003c, 0x80000000, 0, 0};
	if (!public_key || !address) return -1;
	evm_operation args = {public_key, address, digest, signature};
	return bip32(seed, blinding, hmac, path, sizeof(path) / sizeof(path[0]), evm_key, &args);
}

int nu54_hd_first_evm(const uint8_t seed[NU54_HD_SEED_SIZE], const uint8_t blinding[32], nu54_hd_hmac hmac,
		      uint8_t public_key[NU54_HD_PUBLIC_SIZE], uint8_t address[20])
{
	return first_evm(seed, blinding, hmac, public_key, address, NULL, NULL);
}

int nu54_hd_sign_first_evm(const uint8_t seed[NU54_HD_SEED_SIZE], const uint8_t blinding[32], nu54_hd_hmac hmac,
			   const uint8_t address[20], const uint8_t digest[32], uint8_t signature[64])
{
	if (!address || !digest || !signature) return -1;
	uint8_t public_key[NU54_HD_PUBLIC_SIZE], expected[20];
	memcpy(expected, address, sizeof(expected));
	return first_evm(seed, blinding, hmac, public_key, expected, digest, signature);
}

static int market_evm(const uint8_t seed[32], const uint8_t blind[32], nu54_hd_hmac hmac,
		      uint8_t address[20], const uint8_t *digest, uint8_t *signature)
{
	static const uint32_t path[] = {0x8000002c, 0x8000003c, 0x8000205b, 0, 0};
	uint8_t public_key[NU54_HD_PUBLIC_SIZE];
	evm_operation args = {public_key, address, digest, signature};
	return bip32(seed, blind, hmac, path, sizeof(path) / sizeof(path[0]), evm_key, &args);
}

int nu54_hd_market_evm(const uint8_t seed[32], const uint8_t blind[32], nu54_hd_hmac hmac, uint8_t address[20])
{
	return address ? market_evm(seed, blind, hmac, address, NULL, NULL) : -1;
}

int nu54_hd_sign_market_evm(const uint8_t seed[32], const uint8_t blind[32], nu54_hd_hmac hmac,
			   const uint8_t address[20], const uint8_t digest[32], uint8_t signature[64])
{
	if (!address || !digest || !signature) return -1;
	uint8_t expected[20];
	memcpy(expected,address,sizeof(expected));
	return market_evm(seed,blind,hmac,expected,digest,signature);
}

typedef struct {
	uint8_t *public_key, *chain_code;
	const uint8_t *expected, *digest;
	uint8_t *signature;
} compressed_operation;

static int compressed_key(const secp256k1_context *context, const uint8_t node[64], void *opaque)
{
	compressed_operation *args = opaque;
	secp256k1_pubkey point;
	uint8_t public_key[33];
	size_t length = sizeof(public_key);
	if (!secp256k1_ec_pubkey_create(context, &point, node) ||
	    !secp256k1_ec_pubkey_serialize(context, public_key, &length, &point, SECP256K1_EC_COMPRESSED))
		return -1;
	if (args->digest) {
		secp256k1_ecdsa_signature signature;
		return memcmp(public_key, args->expected, sizeof(public_key)) ||
		       !secp256k1_ecdsa_sign(context, &signature, args->digest, node, NULL, NULL) ||
		       !secp256k1_ecdsa_signature_serialize_compact(context, args->signature, &signature) ? -1 : 0;
	}
	memcpy(args->public_key, public_key, sizeof(public_key));
	if (args->chain_code) memcpy(args->chain_code, node + 32, 32);
	return 0;
}

int nu54_hd_bip32_public(const uint8_t seed[NU54_HD_SEED_SIZE], const uint8_t blinding[32], nu54_hd_hmac hmac,
			const uint32_t *path, size_t depth, uint8_t public_key[33], uint8_t *chain_code)
{
	if (!public_key) return -1;
	compressed_operation args = {.public_key = public_key, .chain_code = chain_code};
	return bip32(seed, blinding, hmac, path, depth, compressed_key, &args);
}

int nu54_hd_bip32_sign(const uint8_t seed[NU54_HD_SEED_SIZE], const uint8_t blinding[32], nu54_hd_hmac hmac,
		      const uint32_t *path, size_t depth, const uint8_t expected[33], const uint8_t digest[32],
		      uint8_t signature[64])
{
	if (!expected || !digest || !signature) return -1;
	compressed_operation args = {.expected = expected, .digest = digest, .signature = signature};
	return bip32(seed, blinding, hmac, path, depth, compressed_key, &args);
}
