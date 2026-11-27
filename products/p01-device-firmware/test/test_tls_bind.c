/* What the board asks of a peer's certificates before it binds a TLS session to it
 * (app/src/device_tls_v2.c): the role, the pinned key, and the period the certificates are valid
 * in, judged by the board's own clock. The functions are extracted from the source at build time.
 * Mbed TLS is a stand-in here that hands them a handshake's results as the test sets them, so this
 * runs without the pinned library. The same functions over the real one, with real certificates
 * and a real handshake: test_mbedtls_peer.sh, which needs the pinned Mbed TLS archive. */
#include "nu54_install.h"
#include "nu54_transport_v2.h"

#include <assert.h>
#include <stdio.h>
#include <string.h>

/* ---- Mbed TLS and PSA, as far as the functions look ---- */
#define MBEDTLS_ASN1_CONTEXT_SPECIFIC 0x80
#define MBEDTLS_X509_SAN_UNIFORM_RESOURCE_IDENTIFIER 6
#define MBEDTLS_SSL_VERSION_TLS1_3 0x0304
typedef struct {
	int year, mon, day, hour, min, sec;
} mbedtls_x509_time;
typedef struct {
	int tag;
	size_t len;
	const unsigned char *p;
} mbedtls_x509_buf;
typedef struct mbedtls_x509_sequence {
	mbedtls_x509_buf buf;
	struct mbedtls_x509_sequence *next;
} mbedtls_x509_sequence;
typedef struct mbedtls_x509_crt {
	mbedtls_x509_buf pk_raw;
	mbedtls_x509_time valid_from, valid_to;
	mbedtls_x509_sequence subject_alt_names;
	struct mbedtls_x509_crt *next;
} mbedtls_x509_crt;
typedef struct {
	int version;
	uint32_t verify_result;
	const char *alpn;
	const mbedtls_x509_crt *peer;
	int exporter_fails;
} mbedtls_ssl_context;
static int mbedtls_ssl_get_version_number(const mbedtls_ssl_context *tls) { return tls->version; }
static uint32_t mbedtls_ssl_get_verify_result(const mbedtls_ssl_context *tls) { return tls->verify_result; }
static const char *mbedtls_ssl_get_alpn_protocol(const mbedtls_ssl_context *tls) { return tls->alpn; }
static const mbedtls_x509_crt *mbedtls_ssl_get_peer_cert(const mbedtls_ssl_context *tls) { return tls->peer; }
static int mbedtls_ssl_export_keying_material(mbedtls_ssl_context *tls, uint8_t *out, size_t size, const char *label,
					       size_t label_length, const unsigned char *context, size_t context_length,
					       int use_context)
{
	assert(size == NU54_V2_EPOCH_SIZE && use_context == 1 && label_length == strlen(label));
	assert(!strcmp(label, NU54_V2_EXPORTER_LABEL) && context_length == strlen(tls->alpn) && !memcmp(context, tls->alpn, context_length));
	memset(out, 0xe7, size);
	return tls->exporter_fails;
}
#define PSA_SUCCESS 0
#define PSA_ALG_SHA_256 1
/* Not a hash: the key bytes, folded. It only has to tell two keys apart. */
static int psa_hash_compute(int algorithm, const uint8_t *data, size_t length, uint8_t *out, size_t size, size_t *used)
{
	assert(algorithm == PSA_ALG_SHA_256 && size == NU54_V2_PEER_SIZE);
	memset(out, 0, size);
	for (size_t i = 0; i < length; ++i) out[i % size] ^= (uint8_t)(data[i] + i);
	*used = size;
	return PSA_SUCCESS;
}

typedef struct {
	uint64_t from, until;
} device_tls_v2_validity;
bool device_tls_v2_certificate_current(const mbedtls_x509_crt *, uint64_t);
int device_tls_v2_bind(mbedtls_ssl_context *tls, nu54_v2_link *link, nu54_v2_profile route,
		       const uint8_t expected_peer[NU54_V2_PEER_SIZE], nu54_v2_journal *journal, uint64_t now,
		       device_tls_v2_validity *validity);
#include "board_tls_bind.h"

