#include "../include/nu54_market_lab.h"
#include "../../third_party/secp256k1/include/secp256k1.h"
#include "../../third_party/secp256k1/include/secp256k1_recovery.h"
#include <stdbool.h>
#include <string.h>

extern void Keccak(unsigned int rate, unsigned int capacity, const unsigned char *input,
    unsigned long long inputByteLen, unsigned char delimitedSuffix, unsigned char *output,
    unsigned long long outputByteLen);

enum { VENUE = 0, BASE = 20, QUOTE = 40, SIGNER = 60, CODE_HASH = 80 };
enum { CHAIN = 8283, QUOTE_SIGNATURE_OFFSET = 356 };

static void hash(const uint8_t *data, size_t length, uint8_t out[32])
{
    Keccak(1088, 512, data, length, 0x01, out, 32);
}

static void hash_text(const char *text, uint8_t out[32])
{
    hash((const uint8_t *)text, strlen(text), out);
}

void lab_wipe(void *memory, size_t length)
{
    volatile uint8_t *bytes = memory;
    while (length--) *bytes++ = 0;
}

static int hex_value(char c)
{
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'a' && c <= 'f') return c - 'a' + 10;
    if (c >= 'A' && c <= 'F') return c - 'A' + 10;
    return -1;
}

int lab_decode(const char *encoded, size_t text_length, uint8_t *out, size_t out_length)
{
    if (!encoded || text_length != 2 * out_length) return 0;
    for (size_t i = 0; i < out_length; i++) {
        int first = hex_value(encoded[2*i]), second = hex_value(encoded[2*i+1]);
        if (first < 0 || second < 0) return 0;
        out[i] = (uint8_t)((first << 4) | second);
    }
    return 1;
}

void lab_encode(const uint8_t *value, size_t length, char *encoded)
{
    static const char alphabet[] = "0123456789abcdef";
    for (size_t i = 0; i < length; ++i) {
        encoded[2*i] = alphabet[value[i] >> 4];
        encoded[2*i+1] = alphabet[value[i] & 15];
    }
    encoded[2*length] = 0;
}

static int pubkey_address(const secp256k1_context *ctx, const secp256k1_pubkey *key, uint8_t out[20])
{
    uint8_t bytes[65], digest[32];
    size_t length = sizeof bytes;
    if (!secp256k1_ec_pubkey_serialize(ctx, bytes, &length, key, SECP256K1_EC_UNCOMPRESSED) || length != sizeof bytes) {
        return 0;
    }
    hash(bytes + 1, length - 1, digest);
    memcpy(out, digest + 12, 20);
    return 1;
}

static int key_address(const secp256k1_context *ctx, const uint8_t seed[32], uint8_t out[20])
{
    secp256k1_pubkey key;
    return secp256k1_ec_pubkey_create(ctx, &key, seed) && pubkey_address(ctx, &key, out);
}

static secp256k1_context *context(const uint8_t entropy[32])
{
    secp256k1_context *ctx = secp256k1_context_create(SECP256K1_CONTEXT_NONE);
    if (!ctx || !secp256k1_context_randomize(ctx, entropy)) {
        if (ctx) secp256k1_context_destroy(ctx);
        return NULL;
    }
    return ctx;
}

int lab_address(const uint8_t seed[32], const uint8_t entropy[32], uint8_t out[20])
{
    secp256k1_context *ctx = context(entropy);
    if (!ctx) return 0;
    int valid = key_address(ctx, seed, out);
    secp256k1_context_destroy(ctx);
    return valid;
}

static bool zeros(const uint8_t *data, size_t length)
{
    for (size_t i = 0; i < length; ++i) if (data[i] != 0) return false;
    return true;
}

static bool scope_valid(const uint8_t scope[LAB_SCOPE_LENGTH])
{
    if (zeros(scope + CODE_HASH, 32)) return false;
    for (size_t i = VENUE; i <= SIGNER; i += 20) {
        if (zeros(scope + i, 20)) return false;
        for (size_t j = VENUE; j < i; j += 20) if (!memcmp(scope+i, scope+j, 20)) return false;
    }
    return true;
}

static bool terms_valid(const uint8_t *quote)
{
    static const uint8_t maximum[32] = {
        0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0xd3,0xc2,0x1b,0xce,0xcc,0xed,0xa1,0x00,0x00,0x00
    };
    const uint8_t *a=quote+68,*b=quote+100,*min_a=quote+132,*min_b=quote+164;
    if (zeros(a,32) || memcmp(a,maximum,32)>0 || memcmp(b,maximum,32)>0 || zeros(quote+196,32)) return false;
    switch(quote[67]) {
    case 0: case 1: case 4: case 5:
        return zeros(b,32) && !zeros(min_a,32) && zeros(min_b,32);
    case 2:
        return !zeros(b,32) && !zeros(min_a,32) && zeros(min_b,32);
    case 3:
        return zeros(b,32) && !zeros(min_a,32) && !zeros(min_b,32);
    case 6: case 7:
        return !zeros(b,32) && zeros(min_a,32) && zeros(min_b,32);
    case 8:
        return !zeros(b,32) && zeros(min_b,32);
    default:
        return false;
    }
}

