#include "nu54_owner_finality_store.h"
int nu54_finality_commit(const nu54_finality_port *p,const nu54_finality_claim *claim,const uint8_t *raw,size_t n,const uint8_t signature[64],nu54_finality_verify verify,nu54_finality_hash hash,uint64_t lower,uint64_t upper) {
 if(!p||!p->lock||!p->unlock||!p->load||!p->write||!p->read_counter||!p->advance||p->lock(p->context))return -1;
 int result=-1;uint64_t counter;nu54_finality_record current,next;
 if(p->read_counter(p->context,&counter)||!counter||counter==UINT64_MAX||p->load(p->context,&current)||current.version!=counter)goto done;
 int decision=nu54_finality_apply(&current.state,claim,raw,n,signature,verify,hash,lower,upper,&next.state);
 if(decision==NU54_FINALITY_ALREADY){result=decision;goto done;}
 if(decision)goto done;
 next.version=counter+1;
 if(p->write(p->context,&next)||p->advance(p->context,counter))goto done;
 result=0;
done:p->unlock(p->context);return result;
}
