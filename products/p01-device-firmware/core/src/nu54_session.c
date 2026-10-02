#include "nu54_session.h"

#include <stdio.h>
#include <string.h>

#include "nu54_cbor.h"
#include "nu54_sig.h"

static const char *const STATE_NAME[] = {"UNPROVISIONED", "PROVISIONED_NO_ANCHOR", "READY", "PIN_LOCKED"};
static const uint8_t ZERO_SESSION[8] = {0};

/* ---------------------------------------------------------------- helpers */

#define TXT(s) (const uint8_t *)(s), strlen(s)

static nu54_cbor_entry_t text(const char *key, const char *s)
{
	return (nu54_cbor_entry_t){key, NU54_V_TEXT, (const uint8_t *)s, strlen(s), 0};
}

static nu54_cbor_entry_t bytes(const char *key, const uint8_t *p, size_t n)
{
	return (nu54_cbor_entry_t){key, NU54_V_BYTES, p, n, 0};
}

/* Writes one body; `who` 0 = central, 1 = phone. */
static void emit(nu54_out_t *out, int who, const nu54_cbor_entry_t *e, size_t n)
{
	nu54_cbor_writer_t w;
	if (who == 0) {
		if (out->kiosk_count >= 2) {
			return;
		}
		nu54_cbor_writer_init(&w, out->kiosk[out->kiosk_count], NU54_OUT_MAX);
		if (nu54_cbor_write_map(&w, e, n) == 0) {
			out->kiosk_len[out->kiosk_count++] = w.len;
		}
	} else {
		nu54_cbor_writer_init(&w, out->phone, NU54_OUT_MAX);
		if (nu54_cbor_write_map(&w, e, n) == 0) {
			out->phone_len = w.len;
			out->phone_count = 1;
		}
	}
}

static void error_reply(nu54_out_t *out, const uint8_t sid[8], const char *reason)
{
	nu54_cbor_entry_t e[] = {{"v", NU54_V_UINT, 0, 0, 1}, text("type", "error"), bytes("sessionId", sid, 8), text("reason", reason)};
	emit(out, 0, e, 4);
}

static const uint8_t *reply_sid(const nu54_device_t *d)
{
	return d->session_open ? d->session_id : ZERO_SESSION;
}

static void refused(const nu54_device_t *d, nu54_out_t *out, const char *reason)
{
	nu54_cbor_entry_t e[] = {{"v", NU54_V_UINT, 0, 0, 1}, text("type", "payment.result"), bytes("sessionId", reply_sid(d), 8),
				 text("outcome", "refused"), text("reason", reason)};
	emit(out, 0, e, 5);
}

/* 32-byte big-endian value of a uint item (integer or tag-2 bignum). */
static void u256(const nu54_cbor_item_t *it, uint8_t out[32])
{
	memset(out, 0, 32);
	if (it->type == NU54_CBOR_UINT) {
		for (int k = 0; k < 8; k++) {
			out[31 - k] = (uint8_t)(it->value >> (8 * k));
		}
	} else {
		memcpy(&out[32 - it->len], it->ptr, it->len);
	}
}

/* uint item as uint64; a bignum does not fit and reads as UINT64_MAX. */
static uint64_t u64(const nu54_cbor_item_t *it)
{
	return it->type == NU54_CBOR_UINT ? it->value : UINT64_MAX;
}

static const nu54_cbor_item_t *field(const nu54_msg_t *m, int map, const char *key)
{
	int i = nu54_msg_find(m, map, key);
	return i < 0 ? NULL : &m->items[i];
}

static int same_session(const nu54_device_t *d, const nu54_msg_t *m)
{
	const nu54_cbor_item_t *s = field(m, 0, "sessionId");
	return d->session_open && memcmp(s->ptr, d->session_id, 8) == 0;
}

static int in_session(const nu54_device_t *d, const nu54_msg_t *m, int setup)
{
	return same_session(d, m) && d->session_setup == setup && (setup || d->confirmed);
}

static int device_time(const nu54_device_t *d, uint64_t now, uint64_t *t)
{
	if (!d->anchored) {
		return 0;
	}
	*t = d->anchor_timestamp + (now - d->anchor_at);
	return 1;
}

static void increment(uint8_t n[32])
{
	for (int i = 31; i >= 0 && ++n[i] == 0; i--) {
	}
}

