#ifndef DEVICE_OWNER_FINALITY_PS_H
#define DEVICE_OWNER_FINALITY_PS_H
#include "nu54_owner_finality_enroll.h"
#include <stdbool.h>
/* False until separately qualified boot, clock, rollback-protected PS, key
 * enrollment, signing-mode exclusion and management authorization exist. */
bool device_owner_finality_platform_qualified(void);
int device_owner_finality_enroll_ps(const nu54_finality_enrollment *,uint64_t,uint64_t,uint32_t,nu54_local_button,nu54_local_approval *,nu54_enroll_verify,const uint8_t [64],nu54_enroll_verify,const uint8_t [64],int (*)(const uint8_t [65]));
int device_owner_finality_resume_ps(const uint8_t [32],uint64_t,const uint8_t [20]);
bool device_owner_finality_pending_ps(const uint8_t [32],uint64_t);
int device_owner_finality_read_ps(const uint8_t [32],uint64_t,nu54_finality_state *);
bool device_owner_finality_ready_ps(const uint8_t [32],uint64_t,const uint8_t [20]);
int device_owner_finality_reserve_ps(const nu54_limit_request *,const uint8_t [20],const uint8_t *,size_t,uint64_t,uint64_t);
int device_owner_finality_reconcile_replacement_ps(const nu54_finality_claim *,const uint8_t *,size_t,const uint8_t *,size_t,const uint8_t [64],uint64_t,uint64_t);
int device_owner_finality_reconcile_ps(const nu54_finality_claim *,const uint8_t *,size_t,const uint8_t [64],uint64_t,uint64_t);
#endif
