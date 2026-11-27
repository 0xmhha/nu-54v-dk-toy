#include "nu54_rental_retirement.h"
#include "nu54_keccak.h"
#include "nu54_session_int.h"
#include "nu54_rental_freeze.h"

void ss_time_anchor(nu54_device_t *d, const nu54_msg_t *m, uint64_t now, nu54_out_t *out)
{
	if (!ss_in_session(d, m, 1)) {
		ss_error(out, ss_field(m, 0, "sessionId")->ptr, "NOT_PERMITTED");
		return;
	}
	if (d->state == NU54_STATE_UNPROVISIONED) {
		uint8_t last[32];
		ss_last_anchor(d, last);
		nu54_cbor_entry_t e[] = {{"v", NU54_V_UINT, 0, 0, 1}, ss_text("type", "setup.ack"), ss_bytes("sessionId", ss_reply_sid(d), 8),
					 ss_text("step", "setup.timeAnchor"), {"accepted", NU54_V_BOOL, 0, 0, 0},
					 {"lastAnchor", NU54_V_UINT256, last, 32, 0}, ss_text("reason", "NOT_PERMITTED")};
		out->event = NU54_EVENT_REFUSED;
		ss_emit(out, 0, e, 7); /* no key and no operator to check against */
		return;
	}
	const nu54_cbor_item_t *dev = ss_field(m, 0, "device"), *ts = ss_field(m, 0, "timestamp"), *sig = ss_field(m, 0, "operatorSignature");
	nu54_time_anchor_t a;
	memcpy(a.device, dev->ptr, 20);
	a.timestamp = ss_u64(ts);
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
	ss_last_anchor(d, last);
	nu54_cbor_entry_t e[] = {
		{"v", NU54_V_UINT, 0, 0, 1},
		ss_text("type", "setup.ack"),
		ss_bytes("sessionId", ss_reply_sid(d), 8),
		ss_text("step", "setup.timeAnchor"),
		{"accepted", NU54_V_BOOL, 0, 0, (uint64_t)ok},
		{"lastAnchor", NU54_V_UINT256, last, 32, 0},
		ss_text("reason", "NOT_PERMITTED"), /* refused anchors only */
	};
	if (!ok) {
		out->event = NU54_EVENT_REFUSED;
	}
	ss_emit(out, 0, e, ok ? 6 : 7);
}

/* setup.operator (payment-protocol.md 5, step 1): keep the values in RAM and wait for the button. */
void ss_setup_operator(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out)
{
	/* Recording an operator needs a bonded session, whatever let the session open. */
	if (!ss_in_session(d, m, 1) || !d->session_bonded) {
		ss_error(out, ss_field(m, 0, "sessionId")->ptr, "NOT_PERMITTED");
		return;
	}
	const nu54_cbor_item_t *passkey = ss_field(m, 0, "passkey");
	const nu54_cbor_item_t *journal = ss_field(m, 0, "rentalJournal");
	int journal_requested = journal && journal->value != 0;
	if (d->pending_setup.journal_requested &&
	    (d->pending == NU54_PENDING_SETUP_CONFIRM || d->pending == NU54_PENDING_PIN)) {
		ss_setup_ack(d, out, "setup.operator", 0, "NOT_PERMITTED", NULL);
		return;
	}
	if (journal_requested && (!d->cfg.platform.can_commission_rental_journal ||
				  !d->cfg.platform.can_commission_rental_journal(d->cfg.platform.ctx))) {
		ss_setup_ack(d, out, "setup.operator", 0, "NOT_PERMITTED", NULL);
		return;
	}
	if (d->state != NU54_STATE_UNPROVISIONED || passkey->type != NU54_CBOR_UINT || passkey->value > 999999 || !d->cfg.platform.generate_key) {
		ss_setup_ack(d, out, "setup.operator", 0, "NOT_PERMITTED", NULL); /* once only, six-digit passkey [N23] */
		return;
	}
	nu54_setup_record_t *rec = &d->pending_setup;
	memset(rec, 0, sizeof(*rec));
	memcpy(rec->operator_address, ss_field(m, 0, "operator")->ptr, 20);
	memcpy(rec->contract, ss_field(m, 0, "contract")->ptr, 20);
	ss_u256(ss_field(m, 0, "chainId"), rec->chain_id);
	rec->passkey = (uint32_t)passkey->value;
	rec->journal_requested = (uint8_t)journal_requested;
	d->pending = NU54_PENDING_SETUP_CONFIRM;
}

