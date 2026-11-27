#ifndef NU54_LED_TEST_KERNEL_H
#define NU54_LED_TEST_KERNEL_H
#include <stdint.h>
#define ARRAY_SIZE(a) (sizeof(a) / sizeof((a)[0]))
#define K_MSEC(ms) (ms)
#define K_NO_WAIT 0
struct k_work { int placeholder; };
struct k_work_delayable { void (*handler)(struct k_work *); };
#define K_WORK_DELAYABLE_DEFINE(name, callback) struct k_work_delayable name = { callback }
struct k_spinlock { int placeholder; };
typedef int k_spinlock_key_t;
static inline k_spinlock_key_t k_spin_lock(struct k_spinlock *lock) { (void)lock; return 0; }
static inline void k_spin_unlock(struct k_spinlock *lock, k_spinlock_key_t key) { (void)lock; (void)key; }
int64_t k_uptime_get(void);
void k_work_reschedule(struct k_work_delayable *work, int delay);
#endif
