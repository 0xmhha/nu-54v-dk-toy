/* EVM approval, PSA policy and HD derivation with host substitutes for hardware services. */
#define main unused_device_evm_vector_main
#define device_hd_sign fixture_device_hd_sign
#include "test_device_evm.c"
#undef device_hd_sign
#undef main
#define main unused_ps_fixture_main
#include "test_owner_limits_ps.c"
#undef main
#include "nu54_hd.h"
#include "nu54_keccak.h"
#include "nu54_owner_limits_wire.h"
#include <openssl/hmac.h>
#include <zephyr/sys/byteorder.h>

static uint64_t trusted_now=86410;
static const uint8_t host_seed[NU54_HD_SEED_SIZE] = {7};
static const uint8_t blinding[32] = {8};
static bool fail_signing;

uint64_t device_install_earliest(void) { return trusted_now; }
uint64_t device_install_now(void) { return trusted_now; }

static int host_hmac(const uint8_t *key, size_t key_length, const uint8_t *message, size_t length, uint8_t out[64])
{
 unsigned used = 0;
 return HMAC(EVP_sha512(), key, (int)key_length, message, length, out, &used) && used == 64 ? 0 : -1;
}

int device_hd_sign(const uint8_t address[20], const uint8_t digest[32], uint8_t signature[64])
{
 /* The host supplies the seed and blinding at the ITS/KMU boundary. */
 if (!device_owner_limits_ready_ps(data.state.board, 7) || fail_signing ||
     nu54_hd_sign_first_evm(host_seed, blinding, host_hmac, address, digest, signature)) return -1;
 host_signature_count++;
 return 0;
}

static int policy_hash(const uint8_t *bytes,size_t size,uint8_t out[32]) {
 nu54_keccak256(bytes,size,out);return 0;
}
static int panel_verify(const uint8_t *bytes,size_t size,const uint8_t proof[64]) {
 uint8_t digest[32];nu54_keccak256(bytes,size,digest);
 return (size==NU54_LIMIT_RECIPIENT_STATEMENT_SIZE || size==NU54_LIMIT_SET_STATEMENT_MAX) &&
        !memcmp(digest,proof,32) ? 0 : -1;
}

static int attempt_transfer(uint8_t recipient,uint8_t token,uint8_t id)
{
 uint8_t address[20],public_key[NU54_HD_PUBLIC_SIZE],out[LAB_TX_LENGTH],wire[NU54_DIRECT_RAW_OFFSET+LAB_TX_LENGTH]={0};
 assert(nu54_hd_first_evm(host_seed,blinding,host_hmac,public_key,address)==0);
 nu54_v2_link link={.profile=NU54_V2_MANAGEMENT,.local=NU54_V2_BOARD,.peer=NU54_V2_OWNER,.epoch={8},.pending_id={id}};
 const uint64_t expires=trusted_now+300;
 installed.not_after=expires+1;
 wire[0]=NU54_EVM_TRANSFER_OP;
 sys_put_be64(expires,wire+1);
 sys_put_be64(8283,wire+9);
 sys_put_be64(600000,wire+17);
 memset(wire+33,token,20);
 wire[53]=1;
 sys_put_be64(1,wire+85);
 sys_put_be64(installed.revision,wire+93);
 authority_signature[0]=0xa2;
 memcpy(wire+NU54_DIRECT_PROOF_HEADER,authority_signature,sizeof(authority_signature));
 uint8_t *raw=wire+NU54_DIRECT_RAW_OFFSET;
 size_t raw_length=hex("02f86682205b03030c82a02894040404040404040404040404040404040404040480b844"
     "a9059cbb0000000000000000000000000505050505050505050505050505050505050505"
     "0000000000000000000000000000000000000000000000000000000000000005c0",raw,LAB_TX_LENGTH);
 memset(raw+13,token,20);memset(raw+raw_length-53,recipient,20);
 nu54_evm_policy policy={.chain=8283,.total_fee_limit=600000};
 memset(policy.token,token,20);
 nu54_evm_intent intent;
 assert(!nu54_evm_decode(raw,raw_length,&policy,&intent));
 assert(intent.recipient[0]==recipient && intent.token[0]==token);
 memcpy(authority_statement,"N54V",4);
 memcpy(authority_statement+4,data.state.board,32);
 sys_put_be64(7,authority_statement+36);
 memcpy(authority_statement+44,address,20);
 memcpy(authority_statement+64,wire,NU54_DIRECT_PROOF_HEADER);
 memcpy(authority_statement+64+NU54_DIRECT_PROOF_HEADER,intent.digest,32);
 authority_size=64+NU54_DIRECT_PROOF_HEADER+32;
 nu54_v2_request request={.id={id},.expires=expires,.payload=wire,.payload_length=NU54_DIRECT_RAW_OFFSET+raw_length};
 device_evm state={0};
 assert(!device_evm_stage(&state,&request,&link,trusted_now,7,1,data.state.board,address));
 memcpy(display_body,"N54X",4);
 memset(display_body+4,0x42,32);
 memcpy(display_body+36,wire,request.payload_length);
 display_size=36+request.payload_length;
 assert(!device_evm_display(&state,trusted_now,(uint8_t[64]){0x56}));
 size_t size=0;
 int status=device_evm_complete(&state,&link,trusted_now,7,1,address,out,&size);
 assert(status ? size==0 : (size>raw_length && out[0]==2));
 return status;
}

