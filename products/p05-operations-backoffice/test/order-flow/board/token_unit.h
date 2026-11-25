/* A board counts in 8 bytes. A token with more decimals than that has a unit: how many base units make one unit of
 * the board. What the board signs is in base units; its limits, its totals and its wire are in board units.
 * Used only by copies of the firmware that rename_token.py makes for such a token. */
#ifndef NU54_TOKEN_UNIT_H
#define NU54_TOKEN_UNIT_H

#include <stdbool.h>
#include <stdint.h>
#include <string.h>

#ifndef NU54_TOKEN_UNIT
#error "name the unit of the token"
#endif
/* Each step below stays within 64 bits for any unit up to 2^55. */
_Static_assert(NU54_TOKEN_UNIT > 0 && NU54_TOKEN_UNIT <= (UINT64_C(1) << 55), "unit out of range");

/* A signed amount in board units. False for one the board cannot count: not a whole number of units, or too many. */
static inline bool nu54_token_units(const uint8_t word[32], uint64_t *units)
{
	uint64_t rest = 0, count = 0;
	for (size_t i = 0; i < 32; ++i) {
		rest = (rest << 8) | word[i];
		uint64_t digit = rest / NU54_TOKEN_UNIT;
		rest %= NU54_TOKEN_UNIT;
		if (count >> 56) return false;
		count = (count << 8) | digit;
	}
	if (rest) return false;
	*units = count;
	return true;
}

/* Board units as the 32-byte amount that is signed. */
static inline void nu54_token_word(uint8_t word[32], uint64_t units)
{
	uint64_t carry = 0;
	memset(word, 0, 32);
	for (size_t i = 0; i < 32; ++i) {
		uint64_t digit = i < 8 ? (units >> (8 * i)) & 0xff : 0;
		carry += digit * NU54_TOKEN_UNIT;
		word[31 - i] = (uint8_t)carry;
		carry >>= 8;
	}
}

#endif