/* A setup that did not reach the PIN leaves nothing behind: drop the key it made. */
static void abort_pending(nu54_device_t *d)
{
	if (d->pending == NU54_PENDING_PIN && d->platform.wipe) {
		d->platform.wipe(d->platform.ctx);
	}
	d->pending = NU54_PENDING_NONE;
}

static void last_anchor_u256(const nu54_device_t *d, uint8_t last[32])
{
	memset(last, 0, 32);
	for (int k = 0; k < 8; k++) {
		last[31 - k] = (uint8_t)(d->last_anchor >> (8 * k));
	}
}

/* setup.ack{step, accepted[, reason][, device]}. */
static void setup_ack(const nu54_device_t *d, nu54_out_t *out, const char *step, int accepted, const char *reason, const uint8_t *device)
{
	nu54_cbor_entry_t e[6] = {{"v", NU54_V_UINT, 0, 0, 1}, text("type", "setup.ack"), bytes("sessionId", reply_sid(d), 8),
				  text("step", step), {"accepted", NU54_V_BOOL, 0, 0, (uint64_t)accepted}};
	size_t n = 5;
	if (reason) {
		e[n++] = text("reason", reason);
	}
	if (device) {
		e[n++] = bytes("device", device, 20);
	}
	emit(out, 0, e, n);
}

/* ---------------------------------------------------------------- lifecycle */

void nu54_device_init(nu54_device_t *d, const uint8_t nonce_start[32])
{
	d->state = NU54_STATE_PROVISIONED_NO_ANCHOR;
	d->anchored = 0;
	d->last_anchor = 0;
	memcpy(d->next_nonce, nonce_start, 32);
	d->session_open = 0;
	d->attested = 0;
	d->pending = NU54_PENDING_NONE;
}

/* Clears the key, setup values and anchor in RAM (the platform clears its storage). */
static void forget_setup(nu54_device_t *d)
{
	memset(d->address, 0, sizeof(d->address));
	memset(d->operator_address, 0, sizeof(d->operator_address));
	memset(d->contract, 0, sizeof(d->contract));
	memset(d->chain_id, 0, sizeof(d->chain_id));
	memset(d->next_nonce, 0, sizeof(d->next_nonce));
	d->passkey = 0;
	d->state = NU54_STATE_UNPROVISIONED;
	d->anchored = 0;
	d->last_anchor = 0;
	d->session_open = 0;
	d->attested = 0;
	d->pending = NU54_PENDING_NONE;
}

void nu54_device_init_unprovisioned(nu54_device_t *d)
{
	forget_setup(d);
}

void nu54_device_power_cycle(nu54_device_t *d)
{
	d->anchored = 0;
	d->session_open = 0;
	abort_pending(d);
	if (d->state == NU54_STATE_READY) {
		d->state = NU54_STATE_PROVISIONED_NO_ANCHOR;
	}
}

/* ---------------------------------------------------------------- messages */

static void session_open(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out)
{
	const nu54_cbor_item_t *sid = field(m, 0, "sessionId");
	const nu54_cbor_item_t *mode = field(m, 0, "mode");
	int setup = mode->len == 5 && memcmp(mode->ptr, "setup", 5) == 0;
	if (setup && d->state != NU54_STATE_UNPROVISIONED && d->state != NU54_STATE_PROVISIONED_NO_ANCHOR) {
		error_reply(out, sid->ptr, "NOT_PERMITTED");
		return;
	}
	abort_pending(d);
	d->platform.random(d->platform.ctx, d->device_nonce, 32);
	memcpy(d->session_id, sid->ptr, 8);
	d->session_open = 1;
	d->session_setup = setup;
	d->confirmed = 0;
	d->attested = 0;
	uint8_t last[32];
	last_anchor_u256(d, last);
	nu54_cbor_entry_t e[] = {
		{"v", NU54_V_UINT, 0, 0, 1},
		text("type", "session.open.ok"),
		bytes("sessionId", d->session_id, 8),
		bytes("deviceNonce", d->device_nonce, 32),
		{"anchorValid", NU54_V_BOOL, 0, 0, (uint64_t)d->anchored},
		text("firmware", d->firmware),
		text("state", STATE_NAME[d->state]),
		{"lastAnchor", NU54_V_UINT256, last, 32, 0},
		bytes("device", d->address, 20), /* an UNPROVISIONED device has no key yet */
	};
	emit(out, 0, e, d->state == NU54_STATE_UNPROVISIONED ? 8 : 9);
}

