/* glibc hides O_NOFOLLOW and other POSIX names under -std=c11; macOS shows them anyway. */
#if defined(__linux__) && !defined(_GNU_SOURCE)
#define _GNU_SOURCE
#endif
#include "nu54_transport_v2_openssl.h"
#include "nu54_quote_v2.h"
#include "nu54_pilot.h"
#include "nu54_keccak.h"
#include "nu54_sig.h"

#include <arpa/inet.h>
#include <netinet/in.h>
#include <openssl/pem.h>
#include <openssl/ecdsa.h>
#include <openssl/rand.h>
#include <openssl/core_names.h>
#include <errno.h>
#include <fcntl.h>
#include <poll.h>
#include <sys/stat.h>
#include <signal.h>
#include <secp256k1.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <time.h>
#include <unistd.h>

static int sha256(const uint8_t *message, size_t length, uint8_t digest[32])
{
    return EVP_Digest(message, length, digest, NULL, EVP_sha256(), NULL) == 1 ? 0 : -1;
}

static bool recover_address(const uint8_t digest[32], const uint8_t sig[65], uint8_t out[20])
{
    return nu54_sig_recover(digest, sig, 65, out) == NU54_SIG_OK;
}

struct pilot_signer {
    uint8_t private_key[32], payer[20], cached[65];
    bool signed_once;
};

static int sign_test(void *context, const uint8_t digest[32], uint8_t raw[64])
{
    struct pilot_signer *signer = context;
    secp256k1_context *secp = secp256k1_context_create(SECP256K1_CONTEXT_SIGN);
    secp256k1_ecdsa_signature signature;
    int ok = secp && secp256k1_ecdsa_sign(secp, &signature, digest, signer->private_key, NULL, NULL) &&
             secp256k1_ecdsa_signature_serialize_compact(secp, raw, &signature);
    if (secp) secp256k1_context_destroy(secp);
    return ok ? 0 : -1;
}

static int present(void *context, const uint8_t *preimage, size_t length)
{
    (void)context;
    return length == NU54_LOCAL_PREIMAGE_SIZE && preimage[0] == 0x19 && preimage[1] == 1 ? 0 : -1;
}

static int transfer(SSL *ssl, uint8_t *data, size_t count, bool write)
{
    while (count) {
        size_t length = 0;
        int rc = write ? SSL_write_ex(ssl, data, count, &length) : SSL_read_ex(ssl, data, count, &length);
        if (rc != 1 || length == 0) return -1;
        data += length;
        count -= length;
    }
    return 0;
}

static int send_frame(void *context, const uint8_t *frame, size_t length)
{
    uint8_t packet[NU54_V2_FRAME_MAX + 2] = {(uint8_t)(length >> 8), (uint8_t)length};
    memcpy(packet + 2, frame, length);
    return transfer(context, packet, length + 2, true);
}

static EVP_PKEY *install_id, *display_id;

static int sign_p256(EVP_PKEY *key, const uint8_t digest[32], uint8_t raw[64])
{
    EVP_PKEY_CTX *ctx = EVP_PKEY_CTX_new(key, NULL);
    uint8_t der[80];
    size_t length = sizeof(der);
    int status = -1;
    if (ctx && EVP_PKEY_sign_init(ctx) == 1 && EVP_PKEY_CTX_set_signature_md(ctx, EVP_sha256()) == 1 &&
        EVP_PKEY_sign(ctx, der, &length, digest, 32) == 1) {
        const uint8_t *cursor = der;
        ECDSA_SIG *sig = d2i_ECDSA_SIG(NULL, &cursor, (long)length);
        const BIGNUM *r, *value;
        if (sig) {
            ECDSA_SIG_get0(sig, &r, &value);
            if (BN_bn2binpad(r, raw, 32) == 32 && BN_bn2binpad(value, raw + 32, 32) == 32) status = 0;
            ECDSA_SIG_free(sig);
        }
    }
    EVP_PKEY_CTX_free(ctx);
    return status;
}

static int sign_review(const uint8_t digest[32], uint8_t raw[64])
{
    return sign_p256(install_id, digest, raw);
}

static int verify_test_display(const uint8_t *data, size_t length, const uint8_t raw[64])
{
    ECDSA_SIG *sig = ECDSA_SIG_new();
    BIGNUM *r = BN_bin2bn(raw, 32, NULL), *value = BN_bin2bn(raw + 32, 32, NULL);
    uint8_t der[80], *cursor = der;
    int der_size = -1, rc = -1;
    if (sig && r && value && ECDSA_SIG_set0(sig, r, value)) {
        r = value = NULL;
        der_size = i2d_ECDSA_SIG(sig, &cursor);
    }
    BN_free(r);
    BN_free(value);
    ECDSA_SIG_free(sig);
    EVP_MD_CTX *ctx = EVP_MD_CTX_new();
    if (der_size > 0 && ctx && EVP_DigestVerifyInit(ctx, NULL, EVP_sha256(), NULL, display_id) == 1 &&
        EVP_DigestVerify(ctx, der, (size_t)der_size, data, length) == 1) rc = 0;
    EVP_MD_CTX_free(ctx);
    return rc;
}

