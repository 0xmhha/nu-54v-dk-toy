#ifndef DEVICE_HD_H
#define DEVICE_HD_H

#include "device_tls_v2.h"
#include "nu54_accessory.h"

#include <stdbool.h>
#include <stdint.h>

int device_hd_configure(mbedtls_x509_crt *, mbedtls_pk_context *, uint64_t (*clock)(void),
			const uint8_t *, size_t);
void device_hd_clear(void);
int device_hd_command(const nu54_v2_link *, const nu54_v2_request *, uint64_t now,
		      uint8_t [1 + NU54_RECOVERY_ENVELOPE_MAX], size_t *);
void device_hd_disconnected(void);
bool device_hd_pending(void);
bool device_hd_physical(uint32_t);
bool device_hd_market_address(uint8_t address[20]);
int device_hd_market_sign(const uint8_t address[20], const uint8_t digest[32], uint8_t signature[64]);
int device_hd_sign(const uint8_t address[20], const uint8_t digest[32], uint8_t signature[64]);
bool device_hd_public_address(uint8_t address[20]);
void device_hd_public_status(uint8_t status[77]);

#endif
