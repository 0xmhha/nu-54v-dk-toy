#ifndef TEST_OWNER_LIMITS_KERNEL_H
#define TEST_OWNER_LIMITS_KERNEL_H
#define K_MUTEX_DEFINE(name) int name
#define K_FOREVER 0
static inline int k_mutex_lock(int *m,int timeout) {(void)timeout; if (*m) return -1; *m=1; return 0;}
static inline void k_mutex_unlock(int *m) {*m=0;}
#endif
