#ifndef DEVICE_RECOVERY_PIN_H
#define DEVICE_RECOVERY_PIN_H

#include <stdbool.h>
#include <stdint.h>

/* Main thread only, after the user opens the board's local recovery window. */
int device_recovery_pin(bool virgin);
typedef bool (*device_pin_guard)(void *context, bool local_input, uint32_t input_prompt);
int device_recovery_pin_guarded(bool virgin, device_pin_guard guard, void *context);

#endif
