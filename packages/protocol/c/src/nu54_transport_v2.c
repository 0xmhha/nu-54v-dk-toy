#include "nu54_transport_v2.h"

#include <string.h>

int nu54_v2_payment_review_decode(const uint8_t *body, size_t length, const uint8_t **request_id,
				  const uint8_t **quote)
{
    const uint8_t zero[NU54_V2_ID_SIZE] = {0};
    if (length != NU54_V2_PAYMENT_REVIEW_REQUEST_SIZE || body[0] != NU54_V2_PAYMENT_REVIEW_OP ||
        !memcmp(body + 1, zero, sizeof(zero))) return -1;
    *request_id = body + 1;
    *quote = body + 1 + NU54_V2_ID_SIZE;
    return 0;
}

int nu54_v2_payment_review_reply(const uint8_t *body, size_t length, bool ready,
				 int (*sha256)(const uint8_t *, size_t, uint8_t [32]),
				 uint8_t reply[NU54_V2_PAYMENT_REVIEW_REPLY_SIZE])
{
    const uint8_t *request_id, *quote;
    if (nu54_v2_payment_review_decode(body, length, &request_id, &quote) ||
        sha256(body, length, reply + 1)) return -1;
    reply[0] = NU54_V2_PAYMENT_REVIEW_OP;
    reply[33] = ready ? 1 : 0;
    return 0;
}

bool nu54_v2_rental_review_prepare(const uint8_t *body, size_t length)
{
    static const uint8_t zero[32];
    return body && length == NU54_V2_RENTAL_REVIEW_PREPARE_SIZE &&
        !memcmp(body, "W10Q", 4) && memcmp(body + 4, zero, sizeof(zero));
}

#define FRAME_HEADER 22
#define BODY_HEADER 25
#define MAX_TTL UINT64_C(300)
#define FRAGMENT_TIMEOUT UINT64_C(5)

struct profile {
    const char *alpn;
    nu54_v2_role initiator, responder;
};

static const struct profile profiles[] = {
    {0},
    {"nu54-v2-management", NU54_V2_OWNER, NU54_V2_BOARD},
    {"nu54-v2-payment", NU54_V2_TERMINAL, NU54_V2_BOARD},
    {"nu54-v2-nearby", NU54_V2_CUSTOMER, NU54_V2_TERMINAL},
};

const char *nu54_v2_alpn(nu54_v2_profile profile)
{
    return profile >= NU54_V2_MANAGEMENT && profile <= NU54_V2_NEARBY ? profiles[profile].alpn : NULL;
}

nu54_v2_role nu54_v2_peer_role(nu54_v2_profile profile, nu54_v2_role local)
{
    if (!nu54_v2_alpn(profile)) return 0;
    if (local == profiles[profile].initiator) return profiles[profile].responder;
    if (local == profiles[profile].responder) return profiles[profile].initiator;
    return 0;
}

static bool allowed(nu54_v2_profile profile, nu54_v2_role role, nu54_v2_command command)
{
    if (role == profiles[profile].responder) return command == NU54_V2_REPLY ||
        (profile == NU54_V2_MANAGEMENT && command == NU54_V2_APPROVAL_PROGRESS) ||
        (profile == NU54_V2_PAYMENT && command == NU54_V2_RENTAL_REVIEW);
    if (role != profiles[profile].initiator) return false;
    if (command == NU54_V2_STATUS) return true;
    if (profile == NU54_V2_MANAGEMENT)
        return command == NU54_V2_MANAGE || command == NU54_V2_PAY_PREPARE || command == NU54_V2_CLOCK;
    if (profile == NU54_V2_PAYMENT) return command == NU54_V2_PAY_PREPARE;
    return command == NU54_V2_NEARBY_SUBMIT;
}

static uint64_t read_be(const uint8_t *bytes, size_t size)
{
    uint64_t result = 0;
    for (size_t i = 0; i < size; ++i) result = (result << 8) | bytes[i];
    return result;
}

static void write_be(uint8_t *bytes, size_t size, uint64_t value)
{
    while (size > 0) {
        bytes[--size] = (uint8_t)value;
        value >>= 8;
    }
}

