#ifndef DEVICE_VAULT_H
#define DEVICE_VAULT_H

#include "nu54_vault.h"

/* Load the owner and request records before advertising. Access from the protocol worker. */
int device_vault_load(void);
nu54_vault *device_vault_owner(void);

#endif
