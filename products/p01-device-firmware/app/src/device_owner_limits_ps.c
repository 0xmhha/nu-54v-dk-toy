#include "device_owner_limits_ps.h"
#include <psa/protected_storage.h>
#include <zephyr/kernel.h>
#include <stdint.h>
#include <string.h>

#if !defined(CONFIG_TRUSTED_EXECUTION_NONSECURE) || !defined(CONFIG_TFM_PARTITION_PROTECTED_STORAGE) || !defined(CONFIG_TFM_PS_ROLLBACK_PROTECTION)
#error "Owner-limit adapter requires TF-M protected storage with rollback protection"
#endif

/* Both schemas use the same atomic PS asset. Older firmware rejects schema 2. */
#define OWNER_LIMITS_PS_UID ((psa_storage_uid_t)0x4e35344c00000001ULL)
#define OWNER_LIMITS_SCHEMA 1u
#define OWNER_LIMITS_RECIPIENT_SCHEMA 2u
#define OWNER_LIMITS_FRESH_UID ((psa_storage_uid_t)0x4e35344c00000002ULL)
#define OWNER_LIMITS_INITIALIZED_UID ((psa_storage_uid_t)0x4e35344c00000003ULL)
enum { RECIPIENTS_UNCHANGED, RECIPIENTS_REPLACE, CAPS_REPLACE };
struct owner_limits_fresh_record { uint32_t schema; uint64_t generation; uint8_t board[32]; };
struct owner_limits_v1_record { uint32_t schema; nu54_owner_limits state; };
struct owner_limits_ps_record {
    uint32_t schema;
    nu54_owner_limits state;
    nu54_limit_recipients active, pending;
    uint8_t recipient_change;
};
_Static_assert(offsetof(struct owner_limits_ps_record,active) == sizeof(struct owner_limits_v1_record),
               "schema 1 prefix layout changed");
_Static_assert(sizeof(struct owner_limits_ps_record) <= 2048, "owner limit record exceeds qualified PS asset size");
K_MUTEX_DEFINE(owner_limits_ps_lock);
__attribute__((weak)) bool device_owner_limits_platform_qualified(void) { return false; }

static void singleton(nu54_limit_recipients *set, const uint8_t address[20])
{
    memset(set,0,sizeof(*set));
    set->count=1;
    memcpy(set->addresses[0],address,20);
}

static int load_record(struct owner_limits_ps_record *record)
{
    struct psa_storage_info_t info;
    size_t length=0;
    memset(record,0,sizeof(*record));
    if (psa_ps_get_info(OWNER_LIMITS_PS_UID,&info)!=PSA_SUCCESS ||
        (info.size!=sizeof(struct owner_limits_v1_record) && info.size!=sizeof(*record)) ||
        psa_ps_get(OWNER_LIMITS_PS_UID,0,info.size,record,&length)!=PSA_SUCCESS || length!=info.size) return -1;
    if (length==sizeof(struct owner_limits_v1_record) && record->schema==OWNER_LIMITS_SCHEMA) {
        singleton(&record->active,record->state.destination);
        if (record->state.change.pending) singleton(&record->pending,record->state.change.destination);
    } else if (length!=sizeof(*record) || record->schema!=OWNER_LIMITS_RECIPIENT_SCHEMA) return -1;
    return record->state.change.pending<=1 && record->recipient_change<=CAPS_REPLACE &&
           (!record->recipient_change || record->state.change.pending) &&
           (record->state.change.pending || !record->pending.count) &&
           nu54_limits_recipients_valid(&record->active) &&
           nu54_limits_recipients_valid(&record->pending) ? 0 : -1;
}

static int write_record(const struct owner_limits_ps_record *record)
{
    size_t size=record->schema==OWNER_LIMITS_SCHEMA ? sizeof(struct owner_limits_v1_record) : sizeof(*record);
    /* PS provides an authenticated atomic write. A write error does not authorize signing. */
    return psa_ps_set(OWNER_LIMITS_PS_UID,size,record,PSA_STORAGE_FLAG_NONE)==PSA_SUCCESS ? 0 : -1;
}

