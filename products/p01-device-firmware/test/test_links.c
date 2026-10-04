/* Which central is which (core/nu54_links): the kiosk, the phone app and the operator tool. */
#include <stdio.h>

#include "nu54_links.h"

static int failures;

static void check(int ok, const char *what)
{
	if (!ok) {
		failures++;
		printf("FAIL %s\n", what);
	}
}

enum { KIOSK = 0, PHONE = 1 };

int main(void)
{
	nu54_links_t r;
	nu54_link_t l;

	/* The phone app bonds and listens; the kiosk connects without pairing and opens a session. */
	nu54_links_init(&r);
	nu54_links_connected(&r, PHONE);
	nu54_links_facts(&r, PHONE, 1, 1);
	nu54_links_connected(&r, KIOSK);
	l = nu54_links_call(&r, KIOSK, 0);
	check(!l.bonded && l.phone_present && !l.foreign, "the kiosk's session.open: unpaired, phone present, not foreign");
	nu54_links_after(&r, KIOSK, 1);
	check(nu54_links_session(&r) == KIOSK, "the kiosk holds the session");
	check(nu54_links_is_phone(&r, PHONE) && !nu54_links_is_phone(&r, KIOSK), "the phone is the phone app");

	/* The phone app's device.paymentMode during the kiosk's session: foreign, bonded. */
	l = nu54_links_call(&r, PHONE, 1);
	check(l.bonded && l.foreign && !l.phone_present, "the phone's message in the kiosk's session is foreign");
	nu54_links_after(&r, PHONE, 0);
	check(nu54_links_session(&r) == KIOSK && nu54_links_is_phone(&r, PHONE), "the phone's message takes nothing over");

	/* The phone stops listening: no phone app present for the kiosk. */
	nu54_links_facts(&r, PHONE, 1, 0);
	check(!nu54_links_call(&r, KIOSK, 1).phone_present, "a phone that does not listen is not present");

	/* The kiosk leaves: the session ends with it; the phone's leaving would not. */
	nu54_links_facts(&r, PHONE, 1, 1);
	check(nu54_links_disconnected(&r, PHONE) == 0, "the phone's link held no session");
	check(nu54_links_disconnected(&r, KIOSK) == 1 && nu54_links_session(&r) == -1, "the kiosk's link held the session");

	/* The operator tool (bonded) opens a setup session: it holds it and is not taken as the phone. */
	nu54_links_connected(&r, PHONE);
	nu54_links_facts(&r, PHONE, 1, 1);
	nu54_links_after(&r, PHONE, 1);
	check(!nu54_links_is_phone(&r, PHONE), "the session's own bonded link is not the phone app");
	check(!nu54_links_call(&r, PHONE, 1).foreign, "messages on the session's link are not foreign");

	/* Facts for a link that is not connected are ignored; out-of-range links are safe. */
	nu54_links_facts(&r, KIOSK, 1, 1);
	check(!r.link[KIOSK].bonded, "facts need a connection");
	check(!nu54_links_is_phone(&r, 7) && nu54_links_disconnected(&r, -1) == 0, "out-of-range links");

	printf("%s: links, %d failures\n", failures ? "FAIL" : "ok", failures);
	return failures ? 1 : 0;
}