/* ---- a peer ---- */
#define DAY UINT64_C(86400)
#define ISSUED UINT64_C(1795737600) /* 2026-11-27 00:00:00 UTC */
static const char owner_role[] = "urn:nu54:role:owner", terminal_role[] = "urn:nu54:role:terminal";
static const unsigned char owner_key[] = "the owner's key", other_key[] = "another key";

static mbedtls_x509_time date(uint64_t seconds)
{
	/* Civil date of a day count (the algorithm of H. Hinnant), for dates from 1970 on. */
	const uint64_t days = seconds / DAY, rest = seconds % DAY, z = days + 719468, era = z / 146097;
	const uint64_t doe = z - era * 146097, yoe = (doe - doe / 1460 + doe / 36524 - doe / 146096) / 365;
	const uint64_t doy = doe - (365 * yoe + yoe / 4 - yoe / 100), mp = (5 * doy + 2) / 153;
	const uint64_t month = mp < 10 ? mp + 3 : mp - 9;
	return (mbedtls_x509_time){.year = (int)(yoe + era * 400 + (month <= 2)), .mon = (int)month,
		.day = (int)(doy - (153 * mp + 2) / 5 + 1), .hour = (int)(rest / 3600), .min = (int)(rest % 3600 / 60),
		.sec = (int)(rest % 60)};
}

static mbedtls_x509_crt certificate(const char *role, const unsigned char *key, uint64_t from, uint64_t until)
{
	return (mbedtls_x509_crt){.pk_raw = {.len = strlen((const char *)key), .p = key}, .valid_from = date(from),
		.valid_to = date(until), .subject_alt_names = {.buf = {
			.tag = MBEDTLS_ASN1_CONTEXT_SPECIFIC | MBEDTLS_X509_SAN_UNIFORM_RESOURCE_IDENTIFIER,
			.len = strlen(role), .p = (const unsigned char *)role}}};
}

static uint8_t owner_pin[NU54_V2_PEER_SIZE], other_pin[NU54_V2_PEER_SIZE];
static nu54_v2_journal_entry records[2];
static int digest(const uint8_t *data, size_t length, uint8_t out[32])
{
	size_t used;
	return psa_hash_compute(PSA_ALG_SHA_256, data, length, out, 32, &used);
}
static nu54_v2_journal journal = {records, 2, digest, 0};
static nu54_v2_link link;

/* A handshake that went well with this certificate chain on this route. */
static mbedtls_ssl_context session(const mbedtls_x509_crt *peer, nu54_v2_profile route)
{
	return (mbedtls_ssl_context){.version = MBEDTLS_SSL_VERSION_TLS1_3, .alpn = nu54_v2_alpn(route), .peer = peer};
}

static int bind_at(const mbedtls_x509_crt *peer, nu54_v2_profile route, const uint8_t *pin, uint64_t now,
		   device_tls_v2_validity *validity)
{
	mbedtls_ssl_context tls = session(peer, route);
	memset(&link, 0, sizeof(link));
	link.closed = true;
	return device_tls_v2_bind(&tls, &link, route, pin, &journal, now, validity);
}