/* The button confirmed the operator values: ack, make the key and the nonce start, wait for the PIN. */
void ss_setup_confirmed(nu54_device_t *d, int approve, nu54_out_t *out)
{
	d->pending = NU54_PENDING_NONE;
	if (!approve) {
		ss_setup_ack(d, out, "setup.operator", 0, "USER_REJECTED", NULL);
		return;
	}
	ss_setup_ack(d, out, "setup.operator", 1, NULL, NULL);
	if (d->cfg.platform.generate_key(d->cfg.platform.ctx, d->pending_address) != 0) {
		ss_setup_ack(d, out, "keygen", 0, "NOT_PERMITTED", NULL);
		return;
	}
	/* 31 random bytes and a zero low byte: a multiple of 256 (payment-protocol.md 2). */
	d->cfg.platform.random(d->cfg.platform.ctx, d->pending_setup.nonce_start, 31);
	d->pending_setup.nonce_start[31] = 0;
	d->pending = NU54_PENDING_PIN;
}

/* device.reset (payment-protocol.md 5): an operator-signed DeviceReset for this device wipes it. */
void ss_device_reset(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out)
{
	if (!ss_same_session(d, m)) {
		ss_error(out, ss_field(m, 0, "sessionId")->ptr, "NOT_PERMITTED");
		return;
	}
	const nu54_cbor_item_t *dev = ss_field(m, 0, "device"), *nonce = ss_field(m, 0, "nonce"), *sig = ss_field(m, 0, "operatorSignature");
	int ok = d->state != NU54_STATE_UNPROVISIONED && !d->rental_journal_required && memcmp(dev->ptr, d->address, 20) == 0;
	if (ok) {
		nu54_device_reset_t r;
		memcpy(r.device, dev->ptr, 20);
		ss_u256(nonce, r.nonce);
		uint8_t domain[32], h[32], digest[32], signer[20];
		nu54_eip712_domain_separator(d->chain_id, d->contract, domain);
		nu54_hash_device_reset(&r, h);
		nu54_eip712_digest(domain, h, digest);
		ok = nu54_sig_recover(digest, sig->ptr, sig->len, signer) == NU54_SIG_OK && memcmp(signer, d->operator_address, 20) == 0 &&
		     (!d->cfg.platform.wipe || d->cfg.platform.wipe(d->cfg.platform.ctx) == 0);
	}
	if (!ok) {
		ss_setup_ack(d, out, "device.reset", 0, "NOT_PERMITTED", NULL);
		return;
	}
	ss_setup_ack(d, out, "device.reset", 1, NULL, NULL); /* answered in the session it came in */
	ss_forget_setup(d);                                    /* which ends with the wipe */
}

static int rental_collector_allowed(const nu54_device_t *d, const nu54_cbor_item_t *sid)
{
	return d->call.bonded && d->call.connection_epoch && !memcmp(sid->ptr, ss_zero_session, 8) &&
	       d->rental_journal_required && d->rental_journal && d->cfg.platform.freeze_rental &&
	       d->cfg.platform.allow_rental_collector &&
	       d->cfg.platform.allow_rental_collector(d->cfg.platform.ctx, &d->rental_journal->expected_binding, d->call.peer);
}

