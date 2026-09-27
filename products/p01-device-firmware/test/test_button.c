/* Host tests for nu54_button (click and long-press detection). */
#include <assert.h>
#include <stdio.h>

#include "nu54_button.h"

static void test_click(void)
{
	nu54_button_t b;
	nu54_button_reset(&b);
	assert(nu54_button_update(&b, true, 100) == NU54_BTN_NONE);
	assert(nu54_button_tick(&b, 500) == NU54_BTN_NONE);
	assert(nu54_button_update(&b, false, 400) == NU54_BTN_CLICK);
}

static void test_long_fires_once_while_held(void)
{
	nu54_button_t b;
	nu54_button_reset(&b);
	nu54_button_update(&b, true, 0);
	assert(nu54_button_tick(&b, NU54_BUTTON_LONG_MS - 1) == NU54_BTN_NONE);
	assert(nu54_button_tick(&b, NU54_BUTTON_LONG_MS) == NU54_BTN_LONG);
	assert(nu54_button_tick(&b, NU54_BUTTON_LONG_MS + 500) == NU54_BTN_NONE);
	assert(nu54_button_update(&b, false, 3000) == NU54_BTN_NONE); /* no click after long */
}

static void test_long_on_release_without_tick(void)
{
	/* If ticks were missed, the release still reports the long press. */
	nu54_button_t b;
	nu54_button_reset(&b);
	nu54_button_update(&b, true, 0);
	assert(nu54_button_update(&b, false, 2000) == NU54_BTN_LONG);
}

static void test_repeated_level_is_ignored(void)
{
	nu54_button_t b;
	nu54_button_reset(&b);
	assert(nu54_button_update(&b, false, 10) == NU54_BTN_NONE);
	nu54_button_update(&b, true, 20);
	assert(nu54_button_update(&b, true, 30) == NU54_BTN_NONE);
	assert(nu54_button_update(&b, false, 60) == NU54_BTN_CLICK);
}

static void test_counter_wrap(void)
{
	nu54_button_t b;
	nu54_button_reset(&b);
	nu54_button_update(&b, true, UINT32_MAX - 100);
	assert(nu54_button_update(&b, false, 200) == NU54_BTN_CLICK); /* 301 ms across the wrap */
}

int main(void)
{
	test_click();
	test_long_fires_once_while_held();
	test_long_on_release_without_tick();
	test_repeated_level_is_ignored();
	test_counter_wrap();
	puts("nu54_button: all tests passed");
	return 0;
}
