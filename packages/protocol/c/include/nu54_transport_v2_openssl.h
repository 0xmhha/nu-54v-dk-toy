#ifndef NU54_TRANSPORT_V2_OPENSSL_H
#define NU54_TRANSPORT_V2_OPENSSL_H

#include "nu54_transport_v2.h"
#include <openssl/ssl.h>

/* Configure a fresh context with its certificate, private key and explicit CA already loaded. */
int nu54_v2_openssl_configure(SSL_CTX *context, nu54_v2_profile profile, bool server);
int nu54_v2_openssl_bind(SSL *ssl, nu54_v2_link *link, nu54_v2_profile profile, nu54_v2_role local,
                          const uint8_t expected_peer[NU54_V2_PEER_SIZE], nu54_v2_journal *journal);
int nu54_v2_openssl_spki(X509 *certificate, uint8_t digest[NU54_V2_PEER_SIZE]);

#endif
