#ifndef DEVICE_CHAINS_H
#define DEVICE_CHAINS_H

#include "device_chain_keys.h"
#include "nu54_btc.h"
#include "nu54_btc48.h"

#include "nu54_solana.h"
#include "nu54_transport_v2.h"
#include "nu54_xrpl.h"

#define NU54_CHAIN_PUBLIC_OP 10
#define NU54_CHAIN_TRANSFER_OP 11
#define NU54_CHAIN_XPUB_OP 13
#define NU54_CHAIN_BIP48_REGISTER 17
#define NU54_CHAIN_BIP48_SIGN 18
#define NU54_CHAIN_BIP48_STATUS 19
#define NU54_CHAIN_XPUB_REPLY_SIZE 80
#define NU54_CHAIN_GRANT_SIZE 153
#define NU54_CHAIN_RAW_OFFSET (NU54_CHAIN_GRANT_SIZE + 64)
#define NU54_CHAIN_STANDARD_WIRE_MAX 1800
#define NU54_CHAIN_WIRE_MAX (NU54_CHAIN_RAW_OFFSET + NU54_BTC48_MAX)
#define NU54_CHAIN_REPLY_MAX (NU54_BTC48_MAX + NU54_BTC48_INPUTS * NU54_BTC48_SIGNATURE_FIELD_MAX)
#define NU54_CHAIN_PUBLIC_REPLY_SIZE 68
#define NU54_CHAIN_INDEXED_REPLY_SIZE 72

typedef struct {
	bool pending, displayed;
	uint8_t operation;
	uint32_t prompt;
	uint64_t generation, expiry, lease_revision;
	device_chain_account account;
	uint8_t request[16], epoch[16], challenge[32], commitment[32], display_hash[32];
	size_t raw_size;
	uint8_t raw[NU54_CHAIN_WIRE_MAX - NU54_CHAIN_RAW_OFFSET];
	union { nu54_btc_policy btc; nu54_sol_policy sol; nu54_xrpl_policy xrpl; } policy;
} device_chains;

int device_chains_public(const nu54_v2_request *, uint8_t [NU54_CHAIN_INDEXED_REPLY_SIZE], size_t *);
int device_chains_stage(device_chains *, const nu54_v2_request *, const nu54_v2_link *,
			uint64_t, uint64_t, uint32_t, const uint8_t [32]);
int device_chains_display(device_chains *, uint64_t, const uint8_t [64]);
int device_chains_complete(device_chains *, const nu54_v2_link *, uint64_t, uint64_t,
			    uint32_t, uint8_t [NU54_CHAIN_REPLY_MAX], size_t *);
void device_chains_cancel(device_chains *);

#endif
