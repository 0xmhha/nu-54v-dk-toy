#define _POSIX_C_SOURCE 200809L
#define _DEFAULT_SOURCE
#define _DARWIN_C_SOURCE

#include "nu54_accessory.h"

#include <errno.h>
#include <fcntl.h>
#include <openssl/crypto.h>
#include <openssl/evp.h>
#include <openssl/hmac.h>
#include <openssl/pem.h>
#include <openssl/rand.h>
#include <openssl/ssl.h>
#include <openssl/x509v3.h>
#include <poll.h>
#include <stdbool.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <termios.h>
#include <time.h>
#include <sys/stat.h>
#include <unistd.h>

#define SEALED_OVERHEAD 28
#define STORE_MAX NU54_RECOVERY_ENVELOPE_MAX

struct credential {
	X509 *certificate;
	EVP_PKEY *public_key;
	uint8_t identity[32];
};

struct trust {
	struct credential source, target;
	EVP_PKEY *accessory;
};

static bool valid_role(X509 *certificate, const char *role)
{
	GENERAL_NAMES *names = X509_get_ext_d2i(certificate, NID_subject_alt_name, NULL, NULL);
	const GENERAL_NAME *name = names && sk_GENERAL_NAME_num(names) == 1 ? sk_GENERAL_NAME_value(names, 0) : NULL;
	bool ok = name && name->type == GEN_URI &&
		  (size_t)ASN1_STRING_length(name->d.uniformResourceIdentifier) == strlen(role) &&
		  !memcmp(ASN1_STRING_get0_data(name->d.uniformResourceIdentifier), role, strlen(role));
	GENERAL_NAMES_free(names);
	return ok;
}

static bool parse_hex(uint8_t out[32], const char *text)
{
	if (strlen(text) != 64) return false;
	for (size_t i = 0; i < 32; i++) {
		const char digits[] = "0123456789abcdef";
		const char *hi = strchr(digits, text[2 * i]), *lo = strchr(digits, text[2 * i + 1]);
		if (!hi || !lo || !*hi || !*lo) return false;
		out[i] = (uint8_t)(((hi - digits) << 4) | (lo - digits));
	}
	return true;
}

static int load_board(struct credential *board, const char *filename, const char *identity)
{
	FILE *stream = fopen(filename, "rb");
	if (!stream) return -1;
	board->certificate = PEM_read_X509(stream, NULL, NULL, NULL);
	fclose(stream);
	if (!board->certificate || !valid_role(board->certificate, "urn:nu54:role:board") ||
	    !parse_hex(board->identity, identity)) return -1;
	board->public_key = X509_get_pubkey(board->certificate);
	return board->public_key && EVP_PKEY_is_a(board->public_key, "EC") &&
	       EVP_PKEY_get_bits(board->public_key) == 256 ? 0 : -1;
}

static int sign_proof(void *ctx, enum nu54_recovery_role role, const uint8_t *body, size_t size, uint8_t *out, size_t *length)
{
	struct trust *trust = ctx;
	if (role != NU54_RECOVERY_ACCESSORY) return -1;
	EVP_MD_CTX *md = EVP_MD_CTX_new();
	bool ok = md && EVP_DigestSignInit(md, NULL, EVP_sha256(), NULL, trust->accessory) == 1 &&
		  EVP_DigestSign(md, out, length, body, size) == 1;
	EVP_MD_CTX_free(md);
	return ok ? 0 : -1;
}

static int verify_proof(void *ctx, enum nu54_recovery_role role, const uint8_t *body, size_t size,
			const uint8_t *signature, size_t length)
{
	const struct trust *trust = ctx;
	EVP_PKEY *key = role == NU54_RECOVERY_SOURCE ? trust->source.public_key :
			role == NU54_RECOVERY_TARGET ? trust->target.public_key : trust->accessory;
	EVP_MD_CTX *md = EVP_MD_CTX_new();
	bool ok = md && EVP_DigestVerifyInit(md, NULL, EVP_sha256(), NULL, key) == 1 &&
		  EVP_DigestVerify(md, signature, length, body, size) == 1;
	EVP_MD_CTX_free(md);
	return ok ? 0 : -1;
}

