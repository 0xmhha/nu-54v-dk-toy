#ifndef NU54_OWNER_LIMITS_H
#define NU54_OWNER_LIMITS_H
#include <stdint.h>

/* Pure transition used by the experimental PS adapter: callers must authenticate
 * clock and generation, then persist the reservation before signing. Unknown
 * outcomes remain reserved and are carried in full to each later UTC day. */
#define NU54_LIMIT_RESERVATIONS 16
#define NU54_LIMIT_RECIPIENTS 8
#define NU54_LIMIT_NEW 0
#define NU54_LIMIT_RETRY 1
#define NU54_LIMIT_REJECT -1

typedef struct {
    uint8_t count;
    uint8_t addresses[NU54_LIMIT_RECIPIENTS][20];
} nu54_limit_recipients;
int nu54_limits_recipients_valid(const nu54_limit_recipients *);

typedef struct {
    uint64_t generation, amount, expires, chain;
    uint8_t board[32], id[16], digest[32], asset[20], destination[20];
} nu54_limit_request;
typedef struct {
    uint64_t amount, expires;
    uint8_t id[16], digest[32];
} nu54_limit_reservation;
typedef struct {
    uint64_t generation, revision, per_transfer, daily, day, day_spent, last_lower, chain;
    uint8_t board[32], asset[20], destination[20];
    struct { uint64_t per_transfer, daily, not_before; uint8_t destination[20], request[16], statement_digest[32], pending; } change;
    uint32_t count;
    nu54_limit_reservation reservations[NU54_LIMIT_RESERVATIONS];
} nu54_owner_limits;

int nu54_limits_reserve(const nu54_owner_limits *current, const nu54_limit_request *request,
                         uint64_t trusted_lower, uint64_t trusted_upper, nu54_owner_limits *next);
int nu54_limits_reserve_recipients(const nu54_owner_limits *, const nu54_limit_recipients *,
                                    const nu54_limit_request *, uint64_t, uint64_t, nu54_owner_limits *);
#endif
