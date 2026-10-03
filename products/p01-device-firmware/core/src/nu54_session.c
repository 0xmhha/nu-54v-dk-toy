#include "nu54_session.h"

#include <stdio.h>
#include <string.h>

#include "nu54_cbor.h"
#include "nu54_secure.h"
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
		/* Room for the GCM tag when the reply goes out under the secure channel. */
		nu54_cbor_writer_init(&w, out->kiosk[out->kiosk_count], NU54_OUT_MAX - NU54_GCM_TAG_LEN);
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

/* ---------------------------------------------------------------- secure channel (4.1) */

enum { DIR_KIOSK = 0x01, DIR_DEVICE = 0x02 };

/* The channel a message came in on: its replies go out under it even if the session ends. */
typedef struct {
	int on;
	int first; /* replies before this index were sealed by an earlier call */
	uint32_t id;
	uint8_t key[16];
	uint64_t tx;
} channel_t;

static void make_iv(uint8_t iv[12], uint8_t dir, uint64_t n)
{
	memset(iv, 0, 12);
	iv[0] = dir;
	for (int k = 0; k < 8; k++) {
		iv[11 - k] = (uint8_t)(n >> (8 * k));
	}
}

static void drop_channel(nu54_device_t *d)
{
	memset(d->channel_key, 0, sizeof(d->channel_key));
	d->secure = 0;
}

static void channel_begin(const nu54_device_t *d, channel_t *c, const nu54_out_t *out)
{
	memset(c, 0, sizeof(*c));
	c->first = out->kiosk_count;
	c->on = d->session_open && d->secure;
	if (c->on) {
		c->id = d->channel_id;
		memcpy(c->key, d->channel_key, 16);
		c->tx = d->channel_tx;
	}
}

/* Seals the replies for the central; a reply that cannot be sealed is dropped with the rest and
 * the session closes. A channel whose session ended is wiped. */
static void channel_end(nu54_device_t *d, channel_t *c, nu54_out_t *out)
{
	if (c->on) {
		for (int i = c->first; i < out->kiosk_count; i++) {
			uint8_t iv[12], sealed[NU54_OUT_MAX];
			size_t len = out->kiosk_len[i];
			make_iv(iv, DIR_DEVICE, c->tx++);
			if (len + NU54_GCM_TAG_LEN > NU54_OUT_MAX || d->platform.aead_seal(d->platform.ctx, c->key, iv, out->kiosk[i], len, sealed) != 0) {
				out->kiosk_count = i;
				d->session_open = 0;
				break;
			}
			memcpy(out->kiosk[i], sealed, len + NU54_GCM_TAG_LEN);
			out->kiosk_len[i] = len + NU54_GCM_TAG_LEN;
		}
		if (d->session_open && d->secure && d->channel_id == c->id) {
			d->channel_tx = c->tx;
		}
		memset(c->key, 0, sizeof(c->key));
	}
	if (d->secure && !d->session_open) {
		drop_channel(d);
	}
}

/* A MerchantAttestation (map `a` of `m`) signed by the recorded operator. */
static int operator_attestation(const nu54_device_t *d, const nu54_msg_t *m, int a, nu54_merchant_attestation_t *att)
{
	const nu54_cbor_item_t *merchant = field(m, a, "merchant"), *payout = field(m, a, "payout"), *name = field(m, a, "name"),
			       *from = field(m, a, "validFrom"), *until = field(m, a, "validUntil"), *sig = field(m, a, "operatorSignature");
	memcpy(att->merchant, merchant->ptr, 20);
	memcpy(att->payout, payout->ptr, 20);
	att->name = name->ptr;
	att->name_len = name->len;
	att->valid_from = u64(from);
	att->valid_until = u64(until);
	uint8_t domain[32], h[32], digest[32], signer[20];
	nu54_eip712_domain_separator(d->chain_id, d->contract, domain);
	nu54_hash_merchant_attestation(att, h);
	nu54_eip712_digest(domain, h, digest);
	return from->type == NU54_CBOR_UINT && until->type == NU54_CBOR_UINT && nu54_sig_recover(digest, sig->ptr, sig->len, signer) == NU54_SIG_OK &&
	       memcmp(signer, d->operator_address, 20) == 0;
}

/* The merchant a secure session.open proves (4.1, step 2): the attestation is the operator's, the
 * one-time key is signed by that merchant and is a curve point. Validity times wait for
 * payment.identify, once the device has an anchor. */
