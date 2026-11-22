#ifndef NU54_MARKET_WALLET_H
#define NU54_MARKET_WALLET_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

#define LAB_SCOPE_LENGTH 112
#define LAB_KEY_LENGTH 32
#define LAB_TX_LENGTH 600
#define LAB_QUOTE_LENGTH 452
#define LAB_BOUND_APPROVE_LENGTH 100

typedef struct {
    uint8_t digest[32];
    uint8_t unsigned_fields[LAB_TX_LENGTH];
    size_t fields_length;
} lab_reviewed;

int lab_review(const uint8_t user[20], const uint8_t scope[LAB_SCOPE_LENGTH],
    const uint8_t target[20], uint64_t nonce, uint64_t price, uint64_t now,
    const uint8_t *data, size_t data_length, const uint8_t quote[LAB_QUOTE_LENGTH], lab_reviewed *review);
int lab_serialize(const lab_reviewed *review, const uint8_t signature[65],
    uint8_t out[LAB_TX_LENGTH], size_t *out_length);

int lab_address(const uint8_t seed[LAB_KEY_LENGTH], const uint8_t entropy[32], uint8_t out[20]);
int lab_sign(const uint8_t seed[LAB_KEY_LENGTH], const uint8_t entropy[32], const uint8_t scope[LAB_SCOPE_LENGTH],
    const uint8_t target[20], uint64_t nonce, uint64_t price, uint64_t now, const uint8_t *data, size_t data_length,
    const uint8_t quote[LAB_QUOTE_LENGTH], uint8_t out[LAB_TX_LENGTH], size_t *out_length);
void lab_wipe(void *memory, size_t length);
int lab_decode(const char *encoded, size_t text_length, uint8_t *out, size_t out_length);
void lab_encode(const uint8_t *value, size_t length, char *encoded);

#ifdef __cplusplus
}
#endif
#endif
