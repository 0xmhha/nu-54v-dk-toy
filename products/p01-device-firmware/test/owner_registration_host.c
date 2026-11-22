/* W04 simulator board. TLS, wire2, owner verification and vault transitions are real.
 * The platform's Key/ITS callbacks map to files and local confirmation is injected. */
#include "hd_accessory_store.h"
#include "nu54_accessory.h"
#include "nu54_keccak.h"
#include "nu54_owner.h"
#include "nu54_running_firmware.h"
#include "nu54_owner_idle.h"
#include "nu54_owner_limits_wire.h"
#include "nu54_quote_v2.h"
#include "nu54_pilot.h"
#include "nu54_transport_v2_openssl.h"

#include <arpa/inet.h>
#include <errno.h>
#include <fcntl.h>
#include <openssl/core_names.h>
#include <openssl/ecdsa.h>
#include <openssl/err.h>
#include <openssl/pem.h>
#include <openssl/rand.h>
#include <openssl/sha.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <sys/wait.h>
#include <time.h>
#include <unistd.h>

static int address_from_key(EVP_PKEY *key, uint8_t address[20])
{
    uint8_t point[65], digest[32];
    size_t size;
    if (EVP_PKEY_get_octet_string_param(key, OSSL_PKEY_PARAM_PUB_KEY, point, sizeof(point), &size) != 1 ||
        size != sizeof(point) || point[0] != 4) return -1;
    nu54_keccak256(point + 1, sizeof(point) - 1, digest);
    memcpy(address, digest + 12, 20);
    return 0;
}

static int read_record(void *context, unsigned record, void *data, size_t length)
{
    (void)context;
    char name[20];
    snprintf(name, sizeof(name), "record-%u", record);
    FILE *file = fopen(name, "rb");
    if (!file) return errno == ENOENT ? NU54_VAULT_ABSENT : NU54_VAULT_IO;
    bool read = fread(data, 1, length, file) == length && fgetc(file) == EOF && !ferror(file);
    fclose(file);
    return read ? 0 : NU54_VAULT_IO;
}

static int finish_record(FILE *file, const char *temporary, const char *name, bool written)
{
    bool synced = written && fflush(file) == 0 && fsync(fileno(file)) == 0;
    bool closed = fclose(file) == 0;
    if (synced && closed && rename(temporary, name) == 0) return 0;
    unlink(temporary);
    return NU54_VAULT_IO;
}

static int write_record(void *context, unsigned record, const void *data, size_t length)
{
    (void)context;
    char name[20];
    snprintf(name, sizeof(name), "record-%u", record);
    FILE *file = fopen("record.tmp", "wb");
    if (!file) return NU54_VAULT_IO;
    return finish_record(file, "record.tmp", name, fwrite(data, 1, length, file) == length);
}

static int slot_key(void *context, bool personal, bool create, uint8_t address[20])
{
    (void)context;
    /* W04 owns only the board identity. W05 is the personal root/backup transition. */
    if (personal && !getenv("NU54_W27_BRIDGE")) return create ? NU54_VAULT_DENIED : NU54_VAULT_ABSENT;
    const char *name = personal ? "policy-personal.pem" : "identity.pem";
    const char *temp = personal ? "policy-personal.tmp" : "identity.tmp";
    FILE *file = fopen(name, "r");
    EVP_PKEY *key = NULL;
    if (file) {
        key = PEM_read_PrivateKey(file, NULL, NULL, NULL);
        fclose(file);
    } else if (errno == ENOENT && create) {
        key = EVP_PKEY_Q_keygen(NULL, NULL, "EC", "secp256k1");
        file = key ? fopen(temp, "wb") : NULL;
        if (!file || finish_record(file, temp, name,
            PEM_write_PrivateKey(file, key, NULL, NULL, 0, NULL, NULL) == 1)) {
            EVP_PKEY_free(key);
            return NU54_VAULT_IO;
        }
    } else {
        return errno == ENOENT ? NU54_VAULT_ABSENT : NU54_VAULT_IO;
    }
    int result = key ? address_from_key(key, address) : NU54_VAULT_IO;
    EVP_PKEY_free(key);
    return result;
}

static int random_bytes(uint8_t *bytes, size_t length)
{
    return RAND_bytes(bytes, (int)length) == 1 ? 0 : -1;
}

static int hash(const uint8_t *bytes, size_t length, uint8_t out[32])
{
    return EVP_Digest(bytes, length, out, NULL, EVP_sha256(), NULL) == 1 ? 0 : -1;
}

static int verify(const uint8_t *encoded, size_t length, const uint8_t digest[32],
                  const uint8_t *signature, size_t signature_size)
{
    const unsigned char *cursor = encoded;
    EVP_PKEY *key = d2i_PUBKEY(NULL, &cursor, (long)length);
    EVP_PKEY_CTX *ctx = key ? EVP_PKEY_CTX_new(key, NULL) : NULL;
    bool valid = ctx && cursor == encoded + length && EVP_PKEY_verify_init(ctx) == 1 &&
                 EVP_PKEY_CTX_set_signature_md(ctx, EVP_sha256()) == 1 &&
                 EVP_PKEY_verify(ctx, signature, signature_size, digest, 32) == 1;
    EVP_PKEY_CTX_free(ctx);
    EVP_PKEY_free(key);
    return valid ? 0 : -1;
}