static void session_confirm(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out)
{
	const nu54_cbor_item_t *dn = field(m, 0, "deviceNonce");
	if (!same_session(d, m) || memcmp(dn->ptr, d->device_nonce, 32) != 0) {
		d->session_open = 0;
		error_reply(out, field(m, 0, "sessionId")->ptr, "NOT_PERMITTED");
		return;
	}
	d->confirmed = 1;
}

static void time_anchor(nu54_device_t *d, const nu54_msg_t *m, uint64_t now, nu54_out_t *out)
{
	if (!in_session(d, m, 1)) {
		error_reply(out, field(m, 0, "sessionId")->ptr, "NOT_PERMITTED");
		return;
	}
	if (d->state == NU54_STATE_UNPROVISIONED) {
		uint8_t last[32];
		last_anchor_u256(d, last);
		nu54_cbor_entry_t e[] = {{"v", NU54_V_UINT, 0, 0, 1}, text("type", "setup.ack"), bytes("sessionId", reply_sid(d), 8),
					 text("step", "setup.timeAnchor"), {"accepted", NU54_V_BOOL, 0, 0, 0},
					 {"lastAnchor", NU54_V_UINT256, last, 32, 0}, text("reason", "NOT_PERMITTED")};
		emit(out, 0, e, 7); /* no key and no operator to check against */
		return;
	}
	const nu54_cbor_item_t *dev = field(m, 0, "device"), *ts = field(m, 0, "timestamp"), *sig = field(m, 0, "operatorSignature");
	nu54_time_anchor_t a;
	memcpy(a.device, dev->ptr, 20);
	a.timestamp = u64(ts);
	uint8_t domain[32], h[32], digest[32], signer[20];
	nu54_eip712_domain_separator(d->chain_id, d->contract, domain);
	nu54_hash_time_anchor(&a, h);
	nu54_eip712_digest(domain, h, digest);
	int ok = ts->type == NU54_CBOR_UINT && nu54_sig_recover(digest, sig->ptr, sig->len, signer) == NU54_SIG_OK &&
		 memcmp(dev->ptr, d->address, 20) == 0 && memcmp(signer, d->operator_address, 20) == 0 && a.timestamp > d->last_anchor;
	if (ok) {
		d->anchored = 1;
		d->anchor_timestamp = a.timestamp;
		d->anchor_at = now;
		d->last_anchor = a.timestamp;
		if (d->state == NU54_STATE_PROVISIONED_NO_ANCHOR) {
			d->state = NU54_STATE_READY;
		}
	}
	uint8_t last[32];
	last_anchor_u256(d, last);
	nu54_cbor_entry_t e[] = {
		{"v", NU54_V_UINT, 0, 0, 1},
		text("type", "setup.ack"),
		bytes("sessionId", reply_sid(d), 8),
		text("step", "setup.timeAnchor"),
		{"accepted", NU54_V_BOOL, 0, 0, (uint64_t)ok},
		{"lastAnchor", NU54_V_UINT256, last, 32, 0},
		text("reason", "NOT_PERMITTED"), /* refused anchors only */
	};
	emit(out, 0, e, ok ? 6 : 7);
}

static void identify(nu54_device_t *d, const nu54_msg_t *m, uint64_t now, nu54_out_t *out)
{
	uint64_t t;
	if (!in_session(d, m, 0)) {
		error_reply(out, field(m, 0, "sessionId")->ptr, "NOT_PERMITTED");
		return;
	}
	if (!device_time(d, now, &t)) {
		refused(d, out, "TIME_ANCHOR_MISSING");
		return;
	}
	int a = nu54_msg_find(m, 0, "attestation");
	const nu54_cbor_item_t *merchant = field(m, a, "merchant"), *payout = field(m, a, "payout"), *name = field(m, a, "name"),
			       *from = field(m, a, "validFrom"), *until = field(m, a, "validUntil"), *sig = field(m, a, "operatorSignature");
	nu54_merchant_attestation_t att;
	memcpy(att.merchant, merchant->ptr, 20);
	memcpy(att.payout, payout->ptr, 20);
	att.name = name->ptr;
	att.name_len = name->len;
	att.valid_from = u64(from);
	att.valid_until = u64(until);
	uint8_t domain[32], h[32], digest[32], signer[20];
	nu54_eip712_domain_separator(d->chain_id, d->contract, domain);
	nu54_hash_merchant_attestation(&att, h);
	nu54_eip712_digest(domain, h, digest);
	if (from->type != NU54_CBOR_UINT || until->type != NU54_CBOR_UINT ||
	    nu54_sig_recover(digest, sig->ptr, sig->len, signer) != NU54_SIG_OK || memcmp(signer, d->operator_address, 20) != 0) {
		refused(d, out, "MERCHANT_FORGED");
		return;
	}
	uint64_t skew = d->anchor_clock_skew;
	if (t + skew < att.valid_from || (t > skew && t - skew > att.valid_until) || name->len >= sizeof(d->att_name)) {
		refused(d, out, "ATTESTATION_EXPIRED");
		return;
	}
	memcpy(d->att_merchant, merchant->ptr, 20);
	memcpy(d->att_payout, payout->ptr, 20);
	memcpy(d->att_name, name->ptr, name->len);
	d->att_name_len = name->len;
	d->att_name[name->len] = 0;
	d->attested = 1; /* accepted: no reply, the device answers payment.prepare */
}

