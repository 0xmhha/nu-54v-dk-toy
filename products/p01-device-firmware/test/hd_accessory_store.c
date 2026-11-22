#include "hd_accessory_store.h"

#include <errno.h>
#include <openssl/hmac.h>
#include <openssl/rand.h>
#include <stdio.h>
#include <unistd.h>

int hd_host_read_exact(const char *path, void *value, size_t size)
{
    FILE *file = fopen(path, "rb");
    if (!file) return errno == ENOENT ? 1 : -1;
    bool good = fread(value, 1, size, file) == size && fgetc(file) == EOF && !ferror(file);
    fclose(file);
    return good ? 0 : -1;
}

static int persist(const char *path, const void *value, size_t size)
{
    char temporary[48];
    if (snprintf(temporary, sizeof(temporary), "%s.tmp", path) >= (int)sizeof(temporary)) return -1;
    FILE *file = fopen(temporary, "wb");
    if (!file) return -1;
    bool good = fwrite(value, 1, size, file) == size && fflush(file) == 0 && fsync(fileno(file)) == 0;
    if (fclose(file) || !good || rename(temporary, path)) {
        unlink(temporary);
        return -1;
    }
    return 0;
}

static int load(void *context, nu54_hd_record *record)
{
    (void)context;
    return hd_host_read_exact("hd-record", record, sizeof(*record));
}

static int save(void *context, const nu54_hd_record *record)
{
    (void)context;
    return persist("hd-record", record, sizeof(*record));
}

static int seed_present(void *context)
{
    (void)context;
    uint8_t seed[NU54_HD_SEED_SIZE];
    int result = hd_host_read_exact("hd-seed", seed, sizeof(seed));
    OPENSSL_cleanse(seed, sizeof(seed));
    return result;
}

static int seed_import(void *context, const uint8_t *seed, size_t length)
{
    if (length != NU54_HD_SEED_SIZE || seed_present(context) != 1) return -1;
    return persist("hd-seed", seed, length);
}

static int seed_generate(void *context)
{
    uint8_t seed[NU54_HD_SEED_SIZE];
    int result = RAND_bytes(seed, sizeof(seed)) == 1 ? seed_import(context, seed, sizeof(seed)) : -1;
    OPENSSL_cleanse(seed, sizeof(seed));
    return result;
}

static int hmac512(const uint8_t *key, size_t key_size, const uint8_t *message, size_t size, uint8_t out[64])
{
    unsigned written;
    return HMAC(EVP_sha512(), key, (int)key_size, message, size, out, &written) && written == 64 ? 0 : -1;
}

static int preview(void *context, const uint8_t *seed, size_t length, uint8_t key[65], uint8_t address[20])
{
    (void)context;
    uint8_t blind[32];
    int result = length == NU54_HD_SEED_SIZE && RAND_bytes(blind, sizeof(blind)) == 1 ?
        nu54_hd_first_evm(seed, blind, hmac512, key, address) : -1;
    OPENSSL_cleanse(blind, sizeof(blind));
    return result;
}

static int derive(void *context, uint8_t key[65], uint8_t address[20])
{
    uint8_t seed[NU54_HD_SEED_SIZE];
    int result = hd_host_read_exact("hd-seed", seed, sizeof(seed)) ? -1 : preview(context, seed, sizeof(seed), key, address);
    OPENSSL_cleanse(seed, sizeof(seed));
    return result;
}

int hd_host_open(nu54_hd_slot *slot)
{
    const nu54_hd_backend backend = {load, save, seed_present, seed_generate, preview, seed_import, derive, NULL};
    return nu54_hd_open(slot, &backend);
}
