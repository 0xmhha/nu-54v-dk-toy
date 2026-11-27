#include "nu54_transport_v2.h"

#include <assert.h>
#include <string.h>
#include <stdio.h>
#include <openssl/sha.h>

struct frames {
    uint8_t bytes[NU54_V2_FRAGMENT_MAX][NU54_V2_FRAME_MAX];
    size_t length[NU54_V2_FRAGMENT_MAX];
    size_t count;
};

static int collect(void *ctx, const uint8_t *frame, size_t length)
{
    struct frames *frames = ctx;
    assert(frames->count < NU54_V2_FRAGMENT_MAX);
    memcpy(frames->bytes[frames->count], frame, length);
    frames->length[frames->count++] = length;
    return 0;
}

static int digest_fixture(const uint8_t *bytes, size_t size, uint8_t result[32])
{
    /* Unit fixture only. The TLS interoperability test uses real SHA-256. */
    memset(result, 0, 32);
    for (size_t i = 0; i < size; ++i)
        result[i % 32] ^= bytes[i];
    return 0;
}

static int real_digest(const uint8_t *bytes, size_t size, uint8_t result[32])
{
    return SHA256(bytes, size, result) ? 0 : -1;
}

static void review_vectors(const char *path)
{
    FILE *file = fopen(path, "r");
    assert(file);
    uint8_t body[NU54_V2_PAYMENT_REVIEW_REQUEST_SIZE], reply[NU54_V2_PAYMENT_REVIEW_REPLY_SIZE];
    memset(body, 9, sizeof(body));
    body[0] = NU54_V2_PAYMENT_REVIEW_OP;
    memset(body + 1, 7, NU54_V2_ID_SIZE);
    const char *cases[] = {"exact", "other-request", "other-signature"};
    for (size_t i = 0; i < 3; i++) {
        char name[32], digest[65], encoded[65];
        assert(fscanf(file, " %31[^|]|%64[0-9a-f]", name, digest) == 2);
        assert(!strcmp(name, cases[i]));
        body[1] = i == 1 ? 4 : 7;
        body[sizeof(body) - 1] = i == 2 ? 8 : 9;
        const uint8_t *request, *quote;
        assert(!nu54_v2_payment_review_decode(body, sizeof(body), &request, &quote));
        assert(request == body + 1 && quote == request + NU54_V2_ID_SIZE);
        assert(nu54_v2_payment_review_decode(body, sizeof(body) - 1, &request, &quote));
        for (unsigned ready = 0; ready < 2; ready++) {
            assert(!nu54_v2_payment_review_reply(body, sizeof(body), ready, real_digest, reply));
            assert(reply[0] == 0x75 && reply[33] == ready);
            for (size_t j = 0; j < 32; j++) snprintf(encoded + j * 2, 3, "%02x", reply[j + 1]);
            assert(!strcmp(digest, encoded));
        }
    }
    memset(body + 1, 0, NU54_V2_ID_SIZE);
    assert(nu54_v2_payment_review_reply(body, sizeof(body), true, real_digest, reply));
    assert(!fclose(file));
}

static void deliver(nu54_v2_link *link, const struct frames *frames, uint64_t now,
                    nu54_v2_result final_result)
{
    nu54_v2_request out;
    assert(frames->count);
    for (size_t i = 0; i < frames->count; ++i)
        assert(nu54_v2_link_receive(link, frames->bytes[i], frames->length[i], now, &out) ==
               (i + 1 == frames->count ? final_result : NU54_V2_MORE));
}

