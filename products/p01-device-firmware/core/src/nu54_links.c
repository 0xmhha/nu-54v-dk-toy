#include "nu54_links.h"

#include <string.h>

static int valid(int i)
{
	return i >= 0 && i < NU54_LINK_MAX;
}

void nu54_links_init(nu54_links_t *r)
{
	memset(r, 0, sizeof(*r));
	r->session = -1;
}

void nu54_links_connected(nu54_links_t *r, int i)
{
	if (valid(i)) {
		r->link[i] = (nu54_link_facts_t){.connected = 1};
	}
}

int nu54_links_disconnected(nu54_links_t *r, int i)
{
	if (!valid(i)) {
		return 0;
	}
	r->link[i] = (nu54_link_facts_t){0};
	if (r->session == i) {
		r->session = -1;
		return 1;
	}
	return 0;
}

void nu54_links_facts(nu54_links_t *r, int i, int bonded, int listening)
{
	if (valid(i) && r->link[i].connected) {
		r->link[i].bonded = bonded;
		r->link[i].listening = listening;
	}
}

int nu54_links_is_phone(const nu54_links_t *r, int i)
{
	return valid(i) && i != r->session && r->link[i].connected && r->link[i].bonded && r->link[i].listening;
}

nu54_link_t nu54_links_call(const nu54_links_t *r, int i, int session_open)
{
	nu54_link_t l = {0};
	if (!valid(i)) {
		return l;
	}
	l.bonded = r->link[i].bonded;
	for (int j = 0; j < NU54_LINK_MAX; j++) {
		l.phone_present |= j != i && nu54_links_is_phone(r, j);
	}
	l.foreign = session_open && r->session >= 0 && r->session != i;
	return l;
}

void nu54_links_after(nu54_links_t *r, int i, int opened_session)
{
	if (valid(i) && opened_session) {
		r->session = i;
	}
}

int nu54_links_session(const nu54_links_t *r)
{
	return r->session;
}
