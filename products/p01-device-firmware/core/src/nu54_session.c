#include "nu54_session_int.h"

static const char *const STATE_NAME[] = {"UNPROVISIONED", "PROVISIONED_NO_ANCHOR", "READY", "PIN_LOCKED"};

void nu54_device_init(nu54_device_t *d, const uint8_t nonce_start[32])
{
	d->state = NU54_STATE_PROVISIONED_NO_ANCHOR;
	d->anchored = 0;
	d->last_anchor = 0;
	memcpy(d->next_nonce, nonce_start, 32);
	d->session_open = 0;
	d->attested = 0;
	d->pending = NU54_PENDING_NONE;
	ss_drop_channel(d);
}

void nu54_device_init_unprovisioned(nu54_device_t *d)
{
	ss_forget_setup(d);
}

void nu54_device_power_cycle(nu54_device_t *d)
{
	d->anchored = 0;
	d->session_open = 0;
	ss_abort_pending(d);
	ss_drop_channel(d);
	if (d->state == NU54_STATE_READY) {
		d->state = NU54_STATE_PROVISIONED_NO_ANCHOR;
	}
}

void nu54_session_link_closed(nu54_device_t *d)
{
	d->session_open = 0;
	ss_abort_pending(d);
	ss_drop_channel(d);
}

static void session_open(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out)
{
	const nu54_cbor_item_t *sid = ss_field(m, 0, "sessionId");
	const nu54_cbor_item_t *mode = ss_field(m, 0, "mode");
	int setup = mode->len == 5 && memcmp(mode->ptr, "setup", 5) == 0;
	const nu54_cbor_item_t *eph = ss_field(m, 0, "kioskEphemeral");
	int secure = eph != NULL;
	uint8_t merchant[20], device_x[32];
	/* Setup sessions open from a bonded central only (P01-FR-13). The week-7 fixed setup lets a
	 * provisioned device take one from the unpaired kiosk for its TimeAnchor; setup.operator
	 * still checks session_bonded. */
	int unpaired_ok = d->cfg.dev_unpaired_anchor && d->state != NU54_STATE_UNPROVISIONED;
	if (setup && ((d->state != NU54_STATE_UNPROVISIONED && d->state != NU54_STATE_PROVISIONED_NO_ANCHOR) || !(d->call.bonded || unpaired_ok))) {
		ss_error(out, sid->ptr, "NOT_PERMITTED");
		return;
	}
	/* The channel is for the unpaired kiosk link only; a release device refuses plaintext payments. */
	if ((secure && (setup || d->state == NU54_STATE_UNPROVISIONED || !d->cfg.platform.hkdf || !d->cfg.platform.aead_seal || !d->cfg.platform.aead_open)) ||
	    (!secure && !setup && d->cfg.require_secure)) {
		ss_error(out, sid->ptr, "NOT_PERMITTED");
		return;
	}
	if (secure && !ss_kiosk_merchant(d, m, merchant)) {
		ss_error(out, sid->ptr, "MERCHANT_FORGED");
		return;
	}
	ss_abort_pending(d);
	d->cfg.platform.random(d->cfg.platform.ctx, d->device_nonce, 32);
	if (secure && ss_start_channel(d, eph->ptr, ss_field(m, 0, "kioskNonce")->ptr, device_x) != 0) {
		ss_drop_channel(d);
		d->session_open = 0;
		ss_error(out, sid->ptr, "NOT_PERMITTED");
		return;
	}
	memcpy(d->session_id, sid->ptr, 8);
	d->session_open = 1;
	d->session_setup = setup;
	d->session_bonded = d->call.bonded;
	d->confirmed = 0;
	d->attested = 0;
	d->secure = secure;
	if (secure) {
		memcpy(d->secure_merchant, merchant, 20);
	}
	uint8_t last[32];
	ss_last_anchor(d, last);
	nu54_cbor_entry_t e[] = {
		{"v", NU54_V_UINT, 0, 0, 1},
		ss_text("type", "session.open.ok"),
		ss_bytes("sessionId", d->session_id, 8),
		ss_bytes("deviceNonce", d->device_nonce, 32),
		{"anchorValid", NU54_V_BOOL, 0, 0, (uint64_t)d->anchored},
		ss_text("firmware", d->cfg.firmware),
		ss_text("state", STATE_NAME[d->state]),
		{"lastAnchor", NU54_V_UINT256, last, 32, 0},
		ss_bytes("device", d->address, 20), /* an UNPROVISIONED device has no key yet */
		ss_bytes("deviceEphemeral", device_x, 32), /* secure sessions only */
	};
	ss_emit(out, 0, e, d->state == NU54_STATE_UNPROVISIONED ? 8 : secure ? 10 : 9); /* UNPROVISIONED is never secure */
}

static void session_confirm(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out)
{
	const nu54_cbor_item_t *dn = ss_field(m, 0, "deviceNonce");
	if (!ss_same_session(d, m) || memcmp(dn->ptr, d->device_nonce, 32) != 0) {
		d->session_open = 0;
		ss_error(out, ss_field(m, 0, "sessionId")->ptr, "NOT_PERMITTED");
		return;
	}
	d->confirmed = 1;
}

