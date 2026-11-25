/* The return to the original wallet, checked against the vector the real token accepted. */
#include "nu54_spending.h"

#include <stdio.h>
#include <stdlib.h>
#include <string.h>

static int load(FILE *file, const char *key, uint8_t *out, size_t length)
{
	char line[512];
	size_t size = strlen(key);
	rewind(file);
	while (fgets(line, sizeof(line), file)) {
		if (strncmp(line, key, size) || line[size] != '=') continue;
		const char *hex = line + size + 3; /* =0x */
		if (strcspn(hex, "\n") != length * 2) return 0;
		for (size_t i = 0; i < length; ++i) {
			unsigned byte;
			if (sscanf(hex + 2 * i, "%2x", &byte) != 1) return 0;
			out[i] = (uint8_t)byte;
		}
		return 1;
	}
	return 0;
}

int main(int argc, char **argv)
{
	uint8_t token[20], from[20], to[20], nonce[32], expected[NU54_LOCAL_PREIMAGE_SIZE], built[NU54_LOCAL_PREIMAGE_SIZE];
	FILE *file = argc == 3 ? fopen(argv[1], "r") : NULL;
	if (!file || !load(file, "token", token, 20) || !load(file, "from", from, 20) || !load(file, "to", to, 20) ||
	    !load(file, "nonce", nonce, 32) || !load(file, "preimage", expected, sizeof(expected))) {
		fprintf(stderr, "vector missing\n");
		return 1;
	}
	nu54_spend_return_preimage(token, from, to, strtoull(argv[2], NULL, 10), UINT64_C(1800000500), nonce, built);
	fclose(file);
	if (memcmp(built, expected, sizeof(built))) {
		fprintf(stderr, "the board would sign something else\n");
		return 1;
	}
	return 0;
}
