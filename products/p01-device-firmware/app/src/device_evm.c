#include "device_evm.h"

#include "device_hd.h"
#include "device_install.h"
#include "nu54_sig.h"
#include "nu54_siwe.h"
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA)
#include "device_owner_limits_ps.h"
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY)
#include "device_owner_finality_ps.h"
#endif
#endif

#include <psa/crypto.h>
#include <string.h>
#include <zephyr/sys/byteorder.h>

#define EXTERNAL_CONTEXT_IDS 96
#define EXTERNAL_CONTEXT_PREFIX (EXTERNAL_CONTEXT_IDS + 1)
#define EXTERNAL_ORIGIN_MAX 104
enum {
	ALLOWANCE_SPENDER = 1,
	ALLOWANCE_MAXIMUM = ALLOWANCE_SPENDER + 20,
	ALLOWANCE_DECIMALS = ALLOWANCE_MAXIMUM + 32,
	ALLOWANCE_POLICY_SIZE = ALLOWANCE_DECIMALS + 1,
};

static int external_context(const uint8_t *context, size_t size, bool browser, size_t *context_size)
{
	if (size <= (size_t)EXTERNAL_CONTEXT_PREFIX ||
	    context[EXTERNAL_CONTEXT_IDS] < 9 || context[EXTERNAL_CONTEXT_IDS] > EXTERNAL_ORIGIN_MAX ||
	    size <= (size_t)EXTERNAL_CONTEXT_PREFIX + context[EXTERNAL_CONTEXT_IDS] ||
	    (!browser && !memcmp(context, context + 32, 32))) return -1;
	for (size_t i = 0; i < EXTERNAL_CONTEXT_IDS; i += 32) {
		uint8_t nonzero = 0;
		for (size_t j = 0; j < 32; j++) nonzero |= context[i + j];
		if (!!nonzero != (!browser || i == 0)) return -1;
	}
	const uint8_t *origin = context + EXTERNAL_CONTEXT_PREFIX;
	if (memcmp(origin, "https://", 8)) return -1;
	for (size_t i = 8; i < context[EXTERNAL_CONTEXT_IDS]; i++) {
		uint8_t c = origin[i];
		if (!((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == ':'))
			return -1;
	}
	*context_size = EXTERNAL_CONTEXT_PREFIX + context[EXTERNAL_CONTEXT_IDS];
	return 0;
}

static int allowance_context(const uint8_t *context, size_t size, size_t *context_size,
			     nu54_evm_policy *policy)
{
	static const uint8_t zero[32] = {0};
	if (size <= ALLOWANCE_POLICY_SIZE || !memcmp(policy->token, zero, 20) || context[ALLOWANCE_DECIMALS] > 24)
		return -1;
	if (context[0] > 1 || external_context(context + ALLOWANCE_POLICY_SIZE,
	    size - ALLOWANCE_POLICY_SIZE, context[0] == 0, context_size)) return -1;
	*context_size += ALLOWANCE_POLICY_SIZE;
	memcpy(policy->allowance_spender, context + ALLOWANCE_SPENDER, 20);
	memcpy(policy->allowance_maximum, context + ALLOWANCE_MAXIMUM, 32);
	if (!memcmp(policy->allowance_spender, zero, 20) || !memcmp(policy->allowance_maximum, zero, 32))
		return -1;
	return 0;
}

int device_evm_stage(device_evm *state, const nu54_v2_request *request, const nu54_v2_link *link,
		     uint64_t now, uint64_t generation, uint32_t prompt, const uint8_t board[32], const uint8_t address[20])
{
	if (state->pending || !generation || !prompt || link->closed || link->local != NU54_V2_BOARD ||
	    link->profile != NU54_V2_MANAGEMENT || link->peer != NU54_V2_OWNER ||
	    request->payload_length <= NU54_DIRECT_RAW_OFFSET || request->payload_length > sizeof(state->wire))
		return -1;
	const uint8_t *bytes = request->payload;
	const uint8_t operation = bytes[0];
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY)
	/* Reservation and reconciliation in this profile apply to transfers. */
	if (operation == NU54_EVM_DAPP_ALLOWANCE_OP) return -1;
#endif
	const nu54_install_policy *current = device_install_policy();
	uint64_t expires = sys_get_be64(bytes + 1), revision = sys_get_be64(bytes + 93);
	static const uint8_t zero_genesis[32] = {0};
	if (!current || (operation != NU54_EVM_TRANSFER_OP && operation != NU54_EVM_REVOKE_OP &&
	    operation != NU54_EVM_EXTERNAL_OP && operation != NU54_EVM_DAPP_ALLOWANCE_OP) ||
	    expires <= now || expires > request->expires || expires > current->not_after || !memcmp(bytes + 53, zero_genesis, sizeof(zero_genesis)) ||
	    sys_get_be64(bytes + 85) != 1 || revision != current->revision) return -1;
	nu54_evm_policy policy = {.chain = sys_get_be64(bytes + 9),
		.total_fee_limit = sys_get_be64(bytes + 17), .l1_fee_bound = sys_get_be64(bytes + 25),
		.revoke_allowance = operation == NU54_EVM_REVOKE_OP};
	memcpy(policy.token, bytes + 33, 20);
	size_t context_size = 0;
	if (operation == NU54_EVM_EXTERNAL_OP) {
		static const uint8_t native_token[20] = {0};
		if (memcmp(policy.token, native_token, sizeof(native_token)) ||
		    external_context(bytes + NU54_DIRECT_RAW_OFFSET, request->payload_length - NU54_DIRECT_RAW_OFFSET, false, &context_size))
			return -1;
	} else if (operation == NU54_EVM_DAPP_ALLOWANCE_OP &&
		   (expires - now > 120 || allowance_context(bytes + NU54_DIRECT_RAW_OFFSET,
		    request->payload_length - NU54_DIRECT_RAW_OFFSET, &context_size, &policy))) return -1;
	size_t raw_offset = NU54_DIRECT_RAW_OFFSET + context_size;
	nu54_evm_intent intent;
	if (nu54_evm_decode(bytes + raw_offset, request->payload_length - raw_offset, &policy, &intent))
		return -1;
	enum { fixed = 4 + 32 + 8 + 20 + NU54_DIRECT_PROOF_HEADER + 32 };
	uint8_t statement[fixed + EXTERNAL_CONTEXT_PREFIX + EXTERNAL_ORIGIN_MAX + ALLOWANCE_POLICY_SIZE];
	memcpy(statement, "N54V", 4);
	memcpy(statement + 4, board, 32);
	sys_put_be64(generation, statement + 36);
	memcpy(statement + 44, address, 20);
	memcpy(statement + 64, bytes, NU54_DIRECT_PROOF_HEADER);
	memcpy(statement + 64 + NU54_DIRECT_PROOF_HEADER, intent.digest, 32);
	memcpy(statement + fixed, bytes + NU54_DIRECT_RAW_OFFSET, context_size);
	if (device_install_verify_authority(statement, fixed + context_size, bytes + NU54_DIRECT_PROOF_HEADER) ||
	    psa_generate_random(state->challenge, sizeof(state->challenge)) != PSA_SUCCESS) return -1;
	state->pending = true;
	state->displayed = false;
	state->prompt = prompt;
	state->generation = generation;
	state->expiry = expires;
	state->lease_revision = revision;
	state->policy = policy;
	state->raw_offset = raw_offset;
	state->login = false;
	state->market = false;
	state->intent = intent;
	state->length = request->payload_length;
	memcpy(state->address, address, 20);
	memcpy(state->board, board, sizeof(state->board));
	memcpy(state->request, request->id, sizeof(state->request));
	memcpy(state->epoch, link->epoch, sizeof(state->epoch));
	memcpy(state->wire, bytes, state->length);
	return 0;
}

