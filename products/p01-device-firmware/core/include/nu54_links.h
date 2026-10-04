/*
 * Which connected central is which (payment-protocol.md 3). The kiosk writes without pairing;
 * the phone app and the operator tool are bonded. The central whose session.open opened the
 * device's session holds the session; a bonded, subscribed central that does not hold it is the
 * renter's phone app and gets confirm.show, confirm.limit and the forwarded payment.outcome.
 *
 * Pure state: the board reports connections and per-link facts (bond level, subscription) and
 * asks, per message, what nu54_session_handle needs to know about the link. Not thread-safe:
 * the board calls it from its session work queue only. Board-independent: tested on the host.
 */
#ifndef NU54_LINKS_H
#define NU54_LINKS_H

#include "nu54_session.h"

#define NU54_LINK_MAX 2

typedef struct {
	int connected;
	int bonded;    /* bonded at the level the device's state needs (the board decides) */
	int listening; /* subscribed to TX notifications */
} nu54_link_facts_t;

typedef struct {
	nu54_link_facts_t link[NU54_LINK_MAX];
	int session; /* the link holding the device's session, or -1 */
} nu54_links_t;

void nu54_links_init(nu54_links_t *r);
void nu54_links_connected(nu54_links_t *r, int i);

/* Link i went away. Returns 1 when it held the session: the caller ends the session. */
int nu54_links_disconnected(nu54_links_t *r, int i);

/* The board's current facts about link i (bond level and subscription can change any time). */
void nu54_links_facts(nu54_links_t *r, int i, int bonded, int listening);

/* What a message on link i is, given whether the device has a session open. */
nu54_link_t nu54_links_call(const nu54_links_t *r, int i, int session_open);

/* After the message on link i: a session it opened makes it the session's link. */
void nu54_links_after(nu54_links_t *r, int i, int opened_session);

/* Link i is a phone app: connected, bonded, listening, and not the session's. */
int nu54_links_is_phone(const nu54_links_t *r, int i);

/* The session's link, or -1: where replies to the button and the PIN go. */
int nu54_links_session(const nu54_links_t *r);

#endif /* NU54_LINKS_H */