void ss_rental_freeze_challenge(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out)
{
	static const uint8_t zero[32] = {0};
	uint64_t now;
	if (!rental_collector_allowed(d, ss_field(m, 0, "sessionId")) || !d->cfg.platform.rental_challenge_random ||
	    ss_rental_time(d, &now) != 0 || now > UINT64_MAX - NU54_RENTAL_FREEZE_LIFETIME) {
		ss_error(out, ss_zero_session, "NOT_PERMITTED");
		return;
	}
	uint8_t nonce[32] = {0};
	if (d->cfg.platform.rental_challenge_random(d->cfg.platform.ctx, nonce) != 0 ||
	    !memcmp(nonce, zero, sizeof(nonce)) || !memcmp(nonce, d->last_rental_channel_nonce, sizeof(nonce))) {
		memset(&d->rental_access, 0, sizeof(d->rental_access));
		ss_error(out, ss_zero_session, "NOT_PERMITTED");
		return;
	}
	d->rental_access.cleanup = 0;
	d->rental_access.epoch = d->call.connection_epoch;
	d->rental_access.expires_at = now + NU54_RENTAL_FREEZE_LIFETIME;
	memcpy(d->rental_access.peer, d->call.peer, sizeof(d->rental_access.peer));
	memcpy(d->rental_access.nonce, nonce, sizeof(nonce));
	memcpy(d->last_rental_channel_nonce, nonce, sizeof(nonce));
	nu54_cbor_entry_t e[] = {
		{"v", NU54_V_UINT, NULL, 0, 1}, ss_text("type", "rental.freeze.challenge.result"),
		ss_bytes("sessionId", ss_zero_session, 8), ss_bytes("channelNonce", nonce, sizeof(nonce)),
		{"expiresAt", NU54_V_UINT, NULL, 0, d->rental_access.expires_at},
	};
	ss_emit(out, 0, e, sizeof(e) / sizeof(e[0]));
}

static uint64_t evidence_expiry(const uint8_t bytes[8])
{
	uint64_t expiry = 0;
	for (unsigned i = 0; i < 8; ++i) expiry = (expiry << 8) | bytes[i];
	return expiry;
}

static void rental_freeze_entry(nu54_device_t *d, uint64_t index, const uint8_t expected_root[32], nu54_out_t *out)
{
	nu54_rental_evidence_t projection;
	uint8_t root[32];
	if (nu54_rental_journal_project(d->rental_journal, &projection, root) != 0 ||
	    memcmp(root, expected_root, sizeof(root)) || index >= projection.count) {
		ss_error(out, ss_zero_session, "NOT_PERMITTED");
		return;
	}
	const nu54_rental_evidence_entry_t *entry = &projection.entries[index];
	nu54_cbor_entry_t e[] = {
		{"v", NU54_V_UINT, NULL, 0, 1}, ss_text("type", "rental.freeze.entry"),
		ss_bytes("sessionId", ss_zero_session, 8), {"projectionIndex", NU54_V_UINT, NULL, 0, index},
		ss_bytes("projectionRoot", root, sizeof(root)),
		{"kind", NU54_V_UINT, NULL, 0, entry->kind}, {"state", NU54_V_UINT, NULL, 0, entry->state},
		{"nonce", NU54_V_UINT256, entry->nonce, sizeof(entry->nonce), 0},
		{"expiry", NU54_V_UINT, NULL, 0, evidence_expiry(entry->expiry)},
		ss_bytes("merchant", entry->merchant, sizeof(entry->merchant)),
		ss_bytes("orderId", entry->order, sizeof(entry->order)),
		ss_bytes("approvalDigest", entry->digest, sizeof(entry->digest)),
	};
	ss_emit(out, 0, e, sizeof(e) / sizeof(e[0]));
}

