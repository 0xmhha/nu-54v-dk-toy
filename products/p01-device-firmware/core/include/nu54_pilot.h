#ifndef NU54_PILOT_H
#define NU54_PILOT_H

#include "nu54_local_approval.h"
#include "nu54_quote_v2.h"
#include "nu54_transport_v2.h"

#define NU54_PILOT_LEGACY_CAP UINT64_C(1000000)
#define NU54_PILOT_TESTUSDC2_CAP UINT64_C(30000000)

/* How far ahead of the board's latest possible time a request may end that needs no button. A slow
 * grant leaves the board up to 60 s unsure of the time (OFFER_TTL_MS), so a request must outlive
 * that, and the phone's clock may lead by 5 s: 90 s from the app, 95 s at the board. The bound only
 * limits staleness. TLS, the session epoch and the frame counter refuse a replay whatever it is,
 * and a request that has passed the latest possible time is still refused. */
#if defined(CONFIG_NU54_SPENDING_PROFILE)
#define NU54_PILOT_BRIEF_SECONDS 95
#else
#define NU54_PILOT_BRIEF_SECONDS 10
#endif

typedef struct {
	uint8_t contract[20], token[20], beneficiary[20];
	uint64_t max_amount;
	bool commercial_testnet;
} nu54_pilot_policy;

bool nu54_pilot_approval_preimage(const uint8_t quote[NU54_V2_UNSIGNED_SIZE],
				  uint8_t out[NU54_LOCAL_PREIMAGE_SIZE]);

typedef int (*nu54_pilot_hash)(const uint8_t *, size_t, uint8_t [32]);
typedef struct {
	uint8_t session[16], request[16], digest[32];
	bool ready;
} nu54_pilot_review_binding;

int nu54_pilot_review_bind(nu54_pilot_review_binding *, const nu54_v2_link *,
			  const uint8_t request[16], const uint8_t quote[NU54_V2_SIGNED_SIZE], nu54_pilot_hash);
int nu54_pilot_review_consume(nu54_pilot_review_binding *, const nu54_v2_link *,
			     const nu54_v2_request *, nu54_pilot_hash);

/* Both authenticated routes converge here. Board provisioning defines a separate static
 * asset, destination and cap even when the order service supplies the exact expected quote. */
int nu54_pilot_verify_quote(const uint8_t *, size_t, const nu54_v2_quote_policy *,
                            const nu54_pilot_policy *, const uint8_t payer[20], uint64_t generation);
int nu54_pilot_stage(nu54_local_approval *approval, const nu54_v2_link *channel, const nu54_v2_request *request,
		      const nu54_v2_quote_policy *order, const nu54_pilot_policy *allowlist,
		      const uint8_t payer[20], uint64_t generation, uint32_t physical_prompt,
		      int (*present)(void *, const uint8_t *, size_t), void *context);
/* Only after successful staging and durable approval/refusal lookup on a new C2 request. */
int nu54_pilot_rental_review(const nu54_local_approval *approval, const nu54_v2_link *link,
    const nu54_v2_request *request, const uint8_t challenge[32], const uint8_t board_pin[32],
    uint64_t now, int (*hash)(const uint8_t *, size_t, uint8_t[32]),
    int (*sign)(const uint8_t[32], uint8_t[64]), uint8_t out[NU54_V2_RENTAL_REVIEW_RECEIPT_SIZE]);

#define NU54_W10_DECISION_SIZE 166
int nu54_pilot_rental_decision(const uint8_t review_hash[32], const uint8_t *payer_signature,
    int (*hash)(const uint8_t *, size_t, uint8_t[32]), int (*sign)(const uint8_t[32], uint8_t[64]),
    uint8_t out[NU54_W10_DECISION_SIZE]);

int nu54_pilot_display(const nu54_local_approval *, const uint8_t quote[NU54_V2_SIGNED_SIZE],
		       const uint8_t challenge[32], const uint8_t proof[64],
		       int (*verify)(const uint8_t *, size_t, const uint8_t [64]));

int nu54_pilot_complete(nu54_local_approval *approval, const nu54_v2_link *channel,
			 uint64_t generation, uint64_t now, uint32_t physical_prompt, nu54_local_button button,
			 const uint8_t payer[20], int (*sign)(void *, const uint8_t[32], uint8_t[64]),
			 void *context, uint8_t signature[65]);

#define NU54_PILOT_RECEIPTS 16
typedef struct {
	uint64_t generation;
	uint8_t request[16], digest[32], signature[65], complete;
} nu54_pilot_receipt;

/* read_slot returns 1 for an absent slot, 0 for an exact record, negative for an I/O failure. */
typedef int (*nu54_pilot_receipt_read)(void *context, unsigned slot, nu54_pilot_receipt *);
enum {
	NU54_PILOT_RECEIPT_ERROR = -1, NU54_PILOT_RECEIPT_RECORDED,
	NU54_PILOT_RECEIPT_NEW, NU54_PILOT_RECEIPT_FULL,
};

/* Slots are one-based so zero never identifies an unwritten slot. */
int nu54_pilot_receipt_find(nu54_pilot_receipt_read, void *context, uint64_t generation,
			    const uint8_t request[16], const uint8_t digest[32],
			    nu54_pilot_receipt *, unsigned *free_slot);

#define NU54_PILOT_REFUSALS 4
typedef struct {
	uint64_t expires;
	uint8_t request[16], digest[32];
} nu54_pilot_refusal;

typedef int (*nu54_pilot_refusal_read)(void *context, unsigned slot, nu54_pilot_refusal *);
int nu54_pilot_refusal_find(nu54_pilot_refusal_read, void *context, uint64_t earliest,
                            const uint8_t request[16], const uint8_t digest[32], unsigned *slot);
int nu54_pilot_refuse(nu54_local_approval *, const nu54_v2_link *,
                      const uint8_t quote[NU54_V2_SIGNED_SIZE], uint64_t now,
                      uint32_t prompt, nu54_pilot_hash,
                      nu54_pilot_refusal *);

#define NU54_PILOT_RECLAIM_BODY_SIZE (4 + 32 + 16 + 32 + 8 + 8 + 1)
#define NU54_PILOT_RECLAIM_HOLD_SECONDS 300
#define NU54_PILOT_RECLAIM_FINAL 1
#define NU54_PILOT_RECLAIM_NO_EFFECT 2

int nu54_pilot_reclaim_body(const uint8_t board[32], const nu54_pilot_receipt *,
			    const uint8_t quote[NU54_V2_UNSIGNED_SIZE], uint64_t earliest_now, uint8_t disposition,
			    int (*hash)(const uint8_t *, size_t, uint8_t [32]),
			    uint8_t out[NU54_PILOT_RECLAIM_BODY_SIZE]);

#endif
