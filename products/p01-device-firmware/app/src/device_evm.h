#ifndef DEVICE_EVM_H
#define DEVICE_EVM_H

#include "nu54_evm_tx.h"
#include "nu54_market_lab.h"

#define NU54_EVM_TRANSFER_OP 9
#define NU54_EVM_REVOKE_OP 14
#define NU54_EVM_SIWE_OP 15
#define NU54_EVM_EXTERNAL_OP 20
#define NU54_EVM_MARKET_OP 21
#define NU54_EVM_MARKET_ADDRESS_OP 22
#define NU54_EVM_DAPP_ALLOWANCE_OP 36
#include "nu54_transport_v2.h"

#define NU54_DIRECT_PROOF_HEADER (1 + 8 + 8 + 8 + 8 + 20 + 32 + 8 + 8)
#define NU54_DIRECT_RAW_OFFSET (NU54_DIRECT_PROOF_HEADER + 64)

typedef struct {
	bool pending, displayed, login, market;
	uint32_t prompt;
	uint64_t generation, expiry, lease_revision;
	uint8_t address[20], board[32], challenge[32], request[16], epoch[16];
	uint8_t wire[896];
	size_t length, raw_offset;
	nu54_evm_policy policy;
	nu54_evm_intent intent;
} device_evm;

int device_evm_stage(device_evm *, const nu54_v2_request *, const nu54_v2_link *,
		     uint64_t, uint64_t, uint32_t, const uint8_t [32], const uint8_t [20]);
int device_evm_stage_market(device_evm *, const nu54_v2_request *, const nu54_v2_link *,
		     uint64_t, uint64_t, uint32_t, const uint8_t [32], const uint8_t [20]);
int device_evm_stage_login(device_evm *, const nu54_v2_request *, const nu54_v2_link *,
		     uint64_t, uint64_t, uint32_t, const uint8_t [32], const uint8_t [20]);
int device_evm_display(device_evm *, uint64_t, const uint8_t [64]);
int device_evm_complete(device_evm *, const nu54_v2_link *, uint64_t, uint64_t, uint32_t,
			const uint8_t [20], uint8_t [LAB_TX_LENGTH], size_t *);
void device_evm_cancel(device_evm *);

#endif
