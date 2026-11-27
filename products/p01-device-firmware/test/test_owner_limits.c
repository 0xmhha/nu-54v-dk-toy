#include "nu54_owner_limits.h"
#include <assert.h>
#include <string.h>

int main(void) {
    nu54_owner_limits state = {.generation=7,.revision=1,.per_transfer=10000000,.daily=30000000,.day=1,.last_lower=86400};
    nu54_owner_limits empty_next;
    memset(state.board,1,32); state.chain=8283; memset(state.asset,4,20); memset(state.destination,5,20);
    nu54_limit_request tomorrow = {.generation=7,.amount=1000000,.expires=259000};
    memset(tomorrow.board,1,32); tomorrow.chain=8283; memset(tomorrow.asset,4,20); memset(tomorrow.destination,5,20);
    memset(tomorrow.id,9,sizeof(tomorrow.id)); memset(tomorrow.digest,8,sizeof(tomorrow.digest));
    assert(nu54_limits_reserve(&state,&tomorrow,172800,172800,&empty_next)==NU54_LIMIT_NEW);
    assert(empty_next.day==2 && empty_next.day_spent==1000000);
    nu54_limit_request a = {.generation=7,.amount=9000000,.expires=172000};
    memset(a.board,1,32); a.chain=8283; memset(a.asset,4,20); memset(a.destination,5,20);
    memset(a.id,1,sizeof(a.id)); memset(a.digest,2,sizeof(a.digest));
    nu54_owner_limits next;
    assert(nu54_limits_reserve(&state,&a,86410,86410,&next)==NU54_LIMIT_NEW);
    assert(next.day_spent==9000000);
    assert(nu54_limits_reserve(&next,&a,86410,86410,&state)==NU54_LIMIT_RETRY);
    assert(state.day_spent==9000000);
    nu54_owner_limits corrupt=next;
    corrupt.day_spent=0;
    assert(nu54_limits_reserve(&corrupt,&a,86410,86410,&state)==NU54_LIMIT_REJECT);
    nu54_limit_request altered=a; altered.amount++;
    assert(nu54_limits_reserve(&next,&altered,86410,86410,&state)==NU54_LIMIT_REJECT);
    assert(nu54_limits_reserve(&next,&a,86399,86410,&state)==NU54_LIMIT_REJECT);
    assert(nu54_limits_reserve(&next,&a,0,0,&state)==NU54_LIMIT_REJECT);
    nu54_owner_limits old_clock=next; old_clock.last_lower=86400*2;
    assert(nu54_limits_reserve(&old_clock,&a,86410,86410,&state)==NU54_LIMIT_REJECT);
    nu54_limit_request wrong_asset=a; wrong_asset.asset[0]=9;
    assert(nu54_limits_reserve(&next,&wrong_asset,86410,86410,&state)==NU54_LIMIT_REJECT);
    nu54_limit_request wrong_destination=a; wrong_destination.destination[0]=9;
    assert(nu54_limits_reserve(&next,&wrong_destination,86410,86410,&state)==NU54_LIMIT_REJECT);
    nu54_limit_request mismatch=a; mismatch.generation=8;
    assert(nu54_limits_reserve(&next,&mismatch,86410,86410,&state)==NU54_LIMIT_REJECT);
    nu54_owner_limits near=next; near.day_spent=29000000;
    nu54_limit_request over=a; memset(over.id,4,sizeof(over.id)); over.amount=2000000;
    assert(nu54_limits_reserve(&near,&over,86410,86410,&state)==NU54_LIMIT_REJECT);
    nu54_limit_request other=a; memset(other.id,3,sizeof(other.id)); other.amount=10000001;
    assert(nu54_limits_reserve(&next,&other,86410,86410,&state)==NU54_LIMIT_REJECT);
    other.amount=9000000;
    assert(nu54_limits_reserve(&next,&other,172799,172800,&state)==NU54_LIMIT_REJECT);
    other.expires=259000;
    /* Earlier-day non-pending charges are not repeated; unresolved amounts
     * remain charged in full, even if their request expiry has passed. */
    near.day_spent=29000000;
    assert(nu54_limits_reserve(&near,&other,172800,172800,&state)==NU54_LIMIT_NEW);
    assert(state.day_spent==18000000 && state.count==2);
    assert(nu54_limits_reserve(&next,&other,172800,172800,&state)==NU54_LIMIT_NEW);
    assert(state.day==2 && state.day_spent==18000000 && state.count==2);
    assert(!memcmp(state.reservations[0].digest,next.reservations[0].digest,32));
    nu54_limit_request third=other;memset(third.id,7,sizeof(third.id));third.amount=10000000;
    assert(nu54_limits_reserve(&state,&third,172801,172801,&next)==NU54_LIMIT_NEW);
    assert(next.day_spent==28000000);
    nu54_limit_request fourth=third;memset(fourth.id,8,sizeof(fourth.id));fourth.amount=3000000;
    assert(nu54_limits_reserve(&next,&fourth,172802,172802,&state)==NU54_LIMIT_REJECT);
    fourth.amount=1000000;fourth.expires=345000;
    assert(nu54_limits_reserve(&next,&fourth,259200,259200,&state)==NU54_LIMIT_NEW);
    assert(state.day==3 && state.day_spent==29000000 && state.count==4);
    /* Unresolved IDs survive indefinitely; a full table never silently evicts. */
    nu54_owner_limits full={.generation=7,.revision=1,.per_transfer=10,.daily=30,.day=1,.last_lower=86400,.chain=8283,.count=16,.day_spent=16};
    memset(full.board,1,32);memset(full.asset,4,20);memset(full.destination,5,20);
    for(unsigned i=0;i<16;i++){full.reservations[i].amount=1;full.reservations[i].expires=172000;full.reservations[i].id[0]=(uint8_t)(i+1);full.reservations[i].digest[0]=1;}
    nu54_limit_request blocked=tomorrow;blocked.amount=1;memset(blocked.id,99,16);
    assert(nu54_limits_reserve(&full,&blocked,172800,172800,&state)==NU54_LIMIT_REJECT);
    return 0;
}
