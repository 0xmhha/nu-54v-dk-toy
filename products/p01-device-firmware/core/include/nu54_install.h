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

#endif
