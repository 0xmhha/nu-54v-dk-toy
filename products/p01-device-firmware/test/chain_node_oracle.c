#include "nu54_btc.h"
#include "nu54_commissioned_chain.h"
#include "nu54_hd.h"
#include "nu54_solana.h"
#include "nu54_xrpl.h"

#include <openssl/evp.h>
#include <openssl/hmac.h>
#include <openssl/rand.h>
#include <secp256k1.h>
#include <string.h>

#define HARDENED UINT32_C(0x80000000)
#define HEADER 217

typedef struct {
	const uint8_t *seed;
	uint32_t account;
	uint8_t chain, network, blind[32];
} synthetic_wallet;

static int hash(unsigned expected, const EVP_MD *algorithm, const uint8_t *message, size_t size, uint8_t *output)
{
	unsigned actual;
	return EVP_Digest(message, size, output, &actual, algorithm, NULL) != 1 || actual != expected ? -1 : 0;
}
static int sha256(const uint8_t *message, size_t size, uint8_t digest[32])
{
	return hash(32, EVP_sha256(), message, size, digest);
}
static int hash256(const uint8_t *message, size_t size, uint8_t digest[32])
{
	uint8_t first[32];
	return sha256(message, size, first) || sha256(first, 32, digest) ? -1 : 0;
}
static int hash160(const uint8_t key[33], uint8_t digest[20])
{
	uint8_t first[32];
	return sha256(key, 33, first) || hash(20, EVP_ripemd160(), first, 32, digest) ? -1 : 0;
}
static int half(const uint8_t *message, size_t size, uint8_t digest[32])
{
	uint8_t full[64];
	if (hash(64, EVP_sha512(), message, size, full)) return -1;
	memcpy(digest, full, 32);
	return 0;
}
static int hmac(const uint8_t *key, size_t key_len, const uint8_t *data, size_t length, uint8_t result[64])
{
	unsigned size;
	return !HMAC(EVP_sha512(), key, (int)key_len, data, length, result, &size) || size != 64 ? -1 : 0;
}
static uint64_t big(const uint8_t *p, size_t n)
{
	uint64_t value = 0;
	while (n--) value = value * 256 + *p++;
	return value;
}

static void bip_path(const synthetic_wallet *wallet, uint32_t branch, uint32_t index, uint32_t path[5])
{
	path[0] = HARDENED | (wallet->chain == 1 ? 84 : 44);
	path[1] = HARDENED | (wallet->chain == 1 ? 1 : 144);
	path[2] = HARDENED | nu54_chain_derivation_account(wallet->chain, wallet->network, wallet->account);
	path[3] = branch;
	path[4] = index;
}

typedef struct {
	const uint8_t *expected, *message;
	size_t length;
	uint8_t *result;
} ed_operation;

static int ed(const uint8_t seed[32], void *opaque)
{
	const ed_operation *op = opaque;
	EVP_PKEY *key = EVP_PKEY_new_raw_private_key(EVP_PKEY_ED25519, NULL, seed, 32);
	EVP_MD_CTX *signer = NULL;
	uint8_t public[32];
	size_t size = 32;
	int result = -1;
	if (!key || EVP_PKEY_get_raw_public_key(key, public, &size) != 1 || size != 32) goto done;
	if (!op->expected) {
		memcpy(op->result, public, 32);
		result = 0;
		goto done;
	}
	size = 64;
	if (!memcmp(public, op->expected, 32) && (signer = EVP_MD_CTX_new()) &&
	    EVP_DigestSignInit(signer, NULL, NULL, NULL, key) == 1 &&
	    EVP_DigestSign(signer, op->result, &size, op->message, op->length) == 1 && size == 64) result = 0;
done:
	EVP_MD_CTX_free(signer);
	EVP_PKEY_free(key);
	return result;
}

static int public_at(const uint8_t seed[32], uint8_t chain, uint8_t network, uint32_t account, uint32_t branch,
		     uint32_t index, uint8_t public[33], uint8_t hash[20], uint8_t fingerprint[4])
{
	synthetic_wallet wallet = {.seed = seed, .chain = chain, .network = network, .account = account};
	if (account > 99 || network > NU54_CHAIN_COMMISSIONED) return -1;
	if (chain == 2) {
		ed_operation op = {.result = public};
		memset(public, 0, 33);
		memset(hash, 0, 20);
		memset(fingerprint, 0, 4);
		return nu54_sol_child(seed, account, hmac, ed, &op);
	}
	if (RAND_bytes(wallet.blind, sizeof(wallet.blind)) != 1) return -1;
	uint32_t path[5];
	uint8_t master[33], master_hash[20];
	bip_path(&wallet, branch, index, path);
	if (nu54_hd_bip32_public(seed, wallet.blind, hmac, path, 5, public, NULL) ||
	    hash160(public, hash) ||
	    nu54_hd_bip32_public(seed, wallet.blind, hmac, path, 0, master, NULL) ||
	    hash160(master, master_hash)) return -1;
	memcpy(fingerprint, master_hash, 4);
	return 0;
}