static const nu54_vault_storage storage = {NULL, read_record, write_record, slot_key};
static const nu54_owner_crypto crypto = {random_bytes, hash, verify};
static EVP_PKEY *replacement_verifier;

static int verify_replacement(const uint8_t *message, size_t length, const uint8_t raw[64])
{
    ECDSA_SIG *signature = ECDSA_SIG_new();
    BIGNUM *r = BN_bin2bn(raw, 32, NULL), *s = BN_bin2bn(raw + 32, 32, NULL);
    if (!signature || !r || !s || ECDSA_SIG_set0(signature, r, s) != 1) {
        BN_free(r);
        BN_free(s);
        ECDSA_SIG_free(signature);
        return -1;
    }
    uint8_t encoded[NU54_RECOVERY_DER_MAX], *cursor = encoded;
    int size = i2d_ECDSA_SIG(signature, NULL);
    EVP_MD_CTX *digest = EVP_MD_CTX_new();
    bool ok = size > 0 && (size_t)size <= sizeof(encoded) && i2d_ECDSA_SIG(signature, &cursor) == size &&
        digest && EVP_DigestVerifyInit(digest, NULL, EVP_sha256(), NULL, replacement_verifier) == 1 &&
        EVP_DigestVerify(digest, encoded, (size_t)size, message, length) == 1;
    EVP_MD_CTX_free(digest);
    ECDSA_SIG_free(signature);
    return ok ? 0 : -1;
}

static int independent_replacement(nu54_owner_enrollment *enrollment, const nu54_vault *vault, const nu54_v2_link *link)
{
    unlink("replacement.proof");
    FILE *file = fopen("replacement.tmp", "wb");
    if (!file || finish_record(file, "replacement.tmp", "replacement.context",
        fwrite(enrollment->record, 1, sizeof(enrollment->record), file) == sizeof(enrollment->record))) return -1;
    uint8_t proof[64];
    int status = 1;
    const struct timespec interval = {.tv_nsec = 100 * 1000 * 1000};
    for (size_t attempt = 0; attempt < 150 && status == 1; ++attempt) {
        status = hd_host_read_exact("replacement.proof", proof, sizeof(proof));
        if (status == 1) nanosleep(&interval, NULL);
    }
    unlink("replacement.context");
    unlink("replacement.proof");
    return status ? -1 : nu54_owner_authorize_lost(enrollment, vault, link, (uint64_t)time(NULL), verify_replacement, proof);
}

static int transfer(SSL *tls, uint8_t *packet, size_t length, bool write)
{
    while (length) {
        size_t used;
        int result = write ? SSL_write_ex(tls, packet, length, &used) : SSL_read_ex(tls, packet, length, &used);
        if (result != 1 || used == 0) return -1;
        packet += used;
        length -= used;
    }
    return 0;
}

static int send_frame(void *context, const uint8_t *frame, size_t length)
{
    uint8_t packet[NU54_V2_FRAME_MAX + 2] = {(uint8_t)(length >> 8), (uint8_t)length};
    memcpy(packet + 2, frame, length);
    return transfer(context, packet, length + 2, true);
}

static void report(const char *event, const nu54_vault *vault)
{
    printf("%s %llu ", event, (unsigned long long)vault->record.generation);
    for (size_t i = 0; i < 20; ++i) printf("%02x", vault->record.identity[i]);
    printf(" %u ", vault->record.flags);
    for (size_t i = 0; i < 32; ++i) printf("%02x", vault->record.owner[i]);
    puts("");
    fflush(stdout);
}

static int owner_status(const nu54_vault *vault, const nu54_owner_enrollment *enrollment, uint8_t *out, bool backup)
{
    if (nu54_owner_status(vault, &crypto, enrollment, out)) return -1;
    if (!backup) return 0;
    nu54_hd_slot hd;
    if (hd_host_open(&hd)) return -1;
    if (hd.record.phase >= NU54_HD_BACKUP_REQUIRED) {
        out[2] = 2;
        out[3] = 1;
        out[4] = hd.record.phase == NU54_HD_READY;
    }
    return 0;
}

static int begin_offline(const nu54_vault *vault, const nu54_v2_link *link, const nu54_v2_request *message,
                         uint8_t out[NU54_RECOVERY_OPEN_SIZE], bool target)
{
    if (!vault->record.generation || memcmp(vault->record.owner, link->peer_identity, 32)) return -1;
    nu54_hd_slot hd;
    if (hd_host_open(&hd) || (target ? hd.record.phase != NU54_HD_EMPTY : hd.record.phase == NU54_HD_READY)) return -1;
    nu54_recovery_context context = {.generation = vault->record.generation, .expires = message->expires};
    if (random_bytes(context.challenge, sizeof(context.challenge)) ||
        hash(vault->record.identity, sizeof(vault->record.identity), context.target)) return -1;
    memcpy(context.request, message->id, sizeof(context.request));
    memcpy(context.owner, link->peer_identity, sizeof(context.owner));
    memcpy(context.epoch, link->epoch, sizeof(context.epoch));
    nu54_recovery_open_message(&context, out);
    const char *context_path = target ? "restore.context" : "backup.context";
    FILE *file = fopen("offline.tmp", "wb");
    if (!file || finish_record(file, "offline.tmp", context_path,
        fwrite(out, 1, NU54_RECOVERY_OPEN_SIZE, file) == NU54_RECOVERY_OPEN_SIZE)) return -1;
    out[0] = target ? 5 : 6;
    return 0;
}