enum { MARKET_HASHES = 17, MARKET_SCOPE = 81, MARKET_TARGET = 193, MARKET_NONCE = 213,
       MARKET_FEE = 221, MARKET_MODE = 229, MARKET_QUOTE = 230, MARKET_ALLOWANCE = 682,
       MARKET_PROOF = 64, MARKET_APPROVE_LENGTH = 68 };

static int review_market(const uint8_t *wire, size_t length, uint64_t now, const uint8_t user[20],lab_reviewed *review)
{
	if (length < MARKET_ALLOWANCE+MARKET_PROOF || length > MARKET_ALLOWANCE+LAB_BOUND_APPROVE_LENGTH+MARKET_PROOF) return -1;
	uint8_t mode=wire[MARKET_MODE];
	size_t allowance = mode==1 ? MARKET_APPROVE_LENGTH : mode==2 ? LAB_BOUND_APPROVE_LENGTH : 0;
	uint8_t genesis=0,deployment=0;
	for(size_t i=0;i<32;i++) {genesis|=wire[MARKET_HASHES+i];deployment|=wire[MARKET_HASHES+32+i];}
	if (wire[0]!=NU54_EVM_MARKET_OP || !genesis || !deployment || mode>2 ||
	    length!=MARKET_ALLOWANCE+MARKET_PROOF+allowance) return -1;
	const uint8_t *quote=wire+MARKET_QUOTE;
	if (sys_get_be64(quote+284)<sys_get_be64(wire+1)) return -1;
	return lab_review(user,wire+MARKET_SCOPE,wire+MARKET_TARGET,sys_get_be64(wire+MARKET_NONCE),
		sys_get_be64(wire+MARKET_FEE),now,allowance?wire+MARKET_ALLOWANCE:quote,
		allowance?allowance:LAB_QUOTE_LENGTH,quote,review) ? 0 : -1;
}

