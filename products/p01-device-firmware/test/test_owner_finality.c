#include "nu54_owner_finality.h"
#include "nu54_owner_finality_store.h"
#include <assert.h>
#include <stdio.h>
#include <string.h>
#include <openssl/evp.h>
#include <openssl/ec.h>
#include <openssl/ecdsa.h>
#include <openssl/sha.h>
static size_t hex(const char *s,uint8_t *out) {size_t n=strlen(s)/2;for(size_t i=0;i<n;i++){unsigned v;assert(sscanf(s+2*i,"%2x",&v)==1);out[i]=(uint8_t)v;}return n;}
static int hash_body(const uint8_t *data,size_t n,uint8_t out[32]) {return SHA256(data,n,out)?0:-1;}
static EVP_PKEY *key;
static uint8_t trusted[65];
static int verify(const uint8_t authority[65],const uint8_t *body,size_t n,const uint8_t raw[64]) {
 if(memcmp(authority,trusted,65))return -1;
 ECDSA_SIG *sig=ECDSA_SIG_new();EVP_MD_CTX *ctx=EVP_MD_CTX_new();BIGNUM *r=BN_bin2bn(raw,32,NULL),*s=BN_bin2bn(raw+32,32,NULL);unsigned char der[80],*p=der;int result=-1;
 if(sig&&ctx&&r&&s&&ECDSA_SIG_set0(sig,r,s)){r=NULL;s=NULL;int len=i2d_ECDSA_SIG(sig,&p);if(len>0&&EVP_DigestVerifyInit(ctx,NULL,EVP_sha256(),NULL,key)==1&&EVP_DigestVerify(ctx,der,(size_t)len,body,n)==1)result=0;}
 BN_free(r);BN_free(s);ECDSA_SIG_free(sig);EVP_MD_CTX_free(ctx);return result;
}
static void sign_body(const uint8_t *body,size_t size,uint8_t raw[64]) {
 uint8_t der[80];size_t n=sizeof(der);
 EVP_MD_CTX *ctx=EVP_MD_CTX_new();assert(ctx&&EVP_DigestSignInit(ctx,NULL,EVP_sha256(),NULL,key)==1&&EVP_DigestSign(ctx,der,&n,body,size)==1);
 const unsigned char *p=der;ECDSA_SIG *sig=d2i_ECDSA_SIG(NULL,&p,(long)n);assert(sig);const BIGNUM *r,*s;ECDSA_SIG_get0(sig,&r,&s);assert(BN_bn2binpad(r,raw,32)==32&&BN_bn2binpad(s,raw+32,32)==32);ECDSA_SIG_free(sig);EVP_MD_CTX_free(ctx);
}
static void sign_claim(const nu54_finality_claim *claim,uint8_t raw[64]) {
 uint8_t body[NU54_FINALITY_BODY_SIZE];assert(!nu54_finality_body(claim,body));sign_body(body,sizeof(body),raw);
}
static void sign_replacement(const nu54_finality_claim *claim,uint8_t raw[64]) {
 uint8_t body[NU54_FINALITY_REPLACEMENT_BODY_SIZE];assert(!nu54_finality_replacement_body(claim,body));sign_body(body,sizeof(body),raw);
}
typedef struct {nu54_finality_record record;uint64_t counter;int locked;} fixture;
static int lock_store(void *p){fixture *f=p;if(f->locked)return -1;f->locked=1;return 0;}
static void unlock_store(void *p){((fixture *)p)->locked=0;}
static int load_store(void *p,nu54_finality_record *r){*r=((fixture *)p)->record;return 0;}
static int write_store(void *p,const nu54_finality_record *r){((fixture *)p)->record=*r;return 0;}
static int counter_store(void *p,uint64_t *v){*v=((fixture *)p)->counter;return 0;}
static int advance_store(void *p,uint64_t old){fixture *f=p;if(f->counter!=old)return -1;f->counter++;return 0;}
int main(void) {
 EVP_PKEY_CTX *keyctx=EVP_PKEY_CTX_new_id(EVP_PKEY_EC,NULL);assert(keyctx&&EVP_PKEY_keygen_init(keyctx)==1&&EVP_PKEY_CTX_set_ec_paramgen_curve_nid(keyctx,NID_X9_62_prime256v1)==1&&EVP_PKEY_keygen(keyctx,&key)==1);EVP_PKEY_CTX_free(keyctx);
 nu54_finality_state v={.policy={.generation=7,.revision=1,.chain=8283,.per_transfer=10,.daily=30,.day=1,.last_lower=86400}};
 memset(v.policy.board,1,32);memset(v.policy.asset,4,20);memset(v.policy.destination,5,20);memset(v.genesis,8,32);memset(v.signer,9,20);v.authority[0]=4;v.authority[1]=1;memcpy(trusted,v.authority,65);
 uint8_t raw[256],signature[64]={42};size_t n=hex("02f86782205b010101830186a094040404040404040404040404040404040404040480b844a9059cbb00000000000000000000000005050505050505050505050505050505050505050000000000000000000000000000000000000000000000000000000000000009c0",raw);
 nu54_evm_policy policy={.chain=8283,.total_fee_limit=1000000};memset(policy.token,4,20);nu54_evm_intent intent;assert(!nu54_evm_decode(raw,n,&policy,&intent));
 nu54_limit_request r={.generation=7,.chain=8283,.amount=9,.expires=87000};memset(r.board,1,32);memset(r.asset,4,20);memset(r.destination,5,20);memset(r.id,3,16);memcpy(r.digest,intent.digest,32);
 nu54_finality_state pending,next;assert(nu54_finality_reserve(&v,&r,v.signer,raw,n,86410,86410,&pending)==NU54_LIMIT_NEW);
 nu54_finality_claim c={.generation=7,.chain=8283,.nonce=1,.block_height=2,.finalized_height=2,.block_time=86410,.status=NU54_FINALITY_SUCCESS};memcpy(c.board,r.board,32);memcpy(c.genesis,v.genesis,32);memcpy(c.signer,v.signer,20);memcpy(c.asset,r.asset,20);memcpy(c.request,r.id,16);memcpy(c.digest,r.digest,32);memset(c.block_hash,2,32);
 uint8_t wire[NU54_FINALITY_BODY_SIZE];nu54_finality_claim parsed={0};assert(!nu54_finality_body(&c,wire));assert(!nu54_finality_parse(wire,sizeof(wire),&parsed));
 assert(parsed.nonce==c.nonce && parsed.generation==c.generation && !memcmp(parsed.digest,c.digest,32));
 assert(nu54_finality_parse(wire,sizeof(wire)-1,&parsed));wire[0]^=1;assert(nu54_finality_parse(wire,sizeof(wire),&parsed));wire[0]^=1;
 sign_claim(&c,signature);
 nu54_finality_claim wrong=c;wrong.nonce++;assert(nu54_finality_apply(&pending,&wrong,raw,n,signature,verify,hash_body,86410,86410,&next));
 wrong=c;wrong.genesis[0]^=1;sign_claim(&wrong,signature);assert(nu54_finality_apply(&pending,&wrong,raw,n,signature,verify,hash_body,86410,86410,&next));
 wrong=c;wrong.signer[0]^=1;sign_claim(&wrong,signature);assert(nu54_finality_apply(&pending,&wrong,raw,n,signature,verify,hash_body,86410,86410,&next));
 wrong=c;wrong.asset[0]^=1;sign_claim(&wrong,signature);assert(nu54_finality_apply(&pending,&wrong,raw,n,signature,verify,hash_body,86410,86410,&next));
 wrong=c;wrong.generation++;sign_claim(&wrong,signature);assert(nu54_finality_apply(&pending,&wrong,raw,n,signature,verify,hash_body,86410,86410,&next));
 wrong=c;wrong.chain=84532;sign_claim(&wrong,signature);assert(nu54_finality_apply(&pending,&wrong,raw,n,signature,verify,hash_body,86410,86410,&next));
 wrong=c;wrong.request[0]^=1;sign_claim(&wrong,signature);assert(nu54_finality_apply(&pending,&wrong,raw,n,signature,verify,hash_body,86410,86410,&next));
 wrong=c;wrong.digest[0]^=1;sign_claim(&wrong,signature);assert(nu54_finality_apply(&pending,&wrong,raw,n,signature,verify,hash_body,86410,86410,&next));
 wrong=c;wrong.block_time++;sign_claim(&wrong,signature);assert(nu54_finality_apply(&pending,&wrong,raw,n,signature,verify,hash_body,86410,86410,&next));
 wrong=c;wrong.finalized_height=1;sign_claim(&wrong,signature);assert(nu54_finality_apply(&pending,&wrong,raw,n,signature,verify,hash_body,86410,86410,&next));
 nu54_finality_state wrong_key=pending;wrong_key.authority[1]^=1;sign_claim(&c,signature);assert(nu54_finality_apply(&wrong_key,&c,raw,n,signature,verify,hash_body,86410,86410,&next));

 sign_claim(&c,signature);raw[10]^=1;assert(nu54_finality_apply(&pending,&c,raw,n,signature,verify,hash_body,86410,86410,&next));raw[10]^=1;
 signature[0]^=1;assert(nu54_finality_apply(&pending,&c,raw,n,signature,verify,hash_body,86410,86410,&next));signature[0]^=1;
 fixture durable={.record={.version=1,.state=pending},.counter=1};
 nu54_finality_record old_record=durable.record;
 nu54_finality_port port={&durable,lock_store,unlock_store,load_store,write_store,counter_store,advance_store};
 assert(!nu54_finality_commit(&port,&c,raw,n,signature,verify,hash_body,86410,86410));
 /* A lost reply followed by the same signed evidence never releases twice. */
 assert(nu54_finality_commit(&port,&c,raw,n,signature,verify,hash_body,86410,86410)==NU54_FINALITY_ALREADY);
 assert(durable.record.state.policy.count==0 && durable.record.state.policy.day_spent==9 && durable.counter==2);
 next=durable.record.state;assert(next.policy.count==0 && next.policy.day_spent==9);
 durable.record=old_record; /* restored old snapshot with advanced independent counter */
 assert(nu54_finality_commit(&port,&c,raw,n,signature,verify,hash_body,86410,86410)==-1);
 assert(durable.counter==2);durable.record=(nu54_finality_record){.version=2,.state=next};
 assert(nu54_finality_apply(&next,&c,raw,n,signature,verify,hash_body,86410,86410,&pending)==NU54_FINALITY_ALREADY);assert(nu54_finality_reserve(&next,&r,v.signer,raw,n,86411,86411,&pending)==NU54_LIMIT_REJECT);
 /* A distinct transaction/nonce can reuse the released slot.  A finalized
  * revert releases only its own amount; an old attestation cannot affect it. */
 raw[6]=2;assert(!nu54_evm_decode(raw,n,&policy,&intent));r.id[0]=4;memcpy(r.digest,intent.digest,32);
 assert(nu54_finality_reserve(&next,&r,v.signer,raw,n,86411,86411,&pending)==NU54_LIMIT_NEW);
 assert(pending.policy.count==1 && pending.policy.day_spent==18);
 assert(nu54_finality_apply(&pending,&c,raw,n,signature,verify,hash_body,86411,86411,&v));
 memcpy(c.request,r.id,16);memcpy(c.digest,r.digest,32);c.nonce=2;c.status=NU54_FINALITY_REVERT;c.block_time=86411;sign_claim(&c,signature);
 assert(!nu54_finality_apply(&pending,&c,raw,n,signature,verify,hash_body,86411,86411,&v));
 assert(v.policy.count==0 && v.policy.day_spent==9);
 /* Finalized success on a later day remains charged on its actual block day. */
 raw[6]=3;assert(!nu54_evm_decode(raw,n,&policy,&intent));r.id[0]=5;memcpy(r.digest,intent.digest,32);r.expires=259000;
 assert(nu54_finality_reserve(&v,&r,v.signer,raw,n,86412,86412,&pending)==NU54_LIMIT_NEW);
 memcpy(c.request,r.id,16);memcpy(c.digest,r.digest,32);c.nonce=3;c.status=NU54_FINALITY_SUCCESS;c.block_time=172800;sign_claim(&c,signature);
 assert(!nu54_finality_apply(&pending,&c,raw,n,signature,verify,hash_body,172800,172800,&v));
 assert(v.policy.day==2 && v.policy.day_spent==9 && v.policy.count==0);
 /* A finalized replacement must preserve signer scope, nonce, token and
  * recipient; success retains the full original reservation conservatively. */
 raw[6]=4;assert(!nu54_evm_decode(raw,n,&policy,&intent));r.id[0]=6;memcpy(r.digest,intent.digest,32);
 assert(nu54_finality_reserve(&v,&r,v.signer,raw,n,172801,172801,&pending)==NU54_LIMIT_NEW);
 uint8_t replacement[256];memcpy(replacement,raw,n);replacement[7]=0x80;
 nu54_evm_intent replaced;assert(!nu54_evm_decode(replacement,n,&policy,&replaced));
 memcpy(c.request,r.id,16);memcpy(c.digest,r.digest,32);memcpy(c.replacement_digest,replaced.digest,32);c.nonce=4;c.status=NU54_FINALITY_REPLACEMENT_SUCCESS;c.block_time=172801;sign_replacement(&c,signature);
 assert(!nu54_finality_apply_replacement(&pending,&c,raw,n,replacement,n,signature,verify,hash_body,172801,172801,&next));
 assert(next.policy.count==0 && next.policy.day_spent==18);
 replacement[6]=5;assert(!nu54_evm_decode(replacement,n,&policy,&replaced));memcpy(c.replacement_digest,replaced.digest,32);sign_replacement(&c,signature);
 assert(nu54_finality_apply_replacement(&pending,&c,raw,n,replacement,n,signature,verify,hash_body,172801,172801,&next)==-1);
 replacement[6]=4;replacement[n-1-32-20]^=1;assert(!nu54_evm_decode(replacement,n,&policy,&replaced));memcpy(c.replacement_digest,replaced.digest,32);sign_replacement(&c,signature);
 assert(nu54_finality_apply_replacement(&pending,&c,raw,n,replacement,n,signature,verify,hash_body,172801,172801,&next)==-1);
 replacement[n-1-32-20]^=1;assert(!nu54_evm_decode(replacement,n,&policy,&replaced));memcpy(c.replacement_digest,replaced.digest,32);c.status=NU54_FINALITY_REPLACEMENT_REVERT;sign_replacement(&c,signature);
 assert(!nu54_finality_apply_replacement(&pending,&c,raw,n,replacement,n,signature,verify,hash_body,172801,172801,&next));
 assert(next.policy.count==0 && next.policy.day_spent==9);
 EVP_PKEY_free(key);return 0;
}
