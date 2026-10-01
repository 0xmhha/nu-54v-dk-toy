#include "nu54_keccak.h"

/* Defined in third_party/keccak/Keccak-readable-and-compact.c. */
void Keccak(unsigned int rate, unsigned int capacity, const unsigned char *input,
	    unsigned long long int inputByteLen, unsigned char delimitedSuffix,
	    unsigned char *output, unsigned long long int outputByteLen);

void nu54_keccak256(const uint8_t *in, size_t len, uint8_t out[32])
{
	/* rate 1088, capacity 512, Keccak padding 0x01 (FIPS 202 SHA3 would be 0x06). */
	Keccak(1088, 512, in, len, 0x01, out, 32);
}