int nu54_v2_approval_progress_body(uint8_t tag, uint64_t generation,
                                     uint8_t out[NU54_V2_APPROVAL_PROGRESS_SIZE])
{
    if (!out || !generation || (tag != NU54_V2_APPROVAL_PROGRESS_OP && tag != 1)) return NU54_V2_ERROR;
    out[0] = tag;
    write_be(out + 1, 8, generation);
    return 0;
}

bool nu54_v2_approval_progress_offer(const uint8_t *body, size_t size, uint64_t current_generation)
{
    return body && current_generation && size == NU54_V2_APPROVAL_PROGRESS_SIZE &&
        body[0] == NU54_V2_APPROVAL_PROGRESS_OP && read_be(body + 1, 8) == current_generation;
}

static bool nonzero_id(const uint8_t *id)
{
    const uint8_t zero[NU54_V2_ID_SIZE] = {0};
    return memcmp(id, zero, sizeof(zero)) != 0;
}

int nu54_v2_link_init(nu54_v2_link *link, nu54_v2_profile profile, nu54_v2_role local,
                      const uint8_t epoch[NU54_V2_EPOCH_SIZE], const uint8_t peer[NU54_V2_PEER_SIZE],
                      nu54_v2_journal *journal)
{
    nu54_v2_role peer_role = nu54_v2_peer_role(profile, local);
    if (!link || !epoch || !peer || !peer_role || !journal || !journal->entries ||
        !journal->capacity || !journal->sha256) return NU54_V2_ERROR;
    memset(link, 0, sizeof(*link));
    link->profile = profile;
    link->local = local;
    link->peer = peer_role;
    link->journal = journal;
    memcpy(link->epoch, epoch, sizeof(link->epoch));
    memcpy(link->peer_identity, peer, sizeof(link->peer_identity));
    return 0;
}

void nu54_v2_link_close(nu54_v2_link *link)
{
    memset(link->body, 0, sizeof(link->body));
    memset(link->epoch, 0, sizeof(link->epoch));
    link->pending_expiry = 0;
    link->pending_rental_review = false;
    memset(link->pending_review_challenge, 0, sizeof(link->pending_review_challenge));
    memset(link->pending_review_quote_hash, 0, sizeof(link->pending_review_quote_hash));
    link->approval_generation = 0;
    link->closed = true;
}

static nu54_v2_result fail(nu54_v2_link *link)
{
    nu54_v2_link_close(link);
    return NU54_V2_ERROR;
}

int nu54_v2_link_enable_approval_progress(nu54_v2_link *link, uint64_t generation)
{
    if (!link || link->closed || link->profile != NU54_V2_MANAGEMENT ||
        link->pending_expiry || link->approval_generation || !generation) return NU54_V2_ERROR;
    link->approval_generation = generation;
    return 0;
}

static bool matches_outstanding(const nu54_v2_link *link, const nu54_v2_request *request)
{
    if (request->command != NU54_V2_REPLY && request->command != NU54_V2_APPROVAL_PROGRESS &&
        request->command != NU54_V2_RENTAL_REVIEW)
        return !link->pending_expiry;
    if (!link->pending_expiry || request->expires != link->pending_expiry ||
        !nonzero_id(request->id) || memcmp(link->pending_id, request->id, sizeof(link->pending_id)))
        return false;
    if (request->command == NU54_V2_REPLY) return true;
    if (request->command == NU54_V2_RENTAL_REVIEW) {
        const uint8_t *body = request->payload;
        const size_t peer_offset = link->local == NU54_V2_BOARD ? 36 : 4;
        return link->pending_rental_review && link->pending_fresh && !link->pending_reported &&
            request->payload_length == NU54_V2_RENTAL_REVIEW_RECEIPT_SIZE && !memcmp(body, "W10R", 4) &&
            !memcmp(body + peer_offset, link->peer_identity, NU54_V2_PEER_SIZE) &&
            !memcmp(body + 68, link->epoch, NU54_V2_EPOCH_SIZE) &&
            !memcmp(body + 84, request->id, NU54_V2_ID_SIZE) &&
            !memcmp(body + 100, link->pending_review_challenge, 32) &&
            !memcmp(body + 164, link->pending_review_quote_hash, 32) &&
            read_be(body + 224, 8) == request->expires;
    }
    return link->approval_generation && link->pending_command == NU54_V2_PAY_PREPARE &&
        link->pending_fresh && !link->pending_reported && request->payload_length == NU54_V2_APPROVAL_PROGRESS_SIZE &&
        request->payload[0] == 1 && read_be(request->payload + 1, 8) == link->approval_generation;
}