static bool switch_not_published(void)
{
    const char *path = getenv("NU54_TEST_W10_SWITCH");
    struct stat info;
    return path && *path == '/' && lstat(path, &info) != 0 && errno == ENOENT;
}

static int controlled_switch(int socket_fd, uint64_t expires, bool *approved)
{
    const char *path = getenv("NU54_TEST_W10_SWITCH");
    if (!path || *path != '/') return -1;
    fputs("W10R_SENT\n", stderr);
    while ((uint64_t)time(NULL) < expires) {
        struct pollfd connection = {.fd = socket_fd, .events = POLLIN};
        if (poll(&connection, 1, 25) != 0) return -1;
        int fd = open(path, O_RDONLY | O_NONBLOCK | O_NOFOLLOW);
        if (fd < 0) {
            if (errno == ENOENT) continue;
            return -1;
        }
        struct stat info;
        uint8_t operation[9];
        ssize_t size = !fstat(fd, &info) && S_ISREG(info.st_mode) ? read(fd, operation, sizeof(operation)) : -1;
        close(fd);
        if (size == 8 && !memcmp(operation, "approve\n", 8)) {
            *approved = true;
        } else if (size == 7 && !memcmp(operation, "refuse\n", 7)) {
            *approved = false;
        } else {
            return -1;
        }
        return unlink(path);
    }
    return -1;
}

static int confirm_test_display(const nu54_local_approval *approval, const uint8_t quote[NU54_V2_SIGNED_SIZE])
{
    const char *path = getenv("NU54_TEST_W10_DISPLAY_KEY");
    FILE *file = path && *path == '/' ? fopen(path, "r") : NULL;
    if (!file) return -1;
    display_id = PEM_read_PrivateKey(file, NULL, NULL, NULL);
    fclose(file);
    char group[32];
    uint8_t statement[4 + 32 + NU54_V2_SIGNED_SIZE], digest[32], signature[64];
    int rc = -1;
    memcpy(statement, "N54D", 4);
    memcpy(statement + 36, quote, NU54_V2_SIGNED_SIZE);
    if (display_id && EVP_PKEY_is_a(display_id, "EC") &&
        EVP_PKEY_get_utf8_string_param(display_id, OSSL_PKEY_PARAM_GROUP_NAME, group, sizeof(group), NULL) &&
        !strcmp(group, "prime256v1") && EVP_PKEY_eq(display_id, install_id) == 0 &&
        RAND_bytes(statement + 4, 32) == 1 && !sha256(statement, sizeof(statement), digest) &&
        !sign_p256(display_id, digest, signature) &&
        !nu54_pilot_display(approval, quote, statement + 4, signature, verify_test_display)) rc = 0;
    EVP_PKEY_free(display_id);
    display_id = NULL;
    return rc;
}

static int serve(SSL_CTX *tls, int socket_fd, nu54_v2_profile profile, nu54_v2_role local,
                 const uint8_t *expected_peer, nu54_v2_journal *journal, const nu54_v2_quote_policy *policy,
                 struct pilot_signer *signer)
{
    SSL *ssl = SSL_new(tls);
    nu54_v2_link link;
    int result = -1;
    if (!ssl || SSL_set_fd(ssl, socket_fd) != 1 || SSL_accept(ssl) != 1 ||
        nu54_v2_openssl_bind(ssl, &link, profile, local, expected_peer, journal) != 0) goto done;

