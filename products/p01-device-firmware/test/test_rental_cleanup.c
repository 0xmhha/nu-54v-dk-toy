#include "nu54_eip712.h"
#include "nu54_keccak.h"
#include "nu54_rental_retirement.h"
#include "nu54_sig.h"

#include <assert.h>
#include <stdio.h>
#include <string.h>

enum { INTENT_SIZE = 181, RECEIPT_SIZE = 109, LIFETIME = 120 };

struct scope {
	uint8_t chain[32], settlement[20], operator[20], board[20];
	uint8_t binding[32], disposition[32], request[32];
};

static uint64_t load_u64(const uint8_t bytes[8])
{
	uint64_t value = 0;
	for (unsigned i = 0; i < 8; ++i) value = (value << 8) | bytes[i];
	return value;
}

static void hash_typed(const struct scope *scope, const uint8_t *words, size_t size, uint8_t digest[32])
{
	uint8_t domain[32], hash[32];
	nu54_eip712_domain_separator(scope->chain, scope->settlement, domain);
	nu54_keccak256(words, size, hash);
	nu54_eip712_digest(domain, hash, digest);
}

static void hash_intent(const struct scope *scope, const uint8_t intent[INTENT_SIZE], uint8_t digest[32])
{
	nu54_rental_cleanup_intent_digest(scope->chain, scope->settlement, intent, digest);
}

static void hash_receipt(const struct scope *scope, const uint8_t receipt[RECEIPT_SIZE], uint8_t digest[32])
{
	static const char type[] = "RentalCleanupReceiptV1(bytes32 intent,uint64 retiredAt)";
	uint8_t words[3][32] = {{0}};
	nu54_keccak256((const uint8_t *)type, sizeof(type) - 1, words[0]);
	memcpy(words[1], receipt + 4, 32);
	memcpy(words[2] + 24, receipt + 36, 8);
	hash_typed(scope, words[0], sizeof(words), digest);
}

static int signed_by(const uint8_t digest[32], const uint8_t signature[65], const uint8_t address[20])
{
	uint8_t signer[20];
	return nu54_sig_recover(digest, signature, 65, signer) == NU54_SIG_OK &&
		memcmp(signer, address, sizeof(signer)) == 0;
}

static int present(const uint8_t *bytes, size_t size)
{
	uint8_t value = 0;
	for (size_t i = 0; i < size; ++i) value |= bytes[i];
	return value != 0;
}

static int scope_valid(const struct scope *scope)
{
	return present(scope->chain + 24, 8) && !present(scope->chain, 24) &&
		present(scope->settlement, 20) && present(scope->operator, 20) && present(scope->board, 20) &&
		present(scope->binding, 32) && present(scope->disposition, 32) && present(scope->request, 32) &&
		memcmp(scope->operator, scope->board, 20) != 0;
}

/* Host proof verifier only. It has no device or protected-store write API. */
static int verify_receipt(const struct scope *scope, const uint8_t *intent, size_t intent_size,
			  const uint8_t *receipt, size_t receipt_size, uint64_t now)
{
	if (intent_size != INTENT_SIZE || receipt_size != RECEIPT_SIZE ||
	    memcmp(intent, "RCI1", 4) || memcmp(receipt, "RCR1", 4) ||
	    !scope_valid(scope) ||
	    memcmp(intent + 4, scope->binding, 32) || memcmp(intent + 36, scope->disposition, 32) ||
	    memcmp(intent + 68, scope->request, 32))
		return 0;
	const uint64_t issued = load_u64(intent + 100), expires = load_u64(intent + 108);
	const uint64_t retired = load_u64(receipt + 36);
	if (!issued || retired < issued || retired >= expires || retired > now || expires - issued > LIFETIME)
		return 0;
	uint8_t digest[32];
	hash_intent(scope, intent, digest);
	if (memcmp(digest, receipt + 4, 32) || !signed_by(digest, intent + 116, scope->operator))
		return 0;
	hash_receipt(scope, receipt, digest);
	return signed_by(digest, receipt + 44, scope->board);
}

static void read_vector(FILE *file, uint8_t *bytes, size_t size)
{
	for (size_t i = 0; i < size; ++i) {
		unsigned value;
		assert(fscanf(file, "%2x", &value) == 1);
		bytes[i] = (uint8_t)value;
	}
	assert(fgetc(file) == '\n');
}

