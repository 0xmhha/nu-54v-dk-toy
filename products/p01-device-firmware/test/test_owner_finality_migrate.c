#define main unused_finality_test_main
#include "test_owner_finality.c"
#undef main
#include "nu54_owner_finality_migrate.h"
static int qualified(const nu54_owner_limits *old,const uint8_t signer[20],uint64_t *high,int *has,uint8_t journal[32]){if(old->generation!=7||signer[0]!=9)return -1;*high=8;*has=1;memset(journal,7,32);return 0;}
static int unqualified(const nu54_owner_limits *old,const uint8_t signer[20],uint64_t *high,int *has,uint8_t journal[32]){(void)old;(void)signer;(void)high;(void)has;(void)journal;return -1;}
typedef struct {nu54_owner_limits old;nu54_migration_prepared prepared;int fenced,active,fail_activate,locked,journal_changed;} migration_fixture;
static int mlock(void *p){migration_fixture *f=p;if(f->locked)return -1;f->locked=1;return 0;}
static void munlock(void *p){((migration_fixture *)p)->locked=0;}
static int mread(void *p,nu54_owner_limits *out){*out=((migration_fixture *)p)->old;return 0;}
static int mprepared(void *p,nu54_migration_prepared *out){*out=((migration_fixture *)p)->prepared;return 0;}
static int mlineage(void *p,const nu54_owner_limits *old,const uint8_t signer[20],uint64_t *high,int *has,uint8_t journal[32]){migration_fixture *f=p;int rc=qualified(old,signer,high,has,journal);if(f->journal_changed)journal[0]^=1;return rc;}
static int mfence(void *p,const nu54_owner_limits *old,const uint8_t journal[32]){migration_fixture *f=p;if(memcmp(old,&f->old,sizeof(*old))||memcmp(journal,f->prepared.journal_digest,32))return -1;f->fenced=1;return 0;}
static int mactivate(void *p,const nu54_migration_prepared *prepared){migration_fixture *f=p;if(!f->fenced||memcmp(prepared,&f->prepared,sizeof(*prepared))||f->fail_activate)return -1;f->active=1;return 0;}
int main(void){
 uint8_t raw[256];size_t n=hex("02f86782205b010101830186a094040404040404040404040404040404040404040480b844a9059cbb00000000000000000000000005050505050505050505050505050505050505050000000000000000000000000000000000000000000000000000000000000009c0",raw);
 nu54_evm_policy pol={.chain=8283,.total_fee_limit=1000000};memset(pol.token,4,20);nu54_evm_intent tx;assert(!nu54_evm_decode(raw,n,&pol,&tx));
 nu54_owner_limits old={.generation=7,.revision=3,.chain=8283,.per_transfer=10,.daily=30,.day=1,.day_spent=9,.last_lower=86410,.count=1};memset(old.board,1,32);memset(old.asset,4,20);memset(old.destination,5,20);old.reservations[0].amount=9;old.reservations[0].expires=87000;memset(old.reservations[0].id,3,16);memcpy(old.reservations[0].digest,tx.digest,32);
 nu54_finality_state profile={.policy=old},out;memset(profile.signer,9,20);memset(profile.genesis,8,32);profile.authority[0]=4;profile.authority[1]=1;
 nu54_migration_original original={raw,n};uint8_t journal[32];
 assert(nu54_finality_migration_prepare(&old,&profile,&original,1,unqualified,&out,journal)==-1);
 assert(nu54_finality_migration_prepare(&old,&profile,NULL,0,qualified,&out,journal)==-1);
 raw[7]^=1;assert(nu54_finality_migration_prepare(&old,&profile,&original,1,qualified,&out,journal)==-1);raw[7]^=1;
 assert(!nu54_finality_migration_prepare(&old,&profile,&original,1,qualified,&out,journal));
 assert(out.policy.revision==3 && out.policy.day_spent==9 && out.policy.count==1 && out.nonces[0]==1 && out.high_nonce==8);
 assert(!memcmp(&out.policy,&old,sizeof(old)));
 migration_fixture fixture={.old=old,.prepared={.state=out},.fail_activate=1};memcpy(fixture.prepared.journal_digest,journal,32);
 nu54_migration_port port={.context=&fixture,.lock=mlock,.unlock=munlock,.read_v1=mread,.read_prepared=mprepared,.check_lineage=mlineage,.fence=mfence,.activate=mactivate};
 fixture.journal_changed=1;assert(nu54_finality_migration_resume(&port)==-1 && !fixture.fenced && !fixture.active);
 fixture.journal_changed=0;fixture.old.day_spent++;assert(nu54_finality_migration_resume(&port)==-1 && !fixture.fenced);fixture.old=old;
 assert(nu54_finality_migration_resume(&port)==-1 && fixture.fenced && !fixture.active); /* crash or lost activation */
 fixture.fail_activate=0;assert(!nu54_finality_migration_resume(&port) && fixture.fenced && fixture.active);
 assert(!memcmp(&fixture.old,&old,sizeof(old)));
 old.change.pending=1;assert(nu54_finality_migration_prepare(&old,&profile,&original,1,qualified,&out,journal)==-1);
 return 0;
}