static void prepare(nu54_device_t *d, const nu54_msg_t *m, uint64_t now, nu54_out_t *out)
{
	uint64_t t;
	if (!in_session(d, m, 0)) {
		error_reply(out, field(m, 0, "sessionId")->ptr, "NOT_PERMITTED");
		return;
	}
	if (!device_time(d, now, &t) || d->state != NU54_STATE_READY) {
		refused(d, out, "TIME_ANCHOR_MISSING");
		return;
	}
	if (!d->attested) {
		refused(d, out, "MERCHANT_FORGED");
		return;
	}
	int a = nu54_msg_find(m, 0, "authorization");
	nu54_payment_authorization_t p;
	u256(field(m, a, "chainId"), p.chain_id);
	memcpy(p.contract, field(m, a, "contract")->ptr, 20);
	memcpy(p.merchant, field(m, a, "merchant")->ptr, 20);
	memcpy(p.payout, field(m, a, "payout")->ptr, 20);
	memcpy(p.token, field(m, a, "token")->ptr, 20);
	u256(field(m, a, "amount"), p.amount);
	memcpy(p.order_id, field(m, a, "orderId")->ptr, 32);
	const nu54_cbor_item_t *expiry = field(m, a, "expiry");
	p.expiry = u64(expiry);

	nu54_merchant_order_t o;
	memcpy(o.order_id, p.order_id, 32);
	memcpy(o.token, p.token, 20);
	memcpy(o.amount, p.amount, 32);
	memcpy(o.payout, p.payout, 20);
	o.expiry = p.expiry;
	uint8_t domain[32], h[32], digest[32], signer[20];
	nu54_eip712_domain_separator(d->chain_id, d->contract, domain);
	nu54_hash_merchant_order(&o, h);
	nu54_eip712_digest(domain, h, digest);
	const nu54_cbor_item_t *msig = field(m, 0, "merchantSignature");
	if (expiry->type != NU54_CBOR_UINT || nu54_sig_recover(digest, msig->ptr, msig->len, signer) != NU54_SIG_OK ||
	    memcmp(signer, d->att_merchant, 20) != 0 || memcmp(p.merchant, d->att_merchant, 20) != 0 ||
	    memcmp(p.payout, d->att_payout, 20) != 0 || memcmp(p.chain_id, d->chain_id, 32) != 0 ||
	    memcmp(p.contract, d->contract, 20) != 0) {
		refused(d, out, "MERCHANT_FORGED");
		return;
	}
	if (p.expiry < t || p.expiry > t + d->authorization_expiry) {
		refused(d, out, "ATTESTATION_EXPIRED");
		return;
	}
	/* confirm.show to the phone app, then wait for the button. */
	nu54_cbor_entry_t e[] = {
		{"v", NU54_V_UINT, 0, 0, 1},
		text("type", "confirm.show"),
		bytes("sessionId", d->session_id, 8),
		{"merchantName", NU54_V_TEXT, (const uint8_t *)d->att_name, d->att_name_len, 0},
		bytes("orderId", p.order_id, 32),
		bytes("token", p.token, 20),
		bytes("payout", p.payout, 20),
		{"amount", NU54_V_UINT256, p.amount, 32, 0},
	};
	emit(out, 1, e, 8);
	d->pending_auth = p;
	d->pending = NU54_PENDING_PAYMENT;
}