int main(int argc, char **argv)
{
	assert(argc == 2);
	FILE *file = fopen(argv[1], "r");
	assert(file);
	uint8_t intent[INTENT_SIZE], receipt[RECEIPT_SIZE], expected_intent[32], expected_receipt[32], digest[32];
	struct scope scope = {.chain = {[30] = 0x20, [31] = 0x5b}};
	read_vector(file, intent, sizeof(intent));
	read_vector(file, expected_intent, sizeof(expected_intent));
	read_vector(file, receipt, sizeof(receipt));
	read_vector(file, expected_receipt, sizeof(expected_receipt));
	read_vector(file, scope.settlement, sizeof(scope.settlement));
	read_vector(file, scope.operator, sizeof(scope.operator));
	read_vector(file, scope.board, sizeof(scope.board));
	assert(fgetc(file) == EOF && fclose(file) == 0);
	scope.binding[0] = 1;
	scope.disposition[0] = 2;
	scope.request[0] = 3;
	hash_intent(&scope, intent, digest);
	assert(!memcmp(digest, expected_intent, sizeof(digest)));
	hash_receipt(&scope, receipt, digest);
	assert(!memcmp(digest, expected_receipt, sizeof(digest)));
	const struct scope enrolled = scope;
	assert(scope_valid(&enrolled));
	uint8_t *fields[] = {scope.chain, scope.settlement, scope.operator, scope.board,
		scope.binding, scope.disposition, scope.request};
	const size_t sizes[] = {32, 20, 20, 20, 32, 32, 32};
	for (size_t i = 0; i < sizeof(fields) / sizeof(fields[0]); ++i) {
		scope = enrolled;
		memset(fields[i], 0, sizes[i]);
		assert(!scope_valid(&scope));
		assert(!verify_receipt(&scope, intent, sizeof(intent), receipt, sizeof(receipt), 1200));
	}
	scope = enrolled;
	scope.chain[0] = 1;
	assert(!scope_valid(&scope));
	scope = enrolled;
	assert(verify_receipt(&scope, intent, sizeof(intent), receipt, sizeof(receipt), 1200));
	assert(!verify_receipt(&scope, intent, sizeof(intent), receipt, sizeof(receipt), 1059));
	uint8_t signed_bytes[sizeof(receipt)];
	memcpy(signed_bytes, receipt, sizeof(receipt));
	for (size_t i = 0; i < sizeof(receipt); ++i) {
		receipt[i] ^= 1;
		assert(!verify_receipt(&scope, intent, sizeof(intent), receipt, sizeof(receipt), 1200));
		receipt[i] ^= 1;
	}
	assert(!memcmp(signed_bytes, receipt, sizeof(receipt)));
	assert(!verify_receipt(&scope, intent, sizeof(intent), (const uint8_t *)"setup.ack", 9, 1200));
	assert(!verify_receipt(&scope, intent, sizeof(intent), receipt, sizeof(receipt) - 1, 1200));
	scope.binding[1] ^= 1;
	assert(!verify_receipt(&scope, intent, sizeof(intent), receipt, sizeof(receipt), 1200));
	scope = enrolled;
	scope.disposition[1] ^= 1;
	assert(!verify_receipt(&scope, intent, sizeof(intent), receipt, sizeof(receipt), 1200));
	scope = enrolled;
	scope.request[1] ^= 1;
	assert(!verify_receipt(&scope, intent, sizeof(intent), receipt, sizeof(receipt), 1200));
	scope = enrolled;
	scope.board[1] ^= 1;
	assert(!verify_receipt(&scope, intent, sizeof(intent), receipt, sizeof(receipt), 1200));
	scope = enrolled;
	scope.operator[1] ^= 1;
	assert(!verify_receipt(&scope, intent, sizeof(intent), receipt, sizeof(receipt), 1200));
	scope = enrolled;
	scope.settlement[1] ^= 1;
	assert(!verify_receipt(&scope, intent, sizeof(intent), receipt, sizeof(receipt), 1200));
	scope = enrolled;
	scope.chain[31] ^= 1;
	assert(!verify_receipt(&scope, intent, sizeof(intent), receipt, sizeof(receipt), 1200));
	scope = enrolled;
	memcpy(scope.board, scope.operator, sizeof(scope.board));
	assert(!scope_valid(&scope));
	assert(!verify_receipt(&scope, intent, sizeof(intent), receipt, sizeof(receipt), 1200));
	return 0;
}
