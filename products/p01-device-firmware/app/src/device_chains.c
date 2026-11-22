#include "device_chains.h"
#include "device_install.h"
#include "nu54_btc48.h"
#include "nu54_commissioned_chain.h"
#include <psa/internal_trusted_storage.h>

#include <psa/crypto.h>
#include <secp256k1.h>
#include <string.h>
#include <zephyr/sys/byteorder.h>

#define MAX_GRANT_SECONDS 60

int device_ripemd160(const uint8_t *, size_t, uint8_t [20]);

static const uint8_t bitcoin_regtest_anchor[] =
	"0f9188f13cb7b2c71f2a335e3a4fc328bf5beb436012afca590b1a11466e2206";

static bool zeroes(const uint8_t *data, size_t size)
{
	uint8_t byte = 0;
	while (size--) byte |= *data++;
	return byte == 0;
}

static int sha256(const uint8_t *message, size_t length, uint8_t digest[32])
{
	size_t size;
	return psa_hash_compute(PSA_ALG_SHA_256, message, length, digest, 32, &size) == PSA_SUCCESS &&
	       size == 32 ? 0 : -1;
}

static int hash256(const uint8_t *message, size_t length, uint8_t digest[32])
{
	uint8_t hash[32];
	return sha256(message, length, hash) || sha256(hash, sizeof(hash), digest) ? -1 : 0;
}

static int sha512_half(const uint8_t *message, size_t length, uint8_t digest[32])
{
	uint8_t full[64];
	size_t size;
	if (psa_hash_compute(PSA_ALG_SHA_512, message, length, full, sizeof(full), &size) != PSA_SUCCESS ||
	    size != sizeof(full)) return -1;
	memcpy(digest, full, 32);
	return 0;
}

static int hash160(const uint8_t pubkey[33], uint8_t hash[20])
{
	uint8_t digest[32];
	return sha256(pubkey, 33, digest) || device_ripemd160(digest, sizeof(digest), hash) ? -1 : 0;
}

static device_chain_account decode_account(const uint8_t *wire)
{
	return (device_chain_account){wire[1], wire[2], sys_get_be32(wire + 3)};
}

#define BIP48_UID ((psa_storage_uid_t)0x54a0)
#define BIP48_SIGNET_UID ((psa_storage_uid_t)0x54a1)

static bool bip48_network(uint8_t network)
{
#ifdef CONFIG_NU54_COMMISSIONED_WALLET_CHAINS
	if (network == NU54_CHAIN_COMMISSIONED) return true;
#endif
	return network == 1;
}

static psa_storage_uid_t bip48_uid(uint8_t network)
{
	return network == NU54_CHAIN_COMMISSIONED ? BIP48_SIGNET_UID : BIP48_UID;
}
#define BIP48_POLICY_SIZE 224
#define BIP48_RECEIPT_SIZE 33

static int bip48_policy(const uint8_t wire[BIP48_POLICY_SIZE], nu54_btc48_scope *scope)
{
	if (wire[0] != NU54_CHAIN_BIP48_REGISTER || wire[1] != NU54_CHAIN_BTC ||
	    !bip48_network(wire[2]) || wire[16] != 2) return -1;
	*scope = (nu54_btc48_scope){.network = wire[2], .account = sys_get_be32(wire + 3),
				    .max_fee = sys_get_be64(wire + 7), .owner = wire[15]};
	memcpy(scope->roots, wire + 17, sizeof(scope->roots));
	return nu54_btc48_scope_valid(scope);
}

static int bip48_owned(const device_chain_account *account, const nu54_btc48_scope *scope)
{
	uint8_t master[33], fingerprint[20], pub[33], code[32];
	const nu54_btc48_xpub *key = &scope->roots[scope->owner];
	return device_chain_btc_root(master) || hash160(master, fingerprint) ||
	       device_chain_bip48_pub(account, pub, code) || memcmp(key->fingerprint, fingerprint, 4) ||
	       memcmp(key->public_key, pub, 33) || memcmp(key->chain_code, code, 32) ? -1 : 0;
}

