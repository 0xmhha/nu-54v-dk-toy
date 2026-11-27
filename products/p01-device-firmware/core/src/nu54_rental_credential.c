#include "nu54_rental_credential.h"

#include <string.h>

#include "nu54_eip712.h"
#include "nu54_keccak.h"
#include "nu54_sig.h"

static void encode_u64(uint8_t bytes[8], uint64_t value)
{
	for (size_t i = 0; i < 8; ++i)
		bytes[7 - i] = (uint8_t)(value >> (8 * i));
}

static uint64_t decode_u64(const uint8_t bytes[8])
{
	uint64_t value = 0;
	for (size_t i = 0; i < 8; ++i)
		value = (value << 8) | bytes[i];
	return value;
}

static void encode(const nu54_rental_credential_t *credential, uint8_t bytes[32])
{
	encode_u64(bytes, credential->revision);
	encode_u64(bytes + 8, credential->policy);
	encode_u64(bytes + 16, credential->issued_at);
	encode_u64(bytes + 24, credential->expires_at);
}

void nu54_rental_credential_digest(const nu54_rental_binding_t *binding,
				   const nu54_rental_credential_t *credential, uint8_t digest[32])
{
	static const char type[] = "RentalSlotCredential(bytes32 binding,uint64 revision,uint64 policy,uint64 issuedAt,uint64 expiresAt)";
	uint8_t words[6][32] = {{0}}, fields[32], domain[32], data[32];
	encode(credential, fields);
	nu54_keccak256((const uint8_t *)type, sizeof(type) - 1, words[0]);
	nu54_keccak256((const uint8_t *)binding, sizeof(*binding), words[1]);
	for (size_t i = 0; i < 4; ++i)
		memcpy(words[2 + i] + 24, fields + 8 * i, 8);
	nu54_keccak256((const uint8_t *)words, sizeof(words), data);
	nu54_eip712_domain_separator(binding->chain, binding->contract, domain);
	nu54_eip712_digest(domain, data, digest);
}

static int verified(const nu54_rental_credential_store_t *store, const nu54_rental_binding_t *binding,
		    const uint8_t operator_address[20], const nu54_rental_credential_t *credential,
		    const uint8_t signature[65], uint64_t *checked_at)
{
	uint8_t digest[32], signer[20];
	uint64_t now = 0;
	if (!binding || !operator_address || !credential || !signature || !store->trusted_now || store->trusted_now(store->context, &now) || !now ||
	    !nu54_rental_journal_binding_valid(binding) ||
	    credential->revision != 1 || credential->policy != 1 || !credential->issued_at ||
	    credential->issued_at > now || now >= credential->expires_at) return -1;
	nu54_rental_credential_digest(binding, credential, digest);
	if (nu54_sig_recover(digest, signature, 65, signer) != NU54_SIG_OK ||
	    memcmp(signer, operator_address, sizeof(signer))) return -1;
	if (checked_at) *checked_at = now;
	return 0;
}

int nu54_rental_credential_install(const nu54_rental_credential_store_t *store,
				   const nu54_rental_binding_t *binding, const uint8_t operator_address[20],
				   const nu54_rental_credential_t *credential, const uint8_t signature[65])
{
	uint8_t record[NU54_RENTAL_CREDENTIAL_SIZE], existing[sizeof(record)];
	if (!store || !store->read ||
	    verified(store, binding, operator_address, credential, signature, NULL)) return -1;
	encode(credential, record);
	memcpy(record + 32, signature, 65);
	int status = store->read(store->context, existing, sizeof(existing));
	if (status == 0) return memcmp(existing, record, sizeof(record)) ? -1 : 0;
	if (status != 1 || !store->create || !store->may_issue ||
	    store->may_issue(store->context, binding) != 1) return -1;
	return store->create(store->context, record, sizeof(record));
}

int nu54_rental_credential_current(const nu54_rental_credential_store_t *store,
				   const nu54_rental_binding_t *binding, const uint8_t operator_address[20],
				   nu54_rental_credential_t *credential, uint64_t *checked_at)
{
	uint8_t record[NU54_RENTAL_CREDENTIAL_SIZE];
	if (!credential || !store || !store->read || store->read(store->context, record, sizeof(record))) return -1;
	const nu54_rental_credential_t current = {decode_u64(record), decode_u64(record + 8),
						 decode_u64(record + 16), decode_u64(record + 24)};
	if (verified(store, binding, operator_address, &current, record + 32, checked_at)) return -1;
	*credential = current;
	return 0;
}

static void receipt_digest(const nu54_rental_binding_t *binding, const nu54_rental_credential_receipt_t *receipt,
			   const char *type, uint8_t digest[32])
{
	uint8_t words[4][32] = {{0}}, domain[32], data[32];
	nu54_keccak256((const uint8_t *)type, strlen(type), words[0]);
	nu54_rental_credential_digest(binding, &receipt->credential, words[1]);
	memcpy(words[2], receipt->challenge, sizeof(receipt->challenge));
	encode_u64(words[3] + 24, receipt->checked_at);
	nu54_keccak256((const uint8_t *)words, sizeof(words), data);
	nu54_eip712_domain_separator(binding->chain, binding->contract, domain);
	nu54_eip712_digest(domain, data, digest);
}

void nu54_rental_credential_receipt_digest(const nu54_rental_binding_t *binding,
					   const nu54_rental_credential_receipt_t *receipt, uint8_t digest[32])
{
	receipt_digest(binding, receipt, "RentalSlotCredentialReceipt(bytes32 credential,bytes32 challenge,uint64 checkedAt)", digest);
}

void nu54_rental_slot_open_receipt_digest(const nu54_rental_binding_t *binding,
					 const nu54_rental_credential_receipt_t *receipt, uint8_t digest[32])
{
	receipt_digest(binding, receipt, "RentalSlotOpenReceipt(bytes32 credential,bytes32 challenge,uint64 checkedAt)", digest);
}