static void test_recipient_application(void)
{
 data=(struct record){.schema=1,.state={.generation=7,.revision=1,.per_transfer=10,.daily=30,.day=1,.last_lower=86400,.chain=8283}};
 memset(data.state.board,1,32);memset(data.state.asset,4,20);memset(data.state.destination,5,20);
 assert(device_owner_limits_migrate_recipients_ps(data.state.board,7)==0);
 nu54_owner_limits before;nu54_limit_recipients active,pending;
 assert(device_owner_limits_read_recipients_ps(data.state.board,7,&before,&active,&pending)==0);
 nu54_limit_recipient_change change={.generation=7,.revision=1,.chain=8283,.expires=87000,.not_before=173400,
     .session={2},.request={3},.challenge={4},.recipients={.count=2}};
 memcpy(change.board,before.board,32);memcpy(change.asset,before.asset,20);
 memset(change.recipients.addresses[0],5,20);memset(change.recipients.addresses[1],6,20);
 nu54_local_approval approval={0};
 uint8_t statement[NU54_LIMIT_RECIPIENT_STATEMENT_SIZE],proof[64]={0};
 assert(nu54_limit_recipient_statement(&before,&active,&change,statement)==0);
 policy_hash(statement,sizeof(statement),proof);
 nu54_limit_recipient_change altered=change;
 altered.recipients.addresses[1][0]=5;
 memcpy(altered.recipients.addresses[1],altered.recipients.addresses[0],20);
 assert(device_owner_limits_stage_recipients_ps(&altered,trusted_now,trusted_now,1,&approval,policy_hash,present,NULL)==-1);
 assert(device_owner_limits_stage_recipients_ps(&change,trusted_now,trusted_now,1,&approval,policy_hash,present,NULL)==0);
 altered=change;altered.recipients.addresses[1][0]=7;
 assert(device_owner_limits_confirm_recipients_ps(&altered,trusted_now,trusted_now,1,NU54_LOCAL_APPROVE,
           &approval,proof,policy_hash,panel_verify)==-1 && !approval.pending);
 assert(device_owner_limits_stage_recipients_ps(&change,trusted_now,trusted_now,2,&approval,policy_hash,present,NULL)==0);
 assert(device_owner_limits_confirm_recipients_ps(&change,trusted_now,trusted_now,2,NU54_LOCAL_APPROVE,
           &approval,proof,policy_hash,panel_verify)==0);
 assert(device_owner_limits_read_ps(change.board,7,&before)==-1);
 assert(!device_owner_limits_ready_ps(change.board,8));
 assert(!device_owner_limits_ready_ps((uint8_t[32]){0},7));
 assert(device_owner_limits_read_recipients_ps(change.board,7,&before,&active,&pending)==0);
 assert(before.change.not_before==173400 && active.count==1 && pending.count==2);
 unsigned signed_before=host_signature_count;
 assert(attempt_transfer(6,4,10)==-1 && host_signature_count==signed_before);
 assert(attempt_transfer(5,4,11)==0 && host_signature_count==signed_before+1);
 uint64_t previous_counter=independent_counter;
 assert(attempt_transfer(5,4,11)==-1 && host_signature_count==signed_before+1 && independent_counter==previous_counter);
 assert(device_owner_limits_activate_recipients_ps(change.board,7,173399,173399)==-1);
 assert(device_owner_limits_activate_recipients_ps(change.board,8,173400,173400)==-1);
 trusted_now=173400;
 error_before_write=true;
 assert(device_owner_limits_activate_recipients_ps(change.board,7,trusted_now,trusted_now)==-1);
 error_before_write=false;
 assert(device_owner_limits_read_recipients_ps(change.board,7,&before,&active,&pending)==0 &&
        before.revision==1 && active.count==1 && pending.count==2);
 error_after_write=true;
 assert(device_owner_limits_activate_recipients_ps(change.board,7,trusted_now,trusted_now)==-1);
 error_after_write=false;
 assert(device_owner_limits_read_recipients_ps(change.board,7,&before,&active,&pending)==0 &&
        before.revision==2 && active.count==2 && !before.change.pending);
 assert(data.state.revision==2 && data.state.count==1 && data.state.day_spent==5);
 assert(attempt_transfer(7,4,12)==-1 && attempt_transfer(6,3,12)==-1);
 assert(attempt_transfer(5,4,12)==0 && attempt_transfer(6,4,13)==0);
 assert(host_signature_count==signed_before+3 && data.state.day_spent==15 && data.state.count==3);

 /* A signer failure after reserve leaves the debit in place. Restoring the
  * signer cannot replay a request without an independently persisted receipt. */
 fail_signing=true;
 assert(attempt_transfer(6,4,18)==-1 && data.state.count==4 && data.state.day_spent==20);
 fail_signing=false;
 previous_counter=independent_counter;
 assert(attempt_transfer(6,4,18)==-1 && independent_counter==previous_counter &&
        host_signature_count==signed_before+3);

 /* Removing A leaves B eligible and retains the old A reservations. */
 assert(device_owner_limits_read_recipients_ps(change.board,7,&before,&active,&pending)==0 && active.count==2);
 change.revision=2;change.expires=173700;change.not_before=260100;change.request[0]++;
 memset(&change.recipients,0,sizeof(change.recipients));
 change.recipients.count=1;memset(change.recipients.addresses[0],6,20);
 assert(nu54_limit_recipient_statement(&before,&active,&change,statement)==0);
 policy_hash(statement,sizeof(statement),proof);
 assert(device_owner_limits_stage_recipients_ps(&change,trusted_now,trusted_now,3,&approval,policy_hash,present,NULL)==0);
 assert(device_owner_limits_confirm_recipients_ps(&change,trusted_now,trusted_now,3,NU54_LOCAL_APPROVE,
           &approval,proof,policy_hash,panel_verify)==0);
 trusted_now=260100;
 assert(device_owner_limits_activate_recipients_ps(change.board,7,trusted_now,trusted_now)==0);
 assert(attempt_transfer(5,4,14)==-1 && attempt_transfer(6,4,15)==0);
 assert(data.state.count==5 && data.state.day_spent==25 && host_signature_count==signed_before+4);

 /* An explicitly empty policy permits no new transfer. */
 assert(device_owner_limits_read_recipients_ps(change.board,7,&before,&active,&pending)==0 && active.count==1);
 change.revision=3;change.expires=260400;change.not_before=346800;change.request[0]++;
 memset(&change.recipients,0,sizeof(change.recipients));
 assert(nu54_limit_recipient_statement(&before,&active,&change,statement)==0);
 policy_hash(statement,sizeof(statement),proof);
 assert(device_owner_limits_stage_recipients_ps(&change,trusted_now,trusted_now,4,&approval,policy_hash,present,NULL)==0);
 assert(device_owner_limits_confirm_recipients_ps(&change,trusted_now,trusted_now,4,NU54_LOCAL_APPROVE,
           &approval,proof,policy_hash,panel_verify)==0);
 trusted_now=346800;
 assert(device_owner_limits_activate_recipients_ps(change.board,7,trusted_now,trusted_now)==0);
 assert(attempt_transfer(5,4,16)==-1 && attempt_transfer(6,4,17)==-1);
 assert(data.state.count==5 && data.state.day_spent==25 && host_signature_count==signed_before+4);
}

