#include "nu54_transport_v2_openssl.h"

#include <openssl/x509v3.h>
#include <string.h>

static int select_alpn(SSL *ssl, const unsigned char **output, unsigned char *length,
                       const unsigned char *offered, unsigned int size, void *context)
{
    (void)ssl;
    const char *expected = context;
    size_t wanted = strlen(expected);
    for (size_t at = 0; at < size;) {
        unsigned char count = offered[at++];
        if (!count || count > size - at) return SSL_TLSEXT_ERR_ALERT_FATAL;
        if (count == wanted && memcmp(offered + at, expected, wanted) == 0) {
            *output = offered + at;
            *length = count;
            return SSL_TLSEXT_ERR_OK;
        }
        at += count;
    }
    return SSL_TLSEXT_ERR_ALERT_FATAL;
}

int nu54_v2_openssl_configure(SSL_CTX *context, nu54_v2_profile profile, bool server)
{
    const char *alpn = nu54_v2_alpn(profile);
    if (!context || !alpn || SSL_CTX_set_min_proto_version(context, TLS1_3_VERSION) != 1 ||
        SSL_CTX_set_max_proto_version(context, TLS1_3_VERSION) != 1 ||
        SSL_CTX_set_num_tickets(context, 0) != 1 || SSL_CTX_set_max_early_data(context, 0) != 1) return -1;
    SSL_CTX_set_session_cache_mode(context, SSL_SESS_CACHE_OFF);
    SSL_CTX_set_options(context, SSL_OP_NO_TICKET);
    SSL_CTX_set_verify(context, SSL_VERIFY_PEER | SSL_VERIFY_FAIL_IF_NO_PEER_CERT, NULL);
    if (server) {
        SSL_CTX_set_alpn_select_cb(context, select_alpn, (void *)alpn);
        return 0;
    }
    unsigned char encoded[256];
    size_t length = strlen(alpn);
    encoded[0] = (unsigned char)length;
    memcpy(encoded + 1, alpn, length);
    return SSL_CTX_set_alpn_protos(context, encoded, (unsigned int)(length + 1)) == 0 ? 0 : -1;
}

int nu54_v2_openssl_spki(X509 *certificate, uint8_t digest[NU54_V2_PEER_SIZE])
{
    unsigned char *der = NULL;
    if (!certificate) return -1;
    int length = i2d_X509_PUBKEY(X509_get_X509_PUBKEY(certificate), &der);
    bool success = length > 0 && EVP_Digest(der, (size_t)length, digest, NULL, EVP_sha256(), NULL) == 1;
    OPENSSL_free(der);
    return success ? 0 : -1;
}

static bool matches_role(X509 *certificate, nu54_v2_role role)
{
    static const char *const uris[] = {
        NULL, "urn:nu54:role:owner", "urn:nu54:role:board",
        "urn:nu54:role:terminal", "urn:nu54:role:customer",
    };
    if (!certificate || role < NU54_V2_OWNER || role > NU54_V2_CUSTOMER) return false;
    GENERAL_NAMES *names = X509_get_ext_d2i(certificate, NID_subject_alt_name, NULL, NULL);
    bool match = false;
    if (names && sk_GENERAL_NAME_num(names) == 1) {
        const GENERAL_NAME *name = sk_GENERAL_NAME_value(names, 0);
        const char *expected = uris[role];
        size_t length = strlen(expected);
        match = name->type == GEN_URI && (size_t)ASN1_STRING_length(name->d.uniformResourceIdentifier) == length &&
                memcmp(ASN1_STRING_get0_data(name->d.uniformResourceIdentifier), expected, length) == 0;
    }
    GENERAL_NAMES_free(names);
    return match;
}

int nu54_v2_openssl_bind(SSL *ssl, nu54_v2_link *link, nu54_v2_profile profile, nu54_v2_role local,
                          const uint8_t expected_peer[NU54_V2_PEER_SIZE], nu54_v2_journal *journal)
{
    const char *alpn = nu54_v2_alpn(profile);
    if (!ssl || !alpn || !expected_peer || !SSL_is_init_finished(ssl) || SSL_version(ssl) != TLS1_3_VERSION ||
        !(SSL_get_verify_mode(ssl) & SSL_VERIFY_PEER) || SSL_get_verify_result(ssl) != X509_V_OK ||
        SSL_session_reused(ssl) || SSL_get_early_data_status(ssl) != SSL_EARLY_DATA_NOT_SENT) return -1;
    const unsigned char *selected;
    unsigned int selected_length;
    SSL_get0_alpn_selected(ssl, &selected, &selected_length);
    if (selected_length != strlen(alpn) || memcmp(alpn, selected, selected_length) != 0) return -1;

    uint8_t peer[NU54_V2_PEER_SIZE], epoch[NU54_V2_EPOCH_SIZE];
    X509 *certificate = SSL_get0_peer_certificate(ssl);
    if (!matches_role(certificate, nu54_v2_peer_role(profile, local)) ||
        nu54_v2_openssl_spki(certificate, peer) != 0 ||
        CRYPTO_memcmp(peer, expected_peer, sizeof(peer)) != 0 ||
        SSL_export_keying_material(ssl, epoch, sizeof(epoch), NU54_V2_EXPORTER_LABEL,
                                   sizeof(NU54_V2_EXPORTER_LABEL) - 1, selected, selected_length, 1) != 1) return -1;
    return nu54_v2_link_init(link, profile, local, epoch, peer, journal);
}
