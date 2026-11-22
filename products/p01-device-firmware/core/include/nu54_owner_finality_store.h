#ifndef NU54_OWNER_FINALITY_STORE_H
#define NU54_OWNER_FINALITY_STORE_H
#include "nu54_owner_finality.h"
/* A distinct v2 object, not a reinterpretation of the v1 PS record. */
typedef struct {uint64_t version;nu54_finality_state state;} nu54_finality_record;
typedef struct {void *context;int (*lock)(void *);void (*unlock)(void *);int (*load)(void *,nu54_finality_record *);int (*write)(void *,const nu54_finality_record *);int (*read_counter)(void *,uint64_t *);int (*advance)(void *,uint64_t);} nu54_finality_port;
int nu54_finality_commit(const nu54_finality_port *,const nu54_finality_claim *,const uint8_t *,size_t,const uint8_t [64],nu54_finality_verify,nu54_finality_hash,uint64_t,uint64_t);
#endif
