#define main unused_device_evm_vector_main
#include "test_device_evm.c"
#undef main
#include "device_owner_limits_ps.h"
static int gate_calls;
uint64_t device_install_earliest(void) {return 1000;}
uint64_t device_install_now(void) {return 1000;}
int device_owner_limits_authorize_ps(const uint8_t board[32],uint64_t generation,uint64_t chain,
 const uint8_t asset[20],const uint8_t recipient[20],const uint8_t amount[32],const uint8_t request[16],
 const uint8_t digest[32],uint64_t expiry,uint64_t lower,uint64_t upper) {
 assert(board[0]==1 && generation==7 && chain==8283 && asset[0]==2 && recipient[0]==3 && amount[31]==4);
 assert(request[0]==5 && digest[0]==6 && expiry==2000 && lower==1000 && upper==1000);
 gate_calls++; return -1;
}
int main(void) {
 installed.revision=9; installed.not_after=3000;
 nu54_v2_link link={.profile=NU54_V2_MANAGEMENT,.local=NU54_V2_BOARD,.peer=NU54_V2_OWNER,.epoch={8},.pending_id={5}};
 device_evm state={.pending=true,.displayed=true,.prompt=1,.generation=7,.expiry=2000,.lease_revision=9,
  .address={7},.board={1},.request={5},.epoch={8},.intent={.chain=8283,.token={2},.recipient={3},.digest={6}}};
 state.intent.amount[31]=4;
 uint8_t out[LAB_TX_LENGTH],address[20]={7}; size_t size=0;
 assert(device_evm_complete(&state,&link,1000,7,1,address,out,&size)==-1);
 assert(gate_calls==1 && size==0);
 state.market=true;
 assert(device_evm_complete(&state,&link,1000,7,1,address,out,&size)==-1);
 assert(gate_calls==1);
 return 0;
}
