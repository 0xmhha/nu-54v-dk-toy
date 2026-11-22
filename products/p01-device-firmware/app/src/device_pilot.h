#ifndef DEVICE_PILOT_H
#define DEVICE_PILOT_H

#include <stdbool.h>
#include <stdint.h>

#define NU54_PILOT_SERVICE_UUID_PARTS 0x41ba05a0, 0xe0dd, 0x4fd7, 0x901c, 0x4c25d5010000
#define NU54_PILOT_PROMPT_MASK UINT32_C(0x80000000)

void device_pilot_init(void);
void device_pilot_enrollment_window(void);
int device_pilot_install_pins(uint8_t output[64]);
bool device_pilot_recovery_window(void);
bool device_pilot_active(void);
void device_pilot_button(uint32_t prompt, bool approved);
bool device_pilot_waiting(void);

#endif
