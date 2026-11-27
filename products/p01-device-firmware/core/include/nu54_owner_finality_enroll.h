#ifndef NU54_OWNER_FINALITY_ENROLL_H
#define NU54_OWNER_FINALITY_ENROLL_H
#include "nu54_owner_finality.h"
#include "nu54_local_approval.h"
#define NU54_FINALITY_ENROLL_BODY_SIZE 297u
#define NU54_FINALITY_GRANT_SIZE 233u
typedef struct {
 uint8_t board[32],session[16],request[16],challenge[32],signer[20],genesis[32],asset[20],destination[20],authority[65];
 uint64_t generation,chain,per_transfer,daily,expires;
} nu54_finality_enrollment;
typedef int (*nu54_enroll_verify)(const uint8_t *,size_t,const uint8_t [64]);
int nu54_finality_grant_parse(const uint8_t *,size_t,nu54_finality_enrollment *);
int nu54_finality_grant_body(const nu54_finality_enrollment *,uint8_t [NU54_FINALITY_GRANT_SIZE]);
int nu54_finality_enrollment_body(const nu54_finality_enrollment *,uint8_t [NU54_FINALITY_ENROLL_BODY_SIZE]);
int nu54_finality_enrollment_stage(const nu54_finality_enrollment *,uint64_t,uint64_t,uint32_t,nu54_local_approval *,nu54_finality_hash,nu54_enroll_verify,const uint8_t [64],int (*)(const uint8_t [65]),int (*)(void *,const uint8_t *,size_t),void *);
int nu54_finality_enrollment_confirm(const nu54_finality_enrollment *,uint64_t,uint64_t,uint32_t,nu54_local_button,nu54_local_approval *,nu54_finality_hash,nu54_enroll_verify,const uint8_t [64],nu54_finality_state *);
#endif
