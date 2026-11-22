#ifndef DEVICE_OWNER_LIMITS_PS_H
#define DEVICE_OWNER_LIMITS_PS_H
#include "nu54_owner_limit_change.h"
#include "nu54_quote_v2_generated.h"
/* Platform must attest trusted secure boot, a sufficiently advanced anti-downgrade
 * counter, protected debug/lifecycle and provisioned TF-M PS rollback state. The
 * default implementation returns false; compile flags alone never qualify it. */
bool device_owner_limits_platform_qualified(void);
/* Experimental compile-only adapter. Callers must separately establish trusted
 * clock, owner, secure boot and storage qualification before any use. */
/* NEW is the only permission to produce a signature; RETRY must return a
 * separately persisted matching receipt or fail without signing. */
int device_owner_limits_authorize_ps(const uint8_t [32],uint64_t,uint64_t,const uint8_t [20],const uint8_t [20],
                                     const uint8_t [32],const uint8_t [16],const uint8_t [32],uint64_t,uint64_t,uint64_t);
int device_owner_limits_authorize_pilot_ps(const uint8_t [32],uint64_t,const uint8_t [NU54_V2_UNSIGNED_SIZE],
                                           const uint8_t [16],const uint8_t [32],uint64_t,uint64_t,uint64_t);
/* Called only while creating a genuinely new seed after checking the existing
 * HD seed and hardware witness are both absent; never for seed import. */
int device_owner_limits_mark_fresh_ps(const uint8_t [32],uint64_t);
int device_owner_limits_initialize_stage_ps(const nu54_limit_change *,uint64_t,uint64_t,uint32_t,nu54_local_approval *,nu54_limit_hash,
                                      int (*)(void *,const uint8_t *,size_t),void *);
int device_owner_limits_initialize_confirm_ps(const nu54_limit_change *,uint64_t,uint64_t,uint32_t,nu54_local_button,
                                        nu54_local_approval *,const uint8_t [64],nu54_limit_hash,nu54_limit_verify);
int device_owner_limits_claim_fresh_finality_ps(const uint8_t [32],uint64_t);
bool device_owner_limits_is_fresh_ps(const uint8_t [32],uint64_t);
/* Check protected policy presence without projecting the recipient sets.
 * Transfer callers still have to reserve against the active set. */
bool device_owner_limits_ready_ps(const uint8_t board[32], uint64_t generation);
int device_owner_limits_read_ps(const uint8_t [32],uint64_t,nu54_owner_limits *);
int device_owner_limits_reserve_ps(const nu54_limit_request *, uint64_t, uint64_t,
                                   nu54_owner_limits *);
int device_owner_limits_stage_ps(const nu54_limit_change *,uint64_t,uint64_t,uint32_t,nu54_local_approval *,nu54_limit_hash,
                                 int (*)(void *,const uint8_t *,size_t),void *);
int device_owner_limits_confirm_ps(const nu54_limit_change *,uint64_t,uint64_t,uint32_t,nu54_local_button,
                                   nu54_local_approval *,const uint8_t [64],nu54_limit_hash,nu54_limit_verify);
int device_owner_limits_activate_ps(uint64_t,uint64_t);
/* Recipient-set management for an already enrolled first-EVM policy. Callers
 * must verify the owner session and bind the independent display to the N54R
 * statement before local approval. Legacy wire clients cannot read a set which
 * cannot be represented as a singleton. */
int device_owner_limits_read_recipients_ps(const uint8_t [32],uint64_t,nu54_owner_limits *,
                                          nu54_limit_recipients *,nu54_limit_recipients *);
enum nu54_limit_pending_kind { NU54_LIMIT_PENDING_NONE, NU54_LIMIT_PENDING_LEGACY,
    NU54_LIMIT_PENDING_RECIPIENTS, NU54_LIMIT_PENDING_CAPS };
int device_owner_limits_read_set_ps(const uint8_t [32], uint64_t, nu54_owner_limits *,
    nu54_limit_recipients *, nu54_limit_recipients *, enum nu54_limit_pending_kind *);
int device_owner_limits_activate_set_ps(const uint8_t [32], uint64_t, uint64_t, const uint8_t [16],
    uint64_t, uint64_t);
int device_owner_limits_activate_recipients_ps(const uint8_t [32],uint64_t,uint64_t,uint64_t);
int device_owner_limits_migrate_recipients_ps(const uint8_t [32],uint64_t);
int device_owner_limits_stage_recipients_ps(const nu54_limit_recipient_change *,uint64_t,uint64_t,uint32_t,
    nu54_local_approval *,nu54_limit_hash,int (*)(void *,const uint8_t *,size_t),void *);
int device_owner_limits_confirm_recipients_ps(const nu54_limit_recipient_change *,uint64_t,uint64_t,uint32_t,
    nu54_local_button,nu54_local_approval *,const uint8_t [64],nu54_limit_hash,nu54_limit_verify);
#endif