    for (;;) {
        uint8_t header[2], fragment[NU54_V2_FRAME_MAX];
        if (transfer(ssl, header, 2, false) != 0) break;
        size_t length = ((size_t)header[0] << 8) | header[1];
        if (!length || length > sizeof(fragment) || transfer(ssl, fragment, length, false) != 0) break;
        nu54_v2_request request;
        nu54_v2_result received = nu54_v2_link_receive(&link, fragment, length, (uint64_t)time(NULL), &request);
        if (received == NU54_V2_ERROR) break;
        if (received == NU54_V2_MORE) continue;
        if (request.command == NU54_V2_MANAGE && profile == NU54_V2_MANAGEMENT &&
            received == NU54_V2_NEW && !getenv("NU54_TEST_LEGACY_OWNER") &&
            nu54_v2_approval_progress_offer(request.payload, request.payload_length, 1)) {
            request.command = NU54_V2_REPLY;
            if (nu54_v2_link_send(&link, &request, send_frame, ssl) ||
                nu54_v2_link_enable_approval_progress(&link, 1)) break;
            continue;
        }
        uint8_t body[NU54_V2_BODY_MAX];
        body[0] = (uint8_t)received;
        size_t header_size = 1;
        if (request.command == NU54_V2_PAY_PREPARE) {
            const uint8_t *challenge = NULL;
            if (nu54_v2_rental_review_prepare(request.payload, request.payload_length)) {
                if (!getenv("NU54_TEST_RENTAL_REVIEW") || profile != NU54_V2_PAYMENT || received != NU54_V2_NEW) break;
                challenge = request.payload + 4;
                request.payload += 36;
                request.payload_length -= 36;
            }
            const nu54_v2_quote_crypto crypto = {nu54_keccak256, recover_address};
            if (nu54_v2_quote_verify(request.payload, request.payload_length, policy, &crypto) != NU54_V2_STATUS_OK ||
                !nu54_v2_quote_digest(request.payload, NU54_V2_UNSIGNED_SIZE, &crypto, body + 1)) break;
            header_size += 32;
            nu54_pilot_policy allowlist = {.max_amount = challenge ? NU54_PILOT_TESTUSDC2_CAP : 1000000,
                                           .commercial_testnet = challenge != NULL};
            uint64_t generation = 1;
            if (challenge) {
                generation = 0;
                for (size_t i = 24; i < 32; i++)
                    generation = (generation << 8) | request.payload[NU54_V2_PAYER_GENERATION_OFFSET + i];
            }
            memcpy(allowlist.contract, policy->expected + NU54_V2_VERIFYING_CONTRACT_OFFSET + 12, 20);
            memcpy(allowlist.token, policy->expected + NU54_V2_ASSET_OFFSET + 12, 20);
            memcpy(allowlist.beneficiary, policy->expected + NU54_V2_BENEFICIARY_OFFSET + 12, 20);
            nu54_local_approval approved = {0};
            if (nu54_pilot_stage(&approved, &link, &request, policy, &allowlist, signer->payer, generation, 11, present, NULL)) break;
            if (challenge) {
                uint8_t receipt[NU54_V2_RENTAL_REVIEW_RECEIPT_SIZE], pin[32];
                const uint8_t *quote = request.payload;
                if (signer->signed_once || nu54_v2_openssl_spki(SSL_get_certificate(ssl), pin)) break;
                install_id = SSL_get_privatekey(ssl);
                if (!install_id || nu54_pilot_rental_review(&approved, &link, &request, challenge, pin,
                    (uint64_t)time(NULL), sha256, sign_review, receipt)) break;
                const char *mode = getenv("NU54_TEST_RENTAL_REVIEW");
                const bool controlled = !strcmp(mode, "controlled");
                if (controlled && !switch_not_published()) break;
                request.command = NU54_V2_RENTAL_REVIEW;
                request.payload = receipt;
                request.payload_length = sizeof(receipt);
                if (nu54_v2_link_send(&link, &request, send_frame, ssl)) break;
                /* Host-only decision stimulus, after the C2 review has actually been transmitted. */
                uint8_t review_hash[32], decision[NU54_W10_DECISION_SIZE];
                nu54_pilot_refusal refusal;
                const uint8_t *signature = NULL;
                bool switch_approved = !strcmp(mode, "approve");
                if (controlled && (controlled_switch(socket_fd, request.expires, &switch_approved) ||
                                   confirm_test_display(&approved, quote))) break;
                uint64_t decision_time = (uint64_t)time(NULL);
                if (switch_approved) {
                    if (nu54_pilot_complete(&approved, &link, generation, decision_time, 11, NU54_LOCAL_APPROVE,
                                            signer->payer, sign_test, signer, signer->cached)) break;
                    signer->signed_once = true;
                    signature = signer->cached;
                } else if (nu54_pilot_refuse(&approved, &link, quote, decision_time, 11, sha256, &refusal)) break;
                if (sha256(receipt, sizeof(receipt), review_hash) ||
                    nu54_pilot_rental_decision(review_hash, signature, sha256, sign_review, decision)) break;
                request.command = NU54_V2_REPLY;
                request.payload = decision;
                request.payload_length = sizeof(decision);
                result = nu54_v2_link_send(&link, &request, send_frame, ssl);
                if (!result && controlled)
                    fputs(switch_approved ? "W10D_APPROVED\n" : "W10D_REFUSED\n", stderr);
                break;
            }
            if (received == NU54_V2_NEW) {
                if (link.approval_generation) {
                    uint8_t payload[NU54_V2_APPROVAL_PROGRESS_SIZE];
                    if (signer->signed_once || nu54_v2_approval_progress_body(1, 1, payload)) break;
                    nu54_v2_request progress = request;
                    progress.command = NU54_V2_APPROVAL_PROGRESS;
                    progress.payload = payload;
                    progress.payload_length = sizeof(payload);
                    if (nu54_v2_link_send(&link, &progress, send_frame, ssl)) break;
                }
                if (signer->signed_once ||
                    nu54_pilot_complete(&approved, &link, 1, policy->now + 1, 11, NU54_LOCAL_APPROVE,
                                        signer->payer, sign_test, signer, signer->cached)) break;
                signer->signed_once = true;
            } else if (!signer->signed_once) break;
            memcpy(body + header_size, signer->cached, sizeof(signer->cached));
            header_size += sizeof(signer->cached);
        } else if (request.command != NU54_V2_NEARBY_SUBMIT) {
            break;
        }
        if (request.payload_length > sizeof(body) - header_size) break;
        memcpy(body + header_size, request.payload, request.payload_length);
        request.command = NU54_V2_REPLY;
        request.payload = body;
        request.payload_length += header_size;
        result = nu54_v2_link_send(&link, &request, send_frame, ssl);
        break;
    }
    nu54_v2_link_close(&link);
done:
    if (ssl && SSL_is_init_finished(ssl)) SSL_shutdown(ssl);
    SSL_free(ssl);
    return result;
}