static int kiosk_merchant(const nu54_device_t *d, const nu54_msg_t *m, uint8_t merchant[20])
{
	int a = nu54_msg_find(m, 0, "attestation");
	const nu54_cbor_item_t *eph = field(m, 0, "kioskEphemeral"), *kn = field(m, 0, "kioskNonce"), *ks = field(m, 0, "kioskKeySignature");
	nu54_merchant_attestation_t att;
	nu54_kiosk_key_t k;
	uint8_t domain[32], h[32], digest[32], signer[20];

	if (a < 0 || !ks || !operator_attestation(d, m, a, &att)) {
		return 0;
	}
	memcpy(k.merchant, att.merchant, 20);
	memcpy(k.kiosk_ephemeral, eph->ptr, 32);
	memcpy(k.kiosk_nonce, kn->ptr, 32);
	nu54_eip712_domain_separator(d->chain_id, d->contract, domain);
	nu54_hash_kiosk_key(&k, h);
	nu54_eip712_digest(domain, h, digest);
	if (nu54_sig_recover(digest, ks->ptr, ks->len, signer) != NU54_SIG_OK || memcmp(signer, att.merchant, 20) != 0 ||
	    !nu54_secure_is_point(eph->ptr)) {
		return 0;
	}
	memcpy(merchant, att.merchant, 20);
	return 1;
}

/* The device's one-time key and the session key (4.1, steps 2-3); the private key and the shared
 * value never outlive this call. Needs device_nonce already drawn. */
static int start_channel(nu54_device_t *d, const uint8_t kiosk_x[32], const uint8_t kiosk_nonce[32], uint8_t device_x[32])
{
	uint8_t priv[32], shared[32], salt[64];
	int err = -1;

	for (int tries = 0; tries < 8 && err; tries++) {
		d->platform.random(d->platform.ctx, priv, 32); /* a draw that is not a scalar is drawn again */
		err = nu54_secure_public_x(priv, device_x);
	}
	if (!err) {
		err = nu54_secure_shared_x(priv, kiosk_x, shared);
	}
	memcpy(salt, kiosk_nonce, 32);
	memcpy(&salt[32], d->device_nonce, 32);
	if (!err) {
		err = d->platform.hkdf(d->platform.ctx, shared, salt, NU54_SESSION_KEY_INFO, d->channel_key);
	}
	memset(priv, 0, sizeof(priv));
	memset(shared, 0, sizeof(shared));
	if (!err) {
		d->channel_rx = 0;
		d->channel_tx = 0;
		d->channel_id++;
	}
	return err;
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
	drop_channel(d);
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
	drop_channel(d);
	if (d->state == NU54_STATE_READY) {
		d->state = NU54_STATE_PROVISIONED_NO_ANCHOR;
	}
}

void nu54_session_link_closed(nu54_device_t *d)
{
	d->session_open = 0;
	abort_pending(d);
	drop_channel(d);
}

/* ---------------------------------------------------------------- messages */

static void session_open(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out)
{
	const nu54_cbor_item_t *sid = field(m, 0, "sessionId");
	const nu54_cbor_item_t *mode = field(m, 0, "mode");
	int setup = mode->len == 5 && memcmp(mode->ptr, "setup", 5) == 0;
	const nu54_cbor_item_t *eph = field(m, 0, "kioskEphemeral");
	int secure = eph != NULL;
	uint8_t merchant[20], device_x[32];
	if (setup && d->state != NU54_STATE_UNPROVISIONED && d->state != NU54_STATE_PROVISIONED_NO_ANCHOR) {
		error_reply(out, sid->ptr, "NOT_PERMITTED");
		return;
	}
	/* The channel is for the unpaired kiosk link only; a release device refuses plaintext payments. */
	if ((secure && (setup || d->state == NU54_STATE_UNPROVISIONED || !d->platform.hkdf || !d->platform.aead_seal || !d->platform.aead_open)) ||
	    (!secure && !setup && d->require_secure)) {
		error_reply(out, sid->ptr, "NOT_PERMITTED");
		return;
	}
	if (secure && !kiosk_merchant(d, m, merchant)) {
		error_reply(out, sid->ptr, "MERCHANT_FORGED");
		return;
	}
	abort_pending(d);
	d->platform.random(d->platform.ctx, d->device_nonce, 32);
	if (secure && start_channel(d, eph->ptr, field(m, 0, "kioskNonce")->ptr, device_x) != 0) {
		drop_channel(d);
		d->session_open = 0;
		error_reply(out, sid->ptr, "NOT_PERMITTED");
		return;
	}
	memcpy(d->session_id, sid->ptr, 8);
	d->session_open = 1;
	d->session_setup = setup;
	d->confirmed = 0;
	d->attested = 0;
	d->secure = secure;
	if (secure) {
		memcpy(d->secure_merchant, merchant, 20);
	}
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
		bytes("deviceEphemeral", device_x, 32), /* secure sessions only */
	};
	emit(out, 0, e, d->state == NU54_STATE_UNPROVISIONED ? 8 : secure ? 10 : 9); /* UNPROVISIONED is never secure */
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
	nu54_merchant_attestation_t att;
	/* A secure session serves only the merchant its session.open proved. */
	if (!operator_attestation(d, m, nu54_msg_find(m, 0, "attestation"), &att) || (d->secure && memcmp(att.merchant, d->secure_merchant, 20) != 0)) {
		refused(d, out, "MERCHANT_FORGED");
		return;
	}
	uint64_t skew = d->anchor_clock_skew;
	if (t + skew < att.valid_from || (t > skew && t - skew > att.valid_until) || att.name_len >= sizeof(d->att_name)) {
		refused(d, out, "ATTESTATION_EXPIRED");
		return;
	}
	memcpy(d->att_merchant, att.merchant, 20);
	memcpy(d->att_payout, att.payout, 20);
	memcpy(d->att_name, att.name, att.name_len);
	d->att_name_len = att.name_len;
	d->att_name[att.name_len] = 0;
	d->attested = 1; /* accepted: no reply, the device answers payment.prepare */
}

