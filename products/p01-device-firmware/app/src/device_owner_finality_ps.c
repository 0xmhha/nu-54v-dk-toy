#include "device_owner_finality_ps.h"
#include "device_owner_limits_ps.h"
#include <psa/crypto.h>
#include <psa/protected_storage.h>
#include <zephyr/kernel.h>
#include <string.h>
#if !defined(CONFIG_TRUSTED_EXECUTION_NONSECURE) || !defined(CONFIG_TFM_PARTITION_PROTECTED_STORAGE) || !defined(CONFIG_TFM_PS_ROLLBACK_PROTECTION)
#error "Finality adapter requires rollback-protected TF-M PS"
#endif
/* The separate UID is intentionally not provisioned by any current route.
 * Missing state, an unqualified platform or ambiguous writes fail closed. */
#define FINALITY_UID ((psa_storage_uid_t)0x4e35344c00000004ULL)
#define FINALITY_SCHEMA 2u
struct finality_record {uint32_t schema,phase;uint8_t enrollment_digest[32];nu54_finality_state state;};
_Static_assert(sizeof(struct finality_record)<=2048,"finality PS asset too large");
K_MUTEX_DEFINE(finality_lock);
__attribute__((weak)) bool device_owner_finality_platform_qualified(void){return false;}
static int digest(const uint8_t *body,size_t n,uint8_t out[32]) {
 size_t written=0;return psa_hash_compute(PSA_ALG_SHA_256,body,n,out,32,&written)==PSA_SUCCESS && written==32 ? 0 : -1;
}
static int verify(const uint8_t public_key[65],const uint8_t *body,size_t n,const uint8_t signature[64]) {
 psa_key_attributes_t attrs=PSA_KEY_ATTRIBUTES_INIT;psa_key_id_t key=0;uint8_t hash[32];
 psa_set_key_type(&attrs,PSA_KEY_TYPE_ECC_PUBLIC_KEY(PSA_ECC_FAMILY_SECP_R1));psa_set_key_bits(&attrs,256);
 psa_set_key_usage_flags(&attrs,PSA_KEY_USAGE_VERIFY_HASH);psa_set_key_algorithm(&attrs,PSA_ALG_ECDSA(PSA_ALG_SHA_256));
 psa_status_t imported=psa_import_key(&attrs,public_key,65,&key);psa_reset_key_attributes(&attrs);
 if(imported!=PSA_SUCCESS)return -1;
 int result=digest(body,n,hash) || psa_verify_hash(key,PSA_ALG_ECDSA(PSA_ALG_SHA_256),hash,sizeof(hash),signature,64)!=PSA_SUCCESS ? -1 : 0;
 psa_destroy_key(key);return result;
}
static int load_any(struct finality_record *record) {
 size_t n=0;return psa_ps_get(FINALITY_UID,0,sizeof(*record),record,&n)==PSA_SUCCESS && n==sizeof(*record) && record->schema==FINALITY_SCHEMA && (record->phase==1 || record->phase==2)?0:-1;
}
static int load(struct finality_record *record) {return !load_any(record) && record->phase==2?0:-1;}
/* Resume is only for a previously authenticated and physically approved
 * prepared profile; the v1 write-once witness proves exclusive fresh mode. */