static int bip48_load(uint8_t network, uint8_t wire[BIP48_POLICY_SIZE], nu54_btc48_scope *scope)
{
	size_t actual;
	psa_status_t status = psa_its_get(bip48_uid(network), 0, BIP48_POLICY_SIZE, wire, &actual);
	if (status == PSA_ERROR_DOES_NOT_EXIST) return 1;
	return status == PSA_SUCCESS && actual == BIP48_POLICY_SIZE && !bip48_policy(wire, scope) && scope->network == network ? 0 : -1;
}

static int bip48_review(device_chains *state, const uint8_t *wire, const uint8_t *raw)
{
	uint8_t stored[BIP48_POLICY_SIZE], digest[32];
	nu54_btc48_scope scope;
	nu54_btc48_intent intent;
	if (state->account.chain != NU54_CHAIN_BTC || state->raw_size > NU54_BTC48_MAX ||
	    !zeroes(wire + 55, 58) || bip48_load(state->account.network, stored, &scope) ||
	    scope.account != state->account.account || bip48_owned(&state->account, &scope) ||
	    sha256(stored, sizeof(stored), digest) || memcmp(wire + 23, digest, 32)) return -1;
	uint64_t fee = sys_get_be64(wire + 15);
	if (!fee || fee > scope.max_fee) return -1;
	scope.max_fee = fee;
	if (nu54_btc48_review(raw, state->raw_size, &scope, device_chain_public_hmac, sha256, &intent)) return -1;
	memcpy(state->commitment, intent.commitment, 32);
	state->policy.btc.max_fee = fee;
	return 0;
}

static int bip48_signature(void *account, uint32_t branch, uint32_t index,
			    const uint8_t pub[33], const uint8_t digest[32], uint8_t sig[64])
{
	return device_chain_bip48_sign(account, branch, index, pub, digest, sig);
}

static int bip48_finish(const device_chains *state, uint8_t *out, size_t *length)
{
	uint8_t stored[BIP48_POLICY_SIZE];
	nu54_btc48_scope scope;
	if (bip48_load(state->account.network, stored, &scope) || bip48_owned(&state->account, &scope)) return -1;
	scope.max_fee = state->policy.btc.max_fee;
	device_chain_account account = state->account;
	return nu54_btc48_sign_psbt(state->raw, state->raw_size, &scope, device_chain_public_hmac, sha256,
				    state->commitment, bip48_signature, &account, out, NU54_CHAIN_REPLY_MAX, length);
}

static int bip48_register(const device_chains *state, uint8_t *out, size_t *length)
{
	uint8_t stored[BIP48_POLICY_SIZE];
	nu54_btc48_scope scope;
	if (bip48_load(state->account.network, stored, &scope) != 1 || bip48_policy(state->raw, &scope) ||
	    bip48_owned(&state->account, &scope) || sha256(state->raw, BIP48_POLICY_SIZE, out + 1) ||
	    psa_its_set(bip48_uid(state->account.network), BIP48_POLICY_SIZE, state->raw, PSA_STORAGE_FLAG_WRITE_ONCE) != PSA_SUCCESS) return -1;
	out[0] = NU54_CHAIN_BIP48_REGISTER;
	*length = BIP48_RECEIPT_SIZE;
	return 0;
}

