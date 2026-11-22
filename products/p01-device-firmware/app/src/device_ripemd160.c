/* The pinned NCS tf-psa-crypto module supplies this implementation.
 * Oberon does not expose RIPEMD-160 through its PSA driver. Enable only
 * this upstream translation unit for Bitcoin/XRPL public key hashing. */
#include "tf_psa_crypto_common.h"
#define MBEDTLS_RIPEMD160_C
#define MBEDTLS_DECLARE_PRIVATE_IDENTIFIERS
#include "ripemd160.c"

int device_ripemd160(const uint8_t *bytes, size_t size, uint8_t output[20])
{
	return mbedtls_ripemd160(bytes, size, output);
}