static void test_cap_change_with_eight_recipients(void)
{
 trusted_now=86410;
 data=(struct record){.schema=2,.state={.generation=7,.revision=4,.per_transfer=10,.daily=30,
     .day=1,.last_lower=86400,.chain=8283}};
 memset(data.state.board,1,32);memset(data.state.asset,4,20);
 nu54_limit_recipients seeded={.count=8};
 for(size_t i=0;i<8;i++)memset(seeded.addresses[i],(int)i+1,20);
 memset(extension,0,sizeof(extension));
 memcpy(extension,&seeded,sizeof(seeded));
 assert(!attempt_transfer(5,4,21));
 nu54_owner_limits current;nu54_limit_recipients active,candidate;
 enum nu54_limit_pending_kind kind;
 assert(!device_owner_limits_read_set_ps(data.state.board,7,&current,&active,&candidate,&kind));
 assert(kind==NU54_LIMIT_PENDING_NONE && current.count==1 && current.day_spent==5);
 assert(device_owner_limits_read_ps(data.state.board,7,&current));

 uint8_t wire[26]={NU54_LIMIT_SET_CAPS_OP,2};
 nu54_limits_put64(wire+2,4);nu54_limits_put64(wire+10,8);nu54_limits_put64(wire+18,20);
 nu54_limit_recipient_change change={.generation=7,.chain=8283,.not_before=173400,.expires=87000,
     .session={2},.request={3},.challenge={4}};
 memcpy(change.board,current.board,32);memcpy(change.asset,current.asset,20);
 assert(!nu54_limits_parse_set(wire,sizeof(wire),&active,&change));
 uint8_t statement[NU54_LIMIT_SET_STATEMENT_MAX],proof[64]={0};size_t statement_length;
 assert(!nu54_limit_set_statement(&current,&active,&change,statement,&statement_length));
 assert(statement_length==514 && !memcmp(statement,"N54M",4));
 policy_hash(statement,statement_length,proof);
 nu54_local_approval approval={0};
 assert(!device_owner_limits_stage_recipients_ps(&change,trusted_now,trusted_now,1,&approval,policy_hash,present,NULL));
 nu54_limit_recipient_change changed=change;changed.daily=21;
 assert(device_owner_limits_confirm_recipients_ps(&changed,trusted_now,trusted_now,1,NU54_LOCAL_APPROVE,
        &approval,proof,policy_hash,panel_verify));
 assert(!device_owner_limits_stage_recipients_ps(&change,trusted_now,trusted_now,2,&approval,policy_hash,present,NULL));
 assert(!device_owner_limits_confirm_recipients_ps(&change,trusted_now,trusted_now,2,NU54_LOCAL_APPROVE,
        &approval,proof,policy_hash,panel_verify));
 assert(!device_owner_limits_read_set_ps(change.board,7,&current,&active,&candidate,&kind));
 assert(kind==NU54_LIMIT_PENDING_CAPS && active.count==8 && candidate.count==8 &&
        !memcmp(active.addresses,candidate.addresses,sizeof(active.addresses)) &&
        current.change.per_transfer==8 && current.change.daily==20 && current.daily==30);
 uint8_t reply[NU54_LIMIT_SET_REPLY_SIZE];
 nu54_limits_encode_set(&current,&active,&candidate,(uint8_t)kind,reply);
 assert(sizeof(reply)==466 && reply[103]==8 && reply[264]==NU54_LIMIT_PENDING_CAPS && reply[305]==8);
 assert(device_owner_limits_activate_set_ps(change.board,7,4,change.request,173399,173399));
 assert(device_owner_limits_activate_set_ps(change.board,7,5,change.request,173400,173400));
 assert(device_owner_limits_activate_set_ps(change.board,7,4,(uint8_t[16]){9},173400,173400));
 error_after_write=true;
 assert(device_owner_limits_activate_set_ps(change.board,7,4,change.request,173400,173400));
 error_after_write=false;
 assert(device_owner_limits_activate_set_ps(change.board,7,4,change.request,173400,173400));
 assert(!device_owner_limits_read_set_ps(change.board,7,&current,&active,&candidate,&kind));
 assert(kind==NU54_LIMIT_PENDING_NONE && current.revision==5 && current.per_transfer==8 && current.daily==20 &&
        active.count==8 && current.count==1 && current.day_spent==5 && current.reservations[0].id[0]==21);
 trusted_now=173400;
 assert(attempt_transfer(9,4,22)==-1 && !attempt_transfer(8,4,22));
}

