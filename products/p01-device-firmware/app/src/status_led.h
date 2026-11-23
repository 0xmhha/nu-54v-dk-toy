/*
 * The four LEDs sit next to each other, so the renter cannot tell them apart: they work as one
 * lamp, and each step has its own blink pattern. Every indication ends dark; idle is dark.
 *
 *   idle                                  dark
 *   payment mode (waiting for a kiosk)    a short blip every 2 s; ends with the session
 *   pairing mode (a new bond, 60 s)       two short blips every 2 s
 *   bonded / pairing failed               on for 1 s / three short flashes
 *   waiting for the renter's button       fast blinking (0.2 s)
 *   signed (spending wallet: order or
 *   return), waiting for the kiosk        slow blinking (0.5 s), 30 s at most
 *   approved                              on for 2 s, then dark
 *   refused, failed or cancelled          three short flashes, then dark
 *   PIN entry: start / tap / keep digit   two long flashes / one short / one 0.5 s
 *   button accepted                       on for 0.6 s
 *   PIN wait not over yet                 five very short flashes
 *   reset armed, waiting for SW1          very fast blinking (0.08 s)
 *   reset done                            two long 1.5 s flashes
 *
 * One-off indications (results, PIN feedback) play over the ongoing one and then hand back.
 * This module is the only one that drives the LEDs.
 */
#ifndef STATUS_LED_H
#define STATUS_LED_H

#include <stdbool.h>

#include "nu54_session.h"

/* Payment mode (advertising for a kiosk). */
/* Bounded find pattern. Existing indications take precedence. */
bool status_led_locate(bool start);
void status_led_mode(bool on);
/* Pairing mode: a new bond is accepted (shown over payment mode). */
void status_led_pairing(bool on);
/* A pairing ended: bonded, or failed. */
void status_led_paired(bool ok);
/* The renter's button is awaited. */
void status_led_waiting(bool on);
/* What a session call meant: SIGNED starts the slow blinking, REFUSED plays the failure. */
void status_led_event(nu54_event_t event);
/* The kiosk's payment.outcome, forwarded through the device. */
void status_led_outcome(bool approved);
/* A new session: forget the last result. */
void status_led_new_session(void);
/* Back to idle (payment mode about to start again): dark at once. */
void status_led_clear(void);
/* A session is over: nothing ongoing any more; a result being shown finishes, then dark. */
void status_led_idle(void);

typedef enum { STATUS_PIN_START, STATUS_PIN_TAP, STATUS_PIN_KEEP } status_pin_t;
/* PIN entry feedback; while PIN entry runs the ongoing patterns pause. */
void status_led_pin(status_pin_t what);
void status_led_pin_end(void);

/* Spending wallet: a press was taken, the PIN must wait, a factory reset is armed or done.
 * The PIN starts only after the press was shown. A refusal does not replace the PIN wait. */
#define STATUS_LED_PRESSED_MS 600
void status_led_pressed(void);
void status_led_locked(void);
void status_led_reset_armed(bool on);
void status_led_reset_done(void);

#endif /* STATUS_LED_H */
