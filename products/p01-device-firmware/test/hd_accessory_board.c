/* Host board storage and crypto adapters for the real independent accessory executable. */
#include "nu54_accessory.h"
#include "hd_accessory_store.h"

#include <openssl/pem.h>
#include <stdio.h>
#include <string.h>
#include <sys/stat.h>
#include <time.h>
#include <unistd.h>

typedef struct {
    EVP_PKEY *pinned[3];
    EVP_PKEY *private_key;
    enum nu54_recovery_role role;
} board_keys;

static int sign(void *context, enum nu54_recovery_role role, const uint8_t *data, size_t size, uint8_t *sig, size_t *used)
{
    const board_keys *keys = context;
    if (role != keys->role) return -1;
    EVP_MD_CTX *md = EVP_MD_CTX_new();
    bool good = md && EVP_DigestSignInit(md, NULL, EVP_sha256(), NULL, keys->private_key) == 1 &&
        EVP_DigestSign(md, sig, used, data, size) == 1;
    EVP_MD_CTX_free(md);
    return good ? 0 : -1;
}

static int verify(void *context, enum nu54_recovery_role role, const uint8_t *data, size_t size, const uint8_t *sig, size_t used)
{
    const board_keys *keys = context;
    if ((unsigned)role > NU54_RECOVERY_ACCESSORY) return -1;
    EVP_MD_CTX *md = EVP_MD_CTX_new();
    bool good = md && EVP_DigestVerifyInit(md, NULL, EVP_sha256(), NULL, keys->pinned[role]) == 1 &&
        EVP_DigestVerify(md, sig, used, data, size) == 1;
    EVP_MD_CTX_free(md);
    return good ? 0 : -1;
}

static EVP_PKEY *certificate_key(const char *path)
{
    FILE *file = fopen(path, "r");
    if (!file) return NULL;
    X509 *certificate = PEM_read_X509(file, NULL, NULL, NULL);
    fclose(file);
    EVP_PKEY *key = certificate ? X509_get_pubkey(certificate) : NULL;
    X509_free(certificate);
    return key;
}

static EVP_PKEY *private_key(const char *path)
{
    FILE *file = fopen(path, "r");
    if (!file) return NULL;
    EVP_PKEY *key = PEM_read_PrivateKey(file, NULL, NULL, NULL);
    fclose(file);
    return key;
}

static int output(const void *data, size_t length)
{
    return fwrite(data, 1, length, stdout) == length && fflush(stdout) == 0 ? 0 : -1;
}

static size_t packet(uint8_t *data, size_t capacity)
{
    uint8_t header[2];
    if (fread(header, 1, 2, stdin) != 2) return 0;
    size_t length = (size_t)header[0] * 256 + header[1];
    return length <= capacity && fread(data, 1, length, stdin) == length ? length : 0;
}

static int capsule(nu54_hd_slot *slot, const nu54_recovery_context *request, const nu54_recovery_signers *signers)
{
    uint8_t body[NU54_ACCESSORY_CAPSULE_MAX] = {3};
    if (slot->record.phase != NU54_HD_BACKUP_REQUIRED && nu54_hd_create(slot, request->target)) return -1;
    nu54_recovery_claim(body + 1, request->target, request->generation, slot->record.public_key, slot->record.address);
    size_t signed_size = NU54_RECOVERY_DER_MAX;
    int result = signers->sign(signers->context, NU54_RECOVERY_SOURCE, body + 1, NU54_RECOVERY_CLAIM_SIZE,
        body + 2 + NU54_RECOVERY_CLAIM_SIZE, &signed_size);
    size_t seed_offset = 2 + NU54_RECOVERY_CLAIM_SIZE + signed_size;
    body[1 + NU54_RECOVERY_CLAIM_SIZE] = (uint8_t)signed_size;
    if (!result && signed_size && signed_size <= NU54_RECOVERY_DER_MAX &&
        !hd_host_read_exact("hd-seed", body + seed_offset, NU54_HD_SEED_SIZE))
        result = output(body, seed_offset + NU54_HD_SEED_SIZE);
    else
        result = -1;
    OPENSSL_cleanse(body, sizeof(body));
    return result;
}

