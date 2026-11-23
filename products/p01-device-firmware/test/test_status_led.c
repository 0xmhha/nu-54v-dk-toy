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
    /* Each spending state must look different from the others. */
    status_led_waiting(true);
    advance(0);
    status_led_pressed();
    advance(500);
    assert(lamps[0]); /* a taken press is one steady light, the wait would have blinked */
    advance(101);
    status_led_waiting(false);
    advance(800);
    assert(!lamps[0] && !pending);
    unsigned edges = 0;
    status_led_locked();
    for (int i = 0; i < 1200; ++i) {
        bool before = lamps[0];
        advance(1);
        edges += lamps[0] && !before;
    }
    assert(edges == 5 && !lamps[0] && !pending);
    edges = 0;
    status_led_reset_armed(true);
    for (int i = 0; i < 1599; ++i) {
        bool before = lamps[0];
        advance(1);
        edges += lamps[0] && !before;
    }
    assert(edges == 10 && pending); /* keeps going until SW1 or the timeout */
    status_led_clear();
    advance(200);
    assert(!lamps[0] && !pending);
    edges = 0;
    status_led_reset_done();
    for (int i = 0; i < 3999; ++i) {
        bool before = lamps[0];
        advance(1);
        edges += lamps[0] && !before;
    }
    assert(edges == 2 && !lamps[0] && !pending); /* fits before the restart */
    return 0;
}
