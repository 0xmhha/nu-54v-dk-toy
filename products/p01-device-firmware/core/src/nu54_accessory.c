#include "nu54_accessory.h"

#include <string.h>

static uint64_t read64(const uint8_t *bytes)
{
	uint64_t value = 0;
	for (size_t i = 0; i < 8; ++i) value = (value << 8) | bytes[i];
	return value;
}

static void write64(uint8_t *bytes, uint64_t value)
{
	for (size_t i = 8; i; --i) {
		bytes[i - 1] = (uint8_t)value;
		value >>= 8;
	}
}

void nu54_recovery_open_message(const nu54_recovery_context *request, uint8_t out[NU54_RECOVERY_OPEN_SIZE])
{
	out[0] = 5;
	memcpy(out + 1, request->challenge, 32);
	memcpy(out + 33, request->request, 16);
	memcpy(out + 49, request->target, 32);
	memcpy(out + 81, request->owner, 32);
	memcpy(out + 113, request->epoch, 16);
	write64(out + 129, request->generation);
	write64(out + 137, request->expires);
}

int nu54_recovery_parse_open(const uint8_t *data, size_t length, nu54_recovery_context *request)
{
	if (length != NU54_RECOVERY_OPEN_SIZE || data[0] != 5) return -1;
	memcpy(request->challenge, data + 1, 32);
	memcpy(request->request, data + 33, 16);
	memcpy(request->target, data + 49, 32);
	memcpy(request->owner, data + 81, 32);
	memcpy(request->epoch, data + 113, 16);
	request->generation = read64(data + 129);
	request->expires = read64(data + 137);
	return 0;
}

/* Capsule = type3 || original claim || one-byte sig length || signature || raw seed.
 * The seed traverses only authenticated TLS and the protected participant's sealed store. */
static int capsule_parts(const uint8_t *capsule, size_t length, const uint8_t **claim, const uint8_t **sig,
			 size_t *sig_len, const uint8_t **seed)
{
	const size_t offset = NU54_RECOVERY_CLAIM_SIZE + 1;
	if (length <= offset + 1 + NU54_HD_SEED_SIZE || length > NU54_ACCESSORY_CAPSULE_MAX || capsule[0] != 3)
		return -1;
	*sig_len = capsule[offset];
	if (!*sig_len || *sig_len > NU54_RECOVERY_DER_MAX || length != offset + 1 + *sig_len + NU54_HD_SEED_SIZE)
		return -1;
	*claim = capsule + 1;
	*sig = capsule + offset + 1;
	*seed = *sig + *sig_len;
	return 0;
}

int nu54_accessory_target_proof(const nu54_hd_slot *slot, const uint8_t *capsule, size_t capsule_length,
				const nu54_recovery_context *expected, uint64_t now, const nu54_recovery_signers *signers,
				uint8_t output[NU54_RECOVERY_ENVELOPE_MAX], size_t *length)
{
	const uint8_t *claim, *sig, *seed;
	size_t size;
	if (!slot->enabled || (slot->record.phase != NU54_HD_EMPTY && slot->record.phase != NU54_HD_RESTORING &&
	    slot->record.phase != NU54_HD_READY) ||
	    capsule_parts(capsule, capsule_length, &claim, &sig, &size, &seed) ||
	    nu54_recovery_prepare(output, claim, expected, now) ||
	    signers->verify(signers->context, NU54_RECOVERY_SOURCE, claim, NU54_RECOVERY_CLAIM_SIZE, sig, size))
		return -1;
	uint8_t key[65], address[20];
	if (slot->backend.preview(slot->backend.context, seed, NU54_HD_SEED_SIZE, key, address) ||
	    memcmp(key, claim + 76, sizeof(key)) || memcmp(address, claim + 141, sizeof(address))) return -1;
	if (slot->record.phase != NU54_HD_EMPTY &&
	    (memcmp(slot->record.origin, claim + 23, sizeof(slot->record.origin)) ||
	     memcmp(slot->record.public_key, key, sizeof(key)) || memcmp(slot->record.address, address, sizeof(address))))
		return -1;
	size = NU54_RECOVERY_DER_MAX;
	if (signers->sign(signers->context, NU54_RECOVERY_TARGET, output, NU54_RECOVERY_BODY_SIZE,
			   output + NU54_RECOVERY_BODY_SIZE + 1, &size) || !size || size > NU54_RECOVERY_DER_MAX)
		return -1;
	output[NU54_RECOVERY_BODY_SIZE] = (uint8_t)size;
	*length = NU54_RECOVERY_BODY_SIZE + 1 + size;
	return 0;
}

int nu54_accessory_complete(const uint8_t *capsule, size_t capsule_length, const uint8_t *partial, size_t partial_length,
			    const nu54_recovery_context *expected, uint64_t now, const nu54_recovery_signers *signers,
			    uint8_t output[NU54_RECOVERY_ENVELOPE_MAX], size_t *length)
{
	const uint8_t *claim, *sig, *seed;
	size_t size;
	uint8_t canonical[NU54_RECOVERY_BODY_SIZE];
	if (partial_length <= NU54_RECOVERY_BODY_SIZE + 1 ||
	    partial_length > NU54_RECOVERY_BODY_SIZE + 1 + NU54_RECOVERY_DER_MAX ||
	    partial[NU54_RECOVERY_BODY_SIZE] != partial_length - NU54_RECOVERY_BODY_SIZE - 1 ||
	    capsule_parts(capsule, capsule_length, &claim, &sig, &size, &seed) ||
	    read64(partial + 302) > now || now >= expected->expires ||
	    nu54_recovery_prepare(canonical, claim, expected, read64(partial + 302)) ||
	    memcmp(canonical, partial, sizeof(canonical)) ||
	    signers->verify(signers->context, NU54_RECOVERY_SOURCE, claim, NU54_RECOVERY_CLAIM_SIZE, sig, size) ||
	    signers->verify(signers->context, NU54_RECOVERY_TARGET, partial, NU54_RECOVERY_BODY_SIZE,
			    partial + NU54_RECOVERY_BODY_SIZE + 1, partial[NU54_RECOVERY_BODY_SIZE])) return -1;
	memmove(output, partial, partial_length);
	size_t access_size = NU54_RECOVERY_DER_MAX;
	if (signers->sign(signers->context, NU54_RECOVERY_ACCESSORY, output, NU54_RECOVERY_BODY_SIZE,
			   output + partial_length + 1, &access_size) || !access_size || access_size > NU54_RECOVERY_DER_MAX)
		return -1;
	output[partial_length++] = (uint8_t)access_size;
	partial_length += access_size;
	output[partial_length++] = (uint8_t)size;
	memcpy(output + partial_length, sig, size);
	*length = partial_length + size;
	return 0;
}
