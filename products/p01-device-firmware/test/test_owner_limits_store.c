#include "nu54_owner_limits_store.h"
#include <assert.h>
#include <string.h>

typedef struct {nu54_limits_record stored; uint64_t counter; int fail_write,fail_write_after,fail_advance,locked;} fixture;
static int lock(void *v) {fixture *f=v; if (f->locked) return -1; f->locked=1; return 0;}
static void unlock(void *v) {((fixture *)v)->locked=0;}
static int load(void *v,nu54_limits_record *out) {*out=((fixture *)v)->stored; return 0;}
static int write_record(void *v,const nu54_limits_record *r) {fixture *f=v; if(f->fail_write)return -1; f->stored=*r; return f->fail_write_after ? -1 : 0;}
static int read_counter(void *v,uint64_t *n) {*n=((fixture *)v)->counter; return 0;}
static int advance(void *v,uint64_t old) {fixture *f=v; if(f->fail_advance || f->counter!=old)return -1; f->counter++; return 0;}
int main(void) {
 fixture f={.stored={.version=1,.state={.generation=7,.revision=1,.per_transfer=10000000,.daily=30000000,.day=1,.last_lower=86400}},.counter=1};
 nu54_limits_port p={.context=&f,.lock=lock,.unlock=unlock,.load=load,.write=write_record,.read_counter=read_counter,.advance=advance};
 memset(f.stored.state.board,1,32); f.stored.state.chain=8283; memset(f.stored.state.asset,4,20); memset(f.stored.state.destination,5,20);
 nu54_limit_request req={.generation=7,.amount=9000000,.expires=172000}; memset(req.board,1,32); req.chain=8283; memset(req.asset,4,20); memset(req.destination,5,20); memset(req.id,1,16); memset(req.digest,2,32);
 nu54_limits_record before=f.stored; nu54_owner_limits state;
 nu54_limits_port unqualified=p; unqualified.advance=0;
 assert(nu54_limits_commit_reservation(&unqualified,&req,86410,86410,&state)==NU54_LIMIT_REJECT);
 f.fail_write=1;
 assert(nu54_limits_commit_reservation(&p,&req,86410,86410,&state)==NU54_LIMIT_REJECT);
 assert(f.counter==1 && f.stored.state.day_spent==0 && !f.locked);
 f.fail_write=0;
 assert(nu54_limits_commit_reservation(&p,&req,86410,86410,&state)==NU54_LIMIT_NEW);
 assert(f.counter==2 && state.day_spent==9000000 && !f.locked);
 assert(nu54_limits_commit_reservation(&p,&req,86410,86410,&state)==NU54_LIMIT_RETRY && f.counter==2);
 nu54_limits_record current=f.stored; f.stored=before;
 assert(nu54_limits_commit_reservation(&p,&req,86410,86410,&state)==NU54_LIMIT_REJECT);
 f.stored=current; f.fail_write_after=1; req.id[0]=3;
 assert(nu54_limits_commit_reservation(&p,&req,86410,86410,&state)==NU54_LIMIT_REJECT);
 f.fail_write_after=0;
 assert(nu54_limits_commit_reservation(&p,&req,86410,86410,&state)==NU54_LIMIT_REJECT);
 f.stored=current; f.fail_advance=1;
 assert(nu54_limits_commit_reservation(&p,&req,86410,86410,&state)==NU54_LIMIT_REJECT);
 f.fail_advance=0;
 assert(nu54_limits_commit_reservation(&p,&req,86410,86410,&state)==NU54_LIMIT_REJECT);
 assert(!f.locked);
 return 0;
}