void ss_rental_freeze_command(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out)
{
	const nu54_cbor_item_t *sid = ss_field(m, 0, "sessionId"), *expiry = ss_field(m, 0, "expiresAt");
	const nu54_cbor_item_t *index = ss_field(m, 0, "projectionIndex"), *nonce = ss_field(m, 0, "channelNonce");
	uint64_t now;
	if (!rental_collector_allowed(d, sid) || d->rental_access.cleanup || d->rental_access.epoch != d->call.connection_epoch ||
	    memcmp(d->rental_access.peer, d->call.peer, sizeof(d->call.peer)) ||
	    memcmp(d->rental_access.nonce, nonce->ptr, sizeof(d->rental_access.nonce)) ||
	    ss_rental_time(d, &now) != 0 || d->rental_access.expires_at <= now ||
	    expiry->type != NU54_CBOR_UINT || expiry->value > d->rental_access.expires_at ||
	    (index && (index->type != NU54_CBOR_UINT || !d->rental_journal->loaded ||
		       !d->rental_journal->image.frozen || index->value >= d->rental_journal->image.count))) {
		ss_error(out, ss_zero_session, "NOT_PERMITTED");
		return;
	}
	nu54_rental_freeze_request_t request = {.expires_at = expiry->value};
	memcpy(request.challenge, ss_field(m, 0, "challenge")->ptr, sizeof(request.challenge));
	memcpy(request.channel_nonce, nonce->ptr, sizeof(request.channel_nonce));
	const uint8_t *operator_signature = ss_field(m, 0, "operatorSignature")->ptr;
	nu54_rental_freeze_receipt_t receipt;
	uint8_t identity_signature[65];
	if (d->cfg.platform.freeze_rental(d->cfg.platform.ctx, d, &request, operator_signature, &receipt, identity_signature) != 0) {
		ss_error(out, ss_zero_session, "NOT_PERMITTED");
		return;
	}
	if (index) {
		rental_freeze_entry(d, index->value, receipt.projection_root, out);
		return;
	}
	nu54_cbor_entry_t e[] = {
		{"v", NU54_V_UINT, NULL, 0, 1}, ss_text("type", "rental.freeze.result"),
		ss_bytes("sessionId", ss_zero_session, 8),
		ss_bytes("binding", receipt.binding, 32), ss_bytes("challenge", receipt.challenge, 32),
		ss_bytes("projectionRoot", receipt.projection_root, 32),
		{"firstNonce", NU54_V_UINT256, receipt.first_nonce, 32, 0},
		{"nextNonce", NU54_V_UINT256, receipt.next_nonce, 32, 0},
		{"entryCount", NU54_V_UINT, NULL, 0, receipt.entry_count},
		{"frozenAt", NU54_V_UINT, NULL, 0, receipt.frozen_at},
		{"expiresAt", NU54_V_UINT, NULL, 0, receipt.expires_at},
		ss_bytes("boardSignature", identity_signature, sizeof(identity_signature)),
	};
	ss_emit(out, 0, e, sizeof(e) / sizeof(e[0]));
}

/* The PIN at setup (payment-protocol.md 5, step 2): store everything at once and announce the key. */
void ss_setup_pin(nu54_device_t *d, const char *pin, size_t len, nu54_out_t *out)
{
	nu54_setup_record_t *rec = &d->pending_setup;
	if (pin && rec->journal_requested && !rec->journal_required) {
		ss_setup_ack(d, out, "keygen", 0, "NOT_PERMITTED", NULL);
		return;
	}
	rec->pin = pin;
	rec->pin_len = len;
	if (!pin || d->cfg.platform.commit_setup(d->cfg.platform.ctx, rec) != 0) {
		ss_abort_pending(d); /* nothing is stored */
		ss_setup_ack(d, out, "keygen", 0, pin ? "NOT_PERMITTED" : "TIMEOUT", NULL);
		return;
	}
	d->pending = NU54_PENDING_NONE;
	memcpy(d->address, d->pending_address, 20);
	memcpy(d->operator_address, rec->operator_address, 20);
	memcpy(d->contract, rec->contract, 20);
	memcpy(d->chain_id, rec->chain_id, 32);
	d->passkey = rec->passkey;
	memcpy(d->next_nonce, rec->nonce_start, 32);
	d->rental_journal_required = rec->journal_required;
	d->rental_journal = NULL;
	rec->pin = NULL; /* the platform keeps the PIN; RAM does not */
	rec->pin_len = 0;
	d->state = NU54_STATE_PROVISIONED_NO_ANCHOR;
	ss_setup_ack(d, out, "keygen", 1, NULL, d->address);
}

/* device.info (payment-protocol.md 3): the bonded phone app asks for the state, to choose between
 * wallet setup and its home screen. No session; a link that is not bonded is NOT_PERMITTED. */
