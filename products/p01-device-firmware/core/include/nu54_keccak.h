/*
 * Keccak-256 as Ethereum uses it (original Keccak padding 0x01, not SHA3-256).
 * Backed by the Keccak Team's compact implementation in third_party/keccak (CC0).
 */
#ifndef NU54_KECCAK_H
#define NU54_KECCAK_H

#include <stddef.h>
#include <stdint.h>

void nu54_keccak256(const uint8_t *in, size_t len, uint8_t out[32]);

#endif /* NU54_KECCAK_H */
