#include "device_owner_limits_ps.h"
#include <psa/protected_storage.h>
#include "nu54_quote_v2_generated.h"
#include <assert.h>
#include <stdbool.h>
#include <string.h>

struct record {uint32_t schema; nu54_owner_limits state;};
static struct record data;
static uint8_t extension[512];
static size_t ps_size=sizeof(data);
struct fresh {uint32_t schema; uint64_t generation; uint8_t board[32];};
static struct fresh fresh_record;
static bool fresh_exists;
static struct fresh init_record;
static bool init_exists;
static uint64_t independent_counter, stored_counter;
static bool exists, error_before_write, error_after_write;
static bool qualified = true;
bool device_owner_limits_platform_qualified(void) {return qualified;}
#define TEST_FRESH_UID UINT64_C(0x4e35344c00000002)
#define TEST_INIT_UID UINT64_C(0x4e35344c00000003)
psa_status_t psa_ps_get(psa_storage_uid_t uid,size_t offset,size_t size,void *out,size_t *length) {
 if(uid==TEST_FRESH_UID || uid==TEST_INIT_UID) {struct fresh *r=uid==TEST_FRESH_UID?&fresh_record:&init_record; bool valid=uid==TEST_FRESH_UID?fresh_exists:init_exists; if(!valid || offset || size!=sizeof(*r))return -1;memcpy(out,r,size);*length=size;return 0;}
 if (!exists || independent_counter!=stored_counter || offset || size!=ps_size) return -1;
 memcpy(out,&data,sizeof(data));
 if(size>sizeof(data))memcpy((uint8_t *)out+sizeof(data),extension,size-sizeof(data));
 *length=size; return 0;
}
psa_status_t psa_ps_set(psa_storage_uid_t uid,size_t size,const void *input,uint32_t flags) {
 if(uid==TEST_FRESH_UID || uid==TEST_INIT_UID) {struct fresh *r=uid==TEST_FRESH_UID?&fresh_record:&init_record; bool *valid=uid==TEST_FRESH_UID?&fresh_exists:&init_exists; if(*valid || size!=sizeof(*r) || flags!=PSA_STORAGE_FLAG_WRITE_ONCE)return -1; memcpy(r,input,size); *valid=true; return 0;}
 (void)flags;
 if(error_before_write || size<sizeof(data) || size-sizeof(data)>sizeof(extension))return -1;
 memcpy(&data,input,sizeof(data));
 if(size>sizeof(data))memcpy(extension,(const uint8_t *)input+sizeof(data),size-sizeof(data));
 ps_size=size; exists=true; stored_counter=++independent_counter;
 return error_after_write ? -1 : PSA_SUCCESS;
}
psa_status_t psa_ps_get_info(psa_storage_uid_t uid,struct psa_storage_info_t *info) {
 if(uid==TEST_FRESH_UID || uid==TEST_INIT_UID){if(!(uid==TEST_FRESH_UID?fresh_exists:init_exists))return PSA_ERROR_DOES_NOT_EXIST; info->size=sizeof(fresh_record); info->flags=PSA_STORAGE_FLAG_WRITE_ONCE; return 0;}
 if(!exists)return PSA_ERROR_DOES_NOT_EXIST;
 info->size=ps_size;return 0;
}
static int hash(const uint8_t *v,size_t n,uint8_t out[32]) {memset(out,0,32);for(size_t i=0;i<n;i++)out[i%32]^=(uint8_t)(v[i]+i);return 0;}
static int present(void *v,const uint8_t *p,size_t n) {(void)v;(void)p;return n==66?0:-1;}
static int verify(const uint8_t *v,size_t n,const uint8_t proof[64]) {(void)v;(void)n;return proof[0]==42?0:-1;}
static void test_recipient_migration(void)
{
 data=(struct record){.schema=1,.state={.generation=7,.revision=2,.per_transfer=10,.daily=30,
      .day=1,.day_spent=10,.last_lower=86400,.chain=8283,.count=1}};
 memset(data.state.board,1,32);memset(data.state.asset,4,20);memset(data.state.destination,5,20);
 data.state.reservations[0]=(nu54_limit_reservation){.amount=10,.expires=90000,.id={3},.digest={7}};
 data.state.change.pending=1;data.state.change.per_transfer=15;data.state.change.daily=40;
 data.state.change.not_before=173400;memset(data.state.change.destination,6,20);
 data.state.change.request[0]=8;data.state.change.statement_digest[0]=9;
 const nu54_owner_limits original=data.state;
 nu54_owner_limits state; nu54_limit_recipients active,pending;
 assert(device_owner_limits_read_recipients_ps(original.board,7,&state,&active,&pending)==0);
 assert(active.count==1 && active.addresses[0][0]==5 && pending.count==1 && pending.addresses[0][0]==6);
 error_before_write=true;
 assert(device_owner_limits_migrate_recipients_ps(original.board,7)==-1);
 assert(data.schema==1 && ps_size==sizeof(data));
 error_before_write=false; error_after_write=true;
 assert(device_owner_limits_migrate_recipients_ps(original.board,7)==-1);
 error_after_write=false;
 assert(data.schema==2 && ps_size>sizeof(data));
 uint64_t fence=independent_counter;
 assert(device_owner_limits_migrate_recipients_ps(original.board,7)==0 && independent_counter==fence);
 assert(device_owner_limits_read_recipients_ps(original.board,7,&state,&active,&pending)==0);
 assert(!memcmp(&state,&original,sizeof(state)));
 assert(active.count==1 && active.addresses[0][0]==5 && pending.count==1 && pending.addresses[0][0]==6);
 assert(device_owner_limits_activate_ps(173400,173400)==-1);
 assert(device_owner_limits_activate_set_ps(original.board,7,2,original.change.request,173400,173400)==-1);
 assert(independent_counter==fence);
 data.state.count=0;
 memset(data.state.reservations,0,sizeof(data.state.reservations));
 assert(device_owner_limits_activate_set_ps(original.board,7,2,original.change.request,173399,173399)==-1);
 assert(device_owner_limits_activate_set_ps(original.board,7,2,(uint8_t[16]){9},173400,173400)==-1);
 assert(device_owner_limits_activate_set_ps(original.board,7,2,original.change.request,173400,173400)==0);
 assert(device_owner_limits_read_recipients_ps(original.board,7,&state,&active,&pending)==0);
 assert(state.revision==3 && state.per_transfer==15 && state.daily==40 && state.day_spent==0 &&
        active.count==1 && active.addresses[0][0]==6 && !state.change.pending);
}

