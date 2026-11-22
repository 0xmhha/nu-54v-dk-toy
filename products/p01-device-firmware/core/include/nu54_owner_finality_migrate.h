#ifndef NU54_OWNER_FINALITY_MIGRATE_H
#define NU54_OWNER_FINALITY_MIGRATE_H
#include "nu54_owner_finality.h"
typedef struct {const uint8_t *bytes;size_t length;} nu54_migration_original;
/* Must verify rollback-protected, complete signer-issued nonce history including
 * every non-reservation route, and bind it to this exact V1 snapshot and signer.
 * An RPC account nonce or handset history is never an acceptable witness. */
typedef int (*nu54_migration_lineage)(const nu54_owner_limits *,const uint8_t signer[20],uint64_t *high_nonce,int *has_nonce,uint8_t journal_digest[32]);
int nu54_finality_migration_prepare(const nu54_owner_limits *,const nu54_finality_state *approved_profile,
 const nu54_migration_original *,size_t,nu54_migration_lineage,nu54_finality_state *,uint8_t journal_digest[32]);
typedef struct {nu54_finality_state state;uint8_t journal_digest[32];} nu54_migration_prepared;
/* All V1 signature routes, snapshot checks and immutable fence must share this
 * lock. read_v1 is a privileged recovery read even after the fence. The fence
 * operation is idempotent only for the exact snapshot and journal. */
typedef struct {
 void *context;int (*lock)(void *);void (*unlock)(void *);
 int (*read_v1)(void *,nu54_owner_limits *);int (*read_prepared)(void *,nu54_migration_prepared *);
 int (*write_prepared)(void *,const nu54_migration_prepared *);
 int (*check_lineage)(void *,const nu54_owner_limits *,const uint8_t signer[20],uint64_t *,int *,uint8_t [32]);
 int (*fence)(void *,const nu54_owner_limits *,const uint8_t journal_digest[32]);
 int (*activate)(void *,const nu54_migration_prepared *);
} nu54_migration_port;
int nu54_finality_migration_resume(const nu54_migration_port *);
#endif