/* setup.operator (payment-protocol.md 5, step 1): keep the values in RAM and wait for the button. */
static void setup_operator(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out)
{
	if (!in_session(d, m, 1)) {
		error_reply(out, field(m, 0, "sessionId")->ptr, "NOT_PERMITTED");
		return;
	}
	const nu54_cbor_item_t *passkey = field(m, 0, "passkey");
	if (d->state != NU54_STATE_UNPROVISIONED || passkey->type != NU54_CBOR_UINT || passkey->value > 999999 || !d->platform.generate_key) {
		setup_ack(d, out, "setup.operator", 0, "NOT_PERMITTED", NULL); /* once only, six-digit passkey [N23] */
		return;
	}
	nu54_setup_record_t *rec = &d->pending_setup;
	memset(rec, 0, sizeof(*rec));
	memcpy(rec->operator_address, field(m, 0, "operator")->ptr, 20);
	memcpy(rec->contract, field(m, 0, "contract")->ptr, 20);
	u256(field(m, 0, "chainId"), rec->chain_id);
	rec->passkey = (uint32_t)passkey->value;
	d->pending = NU54_PENDING_SETUP_CONFIRM;
}

/* The button confirmed the operator values: ack, make the key and the nonce start, wait for the PIN. */
static void setup_confirmed(nu54_device_t *d, int approve, nu54_out_t *out)
{
	d->pending = NU54_PENDING_NONE;
	if (!approve) {
		setup_ack(d, out, "setup.operator", 0, "USER_REJECTED", NULL);
		return;
	}
	setup_ack(d, out, "setup.operator", 1, NULL, NULL);
	if (d->platform.generate_key(d->platform.ctx, d->pending_address) != 0) {
		setup_ack(d, out, "keygen", 0, "NOT_PERMITTED", NULL);
		return;
	}
	/* 31 random bytes and a zero low byte: a multiple of 256 (payment-protocol.md 2). */
	d->platform.random(d->platform.ctx, d->pending_setup.nonce_start, 31);
	d->pending_setup.nonce_start[31] = 0;
	d->pending = NU54_PENDING_PIN;
}

void nu54_session_pin(nu54_device_t *d, const char *pin, size_t len, nu54_out_t *out)
{
	if (d->pending != NU54_PENDING_PIN) {
		return;
	}
	nu54_setup_record_t *rec = &d->pending_setup;
	rec->pin = pin;
	rec->pin_len = len;
	if (!pin || d->platform.commit_setup(d->platform.ctx, rec) != 0) {
		abort_pending(d); /* nothing is stored */
		setup_ack(d, out, "keygen", 0, pin ? "NOT_PERMITTED" : "TIMEOUT", NULL);
		return;
	}
	d->pending = NU54_PENDING_NONE;
	memcpy(d->address, d->pending_address, 20);
	memcpy(d->operator_address, rec->operator_address, 20);
	memcpy(d->contract, rec->contract, 20);
	memcpy(d->chain_id, rec->chain_id, 32);
	d->passkey = rec->passkey;
	memcpy(d->next_nonce, rec->nonce_start, 32);
	rec->pin = NULL; /* the platform keeps the PIN; RAM does not */
	rec->pin_len = 0;
	d->state = NU54_STATE_PROVISIONED_NO_ANCHOR;
	setup_ack(d, out, "keygen", 1, NULL, d->address);
}

/* device.reset (payment-protocol.md 5): an operator-signed DeviceReset for this device wipes it. */
static void device_reset(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out)
{
	if (!same_session(d, m)) {
		error_reply(out, field(m, 0, "sessionId")->ptr, "NOT_PERMITTED");
		return;
	}
	const nu54_cbor_item_t *dev = field(m, 0, "device"), *nonce = field(m, 0, "nonce"), *sig = field(m, 0, "operatorSignature");
	int ok = d->state != NU54_STATE_UNPROVISIONED && memcmp(dev->ptr, d->address, 20) == 0;
	if (ok) {
		nu54_device_reset_t r;
		memcpy(r.device, dev->ptr, 20);
		u256(nonce, r.nonce);
		uint8_t domain[32], h[32], digest[32], signer[20];
		nu54_eip712_domain_separator(d->chain_id, d->contract, domain);
		nu54_hash_device_reset(&r, h);
		nu54_eip712_digest(domain, h, digest);
		ok = nu54_sig_recover(digest, sig->ptr, sig->len, signer) == NU54_SIG_OK && memcmp(signer, d->operator_address, 20) == 0 &&
		     (!d->platform.wipe || d->platform.wipe(d->platform.ctx) == 0);
	}
	if (!ok) {
		setup_ack(d, out, "device.reset", 0, "NOT_PERMITTED", NULL);
		return;
	}
	setup_ack(d, out, "device.reset", 1, NULL, NULL); /* answered in the session it came in */
	forget_setup(d);                                    /* which ends with the wipe */
}

