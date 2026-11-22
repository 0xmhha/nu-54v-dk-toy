#ifndef NU54_OWNER_H
#define NU54_OWNER_H

#include "nu54_vault.h"
#include "nu54_transport_v2.h"

#define NU54_OWNER_CHALLENGE_SIZE 130
#define NU54_OWNER_STATUS_SIZE 77
#define NU54_OWNER_REGISTER 1
#define NU54_OWNER_REPLACE 2
#define NU54_OWNER_REVOKE 3
#define NU54_OWNER_RECOVER 4
#define NU54_OWNER_REPLACE_LOST 5

typedef struct {
	int (*random)(uint8_t *, size_t);
	int (*hash)(const uint8_t *, size_t, uint8_t[32]);
	int (*verify)(const uint8_t *, size_t, const uint8_t[32], const uint8_t *, size_t);
} nu54_owner_crypto;

typedef struct {
	uint8_t record[NU54_OWNER_CHALLENGE_SIZE], epoch[16], peer[32];
	uint64_t generation, expiry;
	uint32_t prompt;
	bool issued, proven, independent;
} nu54_owner_enrollment;

int nu54_owner_status(const nu54_vault *, const nu54_owner_crypto *, const nu54_owner_enrollment *,
		      uint8_t [NU54_OWNER_STATUS_SIZE]);
int nu54_owner_begin(nu54_owner_enrollment *, const nu54_vault *, const nu54_owner_crypto *,
		     const nu54_v2_link *, const nu54_v2_request *, uint64_t now, const uint8_t enrollment_peer[32]);
int nu54_owner_begin_lost(nu54_owner_enrollment *, const nu54_vault *, const nu54_owner_crypto *,
			  const nu54_v2_link *, const nu54_v2_request *, uint64_t now);
int nu54_owner_prove(nu54_owner_enrollment *, const nu54_vault *, const nu54_owner_crypto *,
		     const nu54_v2_link *, const nu54_v2_request *, uint64_t now, uint32_t prompt);
int nu54_owner_authorize_lost(nu54_owner_enrollment *, const nu54_vault *, const nu54_v2_link *, uint64_t,
			      int (*verify)(const uint8_t *, size_t, const uint8_t [64]), const uint8_t [64]);
int nu54_owner_confirm(nu54_owner_enrollment *, nu54_vault *, const nu54_v2_link *, uint64_t now,
		       uint32_t prompt, bool approved);
void nu54_owner_cancel(nu54_owner_enrollment *);

#endif