static bool authorize(const char *label, const uint8_t *identity, const uint8_t *request)
{
	if (!isatty(STDIN_FILENO) || !isatty(STDOUT_FILENO)) return false;
	printf("%s\nBoard ", label);
	for (size_t i = 0; i < 32; i++) printf("%02x", identity[i]);
	if (request) {
		printf("\nRequest ");
		for (size_t i = 0; i < 16; i++) printf("%02x", request[i]);
	}
	printf("\nType AUTHORIZE on this offline panel to proceed.\n");
	char response[32];
	return fflush(stdout) == 0 && fgets(response, sizeof(response), stdin) &&
	       strcmp(response, "AUTHORIZE\n") == 0;
}

static int store_crypt(bool encrypt, const uint8_t key[32], const uint8_t *input,
		       size_t length, uint8_t *output, size_t *size)
{
	static const uint8_t aad[] = "NU54 independent recovery storage v1";
	if (encrypt ? length > STORE_MAX : length < SEALED_OVERHEAD || length > STORE_MAX + SEALED_OVERHEAD) return -1;
	size_t plain = encrypt ? length : length - SEALED_OVERHEAD;
	EVP_CIPHER_CTX *ctx = EVP_CIPHER_CTX_new();
	if (!ctx) return -1;
	int written, tail;
	bool ok;
	if (encrypt) {
		ok = RAND_bytes(output, 12) == 1 && EVP_EncryptInit_ex(ctx, EVP_aes_256_gcm(), NULL, key, output) == 1 &&
		     EVP_EncryptUpdate(ctx, NULL, &tail, aad, sizeof(aad)) == 1 &&
		     EVP_EncryptUpdate(ctx, output + 12, &written, input, (int)plain) == 1 &&
		     EVP_EncryptFinal_ex(ctx, output + 12 + written, &tail) == 1 && (size_t)(written + tail) == plain &&
		     EVP_CIPHER_CTX_ctrl(ctx, EVP_CTRL_GCM_GET_TAG, 16, output + 12 + plain) == 1;
	} else {
		ok = EVP_DecryptInit_ex(ctx, EVP_aes_256_gcm(), NULL, key, input) == 1 &&
		     EVP_DecryptUpdate(ctx, NULL, &tail, aad, sizeof(aad)) == 1 &&
		     EVP_DecryptUpdate(ctx, output, &written, input + 12, (int)plain) == 1 &&
		     EVP_CIPHER_CTX_ctrl(ctx, EVP_CTRL_GCM_SET_TAG, 16, (void *)(input + 12 + plain)) == 1 &&
		     EVP_DecryptFinal_ex(ctx, output + written, &tail) == 1 && (size_t)(written + tail) == plain;
	}
	EVP_CIPHER_CTX_free(ctx);
	*size = encrypt ? plain + SEALED_OVERHEAD : plain;
	return ok ? 0 : -1;
}

static int load_sealed(const char *path, const uint8_t key[32], uint8_t *data, size_t *size)
{
	int fd = open(path, O_RDONLY | O_NOFOLLOW);
	if (fd < 0) return errno == ENOENT ? 1 : -1;
	struct stat state;
	uint8_t cipher[STORE_MAX + SEALED_OVERHEAD];
	ssize_t bytes = fstat(fd, &state) || !S_ISREG(state.st_mode) || state.st_uid != geteuid() ||
			(state.st_mode & 077) || state.st_size > (off_t)sizeof(cipher) ? -1 : read(fd, cipher, sizeof(cipher));
	close(fd);
	return bytes > 0 ? store_crypt(false, key, cipher, (size_t)bytes, data, size) : -1;
}

