#include "nu54_spending.h"
#include "nu54_keccak.h"

#include <string.h>

#define SPEND_CHAIN UINT64_C(8283)
#define TRANSFER_TYPE \
	"TransferWithAuthorization(address from,address to,uint256 value,uint256 validAfter,uint256 validBefore,bytes32 nonce)"

static bool zero(const uint8_t *data, size_t length)
{
	uint8_t any = 0;
	for (size_t i = 0; i < length; ++i) any |= data[i];
	return !any;
}

static uint64_t get64(const uint8_t *in)
{
	uint64_t value = 0;
	for (size_t i = 0; i < 8; ++i) value = (value << 8) | in[i];
	return value;
}

static void put64(uint8_t *out, uint64_t value)
{
	for (size_t i = 8; i; --i) {
		out[i - 1] = (uint8_t)value;
		value >>= 8;
	}
}

static void word64(uint8_t word[32], uint64_t value)
{
	memset(word, 0, 24);
	put64(word + 24, value);
}

static void word_address(uint8_t word[32], const uint8_t address[20])
{
	memset(word, 0, 12);
	memcpy(word + 12, address, 20);
}

static void hash_text(const char *text, uint8_t out[32])
{
	nu54_keccak256((const uint8_t *)text, strlen(text), out);
}

int nu54_spend_stage(nu54_spend_action *action, const nu54_v2_link *link, const nu54_v2_request *request,
		     uint64_t now, uint64_t earliest, uint64_t generation, uint32_t prompt)
{
	if (!action || action->pending || !link || link->closed || link->local != NU54_V2_BOARD ||
	    link->profile != NU54_V2_MANAGEMENT || link->peer != NU54_V2_OWNER || !request ||
	    request->command != NU54_V2_MANAGE || !request->payload || !request->payload_length || !generation ||
	    !prompt || !now || request->expires <= now || link->pending_expiry != request->expires ||
	    memcmp(link->pending_id, request->id, sizeof(request->id))) return -1;
	nu54_spend_action next = {.op = request->payload[0], .generation = generation, .expires = request->expires,
				  .prompt = prompt, .pending = true};
	const uint8_t *body = request->payload + 1;
	switch (next.op) {
	case NU54_SPEND_SET_RETURN_OP:
		if (request->payload_length != 21 || zero(body, 20)) return -1;
		memcpy(next.address, body, 20);
		break;
	case NU54_SPEND_RETURN_OP:
		if (request->payload_length != 49) return -1;
		next.amount = get64(body);
		next.valid_before = get64(body + 8);
		memcpy(next.nonce, body + 16, 32);
		if (!next.amount || zero(next.nonce, 32) || !earliest || next.valid_before <= now ||
		    next.valid_before - earliest > NU54_SPEND_RETURN_MAX_SECONDS) return -1;
		break;
	case NU54_SPEND_RESET_OP:
		if (request->payload_length != 1) return -1;
		break;
	case NU54_SPEND_OPERATOR_RESET_OP:
		if (request->payload_length != 97 || zero(body, 32)) return -1;
		memcpy(next.nonce, body, 32);
		break;
	default:
		return -1;
	}
	memcpy(next.session, link->epoch, sizeof(next.session));
	memcpy(next.request, request->id, sizeof(next.request));
	*action = next;
	return 0;
}

bool nu54_spend_consume(nu54_spend_action *action, const nu54_v2_link *link, uint64_t now, uint64_t generation,
			uint32_t prompt, nu54_spend_action *out)
{
	nu54_spend_action current = *action;
	memset(action, 0, sizeof(*action));
	if (!current.pending || link->closed || link->profile != NU54_V2_MANAGEMENT || link->peer != NU54_V2_OWNER ||
	    !now || now >= current.expires || current.prompt != prompt || current.generation != generation ||
	    link->pending_expiry != current.expires || memcmp(link->epoch, current.session, sizeof(current.session)) ||
	    memcmp(link->pending_id, current.request, sizeof(current.request)) ||
	    (current.op == NU54_SPEND_RETURN_OP && now >= current.valid_before)) return false;
	*out = current;
	return true;
}

void nu54_spend_return_preimage(const uint8_t token[20], const uint8_t from[20], const uint8_t to[20],
				uint64_t amount, uint64_t valid_before, const uint8_t nonce[32],
				uint8_t out[NU54_LOCAL_PREIMAGE_SIZE])
{
	uint8_t fields[224];
	hash_text("EIP712Domain(string name,string version,uint256 chainId,address verifyingContract)", fields);
	hash_text("NU54 Test USD", fields + 32);
	hash_text("1", fields + 64);
	word64(fields + 96, SPEND_CHAIN);
	word_address(fields + 128, token);
	out[0] = 0x19;
	out[1] = 1;
	nu54_keccak256(fields, 160, out + 2);
	hash_text(TRANSFER_TYPE, fields);
	word_address(fields + 32, from);
	word_address(fields + 64, to);
	word64(fields + 96, amount);
	word64(fields + 128, 0);
	word64(fields + 160, valid_before);
	memcpy(fields + 192, nonce, 32);
	nu54_keccak256(fields, 224, out + 34);
}

_Static_assert(NU54_SPEND_ORDER_LIFE_MS > NU54_ORDER_MAX_SECONDS * INT64_C(1000), "an order outlives the boot wait");

bool nu54_spend_order_open(nu54_order_read read_slot, void *context, uint64_t earliest, int64_t uptime_ms)
{
	const bool young = uptime_ms < NU54_SPEND_ORDER_LIFE_MS;
	for (unsigned slot = 1; slot <= NU54_ORDER_RECORDS; ++slot) {
		nu54_order_record saved;
		int read = read_slot(context, slot, &saved);
		if (read > 0) continue;
		if (read < 0) {
			if (young) return true;
			continue;
		}
		if (saved.outcome == NU54_ORDER_APPROVED && (earliest ? saved.expires > earliest : young)) return true;
	}
	return false;
}

void nu54_spend_operator_statement(const uint8_t board[32], uint64_t generation, const uint8_t nonce[32],
				   uint8_t out[NU54_SPEND_OPERATOR_STATEMENT_SIZE])
{
	static const uint8_t domain[24] = "NU54 operator reset v1";
	memcpy(out, domain, sizeof(domain));
	memcpy(out + 24, board, 32);
	put64(out + 56, generation);
	memcpy(out + 64, nonce, 32);
}

int nu54_spend_status_account(const uint8_t *account, bool owner_absent, uint8_t out[20])
{
	if (!out || (!account && !owner_absent)) return -1;
	if (account) memcpy(out, account, 20);
	else memset(out, 0, 20);
	return 0;
}
