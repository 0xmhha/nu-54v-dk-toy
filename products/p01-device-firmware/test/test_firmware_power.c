#include "nu54_firmware_power.h"

#include <assert.h>
#include <string.h>

struct input {
    uint8_t mode[2], vin[2];
    int gpio;
    unsigned mode_reads, vin_reads, gpio_reads, calls, fail;
};

static int read_reg(void *context, uint8_t reg, uint8_t *value)
{
    struct input *in = context;
    if (++in->calls == in->fail) return -1;
    switch (reg) {
    case 0x03:
        assert(in->mode_reads < 2);
        *value = in->mode[in->mode_reads++];
        return 0;
    case 0x00:
        assert(in->vin_reads < 2);
        *value = in->vin[in->vin_reads++];
        return 0;
    default:
        assert(0 && "only the non-destructive register allowlist may be read");
        return -1;
    }
}

static int get_pg(void *context)
{
    struct input *in = context;
    in->gpio_reads++;
    if (++in->calls == in->fail) return -1;
    return in->gpio;
}

static enum nu54_firmware_vin observe(struct input *in, bool qualified)
{
    const nu54_firmware_power_source source = {
        .read_register = read_reg, .read_pg_asserted = get_pg, .context = in, .qualified = qualified,
    };
    return nu54_firmware_observe_vin(&source);
}

int main(void)
{
    const struct input good = {.mode = {0x46, 0x46}, .vin = {1, 1}, .gpio = 1};
    struct input in = good;
    assert(observe(&in, false) == NU54_FIRMWARE_VIN_UNKNOWN && !in.calls);
    assert(observe(&in, true) == NU54_FIRMWARE_VIN_GOOD);
    assert(in.mode_reads == 2 && in.vin_reads == 2 && in.gpio_reads == 1);
    in = (struct input){.mode = {0x46, 0x46}};
    assert(observe(&in, true) == NU54_FIRMWARE_VIN_NOT_GOOD);
    in = good; in.gpio = 0; /* SB2 is open or the reported PG signals disagree. */
    assert(observe(&in, true) == NU54_FIRMWARE_VIN_UNKNOWN);
    in = good; in.mode[0] = 0xc6;
    assert(observe(&in, true) == NU54_FIRMWARE_VIN_UNKNOWN && !in.gpio_reads);
    in = good; in.mode[1] = 0xc6;
    assert(observe(&in, true) == NU54_FIRMWARE_VIN_UNKNOWN);
    in = good; in.vin[1] = 0;
    assert(observe(&in, true) == NU54_FIRMWARE_VIN_UNKNOWN);
    in = good; in.gpio = 2;
    assert(observe(&in, true) == NU54_FIRMWARE_VIN_UNKNOWN);
    for (unsigned step = 1; step <= 5; ++step) {
        in = good; in.fail = step;
        assert(observe(&in, true) == NU54_FIRMWARE_VIN_UNKNOWN && in.calls == step);
    }
    const uint8_t request[] = {0x78,0,0,0,0,0,0,1,1};
    const uint8_t expected[] = {0x78,1,2,0,0,0,0,0,0,1,1};
    uint8_t board[32], reply[NU54_FIRMWARE_POWER_REPLY_SIZE];
    memset(board, 0xab, sizeof(board));
    assert(nu54_firmware_power_request(request, sizeof(request), 257));
    assert(!nu54_firmware_power_request(request, sizeof(request)-1, 257));
    assert(!nu54_firmware_power_request(request, sizeof(request), 258));
    assert(!nu54_firmware_power_reply(NU54_FIRMWARE_VIN_GOOD, 257, board, reply));
    assert(!memcmp(expected, reply, sizeof(expected)) && !memcmp(board, reply + 11, sizeof(board)));
    assert(!nu54_firmware_power_reply(NU54_FIRMWARE_VIN_UNKNOWN, 257, board, reply) && reply[2] == 0);
    assert(nu54_firmware_power_reply((enum nu54_firmware_vin)3, 257, board, reply));
    return 0;
}
