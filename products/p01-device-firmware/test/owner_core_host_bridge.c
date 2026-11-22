/* Test-only process exposing actual P01 owner/vault core. Transport, storage,
 * identity provisioning and physical button are simulated. */
#define main unused_owner_test_main
#include "test_owner.c"
#undef main
#include <stdio.h>
#include <stdlib.h>
#include <time.h>
typedef struct {nu54_vault_record vault;nu54_owner_enrollment enrollment;} snapshot;
static int owner_hash(const uint8_t *bytes,size_t n,uint8_t out[32]) {
 uint8_t identity[20]={42};if(n==sizeof(identity)&&!memcmp(bytes,identity,n)){memset(out,0xd1,32);return 0;}return hash(bytes,n,out);
}
static int unhex(const char *s,uint8_t *out,size_t cap){size_t n=strlen(s);if(n%2||n/2>cap)return -1;for(size_t i=0;i<n/2;i++){unsigned v;if(sscanf(s+2*i,"%2x",&v)!=1)return -1;out[i]=(uint8_t)v;}return (int)(n/2);}
static int synthetic_recovery(const uint8_t *body,size_t n,const uint8_t proof[64]){return n==4+NU54_OWNER_CHALLENGE_SIZE && !memcmp(body,"N54R",4) && proof[0]==42?0:-1;}
int main(int argc,char **argv){
 if(argc!=7)return 1;uint8_t payload[512],peer[32],id[16],out[NU54_OWNER_CHALLENGE_SIZE];
 int n=unhex(argv[2],payload,sizeof(payload));if(n<1||unhex(argv[3],peer,32)!=32||unhex(argv[4],id,16)!=16)return 2;
 uint64_t expires=strtoull(argv[5],NULL,10),now=(uint64_t)time(NULL);if(expires<=now)return 3;
 snapshot state={0};state.vault.identity[0]=42;
 if(!strcmp(argv[6],"lost") || !strcmp(argv[6],"lost-no-proof")){state.vault.generation=1;state.vault.flags=NU54_VAULT_PERSONAL|NU54_VAULT_BACKUP;state.vault.root_format=2;state.vault.root_profile_version=1;memset(state.vault.owner,16,32);memset(state.vault.personal,32,20);}
 FILE *f=fopen(argv[1],"rb");if(f){if(fread(&state,sizeof(state),1,f)!=1){fclose(f);return 4;}fclose(f);}
 nu54_vault vault={.enabled=true,.storage.write=save,.record=state.vault};nu54_owner_crypto crypto={random_bytes,owner_hash,verify};
 nu54_v2_link link={.profile=NU54_V2_MANAGEMENT,.local=NU54_V2_BOARD,.peer=NU54_V2_OWNER};memset(link.epoch,7,16);memcpy(link.peer_identity,peer,32);
 nu54_v2_request request={.command=NU54_V2_MANAGE,.expires=expires,.payload=payload,.payload_length=(size_t)n};memcpy(request.id,id,16);
 size_t output=0;
 if(payload[0]==3 && n==1){if(nu54_owner_status(&vault,&crypto,&state.enrollment,out))return 5;output=NU54_OWNER_STATUS_SIZE;}
 else if(payload[0]==1){int result=payload[1]==NU54_OWNER_REPLACE_LOST?nu54_owner_begin_lost(&state.enrollment,&vault,&crypto,&link,&request,now):nu54_owner_begin(&state.enrollment,&vault,&crypto,&link,&request,now,peer);if(result)return 6;memcpy(out,state.enrollment.record,NU54_OWNER_CHALLENGE_SIZE);output=NU54_OWNER_CHALLENGE_SIZE;}
 else if(payload[0]==2){if(nu54_owner_prove(&state.enrollment,&vault,&crypto,&link,&request,now,9))return 7;
  if(state.enrollment.record[1]==NU54_OWNER_REPLACE_LOST && strcmp(argv[6],"lost-no-proof")){uint8_t proof[64]={42};if(nu54_owner_authorize_lost(&state.enrollment,&vault,&link,now,synthetic_recovery,proof))return 7;}
  if(nu54_owner_confirm(&state.enrollment,&vault,&link,now,9,true)||nu54_owner_status(&vault,&crypto,&state.enrollment,out))return 7;output=NU54_OWNER_STATUS_SIZE;}
 else if(payload[0]==7 && n==1){if(nu54_owner_status(&vault,&crypto,&state.enrollment,out))return 5;memcpy(out+NU54_OWNER_STATUS_SIZE,vault.record.personal,20);output=NU54_OWNER_STATUS_SIZE+20;}
 else return 8;
 state.vault=vault.record;f=fopen(argv[1],"wb");if(!f)return 9;if(fwrite(&state,sizeof(state),1,f)!=1){fclose(f);return 10;}fclose(f);
 for(size_t i=0;i<output;i++)printf("%02x",out[i]);puts("");return 0;
}
