/*
 * The BLE side of the payment link (payment-protocol.md 3, 4): the payment GATT service, up to
 * two centrals, pairing (LE Secure Connections with the label passkey, or Just Works while
 * UNPROVISIONED), payment and pairing modes (advertising), and fragmenting a body out to one
 * central. The connection table is this module's alone; callers name centrals by link index.
 *
 * The handlers run on Bluetooth threads and must only hand work over (pay_link.c queues them).
 * Every other function is called from the session work queue.
 */
#ifndef BLE_LINKS_H
#define BLE_LINKS_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#define BLE_LINKS_FRAGMENT_MAX 247

typedef struct {
	void (*connected)(int link, uint64_t epoch);
	void (*disconnected)(int link, uint64_t epoch);
	/* One fragment written to RX. Returns 0, or nonzero when it could not be queued (the write
	 * is then refused with an ATT error). */
	int (*fragment)(int link, uint64_t epoch, const uint8_t *data, uint16_t len);
} ble_links_handlers_t;

/* Starts Bluetooth and loads the bonds. `passkey` and `just_works` set how pairing mode pairs. */
int ble_links_init(const ble_links_handlers_t *h, uint32_t passkey, bool just_works);

/* Frames one body (envelope and fragments for the link's ATT_MTU) and notifies it. */
int ble_links_send(int link, const uint8_t *body, size_t len);
int ble_links_send_epoch(int link, uint64_t expected_epoch, const uint8_t *body, size_t len);
bool ble_links_connection_matches(int link, uint64_t expected_epoch);

/* Link is bonded with LE Secure Connections: passkey-authenticated (need_passkey) or any LESC bond. */
bool ble_links_bonded(int link, bool need_passkey);

/* Authenticated resolved identity plus the current non-reusable connection incarnation. */
#define BLE_RENTAL_PEER_SIZE 7
bool ble_links_rental_peer(int link, uint64_t expected_epoch, uint8_t peer[BLE_RENTAL_PEER_SIZE]);

/* Link subscribed to TX notifications. */
bool ble_links_listening(int link);

/* Payment mode: advertise the payment service for `seconds`; 0 ends it. The window opens also
 * when advertising cannot start at once: the start is tried again for about three seconds, and
 * again at the next connection event. Always returns 0. */
int ble_links_payment_mode(uint32_t seconds);

/* Whether this module has an advertising set on the air right now. A window can be open without
 * it: while a start is being tried again, or while every connection slot is taken. */
bool ble_links_advertising(void);

/* Pairing mode: accept a new bond for `seconds`, with the label passkey or Just Works. A failed
 * start of advertising is handled as in payment mode. */
int ble_links_pairing_mode(uint32_t seconds, uint32_t passkey, bool just_works);

/* Owner wait (CONFIG_NU54_OWNER_WAIT_ADV): a commercial board without an owner advertises the
 * owner service by itself, fast first and then slowly, while a connection slot is free and no
 * button mode is on. It also drops a central that neither installs nor opens the owner's TLS
 * session within CONFIG_NU54_OWNER_WAIT_IDLE_SECONDS, also after the schedule ended by itself.
 * `on` starts the schedule from its fast phase; off ends both. Without the option, turning it on
 * returns -ENOTSUP. */
int ble_links_owner_wait(bool on);

/* Remove every bond (device.reset). Safe before Bluetooth is up. */
int ble_links_unpair_all(void);

/* Bonds in the default identity, or -1 before Bluetooth is ready. */
int ble_links_bond_count(void);

#endif /* BLE_LINKS_H */
