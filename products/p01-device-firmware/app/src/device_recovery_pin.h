#ifndef DEVICE_RECOVERY_PIN_H
#define DEVICE_RECOVERY_PIN_H

#include <stdbool.h>
#include <stdint.h>

/* Main thread only, after the user opens the board's local recovery window. */
int device_recovery_pin(bool virgin);
typedef bool (*device_pin_guard)(void *context, bool local_input, uint32_t input_prompt);
int device_recovery_pin_guarded(bool virgin, device_pin_guard guard, void *context);

#if defined(CONFIG_NU54_SPENDING_PROFILE)
/* Seconds until the PIN may be entered again; 0 when it may. Negative when unreadable. */
int32_t device_recovery_pin_wait(void);
int device_recovery_pin_erase(void);
#endif

#endif
