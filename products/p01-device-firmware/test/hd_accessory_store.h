#ifndef HD_ACCESSORY_STORE_H
#define HD_ACCESSORY_STORE_H

#include "nu54_hd_slot.h"

int hd_host_read_exact(const char *path, void *value, size_t size);
int hd_host_open(nu54_hd_slot *slot);

#endif