static int load_payment_refusal(void *unused, unsigned slot, nu54_pilot_refusal *out)
{
    (void)unused;
    char name[32];
    snprintf(name, sizeof(name), "payment-refusal-%u", slot);
    if (access(name, F_OK)) return errno == ENOENT ? 1 : -1;
    return hd_host_read_exact(name, out, sizeof(*out));
}

static int payment_refusal(const uint8_t *quote, const uint8_t request[16], uint64_t now,
                           nu54_pilot_refusal *record, unsigned *slot)
{
    uint8_t preimage[NU54_LOCAL_PREIMAGE_SIZE];
    if (!nu54_pilot_approval_preimage(quote, preimage) || hash(preimage, sizeof(preimage), record->digest)) return -1;
    memcpy(record->request, request, sizeof(record->request));
    for (size_t i = NU54_V2_EXPIRES_AT_OFFSET + 24; i < NU54_V2_EXPIRES_AT_OFFSET + 32; ++i)
        record->expires = (record->expires << 8) | quote[i];
    return nu54_pilot_refusal_find(load_payment_refusal, NULL, now, request, record->digest, slot);
}

static int review_payment(const nu54_v2_request *request, const nu54_vault *vault, const nu54_v2_link *link,
                          nu54_pilot_review_binding *binding, uint8_t reply[NU54_V2_PAYMENT_REVIEW_REPLY_SIZE], uint64_t now)
{
    const char *validator = getenv("NU54_BOARD_PAYMENT_INSTALL_TEST");
    const uint8_t *original, *quote;
    if (!validator || !vault->record.generation ||
        (vault->record.flags & NU54_VAULT_REVOKED) || memcmp(vault->record.owner, link->peer_identity, 32) ||
        nu54_v2_payment_review_decode(request->payload, request->payload_length, &original, &quote)) return -1;
    uint8_t fixture[NU54_V2_SIGNED_SIZE + 52];
    memcpy(fixture, quote, NU54_V2_SIGNED_SIZE);
    if (hd_host_read_exact("payment-key.fixture", fixture + NU54_V2_SIGNED_SIZE, 52)) return -1;
    FILE *file = fopen("review-input.tmp", "wb");
    if (!file || finish_record(file, "review-input.tmp", "review-input",
        fwrite(fixture, sizeof(fixture), 1, file) == 1)) return -1;
    pid_t child = fork();
    if (!child) {
        execl(validator, validator, "--review-commercial-testnet", "review-input", (char *)NULL);
        _exit(2);
    }
    int status;
    int result = child > 0 && waitpid(child, &status, 0) == child && WIFEXITED(status) && !WEXITSTATUS(status);
    unlink("review-input");
    if (!result) return -1;
    nu54_pilot_refusal record = {0};
    unsigned slot;
    int found = payment_refusal(quote, original, now, &record, &slot);
    if (found == NU54_PILOT_RECEIPT_NEW) {
        if (nu54_pilot_review_bind(binding, link, original, quote, hash)) return -1;
    } else if (found != NU54_PILOT_RECEIPT_RECORDED) return -1;
    return nu54_v2_payment_review_reply(request->payload, request->payload_length,
                                       found == NU54_PILOT_RECEIPT_NEW, hash, reply);
}

/* The simulator transport forwards the exact staged quote and a fresh board challenge to
 * the external display harness. The external display never receives the payer fixture key. */
