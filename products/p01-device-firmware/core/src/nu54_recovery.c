#include "nu54_recovery.h"
#include "nu54_sig.h"

#include <secp256k1.h>
#include <string.h>

static const uint8_t claim_prefix[] = "NU54-backup-claim-v1";
static const uint8_t receipt_prefix[] = "NU54-recovery-receipt-v1";

static void write64(uint8_t *out, uint64_t value)
{
	for (unsigned i = 8; i; --i) {
		out[i - 1] = (uint8_t)value;
		value >>= 8;
	}
}

static uint64_t read64(const uint8_t *data)
{
	uint64_t value = 0;
	for (unsigned i = 0; i < 8; ++i) value = (value << 8) | data[i];
	return value;
}

static bool present(const uint8_t *data, size_t length)
{
	uint8_t any = 0;
	for (size_t i = 0; i < length; ++i) any |= data[i];
	return any != 0;
}

static bool valid_public_account(const uint8_t *key, const uint8_t *address)
{
	secp256k1_pubkey parsed;
	if (key[0] != 4 || !secp256k1_ec_pubkey_parse(secp256k1_context_static, &parsed, key, NU54_HD_PUBLIC_SIZE))
		return false;
	uint8_t derived[NU54_HD_ADDRESS_SIZE];
	nu54_address_of_pubkey(key, derived);
	return memcmp(address, derived, sizeof(derived)) == 0;
}

void nu54_recovery_claim(uint8_t out[NU54_RECOVERY_CLAIM_SIZE], const uint8_t source[32], uint64_t generation,
			 const uint8_t key[NU54_HD_PUBLIC_SIZE], const uint8_t address[NU54_HD_ADDRESS_SIZE])
{
	memset(out, 0, NU54_RECOVERY_CLAIM_SIZE);
	memcpy(out, claim_prefix, sizeof(claim_prefix));
	out[21] = NU54_HD_FORMAT;
	out[22] = NU54_HD_PROFILE;
	memcpy(out + 23, source, 32);
	write64(out + 55, generation);
	out[63] = NU54_HD_PATH_FIRST_EVM;
	write64(out + 68, NU54_HD_CHAIN_EVM_PILOT);
	memcpy(out + 76, key, NU54_HD_PUBLIC_SIZE);
	memcpy(out + 141, address, NU54_HD_ADDRESS_SIZE);
}

int nu54_recovery_prepare(uint8_t out[NU54_RECOVERY_BODY_SIZE], const uint8_t claim[NU54_RECOVERY_CLAIM_SIZE],
			 const nu54_recovery_context *expected, uint64_t now)
{
	if (!expected || !expected->generation || !now || now >= expected->expires ||
	    !present(expected->challenge, 32) || !present(expected->request, 16) ||
	    !present(expected->target, 32) || !present(expected->owner, 32) || !present(expected->epoch, 16) ||
	    !present(claim + 23, 32) || !read64(claim + 55) || !memcmp(claim + 23, expected->target, 32) ||
	    !valid_public_account(claim + 76, claim + 141)) return -1;
	uint8_t canonical[NU54_RECOVERY_CLAIM_SIZE];
	nu54_recovery_claim(canonical, claim + 23, read64(claim + 55), claim + 76, claim + 141);
	if (memcmp(canonical, claim, sizeof(canonical))) return -1;
	memset(out, 0, NU54_RECOVERY_BODY_SIZE);
	memcpy(out, receipt_prefix, sizeof(receipt_prefix));
	memcpy(out + 25, expected->challenge, 32);
	memcpy(out + 57, expected->request, 16);
	memcpy(out + 73, claim + 21, 34);
	memcpy(out + 107, expected->target, 32);
	memcpy(out + 139, claim + 63, 98);
	memcpy(out + 237, expected->owner, 32);
	memcpy(out + 269, expected->epoch, 16);
	memcpy(out + 285, claim + 55, 8);
	write64(out + 293, expected->generation);
	out[301] = 1;
	write64(out + 302, now);
	write64(out + 310, expected->expires);
	return 0;
}

int nu54_recovery_verify(const uint8_t *data, size_t length, const nu54_recovery_context *expected, uint64_t now,
			 const nu54_recovery_signers *signers)
{
	if (!data || !signers || !signers->verify || length <= NU54_RECOVERY_BODY_SIZE ||
	    length > NU54_RECOVERY_ENVELOPE_MAX || !expected || now >= expected->expires) return -1;
	uint8_t claim[NU54_RECOVERY_CLAIM_SIZE], canonical[NU54_RECOVERY_BODY_SIZE];
	nu54_recovery_claim(claim, data + 75, read64(data + 285), data + 152, data + 217);
	uint64_t issued = read64(data + 302);
	if (issued > now || nu54_recovery_prepare(canonical, claim, expected, issued) ||
	    memcmp(canonical, data, sizeof(canonical))) return -1;
	static const enum nu54_recovery_role roles[] = {NU54_RECOVERY_TARGET, NU54_RECOVERY_ACCESSORY, NU54_RECOVERY_SOURCE};
	size_t offset = NU54_RECOVERY_BODY_SIZE;
	for (size_t i = 0; i < sizeof(roles) / sizeof(roles[0]); ++i) {
		if (offset == length) return -1;
		size_t size = data[offset++];
		if (!size || size > NU54_RECOVERY_DER_MAX || size > length - offset ||
		    signers->verify(signers->context, roles[i], i == 2 ? claim : data,
				    i == 2 ? sizeof(claim) : NU54_RECOVERY_BODY_SIZE, data + offset, size)) return -1;
		offset += size;
	}
	return offset == length ? 0 : -1;
}
