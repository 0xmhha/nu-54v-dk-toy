#ifndef NU54_OWNER_IDLE_H
#define NU54_OWNER_IDLE_H

#include <stdbool.h>
#include <stdint.h>

#define NU54_OWNER_IDLE_SECONDS 120
#define NU54_OWNER_IDLE_OP 0x73
#define NU54_OWNER_IDLE_FLAGS 0x03 /* Owner idle lock and link-loss cancellation. */

typedef struct {
    uint64_t generation;
    int64_t last_ms;
} nu54_owner_idle;

static inline void nu54_owner_idle_clear(nu54_owner_idle *idle)
{
    *idle = (nu54_owner_idle){0};
}

static inline void nu54_owner_idle_start(nu54_owner_idle *idle, uint64_t generation, int64_t now)
{
    *idle = now < 0 ? (nu54_owner_idle){0} : (nu54_owner_idle){generation, now};
}

static inline bool nu54_owner_idle_live(const nu54_owner_idle *idle, uint64_t generation, int64_t now)
{
    return idle->generation && idle->generation == generation && now >= idle->last_ms &&
           now - idle->last_ms < NU54_OWNER_IDLE_SECONDS * 1000;
}

/* Invoke only for validated local physical input on the authenticated owner link. */
static inline bool nu54_owner_idle_activity(nu54_owner_idle *idle, uint64_t generation, int64_t now)
{
    if (!nu54_owner_idle_live(idle, generation, now)) return false;
    idle->last_ms = now;
    return true;
}

#endif
