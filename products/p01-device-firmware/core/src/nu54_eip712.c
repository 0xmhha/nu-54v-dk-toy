#include "nu54_eip712.h"

#include <string.h>

#include "nu54_keccak.h"
#include "nu54_protocol.h"

#define MAX_FIELDS 9

/* Words of one struct: typehash followed by one 32-byte word per field. */
typedef struct {
	uint8_t buf[32 * (MAX_FIELDS + 1)];
	size_t len;
} words_t;

static void begin(words_t *w, const char *encode_type)
{
	nu54_keccak256((const uint8_t *)encode_type, strlen(encode_type), w->buf);
	w->len = 32;
}

static void put_bytes32(words_t *w, const uint8_t v[32])
{
	memcpy(&w->buf[w->len], v, 32);
	w->len += 32;
}

static void put_address(words_t *w, const uint8_t v[20])
{
	memset(&w->buf[w->len], 0, 12);
	memcpy(&w->buf[w->len + 12], v, 20);
	w->len += 32;
}

static void put_uint64(words_t *w, uint64_t v)
{
	memset(&w->buf[w->len], 0, 24);
	for (int i = 0; i < 8; i++) {
		w->buf[w->len + 31 - i] = (uint8_t)(v >> (8 * i));
	}
	w->len += 32;
}

static void put_string(words_t *w, const uint8_t *s, size_t n)
{
	nu54_keccak256(s, n, &w->buf[w->len]);
	w->len += 32;
}

static void finish(const words_t *w, uint8_t out[32])
{
	nu54_keccak256(w->buf, w->len, out);
}

void nu54_eip712_domain_separator(const uint8_t chain_id[32], const uint8_t verifying_contract[20], uint8_t out[32])
{
	static const char domain_type[] =
		"EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)";
	words_t w;
	begin(&w, domain_type);
	put_string(&w, (const uint8_t *)NU54_EIP712_DOMAIN_NAME, strlen(NU54_EIP712_DOMAIN_NAME));
	put_string(&w, (const uint8_t *)NU54_EIP712_DOMAIN_VERSION, strlen(NU54_EIP712_DOMAIN_VERSION));
	put_bytes32(&w, chain_id);
	put_address(&w, verifying_contract);
	finish(&w, out);
}

void nu54_hash_payment_authorization(const nu54_payment_authorization_t *v, uint8_t out[32])
{
	words_t w;
	begin(&w, NU54_ENCODE_TYPE_PAYMENT_AUTHORIZATION);
	put_bytes32(&w, v->chain_id);
	put_address(&w, v->contract);
	put_address(&w, v->merchant);
	put_address(&w, v->payout);
	put_address(&w, v->token);
	put_bytes32(&w, v->amount);
	put_bytes32(&w, v->order_id);
	put_bytes32(&w, v->nonce);
	put_uint64(&w, v->expiry);
	finish(&w, out);
}

void nu54_hash_limit_change(const nu54_limit_change_t *v, uint8_t out[32])
{
	words_t w;
	begin(&w, NU54_ENCODE_TYPE_LIMIT_CHANGE);
	put_bytes32(&w, v->chain_id);
	put_address(&w, v->contract);
	put_bytes32(&w, v->per_payment_limit);
	put_bytes32(&w, v->daily_limit);
	put_bytes32(&w, v->nonce);
	put_uint64(&w, v->expiry);
	finish(&w, out);
}

void nu54_hash_merchant_attestation(const nu54_merchant_attestation_t *v, uint8_t out[32])
{
	words_t w;
	begin(&w, NU54_ENCODE_TYPE_MERCHANT_ATTESTATION);
	put_address(&w, v->merchant);
	put_address(&w, v->payout);
	put_string(&w, v->name, v->name_len);
	put_uint64(&w, v->valid_from);
	put_uint64(&w, v->valid_until);
	finish(&w, out);
}

void nu54_hash_merchant_order(const nu54_merchant_order_t *v, uint8_t out[32])
{
	words_t w;
	begin(&w, NU54_ENCODE_TYPE_MERCHANT_ORDER);
	put_bytes32(&w, v->order_id);
	put_address(&w, v->token);
	put_bytes32(&w, v->amount);
	put_address(&w, v->payout);
	put_uint64(&w, v->expiry);
	finish(&w, out);
}

void nu54_hash_time_anchor(const nu54_time_anchor_t *v, uint8_t out[32])
{
	words_t w;
	begin(&w, NU54_ENCODE_TYPE_TIME_ANCHOR);
	put_address(&w, v->device);
	put_uint64(&w, v->timestamp);
	finish(&w, out);
}

void nu54_hash_device_reset(const nu54_device_reset_t *v, uint8_t out[32])
{
	words_t w;
	begin(&w, NU54_ENCODE_TYPE_DEVICE_RESET);
	put_address(&w, v->device);
	put_bytes32(&w, v->nonce);
	finish(&w, out);
}

void nu54_hash_kiosk_key(const nu54_kiosk_key_t *v, uint8_t out[32])
{
	words_t w;
	begin(&w, NU54_ENCODE_TYPE_KIOSK_KEY);
	put_address(&w, v->merchant);
	put_bytes32(&w, v->kiosk_ephemeral);
	put_bytes32(&w, v->kiosk_nonce);
	finish(&w, out);
}

void nu54_eip712_digest(const uint8_t domain_separator[32], const uint8_t struct_hash[32], uint8_t out[32])
{
	uint8_t buf[66];
	buf[0] = 0x19;
	buf[1] = 0x01;
	memcpy(&buf[2], domain_separator, 32);
	memcpy(&buf[34], struct_hash, 32);
	nu54_keccak256(buf, sizeof(buf), out);
}
