#include "nu54_owner_finality.h"
#include <string.h>
static int nonzero(const uint8_t *p,size_t n){uint8_t v=0;for(size_t i=0;i<n;i++)v|=p[i];return v!=0;}
static void put64(uint8_t *p,uint64_t v){for(int i=7;i>=0;i--){p[i]=(uint8_t)v;v>>=8;}}
int nu54_finality_body(const nu54_finality_claim *c,uint8_t out[NU54_FINALITY_BODY_SIZE]) {
 if(!c || !out)return -1;
 size_t i=0;
 memcpy(out+i,"N54F",4);i+=4;memcpy(out+i,c->board,32);i+=32;memcpy(out+i,c->genesis,32);i+=32;
 memcpy(out+i,c->signer,20);i+=20;memcpy(out+i,c->asset,20);i+=20;memcpy(out+i,c->request,16);i+=16;
 memcpy(out+i,c->digest,32);i+=32;memcpy(out+i,c->block_hash,32);i+=32;
 put64(out+i,c->generation);i+=8;put64(out+i,c->chain);i+=8;put64(out+i,c->nonce);i+=8;
 put64(out+i,c->block_height);i+=8;put64(out+i,c->finalized_height);i+=8;put64(out+i,c->block_time);i+=8;out[i++]=c->status;
 return i==NU54_FINALITY_BODY_SIZE?0:-1;
}
int nu54_finality_replacement_body(const nu54_finality_claim *c,uint8_t out[NU54_FINALITY_REPLACEMENT_BODY_SIZE]) {
 if(!c||!out||nu54_finality_body(c,out))return -1;
 memcpy(out,"N54R",4);memcpy(out+NU54_FINALITY_BODY_SIZE,c->replacement_digest,32);return 0;
}
static uint64_t get64(const uint8_t *p){uint64_t v=0;for(size_t i=0;i<8;i++)v=(v<<8)|p[i];return v;}
int nu54_finality_parse(const uint8_t *body,size_t n,nu54_finality_claim *c) {
 if(!body || !c || n!=NU54_FINALITY_BODY_SIZE || memcmp(body,"N54F",4))return -1;
 size_t i=4;memcpy(c->board,body+i,32);i+=32;memcpy(c->genesis,body+i,32);i+=32;memcpy(c->signer,body+i,20);i+=20;
 memcpy(c->asset,body+i,20);i+=20;memcpy(c->request,body+i,16);i+=16;memcpy(c->digest,body+i,32);i+=32;memcpy(c->block_hash,body+i,32);i+=32;
 c->generation=get64(body+i);i+=8;c->chain=get64(body+i);i+=8;c->nonce=get64(body+i);i+=8;c->block_height=get64(body+i);i+=8;
 c->finalized_height=get64(body+i);i+=8;c->block_time=get64(body+i);i+=8;c->status=body[i++];return i==n?0:-1;
}
int nu54_finality_replacement_parse(const uint8_t *body,size_t n,nu54_finality_claim *c) {
 if(!body||!c||n!=NU54_FINALITY_REPLACEMENT_BODY_SIZE||memcmp(body,"N54R",4))return -1;
 uint8_t original[NU54_FINALITY_BODY_SIZE];memcpy(original,body,sizeof(original));memcpy(original,"N54F",4);
 if(nu54_finality_parse(original,sizeof(original),c))return -1;
 memcpy(c->replacement_digest,body+sizeof(original),32);return 0;
}
static int state_valid(const nu54_finality_state *v) {
 return v && v->policy.chain==8283 && v->policy.count<=NU54_LIMIT_RESERVATIONS &&
   nonzero(v->genesis,32) && nonzero(v->signer,20) && v->authority[0]==4 && nonzero(v->authority+1,64);
}
int nu54_finality_reserve(const nu54_finality_state *v,const nu54_limit_request *r,const uint8_t signer[20],const uint8_t *raw,size_t raw_len,uint64_t lower,uint64_t upper,nu54_finality_state *next) {
 if(!state_valid(v) || !r || !signer || memcmp(signer,v->signer,20) || !raw || !next || v==next)return NU54_LIMIT_REJECT;
 nu54_evm_policy txpolicy={.chain=8283,.total_fee_limit=UINT64_MAX};memcpy(txpolicy.token,v->policy.asset,20);
 nu54_evm_intent intent;if(nu54_evm_decode(raw,raw_len,&txpolicy,&intent)||memcmp(intent.digest,r->digest,32)||
   memcmp(intent.recipient,r->destination,20))return NU54_LIMIT_REJECT;
 uint64_t amount=0;for(size_t i=0;i<24;i++)if(intent.amount[i])return NU54_LIMIT_REJECT;
 for(size_t i=24;i<32;i++)amount=(amount<<8)|intent.amount[i];
 if(amount!=r->amount || (v->has_nonce && intent.nonce<=v->high_nonce))return NU54_LIMIT_REJECT;
 uint64_t nonce=intent.nonce;
 nu54_owner_limits policy;int status=nu54_limits_reserve(&v->policy,r,lower,upper,&policy);
 if(status!=NU54_LIMIT_NEW)return status;
 *next=*v;next->policy=policy;next->nonces[policy.count-1]=nonce;next->high_nonce=nonce;next->has_nonce=1;return status;
}
static int apply(const nu54_finality_state *v,const nu54_finality_claim *c,const uint8_t *raw,size_t raw_len,
 const uint8_t *replacement,size_t replacement_len,const uint8_t signature[64],nu54_finality_verify verify,nu54_finality_hash hash,uint64_t lower,uint64_t upper,nu54_finality_state *next) {
 int replacing=replacement!=NULL;
 if(!state_valid(v)||!c||!raw||!signature||!verify||!hash||!next||v==next||!lower||lower>upper||lower<v->policy.last_lower||lower/86400!=upper/86400||lower/86400<v->policy.day||!c->block_time||c->block_time>lower||!c->block_height||c->finalized_height<c->block_height||
    !nonzero(c->block_hash,32)||c->generation!=v->policy.generation||c->chain!=8283||memcmp(c->board,v->policy.board,32)||
    memcmp(c->genesis,v->genesis,32)||memcmp(c->signer,v->signer,20)||memcmp(c->asset,v->policy.asset,20)||
    (replacing ? (c->status!=NU54_FINALITY_REPLACEMENT_SUCCESS && c->status!=NU54_FINALITY_REPLACEMENT_REVERT) :
     (c->status!=NU54_FINALITY_SUCCESS && c->status!=NU54_FINALITY_REVERT)))return -1;
 uint8_t body[NU54_FINALITY_REPLACEMENT_BODY_SIZE],claim_hash[32];size_t body_len=replacing?NU54_FINALITY_REPLACEMENT_BODY_SIZE:NU54_FINALITY_BODY_SIZE;
 if((replacing?nu54_finality_replacement_body(c,body):nu54_finality_body(c,body))||verify(v->authority,body,body_len,signature)||hash(body,body_len,claim_hash)||!nonzero(claim_hash,32))return -1;
 nu54_evm_policy policy={.chain=8283,.total_fee_limit=UINT64_MAX};memcpy(policy.token,c->asset,20);
 nu54_evm_intent intent;if(nu54_evm_decode(raw,raw_len,&policy,&intent)||intent.nonce!=c->nonce||memcmp(intent.digest,c->digest,32))return -1;
 uint64_t amount=0;for(size_t j=0;j<24;j++)if(intent.amount[j])return -1;
 for(size_t j=24;j<32;j++)amount=(amount<<8)|intent.amount[j];
 if(memcmp(intent.recipient,v->policy.destination,20))return -1;
 if(replacing){nu54_evm_intent other;if(nu54_evm_decode(replacement,replacement_len,&policy,&other)||other.nonce!=intent.nonce||
    memcmp(other.digest,c->replacement_digest,32)||!memcmp(other.digest,intent.digest,32)||memcmp(other.recipient,intent.recipient,20))return -1;
    uint64_t replaced=0;for(size_t j=0;j<24;j++)if(other.amount[j])return -1;
    for(size_t j=24;j<32;j++)replaced=(replaced<<8)|other.amount[j];
    if(!replaced||replaced>amount)return -1;}
 uint32_t i;for(i=0;i<v->policy.count;i++)if(!memcmp(v->policy.reservations[i].id,c->request,16))break;
 if(i==v->policy.count)return !memcmp(claim_hash,v->last_claim,32) ? NU54_FINALITY_ALREADY : -1;
 if(v->nonces[i]!=c->nonce || memcmp(v->policy.reservations[i].digest,c->digest,32))return -1;
 const nu54_limit_reservation *r=&v->policy.reservations[i];
 if(amount!=r->amount || r->amount>v->policy.day_spent)return -1;

 uint64_t day=lower/86400,spent=v->policy.day_spent;
 if(day>v->policy.day){spent=0;for(uint32_t j=0;j<v->policy.count;j++){
   if(!v->policy.reservations[j].amount || v->policy.reservations[j].amount>v->policy.daily-spent)return -1;
   spent+=v->policy.reservations[j].amount;
 }}
 if(spent>v->policy.daily || r->amount>spent)return -1;
 *next=*v;memcpy(next->last_claim,claim_hash,32);next->policy.day=day;next->policy.day_spent=spent;next->policy.last_lower=lower;
 if(c->status==NU54_FINALITY_REVERT || c->status==NU54_FINALITY_REPLACEMENT_REVERT || c->block_time/86400<day)next->policy.day_spent-=r->amount;
 for(uint32_t j=i+1;j<v->policy.count;j++){next->policy.reservations[j-1]=next->policy.reservations[j];next->nonces[j-1]=next->nonces[j];}
 next->policy.count--;memset(&next->policy.reservations[next->policy.count],0,sizeof(next->policy.reservations[0]));next->nonces[next->policy.count]=0;
 return 0;
}

int nu54_finality_apply(const nu54_finality_state *v,const nu54_finality_claim *c,const uint8_t *raw,size_t raw_len,const uint8_t signature[64],nu54_finality_verify verify,nu54_finality_hash hash,uint64_t lower,uint64_t upper,nu54_finality_state *next) {
 return apply(v,c,raw,raw_len,NULL,0,signature,verify,hash,lower,upper,next);
}
int nu54_finality_apply_replacement(const nu54_finality_state *v,const nu54_finality_claim *c,const uint8_t *raw,size_t raw_len,const uint8_t *replacement,size_t replacement_len,const uint8_t signature[64],nu54_finality_verify verify,nu54_finality_hash hash,uint64_t lower,uint64_t upper,nu54_finality_state *next) {
 if(!replacement || !replacement_len)return -1;
 return apply(v,c,raw,raw_len,replacement,replacement_len,signature,verify,hash,lower,upper,next);
}
