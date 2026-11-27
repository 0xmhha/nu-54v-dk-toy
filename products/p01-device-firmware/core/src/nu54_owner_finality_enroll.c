#include "nu54_owner_finality_enroll.h"
#include <string.h>
static int present_bytes(const uint8_t *p,size_t n){uint8_t x=0;for(size_t i=0;i<n;i++)x|=p[i];return x!=0;}
static void put64(uint8_t *p,uint64_t v){for(int i=7;i>=0;i--){p[i]=(uint8_t)v;v>>=8;}}
int nu54_finality_enrollment_body(const nu54_finality_enrollment *e,uint8_t out[NU54_FINALITY_ENROLL_BODY_SIZE]) {
 if(!e||!out)return -1;
 size_t i=0;memcpy(out+i,"N54G",4);i+=4;memcpy(out+i,e->board,32);i+=32;put64(out+i,e->generation);i+=8;
 memcpy(out+i,e->session,16);i+=16;memcpy(out+i,e->request,16);i+=16;memcpy(out+i,e->challenge,32);i+=32;
 put64(out+i,e->chain);i+=8;memcpy(out+i,e->signer,20);i+=20;memcpy(out+i,e->genesis,32);i+=32;
 memcpy(out+i,e->asset,20);i+=20;memcpy(out+i,e->destination,20);i+=20;put64(out+i,e->per_transfer);i+=8;
 put64(out+i,e->daily);i+=8;memcpy(out+i,e->authority,65);i+=65;put64(out+i,e->expires);i+=8;
 return i==NU54_FINALITY_ENROLL_BODY_SIZE?0:-1;
}
int nu54_finality_grant_body(const nu54_finality_enrollment *e,uint8_t out[NU54_FINALITY_GRANT_SIZE]) {
 if(!e||!out)return -1;
 size_t i=0;memcpy(out+i,"N54D",4);i+=4;memcpy(out+i,e->board,32);i+=32;put64(out+i,e->generation);i+=8;put64(out+i,e->chain);i+=8;
 memcpy(out+i,e->signer,20);i+=20;memcpy(out+i,e->genesis,32);i+=32;memcpy(out+i,e->asset,20);i+=20;memcpy(out+i,e->destination,20);i+=20;
 put64(out+i,e->per_transfer);i+=8;put64(out+i,e->daily);i+=8;memcpy(out+i,e->authority,65);i+=65;put64(out+i,e->expires);i+=8;
 return i==NU54_FINALITY_GRANT_SIZE?0:-1;
}
static uint64_t get64(const uint8_t *p){uint64_t v=0;for(size_t i=0;i<8;i++)v=(v<<8)|p[i];return v;}
int nu54_finality_grant_parse(const uint8_t *grant,size_t n,nu54_finality_enrollment *e) {
 if(!grant||!e||n!=NU54_FINALITY_GRANT_SIZE||memcmp(grant,"N54D",4))return -1;
 memset(e,0,sizeof(*e));size_t i=4;memcpy(e->board,grant+i,32);i+=32;e->generation=get64(grant+i);i+=8;e->chain=get64(grant+i);i+=8;
 memcpy(e->signer,grant+i,20);i+=20;memcpy(e->genesis,grant+i,32);i+=32;memcpy(e->asset,grant+i,20);i+=20;
 memcpy(e->destination,grant+i,20);i+=20;e->per_transfer=get64(grant+i);i+=8;e->daily=get64(grant+i);i+=8;
 memcpy(e->authority,grant+i,65);i+=65;e->expires=get64(grant+i);i+=8;return i==n?0:-1;
}
static int valid(const nu54_finality_enrollment *e,uint64_t lower,uint64_t upper) {
 return e && e->generation && e->chain==8283 && lower && lower<=upper && lower/86400==upper/86400 && e->expires>upper &&
 e->expires-upper<=600 && e->per_transfer && e->per_transfer<=e->daily && e->authority[0]==4 && present_bytes(e->authority+1,64) &&
 present_bytes(e->board,32) && present_bytes(e->session,16) && present_bytes(e->request,16) && present_bytes(e->challenge,32) &&
 present_bytes(e->signer,20) && present_bytes(e->genesis,32) && present_bytes(e->asset,20) && present_bytes(e->destination,20);
}
static int intent(const nu54_finality_enrollment *e,uint32_t prompt,nu54_finality_hash hash,nu54_local_intent *out) {
 uint8_t body[NU54_FINALITY_ENROLL_BODY_SIZE];static const uint8_t domain[]="NU54-owner-finality-enrollment-v1";
 if(!hash||!out||nu54_finality_enrollment_body(e,body))return -1;
 memset(out,0,sizeof(*out));out->generation=e->generation;out->expires=e->expires;out->physical_prompt=prompt;
 memcpy(out->session,e->session,16);memcpy(out->request,e->request,16);out->preimage[0]=0x19;out->preimage[1]=1;
 return hash(domain,sizeof(domain)-1,out->preimage+2)||hash(body,sizeof(body),out->preimage+34)?-1:0;
}
int nu54_finality_enrollment_stage(const nu54_finality_enrollment *e,uint64_t lower,uint64_t upper,uint32_t prompt,nu54_local_approval *approval,nu54_finality_hash hash,nu54_enroll_verify verify_authority,const uint8_t proof[64],int (*independent)(const uint8_t [65]),int (*present)(void *,const uint8_t *,size_t),void *context) {
 uint8_t grant[NU54_FINALITY_GRANT_SIZE];nu54_local_intent i;
 if(!valid(e,lower,upper)||!approval||!verify_authority||!proof||!independent||!independent(e->authority)||nu54_finality_grant_body(e,grant)||
    verify_authority(grant,sizeof(grant),proof)||intent(e,prompt,hash,&i))return -1;
 return nu54_local_approval_stage(approval,&i,upper,present,context);
}
int nu54_finality_enrollment_confirm(const nu54_finality_enrollment *e,uint64_t lower,uint64_t upper,uint32_t prompt,nu54_local_button button,nu54_local_approval *approval,nu54_finality_hash hash,nu54_enroll_verify verify_display,const uint8_t proof[64],nu54_finality_state *out) {
 uint8_t body[NU54_FINALITY_ENROLL_BODY_SIZE],used[NU54_LOCAL_PREIMAGE_SIZE];nu54_local_intent i;
 if(!valid(e,lower,upper)||!approval||!verify_display||!proof||!out||nu54_finality_enrollment_body(e,body)||
    verify_display(body,sizeof(body),proof)||intent(e,prompt,hash,&i)){if(approval)nu54_local_approval_cancel(approval);return -1;}
 if(nu54_local_approval_consume(approval,&i,upper,prompt,button,used))return -1;
 memset(out,0,sizeof(*out));out->policy.generation=e->generation;out->policy.revision=1;out->policy.chain=e->chain;
 out->policy.per_transfer=e->per_transfer;out->policy.daily=e->daily;out->policy.day=lower/86400;out->policy.last_lower=lower;
 memcpy(out->policy.board,e->board,32);memcpy(out->policy.asset,e->asset,20);memcpy(out->policy.destination,e->destination,20);
 memcpy(out->genesis,e->genesis,32);memcpy(out->signer,e->signer,20);memcpy(out->authority,e->authority,65);return 0;
}
