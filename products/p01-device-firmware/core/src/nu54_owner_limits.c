#include "nu54_owner_limits.h"
#include <stddef.h>
#include <string.h>

static int nonzero(const uint8_t *v, size_t size) {
    uint8_t result=0; for (size_t i=0;i<size;i++) result |= v[i]; return result != 0;
}
int nu54_limits_recipients_valid(const nu54_limit_recipients *set) {
    if (!set || set->count > NU54_LIMIT_RECIPIENTS) return 0;
    for (unsigned i=0;i<NU54_LIMIT_RECIPIENTS;i++) {
        if (i >= set->count) {
            if (nonzero(set->addresses[i],20)) return 0;
        } else if (!nonzero(set->addresses[i],20) ||
                   (i && memcmp(set->addresses[i-1],set->addresses[i],20)>=0)) return 0;
    }
    return 1;
}

static int allowed(const nu54_owner_limits *current, const nu54_limit_recipients *set,
                   const uint8_t destination[20]) {
    if (!set) return nonzero(current->destination,20) && !memcmp(current->destination,destination,20);
    if (!nu54_limits_recipients_valid(set)) return 0;
    for (unsigned i=0;i<set->count;i++)
        if (!memcmp(set->addresses[i],destination,20)) return 1;
    return 0;
}

int nu54_limits_reserve_recipients(const nu54_owner_limits *current, const nu54_limit_recipients *set,
                         const nu54_limit_request *request,
                         uint64_t lower, uint64_t upper, nu54_owner_limits *next) {
    if (!current || !request || !next || current == next || !current->generation || !current->revision ||
        !nonzero(current->board,sizeof(current->board)) || !current->per_transfer || current->per_transfer > current->daily ||
        current->day_spent > current->daily || current->count > NU54_LIMIT_RESERVATIONS ||
        !lower || lower > upper || lower < current->last_lower || lower / 86400 != upper / 86400 ||
        request->generation != current->generation || memcmp(current->board,request->board,sizeof(current->board)) ||
        !current->chain || request->chain != current->chain ||
        !nonzero(current->asset,sizeof(current->asset)) ||
        memcmp(current->asset,request->asset,sizeof(current->asset)) ||
        !allowed(current,set,request->destination) ||
        !request->amount || request->amount > current->per_transfer ||
        request->expires <= upper || !nonzero(request->id,sizeof(request->id)) ||
        !nonzero(request->digest,sizeof(request->digest))) return NU54_LIMIT_REJECT;
    uint64_t sum=0;
    for (uint32_t i=0;i<current->count;i++) {
        const nu54_limit_reservation *old=&current->reservations[i];
        if (!old->amount || old->amount > current->per_transfer ||
            !old->expires || !nonzero(old->id,sizeof(old->id)) || !nonzero(old->digest,sizeof(old->digest)) ||
            old->amount > current->day_spent - sum) return NU54_LIMIT_REJECT;
        sum += old->amount;
        for (uint32_t j=0;j<i;j++) if (!memcmp(old->id,current->reservations[j].id,sizeof(old->id)))
            return NU54_LIMIT_REJECT;
    }
    uint64_t day=lower/86400;
    if (day < current->day) return NU54_LIMIT_REJECT;
    /* Every unresolved signature can still settle today. Carry its full amount
     * into each new day; expiry alone never removes or releases a reservation. */
    for (uint32_t i=0;i<current->count;i++) {
        const nu54_limit_reservation *old=&current->reservations[i];
        if (!memcmp(old->id,request->id,sizeof(old->id))) {
            if (old->amount != request->amount ||
                old->expires != request->expires || memcmp(old->digest,request->digest,sizeof(old->digest)))
                return NU54_LIMIT_REJECT;
            *next=*current; next->last_lower=lower; return NU54_LIMIT_RETRY;
        }
    }
    uint64_t spent=day == current->day ? current->day_spent : sum;
    if (current->count == NU54_LIMIT_RESERVATIONS || request->amount > current->daily - spent)
        return NU54_LIMIT_REJECT;
    *next=*current;
    next->day=day; next->day_spent=spent;
    next->last_lower=lower;
    next->day_spent += request->amount;
    nu54_limit_reservation *reservation=&next->reservations[next->count++];
    reservation->amount=request->amount; reservation->expires=request->expires;
    memcpy(reservation->id,request->id,sizeof(reservation->id));
    memcpy(reservation->digest,request->digest,sizeof(reservation->digest));
    return NU54_LIMIT_NEW;
}

int nu54_limits_reserve(const nu54_owner_limits *current, const nu54_limit_request *request,
                        uint64_t lower, uint64_t upper, nu54_owner_limits *next) {
    return nu54_limits_reserve_recipients(current,NULL,request,lower,upper,next);
}
