/*
 * What the device keeps across resets and the session's platform callbacks (payment-protocol.md
 * 2, 5): the device key, the setup record, the PIN with its failure counter, and the nonce
 * counter. The record, PIN and counter live in PSA secure storage (Zephyr Secure storage now,
 * TF-M ITS later); the PIN is kept only as an HMAC under a device-held key, never in clear.
 */
#ifndef DEVICE_SETUP_H
#define DEVICE_SETUP_H

#include "nu54_session.h"

/* Fills the platform callbacks of `d` and loads what is stored: a finished rental setup
 * (PROVISIONED_NO_ANCHOR, or PIN_LOCKED), or nothing (UNPROVISIONED, after wiping whatever an
 * unfinished setup left). With CONFIG_NU54_DEV_SETUP a device without a rental setup uses the
 * week-7 fixed setup instead. Returns 0 or a negative error. */
int device_setup_load(nu54_device_t *d);

#endif /* DEVICE_SETUP_H */
