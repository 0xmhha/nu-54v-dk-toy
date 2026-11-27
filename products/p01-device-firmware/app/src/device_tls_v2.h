#ifndef DEVICE_TLS_V2_H
#define DEVICE_TLS_V2_H

#include <mbedtls/ssl.h>
#include "nu54_transport_v2.h"

typedef struct {
	mbedtls_ssl_config config;
	const char *alpn[2];
} device_tls_v2_profile;

/* A real TLS context requires a provisioned CA, board cert, and its native private key. */
int device_tls_v2_configure(device_tls_v2_profile *profile, nu54_v2_profile route,
			    const mbedtls_x509_crt *ca, mbedtls_x509_crt *board, mbedtls_pk_context *board_key);
/* A null expected peer admits only a CA-authenticated owner into restricted recovery. */
bool device_tls_v2_certificate_current(const mbedtls_x509_crt *, uint64_t);
int device_tls_v2_bind(mbedtls_ssl_context *tls, nu54_v2_link *link, nu54_v2_profile route,
		       const uint8_t expected_peer[NU54_V2_PEER_SIZE], nu54_v2_journal *journal, uint64_t now);

int device_tls_recovery_configure(device_tls_v2_profile *, const mbedtls_x509_crt *,
				  mbedtls_x509_crt *, mbedtls_pk_context *);
int device_tls_recovery_pin(mbedtls_ssl_context *, const uint8_t expected_peer[32], uint64_t now);
int device_tls_bootstrap_setup(mbedtls_ssl_context *, mbedtls_ssl_config *, mbedtls_x509_crt *station);
int device_tls_bootstrap_bind(mbedtls_ssl_context *, const mbedtls_x509_crt *station,
			      const uint8_t nonce[32], uint8_t exporter[32]);

#endif