int device_chains_public(const nu54_v2_request *request, uint8_t output[NU54_CHAIN_INDEXED_REPLY_SIZE], size_t *length)
{
	if (request->payload_length == 7 && request->payload[0] == NU54_CHAIN_BIP48_STATUS) {
		uint8_t stored[BIP48_POLICY_SIZE];
		nu54_btc48_scope scope;
		device_chain_account account = decode_account(request->payload);
		if (account.chain != NU54_CHAIN_BTC || !bip48_network(account.network) || account.account > 99) return -1;
		int status = bip48_load(account.network, stored, &scope);
		memset(output, 0, NU54_CHAIN_PUBLIC_REPLY_SIZE);
		*length = NU54_CHAIN_PUBLIC_REPLY_SIZE;
		output[0] = NU54_CHAIN_BIP48_STATUS;
		if (status == 1) return 0;
		if (status || account.chain != NU54_CHAIN_BTC || scope.account != account.account ||
		    bip48_owned(&account, &scope) || sha256(stored, sizeof(stored), output + 1)) return -1;
		return 0;
	}
	size_t header = request->payload_length;
	if ((header != 11 && header != 15) || request->payload[0] != NU54_CHAIN_PUBLIC_OP) return -1;
	device_chain_account account = decode_account(request->payload);
	uint32_t branch = sys_get_be32(request->payload + 7);
	uint32_t index = header == 15 ? sys_get_be32(request->payload + 11) : 0;
	if (header == 15 && (account.chain != NU54_CHAIN_BTC || !index)) return -1;
	uint8_t pubkey[33], key_hash[20] = {0}, fingerprint[4] = {0};
	if (device_chain_public(&account, branch, index, pubkey) ||
	    (account.chain != NU54_CHAIN_SOL && hash160(pubkey, key_hash))) return -1;
	if (account.chain == NU54_CHAIN_BTC) {
		uint8_t master[33], hash[20];
		if (device_chain_btc_root(master) || hash160(master, hash)) return -1;
		memcpy(fingerprint, hash, 4);
	}
	memcpy(output, request->payload, header);
	memcpy(output + header, pubkey, 33);
	memcpy(output + header + 33, key_hash, 20);
	memcpy(output + header + 53, fingerprint, 4);
	*length = header + 57;
	return 0;
}

static int btc_derive(void *context, uint32_t branch, uint32_t index, uint8_t pubkey[33], uint8_t hash[20])
{
	return device_chain_public(context, branch, index, pubkey) || hash160(pubkey, hash) ? -1 : 0;
}

static int btc_sign(void *context, const nu54_btc_input *input, uint8_t signature[64])
{
	return device_chain_secp_sign(context, input->branch, input->index, input->pubkey, input->digest, signature);
}

static int sol_sign(void *context, const uint8_t pubkey[32], const uint8_t *message,
		    size_t length, uint8_t signature[64])
{
	return device_chain_sol_sign(context, pubkey, message, length, signature);
}

static int xrpl_sign(void *context, const uint8_t pubkey[33], const uint8_t digest[32],
		     uint8_t der[72], size_t *length)
{
	uint8_t rs[64];
	secp256k1_ecdsa_signature sig;
	const secp256k1_context *secp = secp256k1_context_static;
	return device_chain_secp_sign(context, 0, 0, pubkey, digest, rs) ||
	       !secp256k1_ecdsa_signature_parse_compact(secp, &sig, rs) ||
	       !secp256k1_ecdsa_signature_serialize_der(secp, der, length, &sig) ? -1 : 0;
}

