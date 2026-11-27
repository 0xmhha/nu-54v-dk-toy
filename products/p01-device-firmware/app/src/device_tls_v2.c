#include "device_tls_v2.h"
#include "nu54_install.h"

#include <mbedtls/asn1.h>
#include <mbedtls/build_info.h>

#if MBEDTLS_VERSION_NUMBER != 0x04010200
#error "P01 authenticated TLS requires the pinned Mbed TLS 4.1.2 build input"
#endif
#include <psa/crypto.h>
#include <string.h>

#if defined(MBEDTLS_SSL_PROTO_TLS1_2) || defined(MBEDTLS_SSL_EARLY_DATA) || defined(MBEDTLS_SSL_SESSION_TICKETS) || \
	defined(MBEDTLS_SSL_TLS1_3_KEY_EXCHANGE_MODE_PSK_ENABLED) || defined(MBEDTLS_SSL_TLS1_3_KEY_EXCHANGE_MODE_PSK_EPHEMERAL_ENABLED)
#error "P01 pilot TLS must be certificate-only TLS 1.3 with no early data or tickets"
#endif

static bool role_certificate(const mbedtls_x509_crt *certificate, const char *role)
{
	if (!certificate || !role) return false;
	const mbedtls_x509_sequence *san = &certificate->subject_alt_names;
	size_t length = strlen(role);
	return !san->next && san->buf.tag == (MBEDTLS_ASN1_CONTEXT_SPECIFIC | MBEDTLS_X509_SAN_UNIFORM_RESOURCE_IDENTIFIER) &&
	       san->buf.len == length && memcmp(san->buf.p, role, length) == 0;
}

int device_tls_bootstrap_setup(mbedtls_ssl_context *tls, mbedtls_ssl_config *config, mbedtls_x509_crt *station)
{
	static const char *const alpn[] = {NU54_INSTALL_BOOTSTRAP_ALPN, NULL};
	static const uint16_t groups[] = {MBEDTLS_SSL_IANA_TLS_GROUP_SECP256R1, 0};
	static const int suites[] = {MBEDTLS_TLS1_3_AES_128_GCM_SHA256, 0};
	if (!role_certificate(station, "urn:nu54:role:installer") ||
	    !mbedtls_pk_can_do_psa(&station->pk, PSA_ALG_ECDSA(PSA_ALG_SHA_256), PSA_KEY_USAGE_VERIFY_HASH) ||
	    mbedtls_pk_get_bitlen(&station->pk) != 256 ||
	    mbedtls_ssl_config_defaults(config, MBEDTLS_SSL_IS_CLIENT, MBEDTLS_SSL_TRANSPORT_STREAM,
					MBEDTLS_SSL_PRESET_DEFAULT)) return -1;
	mbedtls_ssl_conf_min_tls_version(config, MBEDTLS_SSL_VERSION_TLS1_3);
	mbedtls_ssl_conf_max_tls_version(config, MBEDTLS_SSL_VERSION_TLS1_3);
	mbedtls_ssl_conf_authmode(config, MBEDTLS_SSL_VERIFY_REQUIRED);
	mbedtls_ssl_conf_tls13_key_exchange_modes(config, MBEDTLS_SSL_TLS1_3_KEY_EXCHANGE_MODE_EPHEMERAL);
	mbedtls_ssl_conf_groups(config, groups);
	mbedtls_ssl_conf_ciphersuites(config, suites);
	mbedtls_ssl_conf_ca_chain(config, station, NULL);
	if (mbedtls_ssl_conf_alpn_protocols(config, alpn) || mbedtls_ssl_setup(tls, config)) return -1;
	/* The authority authenticates the exact station leaf, independently of DNS. */
	return mbedtls_ssl_set_hostname(tls, NULL);
}

int device_tls_bootstrap_bind(mbedtls_ssl_context *tls, const mbedtls_x509_crt *station,
			      const uint8_t nonce[32], uint8_t exporter[32])
{
	const char *alpn = mbedtls_ssl_get_alpn_protocol(tls);
	const mbedtls_x509_crt *peer = mbedtls_ssl_get_peer_cert(tls);
	if (mbedtls_ssl_get_version_number(tls) != MBEDTLS_SSL_VERSION_TLS1_3 ||
	    mbedtls_ssl_get_verify_result(tls) || !alpn || strcmp(alpn, NU54_INSTALL_BOOTSTRAP_ALPN) ||
	    !peer || peer->next || peer->raw.len != station->raw.len ||
	    memcmp(peer->raw.p, station->raw.p, station->raw.len)) return -1;
	return mbedtls_ssl_export_keying_material(tls, exporter, 32, NU54_INSTALL_BOOTSTRAP_EXPORTER,
					      sizeof(NU54_INSTALL_BOOTSTRAP_EXPORTER) - 1, nonce, 32, 1);
}

