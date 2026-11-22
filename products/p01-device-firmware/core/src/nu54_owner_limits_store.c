#include "nu54_owner_limits_store.h"
#include <stdint.h>

int nu54_limits_commit_reservation(const nu54_limits_port *p,const nu54_limit_request *request,
                                    uint64_t lower,uint64_t upper,nu54_owner_limits *out) {
 if (!p || !request || !out || !p->lock || !p->unlock || !p->load || !p->write ||
     !p->read_counter || !p->advance || p->lock(p->context)) return NU54_LIMIT_REJECT;
 int result=NU54_LIMIT_REJECT;
 nu54_limits_record current,updated;
 uint64_t counter;
 if (p->read_counter(p->context,&counter) || p->load(p->context,&current) || !counter ||
     current.version != counter || counter == UINT64_MAX) goto done;
 int decision=nu54_limits_reserve(&current.state,request,lower,upper,&updated.state);
 if (decision==NU54_LIMIT_RETRY) { *out=updated.state; result=decision; goto done; }
 if (decision!=NU54_LIMIT_NEW) goto done;
 updated.version=counter+1;
 /* A failed or interrupted write may have committed. Do not sign or retry until
  * the next locked load/counter comparison establishes a consistent state. */
 if (p->write(p->context,&updated) || p->advance(p->context,counter)) goto done;
 *out=updated.state; result=NU54_LIMIT_NEW;
done:
 p->unlock(p->context);
 return result;
}
