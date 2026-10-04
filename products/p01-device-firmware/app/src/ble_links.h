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
	void (*connected)(int link);
	void (*disconnected)(int link);
	/* One fragment written to RX. Returns 0, or nonzero when it could not be queued (the write
	 * is then refused with an ATT error). */
	int (*fragment)(int link, const uint8_t *data, uint16_t len);
} ble_links_handlers_t;

/* Starts Bluetooth and loads the bonds. `passkey` and `just_works` set how pairing mode pairs. */
int ble_links_init(const ble_links_handlers_t *h, uint32_t passkey, bool just_works);

/* Frames one body (envelope and fragments for the link's ATT_MTU) and notifies it. */
int ble_links_send(int link, const uint8_t *body, size_t len);

/* Link is bonded with LE Secure Connections: passkey-authenticated (need_passkey) or any LESC bond. */
bool ble_links_bonded(int link, bool need_passkey);

/* Link subscribed to TX notifications. */
bool ble_links_listening(int link);

/* Payment mode: advertise the payment service for `seconds`; 0 ends it. */
int ble_links_payment_mode(uint32_t seconds);

/* Pairing mode: accept a new bond for `seconds`, with the label passkey or Just Works. */
int ble_links_pairing_mode(uint32_t seconds, uint32_t passkey, bool just_works);

/* Remove every bond (device.reset). Safe before Bluetooth is up. */
int ble_links_unpair_all(void);

#endif /* BLE_LINKS_H */