static int serve_payment(SSL *tls, nu54_v2_link *link, nu54_v2_request *request, const nu54_vault *vault)
{
    const char *validator = getenv("NU54_BOARD_PAYMENT_INSTALL_TEST");
    if (!validator || link->peer != NU54_V2_OWNER || !vault->record.generation ||
        (vault->record.flags & NU54_VAULT_REVOKED) || memcmp(vault->record.owner, link->peer_identity, 32) ||
        request->payload_length != NU54_V2_SIGNED_SIZE ||
        request->expires <= (uint64_t)time(NULL)) return -1;

    nu54_pilot_refusal record = {0};
    unsigned refusal_slot;
    if (payment_refusal(request->payload, request->id, (uint64_t)time(NULL), &record, &refusal_slot)
        != NU54_PILOT_RECEIPT_NEW) return -1;
    uint8_t fixture[NU54_V2_SIGNED_SIZE + 52], challenge[32];
    memcpy(fixture, request->payload, NU54_V2_SIGNED_SIZE);
    if (hd_host_read_exact("payment-key.fixture", fixture + NU54_V2_SIGNED_SIZE, 52) ||
        RAND_bytes(challenge, sizeof(challenge)) != 1) return -1;
    unlink("display-request");
    unlink("display-proof");
    unlink("display-decision");
    FILE *file = fopen("payment-input.fixture", "wb");
    if (!file || finish_record(file, "payment-input.fixture", "payment-input",
        fwrite(fixture, 1, sizeof(fixture), file) == sizeof(fixture))) return -1;
    file = fopen("challenge.tmp", "wb");
    if (!file || finish_record(file, "challenge.tmp", "display-challenge",
        fwrite(challenge, 1, sizeof(challenge), file) == sizeof(challenge))) return -1;
    file = fopen("request.tmp", "wb");
    if (!file || finish_record(file, "request.tmp", "display-request",
        fwrite(request->payload, 1, request->payload_length, file) == request->payload_length)) return -1;

    if (link->approval_generation) {
        uint8_t payload[NU54_V2_APPROVAL_PROGRESS_SIZE];
        if (link->approval_generation != vault->record.generation ||
            nu54_v2_approval_progress_body(1, vault->record.generation, payload)) return -1;
        nu54_v2_request progress = *request;
        progress.command = NU54_V2_APPROVAL_PROGRESS;
        progress.payload = payload;
        progress.payload_length = sizeof(payload);
        if (nu54_v2_link_send(link, &progress, send_frame, tls)) return -1;
        report("APPROVAL_PROGRESS", vault);
    }
    uint8_t button = 0;
    struct timespec interval = {.tv_nsec = 100000000};
    while (!button && (uint64_t)time(NULL) < request->expires) {
        if (hd_host_read_exact("display-decision", &button, 1)) button = 0;
        if (!button) nanosleep(&interval, NULL);
    }
    if ((button != 1 && button != 2) || (uint64_t)time(NULL) >= request->expires) return -1;
    int output[2];
    if (pipe(output)) return -1;
    pid_t child = fork();
    if (!child) {
        close(output[0]);
        dup2(output[1], STDOUT_FILENO);
        close(output[1]);
        setenv("NU54_INSTALL_DISPLAY_KEY", "display-key", 1);
        setenv("NU54_INSTALL_DISPLAY_PROOF", "display-proof", 1);
        setenv("NU54_INSTALL_DISPLAY_CHALLENGE", "display-challenge", 1);
        if (button == 2) setenv("NU54_INSTALL_TEST_REFUSE", "1", 1);
        else unsetenv("NU54_INSTALL_TEST_REFUSE");
        execl(validator, validator, "--approve-commercial-testnet", "management", "payment-input", (char *)NULL);
        _exit(2);
    }
    close(output[1]);
    uint8_t approval[65];
    size_t used = 0;
    while (used < sizeof(approval)) {
        ssize_t n = read(output[0], approval + used, sizeof(approval) - used);
        if (n < 0 && errno == EINTR) continue;
        if (n <= 0) break;
        used += (size_t)n;
    }
    close(output[0]);
    int status;
    if (child < 0 || waitpid(child, &status, 0) != child || !WIFEXITED(status) || WEXITSTATUS(status) ||
        (uint64_t)time(NULL) >= request->expires) return -1;
    unlink("payment-input");
    unlink("display-request");
    if (button == 2 && !used) {
        char name[32];
        snprintf(name, sizeof(name), "payment-refusal-%u", refusal_slot);
        FILE *record_file = fopen("payment-refusal.tmp", "wb");
        if (!record_file || finish_record(record_file, "payment-refusal.tmp", name,
            fwrite(&record, sizeof(record), 1, record_file) == 1)) return -1;
        approval[0] = 0;
        used = 1;
    } else if (button != 1 || used != sizeof(approval)) {
        return -1;
    }
    request->command = NU54_V2_REPLY;
    request->payload = approval;
    request->payload_length = used;
    return nu54_v2_link_send(link, request, send_frame, tls);
}

static void encode_hex(const uint8_t *source,size_t length,char *output)
{
    const char digits[]="0123456789abcdef";
    for(size_t i=0;i<length;i++) {
        output[2*i]=digits[source[i]>>4];
        output[2*i+1]=digits[source[i]&15];
    }
    output[2*length]=0;
}

