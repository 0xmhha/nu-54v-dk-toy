#ifndef NU54_OWNER_LIMITS_WIRE_H
#define NU54_OWNER_LIMITS_WIRE_H
#include "nu54_owner_limit_change.h"
#include <stdint.h>
#include <stddef.h>
#include <string.h>
#define NU54_LIMIT_SET_READ_OP 32
#define NU54_LIMIT_SET_CHANGE_OP 33
#define NU54_LIMIT_SET_CAPS_OP 34
#define NU54_LIMIT_SET_ACTIVATE_OP 35
#define NU54_LIMIT_SET_WIRE_VERSION 2
#define NU54_LIMIT_SET_BYTES (1 + 20 * NU54_LIMIT_RECIPIENTS)
#define NU54_LIMIT_SET_CHANGE_REQUEST_SIZE (10 + NU54_LIMIT_SET_BYTES)
#define NU54_LIMIT_SET_REPLY_SIZE (144 + 2 * NU54_LIMIT_SET_BYTES)
/* Shared parsing for the owner channel. Authentication, time, display and
 * physical approval remain the caller's responsibility. */
static inline uint64_t nu54_limits_get64(const uint8_t *p) {
 uint64_t value=0; for(size_t i=0;i<8;i++)value=(value<<8)|p[i]; return value;
}
static inline void nu54_limits_put64(uint8_t *p,uint64_t value) {
 for(size_t i=8;i>0;i--){p[i-1]=(uint8_t)value;value>>=8;}
}
static inline int nu54_limits_parse_set(const uint8_t *p, size_t n, const nu54_limit_recipients *active,
                                        nu54_limit_recipient_change *out) {
 if(!p || !active || !out || n<10 || p[1]!=NU54_LIMIT_SET_WIRE_VERSION)return -1;
 out->revision=nu54_limits_get64(p+2);
 if(!out->revision)return -1;
 if(p[0]==NU54_LIMIT_SET_CHANGE_OP && n==NU54_LIMIT_SET_CHANGE_REQUEST_SIZE){
   out->per_transfer=0;out->daily=0;
   out->recipients.count=p[10];memcpy(out->recipients.addresses,p+11,sizeof(out->recipients.addresses));
   return nu54_limits_recipients_valid(&out->recipients) ? 0 : -1;
 }
 if(p[0]!=NU54_LIMIT_SET_CAPS_OP || n!=26)return -1;
 out->recipients=*active;
 out->per_transfer=nu54_limits_get64(p+10);out->daily=nu54_limits_get64(p+18);
 return out->per_transfer && out->per_transfer<=out->daily ? 0 : -1;
}
static inline void nu54_limits_encode_set(const nu54_owner_limits *state, const nu54_limit_recipients *active,
    const nu54_limit_recipients *pending, uint8_t pending_kind, uint8_t reply[NU54_LIMIT_SET_REPLY_SIZE]) {
 memset(reply,0,NU54_LIMIT_SET_REPLY_SIZE);
 reply[0]=NU54_LIMIT_SET_READ_OP;reply[1]=NU54_LIMIT_SET_WIRE_VERSION;reply[2]=1;
 memcpy(reply+3,state->board,32);
 nu54_limits_put64(reply+35,state->generation);nu54_limits_put64(reply+43,state->revision);
 nu54_limits_put64(reply+51,state->chain);memcpy(reply+59,state->asset,20);
 nu54_limits_put64(reply+79,state->per_transfer);nu54_limits_put64(reply+87,state->daily);
 nu54_limits_put64(reply+95,state->day_spent);
 reply[103]=active->count;memcpy(reply+104,active->addresses,sizeof(active->addresses));
 size_t off=103+NU54_LIMIT_SET_BYTES;
 reply[off]=pending_kind;
 if(!pending_kind)return;
 nu54_limits_put64(reply+off+1,state->change.not_before);
 nu54_limits_put64(reply+off+9,state->change.per_transfer);nu54_limits_put64(reply+off+17,state->change.daily);
 memcpy(reply+off+25,state->change.request,16);
 reply[off+41]=pending->count;memcpy(reply+off+42,pending->addresses,sizeof(pending->addresses));
}
static inline int nu54_limits_parse_initialize(const uint8_t *p,size_t n,nu54_limit_change *out) {
 if(!p || !out || n!=65 || p[0]!=26)return -1;
 out->chain=nu54_limits_get64(p+1);memcpy(out->asset,p+9,20);memcpy(out->destination,p+29,20);
 out->per_transfer=nu54_limits_get64(p+49);out->daily=nu54_limits_get64(p+57);return 0;
}
static inline int nu54_limits_parse_change(const uint8_t *p,size_t n,nu54_limit_change *out) {
 if(!p || !out || n!=37 || p[0]!=24)return -1;
 out->per_transfer=nu54_limits_get64(p+1);out->daily=nu54_limits_get64(p+9);memcpy(out->destination,p+17,20);return 0;
}
#endif
