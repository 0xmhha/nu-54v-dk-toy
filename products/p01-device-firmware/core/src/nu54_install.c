#include "nu54_install.h"

#include <string.h>

enum nu54_install_boot_action nu54_install_boot(unsigned found)
{
	const unsigned keys = NU54_INSTALL_FOUND_ID | NU54_INSTALL_FOUND_PAYER;
	const unsigned prepared = NU54_INSTALL_FOUND_MARKER | NU54_INSTALL_FOUND_REPLAY;
	if (!found) return NU54_INSTALL_BOOT_CREATE;
	if (found == NU54_INSTALL_FOUND_MARKER) return NU54_INSTALL_BOOT_CREATE_REPLAY;
	if ((found & prepared) != prepared) return NU54_INSTALL_BOOT_DENY;
	if (!(found & NU54_INSTALL_FOUND_KEYS))
		return (found & ~(prepared | keys)) ? NU54_INSTALL_BOOT_DENY : NU54_INSTALL_BOOT_CREATE_KEYS;
	if ((found & keys) != keys || ((found & NU54_INSTALL_FOUND_COMMIT) && !(found & NU54_INSTALL_FOUND_FLOOR)))
		return NU54_INSTALL_BOOT_DENY;
	return ((found & NU54_INSTALL_FOUND_FLOOR) && !(found & NU54_INSTALL_FOUND_COMMIT)) ?
	       NU54_INSTALL_BOOT_FINISH : NU54_INSTALL_BOOT_READY;
}

static uint64_t integer(const uint8_t *bytes, size_t length)
{
	uint64_t value = 0;
	for (size_t i = 0; i < length; ++i) value = (value << 8) | bytes[i];
	return value;
}

static bool nonzero(const uint8_t *bytes, size_t length)
{
	uint8_t value = 0;
	for (size_t i = 0; i < length; ++i) value |= bytes[i];
	return value != 0;
}

int nu54_install_bootstrap_ticket(const uint8_t *ticket, size_t length,
		const uint8_t public_value[NU54_INSTALL_BOOTSTRAP_PUBLIC_SIZE], nu54_install_verify verify,
		const uint8_t **certificate, size_t *certificate_length)
{
	static const uint8_t domain[] = "NU54IT01";
	enum { header = sizeof(domain) - 1 + NU54_INSTALL_BOOTSTRAP_PUBLIC_SIZE + 2 };
	if (!ticket || !public_value || !verify || !certificate || !certificate_length ||
	    length <= header + NU54_INSTALL_SIGNATURE_SIZE || length > NU54_INSTALL_BOOTSTRAP_TICKET_MAX ||
	    public_value[0] != NU54_INSTALL_BOOTSTRAP_VERSION || !nonzero(public_value + 1, NU54_INSTALL_CHALLENGE_SIZE) ||
	    memcmp(ticket, domain, sizeof(domain) - 1) ||
	    memcmp(ticket + sizeof(domain) - 1, public_value, NU54_INSTALL_BOOTSTRAP_PUBLIC_SIZE)) return -1;
	size_t body = length - NU54_INSTALL_SIGNATURE_SIZE;
	if (integer(ticket + header - 2, 2) != body - header || verify(ticket, body, ticket + body)) return -1;
	*certificate = ticket + header;
	*certificate_length = body - header;
	return 0;
}

int nu54_install_bootstrap_offer_digest(const uint8_t offer[NU54_INSTALL_OFFER_SIZE],
		const uint8_t exporter[32], nu54_install_hash hash, uint8_t output[32])
{
	uint8_t statement[8 + 32 + 32] = "NU54IB01";
	memcpy(statement + 8, exporter, 32);
	return hash(offer, NU54_INSTALL_OFFER_SIZE, statement + 40) || hash(statement, sizeof(statement), output) ? -1 : 0;
}