static int policy_operation(const nu54_vault *vault,const nu54_v2_link *link,
                            const nu54_v2_request *request,uint8_t *reply,size_t *length)
{
    const char *bridge=getenv("NU54_W27_BRIDGE");
    if(!bridge || !vault->enabled || vault->record.generation!=1 ||
       !(vault->record.flags & NU54_VAULT_PERSONAL) || (vault->record.flags & NU54_VAULT_REVOKED) ||
       memcmp(vault->record.owner,link->peer_identity,32) ||
       request->payload_length>NU54_LIMIT_SET_CHANGE_REQUEST_SIZE)return -1;
    uint8_t fingerprint[32], challenge[32];
    if(!SHA256(vault->record.identity,sizeof(vault->record.identity),fingerprint) ||
       RAND_bytes(challenge,sizeof(challenge))!=1)return -1;
    static unsigned activations;
    uint64_t clock=(uint64_t)time(NULL);
    if(request->payload[0]==25 || request->payload[0]==NU54_LIMIT_SET_ACTIVATE_OP) {
        FILE *file=fopen("policy-not-before","r");
        unsigned long long due;
        int ok=file && fscanf(file,"%llu",&due)==1;
        if(file)fclose(file);
        if(!ok || !due)return -1;
        ++activations;
        clock=activations==1?(uint64_t)due-1:activations==2?0:(uint64_t)due;
        printf("POLICY_DEVICE_CLOCK %llu\n",(unsigned long long)clock);
        fflush(stdout);
    }
    char payload[2*NU54_LIMIT_SET_CHANGE_REQUEST_SIZE+1],id[33],epoch[33],board[65],nonce[65],expires[21],time_value[21];
    encode_hex(request->payload,request->payload_length,payload);
    encode_hex(request->id,16,id);encode_hex(link->epoch,16,epoch);
    encode_hex(fingerprint,32,board);encode_hex(challenge,32,nonce);
    snprintf(expires,sizeof(expires),"%llu",(unsigned long long)request->expires);
    snprintf(time_value,sizeof(time_value),"%llu",(unsigned long long)clock);
    int fds[2];
    if(pipe(fds))return -1;
    pid_t child=fork();
    if(child==0) {
        close(fds[0]);
        if(dup2(fds[1],STDOUT_FILENO)<0)_exit(127);
        close(fds[1]);
        execl(bridge,bridge,"policy-state",payload,expires,id,time_value,board,epoch,nonce,(char *)NULL);
        _exit(127);
    }
    close(fds[1]);
    FILE *output=fdopen(fds[0],"r");
    char encoded[2*NU54_LIMIT_SET_REPLY_SIZE+2]={0};
    if(output) {
        (void)fgets(encoded,sizeof(encoded),output);
        fclose(output);
    } else close(fds[0]);
    int status=0;
    if(child<0 || waitpid(child,&status,0)!=child || !WIFEXITED(status))return -1;
    if(WEXITSTATUS(status)) {
        printf("POLICY_REFUSED %u %d\n",request->payload[0],WEXITSTATUS(status));
        fflush(stdout);
        if(request->payload[0]>=NU54_LIMIT_SET_READ_OP)return -1;
        reply[0]=request->payload[0];reply[1]=0;*length=2;
        return 0;
    }
    size_t count=strcspn(encoded,"\n");
    if(!count || count%2 || count/2>NU54_LIMIT_SET_REPLY_SIZE)return -1;
    for(size_t i=0;i<count/2;i++) {
        unsigned field;
        if(sscanf(encoded+2*i,"%2x",&field)!=1)return -1;
        reply[i]=(uint8_t)field;
    }
    *length=count/2;
    const uint8_t operation=request->payload[0];
    const bool set_proposal=operation==NU54_LIMIT_SET_CHANGE_OP || operation==NU54_LIMIT_SET_CAPS_OP;
    const bool proposed=(operation==24 && *length==10 && reply[1]==1) ||
                        (set_proposal && *length==11 && reply[1]==NU54_LIMIT_SET_WIRE_VERSION && reply[2]==1);
    if(proposed) {
        const uint64_t due=nu54_limits_get64(reply+(set_proposal?3:2));
        FILE *file=fopen("policy-not-before","w");
        if(!file)return -1;
        fprintf(file,"%llu\n",(unsigned long long)due);
        fclose(file);
    }
    printf("POLICY_REPLY %u %s",request->payload[0],encoded);
    fflush(stdout);
    if(operation==NU54_LIMIT_SET_ACTIVATE_OP && *length==3 && reply[2]==0)return -1;
    const char *drop=getenv("NU54_W27_DROP_REPLY");
    if(drop && ((proposed && set_proposal && !strcmp(drop,"proposal")) ||
       (operation==NU54_LIMIT_SET_ACTIVATE_OP && *length==3 && reply[2]==1 && !strcmp(drop,"activation")))) {
        printf("POLICY_DROPPED_REPLY %u\n",operation);
        fflush(stdout);
        return -1;
    }
    return 0;
}

static int read_firmware_image(const void *context, uint32_t offset, uint8_t *out, size_t size)
{
    FILE *file = *(FILE *const *)context;
    return fseek(file, offset, SEEK_SET) || fread(out, 1, size, file) != size ? -1 : 0;
}

/* The installed fixture signs and verifies this disposable MCUboot image before serving it. */
static int fixture_running_firmware(const char *name, const nu54_vault *vault,
                                    uint8_t out[NU54_RUNNING_FIRMWARE_REPLY_SIZE])
{
    if (!name || name[0] != '/') return -1;
    FILE *file = fopen(name, "rb");
    if (!file) return -1;
    uint8_t board[32], measurement[32], image[1024];
    nu54_running_firmware version;
    long size = fseek(file, 0, SEEK_END) ? -1 : ftell(file);
    int status = -1;
    if (size > 0 && size <= (long)sizeof(image) &&
        !nu54_running_firmware_inspect(read_firmware_image, &file, (uint32_t)size, &version) &&
        !read_firmware_image(&file, 0, image, version.measured_size) &&
        !hash(image, version.measured_size, measurement) &&
        !hash(vault->record.identity, sizeof(vault->record.identity), board))
        status = nu54_running_firmware_reply(&version, vault->record.generation, board, measurement, out);
    fclose(file);
    return status;
}

