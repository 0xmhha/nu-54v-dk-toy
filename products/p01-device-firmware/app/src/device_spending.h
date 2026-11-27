#ifndef DEVICE_SPENDING_H
#define DEVICE_SPENDING_H

#include "nu54_spending.h"

/* The spending wallet's own key. The install identity, its write-once witnesses and the trusted
 * floors stay through a reset: removing them would leave a board that refuses to boot its
 * commercial service, and they hold nothing of the user. All callers hold the pilot lock. */

/* After the vault and the install state are loaded. Finishes an interrupted reset first. */
int device_spending_init(void);
const uint8_t *device_spending_address(void); /* NULL until init succeeded */
int device_spending_sign(const uint8_t digest[32], uint8_t signature[64]);

/* Set once. Only a reset clears it, so a phone taken over later cannot move the money elsewhere. */
bool device_spending_return_address(uint8_t address[20]);
int device_spending_set_return(const uint8_t address[20]);

/* Destroys the key, the PIN, the owner and every record. Safe to repeat after a power cut. The
 * caller reboots afterwards; a new key is made on that boot. */
int device_spending_erase(void);

#endif
