/* Exercise the board TLS adapter on streams without requiring BLE hardware. */
#include "device_tls_v2.h"
#include <mbedtls/net_sockets.h>
#include <psa/crypto.h>
#include <signal.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <time.h>
#include <unistd.h>

#define REQUIRE(expr) do { \
	if (!(expr)) { fprintf(stderr, "failed: %s at %d\n", #expr, __LINE__); exit(1); } \
} while (0)

struct identity {
	mbedtls_x509_crt certificate;
	mbedtls_pk_context key;
};

static void load(struct identity *identity, const char *name)
{
	char path[64];
	mbedtls_x509_crt_init(&identity->certificate);
	mbedtls_pk_init(&identity->key);
	snprintf(path, sizeof(path), "%s.crt", name);
	REQUIRE(mbedtls_x509_crt_parse_file(&identity->certificate, path) == 0);
	snprintf(path, sizeof(path), "%s.key", name);
	REQUIRE(mbedtls_pk_parse_keyfile(&identity->key, path, NULL) == 0);
}

static void unload(struct identity *identity)
{
	mbedtls_pk_free(&identity->key);
	mbedtls_x509_crt_free(&identity->certificate);
}

static int sha256(const uint8_t *data, size_t length, uint8_t digest[32])
{
	size_t written = 0;
	return (psa_hash_compute(PSA_ALG_SHA_256, data, length, digest, 32, &written) == PSA_SUCCESS &&
		written == 32) ? 0 : -1;
}

static void exchange(mbedtls_x509_crt *ca, struct identity *board, struct identity *peer,
		     const struct identity *pin, nu54_v2_profile route, bool expected, uint8_t epoch[16])
{
	device_tls_v2_profile profile;
	REQUIRE(device_tls_v2_configure(&profile, route, ca, &board->certificate, &board->key) == 0);
	mbedtls_ssl_config config;
	mbedtls_ssl_config_init(&config);
	REQUIRE(mbedtls_ssl_config_defaults(&config, MBEDTLS_SSL_IS_CLIENT, MBEDTLS_SSL_TRANSPORT_STREAM,
					    MBEDTLS_SSL_PRESET_DEFAULT) == 0);
	mbedtls_ssl_conf_min_tls_version(&config, MBEDTLS_SSL_VERSION_TLS1_3);
	mbedtls_ssl_conf_max_tls_version(&config, MBEDTLS_SSL_VERSION_TLS1_3);
	mbedtls_ssl_conf_authmode(&config, MBEDTLS_SSL_VERIFY_REQUIRED);
	mbedtls_ssl_conf_ca_chain(&config, ca, NULL);
	REQUIRE(mbedtls_ssl_conf_own_cert(&config, &peer->certificate, &peer->key) == 0);
	const char *alpn[] = {nu54_v2_alpn(route), NULL};
	REQUIRE(mbedtls_ssl_conf_alpn_protocols(&config, alpn) == 0);

	int sockets[2];
	REQUIRE(socketpair(AF_UNIX, SOCK_STREAM, 0, sockets) == 0);
	mbedtls_net_context server_socket = {.fd = sockets[0]}, client_socket = {.fd = sockets[1]};
	mbedtls_ssl_context server;
	mbedtls_ssl_context client;
	mbedtls_ssl_init(&server);
	mbedtls_ssl_init(&client);
	REQUIRE(mbedtls_ssl_setup(&server, &profile.config) == 0);
	REQUIRE(mbedtls_ssl_setup(&client, &config) == 0);
	REQUIRE(mbedtls_ssl_set_hostname(&client, NULL) == 0);
	mbedtls_ssl_set_bio(&server, &server_socket, mbedtls_net_send, mbedtls_net_recv, NULL);
	mbedtls_ssl_set_bio(&client, &client_socket, mbedtls_net_send, mbedtls_net_recv, NULL);
	REQUIRE(mbedtls_net_set_nonblock(&server_socket) == 0);
	REQUIRE(mbedtls_net_set_nonblock(&client_socket) == 0);
	int result_server = MBEDTLS_ERR_SSL_WANT_READ;
	int result_client = MBEDTLS_ERR_SSL_WANT_READ;
	for (unsigned i = 0; i < 100 && (result_server || result_client); ++i) {
		if (result_server) result_server = mbedtls_ssl_handshake(&server);
		if (result_client) result_client = mbedtls_ssl_handshake(&client);
		REQUIRE(result_server == 0 || result_server == MBEDTLS_ERR_SSL_WANT_READ || result_server == MBEDTLS_ERR_SSL_WANT_WRITE);
		REQUIRE(result_client == 0 || result_client == MBEDTLS_ERR_SSL_WANT_READ || result_client == MBEDTLS_ERR_SSL_WANT_WRITE);
	}
	REQUIRE(result_server == 0 && result_client == 0);
	const mbedtls_x509_crt *board_cert = mbedtls_ssl_get_peer_cert(&client);
	REQUIRE(mbedtls_ssl_get_verify_result(&client) == 0 && board_cert &&
		board_cert->pk_raw.len == board->certificate.pk_raw.len &&
		memcmp(board_cert->pk_raw.p, board->certificate.pk_raw.p, board_cert->pk_raw.len) == 0);

	uint8_t peer_pin[32];
	const mbedtls_x509_buf *key = &pin->certificate.pk_raw;
	REQUIRE(sha256(key->p, key->len, peer_pin) == 0);
	nu54_v2_journal_entry entries[4] = {0};
	nu54_v2_journal journal = {.entries = entries, .capacity = 4, .sha256 = sha256};
	nu54_v2_link link;
	int result = device_tls_v2_bind(&server, &link, route, peer_pin, &journal, (uint64_t)time(NULL));
	REQUIRE((result == 0) == expected);
	if (expected) {
		REQUIRE(mbedtls_ssl_export_keying_material(&client, epoch, NU54_V2_EPOCH_SIZE,
			NU54_V2_EXPORTER_LABEL, strlen(NU54_V2_EXPORTER_LABEL),
			(const unsigned char *)alpn[0], strlen(alpn[0]), 1) == 0);
		REQUIRE(memcmp(link.epoch, epoch, NU54_V2_EPOCH_SIZE) == 0);
	}
	mbedtls_ssl_free(&client);
	mbedtls_ssl_free(&server);
	mbedtls_ssl_config_free(&config);
	mbedtls_ssl_config_free(&profile.config);
	mbedtls_net_free(&client_socket);
	mbedtls_net_free(&server_socket);
}

int main(void)
{
	signal(SIGPIPE, SIG_IGN);
	alarm(15);
	REQUIRE(psa_crypto_init() == PSA_SUCCESS);
	mbedtls_x509_crt ca;
	mbedtls_x509_crt_init(&ca);
	REQUIRE(mbedtls_x509_crt_parse_file(&ca, "ca.crt") == 0);
	struct identity board, owner, terminal;
	load(&board, "board");
	load(&owner, "owner");
	load(&terminal, "terminal");
	uint8_t first[16], second[16];
	exchange(&ca, &board, &owner, &owner, NU54_V2_MANAGEMENT, true, first);
	exchange(&ca, &board, &owner, &owner, NU54_V2_MANAGEMENT, true, second);
	REQUIRE(memcmp(first, second, sizeof(first)) != 0);
	exchange(&ca, &board, &terminal, &terminal, NU54_V2_PAYMENT, true, second);
	exchange(&ca, &board, &terminal, &terminal, NU54_V2_MANAGEMENT, false, second);
	exchange(&ca, &board, &owner, &terminal, NU54_V2_MANAGEMENT, false, second);
	unload(&terminal);
	unload(&owner);
	unload(&board);
	mbedtls_x509_crt_free(&ca);
	puts("Mbed TLS 4.1.2 authenticated board adapter accepted both roles and fresh exporters and rejected wrong role and pin");
}