static void progress_keeps_original_pending(void)
{
    uint8_t epoch[16] = {1}, next[16] = {2}, peer[32] = {3}, busy[] = {1,0,0,0,0,0,0,0,5};
    nu54_v2_journal_entry records[4] = {0};
    nu54_v2_journal journal = {records, 4, digest_fixture, 0};
    nu54_v2_link owner, board;
    nu54_v2_request request = {NU54_V2_PAY_PREPARE, {7}, 60, busy, sizeof(busy)};
    nu54_v2_request progress = {NU54_V2_APPROVAL_PROGRESS, {7}, 60, busy, sizeof(busy)};
    nu54_v2_request reply = {NU54_V2_REPLY, {7}, 60, NULL, 0};
    struct frames sent = {0}, shown = {0};
    assert(!nu54_v2_link_init(&owner, NU54_V2_MANAGEMENT, NU54_V2_OWNER, epoch, peer, &journal));
    assert(!nu54_v2_link_init(&board, NU54_V2_MANAGEMENT, NU54_V2_BOARD, epoch, peer, &journal));
    assert(!nu54_v2_link_enable_approval_progress(&owner, 5));
    assert(!nu54_v2_link_enable_approval_progress(&board, 5));
    assert(nu54_v2_link_send(&owner, &progress, collect, &sent) < 0);
    assert(!nu54_v2_link_send(&owner, &request, collect, &sent));
    deliver(&board, &sent, 10, NU54_V2_NEW);
    progress.expires = 61;
    assert(nu54_v2_link_send(&board, &progress, collect, &shown) < 0);
    progress.expires = 60;
    busy[8] = 6;
    assert(nu54_v2_link_send(&board, &progress, collect, &shown) < 0);
    busy[8] = 5;
    assert(!nu54_v2_link_send(&board, &progress, collect, &shown));
    assert(shown.count == 1 && shown.bytes[0][22] == 6);
    deliver(&owner, &shown, 11, NU54_V2_NEW);
    assert(owner.pending_expiry == 60 && board.pending_expiry == 60);
    assert(!memcmp(owner.pending_id, request.id, 16) && !memcmp(board.pending_id, request.id, 16));
    assert(nu54_v2_link_send(&board, &progress, collect, &shown) < 0);
    assert(nu54_v2_link_send(&owner, &request, collect, &sent) < 0);
    struct frames complete = {0};
    assert(!nu54_v2_link_send(&board, &reply, collect, &complete));
    deliver(&owner, &complete, 12, NU54_V2_NEW);
    assert(!owner.pending_expiry && !board.pending_expiry);
    assert(nu54_v2_link_send(&board, &progress, collect, &shown) < 0);

    assert(!nu54_v2_link_init(&owner, NU54_V2_MANAGEMENT, NU54_V2_OWNER, next, peer, &journal));
    assert(!nu54_v2_link_init(&board, NU54_V2_MANAGEMENT, NU54_V2_BOARD, next, peer, &journal));
    assert(!nu54_v2_link_enable_approval_progress(&owner, 5));
    assert(!nu54_v2_link_enable_approval_progress(&board, 5));
    sent.count = 0;
    assert(!nu54_v2_link_send(&owner, &request, collect, &sent));
    deliver(&board, &sent, 13, NU54_V2_RESUME);
    assert(nu54_v2_link_send(&board, &progress, collect, &shown) < 0);
    deliver(&owner, &shown, 13, NU54_V2_ERROR);
    complete.count = 0;
    assert(!nu54_v2_link_send(&board, &reply, collect, &complete));

    assert(!nu54_v2_link_init(&owner, NU54_V2_MANAGEMENT, NU54_V2_OWNER, epoch, peer, &journal));
    sent.count = 0;
    assert(!nu54_v2_link_send(&owner, &request, collect, &sent));
    deliver(&owner, &shown, 14, NU54_V2_ERROR);
    assert(!nu54_v2_link_init(&board, NU54_V2_PAYMENT, NU54_V2_BOARD, epoch, peer, &journal));
    assert(nu54_v2_link_enable_approval_progress(&board, 5) < 0);
}

static void fresh(nu54_v2_journal_entry records[2][4], nu54_v2_journal *first, nu54_v2_journal *second)
{
    memset(records, 0, 2 * 4 * sizeof(records[0][0]));
    first->trusted_time = second->trusted_time = 0;
}

/* The owner's time command reaches a board whose clock stood still without power: it is the one
 * command that may end more than 300 s after the receiver's time. Nothing else about it differs. */