static void update_outstanding(nu54_v2_link *link, const nu54_v2_request *request, bool fresh)
{
    if (request->command == NU54_V2_APPROVAL_PROGRESS || request->command == NU54_V2_RENTAL_REVIEW) {
        link->pending_reported = true;
        return;
    }
    link->pending_expiry = request->command == NU54_V2_REPLY ? 0 : request->expires;
    link->pending_command = request->command;
    link->pending_fresh = fresh;
    link->pending_reported = false;
    link->pending_rental_review = link->profile == NU54_V2_PAYMENT && request->command == NU54_V2_PAY_PREPARE &&
        nu54_v2_rental_review_prepare(request->payload, request->payload_length) &&
        !link->journal->sha256(request->payload + 36, NU54_V2_SIGNED_SIZE, link->pending_review_quote_hash);
    if (link->pending_rental_review) {
        memcpy(link->pending_review_challenge, request->payload + 4, 32);
    } else {
        memset(link->pending_review_challenge, 0, 32);
        memset(link->pending_review_quote_hash, 0, 32);
    }
    memcpy(link->pending_id, request->id, sizeof(link->pending_id));
}

int nu54_v2_link_send(nu54_v2_link *link, const nu54_v2_request *request,
                      int (*write)(void *, const uint8_t *, size_t), void *context)
{
    if (link->closed || !request || !write || !allowed(link->profile, link->local, request->command) ||
        !nonzero_id(request->id) || !request->expires || request->payload_length > NU54_V2_BODY_MAX - BODY_HEADER ||
        (request->payload_length && !request->payload) || !matches_outstanding(link, request)) return NU54_V2_ERROR;
    if (link->sent == UINT32_MAX) return fail(link);

    uint8_t header[BODY_HEADER];
    header[0] = (uint8_t)request->command;
    memcpy(header + 1, request->id, NU54_V2_ID_SIZE);
    write_be(header + 17, 8, request->expires);
    size_t size = BODY_HEADER + request->payload_length;
    uint8_t count = (uint8_t)((size + NU54_V2_CHUNK_SIZE - 1) / NU54_V2_CHUNK_SIZE);
    uint8_t frame[NU54_V2_FRAME_MAX];
    memcpy(frame, link->epoch, NU54_V2_EPOCH_SIZE);
    write_be(frame + NU54_V2_EPOCH_SIZE, 4, link->sent);
    frame[21] = count;
    for (uint8_t i = 0; i < count; ++i) {
        size_t offset = (size_t)i * NU54_V2_CHUNK_SIZE;
        size_t remaining = size - offset;
        size_t length = remaining < NU54_V2_CHUNK_SIZE ? remaining : NU54_V2_CHUNK_SIZE;
        frame[20] = i;
        size_t prefix = i == 0 ? sizeof(header) : 0;
        if (prefix) memcpy(frame + FRAME_HEADER, header, prefix);
        if (length > prefix) {
            memcpy(frame + FRAME_HEADER + prefix, request->payload + (offset + prefix - BODY_HEADER), length - prefix);
        }
        if (write(context, frame, FRAME_HEADER + length) != 0) return fail(link);
    }
    ++link->sent;
    update_outstanding(link, request, true);
    return 0;
}