static int configure_profile(device_tls_v2_profile *profile, const char *alpn,
			    const mbedtls_x509_crt *ca, mbedtls_x509_crt *board, mbedtls_pk_context *board_key)
{
	if (!profile || !ca || !board_key || !alpn ||
	    !role_certificate(board, "urn:nu54:role:board")) return -1;
	profile->alpn[0] = alpn;
	profile->alpn[1] = NULL;
	mbedtls_ssl_config_init(&profile->config);
	int result = mbedtls_ssl_config_defaults(&profile->config, MBEDTLS_SSL_IS_SERVER, MBEDTLS_SSL_TRANSPORT_STREAM,
						MBEDTLS_SSL_PRESET_DEFAULT);
	if (!result) {
		mbedtls_ssl_conf_min_tls_version(&profile->config, MBEDTLS_SSL_VERSION_TLS1_3);
		mbedtls_ssl_conf_max_tls_version(&profile->config, MBEDTLS_SSL_VERSION_TLS1_3);
		mbedtls_ssl_conf_authmode(&profile->config, MBEDTLS_SSL_VERIFY_REQUIRED);
		mbedtls_ssl_conf_tls13_key_exchange_modes(&profile->config, MBEDTLS_SSL_TLS1_3_KEY_EXCHANGE_MODE_EPHEMERAL);
		mbedtls_ssl_conf_ca_chain(&profile->config, (mbedtls_x509_crt *)ca, NULL);
		result = mbedtls_ssl_conf_own_cert(&profile->config, board, board_key);
	}
	if (!result) result = mbedtls_ssl_conf_alpn_protocols(&profile->config, profile->alpn);
	if (result) mbedtls_ssl_config_free(&profile->config);
	return result;
}

#define ACCESSORY_ALPN "nu54-recovery/1"

int device_tls_v2_configure(device_tls_v2_profile *profile, nu54_v2_profile route,
			    const mbedtls_x509_crt *ca, mbedtls_x509_crt *board, mbedtls_pk_context *board_key)
{
	if (route != NU54_V2_MANAGEMENT && route != NU54_V2_PAYMENT) return -1;
	return configure_profile(profile, nu54_v2_alpn(route), ca, board, board_key);
}

int device_tls_recovery_configure(device_tls_v2_profile *profile, const mbedtls_x509_crt *ca,
				  mbedtls_x509_crt *board, mbedtls_pk_context *board_key)
{
	return configure_profile(profile, ACCESSORY_ALPN, ca, board, board_key);
}

static bool epoch_seconds(const mbedtls_x509_time *time, uint64_t *out)
{
	static const uint8_t month_days[] = {31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31};
	if (time->year < 1970 || time->year > 2199 || time->mon < 1 || time->mon > 12 ||
	    time->day < 1 || time->hour < 0 || time->hour > 23 || time->min < 0 || time->min > 59 ||
	    time->sec < 0 || time->sec > 59) return false;
	unsigned year = (unsigned)time->year;
	unsigned month = (unsigned)time->mon - 1u;
	unsigned leap = year % 4 == 0 && (year % 100 != 0 || year % 400 == 0);
	if ((unsigned)time->day > month_days[month] + (month == 1 ? leap : 0)) return false;
	uint64_t days = (year - 1970) * 365u + (year - 1) / 4u - 1969u / 4u -
			((year - 1) / 100u - 1969u / 100u) + (year - 1) / 400u - 1969u / 400u;
	for (unsigned i = 0; i < month; ++i) days += month_days[i];
	if (month > 1) days += leap;
	*out = (days + (unsigned)time->day - 1u) * 86400u +
	       (unsigned)time->hour * 3600u + (unsigned)time->min * 60u + (unsigned)time->sec;
	return true;
}

