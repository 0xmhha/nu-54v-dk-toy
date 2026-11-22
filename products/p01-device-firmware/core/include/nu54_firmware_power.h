#ifndef NU54_FIRMWARE_POWER_H
#define NU54_FIRMWARE_POWER_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#define NU54_FIRMWARE_POWER_OP 0x78
#define NU54_FIRMWARE_POWER_REQUEST_SIZE 9
#define NU54_FIRMWARE_POWER_REPLY_SIZE 43

enum nu54_firmware_vin {
    NU54_FIRMWARE_VIN_UNKNOWN,
    NU54_FIRMWARE_VIN_NOT_GOOD,
    NU54_FIRMWARE_VIN_GOOD,
};

typedef struct {
    int (*read_register)(void *, uint8_t reg, uint8_t *value);
    int (*read_pg_asserted)(void *);
    void *context;
    bool qualified;
} nu54_firmware_power_source;

/* Observes charger VIN only. The result is never firmware application authority. */
enum nu54_firmware_vin nu54_firmware_observe_vin(const nu54_firmware_power_source *);
bool nu54_firmware_power_request(const uint8_t *body, size_t size, uint64_t current_generation);
int nu54_firmware_power_reply(enum nu54_firmware_vin, uint64_t generation, const uint8_t board[32],
                              uint8_t out[NU54_FIRMWARE_POWER_REPLY_SIZE]);

#endif
