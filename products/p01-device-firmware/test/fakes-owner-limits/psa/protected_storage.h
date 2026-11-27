#ifndef TEST_OWNER_LIMITS_PSA_H
#define TEST_OWNER_LIMITS_PSA_H
#include <stddef.h>
#include <stdint.h>
typedef uint64_t psa_storage_uid_t;
typedef int psa_status_t;
#define PSA_SUCCESS 0
#define PSA_STORAGE_FLAG_NONE 0
#define PSA_STORAGE_FLAG_WRITE_ONCE 1
#define PSA_ERROR_DOES_NOT_EXIST -2
struct psa_storage_info_t {size_t size; uint32_t flags;};
psa_status_t psa_ps_get_info(psa_storage_uid_t,struct psa_storage_info_t *);
psa_status_t psa_ps_get(psa_storage_uid_t,size_t,size_t,void *,size_t *);
psa_status_t psa_ps_set(psa_storage_uid_t,size_t,const void *,uint32_t);
#endif
