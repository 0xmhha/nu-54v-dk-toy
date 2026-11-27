/* Test-only host process: synthetic PS, clock, display verifier and button.
 * Never use this executable as a device or trust evidence. */
#define main unused_ps_fixture_main
#include "test_owner_limits_ps.c"
#undef main
#define main unused_device_evm_vector_main
#include "test_device_evm.c"
#undef main
#include "nu54_owner_limits_wire.h"
static uint64_t clock_now;
uint64_t device_install_earliest(void) {return clock_now;}
uint64_t device_install_now(void) {return clock_now;}
#include <stdio.h>
#include <stdlib.h>
#include <openssl/pem.h>
#include <openssl/ecdsa.h>
#include <openssl/sha.h>
#include <sys/wait.h>
#include <time.h>
#include <unistd.h>
struct saved {
 struct record data;
 struct fresh fresh,init;
 uint64_t independent,stored;
 bool exists,fresh_exists,init_exists;
 size_t ps_size;
 uint8_t extension[sizeof(extension)];
};
static void be(uint8_t *p,uint64_t v){for(int i=7;i>=0;i--){p[i]=(uint8_t)v;v>>=8;}}
static int unhex(const char *s,uint8_t *out,size_t cap){size_t n=strlen(s);if(n%2||n/2>cap)return -1;for(size_t i=0;i<n/2;i++){unsigned v;if(sscanf(s+i*2,"%2x",&v)!=1)return -1;out[i]=(uint8_t)v;}return (int)(n/2);}
static int sha(const uint8_t *data,size_t length,uint8_t out[32])
{
 return SHA256(data,length,out)?0:-1;
}
static int verify_panel(const uint8_t *data,size_t length,const uint8_t proof[64])
{
 FILE *file=fopen("policy-display.pem","r");
 if(!file)return -1;
 EVP_PKEY *key=PEM_read_PUBKEY(file,NULL,NULL,NULL);
 fclose(file);
 ECDSA_SIG *signature=ECDSA_SIG_new();
 BIGNUM *r=BN_bin2bn(proof,32,NULL),*s=BN_bin2bn(proof+32,32,NULL);
 EVP_MD_CTX *ctx=EVP_MD_CTX_new();
 uint8_t der[80],*cursor=der;
 int result=-1;
 if(key && signature && r && s && ctx && ECDSA_SIG_set0(signature,r,s)) {
  r=NULL;s=NULL;
  int size=i2d_ECDSA_SIG(signature,&cursor);
  if(size>0 && EVP_DigestVerifyInit(ctx,NULL,EVP_sha256(),NULL,key)==1 &&
     EVP_DigestVerify(ctx,der,(size_t)size,data,length)==1)result=0;
 }
 EVP_MD_CTX_free(ctx); ECDSA_SIG_free(signature); EVP_PKEY_free(key); BN_free(r); BN_free(s);
 return result;
}
static int panel_proof(const uint8_t challenge[32], const char *operation, uint8_t proof[64])
{
 FILE *file=fopen("policy-challenge","wb");
 if(!file)return -1;
 int result=fwrite(challenge,32,1,file)==1?0:-1;
 fclose(file);
 if(result)return result;
 unlink("policy-display-proof");
 unlink("policy-sw1-event");
 pid_t child=fork();
 if(child==0) {
  dup2(STDERR_FILENO,STDOUT_FILENO);
  execlp("python3","python3",getenv("NU54_W27_PANEL"),".",operation,(char *)NULL);
  _exit(127);
 }
 int status=0;
 if(child<0 || waitpid(child,&status,0)!=child || !WIFEXITED(status) || WEXITSTATUS(status))return -1;
 file=fopen("policy-display-proof","rb");
 if(!file)return -1;
 result=fread(proof,64,1,file)==1 && fgetc(file)==EOF?0:-1;
 fclose(file);
 if(result)return result;
 const struct timespec interval={.tv_nsec=20*1000*1000};
 file=NULL;
 for(unsigned attempt=0;!file && attempt<100;attempt++) {
  file=fopen("policy-sw1-event","rb");
  if(!file)nanosleep(&interval,NULL);
 }
 if(!file)return -1;
 char event[5]={0};
 result=fread(event,1,4,file)==4 && strcmp(event,"SW1\n")==0?0:-1;
 fclose(file);
 return result;
}
int main(int argc,char **argv){
 bool native=argc==9; if(!native && argc!=5 && argc!=6)return 1;const char *file=argv[1];uint8_t wire[NU54_LIMIT_SET_CHANGE_REQUEST_SIZE],reply[NU54_LIMIT_SET_REPLY_SIZE]={0};int n=unhex(argv[2],wire,sizeof(wire));if(n<1)return 2;
 uint64_t expiry=strtoull(argv[3],NULL,10);if(expiry<110)return 3;uint64_t now=expiry-110;clock_now=argc>=6?strtoull(argv[5],NULL,10):now; if(native)now=clock_now;
 struct saved saved={0};FILE *f=fopen(file,"rb");if(f){if(fread(&saved,sizeof(saved),1,f)!=1){fclose(f);return 4;}fclose(f);data=saved.data;fresh_record=saved.fresh;init_record=saved.init;independent_counter=saved.independent;stored_counter=saved.stored;exists=saved.exists;fresh_exists=saved.fresh_exists;init_exists=saved.init_exists;
 if(saved.ps_size<sizeof(data) || saved.ps_size-sizeof(data)>sizeof(extension))return 4;
 ps_size=saved.ps_size;memcpy(extension,saved.extension,sizeof(extension));}
 uint8_t board[32];memset(board,0xd1,32);if(native && unhex(argv[6],board,32)!=32)return 12;if(!fresh_exists && !init_exists && !exists && device_owner_limits_mark_fresh_ps(board,1))return 5;
 nu54_owner_limits stored={0};int available=!device_owner_limits_read_ps(board,1,&stored);size_t len=0;
 if (wire[0] >= NU54_LIMIT_SET_READ_OP && wire[0] <= NU54_LIMIT_SET_ACTIVATE_OP && n >= 2 && wire[1] == 2) {
  nu54_owner_limits state;
  nu54_limit_recipients active,pending;
  enum nu54_limit_pending_kind kind;
  bool found=!device_owner_limits_read_set_ps(board,1,&state,&active,&pending,&kind);
  reply[0]=wire[0];reply[1]=2;len=3;
  if(wire[0]==NU54_LIMIT_SET_READ_OP && n==2){
   if(found){nu54_limits_encode_set(&state,&active,&pending,(uint8_t)kind,reply);len=sizeof(reply);}
  }else if(wire[0]==NU54_LIMIT_SET_ACTIVATE_OP && n==26){
   reply[2]=!device_owner_limits_activate_set_ps(board,1,nu54_limits_get64(wire+2),wire+10,clock_now,clock_now);
  }else if(found && (wire[0]==NU54_LIMIT_SET_CHANGE_OP || wire[0]==NU54_LIMIT_SET_CAPS_OP)){
   nu54_limit_recipient_change change={.generation=1,.chain=state.chain,.expires=expiry,
       .not_before=expiry+NU54_LIMIT_CHANGE_DELAY};
   if(nu54_limits_parse_set(wire,(size_t)n,&active,&change) || unhex(argv[4],change.request,16)!=16)return 6;
   memcpy(change.board,board,32);memcpy(change.asset,state.asset,20);
   memset(change.session,7,16);memset(change.challenge,9,32);
   if(native && (unhex(argv[7],change.session,16)!=16 || unhex(argv[8],change.challenge,32)!=32))return 12;
   nu54_local_approval approval={0};
   uint8_t proof[64]={42};
   nu54_limit_hash digest=native?sha:hash;
   if(device_owner_limits_stage_recipients_ps(&change,now,now,1,&approval,digest,present,NULL))return 7;
   if(native && panel_proof(change.challenge,wire[0]==NU54_LIMIT_SET_CHANGE_OP?"recipients":"caps",proof))return 8;
   if(device_owner_limits_confirm_recipients_ps(&change,now,now,1,NU54_LOCAL_APPROVE,&approval,proof,digest,
                                               native?verify_panel:verify))return 8;
   reply[2]=1;be(reply+3,change.not_before);len=11;
  }else return 9;
 }
 else if(native && wire[0]==100 && n==9){
  nu54_owner_limits current;
  nu54_limit_recipients active,pending;
  enum nu54_limit_pending_kind kind;
  if(device_owner_limits_read_set_ps(board,1,&current,&active,&pending,&kind) || !active.count)return 14;
  nu54_limit_request reserve={.generation=1,.chain=current.chain,.amount=nu54_limits_get64(wire+1),.expires=expiry};
  memcpy(reserve.board,board,32);memcpy(reserve.asset,current.asset,20);memcpy(reserve.destination,active.addresses[0],20);
  if(unhex(argv[4],reserve.id,16)!=16 || !reserve.amount)return 12;
  memset(reserve.digest,0xa5,sizeof(reserve.digest));
  int decision=device_owner_limits_reserve_ps(&reserve,now,now,&current);
  if(decision!=NU54_LIMIT_NEW && decision!=NU54_LIMIT_RETRY)return 14;
  reply[0]=100;reply[1]=decision==NU54_LIMIT_NEW?1:2;len=2;
 }
 else if(wire[0]==23 && n==1){reply[0]=23;reply[1]=available?1:(device_owner_limits_is_fresh_ps(board,1)?2:0);len=2;
  if(available){size_t i=2;memcpy(reply+i,board,32);i+=32;be(reply+i,stored.generation);i+=8;be(reply+i,stored.revision);i+=8;be(reply+i,stored.chain);i+=8;memcpy(reply+i,stored.asset,20);i+=20;memcpy(reply+i,stored.destination,20);i+=20;be(reply+i,stored.per_transfer);i+=8;be(reply+i,stored.daily);i+=8;be(reply+i,stored.day_spent);i+=8;reply[i++]=stored.change.pending;if(stored.change.pending){be(reply+i,stored.change.per_transfer);i+=8;be(reply+i,stored.change.daily);i+=8;be(reply+i,stored.change.not_before);i+=8;memcpy(reply+i,stored.change.destination,20);i+=20;}len=i;}}
 else if(wire[0]==26 || wire[0]==24){nu54_limit_change c={0};int init=wire[0]==26;if((init?nu54_limits_parse_initialize(wire,(size_t)n,&c):nu54_limits_parse_change(wire,(size_t)n,&c)) || (init?available:!available))return 6;
 c.initialize=init;c.generation=1;c.revision=init?1:stored.revision;if(!init){c.chain=stored.chain;memcpy(c.asset,stored.asset,20);}c.expires=expiry;c.not_before=init?0:expiry+NU54_LIMIT_CHANGE_DELAY;memcpy(c.board,board,32);memset(c.session,7,16);if(unhex(argv[4],c.request,16)!=16)return 12;memset(c.challenge,9,32);if(native && (unhex(argv[7],c.session,16)!=16 || unhex(argv[8],c.challenge,32)!=32))return 12;
 nu54_local_approval approval={0};
 uint8_t proof[64]={42};
 nu54_limit_hash digest=native?sha:hash;
 int stage=init?device_owner_limits_initialize_stage_ps(&c,now,now,1,&approval,digest,present,NULL):
                device_owner_limits_stage_ps(&c,now,now,1,&approval,digest,present,NULL);
 if(stage)return 7;
 if(native) {
  nu54_owner_limits baseline=stored;
  if(init) {
   baseline=(nu54_owner_limits){.generation=1,.revision=1,.chain=c.chain,.per_transfer=c.per_transfer,.daily=c.daily};
   memcpy(baseline.board,c.board,32);memcpy(baseline.asset,c.asset,20);memcpy(baseline.destination,c.destination,20);
  }
  if(panel_proof(c.challenge,c.initialize?"initialize":"change",proof))return 8;
 }
 int confirm=init?device_owner_limits_initialize_confirm_ps(&c,now,now,1,NU54_LOCAL_APPROVE,&approval,proof,digest,native?verify_panel:verify):
                  device_owner_limits_confirm_ps(&c,now,now,1,NU54_LOCAL_APPROVE,&approval,proof,digest,native?verify_panel:verify);
 if(confirm)return 8;
 reply[0]=wire[0];reply[1]=1;len=2;if(!init){be(reply+2,c.not_before);len=10;}}
 else if(wire[0]==25 && n==1 && available){
  reply[0]=25;
  reply[1]=device_owner_limits_activate_ps(device_install_earliest(),device_install_now())==0;
  len=2;
 }
 else if(wire[0]==99 && n==9 && available){
  installed.revision=9;installed.not_after=expiry+1;host_key[31]=1;
  nu54_v2_link link={.profile=NU54_V2_MANAGEMENT,.local=NU54_V2_BOARD,.peer=NU54_V2_OWNER};memset(link.epoch,7,16);
  if(unhex(argv[4],link.pending_id,16)!=16)return 12;
  device_evm state={.pending=true,.displayed=true,.prompt=1,.generation=1,.expiry=expiry,.lease_revision=9,.address={7}};
  memcpy(state.board,board,32);memcpy(state.epoch,link.epoch,16);memcpy(state.request,link.pending_id,16);
  state.intent.chain=stored.chain;memcpy(state.intent.token,stored.asset,20);memcpy(state.intent.recipient,stored.destination,20);
  memset(state.intent.digest,6,32);memcpy(state.intent.amount+24,wire+1,8);
  uint8_t output[LAB_TX_LENGTH],address[20]={7};size_t output_size=0;
  int result=device_evm_complete(&state,&link,now,1,1,address,output,&output_size);
  if(result==0)return 13;
  reply[0]=99;reply[1]=state.pending?0:1;len=2;
 }
 else return 9;
 saved=(struct saved){.data=data,.fresh=fresh_record,.init=init_record,.independent=independent_counter,.stored=stored_counter,.exists=exists,.fresh_exists=fresh_exists,.init_exists=init_exists,.ps_size=ps_size};memcpy(saved.extension,extension,sizeof(extension));f=fopen(file,"wb");if(!f)return 10;if(fwrite(&saved,sizeof(saved),1,f)!=1){fclose(f);return 11;}fclose(f);
 for(size_t i=0;i<len;i++)printf("%02x",reply[i]);
 puts("");return 0;
}
