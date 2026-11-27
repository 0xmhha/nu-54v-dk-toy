#ifndef NU54_BTC48_H
#define NU54_BTC48_H
#include "nu54_hd.h"
#include <stdbool.h>

#define NU54_BTC48_MAX 1780
#define NU54_BTC48_SLOTS 3
#define NU54_BTC48_OUTPUTS 4
#define NU54_BTC48_INPUTS 2
#define NU54_BTC48_SIGNATURE_FIELD_MAX 109

typedef struct {
	uint8_t fingerprint[4], chain_code[32], public_key[33];
} nu54_btc48_xpub;

typedef struct {
	uint8_t network, owner;
	uint32_t account;
	uint64_t max_fee;
	nu54_btc48_xpub roots[NU54_BTC48_SLOTS];
} nu54_btc48_scope;

typedef int (*nu54_btc48_hash)(const uint8_t *, size_t, uint8_t [32]);
typedef int (*nu54_btc48_sign)(void *, uint32_t, uint32_t, const uint8_t [33], const uint8_t [32], uint8_t [64]);

typedef struct {
	uint8_t script[34], length;
	uint64_t satoshis;
	bool change;
} nu54_btc48_output;

typedef struct {
	uint8_t digest[32], owned_key[33];
	uint32_t branch, index;
	size_t insertion;
} nu54_btc48_input;

typedef struct {
	uint8_t commitment[32], signatures, input_count, output_count;
	uint32_t locktime;
	uint64_t fee;
	bool replaceable;
	nu54_btc48_input inputs[NU54_BTC48_INPUTS];
	nu54_btc48_output outputs[NU54_BTC48_OUTPUTS];
} nu54_btc48_intent;

int nu54_btc48_scope_valid(const nu54_btc48_scope *);
int nu54_btc48_script(const nu54_btc48_scope *, uint32_t, uint32_t, nu54_hd_hmac, uint8_t [105]);
int nu54_btc48_review(const uint8_t *, size_t, const nu54_btc48_scope *, nu54_hd_hmac,
		       nu54_btc48_hash, nu54_btc48_intent *);
int nu54_btc48_sign_psbt(const uint8_t *, size_t, const nu54_btc48_scope *, nu54_hd_hmac, nu54_btc48_hash,
			  const uint8_t [32], nu54_btc48_sign, void *, uint8_t *, size_t, size_t *);
#endif