static void serve(SSL *tls, const uint8_t expected_peer[32], nu54_v2_journal *journal,
                  bool interrupt_before_commit, bool backup, bool target, enum nu54_recovery_role *backup_role,
                  const uint8_t replacement[32])
{
    nu54_v2_link link;
    nu54_vault vault;
    nu54_owner_enrollment enrollment = {0};
    bool backup_open = false, restore_open = false;
    if (nu54_vault_open(&vault, &storage)) {
        fputs("Vault reopen failed\n", stderr);
        return;
    }
    if (SSL_accept(tls) != 1) {
        fputs("TLS handshake failed\n", stderr);
        ERR_print_errors_fp(stderr);
        return;
    }
    uint8_t peer_pin[32];
    if (nu54_v2_openssl_spki(SSL_get0_peer_certificate(tls), peer_pin) ||
        (memcmp(peer_pin, expected_peer, 32) && (!replacement || memcmp(peer_pin, replacement, 32))) ||
        nu54_v2_openssl_bind(tls, &link, NU54_V2_MANAGEMENT, NU54_V2_BOARD, peer_pin, journal)) {
        fputs("Wire2 TLS binding failed\n", stderr);
        return;
    }

    const bool policy_mode = getenv("NU54_W27_BRIDGE") != NULL;
    if (policy_mode) {
        static unsigned sequence;
        printf("POLICY_SESSION %u\n", ++sequence);
        fflush(stdout);
    }
    nu54_owner_idle idle = {0};
    nu54_pilot_review_binding reviewed = {0};
    if (policy_mode && vault.record.generation && !memcmp(vault.record.owner, peer_pin, sizeof(vault.record.owner)))
        nu54_owner_idle_start(&idle, vault.record.generation, 0);
    bool recovering = replacement && !memcmp(peer_pin, replacement, 32) && memcmp(vault.record.owner, replacement, 32);
    for (;;) {
        uint8_t header[2], frame[NU54_V2_FRAME_MAX];
        if (transfer(tls, header, sizeof(header), false)) break;
        size_t length = ((size_t)header[0] << 8) | header[1];
        if (!length || length > sizeof(frame) || transfer(tls, frame, length, false)) break;
        nu54_v2_request request;
        uint64_t now = (uint64_t)time(NULL);
        nu54_v2_result received = nu54_v2_link_receive(&link, frame, length, now, &request);
        if (received == NU54_V2_MORE) continue;
        if (received == NU54_V2_NEW && request.command == NU54_V2_PAY_PREPARE) {
            if (nu54_pilot_review_consume(&reviewed, &link, &request, hash) ||
                serve_payment(tls, &link, &request, &vault)) break;
            continue;
        }
        if (received != NU54_V2_NEW || request.command != NU54_V2_MANAGE || !request.payload_length) {
            fputs("Wire2 management request rejected\n", stderr);
            break;
        }
        uint8_t out[NU54_LIMIT_SET_REPLY_SIZE > 1 + NU54_RECOVERY_ENVELOPE_MAX ?
                    NU54_LIMIT_SET_REPLY_SIZE : 1 + NU54_RECOVERY_ENVELOPE_MAX];
        size_t out_size = NU54_OWNER_STATUS_SIZE;
        uint8_t operation = request.payload[0];
        if (replacement && !recovering && memcmp(vault.record.owner, link.peer_identity, 32)) {
            report("OLD_OWNER_REFUSED", &vault);
            break;
        }
        if (operation == NU54_V2_APPROVAL_PROGRESS_OP && getenv("NU54_BOARD_PAYMENT_INSTALL_TEST")) {
            if (!vault.enabled || (vault.record.flags & NU54_VAULT_REVOKED) ||
                link.approval_generation || request.expires <= now || request.expires - now > 10 ||
                memcmp(vault.record.owner, peer_pin, sizeof(vault.record.owner)) ||
                !nu54_v2_approval_progress_offer(request.payload, request.payload_length, vault.record.generation)) break;
            request.command = NU54_V2_REPLY;
            if (nu54_v2_link_send(&link, &request, send_frame, tls) ||
                nu54_v2_link_enable_approval_progress(&link, vault.record.generation)) break;
            report("APPROVAL_PROGRESS_OPT_IN", &vault);
            continue;
        }
        if (operation == NU54_RUNNING_FIRMWARE_OP && getenv("NU54_TEST_RUNNING_FIRMWARE")) {
            if (!vault.enabled || (vault.record.flags & NU54_VAULT_REVOKED) ||
                memcmp(vault.record.owner, peer_pin, sizeof(vault.record.owner)) ||
                request.expires <= now || request.expires - now > 10 ||
                !nu54_running_firmware_request(request.payload, request.payload_length, vault.record.generation)) break;
            out[0] = NU54_RUNNING_FIRMWARE_OP;
            out_size = 1;
            if (!fixture_running_firmware(getenv("NU54_TEST_RUNNING_FIRMWARE"), &vault, out)) {
                out_size = NU54_RUNNING_FIRMWARE_REPLY_SIZE;
                report("RUNNING_FIRMWARE", &vault);
                fputs("RUNNING_IMAGE_MEASUREMENT ", stdout);
                for (size_t i = 54; i < out_size; ++i) printf("%02x", out[i]);
                puts("");
                fflush(stdout);
            }
        } else if (operation == NU54_V2_PAYMENT_REVIEW_OP) {
            memset(&reviewed, 0, sizeof(reviewed));
            if (review_payment(&request, &vault, &link, &reviewed, out, now)) break;
            out_size = NU54_V2_PAYMENT_REVIEW_REPLY_SIZE;
        } else if (operation == 3 && request.payload_length == 1) {
            if (owner_status(&vault, &enrollment, out, backup)) break;
            report(backup_open && !out[4] ? "BACKUP_PENDING" : "STATUS", &vault);
        } else if (operation == NU54_OWNER_IDLE_OP && policy_mode && request.payload_length == 1) {
            if (!nu54_owner_idle_live(&idle, vault.record.generation, 0)) break;
            const uint8_t proof[] = {NU54_OWNER_IDLE_OP, 1, NU54_OWNER_IDLE_FLAGS, 0, NU54_OWNER_IDLE_SECONDS};
            memcpy(out, proof, sizeof(proof));
            out_size = sizeof(proof);
            report("POLICY_IDLE_CAPABILITY", &vault);
        } else if (policy_mode && ((operation >= 23 && operation <= 26) ||
                   (operation >= NU54_LIMIT_SET_READ_OP && operation <= NU54_LIMIT_SET_ACTIVATE_OP))) {
            if(operation>=NU54_LIMIT_SET_READ_OP && getenv("NU54_W27_LEGACY_ONLY")) {
                report("POLICY_LEGACY_UNSUPPORTED", &vault);
                break;
            }
            if(policy_operation(&vault,&link,&request,out,&out_size))break;
        } else if (operation >= 0x70 && operation <= 0x72 && request.payload_length == 1) {
            if (recovering || !vault.record.generation || link.peer != NU54_V2_OWNER ||
                (vault.record.flags & NU54_VAULT_REVOKED) || memcmp(vault.record.owner, link.peer_identity, 32) ||
                request.expires <= now || request.expires - now > 10) break;
            if (!(vault.record.flags & NU54_VAULT_PERSONAL) &&
                (owner_status(&vault, &enrollment, out, backup) || out[2] != NU54_HD_FORMAT || out[3] != NU54_HD_PROFILE))
                break;
            out[0] = operation;
            out[1] = 1;
            out_size = 2;
            report(operation == 0x70 ? "TOOL_READ" : operation == 0x71 ? "TOOL_FIND" : "TOOL_CANCEL", &vault);
        } else if (target && operation == 5 && request.payload_length == 1) {
            if (restore_open || begin_offline(&vault, &link, &request, out, true)) {
                out[0] = 5;
                out_size = 1;
                report("RESTORE_REFUSED", &vault);
            } else {
                unlink("restore.receipt");
                restore_open = true;
                out_size = NU54_RECOVERY_OPEN_SIZE;
                report("RESTORE_OPEN", &vault);
            }
        } else if (target && operation == 4 && request.payload_length == 1 && restore_open) {
            out[0] = 4;
            out_size = 1;
            FILE *receipt = fopen("restore.receipt", "rb");
            if (receipt) {
                size_t read = fread(out + 1, 1, NU54_RECOVERY_ENVELOPE_MAX, receipt);
                bool complete = read > NU54_RECOVERY_BODY_SIZE && fgetc(receipt) == EOF && !ferror(receipt);
                fclose(receipt);
                if (!complete) break;
                out_size += read;
                report("RESTORE_RECEIPT", &vault);
            } else {
                report("RESTORE_PENDING", &vault);
            }
        } else if (backup && !target && operation == 6 && request.payload_length == 1) {
            if (!vault.record.generation || memcmp(vault.record.owner, link.peer_identity, 32)) break;
            nu54_hd_slot hd;
            if (hd_host_open(&hd)) break;
            if (!nu54_hd_backup_eligible(&hd, *backup_role)) {
                /* The test adapter can attach the source-capable accessory after refusal. */
                *backup_role = NU54_RECOVERY_SOURCE;
                out[0] = 6;
                out_size = 1;
                report("BACKUP_REFUSED", &vault);
            } else {
                if (backup_open || begin_offline(&vault, &link, &request, out, false)) break;
                backup_open = true;
                out_size = NU54_RECOVERY_OPEN_SIZE;
                report("BACKUP_OPEN", &vault);
            }
        } else if (backup && operation == 7 && request.payload_length == 1) {
            nu54_hd_slot hd;
            if ((!recovering && memcmp(vault.record.owner, link.peer_identity, 32)) ||
                owner_status(&vault, &enrollment, out, true) || !out[4] ||
                hd_host_open(&hd) || nu54_hd_address(&hd, out + NU54_OWNER_STATUS_SIZE)) break;
            out_size = NU54_OWNER_STATUS_SIZE + 20;
            report("PERSONAL_ADDRESS", &vault);
        } else if (operation == 1) {
            if (request.payload_length != 34) break;
            if (recovering) {
                if (request.payload[1] != NU54_OWNER_REPLACE_LOST ||
                    nu54_owner_begin_lost(&enrollment, &vault, &crypto, &link, &request, now)) break;
            } else if (request.payload[1] != NU54_OWNER_REGISTER ||
                       nu54_owner_begin(&enrollment, &vault, &crypto, &link, &request, now, expected_peer)) break;
            out_size = NU54_OWNER_CHALLENGE_SIZE;
            memcpy(out, enrollment.record, out_size);
            report("BEGIN", &vault);
        } else if (operation == 2) {
            const unsigned prompt = 1;
            if (nu54_owner_prove(&enrollment, &vault, &crypto, &link, &request, now, prompt)) break;
            if (!backup && interrupt_before_commit) {
                nu54_owner_cancel(&enrollment);
                report("DROPPED_BEFORE_CONFIRMATION", &vault);
                break;
            }
            if (recovering && independent_replacement(&enrollment, &vault, &link)) {
                if (!nu54_owner_confirm(&enrollment, &vault, &link, now, prompt, true)) break;
                report("REPLACEMENT_REFUSED", &vault);
                break;
            }
            now = (uint64_t)time(NULL);
            if (nu54_owner_confirm(&enrollment, &vault, &link, now, prompt, true)) break;
            if (recovering) {
                report("REPLACEMENT_COMMITTED_REPLY_DROPPED", &vault);
                break;
            }
            if (!backup) {
                /* The simulated PIN/button accepted. Drop the stream after the durable commit. */
                report("DROPPED_COMMITTED_REPLY", &vault);
                break;
            }
            if (owner_status(&vault, &enrollment, out, true)) break;
            report("REGISTERED", &vault);
        } else {
            break;
        }
        request.command = NU54_V2_REPLY;
        request.payload = out;
        request.payload_length = out_size;
        if (nu54_v2_link_send(&link, &request, send_frame, tls)) break;
    }
    if (backup_open) unlink("backup.context");
    if (restore_open) unlink("restore.context");
    nu54_v2_link_close(&link);
}