static void prepare(nu54_device_t *d, const nu54_msg_t *m, uint64_t now, nu54_out_t *out)
{
	uint64_t t;
	if (!in_session(d, m, 0)) {
		error_reply(out, field(m, 0, "sessionId")->ptr, "NOT_PERMITTED");
		return;
	}
	if (d->state == NU54_STATE_PIN_LOCKED) {
		refused(d, out, "PIN_LOCKED"); /* every signature is refused */
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

static void limit_refused(const nu54_device_t *d, nu54_out_t *out, const char *reason)
{
	nu54_cbor_entry_t e[] = {{"v", NU54_V_UINT, 0, 0, 1}, text("type", "limit.result"), bytes("sessionId", reply_sid(d), 8),
				 text("outcome", "refused"), text("reason", reason)};
	emit(out, 0, e, 5);
}

/* limit.change (payment-protocol.md 5): checks, confirm.limit to the phone, then the PIN. */
static void limit_change(nu54_device_t *d, const nu54_msg_t *m, uint64_t now, nu54_out_t *out)
{
	uint64_t t;
	if (!in_session(d, m, 0)) {
		error_reply(out, field(m, 0, "sessionId")->ptr, "NOT_PERMITTED");
		return;
	}
	if (d->state == NU54_STATE_PIN_LOCKED) {
		limit_refused(d, out, "PIN_LOCKED");
		return;
	}
	if (!device_time(d, now, &t) || d->state != NU54_STATE_READY) {
		limit_refused(d, out, "TIME_ANCHOR_MISSING");
		return;
	}
	int c = nu54_msg_find(m, 0, "change");
	nu54_limit_change_t *l = &d->pending_limit;
	memset(l, 0, sizeof(*l));
	u256(field(m, c, "chainId"), l->chain_id);
	memcpy(l->contract, field(m, c, "contract")->ptr, 20);
	u256(field(m, c, "perPaymentLimit"), l->per_payment_limit);
	u256(field(m, c, "dailyLimit"), l->daily_limit);
	const nu54_cbor_item_t *expiry = field(m, c, "expiry");
	l->expiry = u64(expiry);
	if (memcmp(l->chain_id, d->chain_id, 32) != 0 || memcmp(l->contract, d->contract, 20) != 0 || !d->platform.check_pin) {
		limit_refused(d, out, "NOT_PERMITTED");
		return;
	}
	if (expiry->type != NU54_CBOR_UINT || l->expiry < t || l->expiry > t + d->authorization_expiry) {
		limit_refused(d, out, "ATTESTATION_EXPIRED");
		return;
	}
	nu54_cbor_entry_t e[] = {
		{"v", NU54_V_UINT, 0, 0, 1},
		text("type", "confirm.limit"),
		bytes("sessionId", d->session_id, 8),
		{"perPaymentLimit", NU54_V_UINT256, l->per_payment_limit, 32, 0},
		{"dailyLimit", NU54_V_UINT256, l->daily_limit, 32, 0},
		{"expiry", NU54_V_UINT, 0, 0, l->expiry},
	};
	emit(out, 1, e, 6);
	d->pending = NU54_PENDING_LIMIT_PIN;
}

static void limit_pin(nu54_device_t *d, const char *pin, size_t len, nu54_out_t *out)
{
	d->pending = NU54_PENDING_NONE;
	if (!pin) {
		limit_refused(d, out, "TIMEOUT");
		return;
	}
	int r = d->platform.check_pin(d->platform.ctx, pin, len);
	if (r == NU54_PIN_LOCKED) {
		d->state = NU54_STATE_PIN_LOCKED;
		limit_refused(d, out, "PIN_LOCKED");
	} else if (r != NU54_PIN_OK) {
		limit_refused(d, out, "NOT_PERMITTED");
	} else {
		d->pending = NU54_PENDING_LIMIT_CONFIRM;
	}
}

static void limit_confirmed(nu54_device_t *d, int approve, nu54_out_t *out)
{
	d->pending = NU54_PENDING_NONE;
	if (!approve) {
		limit_refused(d, out, "USER_REJECTED");
		return;
	}
	nu54_limit_change_t l = d->pending_limit;
	memcpy(l.nonce, d->next_nonce, 32);
	increment(d->next_nonce); /* the same sequential counter as payments */
	if (d->platform.persist_nonce && d->platform.persist_nonce(d->platform.ctx, d->next_nonce) != 0) {
		limit_refused(d, out, "NOT_PERMITTED");
		return;
	}
	uint8_t domain[32], h[32], digest[32], rs[64], sig[65];
	nu54_eip712_domain_separator(d->chain_id, d->contract, domain);
	nu54_hash_limit_change(&l, h);
	nu54_eip712_digest(domain, h, digest);
	if (d->platform.sign(d->platform.ctx, digest, rs) != 0 || nu54_sig_finish(digest, rs, d->address, sig) != NU54_SIG_OK) {
		limit_refused(d, out, "NOT_PERMITTED");
		return;
	}
	nu54_cbor_entry_t e[] = {
		{"v", NU54_V_UINT, 0, 0, 1},
		text("type", "limit.result"),
		bytes("sessionId", reply_sid(d), 8),
		text("outcome", "approved"),
		bytes("signature", sig, 65),
		{"nonce", NU54_V_UINT256, l.nonce, 32, 0},
	};
	emit(out, 0, e, 6);
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

static void pin_step(nu54_device_t *d, const char *pin, size_t len, nu54_out_t *out)
{
	if (d->pending == NU54_PENDING_LIMIT_PIN) {
		limit_pin(d, pin, len, out);
		return;
	}
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

static void button_step(nu54_device_t *d, int approve, nu54_out_t *out)
{
	if (d->pending == NU54_PENDING_SETUP_CONFIRM) {
		setup_confirmed(d, approve, out);
		return;
	}
	if (d->pending == NU54_PENDING_LIMIT_CONFIRM) {
		limit_confirmed(d, approve, out);
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

static void dispatch(nu54_device_t *d, const uint8_t *body, size_t len, uint64_t now, nu54_out_t *out)
{
	static nu54_msg_t m; /* about 2 KB; the session handles one message at a time */
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
		limit_change(d, &m, now, out);
	} else {
		error_reply(out, field(&m, 0, "sessionId")->ptr, "UNSUPPORTED_TYPE");
	}
}

void nu54_session_handle(nu54_device_t *d, const uint8_t *body, size_t len, uint64_t now, nu54_out_t *out)
{
	static uint8_t plain[2048]; /* the largest body (payment-protocol.md 4) */
	channel_t c;

	memset(out, 0, sizeof(*out));
	channel_begin(d, &c, out);
	if (c.on) {
		uint8_t iv[12];
		make_iv(iv, DIR_KIOSK, d->channel_rx);
		if (len < NU54_GCM_TAG_LEN || len - NU54_GCM_TAG_LEN > sizeof(plain) ||
		    d->platform.aead_open(d->platform.ctx, c.key, iv, body, len, plain) != 0) {
			/* A failed tag closes the session like any bad frame (4.1, step 4). */
			error_reply(out, ZERO_SESSION, "BAD_FRAME");
			d->session_open = 0;
			abort_pending(d);
			channel_end(d, &c, out);
			return;
		}
		d->channel_rx++;
		body = plain;
		len -= NU54_GCM_TAG_LEN;
	}
	dispatch(d, body, len, now, out);
	channel_end(d, &c, out);
}

void nu54_session_button(nu54_device_t *d, int approve, nu54_out_t *out)
{
	channel_t c;
	channel_begin(d, &c, out);
	button_step(d, approve, out);
	channel_end(d, &c, out);
}

void nu54_session_pin(nu54_device_t *d, const char *pin, size_t len, nu54_out_t *out)
{
	channel_t c;
	channel_begin(d, &c, out);
	pin_step(d, pin, len, out);
	channel_end(d, &c, out);
}