int nu54_install_accept(const uint8_t *bundle, size_t length, const uint8_t challenge[32],
			const uint8_t identity[65], const uint8_t payer[20], uint64_t minimum_time,
			const nu54_install_floor *previous, nu54_install_hash hash, nu54_install_verify verify,
			nu54_install_floor *floor, nu54_install_policy *policy, size_t *ca_length, size_t *board_length)
{
	if (!bundle || length < NU54_INSTALL_HEADER_SIZE + NU54_INSTALL_SIGNATURE_SIZE ||
	    length > NU54_INSTALL_BUNDLE_MAX || (memcmp(bundle, "N54P", 4) && memcmp(bundle, "N54Q", 4)) ||
	    memcmp(bundle + 4, challenge, 32) || memcmp(bundle + 36, identity, 65) ||
	    memcmp(bundle + 101, payer, 20)) return -1;
	const uint8_t *raw = bundle + 129;
	const uint8_t *sizes = raw + NU54_INSTALL_POLICY_SIZE;
	size_t ca = integer(sizes, 2), board = integer(sizes + 2, 2);
	if (!ca || !board || length != NU54_INSTALL_HEADER_SIZE + ca + board + NU54_INSTALL_SIGNATURE_SIZE ||
	    verify(bundle, length - NU54_INSTALL_SIGNATURE_SIZE, bundle + length - NU54_INSTALL_SIGNATURE_SIZE))
		return -1;

	nu54_install_floor candidate = {.revision = integer(raw, 8), .time = integer(bundle + 121, 8),
		.generation = integer(raw + 8, 8)};
	uint64_t end = integer(raw + 16, 8);
	bool commercial_testnet = !memcmp(bundle, "N54Q", 4);
	if (!candidate.revision || candidate.revision < previous->revision ||
	    candidate.generation < previous->generation || candidate.time < previous->time || candidate.time < minimum_time ||
	    end <= candidate.time || end - candidate.time > NU54_INSTALL_MAX_LEASE ||
	    hash(raw, length - 129 - NU54_INSTALL_SIGNATURE_SIZE, candidate.digest))
		return -1;
	if (commercial_testnet) {
		uint8_t domain_digest[36];
		memcpy(domain_digest, bundle, 4);
		memcpy(domain_digest + 4, candidate.digest, sizeof(candidate.digest));
		if (hash(domain_digest, sizeof(domain_digest), candidate.digest)) return -1;
	}
	if (candidate.revision == previous->revision && memcmp(candidate.digest, previous->digest, 32))
		return -1;

	nu54_install_policy decoded = {
		.revision = candidate.revision, .payer_generation = integer(raw + 8, 8), .not_after = end,
		.merchant_key_version = integer(raw + 24, 8), .beneficiary_version = integer(raw + 32, 8),
		.allowlist.max_amount = integer(raw + 40, 8), .allowlist.commercial_testnet = commercial_testnet,
		.payer_role = raw[48]
	};
	const uint8_t *fields = raw + 49;
#define COPY_FIELD(field) do { \
		memcpy(decoded.field, fields, sizeof(decoded.field)); \
		fields += sizeof(decoded.field); \
	} while (0)
	COPY_FIELD(allowlist.contract);
	COPY_FIELD(allowlist.token);
	COPY_FIELD(allowlist.beneficiary);
	COPY_FIELD(merchant_signer);
	COPY_FIELD(merchant);
	COPY_FIELD(owner);
	COPY_FIELD(terminal);
	COPY_FIELD(display);
	COPY_FIELD(recovery);
#undef COPY_FIELD
	if (!decoded.payer_generation || !decoded.merchant_key_version || !decoded.beneficiary_version ||
	    !decoded.allowlist.max_amount ||
	    decoded.allowlist.max_amount > (commercial_testnet ? NU54_PILOT_TESTUSDC2_CAP : NU54_PILOT_LEGACY_CAP) ||
	    (decoded.payer_role != NU54_V2_OWNER_ROLE && decoded.payer_role != NU54_V2_RENTER_ROLE) ||
	    !nonzero(decoded.allowlist.contract, 20) || !nonzero(decoded.allowlist.token, 20) ||
	    !nonzero(decoded.allowlist.beneficiary, 20) || !nonzero(decoded.merchant_signer, 20) ||
	    !nonzero(decoded.merchant, 32) ||
	    !nonzero(decoded.owner, 32) || !nonzero(decoded.terminal, 32) ||
	    !memcmp(decoded.owner, decoded.terminal, 32) || decoded.display[0] != 4 ||
	    decoded.recovery[0] != 4 || !memcmp(decoded.display, decoded.recovery, 65) ||
	    (previous->revision && memcmp(previous->recovery, decoded.recovery, 65)))
		return -1;
	memcpy(candidate.recovery, decoded.recovery, sizeof(candidate.recovery));
	*floor = candidate;
	*policy = decoded;
	*ca_length = ca;
	*board_length = board;
	return 0;
}

int nu54_install_restore(const uint8_t *bundle, size_t length, const uint8_t identity[65],
			 const uint8_t payer[20], const nu54_install_floor *committed, nu54_install_hash hash,
			 nu54_install_verify verify, nu54_install_policy *policy, size_t *ca_length, size_t *board_length)
{
	nu54_install_floor stored;
	if (!bundle || !committed || !committed->revision ||
	    length < NU54_INSTALL_HEADER_SIZE + NU54_INSTALL_SIGNATURE_SIZE ||
	    nu54_install_accept(bundle, length, bundle + 4, identity, payer, 0, committed, hash, verify, &stored,
				policy, ca_length, board_length)) return -1;
	return stored.revision == committed->revision && stored.time == committed->time &&
	       stored.generation == committed->generation && !memcmp(stored.digest, committed->digest, 32) &&
	       !memcmp(stored.recovery, committed->recovery, 65) ? 0 : -1;
}

