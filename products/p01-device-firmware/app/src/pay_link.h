/*
 * Payment link: BLE advertising and the GATT service of payment-protocol.md 3, framing of
 * section 4 and the session of sections 5 and 6 (core/nu54_session). All session work runs on
 * one work queue, so messages and button presses never race.
 */
#ifndef PAY_LINK_H
#define PAY_LINK_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>


/* Loads the stored setup (device_setup.h), then starts the session and BLE. Returns 0 or a
 * negative errno. */
int pay_link_init(void);

/* The device address; returns 1 when the device holds a key (not UNPROVISIONED). */
int pay_link_address(uint8_t address[20]);

/* Enters payment mode: advertise the payment service for `seconds`. */
int pay_link_payment_mode(uint32_t seconds);

/* Pairing mode (payment-protocol.md 3): accept a new bond for `seconds`, with the label passkey
 * (LE Secure Connections Passkey Entry), or Just Works while the device is UNPROVISIONED. */
int pay_link_pairing_mode(uint32_t seconds);

/* A commercial board without an owner advertises by itself; off once it has one
 * (ble_links_owner_wait). */
int pay_link_owner_wait(bool on);

/* The renter's button while a payment waits: 1 approve, 0 reject. */
void pay_link_button(uint32_t prompt, int approve);

/* 1 while the session waits for the PIN (setup key generation or a limit change). */
/* The session waits for the renter (a button or the PIN): the main loop polls so it notices
 * when the PIN is asked for next. */
int pay_link_waiting(void);

uint32_t pay_link_pin_prompt(void);

/* The PIN the renter entered on the buttons (NU54_PIN_LEN digits, core/nu54_pin_entry.h), or
 * NULL when the entry timed out. */
void pay_link_pin(uint32_t prompt, const char *pin, size_t len);

#endif /* PAY_LINK_H */