int main(int argc, char **argv)
{
    umask(0077);
    if (argc < 3 || chdir(argv[1])) return 2;
    nu54_vault board;
    if (nu54_vault_open(&board, &storage)) return 2;
    if (argc == 3 && !strcmp(argv[2], "inspect")) {
        report("VAULT", &board);
        return 0;
    }
    bool replace = argc == 7 && !strcmp(argv[6], "replacement");
    bool target = argc == 7 && !strcmp(argv[6], "restore-target");
    bool blocked = argc == 7 && !strcmp(argv[6], "backup-blocked");
    enum nu54_recovery_role backup_role = blocked ? NU54_RECOVERY_TARGET : NU54_RECOVERY_SOURCE;
    bool backup = replace || target || blocked || (argc == 7 && !strcmp(argv[6], "backup"));
    if ((argc != 6 && !backup) || strcmp(argv[2], "serve")) return 2;

    SSL_CTX *context = SSL_CTX_new(TLS_server_method());
    if (!context || SSL_CTX_load_verify_locations(context, "ca.pem", NULL) != 1 ||
        SSL_CTX_use_certificate_file(context, "board.pem", SSL_FILETYPE_PEM) != 1 ||
        SSL_CTX_use_PrivateKey_file(context, "board.key", SSL_FILETYPE_PEM) != 1 ||
        nu54_v2_openssl_configure(context, NU54_V2_MANAGEMENT, true)) return 2;

    FILE *owner_file = fopen("owner.pem", "r");
    X509 *owner = owner_file ? PEM_read_X509(owner_file, NULL, NULL, NULL) : NULL;
    if (owner_file) fclose(owner_file);
    uint8_t expected[32];
    if (!owner || nu54_v2_openssl_spki(owner, expected)) return 2;
    X509_free(owner);
    if ((getenv("NU54_BOARD_PAYMENT_INSTALL_TEST") || getenv("NU54_W27_BRIDGE")) &&
        (!board.record.generation ? nu54_vault_rebind(&board, 0, expected) :
         board.record.generation != 1 || memcmp(board.record.owner, expected, 32))) return 2;
    if (getenv("NU54_W27_BRIDGE")) {
        if(nu54_vault_initialize_personal(&board,1,expected))return 2;
        char personal[41];
        encode_hex(board.record.personal,sizeof(board.record.personal),personal);
        printf("POLICY_WALLET %s\n",personal);
    }
    uint8_t replacement[32];
    if (replace) {
        FILE *phone_file = fopen("replacement.pem", "r");
        FILE *key_file = fopen("replacement-verifier.pem", "r");
        X509 *phone = phone_file ? PEM_read_X509(phone_file, NULL, NULL, NULL) : NULL;
        replacement_verifier = key_file ? PEM_read_PUBKEY(key_file, NULL, NULL, NULL) : NULL;
        if (phone_file) fclose(phone_file);
        if (key_file) fclose(key_file);
        if (!phone || !replacement_verifier || nu54_v2_openssl_spki(phone, replacement) ||
            !memcmp(expected, replacement, 32)) return 2;
        X509_free(phone);
    }

    int fd = socket(AF_INET, SOCK_STREAM, 0);
    int reuse = 1;
    if (fd < 0 || setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &reuse, sizeof(reuse))) return 2;
    char *end;
    unsigned long port = strtoul(argv[3], &end, 10);
    if (*end || port > UINT16_MAX) return 2;
    int count = atoi(argv[4]);
    int timeout = atoi(argv[5]);
    if (count < 1 || timeout < 1) return 2;
    struct sockaddr_in address = {.sin_family = AF_INET, .sin_addr.s_addr = htonl(INADDR_LOOPBACK), .sin_port = htons((uint16_t)port)};
    socklen_t size = sizeof(address);
    if (bind(fd, (struct sockaddr *)&address, sizeof(address)) || getsockname(fd, (struct sockaddr *)&address, &size) ||
        listen(fd, 4)) return 2;
    printf("PORT %u\n", ntohs(address.sin_port));
    fflush(stdout);
    signal(SIGPIPE, SIG_IGN);
    alarm((unsigned)timeout);

    /* W27 compresses more than a day of UI checks into one transport retention
     * window. Keep every request in the host replay journal during that test. */
    nu54_v2_journal_entry entries[64] = {0};
    size_t capacity = getenv("NU54_W27_BRIDGE") ? 64 : 16;
    nu54_v2_journal journal = {entries, capacity, hash, 0};
    for (int attempt = 0; attempt < count; ++attempt) {
        int client = accept(fd, NULL, NULL);
        if (client < 0) return 2;
        SSL *tls = SSL_new(context);
        if (tls && SSL_set_fd(tls, client) == 1) serve(tls, expected, &journal, attempt == 0, backup, target,
                                                           &backup_role, replace ? replacement : NULL);
        SSL_free(tls);
        close(client);
    }
    close(fd);
    SSL_CTX_free(context);
    EVP_PKEY_free(replacement_verifier);
    return 0;
}
