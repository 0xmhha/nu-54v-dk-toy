/* Actual EVM completion -> actual v2 PSA adapter, hardware services simulated. */
#define main unused_device_evm_vector_main
#define hex evm_fixture_hex
#include "test_device_evm.c"
#undef main
#undef hex
#define FINALITY_PS_EMBED
#include "test_owner_finality_ps.c"
#undef main
uint64_t device_install_earliest(void){return 86410;}
uint64_t device_install_now(void){return 86410;}
int main(void){
 EVP_PKEY_CTX *k=EVP_PKEY_CTX_new_id(EVP_PKEY_EC,NULL);assert(k&&EVP_PKEY_keygen_init(k)==1&&EVP_PKEY_CTX_set_ec_paramgen_curve_nid(k,NID_X9_62_prime256v1)==1&&EVP_PKEY_keygen(k,&key)==1);EVP_PKEY_CTX_free(k);
 record.schema=2;record.phase=2;record.state.policy=(nu54_owner_limits){.generation=7,.revision=1,.chain=8283,.per_transfer=10,.daily=30,.day=1,.last_lower=86400};
 memset(record.state.policy.board,1,32);memset(record.state.policy.asset,4,20);memset(record.state.policy.destination,5,20);memset(record.state.genesis,8,32);record.state.signer[0]=7;
 size_t pub_size=65;assert(EVP_PKEY_get_octet_string_param(key,"pub",record.state.authority,65,&pub_size)==1);exists=1;stored=independent=1;
 uint8_t raw[256];size_t n=hex("02f86782205b010101830186a094040404040404040404040404040404040404040480b844a9059cbb00000000000000000000000005050505050505050505050505050505050505050000000000000000000000000000000000000000000000000000000000000009c0",raw);
 installed.revision=9;installed.not_after=90000;host_key[31]=1;
 nu54_v2_link link={.profile=NU54_V2_MANAGEMENT,.local=NU54_V2_BOARD,.peer=NU54_V2_OWNER,.epoch={8},.pending_id={5}};
 device_evm state={.pending=true,.displayed=true,.prompt=1,.generation=7,.expiry=87000,.lease_revision=9,.address={7},.request={5},.epoch={8},.length=n};
 memset(state.board,1,32);memcpy(state.wire,raw,n);state.policy=(nu54_evm_policy){.chain=8283,.total_fee_limit=1000000};memset(state.policy.token,4,20);
 assert(!nu54_evm_decode(raw,n,&state.policy,&state.intent));uint8_t out[LAB_TX_LENGTH],address[20]={7};size_t size=0;
 qualified=0;assert(device_evm_complete(&state,&link,86410,7,1,address,out,&size)==-1 && !record.state.policy.count);
 qualified=1;assert(device_evm_complete(&state,&link,86410,7,1,address,out,&size)==-1); /* synthetic address: signature fails */
 assert(record.state.policy.count==1 && record.state.policy.day_spent==9);
 state.pending=true;assert(device_evm_complete(&state,&link,86410,7,1,address,out,&size)==-1);
 assert(record.state.policy.count==1 && record.state.policy.day_spent==9);
 EVP_PKEY_free(key);return 0;
}
