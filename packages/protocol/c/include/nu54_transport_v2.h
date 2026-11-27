#ifndef NU54_TRANSPORT_V2_H
#define NU54_TRANSPORT_V2_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "nu54_quote_v2.h"

#define NU54_V2_EXPORTER_LABEL "EXPORTER-NU54-wire2"
#define NU54_V2_EPOCH_SIZE 16
#define NU54_V2_PEER_SIZE 32
#define NU54_V2_ID_SIZE 16
#define NU54_V2_BODY_MAX 2048
#define NU54_V2_CHUNK_SIZE 192
#define NU54_V2_FRAME_MAX (22 + NU54_V2_CHUNK_SIZE)
#define NU54_V2_FRAGMENT_MAX ((NU54_V2_BODY_MAX + NU54_V2_CHUNK_SIZE - 1) / NU54_V2_CHUNK_SIZE)

#define NU54_V2_RENTAL_REVIEW_PREPARE_SIZE (36 + NU54_V2_SIGNED_SIZE)
#define NU54_V2_RENTAL_REVIEW_RECEIPT_SIZE 296
bool nu54_v2_rental_review_prepare(const uint8_t *body, size_t length);

#define NU54_V2_APPROVAL_PROGRESS_OP 0x76
#define NU54_V2_APPROVAL_PROGRESS_SIZE 9
/* Encode the capability acknowledgement (0x76) or a live progress body (1). */
int nu54_v2_approval_progress_body(uint8_t tag, uint64_t generation,
                                     uint8_t out[NU54_V2_APPROVAL_PROGRESS_SIZE]);
bool nu54_v2_approval_progress_offer(const uint8_t *body, size_t size, uint64_t current_generation);

#define NU54_V2_PAYMENT_REVIEW_OP 0x75
#define NU54_V2_PAYMENT_REVIEW_REQUEST_SIZE (1 + NU54_V2_ID_SIZE + NU54_V2_SIGNED_SIZE)
#define NU54_V2_PAYMENT_REVIEW_REPLY_SIZE 34

/* The returned views borrow the request payload. This codec does not authorize a quote. */
int nu54_v2_payment_review_decode(const uint8_t *body, size_t length, const uint8_t **request_id,
				  const uint8_t **quote);
int nu54_v2_payment_review_reply(const uint8_t *body, size_t length, bool ready,
				 int (*sha256)(const uint8_t *, size_t, uint8_t [32]),
				 uint8_t reply[NU54_V2_PAYMENT_REVIEW_REPLY_SIZE]);

typedef enum {
    NU54_V2_MANAGEMENT = 1, NU54_V2_PAYMENT, NU54_V2_NEARBY
} nu54_v2_profile;

typedef enum {
    NU54_V2_OWNER = 1, NU54_V2_BOARD, NU54_V2_TERMINAL, NU54_V2_CUSTOMER
} nu54_v2_role;

/* NU54_V2_CLOCK (8): the owner tells the board the time. A board that was without power is behind
 * the calendar, so this one command is not held to the receiver's "ends within 300 s" rule; every
 * other rule applies to it, and it is still refused once it has ended by the receiver's time. The
 * receiver's handler must bind the time it carries to the request's own end. */
typedef enum {
    NU54_V2_MANAGE = 1, NU54_V2_PAY_PREPARE, NU54_V2_STATUS, NU54_V2_NEARBY_SUBMIT, NU54_V2_REPLY,
    NU54_V2_APPROVAL_PROGRESS, NU54_V2_RENTAL_REVIEW, NU54_V2_CLOCK
} nu54_v2_command;

typedef enum {
    NU54_V2_ERROR = -1, NU54_V2_MORE, NU54_V2_NEW, NU54_V2_RESUME
} nu54_v2_result;

typedef struct {
    nu54_v2_command command;
    uint8_t id[NU54_V2_ID_SIZE];
    uint64_t expires;
    const uint8_t *payload;
    size_t payload_length;
} nu54_v2_request;

typedef struct {
    uint64_t retain_until;
    nu54_v2_profile profile;
    uint8_t peer[NU54_V2_PEER_SIZE];
    uint8_t id[NU54_V2_ID_SIZE];
    uint8_t digest[32];
} nu54_v2_journal_entry;

typedef struct {
    nu54_v2_journal_entry *entries; /* Caller zero-initializes and retains across reconnects. */
    size_t capacity;
    int (*sha256)(const uint8_t *message, size_t length, uint8_t digest[32]); /* 0 on success. */
    uint64_t trusted_time; /* Initialize only when creating a new journal, never on reconnect. */
} nu54_v2_journal;

typedef struct {
    nu54_v2_profile profile;
    nu54_v2_role local, peer;
    nu54_v2_journal *journal;
    uint8_t epoch[NU54_V2_EPOCH_SIZE], peer_identity[NU54_V2_PEER_SIZE];
    uint32_t sent, received;
    uint8_t part, count;
    size_t size;
    uint64_t started;
    bool closed;
    uint8_t pending_id[NU54_V2_ID_SIZE];
    uint64_t pending_expiry, approval_generation;
    nu54_v2_command pending_command;
    bool pending_fresh, pending_reported, pending_rental_review;
    uint8_t pending_review_challenge[32], pending_review_quote_hash[32];
    uint8_t body[NU54_V2_BODY_MAX];
} nu54_v2_link;

const char *nu54_v2_alpn(nu54_v2_profile profile);
nu54_v2_role nu54_v2_peer_role(nu54_v2_profile profile, nu54_v2_role local);

/* Call only after mTLS peer-role verification. Epoch is the TLS exporter, peer is the
 * authenticated peer's SPKI SHA-256. Neither value comes from an application message. */
int nu54_v2_link_init(nu54_v2_link *link, nu54_v2_profile profile, nu54_v2_role local,
                      const uint8_t epoch[NU54_V2_EPOCH_SIZE], const uint8_t peer[NU54_V2_PEER_SIZE],
                      nu54_v2_journal *journal);
/* Call after authenticated, idle opt-in confirming the current owner generation. */
int nu54_v2_link_enable_approval_progress(nu54_v2_link *link, uint64_t generation);
int nu54_v2_link_send(nu54_v2_link *link, const nu54_v2_request *request,
                      int (*write)(void *, const uint8_t *, size_t), void *context);
/* On NEW/RESUME, out->payload is borrowed until the next receive or close. NEW is transport
 * acceptance only. Business authorization and durable resume remain separate. */
nu54_v2_result nu54_v2_link_receive(nu54_v2_link *link, const uint8_t *frame, size_t length,
                                     uint64_t now, nu54_v2_request *out);
void nu54_v2_link_close(nu54_v2_link *link);

#endif
