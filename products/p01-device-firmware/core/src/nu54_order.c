#include "nu54_order.h"
#include "nu54_keccak.h"
#include "nu54_sig.h"

#include <string.h>

#define ORDER_CHAIN UINT64_C(8283)
#define DOMAIN_TYPE "EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)"
#define RECEIVE_TYPE \
	"ReceiveWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"
#define LOCK_TYPE "OrderLock(bytes32 quoteDigest,address confirmer)"
#define TOKEN_NAME "NU54 Test USD"
#define TOKEN_VERSION "1"
#define ACCOUNT_DOMAIN_TYPE "EIP712Domain(string name,string version,uint256 chainId)"
#define ACCOUNT_NAME "NU54 Wallet Account"
#define ACCOUNT_TYPE "AccountProof(address account,bytes32 challenge)"

static bool recover(const uint8_t digest[32], const uint8_t signature[65], uint8_t address[20])
{
	return nu54_sig_recover(digest, signature, 65, address) == NU54_SIG_OK;
}

static const nu54_v2_quote_crypto crypto = {nu54_keccak256, recover};

static bool zero(const uint8_t *data, size_t length)
{
	uint8_t any = 0;
	for (size_t i = 0; i < length; ++i) any |= data[i];
	return !any;
}

static uint64_t word_u64(const uint8_t *word)
{
	uint64_t value = 0;
	for (size_t i = 24; i < 32; ++i) value = (value << 8) | word[i];
	return value;
}

static void hash_text(const char *text, uint8_t out[32])
{
	nu54_keccak256((const uint8_t *)text, strlen(text), out);
}

static void address_word(const uint8_t address[20], uint8_t word[32])
{
	memset(word, 0, 12);
	memcpy(word + 12, address, 20);
}

static void chain_word(uint8_t word[32])
{
	memset(word, 0, 32);
	word[30] = (uint8_t)(ORDER_CHAIN >> 8);
	word[31] = (uint8_t)ORDER_CHAIN;
}

bool nu54_order_decode(const uint8_t *body, size_t length, const uint8_t **confirmer, const uint8_t **quote)
{
	if (!body || length != NU54_ORDER_REQUEST_SIZE || memcmp(body, "N54O", 4) || zero(body + 4, 20)) return false;
	*confirmer = body + 4;
	*quote = body + 24;
	return true;
}

bool nu54_order_preimage(const uint8_t quote[NU54_V2_UNSIGNED_SIZE], const uint8_t confirmer[20],
			 uint8_t out[NU54_LOCAL_PREIMAGE_SIZE])
{
	uint8_t fields[224], nonce[32];

	hash_text(LOCK_TYPE, fields);
	if (!nu54_v2_quote_digest(quote, NU54_V2_UNSIGNED_SIZE, &crypto, fields + 32)) return false;
	address_word(confirmer, fields + 64);
	nu54_keccak256(fields, 96, nonce);

	hash_text(DOMAIN_TYPE, fields);
	hash_text(TOKEN_NAME, fields + 32);
	hash_text(TOKEN_VERSION, fields + 64);
	memcpy(fields + 96, quote + NU54_V2_CHAIN_ID_OFFSET, 32);
	memcpy(fields + 128, quote + NU54_V2_ASSET_OFFSET, 32);
	out[0] = 0x19;
	out[1] = 1;
	nu54_keccak256(fields, 160, out + 2);

	hash_text(RECEIVE_TYPE, fields);
	memcpy(fields + 32, quote + NU54_V2_PAYER_OFFSET, 32);
	memcpy(fields + 64, quote + NU54_V2_VERIFYING_CONTRACT_OFFSET, 32);
	memcpy(fields + 96, quote + NU54_V2_AMOUNT_OFFSET, 32);
	memset(fields + 128, 0, 32); /* validAfter */
	memcpy(fields + 160, quote + NU54_V2_EXPIRES_AT_OFFSET, 32);
	memcpy(fields + 192, nonce, 32);
	nu54_keccak256(fields, 224, out + 34);
	return true;
}