static int resume_locked(struct finality_record *record,const uint8_t board[32],uint64_t generation) {
 if(memcmp(record->state.policy.board,board,32)||record->state.policy.generation!=generation)return -1;
 if(record->phase==2)return NU54_FINALITY_ALREADY;
 if(device_owner_limits_claim_fresh_finality_ps(board,generation))return -1;
 record->phase=2;return psa_ps_set(FINALITY_UID,sizeof(*record),record,PSA_STORAGE_FLAG_NONE)==PSA_SUCCESS?0:-1;
}
int device_owner_finality_resume_ps(const uint8_t board[32],uint64_t generation,const uint8_t signer[20]) {
 if(!device_owner_finality_platform_qualified()||!board||!signer||!generation||k_mutex_lock(&finality_lock,K_FOREVER))return -1;
 struct finality_record record;int result=load_any(&record)||memcmp(record.state.signer,signer,20)?-1:resume_locked(&record,board,generation);
 k_mutex_unlock(&finality_lock);return result;
}
int device_owner_finality_enroll_ps(const nu54_finality_enrollment *e,uint64_t lower,uint64_t upper,uint32_t prompt,nu54_local_button button,nu54_local_approval *approval,nu54_enroll_verify verify_authority,const uint8_t authority_proof[64],nu54_enroll_verify verify_display,const uint8_t display_proof[64],int (*independent)(const uint8_t [65])) {
 if(!device_owner_finality_platform_qualified()||!e||!verify_authority||!authority_proof||!independent||!independent(e->authority))return -1;
 uint8_t body[NU54_FINALITY_ENROLL_BODY_SIZE],grant[NU54_FINALITY_GRANT_SIZE],fingerprint[32];nu54_finality_state approved;
 if(nu54_finality_enrollment_body(e,body)||nu54_finality_grant_body(e,grant)||verify_authority(grant,sizeof(grant),authority_proof)||digest(body,sizeof(body),fingerprint)||
    nu54_finality_enrollment_confirm(e,lower,upper,prompt,button,approval,digest,verify_display,display_proof,&approved))return -1;
 if(k_mutex_lock(&finality_lock,K_FOREVER))return -1;
 int result=-1;struct finality_record current;struct psa_storage_info_t info;
 psa_status_t status=psa_ps_get_info(FINALITY_UID,&info);
 if(status==PSA_ERROR_DOES_NOT_EXIST){
   memset(&current,0,sizeof(current));current.schema=FINALITY_SCHEMA;current.phase=1;current.state=approved;memcpy(current.enrollment_digest,fingerprint,32);
   if(psa_ps_set(FINALITY_UID,sizeof(current),&current,PSA_STORAGE_FLAG_NONE)!=PSA_SUCCESS)goto done_enroll;
 } else if(status!=PSA_SUCCESS||load_any(&current)||memcmp(current.enrollment_digest,fingerprint,32))goto done_enroll;
 result=resume_locked(&current,e->board,e->generation);
 done_enroll:k_mutex_unlock(&finality_lock);return result;
}
bool device_owner_finality_pending_ps(const uint8_t board[32],uint64_t generation) {
 if(!device_owner_finality_platform_qualified()||!board||!generation||k_mutex_lock(&finality_lock,K_FOREVER))return false;
 struct finality_record current;bool pending=!load_any(&current) && current.phase==1 && current.state.policy.generation==generation && !memcmp(current.state.policy.board,board,32);
 k_mutex_unlock(&finality_lock);return pending;
}
int device_owner_finality_read_ps(const uint8_t board[32],uint64_t generation,nu54_finality_state *out) {
 if(!device_owner_finality_platform_qualified()||!board||!out||!generation||k_mutex_lock(&finality_lock,K_FOREVER))return -1;
 struct finality_record current;int result=-1;
 if(!load(&current) && current.state.policy.generation==generation && !memcmp(current.state.policy.board,board,32) && current.state.policy.chain==8283){*out=current.state;result=0;}
 k_mutex_unlock(&finality_lock);return result;
}
bool device_owner_finality_ready_ps(const uint8_t board[32],uint64_t generation,const uint8_t signer[20]) {
 if(!device_owner_finality_platform_qualified() || !board || !signer || !generation || k_mutex_lock(&finality_lock,K_FOREVER))return false;
 struct finality_record current;bool ready=!load(&current) && current.state.policy.generation==generation &&
   !memcmp(current.state.policy.board,board,32) && !memcmp(current.state.signer,signer,20) && current.state.policy.chain==8283;
 k_mutex_unlock(&finality_lock);return ready;
}
int device_owner_finality_reserve_ps(const nu54_limit_request *request,const uint8_t signer[20],const uint8_t *raw,size_t raw_len,uint64_t lower,uint64_t upper) {
 if(!device_owner_finality_platform_qualified()||k_mutex_lock(&finality_lock,K_FOREVER))return -1;
 struct finality_record current,next;int result=-1;
 if(load(&current)||nu54_finality_reserve(&current.state,request,signer,raw,raw_len,lower,upper,&next.state)!=NU54_LIMIT_NEW)goto done;
 next.schema=FINALITY_SCHEMA;next.phase=2;memcpy(next.enrollment_digest,current.enrollment_digest,32);if(psa_ps_set(FINALITY_UID,sizeof(next),&next,PSA_STORAGE_FLAG_NONE)==PSA_SUCCESS)result=0;
 done:k_mutex_unlock(&finality_lock);return result;
}
int device_owner_finality_reconcile_ps(const nu54_finality_claim *claim,const uint8_t *raw,size_t raw_len,const uint8_t signature[64],uint64_t lower,uint64_t upper) {
 if(!device_owner_finality_platform_qualified()||k_mutex_lock(&finality_lock,K_FOREVER))return -1;
 struct finality_record current,next;int result=-1;if(load(&current))goto done;
 int decision=nu54_finality_apply(&current.state,claim,raw,raw_len,signature,verify,digest,lower,upper,&next.state);
 if(decision==NU54_FINALITY_ALREADY){result=decision;goto done;}if(decision)goto done;
 next.schema=FINALITY_SCHEMA;next.phase=2;memcpy(next.enrollment_digest,current.enrollment_digest,32);if(psa_ps_set(FINALITY_UID,sizeof(next),&next,PSA_STORAGE_FLAG_NONE)==PSA_SUCCESS)result=0;
 done:k_mutex_unlock(&finality_lock);return result;
}

int device_owner_finality_reconcile_replacement_ps(const nu54_finality_claim *claim,const uint8_t *raw,size_t raw_len,const uint8_t *replacement,size_t replacement_len,const uint8_t signature[64],uint64_t lower,uint64_t upper) {
 if(!device_owner_finality_platform_qualified()||k_mutex_lock(&finality_lock,K_FOREVER))return -1;
 struct finality_record current,next;int result=-1;if(load(&current))goto done;
 int decision=nu54_finality_apply_replacement(&current.state,claim,raw,raw_len,replacement,replacement_len,signature,verify,digest,lower,upper,&next.state);
 if(decision==NU54_FINALITY_ALREADY){result=decision;goto done;}if(decision)goto done;
 next.schema=FINALITY_SCHEMA;next.phase=2;memcpy(next.enrollment_digest,current.enrollment_digest,32);
 if(psa_ps_set(FINALITY_UID,sizeof(next),&next,PSA_STORAGE_FLAG_NONE)==PSA_SUCCESS)result=0;
 done:k_mutex_unlock(&finality_lock);return result;
}
