/* Test-only board-side model runner; accepts only fixture authority keys. */
#define FINALITY_PS_EMBED
#include "test_owner_finality_ps.c"
#undef main
#include <openssl/x509.h>
static uint64_t get64(const uint8_t *p){uint64_t v=0;for(int i=0;i<8;i++)v=(v<<8)|p[i];return v;}
int main(int argc,char **argv){
 assert(argc==5 || argc==6);uint8_t raw[256],replacement[256],body[NU54_FINALITY_REPLACEMENT_BODY_SIZE],sig[64],der[256],public_key[65];
 size_t n=hex(argv[1],raw),body_len=hex(argv[2],body),sig_len=hex(argv[3],sig),der_len=hex(argv[4],der);assert((body_len==NU54_FINALITY_BODY_SIZE || body_len==NU54_FINALITY_REPLACEMENT_BODY_SIZE)&&sig_len==64);
 const unsigned char *p=der;key=d2i_PUBKEY(NULL,&p,(long)der_len);assert(key&&p==der+der_len);
 size_t public_len=sizeof(public_key);assert(EVP_PKEY_get_octet_string_param(key,"pub",public_key,sizeof(public_key),&public_len)==1&&public_len==65);
 memcpy(trusted,public_key,65);assert(!memcmp(body,argc==6?"N54R":"N54F",4));nu54_finality_claim c={0};size_t i=4;
 memcpy(c.board,body+i,32);i+=32;memcpy(c.genesis,body+i,32);i+=32;memcpy(c.signer,body+i,20);i+=20;memcpy(c.asset,body+i,20);i+=20;memcpy(c.request,body+i,16);i+=16;memcpy(c.digest,body+i,32);i+=32;memcpy(c.block_hash,body+i,32);i+=32;
 c.generation=get64(body+i);i+=8;c.chain=get64(body+i);i+=8;c.nonce=get64(body+i);i+=8;c.block_height=get64(body+i);i+=8;c.finalized_height=get64(body+i);i+=8;c.block_time=get64(body+i);i+=8;c.status=body[i++];if(argc==6){memcpy(c.replacement_digest,body+i,32);i+=32;}assert(i==body_len);
 nu54_evm_policy policy={.chain=8283,.total_fee_limit=UINT64_MAX};memcpy(policy.token,c.asset,20);nu54_evm_intent intent;assert(!nu54_evm_decode(raw,n,&policy,&intent));
 nu54_finality_state v={0};v.policy.generation=c.generation;v.policy.revision=1;v.policy.chain=8283;v.policy.per_transfer=10;v.policy.daily=30;v.policy.day=c.block_time/86400;v.policy.last_lower=c.block_time;memcpy(v.policy.board,c.board,32);memcpy(v.policy.asset,c.asset,20);memcpy(v.policy.destination,intent.recipient,20);memcpy(v.genesis,c.genesis,32);memcpy(v.signer,c.signer,20);memcpy(v.authority,public_key,65);
 nu54_limit_request r={.generation=c.generation,.chain=8283,.amount=9,.expires=c.block_time+1000};memcpy(r.board,c.board,32);memcpy(r.asset,c.asset,20);memcpy(r.destination,intent.recipient,20);memcpy(r.id,c.request,16);memcpy(r.digest,c.digest,32);
 record=(stored_record){.schema=2,.phase=2,.state=v};exists=qualified=1;stored=independent=1;
 assert(!device_owner_finality_reserve_ps(&r,v.signer,raw,n,c.block_time,c.block_time));
 if(argc==6){size_t rn=hex(argv[5],replacement);assert(!device_owner_finality_reconcile_replacement_ps(&c,raw,n,replacement,rn,sig,c.block_time,c.block_time));
  assert(device_owner_finality_reconcile_replacement_ps(&c,raw,n,replacement,rn,sig,c.block_time,c.block_time)==NU54_FINALITY_ALREADY);
 } else {assert(!device_owner_finality_reconcile_ps(&c,raw,n,sig,c.block_time,c.block_time));
  assert(device_owner_finality_reconcile_ps(&c,raw,n,sig,c.block_time,c.block_time)==NU54_FINALITY_ALREADY);}
 assert(record.state.policy.count==0 && record.state.policy.day_spent==((c.status==NU54_FINALITY_SUCCESS||c.status==NU54_FINALITY_REPLACEMENT_SUCCESS)?9:0));
 EVP_PKEY_free(key);return 0;
}
