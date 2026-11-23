#ifndef NU54_SPENDING_FAKE_ITS_H
#define NU54_SPENDING_FAKE_ITS_H
#include "crypto.h"
typedef uint64_t psa_storage_uid_t;
struct psa_storage_info_t {size_t size;};
#define PSA_STORAGE_FLAG_NONE 0u
psa_status_t psa_its_get_info(psa_storage_uid_t, struct psa_storage_info_t *);
psa_status_t psa_its_get(psa_storage_uid_t, size_t, size_t, void *, size_t *);
psa_status_t psa_its_set(psa_storage_uid_t, size_t, const void *, uint32_t);
psa_status_t psa_its_remove(psa_storage_uid_t);
#endif