int device_owner_limits_reserve_ps(const nu54_limit_request *request, uint64_t lower, uint64_t upper,
                                   nu54_owner_limits *out)
{
    if (!device_owner_limits_platform_qualified() || !request || !out || k_mutex_lock(&owner_limits_ps_lock,K_FOREVER)) return NU54_LIMIT_REJECT;
    int result=NU54_LIMIT_REJECT;
    struct owner_limits_ps_record record;
    nu54_owner_limits next;
    if (load_record(&record)) goto done;
    int decision=nu54_limits_reserve_recipients(&record.state,&record.active,request,lower,upper,&next);
    if (decision==NU54_LIMIT_NEW) {
        record.state=next;
        if (write_record(&record)) goto done;
    }
    if (decision!=NU54_LIMIT_REJECT) { *out=next; result=decision; }
done:
    k_mutex_unlock(&owner_limits_ps_lock);
    return result;
}

static int fresh_matches(const uint8_t board[32],uint64_t generation)
{
    struct owner_limits_fresh_record marker; struct psa_storage_info_t info; size_t length=0;
    return psa_ps_get_info(OWNER_LIMITS_FRESH_UID,&info)==PSA_SUCCESS && info.size==sizeof(marker) &&
           (info.flags & PSA_STORAGE_FLAG_WRITE_ONCE) &&
           psa_ps_get(OWNER_LIMITS_FRESH_UID,0,sizeof(marker),&marker,&length)==PSA_SUCCESS && length==sizeof(marker) &&
           marker.schema==OWNER_LIMITS_SCHEMA && marker.generation==generation && !memcmp(marker.board,board,32);
}
int device_owner_limits_mark_fresh_ps(const uint8_t board[32],uint64_t generation)
{
    if(!device_owner_limits_platform_qualified() || !board || !generation || k_mutex_lock(&owner_limits_ps_lock,K_FOREVER))return -1;
    int result=-1; struct psa_storage_info_t info;
    if(psa_ps_get_info(OWNER_LIMITS_PS_UID,&info)!=PSA_ERROR_DOES_NOT_EXIST ||
       psa_ps_get_info(OWNER_LIMITS_INITIALIZED_UID,&info)!=PSA_ERROR_DOES_NOT_EXIST)goto done_fresh;
    psa_status_t status=psa_ps_get_info(OWNER_LIMITS_FRESH_UID,&info);
    if(status==PSA_ERROR_DOES_NOT_EXIST) {
        struct owner_limits_fresh_record marker={.schema=OWNER_LIMITS_SCHEMA,.generation=generation};
        memcpy(marker.board,board,32);
        if(psa_ps_set(OWNER_LIMITS_FRESH_UID,sizeof(marker),&marker,PSA_STORAGE_FLAG_WRITE_ONCE)!=PSA_SUCCESS)goto done_fresh;
    } else if(status!=PSA_SUCCESS) goto done_fresh;
    if(fresh_matches(board,generation))result=0;
done_fresh:
    k_mutex_unlock(&owner_limits_ps_lock);return result;
}
bool device_owner_limits_is_fresh_ps(const uint8_t board[32],uint64_t generation)
{
    if(!device_owner_limits_platform_qualified() || !board || !generation || k_mutex_lock(&owner_limits_ps_lock,K_FOREVER))return false;
    struct psa_storage_info_t info;
    bool fresh=fresh_matches(board,generation) &&
      psa_ps_get_info(OWNER_LIMITS_PS_UID,&info)==PSA_ERROR_DOES_NOT_EXIST &&
      psa_ps_get_info(OWNER_LIMITS_INITIALIZED_UID,&info)==PSA_ERROR_DOES_NOT_EXIST;
    k_mutex_unlock(&owner_limits_ps_lock);return fresh;
}
/* One-way mode fence. A crash after this witness but before v2 provisioning
 * leaves both modes blocked; it must never be interpreted as a fresh key. */
