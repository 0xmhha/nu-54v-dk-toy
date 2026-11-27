#ifndef NU54_SPENDING_H
#define NU54_SPENDING_H

#include "nu54_local_approval.h"
#include "nu54_order.h"

/* Local actions of the spending wallet that need SW1 and the PIN. */
#define NU54_SPEND_SET_RETURN_OP 0x7a  /* op, address: once per wallet, until a reset */
#define NU54_SPEND_RETURN_OP 0x7b      /* op, amount(8), validBefore(8), nonce(32) */
#define NU54_SPEND_RESET_OP 0x7c       /* op */
#define NU54_SPEND_RESET_NONCE_OP 0x7d /* op: one-time value for the operator's reset */
#define NU54_SPEND_OPERATOR_RESET_OP 0x7e /* op, nonce(32), authority signature(64) */
#define NU54_SPEND_STATUS_OP 0x7f      /* op: caps, PIN wait, return address, left today */
#define NU54_SPEND_STATUS_SIZE (1 + 1 + 20 + 20 + 8 + 8 + 4 + 1 + 8)
#define NU54_SPEND_RETURN_MAX_SECONDS 600
#define NU54_SPEND_OPERATOR_STATEMENT_SIZE (24 + 32 + 8 + 32)

typedef struct {
	uint8_t op, address[20], nonce[32], session[16], request[16];
	uint64_t amount, valid_before, generation, expires;
	uint32_t prompt;
	bool pending;
} nu54_spend_action;

/* Parses one request and keeps it until the physical answer. `earliest` bounds the transfer time. */
int nu54_spend_stage(nu54_spend_action *, const nu54_v2_link *, const nu54_v2_request *, uint64_t now,
		     uint64_t earliest, uint64_t generation, uint32_t prompt);
/* True only for the same request in the same session, before it expires. One use. */
bool nu54_spend_consume(nu54_spend_action *, const nu54_v2_link *, uint64_t now, uint64_t generation,
			uint32_t prompt, nu54_spend_action *out);

/* EIP-3009 TransferWithAuthorization of the enrolled token to the stored return address. */
void nu54_spend_return_preimage(const uint8_t token[20], const uint8_t from[20], const uint8_t to[20],
				uint64_t amount, uint64_t valid_before, const uint8_t nonce[32],
				uint8_t out[NU54_LOCAL_PREIMAGE_SIZE]);

/* An approved order can still execute until its authorization ends, so a reset or a return waits.
 * Without trusted time (no grant since boot) or for an unreadable record the board counts from
 * boot, which outlasts NU54_ORDER_MAX_SECONDS. So neither can hold a reset back forever. */
#define NU54_SPEND_ORDER_LIFE_MS INT64_C(240000)
bool nu54_spend_order_open(nu54_order_read, void *context, uint64_t earliest, int64_t uptime_ms);

/* The account in a status reply. A board without an owner has no wallet yet and reports twenty
 * zero bytes. With an owner a missing wallet is a failure and is never reported as the zero
 * account: a phone reads zero as "not registered yet". `account` is NULL without a wallet.
 * Returns 0 with `out` written, or -1. */
int nu54_spend_status_account(const uint8_t *account, bool owner_absent, uint8_t out[20]);

void nu54_spend_operator_statement(const uint8_t board[32], uint64_t generation, const uint8_t nonce[32],
				   uint8_t out[NU54_SPEND_OPERATOR_STATEMENT_SIZE]);

#endif
