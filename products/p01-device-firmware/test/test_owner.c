#include "nu54_owner.h"

#include <assert.h>
#include <openssl/evp.h>
#include <openssl/ecdsa.h>
#include <openssl/x509.h>
#include <string.h>

static int save(void *context, unsigned record, const void *data, size_t size)
{
	(void)context;
	(void)record;
	(void)data;
	(void)size;
	return 0;
}

static int random_bytes(uint8_t *bytes, size_t size)
{
	memset(bytes, 5, size);
	return 0;
}

static int hash(const uint8_t *bytes, size_t size, uint8_t digest[32])
{
	return EVP_Digest(bytes, size, digest, NULL, EVP_sha256(), NULL) == 1 ? 0 : -1;
}

static int verify(const uint8_t *bytes, size_t size, const uint8_t digest[32], const uint8_t *sig, size_t sig_size)
{
	EVP_PKEY *key = d2i_PUBKEY(NULL, &bytes, (long)size);
	EVP_PKEY_CTX *ctx = EVP_PKEY_CTX_new(key, NULL);
	bool ok = ctx && EVP_PKEY_verify_init(ctx) == 1 && EVP_PKEY_CTX_set_signature_md(ctx, EVP_sha256()) == 1 &&
		  EVP_PKEY_verify(ctx, sig, sig_size, digest, 32) == 1;
	EVP_PKEY_CTX_free(ctx);
	EVP_PKEY_free(key);
	return ok ? 0 : -1;
}

static EVP_PKEY *recovery_device;

static size_t sign_message(EVP_PKEY *key, const uint8_t *message, size_t size, uint8_t signature[80])
{
	size_t used = 80;
	EVP_MD_CTX *ctx = EVP_MD_CTX_new();
	assert(ctx && EVP_DigestSignInit(ctx, NULL, EVP_sha256(), NULL, key) == 1);
	assert(EVP_DigestSign(ctx, signature, &used, message, size) == 1);
	EVP_MD_CTX_free(ctx);
	return used;
}

static int verify_recovery(const uint8_t *data, size_t size, const uint8_t signature[64])
{
	ECDSA_SIG *value = ECDSA_SIG_new();
	assert(value && ECDSA_SIG_set0(value, BN_bin2bn(signature, 32, NULL), BN_bin2bn(signature + 32, 32, NULL)));
	uint8_t der[80], *cursor = der;
	int length = i2d_ECDSA_SIG(value, &cursor);
	ECDSA_SIG_free(value);
	EVP_MD_CTX *ctx = EVP_MD_CTX_new();
	bool valid = ctx && length > 0 && EVP_DigestVerifyInit(ctx, NULL, EVP_sha256(), NULL, recovery_device) == 1 &&
		     EVP_DigestVerify(ctx, der, (size_t)length, data, size) == 1;
	EVP_MD_CTX_free(ctx);
	return valid ? 0 : -1;
}

static void sign_independent(const nu54_owner_enrollment *enrollment, uint8_t signature[64])
{
	uint8_t message[4 + NU54_OWNER_CHALLENGE_SIZE], der[80];
	memcpy(message, "N54R", 4);
	memcpy(message + 4, enrollment->record, sizeof(enrollment->record));
	size_t length = sign_message(recovery_device, message, sizeof(message), der);
	const uint8_t *cursor = der;
	ECDSA_SIG *value = d2i_ECDSA_SIG(NULL, &cursor, (long)length);
	const BIGNUM *r, *s;
	assert(value);
	ECDSA_SIG_get0(value, &r, &s);
	assert(BN_bn2binpad(r, signature, 32) == 32 && BN_bn2binpad(s, signature + 32, 32) == 32);
	ECDSA_SIG_free(value);
}