int device_owner_limits_claim_fresh_finality_ps(const uint8_t board[32],uint64_t generation)
{
    if(!device_owner_limits_platform_qualified() || !board || !generation || k_mutex_lock(&owner_limits_ps_lock,K_FOREVER))return -1;
    int result=-1;struct psa_storage_info_t info;
    if(!fresh_matches(board,generation) || psa_ps_get_info(OWNER_LIMITS_PS_UID,&info)!=PSA_ERROR_DOES_NOT_EXIST)goto done_claim;
    psa_status_t status=psa_ps_get_info(OWNER_LIMITS_INITIALIZED_UID,&info);
    struct owner_limits_fresh_record witness={.schema=2,.generation=generation};memcpy(witness.board,board,32);
    if(status==PSA_ERROR_DOES_NOT_EXIST) {
      if(psa_ps_set(OWNER_LIMITS_INITIALIZED_UID,sizeof(witness),&witness,PSA_STORAGE_FLAG_WRITE_ONCE)==PSA_SUCCESS)result=0;
    } else if(status==PSA_SUCCESS && info.size==sizeof(witness) && (info.flags & PSA_STORAGE_FLAG_WRITE_ONCE)) {
      struct owner_limits_fresh_record prior;size_t n=0;
      if(psa_ps_get(OWNER_LIMITS_INITIALIZED_UID,0,sizeof(prior),&prior,&n)==PSA_SUCCESS && n==sizeof(prior) &&
         prior.schema==2 && prior.generation==generation && !memcmp(prior.board,board,32))result=0;
    }
done_claim:k_mutex_unlock(&owner_limits_ps_lock);return result;
}
static int initialize_baseline(const nu54_limit_change *change,uint64_t lower,struct owner_limits_ps_record *record)
{
    struct psa_storage_info_t info;
    if(!change || !record || change->initialize!=1 || !lower || !fresh_matches(change->board,change->generation) ||
       psa_ps_get_info(OWNER_LIMITS_PS_UID,&info)!=PSA_ERROR_DOES_NOT_EXIST ||
       psa_ps_get_info(OWNER_LIMITS_INITIALIZED_UID,&info)!=PSA_ERROR_DOES_NOT_EXIST)return -1;
    memset(record,0,sizeof(*record));record->schema=OWNER_LIMITS_SCHEMA;
    record->state.generation=change->generation;record->state.revision=1;record->state.chain=change->chain;
    record->state.per_transfer=change->per_transfer;record->state.daily=change->daily;
    record->state.day=lower/86400;record->state.last_lower=lower;
    memcpy(record->state.board,change->board,32);memcpy(record->state.asset,change->asset,20);
    memcpy(record->state.destination,change->destination,20);
    return 0;
}
int device_owner_limits_initialize_stage_ps(const nu54_limit_change *change,uint64_t lower,uint64_t upper,uint32_t prompt,
                                             nu54_local_approval *approval,nu54_limit_hash hash,
                                             int (*present)(void *,const uint8_t *,size_t),void *context)
{
    if(!device_owner_limits_platform_qualified() || k_mutex_lock(&owner_limits_ps_lock,K_FOREVER))return -1;
    struct owner_limits_ps_record baseline;
    int result=initialize_baseline(change,lower,&baseline) ? -1 :
      nu54_limit_change_stage(&baseline.state,change,lower,upper,prompt,approval,hash,present,context);
    k_mutex_unlock(&owner_limits_ps_lock);return result;
}
int device_owner_limits_initialize_confirm_ps(const nu54_limit_change *change,uint64_t lower,uint64_t upper,uint32_t prompt,
                                               nu54_local_button button,nu54_local_approval *approval,const uint8_t proof[64],
                                               nu54_limit_hash hash,nu54_limit_verify verify)
{
    if(!device_owner_limits_platform_qualified() || k_mutex_lock(&owner_limits_ps_lock,K_FOREVER))return -1;
    struct owner_limits_ps_record baseline,next;int result=-1;
    if(initialize_baseline(change,lower,&baseline) ||
       nu54_limit_change_confirm(&baseline.state,change,lower,upper,prompt,button,approval,proof,hash,verify,&next.state)) {
        if(approval)nu54_local_approval_cancel(approval);
        goto done_initialize;
    }
    struct owner_limits_fresh_record witness={.schema=OWNER_LIMITS_SCHEMA,.generation=change->generation};
    memcpy(witness.board,change->board,32);
    /* Write-once witness first: an interrupted initialization fails closed and
     * cannot silently make an old key appear unused after storage loss. */
    if(psa_ps_set(OWNER_LIMITS_INITIALIZED_UID,sizeof(witness),&witness,PSA_STORAGE_FLAG_WRITE_ONCE)!=PSA_SUCCESS)goto done_initialize;
    next.schema=OWNER_LIMITS_SCHEMA;memset(&next.state.change,0,sizeof(next.state.change));
    if(!write_record(&next))result=0;
done_initialize:
    k_mutex_unlock(&owner_limits_ps_lock);return result;
}
static int valid_owner(const struct owner_limits_ps_record *record,const uint8_t board[32],uint64_t generation)
{
    return record->state.generation==generation && record->state.revision && record->state.chain &&
           record->state.per_transfer && record->state.per_transfer<=record->state.daily &&
           record->state.day_spent<=record->state.daily && record->state.count<=NU54_LIMIT_RESERVATIONS &&
           !memcmp(record->state.board,board,32);
}

