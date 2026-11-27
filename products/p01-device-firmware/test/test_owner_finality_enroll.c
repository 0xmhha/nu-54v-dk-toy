#include "nu54_owner_finality_enroll.h"
#include <assert.h>
#include <string.h>
static int hash(const uint8_t *p,size_t n,uint8_t out[32]){memset(out,0,32);for(size_t i=0;i<n;i++)out[i%32]^=(uint8_t)(p[i]+i);return 0;}
static int verify(const uint8_t *p,size_t n,const uint8_t sig[64]){return p[0]=='N' && (n==NU54_FINALITY_ENROLL_BODY_SIZE || n==NU54_FINALITY_GRANT_SIZE) && sig[0]==42?0:-1;}
static int independent(const uint8_t key[65]){return key[0]==4 && key[1]==1;}
static int present(void *ctx,const uint8_t *p,size_t n){(void)ctx;return n==66&&p[0]==0x19?0:-1;}
int main(void){
 nu54_finality_enrollment e={.generation=7,.chain=8283,.per_transfer=10,.daily=30,.expires=87000};memset(e.board,1,32);memset(e.session,2,16);memset(e.request,3,16);memset(e.challenge,4,32);memset(e.signer,5,20);memset(e.genesis,6,32);memset(e.asset,7,20);memset(e.destination,8,20);e.authority[0]=4;e.authority[1]=1;
 uint8_t proof[64]={42};nu54_local_approval approval={0};nu54_finality_state state;
 assert(!nu54_finality_enrollment_stage(&e,86410,86410,1,&approval,hash,verify,proof,independent,present,0));
 nu54_finality_enrollment changed=e;changed.authority[1]^=1;
 assert(nu54_finality_enrollment_confirm(&changed,86410,86410,1,NU54_LOCAL_APPROVE,&approval,hash,verify,proof,&state)==-1);
 assert(!nu54_finality_enrollment_stage(&e,86410,86410,2,&approval,hash,verify,proof,independent,present,0));
 assert(nu54_finality_enrollment_confirm(&e,86410,86410,2,NU54_LOCAL_REJECT,&approval,hash,verify,proof,&state)==-1);
 assert(!nu54_finality_enrollment_stage(&e,86410,86410,3,&approval,hash,verify,proof,independent,present,0));
 assert(!nu54_finality_enrollment_confirm(&e,86410,86410,3,NU54_LOCAL_APPROVE,&approval,hash,verify,proof,&state));
 assert(state.policy.chain==8283 && state.policy.daily==30 && !memcmp(state.authority,e.authority,65));
 return 0;
}