void ss_device_info(nu54_device_t *d, nu54_out_t *out)
{
	if (!d->call.bonded) {
		ss_error(out, ss_zero_session, "NOT_PERMITTED");
		return;
	}
	nu54_cbor_entry_t e[] = {
		{"v", NU54_V_UINT, 0, 0, 1},
		ss_text("type", "device.info.ack"),
		ss_bytes("sessionId", ss_zero_session, 8),
		ss_text("state", ss_state_name[d->state]),
		ss_text("firmware", d->cfg.firmware),
		{"anchorValid", NU54_V_BOOL, 0, 0, (uint64_t)d->anchored},
		ss_bytes("device", d->address, 20), /* an UNPROVISIONED device has no key yet */
	};
	ss_emit(out, 0, e, d->state == NU54_STATE_UNPROVISIONED ? 6 : 7);
}

/* wallet.check.result{accepted[, signature][, reason]} with the zero session id. */
static void wallet_result(nu54_out_t *out, int who, const uint8_t *sig, const char *reason)
{
	nu54_cbor_entry_t e[5] = {{"v", NU54_V_UINT, 0, 0, 1}, ss_text("type", "wallet.check.result"), ss_bytes("sessionId", ss_zero_session, 8),
				  {"accepted", NU54_V_BOOL, 0, 0, (uint64_t)(sig != NULL)}};
	e[4] = sig ? ss_bytes("signature", sig, 65) : ss_text("reason", reason);
	if (!sig) {
		out->event = NU54_EVENT_REFUSED;
	}
	ss_emit(out, who, e, 5);
}

/* wallet.check (payment-protocol.md 5): the bonded phone app checks the new key. The device waits
 * for the approve button, then signs WalletCheck{device, challenge}; the result goes to the phone
 * app. Refusals before the button answer the link that asked. */
void ss_wallet_check(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out)
{
	/* One button at a time: a payment, setup or limit change that waits keeps it. */
	if (!d->call.bonded || d->state == NU54_STATE_UNPROVISIONED || d->pending != NU54_PENDING_NONE || !d->cfg.platform.sign) {
		wallet_result(out, 0, NULL, "NOT_PERMITTED");
		return;
	}
	if (d->state == NU54_STATE_PIN_LOCKED) {
		wallet_result(out, 0, NULL, "PIN_LOCKED"); /* every signature is refused */
		return;
	}
	memcpy(d->pending_challenge, ss_field(m, 0, "challenge")->ptr, 32);
	d->pending = NU54_PENDING_WALLET_CHECK;
}

void nu54_session_wallet_check_end(nu54_device_t *d, nu54_out_t *out)
{
	if (d->pending == NU54_PENDING_WALLET_CHECK) {
		d->pending = NU54_PENDING_NONE;
		wallet_result(out, 1, NULL, "TIMEOUT");
	}
}

void ss_wallet_check_button(nu54_device_t *d, int approve, nu54_out_t *out)
{
	d->pending = NU54_PENDING_NONE;
	if (!approve) {
		wallet_result(out, 1, NULL, "USER_REJECTED");
		return;
	}
	nu54_wallet_check_t w;
	uint8_t domain[32], h[32], digest[32], rs[64], sig[65];
	memcpy(w.device, d->address, 20);
	memcpy(w.challenge, d->pending_challenge, 32);
	nu54_eip712_domain_separator(d->chain_id, d->contract, domain);
	nu54_hash_wallet_check(&w, h);
	nu54_eip712_digest(domain, h, digest);
	if (d->cfg.platform.sign(d->cfg.platform.ctx, digest, rs) != 0 || nu54_sig_finish(digest, rs, d->address, sig) != NU54_SIG_OK) {
		wallet_result(out, 1, NULL, "NOT_PERMITTED");
		return;
	}
	wallet_result(out, 1, sig, NULL);
}