static void clock_reaches_a_late_board(void)
{
    const uint64_t real = 1800000000, day = 86400;
    uint8_t epoch[16] = {1}, next[16] = {2}, peer[32] = {3}, time[9] = {0x81, 0, 0, 0, 0, 0x6b, 0x49, 0xd2, 0x00};
    nu54_v2_journal_entry records[2][4] = {0};
    nu54_v2_journal board_journal = {records[0], 4, digest_fixture, 0}, other_journal = {records[1], 4, digest_fixture, 0};
    nu54_v2_link owner, board;
    nu54_v2_request clock = {NU54_V2_CLOCK, {9}, real + 90, time, sizeof(time)};
    nu54_v2_request manage = {NU54_V2_MANAGE, {9}, real + 90, time, sizeof(time)}, out;
    struct frames sent = {0};
    static const uint64_t late[] = {0, 5, 600, 3 * 3600, 3 * 86400, 31 * 86400};
    for (size_t i = 0; i < sizeof(late) / sizeof(late[0]); ++i) {
        fresh(records, &board_journal, &other_journal);
        assert(!nu54_v2_link_init(&owner, NU54_V2_MANAGEMENT, NU54_V2_OWNER, epoch, peer, &other_journal));
        assert(!nu54_v2_link_init(&board, NU54_V2_MANAGEMENT, NU54_V2_BOARD, epoch, peer, &board_journal));
        sent.count = 0;
        assert(!nu54_v2_link_send(&owner, &clock, collect, &sent) && sent.count == 1);
        assert(sent.bytes[0][22] == 8); /* the command code on the wire */
        deliver(&board, &sent, real - late[i], NU54_V2_NEW);
        assert(board.pending_expiry == real + 90 && board.pending_command == NU54_V2_CLOCK);
        /* The same bytes as an ordinary management request are refused once the board is 211 s late. */
        fresh(records, &board_journal, &other_journal);
        assert(!nu54_v2_link_init(&owner, NU54_V2_MANAGEMENT, NU54_V2_OWNER, epoch, peer, &other_journal));
        assert(!nu54_v2_link_init(&board, NU54_V2_MANAGEMENT, NU54_V2_BOARD, epoch, peer, &board_journal));
        sent.count = 0;
        assert(!nu54_v2_link_send(&owner, &manage, collect, &sent));
        deliver(&board, &sent, real - late[i], late[i] <= 210 ? NU54_V2_NEW : NU54_V2_ERROR);
    }

    /* However far ahead it ends, its record leaves the journal as early as any other's: more of
     * them than the journal has places, ten minutes apart, are all taken. */
    fresh(records, &board_journal, &other_journal);
    nu54_v2_request far = {NU54_V2_CLOCK, {9}, UINT64_MAX, time, sizeof(time)};
    for (uint64_t i = 0; i < 6; ++i) {
        const uint64_t at = real - 3 * day + i * 601;
        far.id[1] = (uint8_t)(i + 1);
        assert(!nu54_v2_link_init(&owner, NU54_V2_MANAGEMENT, NU54_V2_OWNER, epoch, peer, &other_journal));
        assert(!nu54_v2_link_init(&board, NU54_V2_MANAGEMENT, NU54_V2_BOARD, epoch, peer, &board_journal));
        sent.count = 0;
        assert(!nu54_v2_link_send(&owner, &far, collect, &sent));
        deliver(&board, &sent, at, NU54_V2_NEW);
        for (size_t slot = 0; slot < 4; ++slot) assert(records[0][slot].retain_until <= at + 600);
    }
    /* Within that time the same request is still known: the same bytes again are a retry. */
    assert(!nu54_v2_link_init(&owner, NU54_V2_MANAGEMENT, NU54_V2_OWNER, epoch, peer, &other_journal));
    assert(!nu54_v2_link_init(&board, NU54_V2_MANAGEMENT, NU54_V2_BOARD, epoch, peer, &board_journal));
    sent.count = 0;
    assert(!nu54_v2_link_send(&owner, &far, collect, &sent));
    deliver(&board, &sent, real - 3 * day + 5 * 601 + 599, NU54_V2_RESUME);

    /* It has ended by the board's time: refused like any other request. */
    fresh(records, &board_journal, &other_journal);
    assert(!nu54_v2_link_init(&owner, NU54_V2_MANAGEMENT, NU54_V2_OWNER, epoch, peer, &other_journal));
    assert(!nu54_v2_link_init(&board, NU54_V2_MANAGEMENT, NU54_V2_BOARD, epoch, peer, &board_journal));
    sent.count = 0;
    assert(!nu54_v2_link_send(&owner, &clock, collect, &sent));
    deliver(&board, &sent, real + 90, NU54_V2_ERROR);

    /* Its frame is bound to its session and its place in it like any other: the same frame is
     * refused a second time in its own session and in a later one. */
    fresh(records, &board_journal, &other_journal);
    assert(!nu54_v2_link_init(&owner, NU54_V2_MANAGEMENT, NU54_V2_OWNER, epoch, peer, &other_journal));
    assert(!nu54_v2_link_init(&board, NU54_V2_MANAGEMENT, NU54_V2_BOARD, epoch, peer, &board_journal));
    sent.count = 0;
    assert(!nu54_v2_link_send(&owner, &clock, collect, &sent));
    deliver(&board, &sent, real - 3 * day, NU54_V2_NEW);
    assert(nu54_v2_link_receive(&board, sent.bytes[0], sent.length[0], real - 3 * day, &out) == NU54_V2_ERROR);
    assert(!nu54_v2_link_init(&board, NU54_V2_MANAGEMENT, NU54_V2_BOARD, next, peer, &board_journal));
    assert(nu54_v2_link_receive(&board, sent.bytes[0], sent.length[0], real - 3 * day, &out) == NU54_V2_ERROR);

    /* Only the owner sends it, only on the management route, and the board never does. */
    assert(!nu54_v2_link_init(&board, NU54_V2_MANAGEMENT, NU54_V2_BOARD, epoch, peer, &board_journal));
    assert(nu54_v2_link_send(&board, &clock, collect, &sent) < 0);
    assert(!nu54_v2_link_init(&owner, NU54_V2_PAYMENT, NU54_V2_TERMINAL, epoch, peer, &other_journal));
    assert(nu54_v2_link_send(&owner, &clock, collect, &sent) < 0);
    assert(!nu54_v2_link_init(&owner, NU54_V2_NEARBY, NU54_V2_CUSTOMER, epoch, peer, &other_journal));
    assert(nu54_v2_link_send(&owner, &clock, collect, &sent) < 0);
    /* A terminal that writes the command code into a frame of its own is refused by the board. */
    fresh(records, &board_journal, &other_journal);
    nu54_v2_request prepare = {NU54_V2_PAY_PREPARE, {9}, real + 90, time, sizeof(time)};
    assert(!nu54_v2_link_init(&owner, NU54_V2_PAYMENT, NU54_V2_TERMINAL, epoch, peer, &other_journal));
    assert(!nu54_v2_link_init(&board, NU54_V2_PAYMENT, NU54_V2_BOARD, epoch, peer, &board_journal));
    sent.count = 0;
    assert(!nu54_v2_link_send(&owner, &prepare, collect, &sent));
    sent.bytes[0][22] = NU54_V2_CLOCK;
    deliver(&board, &sent, real, NU54_V2_ERROR);
}