/* ---- cases ---- */
static void test_dates(void)
{
	uint64_t from, until;
	mbedtls_x509_crt one = certificate(owner_role, owner_key, ISSUED, ISSUED + DAY);
	assert(one.valid_from.year == 2026 && one.valid_from.mon == 11 && one.valid_from.day == 27);
	assert(certificate_window(&one, ISSUED, &from, &until) == NU54_CERTIFICATE_CURRENT && from == ISSUED && until == ISSUED + DAY);
	/* Leap days, the end of a year, a second before midnight. */
	static const uint64_t times[] = {UINT64_C(951782400) /* 2000-02-29 */, UINT64_C(1709251199) /* 2024-02-29 23:59:59 */,
					 UINT64_C(1798761599) /* 2026-12-31 23:59:59 */, UINT64_C(4107542400) /* 2100-03-01 */};
	for (size_t i = 0; i < sizeof(times) / sizeof(times[0]); ++i) {
		one = certificate(owner_role, owner_key, times[i], times[i] + 1);
		assert(certificate_window(&one, times[i], &from, &until) == NU54_CERTIFICATE_CURRENT && from == times[i]);
		assert(until == times[i] + 1);
	}
	assert(date(951782400).mon == 2 && date(951782400).day == 29 && date(4107542400).mon == 3 && date(4107542400).day == 1);
	/* A date that is none is no period: refused, at whatever time. */
	one = certificate(owner_role, owner_key, ISSUED, ISSUED + DAY);
	one.valid_to = (mbedtls_x509_time){.year = 2027, .mon = 2, .day = 30};
	assert(certificate_window(&one, ISSUED, &from, &until) == NU54_CERTIFICATE_REFUSED);
	one.valid_to = (mbedtls_x509_time){.year = 2200, .mon = 1, .day = 1};
	assert(certificate_window(&one, ISSUED, &from, &until) == NU54_CERTIFICATE_REFUSED);
	one.valid_to = (mbedtls_x509_time){.year = 2027, .mon = 13, .day = 1};
	assert(certificate_window(&one, ISSUED, &from, &until) == NU54_CERTIFICATE_REFUSED);
	one.valid_to = (mbedtls_x509_time){.year = 2027, .mon = 1, .day = 1, .hour = 24};
	assert(certificate_window(&one, ISSUED, &from, &until) == NU54_CERTIFICATE_REFUSED);
	assert(certificate_window(NULL, ISSUED, &from, &until) == NU54_CERTIFICATE_REFUSED);
	/* What the grant's own certificates are held to. */
	one = certificate(owner_role, owner_key, ISSUED, ISSUED + DAY);
	assert(device_tls_v2_certificate_current(&one, ISSUED) && device_tls_v2_certificate_current(&one, ISSUED + DAY - 1));
	assert(!device_tls_v2_certificate_current(&one, ISSUED - 1) && !device_tls_v2_certificate_current(&one, ISSUED + DAY));
	assert(!device_tls_v2_certificate_current(&one, 0) && !device_tls_v2_certificate_current(NULL, ISSUED));
}

static void test_the_pinned_owner(void)
{
	const mbedtls_x509_crt owner = certificate(owner_role, owner_key, ISSUED, ISSUED + DAY);
	device_tls_v2_validity period = {7, 7};
	/* Valid now: bound, to the key the handshake proved and the session's own exporter. */
	assert(!bind_at(&owner, NU54_V2_MANAGEMENT, owner_pin, ISSUED + 10, NULL));
	assert(!link.closed && link.peer == NU54_V2_OWNER && link.local == NU54_V2_BOARD && link.profile == NU54_V2_MANAGEMENT);
	assert(!memcmp(link.peer_identity, owner_pin, sizeof(owner_pin)) && link.epoch[0] == 0xe7 && link.journal == &journal);
	/* Asked for, the period is reported, the same for a clock that is right. */
	assert(!bind_at(&owner, NU54_V2_MANAGEMENT, owner_pin, ISSUED + 10, &period));
	assert(period.from == ISSUED && period.until == ISSUED + DAY);

	/* An expired certificate is refused: at its end to the second, and long after, whether the
	 * period was asked for or not. The last second before the end is still taken. */
	period = (device_tls_v2_validity){7, 7};
	assert(!bind_at(&owner, NU54_V2_MANAGEMENT, owner_pin, ISSUED + DAY - 1, NULL));
	assert(bind_at(&owner, NU54_V2_MANAGEMENT, owner_pin, ISSUED + DAY, NULL) == -1 && link.closed);
	assert(bind_at(&owner, NU54_V2_MANAGEMENT, owner_pin, ISSUED + DAY, &period) == -1 && link.closed);
	assert(bind_at(&owner, NU54_V2_MANAGEMENT, owner_pin, ISSUED + 40 * DAY, &period) == -1);
	assert(period.from == 7 && period.until == 7); /* nothing is reported for a session that is not bound */
	/* A board without a time binds nothing. */
	assert(bind_at(&owner, NU54_V2_MANAGEMENT, owner_pin, 0, &period) == -1);

	/* Not valid yet by a clock that may be late: refused, unless the caller asked for the period,
	 * which makes the session one that may only tell the time. */
	assert(bind_at(&owner, NU54_V2_MANAGEMENT, owner_pin, ISSUED - 1, NULL) == -1 && link.closed);
	assert(!bind_at(&owner, NU54_V2_MANAGEMENT, owner_pin, ISSUED - 3 * DAY, &period) && !link.closed);
	assert(period.from == ISSUED && period.until == ISSUED + DAY);
	assert(nu54_install_certificate_window(period.from, period.until, ISSUED - 3 * DAY) == NU54_CERTIFICATE_EARLY);

	/* Every certificate the peer sent counts: the period is the one in which all of them are
	 * valid, one that ended refuses the session, and one that is not valid yet needs the asking. */
	mbedtls_x509_crt issuer = certificate("urn:nu54:role:issuer", other_key, ISSUED - DAY, ISSUED + DAY / 2);
	mbedtls_x509_crt leaf = owner;
	leaf.next = &issuer;
	assert(!bind_at(&leaf, NU54_V2_MANAGEMENT, owner_pin, ISSUED + 10, &period));
	assert(period.from == ISSUED && period.until == ISSUED + DAY / 2);
	assert(bind_at(&leaf, NU54_V2_MANAGEMENT, owner_pin, ISSUED + DAY / 2, NULL) == -1);
	assert(bind_at(&leaf, NU54_V2_MANAGEMENT, owner_pin, ISSUED + DAY / 2, &period) == -1);
	issuer = certificate("urn:nu54:role:issuer", other_key, ISSUED + 3600, ISSUED + 2 * DAY);
	assert(bind_at(&leaf, NU54_V2_MANAGEMENT, owner_pin, ISSUED + 10, NULL) == -1);
	assert(!bind_at(&leaf, NU54_V2_MANAGEMENT, owner_pin, ISSUED + 10, &period));
	assert(period.from == ISSUED + 3600 && period.until == ISSUED + DAY);
}

