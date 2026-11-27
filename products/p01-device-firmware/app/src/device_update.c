#include "device_update.h"
#include "device_vault.h"

#include <zephyr/dfu/mcuboot.h>
#include <zephyr/kernel.h>
#include <zephyr/mgmt/mcumgr/grp/img_mgmt/img_mgmt.h>
#include <zephyr/storage/flash_map.h>
#include <zephyr/mgmt/mcumgr/mgmt/callbacks.h>
#include <zephyr/mgmt/mcumgr/grp/img_mgmt/img_mgmt_callbacks.h>
#include <zephyr/mgmt/mcumgr/mgmt/mgmt_defines.h>
#include <zephyr/sys/reboot.h>

#if defined(CONFIG_NU54_PILOT_TLS)
#if !defined(CONFIG_MCUMGR_TRANSPORT_UART) || defined(CONFIG_MCUMGR_TRANSPORT_BT) || \
    defined(CONFIG_MCUMGR_TRANSPORT_UDP) || defined(CONFIG_MCUMGR_TRANSPORT_LORAWAN) || \
    defined(CONFIG_MCUMGR_TRANSPORT_SHELL) || defined(CONFIG_MCUMGR_TRANSPORT_RAW_UART) || \
    defined(CONFIG_MCUMGR_TRANSPORT_DUMMY) || defined(CONFIG_MCUMGR_TRANSPORT_RAW_DUMMY)
#error "Commissioning fingerprints require UART-only SMP"
#endif
#include "device_pilot.h"
#include <zephyr/mgmt/mcumgr/mgmt/mgmt.h>
#include <zcbor_encode.h>

#define INSTALL_PIN_GROUP 0x5451
static int read_install_pins(struct smp_streamer *ctxt)
{
	uint8_t pins[64];
	if (device_pilot_install_pins(pins)) return MGMT_ERR_EACCESSDENIED;
	zcbor_state_t *writer = ctxt->writer->zs;
	struct zcbor_string bytes = {.value = pins, .len = sizeof(pins)};
	return zcbor_tstr_put_lit(writer, "pins") && zcbor_bstr_encode(writer, &bytes) ?
	       MGMT_ERR_EOK : MGMT_ERR_EMSGSIZE;
}

static const struct mgmt_handler install_pin_handlers[] = {{.mh_read = read_install_pins}};
static struct mgmt_group install_pin_group = {
	.mg_handlers = install_pin_handlers,
	.mg_handlers_count = ARRAY_SIZE(install_pin_handlers),
	.mg_group_id = INSTALL_PIN_GROUP,
};
#endif

K_MUTEX_DEFINE(staging_lock);
static bool staging, busy, completed;

static enum mgmt_cb_return gate(uint32_t event, enum mgmt_cb_return previous, int32_t *rc,
				uint16_t *group, bool *abort, void *data, size_t length)
{
	(void)group;
	if (previous != MGMT_CB_OK) return previous;
	if (data && length == sizeof(struct mgmt_evt_op_cmd_arg)) {
		const struct mgmt_evt_op_cmd_arg *command = data;
		k_mutex_lock(&staging_lock, K_FOREVER);
		if (event == MGMT_EVT_OP_CMD_DONE) {
			busy = false;
			k_mutex_unlock(&staging_lock);
			return MGMT_CB_OK;
		}
#if defined(CONFIG_NU54_PILOT_TLS)
		if (!staging && !busy && command->group == INSTALL_PIN_GROUP && command->op == MGMT_OP_READ &&
		    command->id == 0) {
			k_mutex_unlock(&staging_lock);
			return MGMT_CB_OK;
		}
#endif
		bool allowed = staging && !busy && command->group == MGMT_GROUP_ID_IMAGE &&
			       ((command->op == MGMT_OP_WRITE && command->id == IMG_MGMT_ID_UPLOAD) ||
				(command->op == MGMT_OP_READ && command->id == IMG_MGMT_ID_STATE));
		busy |= allowed;
		k_mutex_unlock(&staging_lock);
		if (allowed) return MGMT_CB_OK;
	}
	*rc = MGMT_ERR_EACCESSDENIED;
	*abort = true;
	return MGMT_CB_ERROR_RC;
}

static enum mgmt_cb_return slot(uint32_t event, enum mgmt_cb_return previous, int32_t *rc,
				uint16_t *group, bool *abort, void *data, size_t length)
{
	(void)group;
	if (previous != MGMT_CB_OK) return previous;
	if (event == MGMT_EVT_OP_IMG_MGMT_DFU_CHUNK) {
		const struct img_mgmt_upload_check *upload = data;
		if (!upload || length != sizeof(*upload) || !upload->action ||
		    upload->action->area_id != FIXED_PARTITION_ID(slot1_partition)) {
			*rc = MGMT_ERR_EACCESSDENIED;
			*abort = true;
			return MGMT_CB_ERROR_RC;
		}
	}
	k_mutex_lock(&staging_lock, K_FOREVER);
	if (event == MGMT_EVT_OP_IMG_MGMT_DFU_CHUNK) completed = false;
	if (event == MGMT_EVT_OP_IMG_MGMT_DFU_PENDING) completed = true;
	k_mutex_unlock(&staging_lock);
	return MGMT_CB_OK;
}

static struct mgmt_callback commands = {
	.callback = gate, .event_id = MGMT_EVT_OP_CMD_RECV | MGMT_EVT_OP_CMD_DONE,
};
static struct mgmt_callback image = {
	.callback = slot, .event_id = MGMT_EVT_OP_IMG_MGMT_DFU_CHUNK | MGMT_EVT_OP_IMG_MGMT_DFU_PENDING,
};

static int init(void)
{
#if defined(CONFIG_NU54_PILOT_TLS)
	mgmt_register_group(&install_pin_group);
#endif
	mgmt_callback_register(&commands);
	mgmt_callback_register(&image);
	return 0;
}

SYS_INIT(init, APPLICATION, 0);

void device_update_maintenance(void)
{
	k_mutex_lock(&staging_lock, K_FOREVER);
	staging = true;
	k_mutex_unlock(&staging_lock);
	int64_t end = k_uptime_get() + 120000;
	board_led_set(0, true);
	board_led_set(1, true);
	while (k_uptime_get() < end) {
		board_button_event_t button;
		if (board_button_wait(&button, 200) != 0 || button.event != NU54_BTN_LONG || button.button != 2)
			continue;
		k_mutex_lock(&staging_lock, K_FOREVER);
		bool commit = completed && !busy;
		if (commit) staging = false;
		k_mutex_unlock(&staging_lock);
		if (!commit) continue;
		if (boot_request_upgrade(BOOT_UPGRADE_TEST) == 0) sys_reboot(SYS_REBOOT_COLD);
		break;
	}
	k_mutex_lock(&staging_lock, K_FOREVER);
	staging = false;
	k_mutex_unlock(&staging_lock);
	board_led_set(0, false);
	board_led_set(1, false);
}

int device_update_boot_healthy(void)
{
	if (boot_is_img_confirmed()) return 0;
	if (!device_vault_owner()->enabled) return -1;
	return boot_write_img_confirmed();
}