int device_evm_stage_market(device_evm *state, const nu54_v2_request *request, const nu54_v2_link *link,
		     uint64_t now, uint64_t generation, uint32_t prompt, const uint8_t board[32], const uint8_t address[20])
{
	const nu54_install_policy *current=device_install_policy();
	if (!current || state->pending || !generation || !prompt || link->closed || link->local!=NU54_V2_BOARD ||
	    link->profile!=NU54_V2_MANAGEMENT || link->peer!=NU54_V2_OWNER ||
	    request->payload_length<MARKET_ALLOWANCE+MARKET_PROOF || request->payload_length>sizeof(state->wire)) return -1;
	const uint8_t *wire=request->payload;
	uint64_t expires=sys_get_be64(wire+1), revision=sys_get_be64(wire+9);
	lab_reviewed reviewed;
	if (expires<=now || expires-now>60 || expires>request->expires || expires>current->not_after ||
	    revision!=current->revision || review_market(wire,request->payload_length,now,address,&reviewed)) return -1;

	uint8_t statement[64+MARKET_ALLOWANCE+LAB_BOUND_APPROVE_LENGTH];
	size_t signed_length=request->payload_length-MARKET_PROOF;
	memcpy(statement,"N54J",4);
	memcpy(statement+4,board,32);
	sys_put_be64(generation,statement+36);
	memcpy(statement+44,address,20);
	memcpy(statement+64,wire,signed_length);
	if (device_install_verify_authority(statement,64+signed_length,wire+signed_length) ||
	    psa_generate_random(state->challenge,sizeof(state->challenge))!=PSA_SUCCESS) return -1;
	state->pending=true;
	state->market=true;
	state->login=false;
	state->displayed=false;
	state->prompt=prompt;
	state->generation=generation;
	state->expiry=expires;
	state->lease_revision=revision;
	state->length=request->payload_length;
	memcpy(state->address,address,20);
	memcpy(state->request,request->id,sizeof(state->request));
	memcpy(state->epoch,link->epoch,sizeof(state->epoch));
	memcpy(state->wire,wire,state->length);
	memcpy(state->intent.digest,reviewed.digest,32);
	return 0;
}

int device_evm_stage_login(device_evm *state, const nu54_v2_request *request, const nu54_v2_link *link,
		     uint64_t now, uint64_t generation, uint32_t prompt, const uint8_t board[32], const uint8_t address[20])
{
	const size_t header_size = 58;
	const nu54_install_policy *lease = device_install_policy();
	if (!lease || state->pending || !generation || !prompt || link->closed || link->local != NU54_V2_BOARD ||
	    link->profile != NU54_V2_MANAGEMENT || link->peer != NU54_V2_OWNER ||
	    request->payload_length <= header_size + 66 || request->payload_length > sizeof(state->wire)) return -1;
	const uint8_t *wire = request->payload;
	uint64_t expires = sys_get_be64(wire + 1), revision = sys_get_be64(wire + 49);
	size_t hostname_length = wire[57];
	uint8_t genesis = 0;
	for (size_t i = 17; i < 49; i++) genesis |= wire[i];
	if (wire[0] != NU54_EVM_SIWE_OP || !genesis || !hostname_length || hostname_length > 96 ||
	    request->payload_length < header_size + hostname_length + 66 ||
	    expires > request->expires || expires > lease->not_after || revision != lease->revision) return -1;
	size_t length_at = header_size + hostname_length;
	size_t message_length = sys_get_be16(wire + length_at), signed_length = length_at + 2 + message_length;
	if (request->payload_length != signed_length + 64 ||
	    nu54_siwe_digest(wire + length_at + 2, message_length, wire + header_size, hostname_length, address,
			    sys_get_be64(wire + 9), now, expires, state->intent.digest)) return -1;
	uint8_t statement[64 + sizeof(state->wire)];
	memcpy(statement, "N54S", 4);
	memcpy(statement + 4, board, 32);
	sys_put_be64(generation, statement + 36);
	memcpy(statement + 44, address, 20);
	memcpy(statement + 64, wire, signed_length);
	if (device_install_verify_authority(statement, 64 + signed_length, wire + signed_length) ||
	    psa_generate_random(state->challenge, sizeof(state->challenge)) != PSA_SUCCESS) return -1;
	state->pending = true;
	state->login = true;
	state->market = false;
	state->displayed = false;
	state->prompt = prompt;
	state->generation = generation;
	state->expiry = expires;
	state->lease_revision = revision;
	state->length = request->payload_length;
	memcpy(state->address, address, 20);
	memcpy(state->request, request->id, sizeof(state->request));
	memcpy(state->epoch, link->epoch, sizeof(state->epoch));
	memcpy(state->wire, wire, state->length);
	return 0;
}

