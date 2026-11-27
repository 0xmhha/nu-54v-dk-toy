#include "nu54_rental_registration.h"

#include <string.h>

#include "nu54_eip712.h"
#include "nu54_keccak.h"
#include "nu54_sig.h"

void nu54_rental_registration_digest(const nu54_rental_registration_t *registration, uint8_t digest[32])
{
	static const char type[] = "RentalGeneration(bytes32 binding,uint256 firstNonce,bytes32 challenge,uint64 expiresAt)";
	uint8_t words[5][32] = {{0}};
	uint8_t data[32], domain[32];
	nu54_keccak256((const uint8_t *)type, sizeof(type) - 1, words[0]);
	nu54_keccak256((const uint8_t *)&registration->binding, sizeof(registration->binding), words[1]);
	memcpy(words[2], registration->first, sizeof(registration->first));
	memcpy(words[3], registration->challenge, sizeof(registration->challenge));
	for (unsigned i = 0; i < 8; ++i) words[4][31 - i] = (uint8_t)(registration->expires_at >> (8 * i));
	nu54_keccak256((const uint8_t *)words, sizeof(words), data);
	nu54_eip712_domain_separator(registration->binding.chain, registration->binding.contract, domain);
	nu54_eip712_digest(domain, data, digest);
}

int nu54_rental_registration_verify(const nu54_rental_registration_t *registration,
				   const nu54_rental_registration_trust_t *trust, const uint8_t signature[65])
{
	static const uint8_t empty[32] = {0};
	if (!registration || !trust || !signature || !trust->now ||
	    registration->expires_at <= trust->now ||
	    registration->expires_at - trust->now > NU54_RENTAL_REGISTRATION_LIFETIME ||
	    registration->first[31] != 0 || !memcmp(trust->challenge, empty, sizeof(empty)) ||
	    !memcmp(trust->operator_address, empty, sizeof(trust->operator_address)) ||
	    memcmp(&registration->binding, &trust->binding, sizeof(trust->binding)) ||
	    memcmp(registration->first, trust->first, sizeof(trust->first)) ||
	    memcmp(registration->challenge, trust->challenge, sizeof(trust->challenge))) return -1;

	uint8_t digest[32], signer[20];
	nu54_rental_registration_digest(registration, digest);
	return nu54_sig_recover(digest, signature, 65, signer) == NU54_SIG_OK &&
	       !memcmp(signer, trust->operator_address, sizeof(signer)) ? 0 : -1;
}
