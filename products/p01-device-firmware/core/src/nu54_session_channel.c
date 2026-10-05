#include "nu54_session_int.h"

static void make_iv(uint8_t iv[12], uint8_t dir, uint64_t n)
{
	memset(iv, 0, 12);
	iv[0] = dir;
	for (int k = 0; k < 8; k++) {
		iv[11 - k] = (uint8_t)(n >> (8 * k));
	}
}

int ss_channel_open(nu54_device_t *d, const channel_t *c, const uint8_t *body, size_t len, uint8_t *plain, size_t cap)
{
	uint8_t iv[12];
	make_iv(iv, DIR_KIOSK, d->channel_rx);
	if (len < NU54_GCM_TAG_LEN || len - NU54_GCM_TAG_LEN > cap ||
	    d->cfg.platform.aead_open(d->cfg.platform.ctx, c->key, iv, body, len, plain) != 0) {
		return -1;
	}
	d->channel_rx++;
	return 0;
}

void ss_drop_channel(nu54_device_t *d)
{
	memset(d->channel_key, 0, sizeof(d->channel_key));
	d->secure = 0;
}

void ss_channel_begin(const nu54_device_t *d, channel_t *c, const nu54_out_t *out)
{
	memset(c, 0, sizeof(*c));
	c->first = out->kiosk_count;
	c->on = d->session_open && d->secure && !d->call.foreign;
	if (c->on) {
		c->id = d->channel_id;
		memcpy(c->key, d->channel_key, 16);
		c->tx = d->channel_tx;
	}
}

/* Seals the replies for the central; a reply that cannot be sealed is dropped with the rest and
 * the session closes. A channel whose session ended is wiped. */
void ss_channel_end(nu54_device_t *d, channel_t *c, nu54_out_t *out)
{
	if (c->on) {
		for (int i = c->first; i < out->kiosk_count; i++) {
			uint8_t iv[12], sealed[NU54_OUT_MAX];
			size_t len = out->kiosk_len[i];
			make_iv(iv, DIR_DEVICE, c->tx++);
			if (len + NU54_GCM_TAG_LEN > NU54_OUT_MAX || d->cfg.platform.aead_seal(d->cfg.platform.ctx, c->key, iv, out->kiosk[i], len, sealed) != 0) {
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
		ss_drop_channel(d);
	}
}

/* A MerchantAttestation (map `a` of `m`) signed by the recorded operator. */
int ss_operator_attestation(const nu54_device_t *d, const nu54_msg_t *m, int a, nu54_merchant_attestation_t *att)
{
	const nu54_cbor_item_t *merchant = ss_field(m, a, "merchant"), *payout = ss_field(m, a, "payout"), *name = ss_field(m, a, "name"),
			       *from = ss_field(m, a, "validFrom"), *until = ss_field(m, a, "validUntil"), *sig = ss_field(m, a, "operatorSignature");
	memcpy(att->merchant, merchant->ptr, 20);
	memcpy(att->payout, payout->ptr, 20);
	att->name = name->ptr;
	att->name_len = name->len;
	att->valid_from = ss_u64(from);
	att->valid_until = ss_u64(until);
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
int ss_kiosk_merchant(const nu54_device_t *d, const nu54_msg_t *m, uint8_t merchant[20])
{
	int a = nu54_msg_find(m, 0, "attestation");
	const nu54_cbor_item_t *eph = ss_field(m, 0, "kioskEphemeral"), *kn = ss_field(m, 0, "kioskNonce"), *ks = ss_field(m, 0, "kioskKeySignature");
	nu54_merchant_attestation_t att;
	nu54_kiosk_key_t k;
	uint8_t domain[32], h[32], digest[32], signer[20];

	if (a < 0 || !ks || !ss_operator_attestation(d, m, a, &att)) {
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
int ss_start_channel(nu54_device_t *d, const uint8_t kiosk_x[32], const uint8_t kiosk_nonce[32], uint8_t device_x[32])
{
	uint8_t priv[32], shared[32], salt[64];
	int err = -1;

	for (int tries = 0; tries < 8 && err; tries++) {
		d->cfg.platform.random(d->cfg.platform.ctx, priv, 32); /* a draw that is not a scalar is drawn again */
		err = nu54_secure_public_x(priv, device_x);
	}
	if (!err) {
		err = nu54_secure_shared_x(priv, kiosk_x, shared);
	}
	memcpy(salt, kiosk_nonce, 32);
	memcpy(&salt[32], d->device_nonce, 32);
	if (!err) {
		err = d->cfg.platform.hkdf(d->cfg.platform.ctx, shared, salt, NU54_SESSION_KEY_INFO, d->channel_key);
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
