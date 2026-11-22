#include "nu54_running_firmware.h"

#include <string.h>

static uint32_t little(const uint8_t *bytes, size_t count)
{
    uint32_t value = 0;
    while (count) value = (value << 8) | bytes[--count];
    return value;
}

static void big(uint8_t *bytes, uint64_t value, size_t count)
{
    while (count) {
        bytes[--count] = (uint8_t)value;
        value >>= 8;
    }
}

int nu54_running_firmware_inspect(int (*read)(const void *, uint32_t, uint8_t *, size_t), const void *context,
                                  uint32_t slot_size, nu54_running_firmware *out)
{
    uint8_t header[32], tlv[4];
    if (!read || !out || slot_size < sizeof(header) || read(context, 0, header, sizeof(header)) ||
        little(header, 4) != UINT32_C(0x96f3b83d)) return -1;

    uint32_t header_size = little(header + 8, 2), payload_size = little(header + 12, 4);
    uint32_t protected_size = little(header + 10, 2);
    if (header_size < sizeof(header) || header_size > slot_size || !payload_size ||
        payload_size > slot_size - header_size || protected_size < 12 ||
        protected_size > slot_size - header_size - payload_size) return -1;

    uint32_t offset = header_size + payload_size, end = offset + protected_size, counter = 0;
    if (read(context, offset, tlv, sizeof(tlv)) || little(tlv, 2) != 0x6908 ||
        little(tlv + 2, 2) != protected_size) return -1;
    offset += (uint32_t)sizeof(tlv);
    while (offset < end) {
        if (end - offset < sizeof(tlv) || read(context, offset, tlv, sizeof(tlv))) return -1;
        uint32_t kind = little(tlv, 2), size = little(tlv + 2, 2);
        offset += (uint32_t)sizeof(tlv);
        if (!size || size > end - offset) return -1;
        if (kind == 0x50) {
            if (counter || size != sizeof(tlv) || read(context, offset, tlv, sizeof(tlv))) return -1;
            counter = little(tlv, sizeof(tlv));
            if (!counter) return -1;
        }
        offset += size;
    }
    if (!counter) return -1;
    *out = (nu54_running_firmware){
        .major = header[20], .minor = header[21], .revision = (uint16_t)little(header + 22, 2),
        .build = little(header + 24, 4), .counter = counter, .measured_size = end,
    };
    return 0;
}

bool nu54_running_firmware_request(const uint8_t *body, size_t size, uint64_t generation)
{
    uint8_t expected[NU54_RUNNING_FIRMWARE_REQUEST_SIZE] = {NU54_RUNNING_FIRMWARE_OP};
    big(expected + 1, generation, 8);
    return body && generation && size == sizeof(expected) && !memcmp(body, expected, sizeof(expected));
}

int nu54_running_firmware_reply(const nu54_running_firmware *version, uint64_t generation,
                                const uint8_t board[32], const uint8_t measurement[32],
                                uint8_t out[NU54_RUNNING_FIRMWARE_REPLY_SIZE])
{
    static const uint8_t empty[32];
    if (!version || !version->counter || !generation || !out || !board || !measurement ||
        !memcmp(board, empty, sizeof(empty)) || !memcmp(measurement, empty, sizeof(empty))) return -1;
    out[0] = NU54_RUNNING_FIRMWARE_OP;
    out[1] = 1;
    out[2] = version->major;
    out[3] = version->minor;
    big(out + 4, version->revision, 2);
    big(out + 6, version->build, 4);
    big(out + 10, version->counter, 4);
    big(out + 14, generation, 8);
    memcpy(out + 22, board, 32);
    memcpy(out + 54, measurement, 32);
    return 0;
}