static bool read_owner_record(const uint8_t board[32], uint64_t generation, struct owner_limits_ps_record *record)
{
    if (!device_owner_limits_platform_qualified() || !board || !generation ||
        k_mutex_lock(&owner_limits_ps_lock, K_FOREVER)) return false;
    bool ready = !load_record(record) && valid_owner(record, board, generation);
    k_mutex_unlock(&owner_limits_ps_lock);
    return ready;
}

bool device_owner_limits_ready_ps(const uint8_t board[32], uint64_t generation)
{
    struct owner_limits_ps_record record;
    return read_owner_record(board, generation, &record);
}

int device_owner_limits_read_recipients_ps(const uint8_t board[32],uint64_t generation,
    nu54_owner_limits *out,nu54_limit_recipients *active,nu54_limit_recipients *pending)
{
    enum nu54_limit_pending_kind kind;
    return device_owner_limits_read_set_ps(board, generation, out, active, pending, &kind);
}

int device_owner_limits_read_set_ps(const uint8_t board[32], uint64_t generation,
    nu54_owner_limits *out, nu54_limit_recipients *active, nu54_limit_recipients *pending,
    enum nu54_limit_pending_kind *kind)
{
    struct owner_limits_ps_record record;
    if (!out || !active || !pending || !kind || !read_owner_record(board, generation, &record)) return -1;
    *out = record.state;
    *active = record.active;
    *pending = record.pending;
    *kind = !record.state.change.pending ? NU54_LIMIT_PENDING_NONE :
            (enum nu54_limit_pending_kind)(NU54_LIMIT_PENDING_LEGACY + record.recipient_change);
    return 0;
}

int device_owner_limits_read_ps(const uint8_t board[32],uint64_t generation,nu54_owner_limits *out)
{
    nu54_owner_limits state;
    nu54_limit_recipients active,pending;
    if(!out || device_owner_limits_read_recipients_ps(board,generation,&state,&active,&pending) ||
       active.count!=1 || (state.change.pending && pending.count!=1))return -1;
    memcpy(state.destination,active.addresses[0],20);
    if(state.change.pending)memcpy(state.change.destination,pending.addresses[0],20);
    *out=state;return 0;
}

int device_owner_limits_migrate_recipients_ps(const uint8_t board[32], uint64_t generation)
{
    if(!device_owner_limits_platform_qualified() || !board || !generation ||
       k_mutex_lock(&owner_limits_ps_lock,K_FOREVER))return -1;
    struct owner_limits_ps_record record;
    int result=-1;
    if(!load_record(&record) && valid_owner(&record,board,generation)) {
        if(record.schema==OWNER_LIMITS_RECIPIENT_SCHEMA) result=0;
        else {
            record.schema=OWNER_LIMITS_RECIPIENT_SCHEMA;
            result=write_record(&record);
        }
    }
    k_mutex_unlock(&owner_limits_ps_lock);return result;
}

int device_owner_limits_stage_recipients_ps(const nu54_limit_recipient_change *change,
    uint64_t lower,uint64_t upper,uint32_t prompt,nu54_local_approval *approval,nu54_limit_hash hash,
    int (*present)(void *,const uint8_t *,size_t),void *context)
{
    if(!device_owner_limits_platform_qualified() || k_mutex_lock(&owner_limits_ps_lock,K_FOREVER))return -1;
    struct owner_limits_ps_record record;
    nu54_local_intent intent;
    int result=-1;
    if(approval && !load_record(&record) &&
       !nu54_limit_recipient_intent(&record.state,&record.active,change,lower,upper,prompt,hash,&intent))
        result=nu54_local_approval_stage(approval,&intent,upper,present,context);
    k_mutex_unlock(&owner_limits_ps_lock);return result;
}

