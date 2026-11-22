#ifndef W27_QUALIFICATION_H
#define W27_QUALIFICATION_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <string.h>

#define W27Q_VERSION 2
#define W27Q_IMAGE_BYTES 64
#define W27Q_BOOT_BYTES 32
#define W27Q_REQUEST_BYTES (4 + W27Q_IMAGE_BYTES)
#define W27Q_BOOT_OFFSET 4
#define W27Q_RESPONSE_BYTES (W27Q_BOOT_OFFSET + W27Q_BOOT_BYTES)

enum {
    W27Q_OTP_LIFECYCLE = 1,
    W27Q_BOOTCONF_STATE = 2,
    W27Q_IMAGE_TAG = 4,
    W27Q_BOOT_ID = 8,
    W27Q_VERIFIED_MEASUREMENTS = 16,
    W27Q_CURRENT_DEBUG = 32,
    W27Q_ROLLBACK_STORAGE = 64,
    W27Q_SUPPORTED = W27Q_BOOTCONF_STATE | W27Q_IMAGE_TAG | W27Q_BOOT_ID,
    W27Q_REQUIRED = W27Q_SUPPORTED | W27Q_OTP_LIFECYCLE | W27Q_VERIFIED_MEASUREMENTS | W27Q_CURRENT_DEBUG | W27Q_ROLLBACK_STORAGE,
};

enum { W27Q_BOOTCONF_DEFAULT = 0, W27Q_BOOTCONF_NONDEFAULT = 1 };

_Static_assert(W27Q_RESPONSE_BYTES <= UINT8_MAX, "W27 V2 length is one byte");
_Static_assert((W27Q_SUPPORTED & W27Q_REQUIRED) != W27Q_REQUIRED, "W27 V2 must not qualify");

static inline void w27q_request(uint8_t *wire, const char *image)
{
    wire[0] = W27Q_VERSION;
    wire[1] = W27Q_REQUEST_BYTES;
    wire[2] = W27Q_SUPPORTED;
    wire[3] = 0;
    memcpy(wire + 4, image, W27Q_IMAGE_BYTES);
}

static inline bool w27q_request_valid(const uint8_t *wire, size_t len, const char *image)
{
    return len == W27Q_REQUEST_BYTES && wire[0] == W27Q_VERSION &&
           wire[1] == len && wire[2] == W27Q_SUPPORTED && wire[3] == 0 &&
           memcmp(wire + 4, image, W27Q_IMAGE_BYTES) == 0;
}

static inline void w27q_response(uint8_t *wire, const uint8_t *boot,
                                uint8_t bootconf_state)
{
    wire[0] = W27Q_VERSION;
    wire[1] = W27Q_RESPONSE_BYTES;
    wire[2] = W27Q_SUPPORTED;
    wire[3] = bootconf_state;
    memcpy(wire + W27Q_BOOT_OFFSET, boot, W27Q_BOOT_BYTES);
}

static inline bool w27q_identifier_present(const uint8_t *value, size_t len)
{
    uint8_t any = 0;
    uint8_t all = UINT8_MAX;
    for (size_t i = 0; i < len; ++i) {
        any |= value[i];
        all &= value[i];
    }
    return any != 0 && all != UINT8_MAX;
}

static inline bool w27q_response_valid(const uint8_t *wire, size_t len)
{
    return len == W27Q_RESPONSE_BYTES && wire[0] == W27Q_VERSION &&
           wire[1] == len && wire[2] == W27Q_SUPPORTED &&
           wire[3] <= W27Q_BOOTCONF_NONDEFAULT &&
           w27q_identifier_present(wire + W27Q_BOOT_OFFSET, W27Q_BOOT_BYTES);
}

static inline bool w27q_complete(const uint8_t *wire, size_t len)
{
    /* V2 never supplies the complete evidence set. */
    return w27q_response_valid(wire, len) && (wire[2] & W27Q_REQUIRED) == W27Q_REQUIRED;
}

#endif