static int save_once(const char *path, const uint8_t key[32], const uint8_t *data, size_t length)
{
	uint8_t cipher[STORE_MAX + SEALED_OVERHEAD];
	size_t size;
	if (store_crypt(true, key, data, length, cipher, &size)) return -1;
	size_t path_length = strlen(path);
	if (path_length > 4000) return -1;
	char temp[4020];
	snprintf(temp, sizeof(temp), "%s.new.XXXXXX", path);
	int fd = mkstemp(temp);
	if (fd < 0) return -1;
	bool ok = write(fd, cipher, size) == (ssize_t)size && fsync(fd) == 0;
	ok = close(fd) == 0 && ok;
	ok = ok && link(temp, path) == 0;
	unlink(temp);
	if (ok) {
		const char *slash = strrchr(path, '/');
		size_t prefix = slash ? (size_t)(slash - path) : 0;
		if (slash) {
			if (!prefix) prefix = 1;
			memcpy(temp, path, prefix);
			temp[prefix] = '\0';
		} else strcpy(temp, ".");
		int directory = open(temp, O_RDONLY | O_DIRECTORY);
		ok = directory >= 0 && fsync(directory) == 0;
		if (directory >= 0) close(directory);
	}
	return ok ? 0 : -1;
}

static int ssl_packet(SSL *tls, uint8_t *data, size_t *length, size_t max, bool write)
{
	uint8_t header[2] = {(uint8_t)(*length >> 8), (uint8_t)*length};
	for (unsigned i = 0; i < 2; i++) {
		uint8_t *current = i ? data : header;
		size_t left = i ? *length : sizeof(header);
		while (left) {
			int transferred = write ? SSL_write(tls, current, (int)left) : SSL_read(tls, current, (int)left);
			if (transferred <= 0) return -1;
			current += transferred;
			left -= (size_t)transferred;
		}
		if (!i && !write) *length = ((size_t)header[0] << 8) | header[1];
		if (!*length || *length > max) return -1;
	}
	return 0;
}

static int hmac512(const uint8_t *key, size_t length, const uint8_t *data, size_t size, uint8_t digest[64])
{
	unsigned used;
	return HMAC(EVP_sha512(), key, (int)length, data, size, digest, &used) && used == 64 ? 0 : -1;
}

static bool capsule_valid(const uint8_t *data, size_t size, struct trust *trust)
{
	const size_t at = 1 + NU54_RECOVERY_CLAIM_SIZE;
	if (size <= at + 1 + NU54_HD_SEED_SIZE || size > NU54_ACCESSORY_CAPSULE_MAX || data[0] != 3 ||
	    memcmp(data + 24, trust->source.identity, 32)) return false;
	size_t signature = data[at];
	if (!signature || signature > NU54_RECOVERY_DER_MAX || size != at + 1 + signature + NU54_HD_SEED_SIZE ||
	    verify_proof(trust, NU54_RECOVERY_SOURCE, data + 1, NU54_RECOVERY_CLAIM_SIZE, data + at + 1, signature))
		return false;
	uint8_t blind[32], public_key[65], address[20], canonical[NU54_RECOVERY_CLAIM_SIZE];
	uint64_t generation = 0;
	for (size_t i = 56; i < 64; ++i) generation = (generation << 8) | data[i];
	bool derived = generation && RAND_priv_bytes(blind, sizeof(blind)) == 1 &&
		       nu54_hd_first_evm(data + size - NU54_HD_SEED_SIZE, blind, hmac512, public_key, address) == 0;
	OPENSSL_cleanse(blind, sizeof(blind));
	if (!derived) return false;
	nu54_recovery_claim(canonical, trust->source.identity, generation, public_key, address);
	return memcmp(canonical, data + 1, sizeof(canonical)) == 0;
}