int nu54_order_stage(nu54_local_approval *approval, const nu54_v2_link *channel, const nu54_v2_request *request,
		     const uint8_t confirmer[20], const uint8_t *quote, const nu54_v2_quote_policy *order,
		     const nu54_pilot_policy *allowlist, const uint8_t payer[20], uint64_t generation, uint64_t earliest,
		     uint32_t physical_prompt, int (*present)(void *, const uint8_t *, size_t), void *context)
{
	if (!channel || channel->closed || channel->local != NU54_V2_BOARD || channel->profile != NU54_V2_MANAGEMENT ||
	    channel->peer != NU54_V2_OWNER || !request || request->command != NU54_V2_PAY_PREPARE ||
	    !channel->pending_expiry || channel->pending_expiry != request->expires ||
	    memcmp(channel->pending_id, request->id, sizeof(request->id)) ||
	    !allowlist || !allowlist->commercial_testnet || !order || !confirmer || !quote || zero(confirmer, 20) ||
	    nu54_pilot_verify_quote(quote, NU54_V2_SIGNED_SIZE, order, allowlist, payer, generation) ||
	    quote[NU54_V2_PAYER_ROLE_OFFSET + 31] != NU54_V2_OWNER_ROLE ||
	    /* The store must not hold the key that confirms receipt for the buyer. */
	    !memcmp(confirmer, order->merchant_signer, 20) || !memcmp(confirmer, allowlist->beneficiary, 20)) return -1;
	uint64_t expiry = word_u64(quote + NU54_V2_EXPIRES_AT_OFFSET);
	if (!earliest || earliest > order->now || expiry - earliest > NU54_ORDER_MAX_SECONDS) return -1;
	nu54_local_intent intent = {.generation = generation, .expires = expiry < request->expires ? expiry : request->expires,
				    .physical_prompt = physical_prompt};
	memcpy(intent.session, channel->epoch, sizeof(intent.session));
	memcpy(intent.request, request->id, sizeof(intent.request));
	if (!nu54_order_preimage(quote, confirmer, intent.preimage)) return -1;
	return nu54_local_approval_stage(approval, &intent, order->now, present, context);
}

int nu54_order_charge(const nu54_order_spending *saved, uint64_t earliest, uint64_t amount,
		      uint64_t daily_cap, nu54_order_spending *next)
{
	if (!saved || !next || !earliest || !amount || !daily_cap) return -1;
	uint64_t day = earliest / 86400u;
	nu54_order_spending state = *saved;
	if (day > state.day) state = (nu54_order_spending){.day = day};
	if (state.spent > daily_cap || amount > daily_cap - state.spent) return -1;
	state.spent += amount;
	*next = state;
	return 0;
}

int nu54_order_find(nu54_order_read read_slot, void *context, uint64_t earliest, const uint8_t request[16],
		    const uint8_t digest[32], nu54_order_record *result, unsigned *free_slot)
{
	*free_slot = 0;
	for (unsigned slot = 1; slot <= NU54_ORDER_RECORDS; ++slot) {
		nu54_order_record saved;
		int read = read_slot(context, slot, &saved);
		if (read > 0) {
			if (!*free_slot) *free_slot = slot;
			continue;
		}
		if (read < 0 || !saved.expires ||
		    (saved.outcome != NU54_ORDER_APPROVED && saved.outcome != NU54_ORDER_REFUSED))
			return NU54_PILOT_RECEIPT_ERROR;
		bool same_request = !memcmp(saved.request, request, sizeof(saved.request));
		bool same_digest = !memcmp(saved.digest, digest, sizeof(saved.digest));
		if (same_request != same_digest && saved.expires > earliest) return NU54_PILOT_RECEIPT_ERROR;
		if (same_request && same_digest) {
			*result = saved;
			return NU54_PILOT_RECEIPT_RECORDED;
		}
		if (saved.expires <= earliest && !*free_slot) *free_slot = slot;
	}
	return *free_slot ? NU54_PILOT_RECEIPT_NEW : NU54_PILOT_RECEIPT_FULL;
}

bool nu54_account_preimage(const uint8_t account[20], const uint8_t challenge[32],
			   uint8_t out[NU54_LOCAL_PREIMAGE_SIZE])
{
	uint8_t fields[128];
	if (!account || !challenge || zero(account, 20) || zero(challenge, 32)) return false;
	hash_text(ACCOUNT_DOMAIN_TYPE, fields);
	hash_text(ACCOUNT_NAME, fields + 32);
	hash_text("1", fields + 64);
	chain_word(fields + 96);
	out[0] = 0x19;
	out[1] = 1;
	nu54_keccak256(fields, 128, out + 2);
	hash_text(ACCOUNT_TYPE, fields);
	address_word(account, fields + 32);
	memcpy(fields + 64, challenge, 32);
	nu54_keccak256(fields, 96, out + 34);
	return true;
}

void nu54_account_statement(const uint8_t account[20], const uint8_t challenge[32],
			    const uint8_t signature[65], uint8_t out[NU54_ACCOUNT_STATEMENT_SIZE])
{
	static const uint8_t domain[24] = "NU54 account address v1";
	memcpy(out, domain, sizeof(domain));
	memcpy(out + 24, challenge, 32);
	memset(out + 56, 0, 6);
	out[62] = (uint8_t)(ORDER_CHAIN >> 8);
	out[63] = (uint8_t)ORDER_CHAIN;
	memcpy(out + 64, account, 20);
	memcpy(out + 84, signature, 65);
}
