#define main unused_ps_fixture_main
#include "test_owner_limits_ps.c"
#undef main
#include "nu54_owner_limits_wire.h"
int main(void) {
 uint8_t wire[65]={26}; wire[7]=0x20;wire[8]=0x5b;wire[56]=10;wire[64]=30;
 nu54_limit_change parsed={0};assert(nu54_limits_parse_initialize(wire,sizeof(wire),&parsed)==0);
 assert(parsed.chain==8283 && parsed.per_transfer==10 && parsed.daily==30);
 assert(nu54_limits_parse_initialize(wire,sizeof(wire)-1,&parsed)==-1);
 uint8_t change[37]={24};change[8]=12;change[16]=30;change[17]=5;
 assert(nu54_limits_parse_change(change,sizeof(change),&parsed)==0);
 assert(parsed.per_transfer==12 && parsed.daily==30 && parsed.destination[0]==5);
 change[0]=25;assert(nu54_limits_parse_change(change,sizeof(change),&parsed)==-1);
 uint8_t board[32];memset(board,1,32);
 nu54_limit_change c={.generation=7,.revision=1,.chain=8283,.per_transfer=10000000,.daily=30000000,.expires=87000,.initialize=1};
 memcpy(c.board,board,32);memset(c.session,2,16);memset(c.request,3,16);memset(c.challenge,4,32);memset(c.asset,4,20);memset(c.destination,5,20);
 nu54_local_approval approval={0};uint8_t proof[64]={42};nu54_owner_limits result;
 assert(device_owner_limits_initialize_stage_ps(&c,86410,86410,1,&approval,hash,present,0)==-1);
 qualified=false;
 assert(device_owner_limits_mark_fresh_ps(board,7)==-1);
 qualified=true;
 assert(device_owner_limits_mark_fresh_ps(board,7)==0);
 assert(device_owner_limits_is_fresh_ps(board,7));
 assert(device_owner_limits_initialize_stage_ps(&c,86410,86410,1,&approval,hash,present,0)==0);
 uint8_t invalid[64]={0};
 assert(device_owner_limits_initialize_confirm_ps(&c,86420,86420,1,NU54_LOCAL_APPROVE,&approval,invalid,hash,verify)==-1);
 assert(!init_exists && !exists);
 assert(device_owner_limits_initialize_stage_ps(&c,86420,86420,2,&approval,hash,present,0)==0);
 assert(device_owner_limits_initialize_confirm_ps(&c,86420,86420,2,NU54_LOCAL_APPROVE,&approval,proof,hash,verify)==0);
 assert(device_owner_limits_read_ps(board,7,&result)==0 && result.per_transfer==10000000 && result.daily==30000000);
 assert(!device_owner_limits_is_fresh_ps(board,7));
 assert(device_owner_limits_claim_fresh_finality_ps(board,7)==-1);
 assert(device_owner_limits_initialize_stage_ps(&c,86420,86420,2,&approval,hash,present,0)==-1);
 /* Same normal adapter sequence: current 10/30, independently approved 12/30,
  * durable pending, trusted delay, activation, then a bounded transfer. */
 c.initialize=0;c.per_transfer=12000000;c.not_before=173400;
 approval=(nu54_local_approval){0};
 assert(device_owner_limits_stage_ps(&c,86420,86420,3,&approval,hash,present,0)==0);
 assert(device_owner_limits_confirm_ps(&c,86420,86420,3,NU54_LOCAL_APPROVE,&approval,proof,hash,verify)==0);
 assert(device_owner_limits_read_ps(board,7,&result)==0 && result.per_transfer==10000000 && result.change.pending);
 assert(device_owner_limits_activate_ps(173399,173399)==-1);
 assert(device_owner_limits_activate_ps(173400,173400)==0);
 assert(device_owner_limits_read_ps(board,7,&result)==0 && result.per_transfer==12000000 && result.revision==2);
 uint8_t amount[32]={0}, id[16]={7}, digest[32]={8};
 uint64_t value=11000000;for(int i=31;i>=24;i--){amount[i]=(uint8_t)value;value>>=8;}
 assert(device_owner_limits_authorize_ps(board,7,8283,c.asset,c.destination,amount,id,digest,174000,173400,173400)==0);
 assert(device_owner_limits_authorize_ps(board,7,8283,c.asset,c.destination,amount,id,digest,174000,173400,173400)==-1);
 exists=false; /* even deletion of a provisioned policy cannot be reinitialised */
 assert(device_owner_limits_initialize_stage_ps(&c,86420,86420,2,&approval,hash,present,0)==-1);
 /* A fresh V2 mode claim burns the same write-once witness that guards V1. */
 exists=false;fresh_exists=false;init_exists=false;independent_counter=stored_counter=0;
 assert(!device_owner_limits_mark_fresh_ps(board,7));assert(!device_owner_limits_claim_fresh_finality_ps(board,7));
 assert(!device_owner_limits_is_fresh_ps(board,7));assert(!device_owner_limits_claim_fresh_finality_ps(board,7));
 uint8_t other_board[32]={9};assert(device_owner_limits_claim_fresh_finality_ps(other_board,7)==-1);
 c.initialize=1;assert(device_owner_limits_initialize_stage_ps(&c,86420,86420,2,&approval,hash,present,0)==-1);
 return 0;
}
