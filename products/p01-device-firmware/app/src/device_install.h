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
#if defined(CONFIG_NU54_SPENDING_PROFILE)
/* The spending profile keeps the accepted grant in protected storage and uses it again after a
 * restart, without the operator. Call restore once at start, after an interrupted reset was
 * finished: 0 with the grant in use, 1 with none. `bundle` is working space for the grant. The
 * stored grant must be the one the floor was committed for and must fit the owner's generation.
 *
 * The board has no clock that runs without power. It saves its time every
 * CONFIG_NU54_CLOCK_SAVE_SECONDS and at the grant's end (keep_time; returns the milliseconds to the
 * next call, or -1 when there is nothing to keep) and continues from the saved time, so its clock
 * is late by the time it was off and by what passed since the last save. Without the phone it is
 * never early. */
int device_install_restore(uint8_t *bundle, size_t capacity, mbedtls_x509_crt *, mbedtls_x509_crt *,
			   mbedtls_pk_context *, device_tls_v2_profile [2]);
int64_t device_install_keep_time(void);
/* The registered owner's phone tells the time; the caller has checked who asks. Forward only,
 * before the grant's end, saved before it is used. Returns NU54_CLOCK_MOVED, _NOT_FORWARD or
 * _PAST_END with the reply written, or -1. */
int device_install_set_time(uint64_t time, uint64_t expires, uint64_t session_seconds,
			    uint8_t reply[NU54_CLOCK_SET_REPLY_SIZE]);
#endif
/* A factory reset: removes the stored grant and the saved time and ends the grant in use. The
 * floors stay. Safe to repeat after a power cut. */
int device_install_forget(void);
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
#if defined(CONFIG_NU54_SPENDING_PROFILE)
/* Signs only the fixed-size account statement with the board identity key. */
int device_install_account_statement(const uint8_t *statement, size_t length, uint8_t [64]);
#endif
int device_install_independent_finality_key(const uint8_t [65]);
bool device_install_independent_peer(const uint8_t [32]);
int device_install_verify_recovery(const uint8_t *, size_t, const uint8_t [64]);
int device_install_verify_display(const uint8_t *, size_t, const uint8_t [64]);

#endif