int main(int argc, char **argv)
{
    assert(argc == 2);
    review_vectors(argv[1]);
    progress_keeps_original_pending();
    clock_reaches_a_late_board();
    uint8_t epoch[16] = {1}, new_epoch[16] = {2}, peer[32] = {3}, body[737] = {4};
    nu54_v2_journal_entry entries[4] = {0};
    nu54_v2_journal journal = {entries, 4, digest_fixture, 0};
    nu54_v2_link owner, board;
    nu54_v2_request in = {NU54_V2_PAY_PREPARE, {7}, 60, body, sizeof(body)}, out;
    struct frames frames = {0};
    assert(nu54_v2_link_init(&owner, NU54_V2_MANAGEMENT, NU54_V2_OWNER, epoch, peer, &journal) == 0);
    assert(nu54_v2_link_init(&board, NU54_V2_MANAGEMENT, NU54_V2_BOARD, epoch, peer, &journal) == 0);
    assert(nu54_v2_link_send(&owner, &in, collect, &frames) == 0);
    assert(frames.count > 1);
    for (size_t i = 0; i < frames.count; ++i)
        assert(nu54_v2_link_receive(&board, frames.bytes[i], frames.length[i], 10, &out) ==
               (i + 1 == frames.count ? NU54_V2_NEW : NU54_V2_MORE));
    assert(out.payload_length == sizeof(body) && memcmp(out.payload, body, sizeof(body)) == 0);
    struct frames response = {0};
    nu54_v2_request reply = {NU54_V2_REPLY, {7}, 61, NULL, 0};
    assert(nu54_v2_link_send(&board, &reply, collect, &response) < 0);
    assert(nu54_v2_link_send(&owner, &in, collect, &response) < 0);
    reply.expires = 60;
    assert(nu54_v2_link_send(&board, &reply, collect, &response) == 0);
    assert(nu54_v2_link_receive(&owner, response.bytes[0], response.length[0], 10, &out) == NU54_V2_NEW);
    assert(nu54_v2_link_receive(&board, frames.bytes[0], frames.length[0], 15, &out) < 0);

    assert(nu54_v2_link_init(&board, NU54_V2_MANAGEMENT, NU54_V2_BOARD, epoch, peer, &journal) == 0);
    assert(nu54_v2_link_receive(&board, frames.bytes[0], frames.length[0], 10, &out) == NU54_V2_MORE);
    assert(nu54_v2_link_receive(&board, frames.bytes[1], frames.length[1], 15, &out) == NU54_V2_ERROR);

    assert(nu54_v2_link_init(&board, NU54_V2_MANAGEMENT, NU54_V2_BOARD, new_epoch, peer, &journal) == 0);
    assert(nu54_v2_link_receive(&board, frames.bytes[0], frames.length[0], 10, &out) < 0);
    assert(nu54_v2_link_init(&board, NU54_V2_MANAGEMENT, NU54_V2_BOARD, new_epoch, peer, &journal) == 0);
    assert(nu54_v2_link_init(&owner, NU54_V2_MANAGEMENT, NU54_V2_OWNER, new_epoch, peer, &journal) == 0);
    frames.count = 0;
    assert(nu54_v2_link_send(&owner, &in, collect, &frames) == 0);
    for (size_t i = 0; i < frames.count; ++i)
        assert(nu54_v2_link_receive(&board, frames.bytes[i], frames.length[i], 16, &out) ==
               (i + 1 == frames.count ? NU54_V2_RESUME : NU54_V2_MORE));

    assert(nu54_v2_link_init(&owner, NU54_V2_NEARBY, NU54_V2_CUSTOMER, epoch, peer, &journal) == 0);
    assert(nu54_v2_link_send(&owner, &in, collect, &frames) < 0);

    uint8_t third[16] = {3}, fourth[16] = {4};
    assert(nu54_v2_link_init(&owner, NU54_V2_MANAGEMENT, NU54_V2_OWNER, third, peer, &journal) == 0);
    assert(nu54_v2_link_init(&board, NU54_V2_MANAGEMENT, NU54_V2_BOARD, third, peer, &journal) == 0);
    in.id[0] = 8;
    in.expires = 420;
    frames.count = 0;
    assert(nu54_v2_link_send(&owner, &in, collect, &frames) == 0);
    for (size_t i = 0; i < frames.count; ++i)
        assert(nu54_v2_link_receive(&board, frames.bytes[i], frames.length[i], 360, &out) ==
               (i + 1 == frames.count ? NU54_V2_NEW : NU54_V2_MORE));
    assert(nu54_v2_link_init(&owner, NU54_V2_MANAGEMENT, NU54_V2_OWNER, fourth, peer, &journal) == 0);
    assert(nu54_v2_link_init(&board, NU54_V2_MANAGEMENT, NU54_V2_BOARD, fourth, peer, &journal) == 0);
    in.id[0] = 7;
    in.expires = 60;
    frames.count = 0;
    assert(nu54_v2_link_send(&owner, &in, collect, &frames) == 0);
    assert(nu54_v2_link_receive(&board, frames.bytes[0], frames.length[0], 55, &out) == NU54_V2_ERROR);
    return 0;
}
