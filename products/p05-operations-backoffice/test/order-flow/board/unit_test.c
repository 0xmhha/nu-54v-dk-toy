/* The conversion between what a board counts in and what it signs. */
#include "token_unit.h"

#include <stdio.h>

static int failed;
static void expect(const char *what, bool value)
{
	if (value) return;
	fprintf(stderr, "failed: %s\n", what);
	failed = 1;
}

static void word_of(const char *hex, uint8_t word[32])
{
	for (size_t i = 0; i < 32; ++i) {
		unsigned byte = 0;
		sscanf(hex + 2 * i, "%2x", &byte);
		word[i] = (uint8_t)byte;
	}
}

static bool reads(const char *hex, uint64_t wanted)
{
	uint8_t word[32];
	uint64_t units = 12345;
	word_of(hex, word);
	return nu54_token_units(word, &units) && units == wanted;
}

static bool refuses(const char *hex)
{
	uint8_t word[32];
	uint64_t units = 12345;
	word_of(hex, word);
	return !nu54_token_units(word, &units) && units == 12345;
}

int main(void)
{
	/* 6,500,000 units of 10^16: 65,000 won. */
	expect("an amount", reads("000000000000000000000000000000000000000000000dc3a8351f3d86a00000", 6500000));
	expect("one unit", reads("000000000000000000000000000000000000000000000000002386f26fc10000", 1));
	expect("nothing", reads("0000000000000000000000000000000000000000000000000000000000000000", 0));
	expect("the most", reads("00000000000000000000000000000000002386f26fc0ffffffdc790d903f0000", UINT64_MAX));
	expect("one more than the most", refuses("00000000000000000000000000000000002386f26fc100000000000000000000"));
	expect("a part of a unit", refuses("0000000000000000000000000000000000000000000000000000000000000001"));
	expect("a unit less one", refuses("000000000000000000000000000000000000000000000000002386f26fc0ffff"));
	expect("a unit and one", refuses("000000000000000000000000000000000000000000000000002386f26fc10001"));
	expect("a huge amount", refuses("ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"));
	expect("an amount with only high bytes", refuses("0100000000000000000000000000000000000000000000000000000000000000"));
	for (uint64_t units = 1, step = 0; step < 4000; ++step, units = units * 6364136223846793005u + 1442695040888963407u) {
		uint8_t word[32];
		uint64_t back = 0;
		nu54_token_word(word, units);
		if (!nu54_token_units(word, &back) || back != units) { expect("there and back", false); break; }
		word[31] ^= 1;
		if (nu54_token_units(word, &back)) { expect("an odd amount refused", false); break; }
	}
	return failed;
}
