/*
 * Payment link: BLE advertising and the GATT service of payment-protocol.md 3, framing of
 * section 4 and the session of sections 5 and 6 (core/nu54_session). All session work runs on
 * one work queue, so messages and button presses never race.
 */
#ifndef PAY_LINK_H
#define PAY_LINK_H

#include <stdint.h>

typedef enum {
	PAY_LED_MODE = 0,     /* LED1: payment mode (advertising or connected) */
	PAY_LED_WAITING = 1,  /* LED2: waiting for the button */
	PAY_LED_APPROVED = 2, /* LED3: payment approved */
	PAY_LED_REFUSED = 3,  /* LED4: refused or failed */
} pay_led_t;

/* Starts BLE and the session with the device address and key. Returns 0 or a negative errno. */
int pay_link_init(const uint8_t address[20]);

/* Enters payment mode: advertise the payment service for `seconds`. */
int pay_link_payment_mode(uint32_t seconds);

/* The renter's button while a payment waits: 1 approve, 0 reject. */
void pay_link_button(int approve);

#endif /* PAY_LINK_H */
