#include "nu54_owner_idle.h"
#include <assert.h>

int main(void)
{
    nu54_owner_idle idle = {0};
    assert(!nu54_owner_idle_live(&idle, 1, 100));
    nu54_owner_idle_start(&idle, 1, 100);
    assert(!nu54_owner_idle_live(&idle, 2, 100));
    assert(!nu54_owner_idle_live(&idle, 1, 99));
    for (int64_t now = 100; now < 120100; now += 1000)
        assert(nu54_owner_idle_live(&idle, 1, now)); /* Read-only polling never shifts the boundary. */
    assert(!nu54_owner_idle_activity(&idle, 2, 120000));
    assert(nu54_owner_idle_activity(&idle, 1, 120099));
    assert(nu54_owner_idle_live(&idle, 1, 240098));
    assert(!nu54_owner_idle_activity(&idle, 1, 240099));
    assert(!nu54_owner_idle_live(&idle, 1, 240099));
    nu54_owner_idle_clear(&idle);
    assert(!nu54_owner_idle_live(&idle, 1, 120099));
    nu54_owner_idle_start(&idle, 2, 240100);
    assert(!nu54_owner_idle_activity(&idle, 1, 240100));
    assert(nu54_owner_idle_live(&idle, 2, 240100));
    return 0;
}