int main(void) {
 data=(struct record){.schema=1,.state={.generation=7,.revision=1,.per_transfer=10,.daily=30,.day=1,.last_lower=86400,.chain=8283}};
 memset(data.state.board,1,32); memset(data.state.asset,4,20); memset(data.state.destination,5,20);
 exists=true; stored_counter=independent_counter=1;
 installed.revision=9; installed.not_after=90000; host_key[31]=1;
 nu54_v2_link link={.profile=NU54_V2_MANAGEMENT,.local=NU54_V2_BOARD,.peer=NU54_V2_OWNER,.epoch={8},.pending_id={5}};
 device_evm state={.pending=true,.displayed=true,.prompt=1,.generation=7,.expiry=87000,.lease_revision=9,.address={7},.request={5},.epoch={8}};
 memset(state.board,1,32); state.intent.chain=8283; memset(state.intent.token,4,20); memset(state.intent.recipient,5,20);
 memset(state.intent.digest,6,32); state.intent.amount[31]=11;
 uint8_t out[LAB_TX_LENGTH],address[20]={7};size_t size=0;
 assert(device_evm_complete(&state,&link,86410,7,1,address,out,&size)==-1);
 assert(data.state.day_spent==0 && state.pending);
 state.intent.amount[31]=10;
 /* The dummy address/transaction fails after durable reservation; no release is inferred. */
 assert(device_evm_complete(&state,&link,86410,7,1,address,out,&size)==-1);
 assert(data.state.day_spent==10 && independent_counter==2);
 state.pending=true;
 assert(device_evm_complete(&state,&link,86410,7,1,address,out,&size)==-1);
 assert(data.state.day_spent==10 && independent_counter==2 && state.pending);
 state.intent.digest[0]^=1;
 assert(device_evm_complete(&state,&link,86410,7,1,address,out,&size)==-1);
 assert(data.state.day_spent==10 && independent_counter==2);
 test_recipient_application();
 test_cap_change_with_eight_recipients();
 return 0;
}
