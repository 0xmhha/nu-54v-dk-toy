#include "nu54_quote_v2.h"

#include "nu54_keccak.h"
#include "nu54_sig.h"
#include <assert.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static bool recover_address(const uint8_t digest[32], const uint8_t sig[65], uint8_t out[20])
{
	return nu54_sig_recover(digest, sig, 65, out) == NU54_SIG_OK;
}

static uint8_t nibble(char c)
{
	if (c >= '0' && c <= '9') return (uint8_t)(c - '0');
	assert(c >= 'a' && c <= 'f');
	return (uint8_t)(c - 'a' + 10);
}

static size_t decode(const char *hex, uint8_t *out, size_t capacity)
{
	size_t length = strlen(hex) / 2;
	assert(strlen(hex) % 2 == 0 && length <= capacity);
	for (size_t i = 0; i < length; i++) {
		out[i] = (uint8_t)((nibble(hex[2 * i]) << 4) | nibble(hex[2 * i + 1]));
	}
	return length;
}

int main(int argc, char **argv)
{
	assert(argc == 2);
	FILE *fixtures = fopen(argv[1], "r");
	assert(fixtures != NULL);
	const nu54_v2_quote_crypto crypto = {nu54_keccak256, recover_address};
	const uint8_t widths[] = NU54_V2_WIDTHS;
	char line[5000];
	int count = 0;
	while (fgets(line, sizeof line, fixtures)) {
		if (line[0] == '#') continue;
		char *columns[9] = {line};
		for (size_t i = 1; i < 9; i++) {
			char *separator = strchr(columns[i - 1], '|');
			assert(separator != NULL);
			*separator = '\0';
			columns[i] = separator + 1;
		}
		uint8_t signed_quote[NU54_V2_SIGNED_SIZE + 1], raw[NU54_V2_UNSIGNED_SIZE];
		nu54_v2_quote_policy policy = {0};
		size_t length = decode(columns[1], signed_quote, sizeof signed_quote);
		assert(decode(columns[2], policy.expected, sizeof policy.expected) == sizeof policy.expected);
		assert(decode(columns[3], raw, sizeof raw) == sizeof raw);
		assert(decode(columns[4], policy.merchant_signer, sizeof policy.merchant_signer) == sizeof policy.merchant_signer);
		policy.now = strtoull(columns[5], NULL, 10);
		policy.consumed = strcmp(columns[6], "true") == 0;
		uint8_t digest[32], expected_digest[32];
		bool valid = nu54_v2_quote_digest(raw, sizeof raw, &crypto, digest);
		if (columns[7][0] == '\0') {
			assert(!valid);
		} else {
			assert(valid);
			nu54_v2_quote_field fields[NU54_V2_FIELD_COUNT];
			for (size_t i = 0; i < NU54_V2_FIELD_COUNT; i++) {
				fields[i] = (nu54_v2_quote_field){raw + (i + 1) * 32 - widths[i], widths[i]};
			}
			uint8_t encoded[NU54_V2_UNSIGNED_SIZE];
			assert(nu54_v2_quote_encode(fields, NU54_V2_FIELD_COUNT, encoded));
			assert(memcmp(encoded, raw, sizeof raw) == 0);
			assert(decode(columns[7], expected_digest, sizeof expected_digest) == sizeof expected_digest);
			assert(memcmp(digest, expected_digest, sizeof digest) == 0);
		}
		int status = nu54_v2_quote_verify(signed_quote, length, &policy, &crypto);
		if (status != (int)strtol(columns[8], NULL, 10)) {
			fprintf(stderr, "%s unexpected status %d\n", columns[0], status);
			return 1;
		}
		count++;
	}
	fclose(fixtures);
	assert(count >= 23);
	printf("%d C quote vectors passed\n", count);
	return 0;
}