int main(int argc, char **argv)
{
    if (argc != 7) return 2;
    nu54_v2_profile profile = (nu54_v2_profile)atoi(argv[5]);
    nu54_v2_role local = profile == NU54_V2_NEARBY ? NU54_V2_TERMINAL : NU54_V2_BOARD;
    uint8_t expected_peer[NU54_V2_PEER_SIZE], epoch_seconds[8];
    nu54_v2_quote_policy policy = {0};
    struct pilot_signer signer = {0};
    FILE *peer_file = fopen(argv[4], "r");
    FILE *quote_file = fopen(argv[6], "rb");
    if (!peer_file || !quote_file) return 2;
    X509 *peer = PEM_read_X509(peer_file, NULL, NULL, NULL);
    bool policy_loaded = fread(policy.expected, 1, sizeof(policy.expected), quote_file) == sizeof(policy.expected) &&
                         fread(policy.merchant_signer, 1, sizeof(policy.merchant_signer), quote_file) == sizeof(policy.merchant_signer) &&
                         fread(epoch_seconds, 1, sizeof(epoch_seconds), quote_file) == sizeof(epoch_seconds) &&
                         fread(signer.private_key, 1, sizeof(signer.private_key), quote_file) == sizeof(signer.private_key);
    fclose(peer_file);
    fclose(quote_file);
    if (!policy_loaded || nu54_v2_openssl_spki(peer, expected_peer) != 0) return 2;
    memcpy(signer.payer, policy.expected + NU54_V2_PAYER_OFFSET + 12, sizeof(signer.payer));
    for (size_t i = 0; i < sizeof(epoch_seconds); ++i) policy.now = (policy.now << 8) | epoch_seconds[i];
    X509_free(peer);

    SSL_CTX *tls = SSL_CTX_new(TLS_server_method());
    if (!tls || SSL_CTX_load_verify_locations(tls, argv[1], NULL) != 1 ||
        SSL_CTX_use_certificate_file(tls, argv[2], SSL_FILETYPE_PEM) != 1 ||
        SSL_CTX_use_PrivateKey_file(tls, argv[3], SSL_FILETYPE_PEM) != 1 ||
        nu54_v2_openssl_configure(tls, profile, true) != 0) return 2;

    int listener = socket(AF_INET, SOCK_STREAM, 0);
    struct sockaddr_in address = {.sin_family = AF_INET, .sin_addr.s_addr = htonl(INADDR_LOOPBACK)};
    socklen_t length = sizeof(address);
    if (listener < 0 || bind(listener, (struct sockaddr *)&address, length) != 0 ||
        getsockname(listener, (struct sockaddr *)&address, &length) != 0 || listen(listener, 8) != 0) return 2;
    printf("%u\n", ntohs(address.sin_port));
    fflush(stdout);
    signal(SIGPIPE, SIG_IGN);
    alarm(30);

    nu54_v2_journal_entry entries[16] = {0};
    nu54_v2_journal journal = {entries, 16, sha256, 0};
    for (;;) {
        int client = accept(listener, NULL, NULL);
        if (client < 0) break;
        serve(tls, client, profile, local, expected_peer, &journal, &policy, &signer);
        close(client);
    }
    SSL_CTX_free(tls);
    close(listener);
    return 0;
}