static bool quote_valid(const secp256k1_context *ctx, const uint8_t scope[LAB_SCOPE_LENGTH],
    const uint8_t user[20], const uint8_t quote[LAB_QUOTE_LENGTH], uint8_t digest[32])
{
    uint8_t expected_selector[32];
    hash_text("execute((address,uint8,uint256,uint256,uint256,uint256,uint256,uint256,uint256),bytes)", expected_selector);
    if (memcmp(quote, expected_selector, 4) || !zeros(quote + 4, 12) || memcmp(quote + 16, user, 20) ||
        !zeros(quote + 36, 31) || quote[67] > 8 || !zeros(quote + 292, 30) || quote[322] != 1 || quote[323] != 64 ||
        !zeros(quote + 324, 31) || quote[355] != 65 || !zeros(quote + 421, 31) || !terms_valid(quote)) return false;

    // Domain includes the pinned runtime code hash as salt, never app-provided arbitrary digests.
    uint8_t domain[192] = {0}, message[320], digest_input[66] = {0x19, 1}, signer[20];
    hash_text("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract,bytes32 salt)", domain);
    hash_text("NU54 Isolated Market", domain + 32);
    hash_text("1", domain + 64);
    domain[126] = CHAIN >> 8;
    domain[127] = CHAIN & 255;
    memcpy(domain + 140, scope + VENUE, 20);
    memcpy(domain + 160, scope + CODE_HASH, 32);
    hash_text("Quote(address user,uint8 action,uint256 a,uint256 b,uint256 minA,uint256 minB,uint256 priceVersion,uint256 nonce,uint256 deadline)",
        message);
    memcpy(message + 32, quote + 4, 288);
    hash(domain, sizeof domain, digest_input + 2);
    hash(message, sizeof message, digest_input + 34);
    hash(digest_input, sizeof digest_input, digest);

    const uint8_t *raw = quote + QUOTE_SIGNATURE_OFFSET;
    int recovery = raw[64] - 27;
    secp256k1_ecdsa_recoverable_signature signature;
    secp256k1_ecdsa_signature standard;
    secp256k1_pubkey key;
    return recovery >= 0 && recovery <= 1 &&
        secp256k1_ecdsa_recoverable_signature_parse_compact(ctx, &signature, raw, recovery) &&
        secp256k1_ecdsa_recoverable_signature_convert(ctx, &standard, &signature) &&
        !secp256k1_ecdsa_signature_normalize(ctx, NULL, &standard) &&
        secp256k1_ecdsa_recover(ctx, &key, &signature, digest) && pubkey_address(ctx, &key, signer) &&
        !memcmp(signer, scope + SIGNER, 20);
}

static bool deadline_valid(const uint8_t quote[LAB_QUOTE_LENGTH], uint64_t now)
{
    const uint8_t *encoded = quote + 260;
    if (!now || !zeros(encoded, 24)) return false;
    uint64_t expiry = 0;
    for (size_t i = 24; i < 32; ++i) expiry = (expiry << 8) | encoded[i];
    return expiry > now && expiry - now <= 60;
}

static uint64_t check_target(const uint8_t scope[LAB_SCOPE_LENGTH], const uint8_t target[20],
    const uint8_t *data, size_t length, const uint8_t quote[LAB_QUOTE_LENGTH])
{
    if (!memcmp(target, scope + VENUE, 20)) {
        return length == LAB_QUOTE_LENGTH && !memcmp(data, quote, length) ? 600000 : 0;
    }
    static const uint8_t approve[] = {0x09, 0x5e, 0xa7, 0xb3};
    if ((length != 68 && length != LAB_BOUND_APPROVE_LENGTH) || memcmp(data, approve, 4) || !zeros(data + 4, 12) || memcmp(data + 16, scope + VENUE, 20) ) return 0;
    const uint8_t action = quote[67];
    if (!memcmp(target, scope + BASE, 20) && (action == 0 || action == 2) && !memcmp(data + 36, quote + 68, 32)) {
        return 65000;
    }
    if (!memcmp(target, scope + QUOTE, 20) && (action == 1 || action == 2 || action == 6 || action == 7) &&
        !memcmp(data + 36, quote + (action == 2 ? 100 : 68), 32)) return 65000;
    return 0;
}