#if defined(CONFIG_NU54_EXPERIMENTAL_RENTAL_JOURNAL_PS) || defined(NU54_TEST_RENTAL_CLEANUP)
static int cleanup_hex_digit(uint8_t value)
{
	if (value >= '0' && value <= '9') return value - '0';
	if (value >= 'a' && value <= 'f') return value - 'a' + 10;
	return -1;
}

static int cleanup_wire(const nu54_cbor_item_t *field, uint8_t raw[NU54_RENTAL_CLEANUP_INTENT_SIZE])
{
	if (!field || field->len != 2 * NU54_RENTAL_CLEANUP_INTENT_SIZE) return -1;
	for (size_t i = 0; i < NU54_RENTAL_CLEANUP_INTENT_SIZE; ++i) {
		int high = cleanup_hex_digit(field->ptr[2 * i]);
		int low = cleanup_hex_digit(field->ptr[2 * i + 1]);
		if (high < 0 || low < 0) return -1;
		raw[i] = (uint8_t)((high << 4) | low);
	}
	return 0;
}

static bool cleanup_peer(const nu54_device_t *d, const nu54_cbor_item_t *sid)
{
	return d->call.bonded && d->call.connection_epoch && d->rental_journal_required &&
	       !memcmp(sid->ptr, ss_zero_session, 8) && d->cfg.platform.allow_rental_cleanup_station;
}

static bool cleanup_available(const nu54_platform_t *platform)
{
	return platform->begin_rental_cleanup && platform->end_rental_cleanup &&
	       platform->rental_cleanup_time && platform->rental_cleanup_binding &&
	       platform->rental_cleanup_witness && platform->fence_rental_cleanup;
}

static bool cleanup_scope(nu54_device_t *d, const nu54_cbor_item_t *sid,
			  nu54_rental_binding_t *binding)
{
	const nu54_platform_t *platform = &d->cfg.platform;
	return cleanup_peer(d, sid) &&
	       platform->rental_cleanup_binding(platform->ctx, binding) == 0 &&
	       !memcmp(binding->chain, d->chain_id, sizeof(d->chain_id)) &&
	       !memcmp(binding->contract, d->contract, sizeof(d->contract)) &&
	       !memcmp(binding->account, d->address, sizeof(d->address)) &&
	       platform->allow_rental_cleanup_station(platform->ctx, binding, d->call.peer);
}

void ss_rental_cleanup_challenge(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out)
{
	const nu54_cbor_item_t *sid = ss_field(m, 0, "sessionId");
	nu54_platform_t *platform = &d->cfg.platform;
	nu54_rental_binding_t binding;
	uint64_t now = 0;
	uint8_t nonce[32] = {0};
	static const uint8_t zero[32] = {0};
	if (!cleanup_peer(d, sid) || !cleanup_available(platform) || !platform->rental_challenge_random ||
	    platform->begin_rental_cleanup(platform->ctx) != 0) {
		ss_error(out, ss_zero_session, "NOT_PERMITTED");
		return;
	}
	const bool scoped = cleanup_scope(d, sid, &binding);
	bool allowed = scoped && platform->rental_cleanup_time(platform->ctx, &now) == 0 && now && now <= UINT64_MAX - 120 &&
		platform->rental_challenge_random(platform->ctx, nonce) == 0 &&
		memcmp(nonce, zero, sizeof(nonce)) && memcmp(nonce, d->last_rental_channel_nonce, sizeof(nonce));
	if (allowed) {
		d->rental_access.cleanup = 1;
		d->rental_access.epoch = d->call.connection_epoch;
		d->rental_access.expires_at = now + 120;
		memcpy(d->rental_access.peer, d->call.peer, sizeof(d->call.peer));
		memcpy(d->rental_access.nonce, nonce, sizeof(nonce));
		nu54_keccak256((const uint8_t *)&binding, sizeof(binding), d->rental_access.cleanup_binding);
		memcpy(d->last_rental_channel_nonce, nonce, sizeof(nonce));
	} else if (scoped) {
		memset(&d->rental_access, 0, sizeof(d->rental_access));
	}
	platform->end_rental_cleanup(platform->ctx);
	if (!allowed) {
		ss_error(out, ss_zero_session, "NOT_PERMITTED");
		return;
	}
	nu54_cbor_entry_t reply[] = {
		{"v", NU54_V_UINT, NULL, 0, 1}, ss_text("type", "rental.cleanup.challenge.result"),
		ss_bytes("sessionId", ss_zero_session, 8), ss_bytes("channelNonce", nonce, sizeof(nonce)),
		{"expiresAt", NU54_V_UINT, NULL, 0, now + 120},
	};
	ss_emit(out, 0, reply, sizeof(reply) / sizeof(reply[0]));
}