int chain_host_public_network(const uint8_t seed[32], uint8_t chain, uint8_t network, uint32_t account, uint32_t branch,
			      uint8_t public[33], uint8_t hash[20], uint8_t fingerprint[4])
{
	return public_at(seed, chain, network, account, branch, 0, public, hash, fingerprint);
}

int chain_host_bitcoin_public_at(const uint8_t seed[32], uint32_t account, uint32_t branch, uint32_t index,
				 uint8_t public[33], uint8_t hash[20], uint8_t fingerprint[4])
{
	if (branch > 1 || index > 9999) return -1;
	return public_at(seed, 1, 1, account, branch, index, public, hash, fingerprint);
}

int chain_host_public(const uint8_t seed[32], uint8_t chain, uint32_t account, uint32_t branch,
		      uint8_t public[33], uint8_t hash[20], uint8_t fingerprint[4])
{
	return chain_host_public_network(seed, chain, 1, account, branch, public, hash, fingerprint);
}

int chain_host_watch(const uint8_t seed[32], uint32_t account, uint8_t output[80])
{
	if (account > 99) return -1;
	synthetic_wallet wallet = {.seed = seed, .chain = 1, .network = NU54_CHAIN_COMMISSIONED, .account = account};
	uint32_t path[5];
	uint8_t parent[33], root[33], digest[20];
	bip_path(&wallet, 0, 0, path);
	if (RAND_bytes(wallet.blind, sizeof(wallet.blind)) != 1 ||
	    nu54_hd_bip32_public(seed, wallet.blind, hmac, path, 0, root, NULL) || hash160(root, digest))
		return -1;
	const uint8_t header[7] = {13, 1, NU54_CHAIN_COMMISSIONED, 0, 0, 0, (uint8_t)account};
	memcpy(output, header, sizeof(header));
	memcpy(output + 7, digest, 4);
	if (nu54_hd_bip32_public(seed, wallet.blind, hmac, path, 2, parent, NULL) || hash160(parent, digest))
		return -1;
	memcpy(output + 11, digest, 4);
	return nu54_hd_bip32_public(seed, wallet.blind, hmac, path, 3, output + 47, output + 15);
}

static int btc_public(void *opaque, uint32_t branch, uint32_t index, uint8_t public[33], uint8_t pkhash[20])
{
	synthetic_wallet *wallet = opaque;
	uint32_t path[5];
	bip_path(wallet, branch, index, path);
	return nu54_hd_bip32_public(wallet->seed, wallet->blind, hmac, path, 5, public, NULL) ||
	       hash160(public, pkhash) ? -1 : 0;
}
static int btc_sign(void *opaque, const nu54_btc_input *input, uint8_t signature[64])
{
	synthetic_wallet *wallet = opaque;
	uint32_t path[5];
	bip_path(wallet, input->branch, input->index, path);
	return nu54_hd_bip32_sign(wallet->seed, wallet->blind, hmac, path, 5, input->pubkey, input->digest, signature);
}
static int sol_sign(void *opaque, const uint8_t public[32], const uint8_t *message, size_t length, uint8_t sig[64])
{
	synthetic_wallet *wallet = opaque;
	ed_operation op = {.expected = public, .message = message, .length = length, .result = sig};
	return nu54_sol_child(wallet->seed, wallet->account, hmac, ed, &op);
}
static int xrp_sign(void *opaque, const uint8_t public[33], const uint8_t digest[32], uint8_t der[72], size_t *length)
{
	synthetic_wallet *wallet = opaque;
	uint32_t path[5];
	uint8_t signature[64];
	secp256k1_ecdsa_signature parsed;
	bip_path(wallet, 0, 0, path);
	return nu54_hd_bip32_sign(wallet->seed, wallet->blind, hmac, path, 5, public, digest, signature) ||
	       !secp256k1_ecdsa_signature_parse_compact(secp256k1_context_static, &parsed, signature) ||
	       !secp256k1_ecdsa_signature_serialize_der(secp256k1_context_static, der, length, &parsed) ? -1 : 0;
}