static void dispatch(nu54_device_t *d, const uint8_t *body, size_t len, uint64_t now, nu54_out_t *out)
{
	static nu54_msg_t m; /* about 2 KB; the session handles one message at a time */
	nu54_msg_result_t r = nu54_msg_decode(body, len, &m);
	if (r != NU54_MSG_OK) {
		/* Drop the message, report and close the session (payment-protocol.md 4, 4.2). */
		ss_error(out, ss_zero_session, r == NU54_MSG_UNSUPPORTED_TYPE ? "UNSUPPORTED_TYPE" : "BAD_FRAME");
		d->session_open = 0;
		ss_abort_pending(d);
		return;
	}
	const char *type = m.message->type;
	if (strcmp(type, "session.open") == 0) {
		session_open(d, &m, out);
	} else if (strcmp(type, "session.confirm") == 0) {
		session_confirm(d, &m, out);
	} else if (strcmp(type, "session.cancel") == 0) {
		d->session_open = 0;
		ss_abort_pending(d);
	} else if (strcmp(type, "setup.timeAnchor") == 0) {
		ss_time_anchor(d, &m, now, out);
	} else if (strcmp(type, "payment.identify") == 0) {
		ss_identify(d, &m, now, out);
	} else if (strcmp(type, "payment.prepare") == 0) {
		ss_prepare(d, &m, now, out);
	} else if (strcmp(type, "payment.outcome") == 0) {
		/* The kiosk's final result: forwarded unchanged to the phone app for the payment session
		 * it belongs to (schema payment.outcome, P02-FR-07); no reply to the kiosk. */
		if (ss_in_session(d, &m, 0) && len <= NU54_OUT_MAX) {
			memcpy(out->phone, body, len);
			out->phone_len = len;
			out->phone_count = 1;
		}
	} else if (strcmp(type, "setup.operator") == 0) {
		ss_setup_operator(d, &m, out);
	} else if (strcmp(type, "device.reset") == 0) {
		ss_device_reset(d, &m, out);
	} else if (strcmp(type, "limit.change") == 0) {
		ss_limit_change(d, &m, now, out);
	} else if (strcmp(type, "device.paymentMode") == 0) {
		ss_payment_mode(d, &m, out);
	} else {
		ss_error(out, ss_field(&m, 0, "sessionId")->ptr, "UNSUPPORTED_TYPE");
	}
}

void nu54_session_handle(nu54_device_t *d, const nu54_link_t *link, const uint8_t *body, size_t len, uint64_t now, nu54_out_t *out)
{
	static uint8_t plain[2048]; /* the largest body (payment-protocol.md 4) */
	channel_t c;

	memset(out, 0, sizeof(*out));
	d->call = *link;
	ss_channel_begin(d, &c, out);
	if (c.on) {
		if (ss_channel_open(d, &c, body, len, plain, sizeof(plain)) != 0) {
			/* A failed tag closes the session like any bad frame (4.1, step 4). */
			ss_error(out, ss_zero_session, "BAD_FRAME");
			d->session_open = 0;
			ss_abort_pending(d);
			ss_channel_end(d, &c, out);
			memset(&d->call, 0, sizeof(d->call));
			return;
		}
		body = plain;
		len -= NU54_GCM_TAG_LEN;
	}
	dispatch(d, body, len, now, out);
	ss_channel_end(d, &c, out);
	memset(&d->call, 0, sizeof(d->call)); /* the link belongs to this message only */
}

/* The renter's button goes to whatever waits for it. */
static void button_step(nu54_device_t *d, int approve, nu54_out_t *out)
{
	if (d->pending == NU54_PENDING_SETUP_CONFIRM) {
		ss_setup_confirmed(d, approve, out);
	} else if (d->pending == NU54_PENDING_LIMIT_CONFIRM) {
		ss_limit_confirmed(d, approve, out);
	} else if (d->pending == NU54_PENDING_PAYMENT) {
		ss_payment_button(d, approve, out);
	}
}

/* The PIN goes to whatever asks for it: a limit change or setup key generation. */
static void pin_step(nu54_device_t *d, const char *pin, size_t len, nu54_out_t *out)
{
	if (d->pending == NU54_PENDING_LIMIT_PIN) {
		ss_limit_pin(d, pin, len, out);
	} else if (d->pending == NU54_PENDING_PIN) {
		ss_setup_pin(d, pin, len, out);
	}
}

void nu54_session_button(nu54_device_t *d, int approve, nu54_out_t *out)
{
	channel_t c;
	ss_channel_begin(d, &c, out);
	button_step(d, approve, out);
	ss_channel_end(d, &c, out);
}

void nu54_session_pin(nu54_device_t *d, const char *pin, size_t len, nu54_out_t *out)
{
	channel_t c;
	ss_channel_begin(d, &c, out);
	pin_step(d, pin, len, out);
	ss_channel_end(d, &c, out);
}
