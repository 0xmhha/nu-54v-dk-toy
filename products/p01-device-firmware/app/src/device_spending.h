#ifndef DEVICE_SPENDING_H
#define DEVICE_SPENDING_H

#include "nu54_spending.h"

/* The spending wallet's own key. The install identity, its write-once witnesses and the trusted
 * floors stay through a reset: removing them would leave a board that refuses to boot its
 * commercial service, and they hold nothing of the user. All callers hold the pilot lock. */

/* A board without an owner holds no wallet key. The key is made when the first owner registers
 * and destroyed by a reset; it is never made at start. */

/* After the vault and the install state are loaded. Finishes an interrupted reset first. Without
 * an owner it removes any key it finds; with one, a missing key fails and is not replaced. */
int device_spending_init(void);
/* Makes the key for the first owner. Call it while the board has no owner, right before the vault
 * takes one. When the vault then does not take the owner, call withdraw. A power cut in between
 * leaves a key without an owner, which the next start removes. */
int device_spending_enroll(void);
void device_spending_withdraw(void);
const uint8_t *device_spending_address(void); /* NULL without a wallet: no owner yet, or init failed */
int device_spending_sign(const uint8_t digest[32], uint8_t signature[64]);

/* Set once. Only a reset clears it, so a phone taken over later cannot move the money elsewhere. */
bool device_spending_return_address(uint8_t address[20]);
int device_spending_set_return(const uint8_t address[20]);

/* Destroys the key, the PIN, the owner and every record, and succeeds only after the key store
 * confirmed that the key is gone. Safe to repeat after a power cut, and the next start repeats it
 * until it succeeds. The caller reboots afterwards; that boot makes no key. */
int device_spending_erase(void);

#endif
