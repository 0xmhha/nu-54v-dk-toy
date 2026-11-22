#include "nu54_owner.h"

#include <string.h>

static void put64(uint8_t *output, uint64_t value)
{
	for (size_t i = 8; i; --i) {
		output[i - 1] = (uint8_t)value;
		value >>= 8;
	}
}

static bool bytes_present(const uint8_t *data, size_t length)
{
	uint8_t bits = 0;
	for (size_t i = 0; i < length; ++i) bits |= data[i];
	return bits != 0;
}

static bool management(const nu54_v2_link *link)
{
	return link && !link->closed && link->profile == NU54_V2_MANAGEMENT &&
	       link->peer == NU54_V2_OWNER && link->local == NU54_V2_BOARD;
}

void nu54_owner_cancel(nu54_owner_enrollment *enrollment)
{
	memset(enrollment, 0, sizeof(*enrollment));
}

int nu54_owner_status(const nu54_vault *vault, const nu54_owner_crypto *crypto,
		      const nu54_owner_enrollment *enrollment, uint8_t out[NU54_OWNER_STATUS_SIZE])
{
	if (!vault->enabled || crypto->hash(vault->record.identity, sizeof(vault->record.identity), out + 13)) return -1;
	out[0] = 3;
	out[1] = enrollment->proven ? 2 : vault->record.flags & NU54_VAULT_REVOKED ? 3 : vault->record.generation != 0;
	out[2] = (uint8_t)vault->record.root_format;
	out[3] = (uint8_t)vault->record.root_profile_version;
	out[4] = !!(vault->record.flags & NU54_VAULT_BACKUP);
	put64(out + 5, vault->record.generation);
	memcpy(out + 45, vault->record.owner, 32);
	return 0;
}

static int begin(nu54_owner_enrollment *enrollment, const nu54_vault *vault, const nu54_owner_crypto *crypto,
		     const nu54_v2_link *link, const nu54_v2_request *request, uint64_t now,
		     const uint8_t enrollment_peer[32], bool lost)
{
	if (!management(link) || !vault->enabled || enrollment->issued || request->command != NU54_V2_MANAGE ||
	    request->payload_length != 34 || request->payload[0] != 1 || request->expires <= now ||
	    vault->record.generation == UINT64_MAX) return -1;
	uint8_t operation = request->payload[1];
	const uint8_t *target = request->payload + 2;
	const uint8_t *peer = link->peer_identity;
	bool virgin = vault->record.generation == 0;
	bool active = !virgin && !(vault->record.flags & NU54_VAULT_REVOKED);
	if (!bytes_present(target, 32)) return -1;
	if (lost != (operation == NU54_OWNER_REPLACE_LOST)) return -1;
	switch (operation) {
	case NU54_OWNER_REGISTER:
		if (!virgin || !enrollment_peer || memcmp(peer, enrollment_peer, 32) || memcmp(peer, target, 32)) return -1;
		break;
	case NU54_OWNER_REPLACE_LOST:
		if (!active || memcmp(peer, target, 32) || !memcmp(peer, vault->record.owner, 32)) return -1;
		break;
	case NU54_OWNER_REPLACE:
	case NU54_OWNER_REVOKE:
		if (!active || memcmp(peer, vault->record.owner, 32) ||
		    ((operation == NU54_OWNER_REVOKE) != (memcmp(peer, target, 32) == 0))) return -1;
		break;
	default:
		/* A non-exportable single-key root cannot produce an independent restore proof. */
		return -1;
	}
	nu54_owner_enrollment next = {.generation = vault->record.generation, .expiry = request->expires, .issued = true};
	next.record[0] = 1;
	next.record[1] = operation;
	if (crypto->hash(vault->record.identity, sizeof(vault->record.identity), next.record + 2) ||
	    crypto->random(next.record + 74, 32) || !bytes_present(next.record + 74, 32)) return -1;
	put64(next.record + 34, next.generation);
	memcpy(next.record + 42, target, 32);
	memcpy(next.record + 106, request->id, 16);
	put64(next.record + 122, next.expiry);
	memcpy(next.epoch, link->epoch, sizeof(next.epoch));
	memcpy(next.peer, peer, sizeof(next.peer));
	*enrollment = next;
	return 0;
}