static bool cleanup_connection(const nu54_device_t *d, const nu54_cbor_item_t *nonce)
{
	return d->rental_access.cleanup && d->rental_access.epoch == d->call.connection_epoch &&
	       !memcmp(d->rental_access.peer, d->call.peer, sizeof(d->call.peer)) &&
	       !memcmp(d->rental_access.nonce, nonce->ptr, sizeof(d->rental_access.nonce));
}

static int admit_cleanup(nu54_device_t *d, const nu54_msg_t *m)
{
	const nu54_cbor_item_t *sid = ss_field(m, 0, "sessionId");
	const nu54_cbor_item_t *nonce = ss_field(m, 0, "channelNonce");
	const nu54_platform_t *platform = &d->cfg.platform;
	uint8_t raw[NU54_RENTAL_CLEANUP_INTENT_SIZE];
	if (!cleanup_peer(d, sid) || !cleanup_connection(d, nonce) || !cleanup_available(platform) ||
	    cleanup_wire(ss_field(m, 0, "intent"), raw) || platform->begin_rental_cleanup(platform->ctx) != 0)
		return NU54_RENTAL_JOURNAL_ERROR;

	int result = NU54_RENTAL_JOURNAL_ERROR;
	nu54_rental_binding_t binding;
	nu54_rental_cleanup_intent_t intent;
	nu54_rental_cleanup_witness_t witness = {0};
	nu54_rental_retirement_authority_t authority;
	uint64_t now;
	if (cleanup_connection(d, nonce) && cleanup_scope(d, sid, &binding) &&
	    nu54_rental_cleanup_intent_verify(&binding, d->operator_address, raw, sizeof(raw), &intent) == 0 &&
	    !memcmp(intent.binding, d->rental_access.cleanup_binding, sizeof(intent.binding)) &&
	    platform->rental_cleanup_witness(platform->ctx, &binding, &intent, &witness) == 0 &&
	    platform->rental_cleanup_time(platform->ctx, &now) == 0 && now < d->rental_access.expires_at &&
	    nu54_rental_cleanup_authorize(&intent, &witness, now, &authority) == 0 &&
	    cleanup_connection(d, nonce) && platform->allow_rental_cleanup_station(platform->ctx, &binding, d->call.peer))
		result = platform->fence_rental_cleanup(platform->ctx, &authority, now);
	platform->end_rental_cleanup(platform->ctx);
	return result;
}

void ss_rental_cleanup_command(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out)
{
	const int result = admit_cleanup(d, m);
	if (result != NU54_RENTAL_JOURNAL_RESERVED && result != NU54_RENTAL_JOURNAL_REPLAY) {
		ss_error(out, ss_zero_session, "NOT_PERMITTED");
		return;
	}
	nu54_cbor_entry_t reply[] = {
		{"v", NU54_V_UINT, NULL, 0, 1}, ss_text("type", "rental.cleanup.status"),
		ss_bytes("sessionId", ss_zero_session, 8), ss_text("status", "pending"),
	};
	ss_emit(out, 0, reply, sizeof(reply) / sizeof(reply[0]));
}
#else
void ss_rental_cleanup_challenge(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out)
{
	(void)d;
	(void)m;
	ss_error(out, ss_zero_session, "NOT_PERMITTED");
}

void ss_rental_cleanup_command(nu54_device_t *d, const nu54_msg_t *m, nu54_out_t *out)
{
	(void)d;
	(void)m;
	ss_error(out, ss_zero_session, "NOT_PERMITTED");
}
#endif
