#ifndef NU54_OWNER_LIMITS_STORE_H
#define NU54_OWNER_LIMITS_STORE_H
#include "nu54_owner_limits.h"

typedef struct {uint64_t version; nu54_owner_limits state;} nu54_limits_record;
/* Contract: lock serializes ALL policy readers/writers; load/write use authenticated,
 * atomic protected storage; counter is independent, persistent, rollback-resistant,
 * and advance(old) atomically changes exactly old -> old+1. A port lacking any of
 * these properties is unqualified and MUST NOT call the signing path. RETRY is
 * only permission to return an existing durable receipt, never to sign again. */
typedef struct {
 void *context;
 int (*lock)(void *); void (*unlock)(void *);
 int (*load)(void *,nu54_limits_record *); int (*write)(void *,const nu54_limits_record *);
 int (*read_counter)(void *,uint64_t *); int (*advance)(void *,uint64_t);
} nu54_limits_port;
int nu54_limits_commit_reservation(const nu54_limits_port *,const nu54_limit_request *,
                                    uint64_t trusted_lower,uint64_t trusted_upper,nu54_owner_limits *out);
#endif