int nu54_owner_begin(nu54_owner_enrollment *enrollment, const nu54_vault *vault, const nu54_owner_crypto *crypto,
		     const nu54_v2_link *link, const nu54_v2_request *request, uint64_t now,
		     const uint8_t enrollment_peer[32])
{
	return begin(enrollment, vault, crypto, link, request, now, enrollment_peer, false);
}

int nu54_owner_begin_lost(nu54_owner_enrollment *enrollment, const nu54_vault *vault, const nu54_owner_crypto *crypto,
			  const nu54_v2_link *link, const nu54_v2_request *request, uint64_t now)
{
	return begin(enrollment, vault, crypto, link, request, now, NULL, true);
}

static bool current(const nu54_owner_enrollment *enrollment, const nu54_vault *vault,
		    const nu54_v2_link *link, uint64_t now)
{
	return management(link) && vault->enabled && enrollment->issued && now < enrollment->expiry &&
	       enrollment->generation == vault->record.generation &&
	       !memcmp(enrollment->epoch, link->epoch, sizeof(enrollment->epoch)) &&
	       !memcmp(enrollment->peer, link->peer_identity, sizeof(enrollment->peer));
}

int nu54_owner_prove(nu54_owner_enrollment *enrollment, const nu54_vault *vault, const nu54_owner_crypto *crypto,
		     const nu54_v2_link *link, const nu54_v2_request *request, uint64_t now, uint32_t prompt)
{
	if (!current(enrollment, vault, link, now) || enrollment->proven || !prompt ||
	    request->command != NU54_V2_MANAGE || request->payload_length < 134 ||
	    request->payload[0] != 2 || memcmp(request->payload + 1, enrollment->record, NU54_OWNER_CHALLENGE_SIZE)) return -1;
	size_t key_size = request->payload[131];
	size_t signature_offset = 132 + key_size;
	if (!key_size || signature_offset >= request->payload_length) return -1;
	size_t signature_size = request->payload[signature_offset];
	if (!signature_size || signature_offset + 1 + signature_size != request->payload_length) return -1;
	const uint8_t *key = request->payload + 132;
	uint8_t digest[32];
	if (crypto->hash(key, key_size, digest) || memcmp(digest, enrollment->record + 42, 32) ||
	    crypto->hash(enrollment->record, sizeof(enrollment->record), digest) ||
	    crypto->verify(key, key_size, digest, request->payload + signature_offset + 1, signature_size)) return -1;
	enrollment->proven = true;
	enrollment->prompt = prompt;
	return 0;
}

int nu54_owner_authorize_lost(nu54_owner_enrollment *enrollment, const nu54_vault *vault,
			      const nu54_v2_link *link, uint64_t now,
			      int (*verify)(const uint8_t *, size_t, const uint8_t [64]), const uint8_t proof[64])
{
	if (!current(enrollment, vault, link, now) || !enrollment->proven || enrollment->independent ||
	    enrollment->record[1] != NU54_OWNER_REPLACE_LOST || !verify || !proof) return -1;
	uint8_t message[4 + NU54_OWNER_CHALLENGE_SIZE];
	memcpy(message, "N54R", 4);
	memcpy(message + 4, enrollment->record, sizeof(enrollment->record));
	if (verify(message, sizeof(message), proof)) return -1;
	enrollment->independent = true;
	return 0;
}

int nu54_owner_confirm(nu54_owner_enrollment *enrollment, nu54_vault *vault, const nu54_v2_link *link,
		       uint64_t now, uint32_t prompt, bool approved)
{
	if (!current(enrollment, vault, link, now) || !enrollment->proven || prompt != enrollment->prompt || !approved ||
	    (enrollment->record[1] == NU54_OWNER_REPLACE_LOST && !enrollment->independent)) {
		nu54_owner_cancel(enrollment);
		return -1;
	}
	nu54_owner_enrollment accepted = *enrollment;
	nu54_owner_cancel(enrollment);
	if (accepted.record[1] == NU54_OWNER_REVOKE)
		return nu54_vault_revoke(vault, accepted.generation, accepted.peer);
	return nu54_vault_rebind(vault, accepted.generation, accepted.record + 42);
}