static nu54_v2_result journal_record(nu54_v2_link *link, const nu54_v2_request *request, uint64_t now)
{
    nu54_v2_journal_entry *available = NULL;
    uint8_t digest[32];
    if (link->journal->sha256(link->body, link->size, digest) != 0) return NU54_V2_ERROR;
    for (size_t i = 0; i < link->journal->capacity; ++i) {
        nu54_v2_journal_entry *entry = &link->journal->entries[i];
        if (entry->retain_until <= now) {
            if (!available) available = entry;
        } else if (entry->profile == link->profile &&
                   memcmp(entry->id, request->id, sizeof(entry->id)) == 0 &&
                   memcmp(entry->peer, link->peer_identity, sizeof(entry->peer)) == 0) {
            return memcmp(digest, entry->digest, sizeof(digest)) == 0 ? NU54_V2_RESUME : NU54_V2_ERROR;
        }
    }
    if (!available) return NU54_V2_ERROR;
    available->retain_until = request->expires > UINT64_MAX - MAX_TTL ? UINT64_MAX : request->expires + MAX_TTL;
    /* The time command may end any time ahead. Its record is kept no longer than the record of any
     * other request can be, so that it cannot hold a place in the journal for ever. */
    if (request->command == NU54_V2_CLOCK && available->retain_until - now > 2 * MAX_TTL)
        available->retain_until = now + 2 * MAX_TTL;
    available->profile = link->profile;
    memcpy(available->id, request->id, sizeof(available->id));
    memcpy(available->peer, link->peer_identity, sizeof(available->peer));
    memcpy(available->digest, digest, sizeof(digest));
    return NU54_V2_NEW;
}

nu54_v2_result nu54_v2_link_receive(nu54_v2_link *link, const uint8_t *frame, size_t length,
                                     uint64_t now, nu54_v2_request *out)
{
    if (link->closed) return NU54_V2_ERROR;
    if (!frame || !out || length <= FRAME_HEADER || length > NU54_V2_FRAME_MAX ||
        memcmp(frame, link->epoch, NU54_V2_EPOCH_SIZE) != 0 || link->received == UINT32_MAX ||
        read_be(frame + NU54_V2_EPOCH_SIZE, 4) != link->received) return fail(link);
    uint8_t part = frame[20], count = frame[21];
    size_t chunk = length - FRAME_HEADER;
    if (!count || count > NU54_V2_FRAGMENT_MAX || part >= count || part != link->part ||
        (part && count != link->count) || (part + 1 < count && chunk != NU54_V2_CHUNK_SIZE) ||
        link->size + chunk > NU54_V2_BODY_MAX) return fail(link);
    if (now < link->journal->trusted_time) return fail(link);
    link->journal->trusted_time = now;
    if (link->part && now - link->started >= FRAGMENT_TIMEOUT) return fail(link);

    link->count = count;
    if (!link->part) link->started = now;
    link->part++;
    memcpy(link->body + link->size, frame + FRAME_HEADER, chunk);
    link->size += chunk;
    if (link->part < count) return NU54_V2_MORE;

    out->command = (nu54_v2_command)link->body[0];
    if (link->size < BODY_HEADER || !nonzero_id(link->body + 1) ||
        !allowed(link->profile, link->peer, out->command)) return fail(link);
    memcpy(out->id, link->body + 1, sizeof(out->id));
    out->expires = read_be(link->body + 17, 8);
    /* The time command comes from a peer whose clock is ahead of a board that was without power. */
    if (out->expires <= now || (out->command != NU54_V2_CLOCK && out->expires - now > MAX_TTL)) return fail(link);
    out->payload = link->body + BODY_HEADER;
    out->payload_length = link->size - BODY_HEADER;
    if (!matches_outstanding(link, out)) return fail(link);
    nu54_v2_result result = out->command == NU54_V2_REPLY || out->command == NU54_V2_APPROVAL_PROGRESS ||
        out->command == NU54_V2_RENTAL_REVIEW
        ? NU54_V2_NEW : journal_record(link, out, now);
    if (result == NU54_V2_ERROR) return fail(link);
    link->part = link->count = 0;
    link->size = 0;
    ++link->received;
    update_outstanding(link, out, result == NU54_V2_NEW);
    return result;
}
