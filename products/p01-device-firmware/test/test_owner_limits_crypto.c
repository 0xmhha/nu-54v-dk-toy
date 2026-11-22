#define main unused_ps_fixture_main
#include "test_owner_limits_ps.c"
#undef main
#include <openssl/evp.h>
#include <openssl/ec.h>
#include <openssl/ecdsa.h>
#include <openssl/sha.h>
static EVP_PKEY *display_key;
static int real_hash(const uint8_t *v,size_t n,uint8_t out[32]) {return SHA256(v,n,out) ? 0 : -1;}
static int real_verify(const uint8_t *data,size_t size,const uint8_t raw[64]) {
 int result=-1; ECDSA_SIG *signature=ECDSA_SIG_new(); EVP_MD_CTX *ctx=EVP_MD_CTX_new();
 BIGNUM *r=BN_bin2bn(raw,32,NULL),*s=BN_bin2bn(raw+32,32,NULL);unsigned char der[80],*p=der;
 if(signature && ctx && r && s && ECDSA_SIG_set0(signature,r,s)) {
   r=NULL;s=NULL; int length=i2d_ECDSA_SIG(signature,&p);
   if(length>0 && EVP_DigestVerifyInit(ctx,NULL,EVP_sha256(),NULL,display_key)==1 &&
      EVP_DigestVerify(ctx,der,(size_t)length,data,size)==1)result=0;
 }
 BN_free(r);BN_free(s);ECDSA_SIG_free(signature);EVP_MD_CTX_free(ctx);return result;
}
static void sign_statement(const uint8_t *body,size_t size,uint8_t out[64]) {
 EVP_MD_CTX *ctx=EVP_MD_CTX_new(); unsigned char der[80]; size_t length=sizeof(der);
 assert(ctx && EVP_DigestSignInit(ctx,NULL,EVP_sha256(),NULL,display_key)==1);
 assert(EVP_DigestSign(ctx,der,&length,body,size)==1); const unsigned char *p=der;
 ECDSA_SIG *sig=d2i_ECDSA_SIG(NULL,&p,(long)length); assert(sig && p==der+length);
 const BIGNUM *r,*s;ECDSA_SIG_get0(sig,&r,&s);
 assert(BN_bn2binpad(r,out,32)==32 && BN_bn2binpad(s,out+32,32)==32);
 ECDSA_SIG_free(sig);EVP_MD_CTX_free(ctx);
}
int main(void) {
 EVP_PKEY_CTX *keyctx=EVP_PKEY_CTX_new_id(EVP_PKEY_EC,NULL);
 assert(keyctx && EVP_PKEY_keygen_init(keyctx)==1 && EVP_PKEY_CTX_set_ec_paramgen_curve_nid(keyctx,NID_X9_62_prime256v1)==1);
 assert(EVP_PKEY_keygen(keyctx,&display_key)==1); EVP_PKEY_CTX_free(keyctx);
 uint8_t board[32];memset(board,1,32); assert(device_owner_limits_mark_fresh_ps(board,7)==0);
 nu54_limit_change c={.generation=7,.revision=1,.chain=8283,.per_transfer=10000000,.daily=30000000,.expires=87000,.initialize=1};
 memcpy(c.board,board,32);memset(c.session,2,16);memset(c.request,3,16);memset(c.challenge,4,32);memset(c.asset,4,20);memset(c.destination,5,20);
 nu54_owner_limits base={.generation=7,.revision=1,.chain=8283,.per_transfer=10000000,.daily=30000000};
 memcpy(base.board,board,32);memcpy(base.asset,c.asset,20);memcpy(base.destination,c.destination,20);
 uint8_t statement[NU54_LIMIT_CHANGE_STATEMENT_SIZE],proof[64];
 assert(nu54_limit_change_statement(&base,&c,statement)==0);sign_statement(statement,sizeof(statement),proof);
 nu54_local_approval approval={0};assert(device_owner_limits_initialize_stage_ps(&c,86410,86410,1,&approval,real_hash,present,0)==0);
 proof[0]^=1;assert(device_owner_limits_initialize_confirm_ps(&c,86420,86420,1,NU54_LOCAL_APPROVE,&approval,proof,real_hash,real_verify)==-1);
 assert(!exists && !init_exists);proof[0]^=1;
 assert(device_owner_limits_initialize_stage_ps(&c,86420,86420,2,&approval,real_hash,present,0)==0);
 assert(device_owner_limits_initialize_confirm_ps(&c,86420,86420,2,NU54_LOCAL_APPROVE,&approval,proof,real_hash,real_verify)==0);
 nu54_owner_limits state;assert(device_owner_limits_read_ps(board,7,&state)==0 && state.per_transfer==10000000);
 EVP_PKEY_free(display_key);return 0;
}