int device_evm_display(device_evm *state, uint64_t now, const uint8_t proof[64])
{
	if (!state->pending || state->displayed || now >= state->expiry) return -1;
	uint8_t message[4 + 32 + sizeof(state->wire)];
	memcpy(message, state->market ? "N54Z" : state->login ? "N54I" : "N54X", 4);
	memcpy(message + 4, state->challenge, sizeof(state->challenge));
	memcpy(message + 36, state->wire, state->length);
	if (device_install_verify_display(message, 36 + state->length, proof)) return -1;
	state->displayed = true;
	return 0;
}

int device_evm_complete(device_evm *state, const nu54_v2_link *link, uint64_t now, uint64_t generation,
			uint32_t prompt, const uint8_t address[20], uint8_t signed_tx[LAB_TX_LENGTH], size_t *length)
{
	const nu54_install_policy *current = device_install_policy();
	if (!current || current->revision != state->lease_revision || !state->pending || !state->displayed || state->prompt != prompt ||
	    now >= state->expiry || state->generation != generation || memcmp(state->address, address, 20) ||
	    memcmp(state->request, link->pending_id, sizeof(state->request)) ||
	    link->closed || memcmp(state->epoch, link->epoch, sizeof(state->epoch))) return -1;
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_LIMITS_PSA)
	/* Login, allowance revocation and isolated market accounts have separate
	 * authority. All ordinary direct transfers fail closed until an owner policy
	 * is provisioned; reserve durably before creating any signature. */
	if (!state->market && !state->login && !state->policy.revoke_allowance) {
#if defined(CONFIG_NU54_EXPERIMENTAL_OWNER_FINALITY_ONLY)
		nu54_limit_request limited={.generation=generation,.chain=state->intent.chain,.expires=state->expiry};
		for(size_t i=0;i<24;i++)if(state->intent.amount[i])return -1;
		for(size_t i=24;i<32;i++)limited.amount=(limited.amount<<8)|state->intent.amount[i];
		memcpy(limited.board,state->board,32);memcpy(limited.asset,state->intent.token,20);
		memcpy(limited.destination,state->intent.recipient,20);memcpy(limited.id,state->request,16);
		memcpy(limited.digest,state->intent.digest,32);
		if(state->raw_offset>state->length || device_owner_finality_reserve_ps(&limited,address,
		  state->wire+state->raw_offset,state->length-state->raw_offset,device_install_earliest(),device_install_now()))return -1;
#else
		if(device_owner_limits_authorize_ps(state->board,generation,state->intent.chain,state->intent.token,
		  state->intent.recipient,state->intent.amount,state->request,state->intent.digest,
		  state->expiry,device_install_earliest(),device_install_now()))return -1;
#endif
	}
#endif
	state->pending = false;
	lab_reviewed reviewed;
	if (state->market && (review_market(state->wire,state->length,now,address,&reviewed) ||
	    memcmp(reviewed.digest,state->intent.digest,32))) return -1;
	uint8_t raw[64], signature[65];
	if ((state->market ? device_hd_market_sign(address, state->intent.digest, raw) :
	                     device_hd_sign(address, state->intent.digest, raw)) ||
	    nu54_sig_finish(state->intent.digest, raw, address, signature) != NU54_SIG_OK) return -1;
	if (state->market) {
		signature[64]-=27;
		return lab_serialize(&reviewed,signature,signed_tx,length) ? 0 : -1;
	}
	if (state->login) {
		memcpy(signed_tx, signature, sizeof(signature));
		*length = sizeof(signature);
		return 0;
	}
	return nu54_evm_serialize(state->wire + state->raw_offset, state->length - state->raw_offset,
				 &state->policy, signature, signed_tx, length);
}

void device_evm_cancel(device_evm *state)
{
	memset(state, 0, sizeof(*state));
}
