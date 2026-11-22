#ifndef DEVICE_INSTALL_H
#define DEVICE_INSTALL_H

#include "device_tls_v2.h"
#include "nu54_install.h"
#include "nu54_running_firmware.h"

bool device_install_required(void);
int device_install_init(void);
int device_install_bound_offer(const uint8_t exporter[32], uint8_t output[NU54_INSTALL_BOUND_OFFER_SIZE]);
int device_install_public_pins(uint8_t output[64]);
bool device_install_distinct_station(const mbedtls_x509_crt *);
int device_install_load(const uint8_t *, size_t, mbedtls_x509_crt *, mbedtls_x509_crt *,
			mbedtls_pk_context *, device_tls_v2_profile [2]);
const nu54_install_policy *device_install_policy(void);
const uint8_t *device_install_payer(void);
uint64_t device_install_now(void);
uint64_t device_install_earliest(void);
int device_install_fence_wallet_expiry(uint64_t expiry);
uint64_t device_install_wallet_replay_floor(void);
int device_install_fence_receipt_expiry(uint64_t expiry);
int device_install_public_authority(uint8_t [65]);
int device_install_verify_authority(const uint8_t *, size_t, const uint8_t [64]);
#if defined(CONFIG_NU54_EXPERIMENTAL_RENTAL_JOURNAL_PS)
#define NU54_RENTAL_INSTALL_OP 0x74
#define NU54_RENTAL_INSTALL_BODY_SIZE 375
#define NU54_RENTAL_INSTALL_WIRE_SIZE (NU54_RENTAL_INSTALL_BODY_SIZE + 64)
int device_install_rental_ack(const uint8_t [NU54_RENTAL_INSTALL_WIRE_SIZE],
			      uint8_t [NU54_RENTAL_INSTALL_WIRE_SIZE]);
#endif
int device_install_running_firmware(uint64_t generation, uint8_t [NU54_RUNNING_FIRMWARE_REPLY_SIZE]);
int device_install_rental_review(const nu54_local_approval *, const nu54_v2_link *,
    const nu54_v2_request *, const uint8_t [32], uint8_t [NU54_V2_RENTAL_REVIEW_RECEIPT_SIZE]);
int device_install_rental_decision(const nu54_v2_link *, uint64_t generation, const uint8_t review_hash[32],
    const uint8_t *payer_signature, uint8_t output[NU54_W10_DECISION_SIZE]);
int device_install_update_ack(const uint8_t [48], uint8_t [180]);
int device_install_check_hd(const uint8_t *, size_t);
void device_install_consume_offer(void);
int device_install_sign(const uint8_t [32], uint8_t [64]);
int device_install_independent_finality_key(const uint8_t [65]);
bool device_install_independent_peer(const uint8_t [32]);
int device_install_verify_recovery(const uint8_t *, size_t, const uint8_t [64]);
int device_install_verify_display(const uint8_t *, size_t, const uint8_t [64]);

#endif
