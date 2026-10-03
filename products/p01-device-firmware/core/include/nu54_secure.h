/*
 * Curve operations of the payment session's secure channel (payment-protocol.md 4.1). One-time
 * keys are 32-byte secp256k1 scalars; public keys travel as their x coordinate and are lifted
 * with an even y, which gives the same shared x coordinate either way. HKDF and AES-GCM are
 * platform callbacks (nu54_session.h). Board-independent: builds and tests on the host.
 */
#ifndef NU54_SECURE_H
#define NU54_SECURE_H

#include <stdint.h>

#define NU54_SESSION_KEY_INFO "nu54 session v1"
#define NU54_GCM_TAG_LEN 16

/* x coordinate of the public key of `priv`. Returns 0, or -1 when `priv` is not a valid scalar
 * (zero or not below the group order): the caller draws again. */
int nu54_secure_public_x(const uint8_t priv[32], uint8_t x[32]);

/* 1 when `x` is the x coordinate of a curve point. */
int nu54_secure_is_point(const uint8_t x[32]);

/* x coordinate of priv * (peer_x, even y). Returns 0, or -1 for an invalid scalar or point. */
int nu54_secure_shared_x(const uint8_t priv[32], const uint8_t peer_x[32], uint8_t shared[32]);

#endif /* NU54_SECURE_H */
