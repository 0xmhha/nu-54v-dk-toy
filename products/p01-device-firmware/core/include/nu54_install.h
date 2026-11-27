#ifndef NU54_INSTALL_H
#define NU54_INSTALL_H

#include "nu54_pilot.h"

#define NU54_INSTALL_CHALLENGE_SIZE 32
#define NU54_INSTALL_PUBLIC_SIZE 65
#define NU54_INSTALL_SIGNATURE_SIZE 64
/* The longest a grant may run, from its own time to its end: 31 days. The operator writes the
 * actual end into the grant. */
#define NU54_INSTALL_MAX_LEASE (31 * 86400)
#define NU54_INSTALL_POLICY_SIZE 355
#define NU54_INSTALL_HEADER_SIZE (4 + 32 + 65 + 20 + 8 + NU54_INSTALL_POLICY_SIZE + 4)
#define NU54_INSTALL_BUNDLE_MAX 1600
#define NU54_INSTALL_BOOTSTRAP_VERSION 1
#define NU54_INSTALL_BOOTSTRAP_PUBLIC_SIZE (1 + NU54_INSTALL_CHALLENGE_SIZE)
#define NU54_INSTALL_BOOTSTRAP_TICKET_MAX 768
#define NU54_INSTALL_OFFER_SIZE (4 + 32 + 65 + 20 + 32 + 64)
#define NU54_INSTALL_BOUND_OFFER_SIZE (NU54_INSTALL_OFFER_SIZE + NU54_INSTALL_SIGNATURE_SIZE)
#define NU54_INSTALL_BOOTSTRAP_ALPN "nu54-install/1"
#define NU54_INSTALL_BOOTSTRAP_EXPORTER "EXPORTER-NU54-INSTALL-1"

enum nu54_install_found {
	NU54_INSTALL_FOUND_MARKER = 1u,
	NU54_INSTALL_FOUND_REPLAY = 2u,
	NU54_INSTALL_FOUND_ID = 4u,
	NU54_INSTALL_FOUND_PAYER = 8u,
	NU54_INSTALL_FOUND_KEYS = 16u,
	NU54_INSTALL_FOUND_FLOOR = 32u,
	NU54_INSTALL_FOUND_COMMIT = 64u,
};

enum nu54_install_boot_action {
	NU54_INSTALL_BOOT_DENY, NU54_INSTALL_BOOT_CREATE, NU54_INSTALL_BOOT_CREATE_REPLAY,
	NU54_INSTALL_BOOT_CREATE_KEYS, NU54_INSTALL_BOOT_FINISH, NU54_INSTALL_BOOT_READY
};

/* Call only after authentic reads; an I/O error or malformed record is never absence. */
enum nu54_install_boot_action nu54_install_boot(unsigned found);

typedef struct {
	uint64_t revision, payer_generation, not_after, merchant_key_version, beneficiary_version;
	nu54_pilot_policy allowlist;
	uint8_t merchant_signer[20], merchant[32];
	uint8_t owner[32], terminal[32], display[65], recovery[65], payer_role;
} nu54_install_policy;

typedef struct {
	uint64_t revision, time, generation;
	uint8_t digest[32], recovery[65];
} nu54_install_floor;

typedef int (*nu54_install_hash)(const uint8_t *, size_t, uint8_t [32]);
typedef int (*nu54_install_verify)(const uint8_t *, size_t, const uint8_t [64]);

/* The trusted authorization binds this commissioning window to the exact
 * station certificate. The TLS handshake must still prove key possession. */
int nu54_install_bootstrap_ticket(const uint8_t *ticket, size_t length,
		const uint8_t public_value[NU54_INSTALL_BOOTSTRAP_PUBLIC_SIZE], nu54_install_verify verify,
		const uint8_t **certificate, size_t *certificate_length);
int nu54_install_bootstrap_offer_digest(const uint8_t offer[NU54_INSTALL_OFFER_SIZE],
		const uint8_t exporter[32], nu54_install_hash hash, uint8_t output[32]);

/* Input signatures are raw P-256 R || S over SHA-256 of the signed body. */
int nu54_install_accept(const uint8_t *bundle, size_t length, const uint8_t challenge[32],
			const uint8_t identity[65], const uint8_t payer[20], uint64_t minimum_time,
			const nu54_install_floor *, nu54_install_hash, nu54_install_verify,
			nu54_install_floor *, nu54_install_policy *, size_t *ca_length, size_t *board_length);

int nu54_install_quote(const nu54_install_policy *, const uint8_t *payload, size_t length,
		       uint64_t now, nu54_v2_quote_policy *order);