static int review(device_chains *state, const uint8_t *wire, const uint8_t raw_hash[32])
{
	const uint8_t *data = wire + 23;
	const uint8_t *raw = wire + NU54_CHAIN_RAW_OFFSET;
	const uint64_t fee_limit = sys_get_be64(wire + 15);
	uint8_t pubkey[33];
	if (!fee_limit || device_chain_public(&state->account, 0, 0, pubkey)) return -1;
	switch (state->account.chain) {
	case NU54_CHAIN_BTC: {
		uint8_t master[33], hash[20];
		nu54_btc_intent intent;
		if (fee_limit > 1000000 ||
		    device_chain_btc_root(master) || hash160(master, hash)) return -1;
		state->policy.btc = (nu54_btc_policy){
			.hash256 = hash256, .derive = btc_derive, .wallet = &state->account,
			.coin = state->account.network ? 1 : 0, .account = state->account.account,
			.fingerprint = sys_get_le32(hash), .max_fee = fee_limit,
		};
		if (nu54_btc_review(raw, state->raw_size, &state->policy.btc, &intent) ||
		    nu54_btc_check_context(&intent, data, fee_limit) ||
		    (state->account.network == NU54_CHAIN_COMMISSIONED && intent.replaceable)) return -1;
		memcpy(state->commitment, intent.commitment, 32);
		return 0;
	}
	case NU54_CHAIN_SOL: {
		nu54_sol_intent intent;
		nu54_sol_policy *p = &state->policy.sol;
		if (fee_limit > 1000000 || data[89] > 1 ||
		    (state->account.network == NU54_CHAIN_COMMISSIONED && data[89]) ||
		    (!data[89] && !zeroes(data + 56, 33)) || (data[89] && zeroes(data + 56, 32))) return -1;
		*p = (nu54_sol_policy){.sha256 = sha256, .block_height = sys_get_be64(data + 32),
				      .last_valid_block_height = sys_get_be64(data + 40),
				      .fee = sys_get_be64(data + 48), .max_fee = fee_limit,
				      .decimals = data[88], .allow_spl = data[89] != 0};
		if (p->last_valid_block_height <= p->block_height ||
		    p->last_valid_block_height - p->block_height > 150 ||
		    p->last_valid_block_height - p->block_height < 80) return -1;
		memcpy(p->payer, pubkey, 32);
		memcpy(p->blockhash, data, 32);
		memcpy(p->mint, data + 56, 32);
		memcpy(p->message_hash, raw_hash, 32);
		if (nu54_sol_review(raw, state->raw_size, p, &intent)) return -1;
		memcpy(state->commitment, intent.commitment, 32);
		return 0;
	}
	case NU54_CHAIN_XRPL: {
		nu54_xrpl_intent intent;
		nu54_xrpl_policy *p = &state->policy.xrpl;
		if (fee_limit > 200000 || data[56] > 1 ||
		    (state->account.network == NU54_CHAIN_COMMISSIONED && data[56]) || !zeroes(data + 57, 33) ||
		    (!data[56] && !zeroes(data + 16, 40))) return -1;
		*p = (nu54_xrpl_policy){.sha512_half = sha512_half, .ledger = sys_get_be32(data),
				       .max_ledger_delta = sys_get_be32(data + 4),
				       .sequence = sys_get_be32(data + 8), .ticket = sys_get_be32(data + 12),
				       .max_fee = fee_limit, .allow_issued = data[56] != 0};
		if (!p->ledger || p->max_ledger_delta > 25 || hash160(pubkey, p->account)) return -1;
		memcpy(p->public_key, pubkey, 33);
		memcpy(p->currency, data + 16, 20);
		memcpy(p->issuer, data + 36, 20);
		if (nu54_xrpl_review(raw, state->raw_size, p, &intent) || intent.last_ledger - p->ledger < 12)
			return -1;
		memcpy(state->commitment, intent.digest, 32);
		return 0;
	}
	default:
		return -1;
	}
}