/* The period is asked for, and a certificate that is not valid yet taken, for a pinned owner only:
 * never without a pinned peer, and never on the payment route. */
static void test_no_period_without_a_pinned_owner(void)
{
	const mbedtls_x509_crt owner = certificate(owner_role, owner_key, ISSUED, ISSUED + DAY);
	const mbedtls_x509_crt terminal = certificate(terminal_role, owner_key, ISSUED, ISSUED + DAY);
	device_tls_v2_validity period = {7, 7};
	/* Without a pinned peer (the recovery session): a valid certificate binds, and the caller
	 * learns whose key it was. */
	assert(!bind_at(&owner, NU54_V2_MANAGEMENT, NULL, ISSUED + 10, NULL));
	assert(!memcmp(link.peer_identity, owner_pin, sizeof(owner_pin)));
	/* Asking for the period there is refused whole, also with a certificate that is valid. */
	assert(bind_at(&owner, NU54_V2_MANAGEMENT, NULL, ISSUED + 10, &period) == -1 && link.closed);
	assert(bind_at(&owner, NU54_V2_MANAGEMENT, NULL, ISSUED - 3600, &period) == -1 && link.closed);
	assert(period.from == 7 && period.until == 7);
	/* And without the asking, one that is not valid yet or has ended is refused as always. */
	assert(bind_at(&owner, NU54_V2_MANAGEMENT, NULL, ISSUED - 3600, NULL) == -1);
	assert(bind_at(&owner, NU54_V2_MANAGEMENT, NULL, ISSUED + DAY, NULL) == -1);

	/* The payment route: a pinned terminal with a valid certificate, and nothing else. */
	assert(!bind_at(&terminal, NU54_V2_PAYMENT, owner_pin, ISSUED + 10, NULL) && link.peer == NU54_V2_TERMINAL);
	assert(bind_at(&terminal, NU54_V2_PAYMENT, owner_pin, ISSUED + 10, &period) == -1 && link.closed);
	assert(bind_at(&terminal, NU54_V2_PAYMENT, owner_pin, ISSUED - 3600, &period) == -1);
	assert(bind_at(&terminal, NU54_V2_PAYMENT, owner_pin, ISSUED - 3600, NULL) == -1);
	assert(bind_at(&terminal, NU54_V2_PAYMENT, owner_pin, ISSUED + DAY, NULL) == -1);
	assert(bind_at(&terminal, NU54_V2_PAYMENT, NULL, ISSUED + 10, NULL) == -1);
	assert(bind_at(&terminal, NU54_V2_NEARBY, owner_pin, ISSUED + 10, NULL) == -1);
	assert(period.from == 7 && period.until == 7);
}