bool nu54_install_generation_current(uint64_t grant_generation, uint64_t board_generation, bool no_owner)
{
	if (!grant_generation) return false;
	if (!no_owner) return grant_generation == board_generation;
	return board_generation != UINT64_MAX && grant_generation == board_generation + 1;
}

uint64_t nu54_install_resume(const nu54_install_clock *saved, uint64_t floor_time, uint64_t replay_floor)
{
	uint64_t time = floor_time > replay_floor ? floor_time : replay_floor;
	return saved && saved->earliest > time ? saved->earliest : time;
}

int64_t nu54_install_clock_wait_ms(uint64_t latest, uint64_t not_after, uint32_t interval_seconds)
{
	uint64_t seconds = interval_seconds ? interval_seconds : 1;
	if (latest < not_after && not_after - latest < seconds) seconds = not_after - latest;
	else if (latest >= not_after) seconds = 1;
	return (int64_t)seconds * 1000;
}

static void put_integer(uint8_t *out, size_t length, uint64_t value)
{
	while (length) {
		out[--length] = (uint8_t)value;
		value >>= 8;
	}
}

int nu54_install_clock_set(const nu54_install_clock *current, uint64_t not_after, uint64_t time,
			   uint64_t expires, uint64_t session_seconds, nu54_install_clock *next)
{
	if (!current || !next || !current->earliest || !time || expires <= time ||
	    expires - time > NU54_PILOT_BRIEF_SECONDS) return -1;
	if (time <= current->earliest) return NU54_CLOCK_NOT_FORWARD;
	if (time >= not_after) return NU54_CLOCK_PAST_END;
	const uint64_t latest = current->offset > UINT64_MAX - current->earliest ? UINT64_MAX :
				current->earliest + current->offset;
	uint64_t offset = session_seconds > UINT64_MAX - NU54_CLOCK_PHONE_SECONDS ? UINT64_MAX :
			  session_seconds + NU54_CLOCK_PHONE_SECONDS;
	if (latest > time && latest - time > offset) offset = latest - time;
	const bool later_day = time / 86400u > current->earliest / 86400u;
	*next = (nu54_install_clock){.earliest = time, .offset = offset,
		.phone_days = current->phone_days + (later_day && current->phone_days != UINT64_MAX)};
	return NU54_CLOCK_MOVED;
}

void nu54_install_clock_reply(int result, const nu54_install_clock *clock, uint64_t not_after,
			      uint8_t out[NU54_CLOCK_SET_REPLY_SIZE])
{
	const uint64_t latest = clock->offset > UINT64_MAX - clock->earliest ? UINT64_MAX : clock->earliest + clock->offset;
	out[0] = NU54_CLOCK_SET_OP;
	out[1] = (uint8_t)result;
	put_integer(out + 2, 8, clock->earliest);
	put_integer(out + 10, 8, latest);
	put_integer(out + 18, 8, not_after);
	put_integer(out + 26, 4, clock->phone_days > UINT32_MAX ? UINT32_MAX : clock->phone_days);
}

int nu54_install_certificate_window(uint64_t from, uint64_t until, uint64_t now)
{
	if (!now || now >= until || from >= until) return NU54_CERTIFICATE_REFUSED;
	return from <= now ? NU54_CERTIFICATE_CURRENT : NU54_CERTIFICATE_EARLY;
}

bool nu54_install_session_admits(bool clock_only, nu54_v2_command command)
{
	return !clock_only || command == NU54_V2_CLOCK;
}

int nu54_install_quote(const nu54_install_policy *policy, const uint8_t *payload, size_t length,
		       uint64_t now, nu54_v2_quote_policy *order)
{
	if (!payload || length != NU54_V2_SIGNED_SIZE || !now || now >= policy->not_after ||
	    memcmp(payload + NU54_V2_MERCHANT_ID_OFFSET, policy->merchant, 32) ||
	    payload[NU54_V2_PAYER_ROLE_OFFSET + 31] != policy->payer_role ||
	    integer(payload + NU54_V2_MERCHANT_KEY_VERSION_OFFSET + 24, 8) != policy->merchant_key_version ||
	    integer(payload + NU54_V2_BENEFICIARY_VERSION_OFFSET + 24, 8) != policy->beneficiary_version ||
	    integer(payload + NU54_V2_EXPIRES_AT_OFFSET + 24, 8) > policy->not_after)
		return -1;
	/* The issuer authenticates the dynamic fields. Independently pinned policy constrains their scope. */
	*order = (nu54_v2_quote_policy){.now = now};
	memcpy(order->merchant_signer, policy->merchant_signer, 20);
	memcpy(order->expected, payload, sizeof(order->expected));
	return 0;
}