static int backup(SSL *tls, struct trust *trust, const uint8_t key[32], const char *path, const char *proof_path)
{
	uint8_t data[STORE_MAX], previous[STORE_MAX], reply[1 + NU54_RECOVERY_ENVELOPE_MAX] = {7};
	size_t length = 0, saved = 0, received_proof = 0;
	int result = -1;
	uint8_t context[NU54_RECOVERY_OPEN_SIZE], ready[1] = {10};
	size_t context_length = 0, ready_length = sizeof(ready);
	nu54_recovery_context expected;
	if (ssl_packet(tls, context, &context_length, sizeof(context), false) || context[0] != 6) goto done;
	context[0] = 5;
	if (nu54_recovery_parse_open(context, context_length, &expected) ||
	    memcmp(expected.target, trust->source.identity, 32) ||
	    !authorize("Store a backup on this independent device.", expected.target, expected.request) ||
	    ssl_packet(tls, ready, &ready_length, sizeof(ready), true) ||
	    ssl_packet(tls, data, &length, sizeof(data), false) || !capsule_valid(data, length, trust)) goto done;
	int read = load_sealed(path, key, previous, &saved);
	if (read == 1) {
		if (save_once(path, key, data, length)) goto done;
	} else if (read || !capsule_valid(previous, saved, trust) ||
		   memcmp(data + 1, previous + 1, 55) ||
		   memcmp(data + 64, previous + 64, NU54_RECOVERY_CLAIM_SIZE - 63) ||
		   CRYPTO_memcmp(data + length - 32, previous + saved - 32, 32)) goto done;
	if (load_sealed(proof_path, key, reply + 1, &received_proof) == 0) reply[0] = 8;
	received_proof++;
	result = ssl_packet(tls, reply, &received_proof, sizeof(reply), true);
done:
	OPENSSL_cleanse(data, sizeof(data));
	OPENSSL_cleanse(previous, sizeof(previous));
	return result;
}

static int restore(SSL *tls, struct trust *trust, const uint8_t key[32], const char *path, const char *proof_path)
{
	uint8_t request[NU54_RECOVERY_OPEN_SIZE], capsule[STORE_MAX], partial[NU54_RECOVERY_ENVELOPE_MAX],
		envelope[NU54_RECOVERY_ENVELOPE_MAX];
	nu54_recovery_context context;
	size_t request_size = 0, capsule_size = 0, partial_size = 0, length = 0;
	int result = -1;
	const nu54_recovery_signers signers = {sign_proof, verify_proof, trust};
	if (ssl_packet(tls, request, &request_size, sizeof(request), false) ||
	    nu54_recovery_parse_open(request, request_size, &context) ||
	    memcmp(context.target, trust->target.identity, 32) ||
	    !authorize("Restore the enrolled wallet to this distinct board.", context.target, context.request) ||
	    load_sealed(path, key, capsule, &capsule_size) || !capsule_valid(capsule, capsule_size, trust) ||
	    ssl_packet(tls, capsule, &capsule_size, sizeof(capsule), true) ||
	    ssl_packet(tls, partial, &partial_size, sizeof(partial), false) ||
	    nu54_accessory_complete(capsule, capsule_size, partial, partial_size, &context, (uint64_t)time(NULL),
				    &signers, envelope, &length)) goto done;
	uint8_t committed[33], digest[32];
	size_t received = 0;
	if (ssl_packet(tls, envelope, &length, sizeof(envelope), true) ||
	    ssl_packet(tls, committed, &received, sizeof(committed), false) ||
	    received != sizeof(committed) || committed[0] != 9 ||
	    EVP_Digest(envelope, length, digest, NULL, EVP_sha256(), NULL) != 1 ||
	    CRYPTO_memcmp(committed + 1, digest, sizeof(digest))) goto done;
	/* Only retain external proof after the pinned target acknowledges its durable commit. */
	size_t previous;
	int stored = load_sealed(proof_path, key, partial, &previous);
	if (stored == 1 && save_once(proof_path, key, envelope, length)) goto done;
	if (stored < 0) goto done;
	result = 0;
done:
	OPENSSL_cleanse(capsule, sizeof(capsule));
	return result;
}

