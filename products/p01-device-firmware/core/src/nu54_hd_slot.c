#include "nu54_hd_slot.h"

#include <string.h>

_Static_assert(sizeof(nu54_hd_record) <= 128, "HD metadata does not fit one authenticated ITS record");

static int save(nu54_hd_slot *slot, const nu54_hd_record *record)
{
	if (slot->backend.save(slot->backend.context, record)) {
		slot->enabled = false;
		return -1;
	}
	slot->record = *record;
	return 0;
}

static int compare_key(nu54_hd_slot *slot, const nu54_hd_record *record)
{
	uint8_t key[NU54_HD_PUBLIC_SIZE], address[20];
	return slot->backend.derive(slot->backend.context, key, address) ||
	       memcmp(record->public_key, key, sizeof(key)) || memcmp(record->address, address, sizeof(address)) ? -1 : 0;
}

int nu54_hd_open(nu54_hd_slot *slot, const nu54_hd_backend *backend)
{
	nu54_hd_backend io = *backend;
	memset(slot, 0, sizeof(*slot));
	slot->backend = io;
	int state = io.load(io.context, &slot->record);
	int present = io.present(io.context);
	if (state == 1 && present == 1) {
		slot->enabled = true;
		return 0;
	}
	uint8_t reserved[3] = {0};
	if (state || (present != 0 && present != 1) || slot->record.version != NU54_HD_RECORD_VERSION ||
	    memcmp(slot->record.reserved, reserved, sizeof(reserved)) ||
	    slot->record.phase <= NU54_HD_EMPTY || slot->record.phase > NU54_HD_READY) return -1;
	if (slot->record.phase == NU54_HD_READY || slot->record.phase == NU54_HD_BACKUP_REQUIRED) {
		if (present || compare_key(slot, &slot->record)) return -1;
	}
	slot->enabled = true;
	return 0;
}

bool nu54_hd_backup_eligible(const nu54_hd_slot *slot, enum nu54_recovery_role recovery_role)
{
	return slot->enabled && recovery_role == NU54_RECOVERY_SOURCE &&
	       (slot->record.phase == NU54_HD_EMPTY || slot->record.phase == NU54_HD_CREATING ||
		slot->record.phase == NU54_HD_BACKUP_REQUIRED);
}

int nu54_hd_create(nu54_hd_slot *slot, const uint8_t identity[32])
{
	void *context = slot->backend.context;
	if (!slot->enabled || (slot->record.phase != NU54_HD_EMPTY && slot->record.phase != NU54_HD_CREATING))
		return -1;
	if (slot->record.phase == NU54_HD_EMPTY) {
		nu54_hd_record staged = {.version = NU54_HD_RECORD_VERSION, .phase = NU54_HD_CREATING};
		memcpy(staged.origin, identity, sizeof(staged.origin));
		if (slot->backend.present(context) != 1 || save(slot, &staged)) return -1;
	}
	if (memcmp(slot->record.origin, identity, sizeof(slot->record.origin))) return -1;
	int state = slot->backend.present(context);
	if ((state == 1 && slot->backend.generate(context)) || (state != 0 && state != 1)) {
		slot->enabled = false;
		return -1;
	}
	nu54_hd_record ready = slot->record;
	if (slot->backend.derive(context, ready.public_key, ready.address)) return -1;
	ready.phase = NU54_HD_BACKUP_REQUIRED;
	return save(slot, &ready);
}

int nu54_hd_restore(nu54_hd_slot *slot, const uint8_t *capsule, size_t capsule_length, const uint8_t *receipt,
		    size_t receipt_length, const nu54_recovery_context *expected, uint64_t now,
		    const nu54_recovery_signers *signers)
{
	if (!slot->enabled || (slot->record.phase != NU54_HD_EMPTY && slot->record.phase != NU54_HD_RESTORING &&
	    slot->record.phase != NU54_HD_READY) ||
	    !capsule || !capsule_length || nu54_recovery_verify(receipt, receipt_length, expected, now, signers)) return -1;
	nu54_hd_record wanted = {.version = NU54_HD_RECORD_VERSION, .phase = NU54_HD_RESTORING};
	memcpy(wanted.origin, receipt + 75, sizeof(wanted.origin));
	memcpy(wanted.public_key, receipt + 152, sizeof(wanted.public_key));
	memcpy(wanted.address, receipt + 217, sizeof(wanted.address));
	uint8_t key[NU54_HD_PUBLIC_SIZE], address[20];
	if (slot->backend.preview(slot->backend.context, capsule, capsule_length, key, address) ||
	    memcmp(wanted.public_key, key, sizeof(key)) || memcmp(wanted.address, address, sizeof(address))) return -1;
	if (slot->record.phase == NU54_HD_EMPTY) {
		if (slot->backend.present(slot->backend.context) != 1 || save(slot, &wanted)) return -1;
	} else {
		wanted.phase = slot->record.phase;
		if (memcmp(&slot->record, &wanted, sizeof(wanted))) return -1;
		if (wanted.phase == NU54_HD_READY) return compare_key(slot, &wanted);
	}
	int state = slot->backend.present(slot->backend.context);
	if ((state == 1 && slot->backend.import(slot->backend.context, capsule, capsule_length)) ||
	    (state != 0 && state != 1)) {
		slot->enabled = false;
		return -1;
	}
	if (compare_key(slot, &wanted)) {
		slot->enabled = false;
		return -1;
	}
	wanted.phase = NU54_HD_READY;
	return save(slot, &wanted);
}

int nu54_hd_mark_backup(nu54_hd_slot *slot, const uint8_t *receipt, size_t receipt_length,
			const nu54_recovery_context *expected, uint64_t now, const nu54_recovery_signers *signers)
{
	if (!slot->enabled || slot->record.phase != NU54_HD_BACKUP_REQUIRED ||
	    nu54_recovery_verify(receipt, receipt_length, expected, now, signers) ||
	    memcmp(slot->record.origin, receipt + 75, 32) ||
	    memcmp(slot->record.public_key, receipt + 152, NU54_HD_PUBLIC_SIZE) ||
	    memcmp(slot->record.address, receipt + 217, 20)) return -1;
	nu54_hd_record ready = slot->record;
	ready.phase = NU54_HD_READY;
	return save(slot, &ready);
}

int nu54_hd_address(const nu54_hd_slot *slot, uint8_t address[20])
{
	if (!slot->enabled || slot->record.phase != NU54_HD_READY) return -1;
	memcpy(address, slot->record.address, 20);
	return 0;
}