static void replace_lost_phone(void)
{
	nu54_vault vault = {.enabled = true, .storage.write = save,
		.record = {.generation = 1, .identity = {42}, .owner = {13}, .personal = {17},
			   .flags = NU54_VAULT_PERSONAL | NU54_VAULT_BACKUP}};
	nu54_owner_crypto crypto = {random_bytes, hash, verify};
	nu54_v2_link new_link = {.profile = NU54_V2_MANAGEMENT, .local = NU54_V2_BOARD, .peer = NU54_V2_OWNER, .epoch = {1}};
	EVP_PKEY *phone = EVP_PKEY_Q_keygen(NULL, NULL, "EC", "prime256v1");
	recovery_device = EVP_PKEY_Q_keygen(NULL, NULL, "EC", "prime256v1");
	assert(phone && recovery_device);
	uint8_t spki[100], *cursor = spki;
	int spki_size = i2d_PUBKEY(phone, &cursor);
	assert(spki_size == 91 && !hash(spki, (size_t)spki_size, new_link.peer_identity));
	uint8_t begin[34] = {1, NU54_OWNER_REPLACE_LOST}, prior_owner[32] = {13};
	memcpy(begin + 2, new_link.peer_identity, 32);
	nu54_v2_request request = {.command = NU54_V2_MANAGE, .expires = 12, .id = {14},
		.payload = begin, .payload_length = sizeof(begin)};
	nu54_owner_enrollment lost = {0};
	assert(nu54_owner_begin(&lost, &vault, &crypto, &new_link, &request, 1, prior_owner));
	assert(!nu54_owner_begin_lost(&lost, &vault, &crypto, &new_link, &request, 1));
	/* The signature goes after the record and the key; sign_message may write up to 80 bytes there. */
	uint8_t proof[320] = {2};
	memcpy(proof + 1, lost.record, sizeof(lost.record));
	proof[131] = (uint8_t)spki_size;
	memcpy(proof + 132, spki, (size_t)spki_size);
	size_t length = sign_message(phone, lost.record, sizeof(lost.record), proof + 133 + spki_size);
	proof[132 + spki_size] = (uint8_t)length;
	request.payload = proof;
	request.payload_length = 133 + (size_t)spki_size + length;
	assert(!nu54_owner_prove(&lost, &vault, &crypto, &new_link, &request, 2, 9));
	nu54_owner_enrollment withheld = lost;
	assert(nu54_owner_confirm(&withheld, &vault, &new_link, 3, 9, true) && vault.record.generation == 1);

	uint8_t wrong[64], valid[64];
	nu54_owner_enrollment mismatched = lost;
	mismatched.record[2] ^= 1;
	sign_independent(&mismatched, wrong);
	assert(nu54_owner_authorize_lost(&lost, &vault, &new_link, 3, verify_recovery, wrong));
	mismatched = lost;
	mismatched.record[42] ^= 1;
	sign_independent(&mismatched, wrong);
	assert(nu54_owner_authorize_lost(&lost, &vault, &new_link, 3, verify_recovery, wrong));

	sign_independent(&lost, valid);
	nu54_v2_link stale_link = new_link;
	stale_link.epoch[0]++;
	assert(nu54_owner_authorize_lost(&lost, &vault, &stale_link, 3, verify_recovery, valid));
	assert(nu54_owner_authorize_lost(&lost, &vault, &new_link, 12, verify_recovery, valid));
	assert(!nu54_owner_authorize_lost(&lost, &vault, &new_link, 3, verify_recovery, valid));
	assert(nu54_owner_authorize_lost(&lost, &vault, &new_link, 3, verify_recovery, valid));

	nu54_owner_enrollment late = lost;
	assert(!nu54_owner_confirm(&lost, &vault, &new_link, 4, 9, true));
	assert(vault.record.generation == 2 && !memcmp(vault.record.owner, new_link.peer_identity, 32));
	assert(vault.record.personal[0] == 17 && vault.record.identity[0] == 42);
	assert(nu54_owner_confirm(&late, &vault, &new_link, 4, 9, true));

	uint8_t address[20];
	assert(nu54_vault_personal_address(&vault, 1, prior_owner, address));
	assert(!nu54_vault_personal_address(&vault, 2, new_link.peer_identity, address) && address[0] == 17);
	EVP_PKEY_free(recovery_device);
	EVP_PKEY_free(phone);
}