/* What was asked before the period mattered still is. */
static void test_who_the_peer_is(void)
{
	const mbedtls_x509_crt owner = certificate(owner_role, owner_key, ISSUED, ISSUED + DAY);
	const mbedtls_x509_crt terminal = certificate(terminal_role, owner_key, ISSUED, ISSUED + DAY);
	const mbedtls_x509_crt stranger = certificate(owner_role, other_key, ISSUED, ISSUED + DAY);
	device_tls_v2_validity period;
	/* Another key than the pinned one, early or not; the other role's certificate on a route. */
	assert(bind_at(&stranger, NU54_V2_MANAGEMENT, owner_pin, ISSUED + 10, NULL) == -1);
	assert(bind_at(&stranger, NU54_V2_MANAGEMENT, owner_pin, ISSUED - 3600, &period) == -1);
	assert(!bind_at(&stranger, NU54_V2_MANAGEMENT, other_pin, ISSUED + 10, NULL));
	assert(bind_at(&terminal, NU54_V2_MANAGEMENT, owner_pin, ISSUED + 10, NULL) == -1);
	assert(bind_at(&owner, NU54_V2_PAYMENT, owner_pin, ISSUED + 10, NULL) == -1);
	/* Two names in the certificate, or a name that only begins like the role. */
	mbedtls_x509_crt named = owner;
	mbedtls_x509_sequence second = owner.subject_alt_names;
	named.subject_alt_names.next = &second;
	assert(bind_at(&named, NU54_V2_MANAGEMENT, owner_pin, ISSUED + 10, NULL) == -1);
	named = certificate("urn:nu54:role:owner2", owner_key, ISSUED, ISSUED + DAY);
	assert(bind_at(&named, NU54_V2_MANAGEMENT, owner_pin, ISSUED + 10, NULL) == -1);
	/* A handshake that is not what the board configured, or did not verify, or has no peer. */
	mbedtls_ssl_context tls = session(&owner, NU54_V2_MANAGEMENT);
	assert(!device_tls_v2_bind(&tls, &link, NU54_V2_MANAGEMENT, owner_pin, &journal, ISSUED + 10, NULL));
	tls.verify_result = 8;
	assert(device_tls_v2_bind(&tls, &link, NU54_V2_MANAGEMENT, owner_pin, &journal, ISSUED + 10, NULL) == -1);
	tls = session(&owner, NU54_V2_MANAGEMENT);
	tls.version = 0x0303;
	assert(device_tls_v2_bind(&tls, &link, NU54_V2_MANAGEMENT, owner_pin, &journal, ISSUED + 10, NULL) == -1);
	tls = session(&owner, NU54_V2_MANAGEMENT);
	tls.alpn = nu54_v2_alpn(NU54_V2_PAYMENT);
	assert(device_tls_v2_bind(&tls, &link, NU54_V2_MANAGEMENT, owner_pin, &journal, ISSUED + 10, NULL) == -1);
	tls.alpn = NULL;
	assert(device_tls_v2_bind(&tls, &link, NU54_V2_MANAGEMENT, owner_pin, &journal, ISSUED + 10, NULL) == -1);
	tls = session(NULL, NU54_V2_MANAGEMENT);
	assert(device_tls_v2_bind(&tls, &link, NU54_V2_MANAGEMENT, owner_pin, &journal, ISSUED + 10, NULL) == -1);
	tls = session(&owner, NU54_V2_MANAGEMENT);
	tls.exporter_fails = -1;
	assert(device_tls_v2_bind(&tls, &link, NU54_V2_MANAGEMENT, owner_pin, &journal, ISSUED + 10, NULL) == -1);
}

int main(void)
{
	size_t used;
	psa_hash_compute(PSA_ALG_SHA_256, owner_key, strlen((const char *)owner_key), owner_pin, sizeof(owner_pin), &used);
	psa_hash_compute(PSA_ALG_SHA_256, other_key, strlen((const char *)other_key), other_pin, sizeof(other_pin), &used);
	assert(memcmp(owner_pin, other_pin, sizeof(owner_pin)));
	test_dates();
	test_the_pinned_owner();
	test_no_period_without_a_pinned_owner();
	test_who_the_peer_is();
	puts("tls bind ok");
	return 0;
}
