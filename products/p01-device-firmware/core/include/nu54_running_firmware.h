#ifndef NU54_RUNNING_FIRMWARE_H
#define NU54_RUNNING_FIRMWARE_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#define NU54_RUNNING_FIRMWARE_OP 0x77
#define NU54_RUNNING_FIRMWARE_REQUEST_SIZE 9
#define NU54_RUNNING_FIRMWARE_REPLY_SIZE 86

typedef struct {
    uint8_t major, minor;
    uint16_t revision;
    uint32_t build, counter, measured_size;
} nu54_running_firmware;

/* Read the version and unique security counter from the bytes covered by the image measurement. */
int nu54_running_firmware_inspect(int (*read)(const void *, uint32_t, uint8_t *, size_t), const void *context,
                                  uint32_t slot_size, nu54_running_firmware *out);
bool nu54_running_firmware_request(const uint8_t *body, size_t size, uint64_t generation);
int nu54_running_firmware_reply(const nu54_running_firmware *version, uint64_t generation,
                                const uint8_t board[32], const uint8_t measurement[32],
                                uint8_t out[NU54_RUNNING_FIRMWARE_REPLY_SIZE]);

#endif