int main(int argc, char **argv)
{
	if (argc != 12) {
		fprintf(stderr, "usage: recovery-accessory PORT BOARD_CA ACCESSORY_CERT ACCESSORY_KEY SOURCE_CERT SOURCE_ID TARGET_CERT TARGET_ID BACKUP_FILE PROOF_FILE SEAL_KEY_FD\n");
		return 2;
	}
	static const uint8_t alpn[] = "\017nu54-recovery/1";
	struct trust trust = {0};
	uint8_t storage_key[32];
	char *end;
	long key_fd = strtol(argv[11], &end, 10);
	int result = 1, port = -1;
	SSL_CTX *profile = SSL_CTX_new(TLS_client_method());
	SSL *tls = NULL;
	BIO *bio = NULL;
	X509 *peer = NULL;
	EVP_PKEY *peer_key = NULL;
	if (!profile || *end || key_fd < 3 || key_fd > 1024 ||
	    read((int)key_fd, storage_key, sizeof(storage_key)) != sizeof(storage_key) ||
	    load_board(&trust.source, argv[5], argv[6]) || load_board(&trust.target, argv[7], argv[8]) ||
	    !memcmp(trust.source.identity, trust.target.identity, 32) ||
	    EVP_PKEY_eq(trust.source.public_key, trust.target.public_key) != 0 ||
	    SSL_CTX_set_min_proto_version(profile, TLS1_3_VERSION) != 1 ||
	    SSL_CTX_set_max_proto_version(profile, TLS1_3_VERSION) != 1 ||
	    SSL_CTX_set_alpn_protos(profile, alpn, sizeof(alpn) - 1) != 0 ||
	    SSL_CTX_load_verify_locations(profile, argv[2], NULL) != 1 ||
	    SSL_CTX_use_certificate_chain_file(profile, argv[3]) != 1 ||
	    SSL_CTX_use_PrivateKey_file(profile, argv[4], SSL_FILETYPE_PEM) != 1 ||
	    SSL_CTX_check_private_key(profile) != 1) goto done;
	close((int)key_fd);
	SSL_CTX_set_verify(profile, SSL_VERIFY_PEER, NULL);
	SSL_CTX_set_options(profile, SSL_OP_NO_TICKET);
	trust.accessory = SSL_CTX_get0_privatekey(profile);
	if (!valid_role(SSL_CTX_get0_certificate(profile), "urn:nu54:role:recovery-accessory") ||
	    EVP_PKEY_eq(trust.accessory, trust.source.public_key) != 0 ||
	    EVP_PKEY_eq(trust.accessory, trust.target.public_key) != 0) goto done;
	port = open(argv[1], O_RDWR | O_NOCTTY | O_NOFOLLOW);
	struct termios serial;
	if (port < 0 || tcgetattr(port, &serial)) goto done;
	cfmakeraw(&serial);
	cfsetspeed(&serial, B115200);
	serial.c_cc[VMIN] = 0;
	serial.c_cc[VTIME] = 100;
	if (tcsetattr(port, TCSAFLUSH, &serial)) goto done;
	tls = SSL_new(profile);
	bio = BIO_new_fd(port, BIO_NOCLOSE);
	if (!tls || !bio) goto done;
	SSL_set_bio(tls, bio, bio);
	bio = NULL;
	static const char expected_alpn[] = "nu54-recovery/1";
	const unsigned char *selected;
	unsigned selected_length;
	if (SSL_connect(tls) != 1 || SSL_get_verify_result(tls) != X509_V_OK || SSL_session_reused(tls)) goto done;
	SSL_get0_alpn_selected(tls, &selected, &selected_length);
	if (selected_length != sizeof(expected_alpn) - 1 || memcmp(selected, expected_alpn, selected_length)) goto done;
	peer = SSL_get1_peer_certificate(tls);
	if (!peer || !valid_role(peer, "urn:nu54:role:board")) goto done;
	peer_key = X509_get_pubkey(peer);
	if (peer_key && EVP_PKEY_eq(peer_key, trust.source.public_key) == 1)
		result = backup(tls, &trust, storage_key, argv[9], argv[10]) != 0;
	else if (peer_key && EVP_PKEY_eq(peer_key, trust.target.public_key) == 1)
		result = restore(tls, &trust, storage_key, argv[9], argv[10]) != 0;
done:
	if (result) fputs("Recovery operation unavailable or failed.\n", stderr);
	OPENSSL_cleanse(storage_key, sizeof(storage_key));
	EVP_PKEY_free(peer_key);
	X509_free(peer);
	BIO_free(bio);
	SSL_free(tls);
	if (port >= 0) close(port);
	SSL_CTX_free(profile);
	X509_free(trust.source.certificate);
	X509_free(trust.target.certificate);
	EVP_PKEY_free(trust.source.public_key);
	EVP_PKEY_free(trust.target.public_key);
	return result;
}
