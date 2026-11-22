#ifndef NU54_OWNER_LIMIT_CHANGE_H
#define NU54_OWNER_LIMIT_CHANGE_H
#include "nu54_owner_limits.h"
#include "nu54_local_approval.h"
#include <stddef.h>

#define NU54_LIMIT_CHANGE_DELAY 86400u
#define NU54_LIMIT_CHANGE_STATEMENT_SIZE 232u
/* Experimental pure transition. Caller must verify enrolled board, authenticated
 * session, physical GPIO source, trusted clock and persistence. The experimental
 * board adapter supplies those boundaries; the default qualification is disabled. */
typedef struct {
 uint64_t generation, revision, chain, per_transfer, daily, not_before, expires;
 uint8_t board[32], asset[20], destination[20], session[16], request[16], challenge[32], initialize;
} nu54_limit_change;
typedef int (*nu54_limit_hash)(const uint8_t *,size_t,uint8_t [32]);
typedef int (*nu54_limit_verify)(const uint8_t *,size_t,const uint8_t [64]);
int nu54_limit_change_statement(const nu54_owner_limits *,const nu54_limit_change *,uint8_t [NU54_LIMIT_CHANGE_STATEMENT_SIZE]);
int nu54_limit_change_stage(const nu54_owner_limits *,const nu54_limit_change *,uint64_t,uint64_t,uint32_t,
                            nu54_local_approval *,nu54_limit_hash,int (*)(void *,const uint8_t *,size_t),void *);
int nu54_limit_change_confirm(const nu54_owner_limits *,const nu54_limit_change *,uint64_t,uint64_t,uint32_t,
                              nu54_local_button,nu54_local_approval *,const uint8_t [64],nu54_limit_hash,
                              nu54_limit_verify,nu54_owner_limits *);
int nu54_limit_change_activate(const nu54_owner_limits *,uint64_t,uint64_t,nu54_owner_limits *);

#define NU54_LIMIT_RECIPIENT_STATEMENT_SIZE (176u + 2u * (1u + 20u * NU54_LIMIT_RECIPIENTS))
#define NU54_LIMIT_SET_STATEMENT_MAX (NU54_LIMIT_RECIPIENT_STATEMENT_SIZE + 16u)
typedef struct {
    uint64_t generation, revision, chain, not_before, expires, per_transfer, daily;
    uint8_t board[32], asset[20], session[16], request[16], challenge[32];
    nu54_limit_recipients recipients;
} nu54_limit_recipient_change;
int nu54_limit_recipient_statement(const nu54_owner_limits *, const nu54_limit_recipients *,
    const nu54_limit_recipient_change *, uint8_t [NU54_LIMIT_RECIPIENT_STATEMENT_SIZE]);
int nu54_limit_set_statement(const nu54_owner_limits *, const nu54_limit_recipients *,
    const nu54_limit_recipient_change *, uint8_t [NU54_LIMIT_SET_STATEMENT_MAX], size_t *);
int nu54_limit_recipient_intent(const nu54_owner_limits *, const nu54_limit_recipients *,
    const nu54_limit_recipient_change *, uint64_t, uint64_t, uint32_t, nu54_limit_hash, nu54_local_intent *);

#endif
