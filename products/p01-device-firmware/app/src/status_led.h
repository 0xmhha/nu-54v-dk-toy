/*
 * The four LEDs as the renter reads them (P01 design 8):
 *
 *   LED1 on        payment mode: advertising, or a kiosk connected
 *   LED2 on        waiting for the renter's button
 *   LED3 blinking  signed; waiting for the kiosk's result (30 s at most)
 *   LED3 on 5 s    payment approved (the kiosk saw it settled)
 *   LED4 on 5 s    refused, failed or cancelled: a device refusal, the kiosk's refused or failed
 *                  outcome, or the link lost while waiting
 *   all off        idle
 *
 * A new session clears the result LEDs. While the PIN is entered, status_led_pin() shows the kept
 * digits instead (LED k for digit k) until status_led_pin_end(). This module is the only one that
 * drives the LEDs.
 */
#ifndef STATUS_LED_H
#define STATUS_LED_H

#include <stdbool.h>

#include "nu54_session.h"

void status_led_mode(bool on);
void status_led_waiting(bool on);
/* What a session call meant: SIGNED starts the LED3 blink, REFUSED shows LED4. */
void status_led_event(nu54_event_t event);
/* The kiosk's payment.outcome, forwarded through the device. */
void status_led_outcome(bool approved);
/* A new session: forget the last result. */
void status_led_new_session(void);
/* Everything off (payment mode about to start again). */
void status_led_clear(void);
/* PIN entry (main.c): show `mask` (bit k-1 = digit k kept); end goes back to the status. */
void status_led_pin(uint8_t mask);
void status_led_pin_end(void);

#endif /* STATUS_LED_H */