void nu54_session_button(nu54_device_t *d, int approve, nu54_out_t *out)
{
	if (d->pending == NU54_PENDING_SETUP_CONFIRM) {
		setup_confirmed(d, approve, out);
		return;
	}
	if (d->pending != NU54_PENDING_PAYMENT) {
		return;
	}
	d->pending = NU54_PENDING_NONE;
	if (!approve) {
		refused(d, out, "USER_REJECTED");
		return;
	}
	nu54_payment_authorization_t p = d->pending_auth;
	memcpy(p.nonce, d->next_nonce, 32);
	increment(d->next_nonce); /* sequential: consecutive nonces share one bitmap slot */
	if (d->platform.persist_nonce && d->platform.persist_nonce(d->platform.ctx, d->next_nonce) != 0) {
		refused(d, out, "NOT_PERMITTED"); /* the counter must be stored before signing */
		return;
	}
	uint8_t domain[32], h[32], digest[32], rs[64], sig[65];
	nu54_eip712_domain_separator(d->chain_id, d->contract, domain);
	nu54_hash_payment_authorization(&p, h);
	nu54_eip712_digest(domain, h, digest);
	if (d->platform.sign(d->platform.ctx, digest, rs) != 0 || nu54_sig_finish(digest, rs, d->address, sig) != NU54_SIG_OK) {
		refused(d, out, "NOT_PERMITTED");
		return;
	}
	nu54_cbor_entry_t e[] = {
		{"v", NU54_V_UINT, 0, 0, 1},
		text("type", "payment.result"),
		bytes("sessionId", reply_sid(d), 8),
		text("outcome", "approved"),
		bytes("signature", sig, 65),
		{"nonce", NU54_V_UINT256, p.nonce, 32, 0},
	};
	emit(out, 0, e, 6);
}

void nu54_session_handle(nu54_device_t *d, const uint8_t *body, size_t len, uint64_t now, nu54_out_t *out)
{
	static nu54_msg_t m; /* about 2 KB; the session handles one message at a time */
	memset(out, 0, sizeof(*out));
	nu54_msg_result_t r = nu54_msg_decode(body, len, &m);
	if (r != NU54_MSG_OK) {
		/* Drop the message, report and close the session (payment-protocol.md 4, 4.2). */
		error_reply(out, ZERO_SESSION, r == NU54_MSG_UNSUPPORTED_TYPE ? "UNSUPPORTED_TYPE" : "BAD_FRAME");
		d->session_open = 0;
		abort_pending(d);
		return;
	}
	const char *type = m.message->type;
	if (strcmp(type, "session.open") == 0) {
		session_open(d, &m, out);
	} else if (strcmp(type, "session.confirm") == 0) {
		session_confirm(d, &m, out);
	} else if (strcmp(type, "session.cancel") == 0) {
		d->session_open = 0;
		abort_pending(d);
	} else if (strcmp(type, "setup.timeAnchor") == 0) {
		time_anchor(d, &m, now, out);
	} else if (strcmp(type, "payment.identify") == 0) {
		identify(d, &m, now, out);
	} else if (strcmp(type, "payment.prepare") == 0) {
		prepare(d, &m, now, out);
	} else if (strcmp(type, "payment.outcome") == 0) {
		/* The kiosk's final result: forwarded unchanged to the phone app for the payment session
		 * it belongs to (schema payment.outcome, P02-FR-07); no reply to the kiosk. */
		if (in_session(d, &m, 0) && len <= NU54_OUT_MAX) {
			memcpy(out->phone, body, len);
			out->phone_len = len;
			out->phone_count = 1;
		}
	} else if (strcmp(type, "setup.operator") == 0) {
		setup_operator(d, &m, out);
	} else if (strcmp(type, "device.reset") == 0) {
		device_reset(d, &m, out);
	} else if (strcmp(type, "limit.change") == 0) {
		error_reply(out, field(&m, 0, "sessionId")->ptr, "NOT_PERMITTED"); /* not implemented yet */
	} else {
		error_reply(out, field(&m, 0, "sessionId")->ptr, "UNSUPPORTED_TYPE");
	}
}