int device_chains_stage(device_chains *state, const nu54_v2_request *request, const nu54_v2_link *link,
			uint64_t now, uint64_t generation, uint32_t prompt, const uint8_t board[32])
{
	const nu54_install_policy *current = device_install_policy();
	if (!current || state->pending || !now || !generation || !prompt || link->closed ||
	    link->local != NU54_V2_BOARD || link->profile != NU54_V2_MANAGEMENT || link->peer != NU54_V2_OWNER ||
	    request->payload_length < 7 || request->payload_length > NU54_CHAIN_WIRE_MAX)
		return -1;
	const uint8_t *wire = request->payload;
	if (wire[0] != NU54_CHAIN_BIP48_SIGN && request->payload_length > NU54_CHAIN_STANDARD_WIRE_MAX) return -1;
	uint64_t expiry = request->expires;
	if (expiry <= now || expiry - now > MAX_GRANT_SECONDS) return -1;
	bool commissioned = false;
#ifdef CONFIG_NU54_COMMISSIONED_WALLET_CHAINS
	commissioned = wire[2] == NU54_CHAIN_COMMISSIONED && (wire[0] == NU54_CHAIN_TRANSFER_OP ||
		(wire[1] == NU54_CHAIN_BTC && (wire[0] == NU54_CHAIN_BIP48_REGISTER ||
		wire[0] == NU54_CHAIN_BIP48_SIGN || wire[0] == NU54_CHAIN_XPUB_OP)));
#endif
	if (wire[2] != 1 && !commissioned) return -1;
	state->account = decode_account(wire);
	if (wire[0] == NU54_CHAIN_TRANSFER_OP || wire[0] == NU54_CHAIN_BIP48_SIGN) {
		if (request->payload_length <= NU54_CHAIN_RAW_OFFSET) goto reject;
		if (commissioned || wire[1] == NU54_CHAIN_BTC) {
			const char *anchor = commissioned ? nu54_commissioned_anchor(wire[1]) :
				(const char *)bitcoin_regtest_anchor;
			uint8_t network[32];
			if (!anchor || sha256((const uint8_t *)anchor, strlen(anchor), network) ||
			    memcmp(wire + 113, network, sizeof(network))) goto reject;
		}
		uint64_t grant_expiry = sys_get_be64(wire + 7), revision = sys_get_be64(wire + 145);
		if (zeroes(wire + 113, 32) || revision != current->revision ||
		    grant_expiry <= now || grant_expiry > expiry) goto reject;
		expiry = grant_expiry;
		uint8_t statement[4 + 32 + 8 + NU54_CHAIN_GRANT_SIZE + 32];
		uint8_t *raw_hash = statement + sizeof(statement) - 32;
		memcpy(statement, "N54M", 4);
		memcpy(statement + 4, board, 32);
		sys_put_be64(generation, statement + 36);
		memcpy(statement + 44, wire, NU54_CHAIN_GRANT_SIZE);
		state->raw_size = request->payload_length - NU54_CHAIN_RAW_OFFSET;
		if (sha256(wire + NU54_CHAIN_RAW_OFFSET, state->raw_size, raw_hash) ||
		    device_install_verify_authority(statement, sizeof(statement), wire + NU54_CHAIN_GRANT_SIZE) ||
		    (wire[0] == NU54_CHAIN_BIP48_SIGN ? bip48_review(state, wire, wire + NU54_CHAIN_RAW_OFFSET) :
						       review(state, wire, raw_hash))) goto reject;
		memcpy(state->raw, wire + NU54_CHAIN_RAW_OFFSET, state->raw_size);
	} else if (wire[0] == NU54_CHAIN_BIP48_REGISTER) {
		uint8_t stored[BIP48_POLICY_SIZE];
		nu54_btc48_scope scope;
		if (request->payload_length != BIP48_POLICY_SIZE || bip48_load(state->account.network, stored, &scope) != 1 ||
		    bip48_policy(wire, &scope) || bip48_owned(&state->account, &scope)) goto reject;
		memcpy(state->raw, wire, BIP48_POLICY_SIZE);
	} else if (wire[0] == NU54_CHAIN_XPUB_OP) {
		if ((request->payload_length != 7 && !(request->payload_length == 8 && wire[7] == 48)) ||
		    wire[1] != NU54_CHAIN_BTC || state->account.account > 99) goto reject;
		state->raw_size = request->payload_length == 8 ? 1 : 0;
	} else {
		goto reject;
	}
	if (sha256(wire, request->payload_length, state->display_hash) ||
	    psa_generate_random(state->challenge, sizeof(state->challenge)) != PSA_SUCCESS) goto reject;
	state->pending = true;
	state->operation = wire[0];
	state->prompt = prompt;
	state->generation = generation;
	state->expiry = expiry;
	state->lease_revision = current->revision;
	memcpy(state->request, request->id, sizeof(state->request));
	memcpy(state->epoch, link->epoch, sizeof(state->epoch));
	return 0;
reject:
	device_chains_cancel(state);
	return -1;
}