/* ---- a grant kept through a restart ---- */

/* Checks a grant read back from storage. It must carry the authority's signature and be exactly
 * the grant the floor was committed for: an older grant, or one whose commit did not complete,
 * is not it. No challenge is asked for; the floor takes its place. */
int nu54_install_restore(const uint8_t *bundle, size_t length, const uint8_t identity[65],
			 const uint8_t payer[20], const nu54_install_floor *committed, nu54_install_hash,
			 nu54_install_verify, nu54_install_policy *, size_t *ca_length, size_t *board_length);

/* Whether a stored grant still belongs to the board's owner state. With an owner it must name the
 * owner's generation. Without one (new, released by a reset, or revoked) it must name the next
 * generation, the one the owner it admits will get. A reset raises the generation, so a grant
 * from before a reset matches neither. */
bool nu54_install_generation_current(uint64_t grant_generation, uint64_t board_generation, bool no_owner);

/* The time the board keeps in storage: its trusted lower bound, how far the latest possible time
 * was above it, how often the owner's phone moved the lower bound into a later day under this
 * grant, and the board time up to which an order that was open when the phone moved the clock
 * still counts as open (zero: none). The phone's time may be ahead of the calendar, so it must
 * not end such an order for the board, and a restart must not forget that. */
typedef struct {
	uint64_t earliest, offset, phone_days, hold;
} nu54_install_clock;

/* The lower bound a restarted board continues from: the latest of what it saved and of its other
 * floors. Time without power is not in it, so the board's clock is late by that much and never
 * early. `saved` is NULL when nothing was saved. */
uint64_t nu54_install_resume(const nu54_install_clock *saved, uint64_t floor_time, uint64_t replay_floor);

/* Milliseconds until the time is saved next: every interval_seconds, and at the grant's end, so
 * that a restart after the end finds the end in storage. `latest` is the latest possible time. */
int64_t nu54_install_clock_wait_ms(uint64_t latest, uint64_t not_after, uint32_t interval_seconds);

/* ---- the owner's phone tells a restarted board the time (decision of 2026-11-27) ---- */

/* Request: op, the phone's time as eight big-endian bytes, in a NU54_V2_CLOCK message.
 * Reply: op, result, the board's earliest and latest possible time, the grant's end (8 each),
 * and the number of times the phone's time opened a later day under this grant (4). */
#define NU54_CLOCK_SET_OP 0x81
#define NU54_CLOCK_SET_REQUEST_SIZE 9
#define NU54_CLOCK_SET_REPLY_SIZE 30
/* How far the phone's clock may be off (the same 5 s the short requests allow for). */
#define NU54_CLOCK_PHONE_SECONDS 5
enum { NU54_CLOCK_MOVED = 1, NU54_CLOCK_NOT_FORWARD = 2, NU54_CLOCK_PAST_END = 3 };

/* Decides on the time a registered owner's phone tells in an authenticated session.
 *
 * The time only moves forward and stays before the grant's end. The message that carries it must
 * end within NU54_PILOT_BRIEF_SECONDS after that time, by its own word: a message can then only
 * ever set a time shortly before its own end, and one that is replayed after it ended is refused
 * as ended, whatever it carries.
 *
 * `session_seconds` is how long the board has had this session. The phone read its clock after the
 * session began, so the true time is at most that much, and the phone's own error, past the time
 * it sent: that is the new distance to the latest possible time. The latest possible time never
 * moves back.
 *
 * Returns -1 for a message that is not well formed, else one of the results above. `next` is
 * written for NU54_CLOCK_MOVED only; its hold is the current one, and the caller sets the new. */
int nu54_install_clock_set(const nu54_install_clock *current, uint64_t not_after, uint64_t time,
			   uint64_t expires, uint64_t session_seconds, nu54_install_clock *next);
void nu54_install_clock_reply(int result, const nu54_install_clock *clock, uint64_t not_after,
			      uint8_t out[NU54_CLOCK_SET_REPLY_SIZE]);

/* Whether a peer certificate valid from `from` until `until` may open a session at the board's
 * latest possible time `now`. One that has ended is refused. One that is not valid yet by a clock
 * that may be late opens a session for the time command only (NU54_CERTIFICATE_EARLY), and only
 * where the caller allows that. */
enum { NU54_CERTIFICATE_REFUSED, NU54_CERTIFICATE_CURRENT, NU54_CERTIFICATE_EARLY };
int nu54_install_certificate_window(uint64_t from, uint64_t until, uint64_t now);
/* A session that was opened early may do nothing but tell the time. */
bool nu54_install_session_admits(bool clock_only, nu54_v2_command command);

#endif