int chain_host_sign(const uint8_t seed[32], const uint8_t *grant, size_t n, uint8_t output[1232], size_t *written)
{
	*written = 0;
	if (n <= HEADER || n > 1800 || grant[0] != 11 || (grant[2] != 1 && grant[2] != NU54_CHAIN_COMMISSIONED)) return -1;
	const uint8_t *data = grant + 23, *raw = grant + HEADER;
	size_t length = n - HEADER;
	uint64_t fee_max = big(grant + 15, 8);
	synthetic_wallet wallet = {.seed = seed, .chain = grant[1], .network = grant[2], .account = (uint32_t)big(grant + 3, 4)};
	uint8_t public[33], pkhash[20], fingerprint[4];
	if (RAND_bytes(wallet.blind, sizeof(wallet.blind)) != 1 ||
	    chain_host_public_network(seed, wallet.chain, wallet.network, wallet.account, 0, public, pkhash, fingerprint)) return -1;
	switch (wallet.chain) {
	case 1: {
		uint32_t fp = 0;
		for (unsigned i = 0; i < 4; i++) fp |= (uint32_t)fingerprint[i] << (8 * i);
		nu54_btc_policy policy = {.hash256 = hash256, .derive = btc_public, .wallet = &wallet,
			.fingerprint = fp, .coin = 1, .account = wallet.account, .max_fee = fee_max};
		nu54_btc_intent review;
		if (nu54_btc_review(raw, length, &policy, &review) || nu54_btc_check_context(&review, data, fee_max)) return -1;
		return nu54_btc_finalize(raw, length, &policy, review.commitment, btc_sign, output, 1232, written);
	}
	case 2: {
		nu54_sol_policy policy = {.block_height = big(data + 32, 8), .last_valid_block_height = big(data + 40, 8),
			.fee = big(data + 48, 8), .max_fee = fee_max, .allow_spl = data[89], .decimals = data[88], .sha256 = sha256};
		memcpy(policy.payer, public, 32);
		memcpy(policy.blockhash, data, 32);
		memcpy(policy.mint, data + 56, 32);
		nu54_sol_intent review;
		if (sha256(raw, length, policy.message_hash) || nu54_sol_review(raw, length, &policy, &review)) return -1;
		return nu54_sol_finalize(raw, length, &policy, review.commitment, sol_sign, &wallet, output, 1232, written);
	}
	case 3: {
		nu54_xrpl_policy policy = {.ledger = (uint32_t)big(data, 4), .max_ledger_delta = (uint32_t)big(data + 4, 4),
			.sequence = (uint32_t)big(data + 8, 4), .ticket = (uint32_t)big(data + 12, 4),
			.max_fee = fee_max, .allow_issued = data[56], .sha512_half = half};
		memcpy(policy.account, pkhash, 20);
		memcpy(policy.public_key, public, 33);
		memcpy(policy.currency, data + 16, 20);
		memcpy(policy.issuer, data + 36, 20);
		nu54_xrpl_intent review;
		if (nu54_xrpl_review(raw, length, &policy, &review)) return -1;
		return nu54_xrpl_finalize(raw, length, &policy, review.digest, xrp_sign, &wallet, output, 1232, written);
	}
	default:
		return -1;
	}
}

#include "nu54_btc48.h"

static int sign_bip48(void *opaque, uint32_t branch, uint32_t index,
		      const uint8_t public_key[33], const uint8_t digest[32], uint8_t signature[64])
{
	synthetic_wallet *wallet = opaque;
	const uint32_t path[] = {HARDENED | 48, HARDENED | 1, HARDENED | wallet->account,
		HARDENED | 2, branch, index};
	return nu54_hd_bip32_sign(wallet->seed, wallet->blind, hmac, path, 6, public_key, digest, signature);
}

int chain_host_bip48_network(const uint8_t seed[32], const uint8_t roots[207], uint8_t owner, uint8_t network,
		     const uint8_t *psbt, size_t n, uint8_t out[NU54_BTC48_MAX+NU54_BTC48_INPUTS*NU54_BTC48_SIGNATURE_FIELD_MAX], size_t *written)
{
	*written = 0;
	if (owner >= 3) return -1;
	synthetic_wallet wallet = {.seed = seed};
	if (RAND_bytes(wallet.blind, sizeof(wallet.blind)) != 1) return -1;
	nu54_btc48_scope scope = {.network = network, .owner = owner, .max_fee = 100000};
	memcpy(scope.roots, roots, sizeof(scope.roots));
	const uint32_t path[] = {HARDENED | 48, HARDENED | 1, HARDENED, HARDENED | 2};
	uint8_t key[33], code[32], master[33], hash[20];
	if (nu54_hd_bip32_public(seed, wallet.blind, hmac, path, 4, key, code) ||
	    nu54_hd_bip32_public(seed, wallet.blind, hmac, path, 0, master, NULL) ||
	    hash160(master, hash) || memcmp(key, scope.roots[owner].public_key, 33) ||
	    memcmp(code, scope.roots[owner].chain_code, 32) || memcmp(hash, scope.roots[owner].fingerprint, 4)) return -1;
	nu54_btc48_intent intent;
	if (nu54_btc48_review(psbt, n, &scope, hmac, sha256, &intent)) return -1;
	return nu54_btc48_sign_psbt(psbt, n, &scope, hmac, sha256, intent.commitment,
				    sign_bip48, &wallet, out, NU54_BTC48_MAX+NU54_BTC48_INPUTS*NU54_BTC48_SIGNATURE_FIELD_MAX, written);
}


int chain_host_bip48(const uint8_t seed[32], const uint8_t roots[207], uint8_t owner,
		     const uint8_t *psbt, size_t n, uint8_t out[NU54_BTC48_MAX+NU54_BTC48_INPUTS*NU54_BTC48_SIGNATURE_FIELD_MAX], size_t *written)
{
	return chain_host_bip48_network(seed, roots, owner, 1, psbt, n, out, written);
}