int device_chains_display(device_chains *state, uint64_t now, const uint8_t proof[64])
{
	if (!state->pending || state->displayed || now >= state->expiry) return -1;
	uint8_t statement[68];
	memcpy(statement, "N54D", 4);
	memcpy(statement + 4, state->challenge, 32);
	memcpy(statement + 36, state->display_hash, 32);
	if (device_install_verify_display(statement, sizeof(statement), proof)) return -1;
	state->displayed = true;
	return 0;
}

static int export_xpub(const device_chains *state, uint8_t *output, size_t *length)
{
	uint8_t root[33], parent[33], root_hash[20], parent_hash[20];
	const bool bip48 = state->raw_size == 1;
	const size_t offset = bip48 ? 1 : 0;
	if (device_chain_btc_root(root)) return -1;
	if (bip48) {
		if (device_chain_bip48_pub(&state->account, output + 48, output + 16) ||
		    device_chain_bip48_parent(&state->account, parent)) return -1;
		output[7] = 48;
	} else if (device_chain_btc_account(&state->account, output + 47, output + 15, parent)) return -1;
	if (hash160(root, root_hash) || hash160(parent, parent_hash)) return -1;
	output[0] = NU54_CHAIN_XPUB_OP;
	output[1] = NU54_CHAIN_BTC;
	output[2] = state->account.network;
	sys_put_be32(state->account.account, output + 3);
	memcpy(output + 7 + offset, root_hash, 4);
	memcpy(output + 11 + offset, parent_hash, 4);
	*length = NU54_CHAIN_XPUB_REPLY_SIZE + offset;
	return 0;
}

int device_chains_complete(device_chains *state, const nu54_v2_link *link, uint64_t now, uint64_t generation,
			    uint32_t prompt, uint8_t result[NU54_CHAIN_REPLY_MAX], size_t *length)
{
	*length = 0;
	const nu54_install_policy *current = device_install_policy();
	if (!current || current->revision != state->lease_revision || !state->pending || !state->displayed || state->prompt != prompt ||
	    now >= state->expiry || state->generation != generation ||
	    memcmp(state->request, link->pending_id, sizeof(state->request)) ||
	    link->closed || memcmp(state->epoch, link->epoch, sizeof(state->epoch))) return -1;
	state->pending = false;
	if (state->operation == NU54_CHAIN_XPUB_OP) return export_xpub(state, result, length);
	if (state->operation == NU54_CHAIN_BIP48_REGISTER) return bip48_register(state, result, length);
	if (state->operation == NU54_CHAIN_BIP48_SIGN) return bip48_finish(state, result, length);
	switch (state->account.chain) {
	case NU54_CHAIN_BTC:
		return nu54_btc_finalize(state->raw, state->raw_size, &state->policy.btc, state->commitment,
					 btc_sign, result, NU54_CHAIN_REPLY_MAX, length);
	case NU54_CHAIN_SOL:
		return nu54_sol_finalize(state->raw, state->raw_size, &state->policy.sol, state->commitment,
					 sol_sign, &state->account, result, NU54_CHAIN_REPLY_MAX, length);
	case NU54_CHAIN_XRPL:
		return nu54_xrpl_finalize(state->raw, state->raw_size, &state->policy.xrpl, state->commitment,
					  xrpl_sign, &state->account, result, NU54_CHAIN_REPLY_MAX, length);
	default:
		return -1;
	}
}

void device_chains_cancel(device_chains *state)
{
	memset(state, 0, sizeof(*state));
}
