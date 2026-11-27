#define main unused_finality_test_main
#include "test_owner_finality.c"
#undef main
#include "device_owner_finality_ps.h"
#include <psa/protected_storage.h>
#include <psa/crypto.h>
#include <openssl/x509.h>
typedef struct {uint32_t schema,phase;uint8_t enrollment_digest[32];nu54_finality_state state;} stored_record;
static stored_record record;static uint64_t independent,stored;static int exists,fail_after_write,qualified;
static EVP_PKEY *imported,*issuer_key;static int allow_mode_fence,mode_fence;
bool device_owner_finality_platform_qualified(void){return qualified;}
int device_owner_limits_claim_fresh_finality_ps(const uint8_t board[32],uint64_t generation){if(!allow_mode_fence||board[0]!=1||generation!=7)return -1;mode_fence=1;return 0;}
psa_status_t psa_ps_get_info(psa_storage_uid_t uid,struct psa_storage_info_t *info){(void)uid;if(!exists)return PSA_ERROR_DOES_NOT_EXIST;info->size=sizeof(record);info->flags=0;return PSA_SUCCESS;}
psa_status_t psa_ps_get(psa_storage_uid_t uid,size_t offset,size_t size,void *out,size_t *length){(void)uid;if(!exists||independent!=stored||offset||size!=sizeof(record))return -1;memcpy(out,&record,size);*length=size;return 0;}
psa_status_t psa_ps_set(psa_storage_uid_t uid,size_t size,const void *input,uint32_t flags){(void)uid;(void)flags;if(size!=sizeof(record))return -1;memcpy(&record,input,size);exists=1;stored=++independent;return fail_after_write?-1:PSA_SUCCESS;}
psa_status_t psa_hash_compute(int alg,const uint8_t *body,size_t n,uint8_t *out,size_t cap,size_t *written){(void)alg;if(cap<32||!SHA256(body,n,out))return -1;*written=32;return PSA_SUCCESS;}
psa_status_t psa_import_key(const psa_key_attributes_t *attrs,const uint8_t *point,size_t n,psa_key_id_t *id){(void)attrs;static const uint8_t prefix[]={0x30,0x59,0x30,0x13,0x06,0x07,0x2a,0x86,0x48,0xce,0x3d,0x02,0x01,0x06,0x08,0x2a,0x86,0x48,0xce,0x3d,0x03,0x01,0x07,0x03,0x42,0x00};if(n!=65)return -1;uint8_t der[sizeof(prefix)+65];memcpy(der,prefix,sizeof(prefix));memcpy(der+sizeof(prefix),point,65);const unsigned char *p=der;imported=d2i_PUBKEY(NULL,&p,sizeof(der));if(!imported)return -1;*id=1;return PSA_SUCCESS;}
psa_status_t psa_verify_hash(psa_key_id_t id,int alg,const uint8_t *digest,size_t n,const uint8_t *raw,size_t len){(void)id;(void)alg;if(!imported||n!=32||len!=64)return -1;ECDSA_SIG *sig=ECDSA_SIG_new();BIGNUM *r=BN_bin2bn(raw,32,NULL),*s=BN_bin2bn(raw+32,32,NULL);uint8_t der[80],*p=der;EVP_PKEY_CTX *ctx=EVP_PKEY_CTX_new(imported,NULL);int ok=0;if(sig&&r&&s&&ctx&&ECDSA_SIG_set0(sig,r,s)){r=NULL;s=NULL;int size=i2d_ECDSA_SIG(sig,&p);if(size>0&&EVP_PKEY_verify_init(ctx)==1&&EVP_PKEY_CTX_set_signature_md(ctx,EVP_sha256())==1&&EVP_PKEY_verify(ctx,der,(size_t)size,digest,n)==1)ok=1;}BN_free(r);BN_free(s);ECDSA_SIG_free(sig);EVP_PKEY_CTX_free(ctx);return ok?PSA_SUCCESS:-1;}
psa_status_t psa_destroy_key(psa_key_id_t id){(void)id;EVP_PKEY_free(imported);imported=NULL;return PSA_SUCCESS;}
static int enrollment_verify(EVP_PKEY *wanted,const uint8_t *body,size_t n,const uint8_t proof[64]) {
 EVP_PKEY *old=key;key=wanted;int result=verify(trusted,body,n,proof);key=old;return result;
}
static int verify_issuer(const uint8_t *body,size_t n,const uint8_t proof[64]) {return enrollment_verify(issuer_key,body,n,proof);}
static int verify_display(const uint8_t *body,size_t n,const uint8_t proof[64]) {return enrollment_verify(key,body,n,proof);}
static void sign_bytes(EVP_PKEY *who,const uint8_t *body,size_t size,uint8_t out[64]) {
 EVP_MD_CTX *ctx=EVP_MD_CTX_new();uint8_t der[80];size_t n=sizeof(der);assert(ctx&&EVP_DigestSignInit(ctx,NULL,EVP_sha256(),NULL,who)==1&&EVP_DigestSign(ctx,der,&n,body,size)==1);
 const unsigned char *p=der;ECDSA_SIG *sig=d2i_ECDSA_SIG(NULL,&p,(long)n);assert(sig);const BIGNUM *r,*s;ECDSA_SIG_get0(sig,&r,&s);assert(BN_bn2binpad(r,out,32)==32&&BN_bn2binpad(s,out+32,32)==32);ECDSA_SIG_free(sig);EVP_MD_CTX_free(ctx);
}
static int key_independent(const uint8_t point[65]) {
 uint8_t a[65],b[65];size_t an=sizeof(a),bn=sizeof(b);
 return EVP_PKEY_get_octet_string_param(key,"pub",a,sizeof(a),&an)==1&&EVP_PKEY_get_octet_string_param(issuer_key,"pub",b,sizeof(b),&bn)==1&&an==65&&bn==65&&memcmp(point,a,65)&&memcmp(point,b,65);
}
static int show_enrollment(void *ctx,const uint8_t *preimage,size_t n){(void)ctx;return n==66&&preimage[0]==0x19?0:-1;}
#ifdef FINALITY_PS_EMBED
#define main unused_finality_ps_test_main
#endif
int main(void){
 EVP_PKEY_CTX *k=EVP_PKEY_CTX_new_id(EVP_PKEY_EC,NULL);assert(k&&EVP_PKEY_keygen_init(k)==1&&EVP_PKEY_CTX_set_ec_paramgen_curve_nid(k,NID_X9_62_prime256v1)==1&&EVP_PKEY_keygen(k,&key)==1);EVP_PKEY_CTX_free(k);
 nu54_finality_state *v=&record.state;record.schema=2;record.phase=2;v->policy=(nu54_owner_limits){.generation=7,.revision=1,.chain=8283,.per_transfer=10,.daily=30,.day=1,.last_lower=86400};memset(v->policy.board,1,32);memset(v->policy.asset,4,20);memset(v->policy.destination,5,20);memset(v->genesis,8,32);memset(v->signer,9,20);
 size_t public_size=65;assert(EVP_PKEY_get_octet_string_param(key,"pub",v->authority,65,&public_size)==1&&public_size==65);exists=1;independent=stored=1;
 uint8_t raw[256],signature[64];size_t n=hex("02f86782205b010101830186a094040404040404040404040404040404040404040480b844a9059cbb00000000000000000000000005050505050505050505050505050505050505050000000000000000000000000000000000000000000000000000000000000009c0",raw);
 nu54_evm_policy pol={.chain=8283,.total_fee_limit=1000000};memset(pol.token,4,20);nu54_evm_intent intent;assert(!nu54_evm_decode(raw,n,&pol,&intent));
 nu54_limit_request r={.generation=7,.chain=8283,.amount=9,.expires=87000};memset(r.board,1,32);memset(r.asset,4,20);memset(r.destination,5,20);memset(r.id,3,16);memcpy(r.digest,intent.digest,32);
 assert(device_owner_finality_reserve_ps(&r,v->signer,raw,n,86410,86410)==-1);qualified=1;assert(!device_owner_finality_reserve_ps(&r,v->signer,raw,n,86410,86410));
 nu54_finality_claim c={.generation=7,.chain=8283,.nonce=1,.block_height=2,.finalized_height=2,.block_time=86410,.status=NU54_FINALITY_SUCCESS};memcpy(c.board,r.board,32);memcpy(c.genesis,v->genesis,32);memcpy(c.signer,v->signer,20);memcpy(c.asset,r.asset,20);memcpy(c.request,r.id,16);memcpy(c.digest,r.digest,32);memset(c.block_hash,2,32);sign_claim(&c,signature);
 signature[0]^=1;assert(device_owner_finality_reconcile_ps(&c,raw,n,signature,86410,86410)==-1);signature[0]^=1;
 fail_after_write=1;assert(device_owner_finality_reconcile_ps(&c,raw,n,signature,86410,86410)==-1);fail_after_write=0;
 assert(record.state.policy.count==0&&record.state.policy.day_spent==9);assert(device_owner_finality_reconcile_ps(&c,raw,n,signature,86410,86410)==NU54_FINALITY_ALREADY);
 /* Approved preparation survives an ambiguous write, then resumes only the
  * exact stored profile through the independent V1 mode fence. */
 EVP_PKEY_CTX *issuer_ctx=EVP_PKEY_CTX_new_id(EVP_PKEY_EC,NULL);assert(issuer_ctx&&EVP_PKEY_keygen_init(issuer_ctx)==1&&EVP_PKEY_CTX_set_ec_paramgen_curve_nid(issuer_ctx,NID_X9_62_prime256v1)==1&&EVP_PKEY_keygen(issuer_ctx,&issuer_key)==1);EVP_PKEY_CTX_free(issuer_ctx);
 EVP_PKEY_CTX *final_ctx=EVP_PKEY_CTX_new_id(EVP_PKEY_EC,NULL);EVP_PKEY *final_key=NULL;assert(final_ctx&&EVP_PKEY_keygen_init(final_ctx)==1&&EVP_PKEY_CTX_set_ec_paramgen_curve_nid(final_ctx,NID_X9_62_prime256v1)==1&&EVP_PKEY_keygen(final_ctx,&final_key)==1);EVP_PKEY_CTX_free(final_ctx);
 nu54_finality_enrollment e={.generation=7,.chain=8283,.per_transfer=10,.daily=30,.expires=87000};memset(e.board,1,32);memset(e.session,2,16);memset(e.request,3,16);memset(e.challenge,4,32);memset(e.signer,5,20);memset(e.genesis,6,32);memset(e.asset,7,20);memset(e.destination,8,20);size_t plen=65;assert(EVP_PKEY_get_octet_string_param(final_key,"pub",e.authority,65,&plen)==1&&plen==65);
 uint8_t body[NU54_FINALITY_ENROLL_BODY_SIZE],grant[NU54_FINALITY_GRANT_SIZE],issuer_proof[64],display_proof[64];assert(!nu54_finality_enrollment_body(&e,body));assert(!nu54_finality_grant_body(&e,grant));sign_bytes(issuer_key,grant,sizeof(grant),issuer_proof);sign_bytes(key,body,sizeof(body),display_proof);
 nu54_local_approval approval={0};memcpy(trusted,v->authority,65); /* verifier compares this fixture marker */
 assert(!nu54_finality_enrollment_stage(&e,86410,86410,7,&approval,hash_body,verify_issuer,issuer_proof,key_independent,show_enrollment,NULL));
 exists=0;independent=stored=10;allow_mode_fence=0;mode_fence=0;fail_after_write=1;
 assert(device_owner_finality_enroll_ps(&e,86410,86410,7,NU54_LOCAL_APPROVE,&approval,verify_issuer,issuer_proof,verify_display,display_proof,key_independent)==-1);
 assert(record.phase==1 && !mode_fence && !device_owner_finality_ready_ps(e.board,7,e.signer));
 fail_after_write=0;assert(device_owner_finality_resume_ps(e.board,7,e.signer)==-1);allow_mode_fence=1;
 uint8_t wrong_signer[20]={1};assert(device_owner_finality_resume_ps(e.board,7,wrong_signer)==-1 && !mode_fence);
 assert(!device_owner_finality_resume_ps(e.board,7,e.signer));assert(mode_fence && device_owner_finality_ready_ps(e.board,7,e.signer));
 assert(device_owner_finality_resume_ps(e.board,7,e.signer)==NU54_FINALITY_ALREADY);
 EVP_PKEY_free(final_key);EVP_PKEY_free(issuer_key);EVP_PKEY_free(key);return 0;
}
