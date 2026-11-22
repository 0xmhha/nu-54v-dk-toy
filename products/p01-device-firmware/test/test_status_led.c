#include <assert.h>
#include <stdbool.h>
#include <stdint.h>
#include "zephyr/kernel.h"
#include "status_led.h"
#include "board_io.h"

static struct k_work_delayable *pending;
static int64_t uptime, next_tick;
static bool lamps[BOARD_IO_COUNT];

int64_t k_uptime_get(void) { return uptime; }
void k_work_reschedule(struct k_work_delayable *work, int delay)
{
    pending = work;
    next_tick = uptime + delay;
}

void board_led_set(uint8_t index, bool on) { lamps[index] = on; }

static void advance(int milliseconds)
{
    const int64_t end = uptime + milliseconds;
    while (pending && next_tick <= end) {
        struct k_work_delayable *task = pending;
        pending = NULL;
        uptime = next_tick;
        task->handler(NULL);
    }
    uptime = end;
}

int main(void)
{
    assert(status_led_locate(true));
    advance(0);
    assert(lamps[0]);
    assert(!status_led_locate(true)); /* A retried request cannot extend it. */
    advance(4200);
    assert(!lamps[0] && !pending);

    assert(status_led_locate(true));
    advance(0);
    assert(status_led_locate(false));
    advance(0);
    assert(!lamps[0] && !pending);

    assert(status_led_locate(true));
    status_led_waiting(true);
    assert(!status_led_locate(true));
    assert(status_led_locate(false)); /* Cancelling find preserves approval feedback. */
    advance(0);
    assert(lamps[0]);
    advance(200);
    assert(!lamps[0] && pending);

    status_led_waiting(false);
    status_led_pin(STATUS_PIN_START);
    assert(!status_led_locate(true));
    advance(0);
    assert(lamps[0]);
    status_led_pin_end();
    advance(801);
    assert(!lamps[0] && !pending);
    return 0;
}