static size_t length_prefix(uint8_t *out, size_t size, uint8_t offset)
{
    if (size < 56) {
        out[0] = (uint8_t)(offset + size);
        return 1;
    }
    if (size < 256) {
        out[0] = offset + 56;
        out[1] = (uint8_t)size;
        return 2;
    }
    out[0] = offset + 57;
    out[1] = (uint8_t)(size >> 8);
    out[2] = (uint8_t)size;
    return 3;
}

static size_t field(uint8_t *out, const uint8_t *data, size_t length)
{
    if (length == 1 && data[0] < 128) {
        out[0] = data[0];
        return 1;
    }
    size_t prefix = length_prefix(out, length, 128);
    if (length) memcpy(out + prefix, data, length);
    return prefix + length;
}

static size_t integer(uint8_t *out, uint64_t value)
{
    uint8_t buffer[8];
    size_t index = sizeof buffer;
    while (value) {
        buffer[--index] = (uint8_t)value;
        value >>= 8;
    }
    return field(out, buffer + index, sizeof buffer - index);
}

static size_t word(uint8_t *out, const uint8_t bytes[32])
{
    size_t index = 0;
    while (index < 32 && !bytes[index]) ++index;
    return field(out, bytes + index, 32 - index);
}

int lab_review(const uint8_t user[20], const uint8_t scope[LAB_SCOPE_LENGTH],
    const uint8_t target[20], uint64_t nonce, uint64_t price, uint64_t now, const uint8_t *data,
    size_t data_length, const uint8_t quote[LAB_QUOTE_LENGTH], lab_reviewed *review)
{
    if (price == 0 || price > 100000000000ULL || !scope_valid(scope) || !deadline_valid(quote, now)) return 0;
    uint64_t gas = check_target(scope, target, data, data_length, quote);
    uint8_t offer_digest[32];
    if (gas == 0 || !quote_valid(secp256k1_context_static, scope, user, quote, offer_digest) ||
        (data_length == LAB_BOUND_APPROVE_LENGTH && memcmp(data + 68, offer_digest, sizeof offer_digest))) return 0;
    uint8_t *fields = review->unsigned_fields;
    size_t length = integer(fields, nonce);
    length += integer(fields + length, price);
    length += integer(fields + length, gas);
    length += field(fields + length, target, 20);
    length += integer(fields + length, 0);
    length += field(fields + length, data, data_length);
    review->fields_length = length;
    size_t unsigned_length = length + integer(fields + length, CHAIN);
    unsigned_length += integer(fields + unsigned_length, 0);
    unsigned_length += integer(fields + unsigned_length, 0);
    uint8_t packet[LAB_TX_LENGTH];
    size_t prefix = length_prefix(packet, unsigned_length, 192);
    memcpy(packet + prefix, fields, unsigned_length);
    hash(packet, prefix + unsigned_length, review->digest);
    return 1;
}

int lab_serialize(const lab_reviewed *review, const uint8_t signature[65],
    uint8_t out[LAB_TX_LENGTH], size_t *out_length)
{
    if (!review->fields_length || review->fields_length > LAB_TX_LENGTH - 74 || signature[64] > 1) return 0;
    uint8_t fields[LAB_TX_LENGTH];
    size_t length=review->fields_length;
    memcpy(fields,review->unsigned_fields,length);
    length += integer(fields + length, 35 + CHAIN * 2 + signature[64]);
    length += word(fields + length, signature);
    length += word(fields + length, signature + 32);
    size_t prefix = length_prefix(out, length, 192);
    memcpy(out + prefix, fields, length);
    *out_length = prefix + length;
    return 1;
}

int lab_sign(const uint8_t seed[32], const uint8_t entropy[32], const uint8_t scope[LAB_SCOPE_LENGTH],
    const uint8_t target[20], uint64_t nonce, uint64_t price, uint64_t now, const uint8_t *data, size_t data_length,
    const uint8_t quote[LAB_QUOTE_LENGTH], uint8_t out[LAB_TX_LENGTH], size_t *out_length)
{
    secp256k1_context *ctx = context(entropy);
    if (!ctx) return 0;
    uint8_t user[20], signature[65];
    int ok = 0, recovery = 0;
    lab_reviewed reviewed = {0};
    if (!key_address(ctx, seed, user) || !lab_review(user,scope,target,nonce,price,now,data,data_length,quote,&reviewed)) goto done;
    secp256k1_ecdsa_recoverable_signature signed_digest;
    if (!secp256k1_ecdsa_sign_recoverable(ctx, &signed_digest, reviewed.digest, seed, NULL, NULL) ||
        !secp256k1_ecdsa_recoverable_signature_serialize_compact(ctx, signature, &recovery, &signed_digest) || recovery > 1) goto done;
    signature[64]=(uint8_t)recovery;
    ok=lab_serialize(&reviewed,signature,out,out_length);
done:
    secp256k1_context_destroy(ctx);
    return ok;
}
