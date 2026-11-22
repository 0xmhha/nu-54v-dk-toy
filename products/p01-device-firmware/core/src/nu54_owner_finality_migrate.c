#include "nu54_owner_finality_migrate.h"
#include <string.h>
static int nonzero(const uint8_t *p,size_t n){uint8_t x=0;for(size_t i=0;i<n;i++)x|=p[i];return x!=0;}
int nu54_finality_migration_prepare(const nu54_owner_limits *old,const nu54_finality_state *profile,
 const nu54_migration_original *originals,size_t count,nu54_migration_lineage lineage,nu54_finality_state *next,uint8_t journal_digest[32]) {
 if(!old||!profile||!lineage||!next||!journal_digest||old==&next->policy||profile==next||count!=old->count||
    count>NU54_LIMIT_RESERVATIONS||(count&&!originals)||old->change.pending||!old->generation||!old->revision||
    old->chain!=8283||!old->per_transfer||old->per_transfer>old->daily||old->day_spent>old->daily||
    !nonzero(old->board,32)||!nonzero(old->asset,20)||!nonzero(old->destination,20)||
    profile->policy.generation!=old->generation||profile->policy.chain!=old->chain||
    memcmp(profile->policy.board,old->board,32)||memcmp(profile->policy.asset,old->asset,20)||
    memcmp(profile->policy.destination,old->destination,20)||profile->policy.per_transfer!=old->per_transfer||
    profile->policy.daily!=old->daily||!nonzero(profile->signer,20)||!nonzero(profile->genesis,32)||
    profile->authority[0]!=4||!nonzero(profile->authority+1,64))return -1;
 uint64_t high=0;int has=0;uint8_t journal[32];if(lineage(old,profile->signer,&high,&has,journal)||!nonzero(journal,32)|| (has!=0 && has!=1) || (count&&!has))return -1;
 nu54_finality_state prepared=*profile;prepared.policy=*old;prepared.has_nonce=(uint8_t)has;prepared.high_nonce=high;
 memset(prepared.nonces,0,sizeof(prepared.nonces));memset(prepared.last_claim,0,sizeof(prepared.last_claim));
 nu54_evm_policy policy={.chain=8283,.total_fee_limit=UINT64_MAX};memcpy(policy.token,old->asset,20);uint64_t sum=0;
 for(size_t i=0;i<count;i++) {
   const nu54_limit_reservation *r=&old->reservations[i];
   if(!r->amount||r->amount>old->per_transfer||!r->expires||!nonzero(r->id,16)||!nonzero(r->digest,32)||
      r->amount>old->day_spent-sum||!originals[i].bytes||!originals[i].length)return -1;
   sum+=r->amount;for(size_t j=0;j<i;j++)if(!memcmp(r->id,old->reservations[j].id,16))return -1;
   nu54_evm_intent intent;if(nu54_evm_decode(originals[i].bytes,originals[i].length,&policy,&intent)||
      memcmp(intent.digest,r->digest,32)||memcmp(intent.recipient,old->destination,20)||intent.nonce>high)return -1;
   uint64_t amount=0;for(size_t j=0;j<24;j++)if(intent.amount[j])return -1;
   for(size_t j=24;j<32;j++)amount=(amount<<8)|intent.amount[j];
   if(amount!=r->amount)return -1;
   prepared.nonces[i]=intent.nonce;
 }
 *next=prepared;memcpy(journal_digest,journal,32);return 0;
}

int nu54_finality_migration_resume(const nu54_migration_port *p) {
 if(!p||!p->lock||!p->unlock||!p->read_v1||!p->read_prepared||!p->check_lineage||!p->fence||!p->activate||p->lock(p->context))return -1;
 int result=-1;nu54_owner_limits old;nu54_migration_prepared prepared;uint64_t high=0;int has=0;uint8_t journal[32];
 if(p->read_v1(p->context,&old)||p->read_prepared(p->context,&prepared)||memcmp(&old,&prepared.state.policy,sizeof(old))||
    p->check_lineage(p->context,&old,prepared.state.signer,&high,&has,journal)||!nonzero(journal,32)||
    has!=prepared.state.has_nonce||high!=prepared.state.high_nonce||memcmp(journal,prepared.journal_digest,32))goto done;
 /* After fence commit, V1 can no longer sign even if activation fails. */
 if(p->fence(p->context,&old,journal)||p->activate(p->context,&prepared))goto done;
 result=0;
done:p->unlock(p->context);return result;
}
