#ifndef NU54_COMMISSIONED_CHAIN_H
#define NU54_COMMISSIONED_CHAIN_H

#include <stdint.h>

/* Signed N54M network codes. Keep laboratory 1 and the historical mainnet
 * derivation namespace 0 distinct from commissioned public test networks. */
#define NU54_CHAIN_COMMISSIONED 2

/* Reserve hardened BIP44 accounts 100–199 for commissioned XRPL Testnet.
 * Legacy namespaces use 0–99. XRPL legacy transactions lack a network ID. */
static inline uint32_t nu54_chain_derivation_account(uint8_t chain, uint8_t network, uint32_t account)
{
	return chain == 3 && network == NU54_CHAIN_COMMISSIONED ? account + 100U : account;
}

static inline const char *nu54_commissioned_anchor(uint8_t chain)
{
	switch (chain) {
	case 1:
		return "00000008819873e925422c1ff0f99f7cc9bbb232af63a077a480a3633bee1ef6:"
		       "00000002a5e0ba0498f1e9f4591af0b66b63c654665efe65206fd0ae7bbaf923";
	case 2:
		return "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG";
	case 3:
		return "1:60691D3CA08E04064EBB014D0ADE8DA2CB2BFE0AE9D820D88502FB3A9A8F7149";
	default:
		return 0;
	}
}
#endif
