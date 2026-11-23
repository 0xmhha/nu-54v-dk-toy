#ifndef NU54_ORDER_H
#define NU54_ORDER_H

#include "nu54_pilot.h"

/* Paying one order into OrderEscrow (products/p06-stablenet-contracts/src/OrderEscrow.sol).
 *
 * The board signs a single EIP-3009 ReceiveWithAuthorization of the quote's asset to the escrow.
 * Its nonce commits to the merchant quote and to the phone key that may later confirm or dispute
 * the order, so one signature fixes the amount, the store, the order and the phone. */

/* Request body: magic, phone key address, merchant-signed quote. */
#define NU54_ORDER_REQUEST_SIZE (4 + 20 + NU54_V2_SIGNED_SIZE)
bool nu54_order_decode(const uint8_t *body, size_t length, const uint8_t **confirmer, const uint8_t **quote);

bool nu54_order_preimage(const uint8_t quote[NU54_V2_UNSIGNED_SIZE], const uint8_t confirmer[20],
			 uint8_t out[NU54_LOCAL_PREIMAGE_SIZE]);

/* Only the owner's phone session can stage an order, and only under a testnet grant. The signed
 * quote must come from the enrolled merchant and stay within the grant's per-payment cap. */
int nu54_order_stage(nu54_local_approval *, const nu54_v2_link *, const nu54_v2_request *,
		     const uint8_t confirmer[20], const uint8_t *quote, const nu54_v2_quote_policy *order,
		     const nu54_pilot_policy *allowlist, const uint8_t payer[20], uint64_t generation,
		     uint32_t physical_prompt, int (*present)(void *, const uint8_t *, size_t), void *context);

/* Daily total in asset base units. `earliest` is the trusted lower bound of the time, so a clock
 * that runs ahead cannot open the next day early, and an older time never lowers the total. */
typedef struct {
	uint64_t day, spent;
} nu54_order_spending;

int nu54_order_charge(const nu54_order_spending *saved, uint64_t earliest, uint64_t amount,
		      uint64_t daily_cap, nu54_order_spending *next);

/* One record per answered order. A record may be overwritten only after its authorization can no
 * longer execute, so a repeated request gets the same answer for as long as it matters. */
#define NU54_ORDER_RECORDS 16
enum { NU54_ORDER_APPROVED = 1, NU54_ORDER_REFUSED = 2 };
typedef struct {
	uint64_t expires;
	uint8_t request[16], digest[32], signature[65], outcome;
} nu54_order_record;

/* read_slot returns 1 for an absent slot, 0 for an exact record, negative for an I/O failure. */
typedef int (*nu54_order_read)(void *context, unsigned slot, nu54_order_record *);
int nu54_order_find(nu54_order_read, void *context, uint64_t earliest, const uint8_t request[16],
		    const uint8_t digest[32], nu54_order_record *, unsigned *free_slot);

/* Address test before the user sends money: the account key signs a one-time challenge under a
 * domain that no token or escrow accepts. */
#define NU54_ACCOUNT_PROOF_OP 0x79
#define NU54_ACCOUNT_PROOF_REQUEST_SIZE 33
#define NU54_ACCOUNT_PROOF_REPLY_SIZE (1 + 20 + 65 + 64)
#define NU54_ACCOUNT_STATEMENT_SIZE (24 + 32 + 8 + 20 + 65)
bool nu54_account_preimage(const uint8_t account[20], const uint8_t challenge[32],
			   uint8_t out[NU54_LOCAL_PREIMAGE_SIZE]);
/* What the board identity key signs, so the phone can tie the account to a genuine board. */
void nu54_account_statement(const uint8_t account[20], const uint8_t challenge[32],
			    const uint8_t signature[65], uint8_t out[NU54_ACCOUNT_STATEMENT_SIZE]);

#endif