int main(void)
{
	replace_lost_phone();
	nu54_vault vault = {.enabled = true, .storage.write = save, .record.identity = {42}};
	nu54_owner_crypto crypto = {random_bytes, hash, verify};
	nu54_v2_link link = {.profile = NU54_V2_MANAGEMENT, .local = NU54_V2_BOARD, .peer = NU54_V2_OWNER, .epoch = {1}};
	nu54_owner_enrollment enrollment = {0};
	EVP_PKEY *owner = EVP_PKEY_Q_keygen(NULL, NULL, "EC", "prime256v1");
	assert(owner);
	uint8_t spki[100], *cursor = spki;
	int spki_size = i2d_PUBKEY(owner, &cursor);
	assert(spki_size == 91 && hash(spki, (size_t)spki_size, link.peer_identity) == 0);
	uint8_t begin[34] = {1, NU54_OWNER_REGISTER}, invalid_peer[32] = {1};
	memcpy(begin + 2, link.peer_identity, 32);
	nu54_v2_request request = {.command = NU54_V2_MANAGE, .expires = 12, .payload = begin, .payload_length = sizeof(begin)};
	assert(nu54_owner_begin(&enrollment, &vault, &crypto, &link, &request, 1, invalid_peer) < 0);
	assert(nu54_owner_begin(&enrollment, &vault, &crypto, &link, &request, 1, link.peer_identity) == 0);
	uint8_t proof[300] = {2}, reference[130];
	memcpy(reference, enrollment.record, sizeof(reference));
	memcpy(proof + 1, reference, sizeof(reference));
	proof[131] = (uint8_t)spki_size;
	memcpy(proof + 132, spki, (size_t)spki_size);
	size_t sig_size = 72;
	EVP_MD_CTX *sign = EVP_MD_CTX_new();
	assert(sign && EVP_DigestSignInit(sign, NULL, EVP_sha256(), NULL, owner) == 1);
	assert(EVP_DigestSign(sign, proof + 133 + spki_size, &sig_size, reference, sizeof(reference)) == 1);
	proof[132 + spki_size] = (uint8_t)sig_size;
	request.payload = proof;
	request.payload_length = 133 + (size_t)spki_size + sig_size;
	proof[2] ^= 1;
	assert(nu54_owner_prove(&enrollment, &vault, &crypto, &link, &request, 1, 9) < 0);
	proof[2] ^= 1;
	link.epoch[0]++;
	assert(nu54_owner_prove(&enrollment, &vault, &crypto, &link, &request, 1, 9) < 0);
	link.epoch[0]--;
	assert(nu54_owner_prove(&enrollment, &vault, &crypto, &link, &request, 12, 9) < 0);
	assert(nu54_owner_prove(&enrollment, &vault, &crypto, &link, &request, 1, 9) == 0);
	assert(vault.record.generation == 0);
	assert(nu54_owner_confirm(&enrollment, &vault, &link, 2, 8, true) < 0);
	assert(vault.record.generation == 0);
	request.payload = begin;
	request.payload_length = sizeof(begin);
	assert(nu54_owner_begin(&enrollment, &vault, &crypto, &link, &request, 2, link.peer_identity) == 0);
	request.payload = proof;
	request.payload_length = 133 + (size_t)spki_size + sig_size;
	assert(nu54_owner_prove(&enrollment, &vault, &crypto, &link, &request, 2, 9) == 0);
	assert(nu54_owner_confirm(&enrollment, &vault, &link, 3, 9, true) == 0);
	assert(vault.record.generation == 1 && memcmp(vault.record.owner, link.peer_identity, 32) == 0);
	uint8_t status[NU54_OWNER_STATUS_SIZE];
	assert(nu54_owner_status(&vault, &crypto, &enrollment, status) == 0 && status[1] == 1 && status[12] == 1);
	assert(nu54_vault_revoke(&vault, 0, link.peer_identity) < 0);
	assert(nu54_vault_revoke(&vault, 1, link.peer_identity) == 0 && vault.record.generation == 2);
	assert(nu54_owner_status(&vault, &crypto, &enrollment, status) == 0 && status[1] == 3 && status[12] == 2);
	const uint8_t *session;
	const uint8_t grant_phone[32] = {77};
	assert(!nu54_owner_session_pin(&vault, grant_phone, &session)); /* revoked: nobody */
	/* A revoked owner stays revoked: only a local reset lets a new phone register. */
	nu54_owner_cancel(&enrollment);
	request.payload = begin;
	request.payload_length = sizeof(begin);
	assert(nu54_owner_begin(&enrollment, &vault, &crypto, &link, &request, 3, link.peer_identity) < 0);
	vault.record.flags = 0;
	memcpy(vault.record.owner, link.peer_identity, 32);
	assert(nu54_owner_begin(&enrollment, &vault, &crypto, &link, &request, 3, link.peer_identity) < 0); /* owned */
	assert(nu54_vault_release(&vault) == 0 && vault.record.generation == 3);
	assert(nu54_vault_release(&vault) == 0 && vault.record.generation == 3); /* repeating changes nothing */
	assert(nu54_owner_status(&vault, &crypto, &enrollment, status) == 0 && status[1] == 4 && status[12] == 3);
	assert(nu54_vault_revoke(&vault, 3, link.peer_identity) < 0);
	assert(nu54_owner_begin(&enrollment, &vault, &crypto, &link, &request, 3, link.peer_identity) == 0);
	nu54_owner_cancel(&enrollment);
	/* After a reset the grant's phone must be able to connect, or the board could never be used again. */
	assert(nu54_owner_session_pin(&vault, grant_phone, &session) && session == grant_phone);
	assert(nu54_vault_rebind(&vault, 3, link.peer_identity) == 0 && vault.record.generation == 4 && !vault.record.flags);
	assert(nu54_owner_begin(&enrollment, &vault, &crypto, &link, &request, 3, link.peer_identity) < 0);
	nu54_owner_cancel(&enrollment);
	assert(nu54_owner_session_pin(&vault, grant_phone, &session) && session == vault.record.owner);
	vault.enabled = false;
	assert(!nu54_owner_session_pin(&vault, grant_phone, &session));
	vault.enabled = true;
	vault.record.generation = 0;
	assert(nu54_owner_session_pin(&vault, grant_phone, &session) && session == grant_phone);
	vault.record.generation = 4;
	assert(nu54_owner_status(&vault, &crypto, &enrollment, status) == 0 && status[1] == 1);
	/* The bond flag keeps the backup bit, sets or clears only bit 1, and leaves other bytes alone. */
	uint8_t flagged[NU54_OWNER_STATUS_SIZE];
	for (uint8_t backup = 0; backup < 2; backup++) {
		status[4] = backup;
		memcpy(flagged, status, sizeof(flagged));
		nu54_owner_status_bonds(flagged, true);
		assert(flagged[4] == (backup | NU54_OWNER_STATUS_BONDS_CLEAR));
		nu54_owner_status_bonds(flagged, false);
		assert(memcmp(flagged, status, sizeof(flagged)) == 0);
	}
	EVP_MD_CTX_free(sign);
	EVP_PKEY_free(owner);
	return 0;
}
