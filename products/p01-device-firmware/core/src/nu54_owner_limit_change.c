#include "nu54_owner_limit_change.h"
#include <string.h>
#include <stdint.h>
static int present_bytes(const uint8_t *v,size_t n) {uint8_t x=0;for(size_t i=0;i<n;i++)x|=v[i];return x!=0;}
static void put64(uint8_t **p,uint64_t n) {for(int i=7;i>=0;i--)*(*p)++=(uint8_t)(n>>(i*8));}
static void put(uint8_t **p,const void *v,size_t n) {memcpy(*p,v,n);*p+=n;}
int nu54_limit_change_statement(const nu54_owner_limits *s,const nu54_limit_change *c,uint8_t out[NU54_LIMIT_CHANGE_STATEMENT_SIZE]) {
 if(!s || !c || !out)return -1;
 uint8_t *p=out; put(&p,c->initialize ? "N54I" : "N54P",4); put(&p,c->board,32); put(&p,c->session,16); put(&p,c->request,16);
 put(&p,c->challenge,32); put64(&p,c->generation); put64(&p,c->revision); put64(&p,c->chain);
 uint8_t zeros[20]={0};
 put(&p,c->asset,20); put(&p,c->initialize ? zeros : s->destination,20); put(&p,c->destination,20);
 put64(&p,c->initialize ? 0 : s->per_transfer); put64(&p,c->initialize ? 0 : s->daily); put64(&p,c->per_transfer); put64(&p,c->daily);
 put64(&p,c->not_before); put64(&p,c->expires);
 return (size_t)(p-out)==NU54_LIMIT_CHANGE_STATEMENT_SIZE ? 0 : -1;
}
static int validate(const nu54_owner_limits *s,const nu54_limit_change *c,uint64_t lower,uint64_t upper) {
 return s && c && c->initialize<=1 && (!c->initialize || (s->revision==1 && s->per_transfer==c->per_transfer &&
   s->daily==c->daily && !memcmp(s->destination,c->destination,20))) && s->generation && s->revision && s->generation==c->generation && s->revision==c->revision &&
  s->chain && s->chain==c->chain && !s->change.pending && !s->count && lower && lower<=upper && lower>=s->last_lower &&
  (c->initialize ? c->not_before==0 : (c->expires<=UINT64_MAX-NU54_LIMIT_CHANGE_DELAY &&
   c->not_before>=c->expires+NU54_LIMIT_CHANGE_DELAY)) &&
  c->expires>upper && c->expires-upper<=600 && c->per_transfer && c->per_transfer<=c->daily &&
  present_bytes(s->board,32) && !memcmp(c->board,s->board,32) && present_bytes(c->session,16) && present_bytes(c->request,16) &&
  present_bytes(c->challenge,32) && present_bytes(c->asset,20) && present_bytes(c->destination,20) &&
  !memcmp(c->asset,s->asset,20) && present_bytes(s->destination,20) && s->per_transfer && s->per_transfer<=s->daily &&
  s->day_spent<=s->daily;
}
static int intent(const nu54_owner_limits *s,const nu54_limit_change *c,uint32_t prompt,nu54_limit_hash hash,nu54_local_intent *out) {
 uint8_t body[NU54_LIMIT_CHANGE_STATEMENT_SIZE];
 if(!hash || !out || nu54_limit_change_statement(s,c,body))return -1;
 memset(out,0,sizeof(*out)); out->generation=c->generation; out->expires=c->expires; out->physical_prompt=prompt;
 memcpy(out->session,c->session,16); memcpy(out->request,c->request,16); out->preimage[0]=0x19; out->preimage[1]=1;
 static const uint8_t domain[]="NU54-owner-policy-v1";
 return hash(domain,sizeof(domain)-1,out->preimage+2) || hash(body,sizeof(body),out->preimage+34) ? -1 : 0;
}
int nu54_limit_change_stage(const nu54_owner_limits *s,const nu54_limit_change *c,uint64_t lower,uint64_t upper,uint32_t prompt,
                            nu54_local_approval *a,nu54_limit_hash hash,int (*present)(void *,const uint8_t *,size_t),void *context) {
 nu54_local_intent i;
 if(!a || !validate(s,c,lower,upper) || intent(s,c,prompt,hash,&i))return -1;
 return nu54_local_approval_stage(a,&i,upper,present,context);
}
int nu54_limit_change_confirm(const nu54_owner_limits *s,const nu54_limit_change *c,uint64_t lower,uint64_t upper,uint32_t prompt,
                              nu54_local_button button,nu54_local_approval *a,const uint8_t proof[64],nu54_limit_hash hash,
                              nu54_limit_verify verify,nu54_owner_limits *next) {
 uint8_t body[NU54_LIMIT_CHANGE_STATEMENT_SIZE],signed_bytes[NU54_LOCAL_PREIMAGE_SIZE]; nu54_local_intent i;
 if(!a || !proof || !verify || !next || next==s || !validate(s,c,lower,upper) || intent(s,c,prompt,hash,&i) ||
    nu54_limit_change_statement(s,c,body) || verify(body,sizeof(body),proof)) {if(a)nu54_local_approval_cancel(a);return -1;}
 if(nu54_local_approval_consume(a,&i,upper,prompt,button,signed_bytes))return -1;
 *next=*s; next->change.pending=1; next->change.per_transfer=c->per_transfer; next->change.daily=c->daily;
 next->change.not_before=c->not_before; memcpy(next->change.destination,c->destination,20);
 memcpy(next->change.request,c->request,16); memcpy(next->change.statement_digest,i.preimage+34,32); next->last_lower=lower;
 return 0;
}
int nu54_limit_change_activate(const nu54_owner_limits *s,uint64_t lower,uint64_t upper,nu54_owner_limits *next) {
 if(!s || !next || s==next || !s->change.pending || s->count || !s->generation || !s->revision ||
    s->revision==UINT64_MAX || !s->chain || !present_bytes(s->board,32) || !present_bytes(s->asset,20) ||
    !present_bytes(s->destination,20) || !s->per_transfer || s->per_transfer>s->daily || s->day_spent>s->daily ||
    !lower || lower>upper || lower/86400!=upper/86400 || lower/86400<s->day || lower<s->last_lower || lower<s->change.not_before ||
    !s->change.per_transfer || s->change.per_transfer>s->change.daily || !present_bytes(s->change.destination,20) ||
    !present_bytes(s->change.request,16) || !present_bytes(s->change.statement_digest,32))return -1;
 if(lower/86400==s->day && s->day_spent>s->change.daily)return -1;
 *next=*s; if(lower/86400>s->day){next->day=lower/86400;next->day_spent=0;}
 next->per_transfer=s->change.per_transfer; next->daily=s->change.daily;
 memcpy(next->destination,s->change.destination,20); memset(&next->change,0,sizeof(next->change));
 next->revision++; next->last_lower=lower; return 0;
}

