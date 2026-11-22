#ifndef NU54_OWNER_FINALITY_H
#define NU54_OWNER_FINALITY_H
#include "nu54_owner_limits.h"
#include "nu54_evm_tx.h"
#include <stddef.h>
/* Experimental v2 only. Must be held in one atomic, rollback-protected record.
 * No v1 migration or authority enrollment is implied by this declaration. */
#define NU54_FINALITY_BODY_SIZE 237u
#define NU54_FINALITY_REPLACEMENT_BODY_SIZE 269u
#define NU54_FINALITY_ALREADY 1
#define NU54_FINALITY_SUCCESS 1u
#define NU54_FINALITY_REVERT 2u
#define NU54_FINALITY_REPLACEMENT_SUCCESS 3u
#define NU54_FINALITY_REPLACEMENT_REVERT 4u
typedef struct {
 nu54_owner_limits policy;
 uint8_t genesis[32],signer[20],authority[65],last_claim[32];
 uint64_t high_nonce; uint8_t has_nonce;
 uint64_t nonces[NU54_LIMIT_RESERVATIONS];
} nu54_finality_state;
typedef struct {
 uint8_t board[32],genesis[32],signer[20],asset[20],request[16],digest[32],block_hash[32],replacement_digest[32];
 uint64_t generation,chain,nonce,block_height,finalized_height,block_time;
 uint8_t status;
} nu54_finality_claim;
typedef int (*nu54_finality_hash)(const uint8_t *,size_t,uint8_t [32]);
typedef int (*nu54_finality_verify)(const uint8_t authority[65],const uint8_t *body,size_t len,const uint8_t signature[64]);
int nu54_finality_body(const nu54_finality_claim *,uint8_t out[NU54_FINALITY_BODY_SIZE]);
int nu54_finality_replacement_body(const nu54_finality_claim *,uint8_t [NU54_FINALITY_REPLACEMENT_BODY_SIZE]);
int nu54_finality_replacement_parse(const uint8_t *,size_t,nu54_finality_claim *);
int nu54_finality_parse(const uint8_t *,size_t,nu54_finality_claim *);
int nu54_finality_reserve(const nu54_finality_state *,const nu54_limit_request *,const uint8_t signer[20],const uint8_t *raw,size_t raw_len,uint64_t lower,uint64_t upper,nu54_finality_state *);
int nu54_finality_apply_replacement(const nu54_finality_state *,const nu54_finality_claim *,const uint8_t *,size_t,const uint8_t *,size_t,
 const uint8_t [64],nu54_finality_verify,nu54_finality_hash,uint64_t,uint64_t,nu54_finality_state *);
int nu54_finality_apply(const nu54_finality_state *,const nu54_finality_claim *,const uint8_t *raw,size_t raw_len,
                        const uint8_t signature[64],nu54_finality_verify,nu54_finality_hash,uint64_t lower,uint64_t upper,nu54_finality_state *);
#endif
