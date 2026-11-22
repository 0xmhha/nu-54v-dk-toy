#include "device_recovery_pin.h"
#include "board_io.h"
#include "status_led.h"
#include <psa/internal_trusted_storage.h>
#include <assert.h>
#include <string.h>

static const uint32_t prompt = 27;
static uint32_t milliseconds;
static unsigned waits, clicks, writes, starts, ends;
static bool live, stale_input;

uint32_t k_uptime_get_32(void) {return milliseconds;}
int board_button_wait(board_button_event_t *out, int32_t timeout_ms)
{
    assert(timeout_ms == 100);
    milliseconds += 10;
    ++waits;
    if (waits == 2) live = false; /* BLE disconnect races with a queued click. */
    *out = (board_button_event_t){.button=0,.event=NU54_BTN_CLICK,.prompt=stale_input ? prompt-1 : prompt};
    return 0;
}
void status_led_pin(status_pin_t what) {if (what == STATUS_PIN_START) ++starts;}
void status_led_pin_end(void) {++ends;}
int psa_its_get_info(psa_storage_uid_t uid, struct psa_storage_info_t *info)
{(void)uid;info->size=33;return PSA_SUCCESS;}
int psa_its_get(psa_storage_uid_t uid, size_t offset, size_t length, void *data, size_t *used)
{(void)uid;assert(!offset && length==33);memset(data,0,length);*used=length;return PSA_SUCCESS;}
int psa_its_set(psa_storage_uid_t uid, size_t length, const void *data, uint32_t flags)
{(void)uid;(void)length;(void)data;(void)flags;++writes;return PSA_SUCCESS;}
int psa_generate_key(const psa_key_attributes_t *attrs, psa_key_id_t *id)
{(void)attrs;(void)id;assert(false);return -1;}
int psa_get_key_attributes(psa_key_id_t id, psa_key_attributes_t *attrs)
{(void)id;(void)attrs;assert(false);return -1;}
int psa_mac_compute(psa_key_id_t id,int alg,const uint8_t *data,size_t size,uint8_t *result,size_t output_size,size_t *used)
{(void)id;(void)alg;(void)data;(void)size;(void)result;(void)output_size;(void)used;assert(false);return -1;}
int psa_mac_verify(psa_key_id_t id,int alg,const uint8_t *data,size_t length,const uint8_t *digest,size_t size)
{(void)id;(void)alg;(void)data;(void)length;(void)digest;(void)size;assert(false);return -1;}

static bool guard(void *unused, bool physical_input, uint32_t input_prompt)
{
    (void)unused;
    if (!live || (physical_input && input_prompt != prompt)) return false;
    if (physical_input) ++clicks;
    return true;
}

int main(void)
{
    live=true;
    assert(device_recovery_pin_guarded(false,guard,NULL)==-1);
    assert(waits==2 && clicks==1 && !writes && starts==1 && ends==1);
    live=true;
    waits=0;
    stale_input=true;
    assert(device_recovery_pin_guarded(false,guard,NULL)==-1);
    assert(waits==1 && clicks==1 && !writes && starts==2 && ends==2);
    return 0;
}
