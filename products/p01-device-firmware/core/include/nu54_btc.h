#ifndef NU54_BTC_H
#define NU54_BTC_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#define NU54_BTC_PSBT_MAX 4096
#define NU54_BTC_INPUT_MAX 4
#define NU54_BTC_OUTPUT_MAX 4
#define NU54_BTC_SCRIPT_MAX 34
#define NU54_BTC_SIGNED_MAX 800

/* Trusted crypto and wallet policy. derive restricts paths to the selected
 * account and returns HASH160(compressed pubkey) for the named child. */
typedef int (*nu54_btc_hash256)(const uint8_t *, size_t, uint8_t [32]);
typedef int (*nu54_btc_derive)(void *, uint32_t, uint32_t, uint8_t [33], uint8_t [20]);

typedef struct {
	nu54_btc_hash256 hash256;
	nu54_btc_derive derive;
	void *wallet;
	uint32_t fingerprint;
	uint32_t coin;
	uint32_t account;
	uint64_t max_fee;
} nu54_btc_policy;

typedef struct {
	uint32_t branch, index;
	uint64_t amount;
	uint8_t pubkey[33], digest[32];
} nu54_btc_input;

typedef struct {
	uint64_t amount;
	uint8_t script[NU54_BTC_SCRIPT_MAX], length;
	bool change;
} nu54_btc_output;

typedef struct {
	nu54_btc_input inputs[NU54_BTC_INPUT_MAX];
	nu54_btc_output outputs[NU54_BTC_OUTPUT_MAX];
	uint64_t fee;
	uint32_t locktime;
	uint8_t input_count, output_count;
	uint8_t commitment[32];
	bool replaceable;
} nu54_btc_intent;

int nu54_btc_review(const uint8_t *, size_t, const nu54_btc_policy *, nu54_btc_intent *);

/* Sign in the trusted device path after confirming the commitment on its
 * independent display. The callback produces compact, low-S ECDSA R||S. */
typedef int (*nu54_btc_sign)(void *, const nu54_btc_input *, uint8_t [64]);
int nu54_btc_finalize(const uint8_t *, size_t, const nu54_btc_policy *,
		      const uint8_t [32], nu54_btc_sign, uint8_t *, size_t, size_t *);

/* Context 0 is an ordinary transfer. Context 1 identifies an attested
 * original txid and a smaller fee, with a fresh change-bearing approval. */
int nu54_btc_check_context(const nu54_btc_intent *, const uint8_t [90], uint64_t);

#endif