int nu54_limit_recipient_statement(const nu54_owner_limits *s,const nu54_limit_recipients *current,
    const nu54_limit_recipient_change *c,uint8_t out[NU54_LIMIT_RECIPIENT_STATEMENT_SIZE]) {
    if (!s || !c || !out || !nu54_limits_recipients_valid(current) ||
        !nu54_limits_recipients_valid(&c->recipients)) return -1;
    uint8_t *p=out;
    put(&p,"N54R",4); put(&p,c->board,32); put(&p,c->session,16); put(&p,c->request,16);
    put(&p,c->challenge,32); put64(&p,c->generation); put64(&p,c->revision); put64(&p,c->chain);
    put(&p,c->asset,20); put64(&p,s->per_transfer); put64(&p,s->daily);
    put64(&p,c->not_before); put64(&p,c->expires);
    *p++=current->count; put(&p,current->addresses,sizeof(current->addresses));
    *p++=c->recipients.count; put(&p,c->recipients.addresses,sizeof(c->recipients.addresses));
    return (size_t)(p-out)==NU54_LIMIT_RECIPIENT_STATEMENT_SIZE ? 0 : -1;
}

int nu54_limit_set_statement(const nu54_owner_limits *state, const nu54_limit_recipients *active,
    const nu54_limit_recipient_change *change, uint8_t output[NU54_LIMIT_SET_STATEMENT_MAX], size_t *length)
{
    if (!length || nu54_limit_recipient_statement(state, active, change, output)) return -1;
    *length = NU54_LIMIT_RECIPIENT_STATEMENT_SIZE;
    if (change->per_transfer || change->daily) {
        memcpy(output, "N54M", 4);
        uint8_t *tail = output + *length;
        put64(&tail, change->per_transfer);
        put64(&tail, change->daily);
        *length = (size_t)(tail - output);
    }
    return 0;
}

int nu54_limit_recipient_intent(const nu54_owner_limits *s,const nu54_limit_recipients *current,
    const nu54_limit_recipient_change *c,uint64_t lower,uint64_t upper,uint32_t prompt,
    nu54_limit_hash hash,nu54_local_intent *out) {
    uint8_t body[NU54_LIMIT_SET_STATEMENT_MAX];
    size_t length;
    if (!s || !c || !hash || !out || !s->generation || !s->revision || s->revision==UINT64_MAX ||
        s->generation!=c->generation || s->revision!=c->revision || !s->chain || s->chain!=c->chain ||
        s->change.pending || !lower || lower>upper || lower<s->last_lower ||
        !s->per_transfer || s->per_transfer>s->daily || s->day_spent>s->daily ||
        c->expires<=upper || c->expires-upper>600 || c->expires>UINT64_MAX-NU54_LIMIT_CHANGE_DELAY ||
        c->not_before<c->expires+NU54_LIMIT_CHANGE_DELAY ||
        !present_bytes(c->board,32) || memcmp(s->board,c->board,32) ||
        !present_bytes(c->asset,20) || memcmp(s->asset,c->asset,20) ||
        !present_bytes(c->session,16) || !present_bytes(c->request,16) || !present_bytes(c->challenge,32) ||
        nu54_limit_set_statement(s, current, c, body, &length)) return -1;
    bool same_set = current->count == c->recipients.count &&
                    !memcmp(current->addresses, c->recipients.addresses, sizeof(current->addresses));
    bool caps_only = c->per_transfer || c->daily;
    if (caps_only ? (!same_set || !c->per_transfer || c->per_transfer > c->daily ||
                     (s->per_transfer == c->per_transfer && s->daily == c->daily)) : same_set) return -1;
    memset(out,0,sizeof(*out));
    out->generation=c->generation; out->expires=c->expires; out->physical_prompt=prompt;
    memcpy(out->session,c->session,16); memcpy(out->request,c->request,16);
    out->preimage[0]=0x19; out->preimage[1]=1;
    const char *domain = caps_only ? "NU54-owner-caps-v2" : "NU54-owner-recipients-v2";
    return hash((const uint8_t *)domain, strlen(domain), out->preimage + 2) ||
           hash(body, length, out->preimage + 34) ? -1 : 0;
}
