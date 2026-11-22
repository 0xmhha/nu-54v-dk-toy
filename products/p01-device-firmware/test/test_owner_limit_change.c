#include "nu54_owner_limit_change.h"
#include <assert.h>
#include <string.h>
static int hash(const uint8_t *v,size_t n,uint8_t out[32]) {memset(out,0,32); for(size_t i=0;i<n;i++)out[i%32]^=(uint8_t)(v[i]+i);return 0;}
static int present(void *v,const uint8_t *p,size_t n) {(void)v;(void)p;return n==66?0:-1;}
static int verify(const uint8_t *v,size_t n,const uint8_t proof[64]) {(void)v;(void)n;return proof[0]==42?0:-1;}
int main(void) {
 nu54_owner_limits state={.generation=7,.revision=1,.chain=8283,.per_transfer=10000000,.daily=30000000,.day=1,.last_lower=86400};
 memset(state.board,1,32); memset(state.asset,4,20); memset(state.destination,5,20);
 nu54_limit_change c={.generation=7,.revision=1,.chain=8283,.per_transfer=12000000,.daily=30000000,.not_before=173400,.expires=87000};
 memset(c.board,1,32); memset(c.session,2,16); memset(c.request,3,16); memset(c.challenge,4,32); memset(c.asset,4,20); memset(c.destination,5,20);
 nu54_local_approval a={0}; uint8_t proof[64]={42}; nu54_owner_limits next;
 assert(nu54_limit_change_stage(&state,&c,86410,86410,1,&a,hash,present,0)==0);
 assert(nu54_limit_change_confirm(&state,&c,86420,86420,1,NU54_LOCAL_APPROVE,&a,proof,hash,verify,&next)==0);
 assert(next.per_transfer==10000000 && next.daily==30000000 && next.change.pending);
 assert(nu54_limit_change_activate(&next,173399,173399,&state)==-1);
 assert(nu54_limit_change_activate(&next,173400,173400,&state)==0);
 assert(state.per_transfer==12000000 && state.revision==2 && !state.change.pending);
 assert(nu54_limit_change_stage(&state,&c,86410,86410,2,&a,hash,present,0)==-1);
 state=next; state.change.pending=0; a=(nu54_local_approval){0};
 assert(nu54_limit_change_stage(&state,&c,86420,86420,3,&a,hash,present,0)==0);
 nu54_limit_change altered=c; altered.per_transfer++;
 assert(nu54_limit_change_confirm(&state,&altered,86420,86420,3,NU54_LOCAL_APPROVE,&a,proof,hash,verify,&next)==-1);
 a=(nu54_local_approval){0};
 assert(nu54_limit_change_stage(&state,&c,86420,86420,3,&a,hash,present,0)==0);
 assert(nu54_limit_change_confirm(&state,&c,86420,86420,3,NU54_LOCAL_REJECT,&a,proof,hash,verify,&next)==-1);
 assert(!a.pending && state.per_transfer==10000000);
 a=(nu54_local_approval){0};
 assert(nu54_limit_change_stage(&state,&c,86420,86420,4,&a,hash,present,0)==0);
 proof[0]=0;
 assert(nu54_limit_change_confirm(&state,&c,86420,86420,4,NU54_LOCAL_APPROVE,&a,proof,hash,verify,&next)==-1);
 assert(state.per_transfer==10000000);
 /* Same-day finalized spend cannot disappear when lowering the daily cap. */
 nu54_owner_limits pending=state;pending.change.pending=1;pending.change.per_transfer=9000000;pending.change.daily=10000000;
 pending.change.not_before=172800;memset(pending.change.destination,5,20);memset(pending.change.request,6,16);memset(pending.change.statement_digest,7,32);
 pending.day=2;pending.day_spent=20000000;pending.last_lower=172800;
 assert(nu54_limit_change_activate(&pending,172801,172801,&next)==-1);
 assert(nu54_limit_change_activate(&pending,259199,259200,&next)==-1);
 assert(nu54_limit_change_activate(&pending,259200,259200,&next)==0);
 assert(next.day==3 && next.day_spent==0 && next.daily==10000000);
 return 0;
}
