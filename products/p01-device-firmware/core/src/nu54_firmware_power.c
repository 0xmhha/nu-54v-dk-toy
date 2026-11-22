#include "nu54_firmware_power.h"

#include <string.h>

#define BQ25186_STAT0 0x00
#define BQ25186_VBAT_CTRL 0x03
#define BQ25186_VIN_GOOD 0x01
#define BQ25186_PG_IS_GPO 0x80

enum nu54_firmware_vin nu54_firmware_observe_vin(const nu54_firmware_power_source *source)
{
    uint8_t mode, before, after;
    if (!source || !source->qualified || !source->read_register || !source->read_pg_asserted ||
        source->read_register(source->context, BQ25186_VBAT_CTRL, &mode) || (mode & BQ25186_PG_IS_GPO) ||
        source->read_register(source->context, BQ25186_STAT0, &before))
        return NU54_FIRMWARE_VIN_UNKNOWN;
    int pg = source->read_pg_asserted(source->context);
    if ((pg != 0 && pg != 1) || source->read_register(source->context, BQ25186_STAT0, &after) ||
        source->read_register(source->context, BQ25186_VBAT_CTRL, &mode) || (mode & BQ25186_PG_IS_GPO) ||
        (before & BQ25186_VIN_GOOD) != (after & BQ25186_VIN_GOOD) ||
        (before & BQ25186_VIN_GOOD) != pg)
        return NU54_FIRMWARE_VIN_UNKNOWN;
    return pg ? NU54_FIRMWARE_VIN_GOOD : NU54_FIRMWARE_VIN_NOT_GOOD;
}


static void write_generation(uint8_t *out, uint64_t generation)
{
    for (size_t i = 8; i > 0;) {
        out[--i] = (uint8_t)generation;
        generation >>= 8;
    }
}

bool nu54_firmware_power_request(const uint8_t *body, size_t size, uint64_t current_generation)
{
    uint8_t expected[NU54_FIRMWARE_POWER_REQUEST_SIZE] = {NU54_FIRMWARE_POWER_OP};
    write_generation(expected + 1, current_generation);
    return body && current_generation && size == sizeof(expected) && !memcmp(body, expected, sizeof(expected));
}

int nu54_firmware_power_reply(enum nu54_firmware_vin vin, uint64_t generation, const uint8_t board[32],
                              uint8_t out[NU54_FIRMWARE_POWER_REPLY_SIZE])
{
    static const uint8_t empty[32];
    if ((unsigned)vin > NU54_FIRMWARE_VIN_GOOD || !generation || !out || !board ||
        !memcmp(board, empty, sizeof(empty))) return -1;
    out[0] = NU54_FIRMWARE_POWER_OP;
    out[1] = 1;
    out[2] = (uint8_t)vin;
    write_generation(out + 3, generation);
    memcpy(out + 11, board, 32);
    return 0;
}
