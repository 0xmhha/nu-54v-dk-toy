#include "nu54_running_firmware.h"

#include <assert.h>
#include <stdio.h>
#include <string.h>

struct medium {
    uint8_t bytes[80];
    bool unavailable;
};

static int read_image(const void *context, uint32_t offset, uint8_t *out, size_t size)
{
    const struct medium *image = context;
    if (image->unavailable || offset > sizeof(image->bytes) || size > sizeof(image->bytes) - offset) return -1;
    memcpy(out, image->bytes + offset, size);
    return 0;
}

static int read_file(const void *context, uint32_t offset, uint8_t *out, size_t size)
{
    FILE *file = *(FILE *const *)context;
    return fseek(file, offset, SEEK_SET) || fread(out, 1, size, file) != size ? -1 : 0;
}

static void inspect_counted_fixture(const char *name)
{
    FILE *file = fopen(name, "rb");
    assert(file && !fseek(file, 0, SEEK_END));
    long size = ftell(file);
    assert(size > 0 && (uint64_t)size <= UINT32_MAX);
    nu54_running_firmware actual;
    assert(!nu54_running_firmware_inspect(read_file, &file, (uint32_t)size, &actual));
    assert(actual.major == 2 && actual.minor == 3 && actual.revision == 258 &&
           actual.build == 65538 && actual.counter == 512);
    assert(actual.measured_size < (uint32_t)size);
    assert(!fclose(file));
}

int main(int argc, char **argv)
{
    assert(argc == 1 || argc == 2);
    if (argc == 2) inspect_counted_fixture(argv[1]);
    struct medium image = {.bytes = {
        [0] = 0x3d, [1] = 0xb8, [2] = 0xf3, [3] = 0x96,
        [8] = 32, [10] = 12, [12] = 8,
        [20] = 2, [21] = 3, [22] = 2, [23] = 1, [24] = 2, [26] = 1,
        [40] = 8, [41] = 0x69, [42] = 12, [44] = 0x50, [46] = 4, [49] = 2,
    }};
    nu54_running_firmware actual;
    assert(!nu54_running_firmware_inspect(read_image, &image, sizeof(image.bytes), &actual));
    assert(actual.major == 2 && actual.minor == 3 && actual.revision == 258 &&
           actual.build == 65538 && actual.counter == 512 && actual.measured_size == 52);
    uint8_t board[32], measurement[32], reply[NU54_RUNNING_FIRMWARE_REPLY_SIZE];
    memset(board, 0xab, sizeof(board));
    memset(measurement, 0xdc, sizeof(measurement));
    const uint8_t expected[] = {0x77,1,2,3,1,2,0,1,0,2,0,0,2,0,0,0,0,0,0,0,1,1};
    assert(!nu54_running_firmware_reply(&actual, 257, board, measurement, reply));
    assert(!memcmp(reply, expected, sizeof(expected)) && !memcmp(reply + 22, board, 32) &&
           !memcmp(reply + 54, measurement, 32));
    const uint8_t request[] = {0x77,0,0,0,0,0,0,1,1};
    assert(nu54_running_firmware_request(request, sizeof(request), 257));
    assert(!nu54_running_firmware_request(request, sizeof(request), 258));
    assert(!nu54_running_firmware_request(request, sizeof(request)-1, 257));
    assert(nu54_running_firmware_inspect(read_image, &image, 51, &actual));
    image.unavailable = true;
    assert(nu54_running_firmware_inspect(read_image, &image, sizeof(image.bytes), &actual));
    image.unavailable = false;
    image.bytes[49] = 0;
    assert(nu54_running_firmware_inspect(read_image, &image, sizeof(image.bytes), &actual));
    image.bytes[49] = 2;
    image.bytes[10] = 20;
    image.bytes[42] = 20;
    memcpy(image.bytes + 52, image.bytes + 44, 8);
    assert(nu54_running_firmware_inspect(read_image, &image, sizeof(image.bytes), &actual));
    image.bytes[55] = 0xff;
    assert(nu54_running_firmware_inspect(read_image, &image, sizeof(image.bytes), &actual));
    memset(measurement, 0, sizeof(measurement));
    assert(nu54_running_firmware_reply(&actual, 257, board, measurement, reply));
    return 0;
}