int device_owner_limits_confirm_recipients_ps(const nu54_limit_recipient_change *change,
    uint64_t lower,uint64_t upper,uint32_t prompt,nu54_local_button button,nu54_local_approval *approval,
    const uint8_t proof[64],nu54_limit_hash hash,nu54_limit_verify verify)
{
    if(!device_owner_limits_platform_qualified() || k_mutex_lock(&owner_limits_ps_lock,K_FOREVER))return -1;
    struct owner_limits_ps_record record;
    nu54_local_intent intent;
    uint8_t statement[NU54_LIMIT_SET_STATEMENT_MAX],approved[NU54_LOCAL_PREIMAGE_SIZE];
    size_t statement_length;
    int result=-1;
    if(!approval || !proof || !verify || load_record(&record) ||
       nu54_limit_recipient_intent(&record.state,&record.active,change,lower,upper,prompt,hash,&intent) ||
       nu54_limit_set_statement(&record.state, &record.active, change, statement, &statement_length) ||
       verify(statement, statement_length, proof)) {
        if(approval)nu54_local_approval_cancel(approval);
    } else if(!nu54_local_approval_consume(approval,&intent,upper,prompt,button,approved)) {
        record.schema=OWNER_LIMITS_RECIPIENT_SCHEMA;
        bool caps_only = change->per_transfer != 0;
        record.recipient_change = caps_only ? CAPS_REPLACE : RECIPIENTS_REPLACE;
        record.pending=change->recipients;
        record.state.change.pending=1;
        record.state.change.per_transfer = caps_only ? change->per_transfer : record.state.per_transfer;
        record.state.change.daily = caps_only ? change->daily : record.state.daily;
        record.state.change.not_before=change->not_before;
        memset(record.state.change.destination,0,20);
        memcpy(record.state.change.request,change->request,16);
        memcpy(record.state.change.statement_digest,intent.preimage+34,32);
        record.state.last_lower=lower;
        result=write_record(&record);
    }
    k_mutex_unlock(&owner_limits_ps_lock);return result;
}

int device_owner_limits_stage_ps(const nu54_limit_change *change,uint64_t lower,uint64_t upper,uint32_t prompt,
                                 nu54_local_approval *approval,nu54_limit_hash hash,
                                 int (*present)(void *,const uint8_t *,size_t),void *context)
{
    if(!device_owner_limits_platform_qualified() || k_mutex_lock(&owner_limits_ps_lock,K_FOREVER))return -1;
    struct owner_limits_ps_record record;
    int result=load_record(&record) || record.schema!=OWNER_LIMITS_SCHEMA ? -1 : nu54_limit_change_stage(&record.state,change,lower,upper,prompt,approval,hash,present,context);
    k_mutex_unlock(&owner_limits_ps_lock); return result;
}
int device_owner_limits_confirm_ps(const nu54_limit_change *change,uint64_t lower,uint64_t upper,uint32_t prompt,
                                   nu54_local_button button,nu54_local_approval *approval,const uint8_t proof[64],
                                   nu54_limit_hash hash,nu54_limit_verify verify)
{
    if(!device_owner_limits_platform_qualified() || k_mutex_lock(&owner_limits_ps_lock,K_FOREVER))return -1;
    struct owner_limits_ps_record current,next;
    int result=-1;
    if(!load_record(&current) && current.schema==OWNER_LIMITS_SCHEMA && !nu54_limit_change_confirm(&current.state,change,lower,upper,prompt,button,
                                                             approval,proof,hash,verify,&next.state)) {
        next.schema=OWNER_LIMITS_SCHEMA;
        if(!write_record(&next))result=0;
    } else if(approval) nu54_local_approval_cancel(approval);
    k_mutex_unlock(&owner_limits_ps_lock); return result;
}
static int activate_set_change(struct owner_limits_ps_record *record,uint64_t lower,uint64_t upper)
{
    nu54_owner_limits *state=&record->state;
    if(!state->generation || !state->revision || state->revision==UINT64_MAX || !lower || lower>upper ||
       lower/86400!=upper/86400 || lower/86400<state->day || lower<state->last_lower ||
       lower<state->change.not_before || !state->change.per_transfer ||
       state->change.per_transfer > state->change.daily || state->day_spent > state->change.daily ||
       !memcmp(state->change.request, (uint8_t[16]){0}, 16) ||
       !memcmp(state->change.statement_digest, (uint8_t[32]){0}, 32)) return -1;
    if (record->recipient_change == CAPS_REPLACE &&
        (record->pending.count != record->active.count ||
         memcmp(record->pending.addresses, record->active.addresses, sizeof(record->active.addresses)))) return -1;
    record->active=record->pending;
    memset(&record->pending,0,sizeof(record->pending));
    memset(state->destination,0,20);
    if(record->active.count==1)memcpy(state->destination,record->active.addresses[0],20);
    state->per_transfer = state->change.per_transfer;
    state->daily = state->change.daily;
    memset(&state->change,0,sizeof(state->change));
    record->recipient_change=0;
    state->revision++;
    state->last_lower=lower;
    /* Preserve the old day and full reservation set. Reserve carries unresolved
     * amounts into a new UTC day atomically with the next accepted request. */
    return write_record(record);
}