int main(void) {
 nu54_limit_request req={.generation=7,.amount=9000000,.expires=172000};
 memset(req.board,1,32); req.chain=8283; memset(req.asset,4,20); memset(req.destination,5,20); memset(req.id,1,16); memset(req.digest,2,32);
 nu54_owner_limits out;
 assert(device_owner_limits_mark_fresh_ps(req.board,7)==0);
 assert(device_owner_limits_mark_fresh_ps(req.board,7)==0);
 uint8_t wrong_fresh[32]={1}; assert(device_owner_limits_mark_fresh_ps(wrong_fresh,7)==-1);
 assert(device_owner_limits_reserve_ps(&req,86410,86410,&out)==NU54_LIMIT_REJECT);
 uint8_t amount[32]={0}; amount[29]=0x89; amount[30]=0x54; amount[31]=0x40;
 assert(device_owner_limits_authorize_ps(req.board,7,8283,req.asset,req.destination,amount,req.id,req.digest,req.expires,86410,86410)==-1);
 data=(struct record){.schema=1,.state={.generation=7,.revision=1,.per_transfer=10000000,.daily=30000000,.day=1,.last_lower=86400}};
 memset(data.state.board,1,32); data.state.chain=8283; memset(data.state.asset,4,20); memset(data.state.destination,5,20);
 exists=true; stored_counter=independent_counter=1;
 struct record old=data; uint64_t old_counter=stored_counter;
 assert(device_owner_limits_authorize_ps(req.board,7,8283,req.asset,req.destination,amount,req.id,req.digest,req.expires,86410,86410)==0);
 assert(device_owner_limits_authorize_ps(req.board,7,8283,req.asset,req.destination,amount,req.id,req.digest,req.expires,86410,86410)==-1);
 out=data.state;
 assert(out.day_spent==9000000 && independent_counter==2);
 assert(device_owner_limits_reserve_ps(&req,86410,86410,&out)==NU54_LIMIT_RETRY);
 struct record current=data; uint64_t current_counter=stored_counter;
 data=old; stored_counter=old_counter;
 assert(device_owner_limits_reserve_ps(&req,86410,86410,&out)==NU54_LIMIT_REJECT);
 data=current; stored_counter=current_counter; req.id[0]=3; error_after_write=true;
 assert(device_owner_limits_reserve_ps(&req,86410,86410,&out)==NU54_LIMIT_REJECT);
 error_after_write=false;
 assert(device_owner_limits_reserve_ps(&req,86410,86410,&out)==NU54_LIMIT_RETRY);
 assert(out.day_spent==18000000);
 data=(struct record){.schema=1,.state={.generation=7,.revision=1,.per_transfer=10000000,.daily=30000000,.day=1,.last_lower=86400,.chain=8283}};
 memset(data.state.board,1,32); memset(data.state.asset,4,20); memset(data.state.destination,5,20);
 stored_counter=independent_counter=5;
 uint8_t quote[NU54_V2_UNSIGNED_SIZE]={0}; quote[NU54_V2_CHAIN_ID_OFFSET+30]=0x20; quote[NU54_V2_CHAIN_ID_OFFSET+31]=0x5b;
 memcpy(quote+NU54_V2_ASSET_OFFSET+12,req.asset,20); memcpy(quote+NU54_V2_BENEFICIARY_OFFSET+12,req.destination,20);
 memcpy(quote+NU54_V2_AMOUNT_OFFSET,amount,32);
 assert(device_owner_limits_authorize_pilot_ps(req.board,7,quote,req.id,req.digest,req.expires,86410,86410)==0);
 assert(device_owner_limits_authorize_pilot_ps(req.board,7,quote,req.id,req.digest,req.expires,86410,86410)==-1);
 data=(struct record){.schema=1,.state={.generation=7,.revision=1,.per_transfer=10000000,.daily=30000000,.day=1,.last_lower=86400,.chain=8283}};
 memset(data.state.board,1,32); memset(data.state.asset,4,20); memset(data.state.destination,5,20);
 stored_counter=independent_counter=10;
 nu54_limit_change c={.generation=7,.revision=1,.chain=8283,.per_transfer=12000000,.daily=30000000,.not_before=173400,.expires=87000};
 memset(c.board,1,32); memset(c.session,2,16); memset(c.request,3,16); memset(c.challenge,4,32); memset(c.asset,4,20); memset(c.destination,5,20);
 nu54_owner_limits readback; uint8_t wrong_board[32]={0};
 assert(device_owner_limits_read_ps(wrong_board,7,&readback)==-1);
 assert(device_owner_limits_read_ps(c.board,8,&readback)==-1);
 assert(device_owner_limits_read_ps(c.board,7,&readback)==0 && readback.per_transfer==10000000);
 nu54_local_approval approval={0}; uint8_t proof[64]={42};
 assert(device_owner_limits_stage_ps(&c,86410,86410,1,&approval,hash,present,0)==0);
 assert(device_owner_limits_confirm_ps(&c,86420,86420,1,NU54_LOCAL_APPROVE,&approval,proof,hash,verify)==0);
 assert(data.state.per_transfer==10000000 && data.state.change.pending);
 assert(!memcmp(data.state.change.request,c.request,16));
 assert(device_owner_limits_read_ps(c.board,7,&readback)==0 && readback.change.pending);
 assert(device_owner_limits_activate_ps(0,0)==-1);
 assert(device_owner_limits_activate_ps(173399,173399)==-1);
 assert(data.state.per_transfer==10000000);
 assert(device_owner_limits_activate_ps(173400,173400)==0);
 assert(data.state.per_transfer==12000000 && data.state.revision==2);
 assert(device_owner_limits_read_ps(c.board,7,&readback)==0 && readback.per_transfer==12000000);
 data.state.per_transfer=10000000; data.state.revision=1; data.state.last_lower=86400;
 stored_counter=independent_counter=20; approval=(nu54_local_approval){0};
 assert(device_owner_limits_stage_ps(&c,86410,86410,2,&approval,hash,present,0)==0);
 error_after_write=true;
 assert(device_owner_limits_confirm_ps(&c,86420,86420,2,NU54_LOCAL_APPROVE,&approval,proof,hash,verify)==-1);
 error_after_write=false;
 assert(device_owner_limits_read_ps(c.board,7,&readback)==0 && readback.change.pending);
 assert(!memcmp(readback.change.request,c.request,16) && readback.per_transfer==10000000);
 assert(device_owner_limits_stage_ps(&c,86420,86420,3,&approval,hash,present,0)==-1);
 /* Unknown write at UTC rollover: both the full old reservation carry and the
  * new reservation commit together, and a retry cannot authorize signing. */
 data=(struct record){.schema=1,.state={.generation=7,.revision=1,.per_transfer=10000000,.daily=30000000,.day=1,.last_lower=86400,.chain=8283}};
 memset(data.state.board,1,32);memset(data.state.asset,4,20);memset(data.state.destination,5,20);
 stored_counter=independent_counter=30;req.expires=172000;memset(req.id,10,16);
 assert(device_owner_limits_reserve_ps(&req,86410,86410,&out)==NU54_LIMIT_NEW);
 req.expires=259000;memset(req.id,11,16);error_after_write=true;
 assert(device_owner_limits_reserve_ps(&req,172800,172800,&out)==NU54_LIMIT_REJECT);
 error_after_write=false;
 assert(device_owner_limits_read_ps(req.board,7,&out)==0 && out.day==2 && out.day_spent==18000000 && out.count==2);
 assert(device_owner_limits_reserve_ps(&req,172800,172800,&out)==NU54_LIMIT_RETRY && out.day_spent==18000000);
 test_recipient_migration();
 return 0;
}