static int certificate_window(const mbedtls_x509_crt *certificate, uint64_t now, uint64_t *from, uint64_t *until)
{
	return certificate && epoch_seconds(&certificate->valid_from, from) && epoch_seconds(&certificate->valid_to, until) ?
	       nu54_install_certificate_window(*from, *until, now) : NU54_CERTIFICATE_REFUSED;
}

bool device_tls_v2_certificate_current(const mbedtls_x509_crt *certificate, uint64_t now)
{
	uint64_t from, until;
	return certificate_window(certificate, now, &from, &until) == NU54_CERTIFICATE_CURRENT;
}

/* `validity`, when given, receives the period in which every certificate the peer sent is valid,
 * and a certificate that is not valid yet at `now` is then left to the caller. One that has ended
 * is refused in either case. */
static int peer_pin(mbedtls_ssl_context *tls, const char *alpn, const char *role,
		    const uint8_t expected_peer[NU54_V2_PEER_SIZE], uint8_t output[NU54_V2_PEER_SIZE], uint64_t now,
		    device_tls_v2_validity *validity)
{
	if (!tls || !role || (!expected_peer && !output) || mbedtls_ssl_get_version_number(tls) != MBEDTLS_SSL_VERSION_TLS1_3 ||
	    mbedtls_ssl_get_verify_result(tls) != 0) return -1;
	const char *selected = mbedtls_ssl_get_alpn_protocol(tls);
	const mbedtls_x509_crt *peer = mbedtls_ssl_get_peer_cert(tls);
	if (!selected || strcmp(selected, alpn) || !role_certificate(peer, role)) return -1;
	device_tls_v2_validity all = {.from = 0, .until = UINT64_MAX};
	for (const mbedtls_x509_crt *entry = peer; entry; entry = entry->next) {
		uint64_t from, until;
		const int window = certificate_window(entry, now, &from, &until);
		if (window == NU54_CERTIFICATE_REFUSED || (window == NU54_CERTIFICATE_EARLY && !validity)) return -1;
		if (from > all.from) all.from = from;
		if (until < all.until) all.until = until;
	}
	if (validity) *validity = all;
	uint8_t binding[NU54_V2_PEER_SIZE];
	size_t length;
	if (psa_hash_compute(PSA_ALG_SHA_256, peer->pk_raw.p, peer->pk_raw.len, binding, sizeof(binding), &length) != PSA_SUCCESS ||
	    length != sizeof(binding) || (expected_peer && memcmp(binding, expected_peer, sizeof(binding)))) return -1;
	if (output) memcpy(output, binding, sizeof(binding));
	return 0;
}

int device_tls_recovery_pin(mbedtls_ssl_context *tls, const uint8_t expected_peer[32], uint64_t now)
{
	return peer_pin(tls, ACCESSORY_ALPN, "urn:nu54:role:recovery-accessory", expected_peer, NULL, now, NULL);
}

int device_tls_v2_bind(mbedtls_ssl_context *tls, nu54_v2_link *link, nu54_v2_profile route,
		       const uint8_t expected_peer[NU54_V2_PEER_SIZE], nu54_v2_journal *journal, uint64_t now,
		       device_tls_v2_validity *validity)
{
	const char *alpn = nu54_v2_alpn(route);
	const char *role = route == NU54_V2_MANAGEMENT ? "urn:nu54:role:owner" :
			   route == NU54_V2_PAYMENT ? "urn:nu54:role:terminal" : NULL;
	uint8_t identity[NU54_V2_PEER_SIZE];
	/* Only a pinned owner may come with a certificate that is not valid yet. */
	if (!alpn || (!expected_peer && route != NU54_V2_MANAGEMENT) ||
	    (validity && (!expected_peer || route != NU54_V2_MANAGEMENT)) ||
	    peer_pin(tls, alpn, role, expected_peer, identity, now, validity)) return -1;
	uint8_t epoch[NU54_V2_EPOCH_SIZE];
	if (mbedtls_ssl_export_keying_material(tls, epoch, sizeof(epoch), NU54_V2_EXPORTER_LABEL,
					      sizeof(NU54_V2_EXPORTER_LABEL) - 1,
					      (const unsigned char *)alpn, strlen(alpn), 1) != 0) return -1;
	return nu54_v2_link_init(link, route, NU54_V2_BOARD, epoch, identity, journal);
}