int device_owner_limits_activate_recipients_ps(const uint8_t board[32],uint64_t generation,
    uint64_t lower,uint64_t upper)
{
    if(!device_owner_limits_platform_qualified() || !board || !generation ||
       k_mutex_lock(&owner_limits_ps_lock,K_FOREVER))return -1;
    struct owner_limits_ps_record record;
    int result=-1;
    if(!load_record(&record) && record.recipient_change == RECIPIENTS_REPLACE && valid_owner(&record,board,generation))
        result=activate_set_change(&record,lower,upper);
    k_mutex_unlock(&owner_limits_ps_lock); return result;
}

static int activate_legacy_change(struct owner_limits_ps_record *record,uint64_t lower,uint64_t upper)
{
    nu54_owner_limits next;
    if(nu54_limit_change_activate(&record->state,lower,upper,&next))return -1;
    record->state=next;
    record->active=record->pending;
    memset(&record->pending,0,sizeof(record->pending));
    return write_record(record);
}

int device_owner_limits_activate_set_ps(const uint8_t board[32], uint64_t generation,
    uint64_t revision, const uint8_t request[16], uint64_t lower, uint64_t upper)
{
    if (!device_owner_limits_platform_qualified() || !board || !generation || !revision || !request ||
        k_mutex_lock(&owner_limits_ps_lock, K_FOREVER)) return -1;
    struct owner_limits_ps_record record;
    int result = -1;
    if (!load_record(&record) && valid_owner(&record, board, generation) && record.state.change.pending &&
        record.state.revision == revision && !memcmp(request, record.state.change.request, 16))
        result = record.recipient_change ? activate_set_change(&record, lower, upper) :
                                          activate_legacy_change(&record, lower, upper);
    k_mutex_unlock(&owner_limits_ps_lock);
    return result;
}

int device_owner_limits_activate_ps(uint64_t lower,uint64_t upper)
{
    if(!device_owner_limits_platform_qualified() || k_mutex_lock(&owner_limits_ps_lock,K_FOREVER))return -1;
    struct owner_limits_ps_record record;
    int result=-1;
    if(!load_record(&record) && !record.recipient_change)result=activate_legacy_change(&record,lower,upper);
    k_mutex_unlock(&owner_limits_ps_lock); return result;
}

int device_owner_limits_authorize_ps(const uint8_t board[32],uint64_t generation,uint64_t chain,
                                     const uint8_t asset[20],const uint8_t destination[20],const uint8_t amount[32],
                                     const uint8_t id[16],const uint8_t digest[32],uint64_t expires,
                                     uint64_t lower,uint64_t upper)
{
    if(!board || !asset || !destination || !amount || !id || !digest)return -1;
    for(size_t i=0;i<24;i++) if(amount[i])return -1;
    nu54_limit_request request={.generation=generation,.chain=chain,.expires=expires};
    for(size_t i=24;i<32;i++) request.amount=(request.amount<<8)|amount[i];
    memcpy(request.board,board,32); memcpy(request.asset,asset,20); memcpy(request.destination,destination,20);
    memcpy(request.id,id,16); memcpy(request.digest,digest,32);
    nu54_owner_limits state;
    return device_owner_limits_reserve_ps(&request,lower,upper,&state)==NU54_LIMIT_NEW ? 0 : -1;
}

int device_owner_limits_authorize_pilot_ps(const uint8_t board[32],uint64_t generation,
                                           const uint8_t quote[NU54_V2_UNSIGNED_SIZE],const uint8_t id[16],
                                           const uint8_t digest[32],uint64_t expires,uint64_t lower,uint64_t upper)
{
    if(!quote)return -1;
    const uint8_t zero[24]={0};
    if(memcmp(quote+NU54_V2_CHAIN_ID_OFFSET,zero,24) ||
       memcmp(quote+NU54_V2_ASSET_OFFSET,zero,12) ||
       memcmp(quote+NU54_V2_BENEFICIARY_OFFSET,zero,12))return -1;
    uint64_t chain=0;
    for(size_t i=24;i<32;i++)chain=(chain<<8)|quote[NU54_V2_CHAIN_ID_OFFSET+i];
    return device_owner_limits_authorize_ps(board,generation,chain,quote+NU54_V2_ASSET_OFFSET+12,
           quote+NU54_V2_BENEFICIARY_OFFSET+12,quote+NU54_V2_AMOUNT_OFFSET,id,digest,expires,lower,upper);
}