static int target(nu54_hd_slot *slot, const char *operation, const nu54_recovery_context *request,
                  uint64_t now, const nu54_recovery_signers *signers)
{
    uint8_t seed_packet[NU54_ACCESSORY_CAPSULE_MAX], proof[NU54_RECOVERY_ENVELOPE_MAX];
    size_t seed_packet_size = packet(seed_packet, sizeof(seed_packet));
    int result = -1;
    if (seed_packet_size > NU54_HD_SEED_SIZE && !strcmp(operation, "partial")) {
        size_t written;
        if (!nu54_accessory_target_proof(slot, seed_packet, seed_packet_size, request, now, signers, proof, &written))
            result = output(proof, written);
    } else if (seed_packet_size > NU54_HD_SEED_SIZE && !strcmp(operation, "commit")) {
        size_t length = packet(proof, sizeof(proof));
        if (length && !nu54_hd_restore(slot, seed_packet + seed_packet_size - NU54_HD_SEED_SIZE, NU54_HD_SEED_SIZE,
            proof, length, request, now, signers)) {
            uint8_t ack[33] = {9};
            if (EVP_Digest(proof, length, ack + 1, NULL, EVP_sha256(), NULL) == 1)
                result = output(ack, sizeof(ack));
        }
    }
    OPENSSL_cleanse(seed_packet, sizeof(seed_packet));
    return result;
}

int main(int argc, char **argv)
{
    umask(0077);
    if ((argc != 3 && argc != 8) || chdir(argv[1])) return 2;
    nu54_hd_slot slot;
    if (hd_host_open(&slot)) return 2;
    if (argc == 3 && !strcmp(argv[2], "inspect")) {
        uint8_t address[20];
        if (nu54_hd_address(&slot, address)) return output("LOCKED\n", 7) ? 1 : 0;
        return output(address, sizeof(address)) ? 1 : 0;
    }
    if (argc != 8 || (strcmp(argv[3], "source") && strcmp(argv[3], "target"))) return 2;
    board_keys keys = {.role = !strcmp(argv[3], "source") ? NU54_RECOVERY_SOURCE : NU54_RECOVERY_TARGET};
    for (size_t role = 0; role < 3; ++role) keys.pinned[role] = certificate_key(argv[4 + role]);
    keys.private_key = private_key(argv[7]);
    const nu54_recovery_signers signers = {sign, verify, &keys};
    int result = -1;
    uint8_t open[NU54_RECOVERY_OPEN_SIZE], proof[NU54_RECOVERY_ENVELOPE_MAX];
    nu54_recovery_context request;
    uint64_t now = (uint64_t)time(NULL);
    if (!keys.pinned[0] || !keys.pinned[1] || !keys.pinned[2] || !keys.private_key ||
        EVP_PKEY_eq(keys.private_key, keys.pinned[keys.role]) != 1 ||
        packet(open, sizeof(open)) != sizeof(open) || nu54_recovery_parse_open(open, sizeof(open), &request) ||
        request.expires <= now) goto done;

    if (keys.role == NU54_RECOVERY_SOURCE && !strcmp(argv[2], "capsule")) {
        result = capsule(&slot, &request, &signers);
    } else if (keys.role == NU54_RECOVERY_SOURCE && !strcmp(argv[2], "mark")) {
        size_t length = packet(proof, sizeof(proof));
        if (length) result = nu54_hd_mark_backup(&slot, proof, length, &request, now, &signers);
    } else if (keys.role == NU54_RECOVERY_TARGET) {
        result = target(&slot, argv[2], &request, now, &signers);
    }
done:
    EVP_PKEY_free(keys.private_key);
    for (size_t role = 0; role < 3; ++role) EVP_PKEY_free(keys.pinned[role]);
    return result ? 1 : 0;
}
