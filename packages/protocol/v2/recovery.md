# NU54 offline recovery proof v1

The first profile is root format 2, profile 1, path 1, account 0 and StableNet
test profile chain ID 8283. Its first key is secp256k1 BIP-32
`m/44'/60'/0'/0/0`. Integers are unsigned and big-endian. Role credentials
are independently provisioned P-256 keys. The phone receives public evidence
over authenticated management after recovery. Seed and capsule transport is
confined to the separate offline accessory channel.

Stable backup claim bytes, signed by the original board:

| Offset | Field | Bytes |
| --- | --- | ---: |
| 0 | ASCII `NU54-backup-claim-v1` followed by NUL | 21 |
| 21 | root format, profile | 2 |
| 23 | original enrolled device identity | 32 |
| 55 | original owner generation | 8 |
| 63 | path | 1 |
| 64 | account | 4 |
| 68 | chain ID | 8 |
| 76 | recovered secp256k1 uncompressed public key | 65 |
| 141 | EVM address | 20 |

Fresh restore receipt bytes, signed separately by the target board and the
independently provisioned accessory:

| Offset | Field | Bytes |
| --- | --- | ---: |
| 0 | ASCII `NU54-recovery-receipt-v1` followed by NUL | 25 |
| 25 | restore challenge | 32 |
| 57 | request ID | 16 |
| 73 | root format, profile | 2 |
| 75 | original enrolled device identity | 32 |
| 107 | target enrolled device identity | 32 |
| 139 | path | 1 |
| 140 | account | 4 |
| 144 | chain ID | 8 |
| 152 | recovered secp256k1 uncompressed public key | 65 |
| 217 | EVM address | 20 |
| 237 | target owner SPKI SHA-256 | 32 |
| 269 | target authenticated session epoch | 16 |
| 285 | original and target owner generations | 8 each |
| 301 | purpose (1 means restore) | 1 |
| 302 | issue time and expiration | 8 each |

The envelope is the 318-byte receipt, then one uint8 length and canonical DER
ECDSA signature for each signer in order target board, accessory, original
board. Sign with ECDSA P-256/SHA-256 over the receipt or stable claim as specified.
Verify the original claim reconstructed from the public values carried by the
receipt. The target signs only after its own independently derived public key
and address match the original claim. The accessory signs only on local
authorization for this target identity, owner epoch, challenge and wallet
profile. The original signer supplies its public claim at backup time so that
the source device can be offline or unavailable during restore.

Each signer is bound to a separately enrolled trust pin. Source and target
IDs must be distinct. Verify the exact current challenge and request, target
generation and session, expiry, and every signature before reporting verified
restore. A supported restored address becomes visible only after verified
receipt and durable local commit. Device reset, expired or replayed sessions,
partial capsules and unsupported profiles preserve existing roots and expose
no new address.

The management route accepts `[5]` to open a restore context and `[6]` to
open a source backup context. Both replies carry the operation byte, 32-byte
challenge, 16-byte management request ID, 32-byte local device identity,
32-byte active owner SPKI hash, 16-byte management TLS exporter epoch,
8-byte active generation and 8-byte expiry. A reply containing just the
operation means unavailable. Management `[4]` fetches `[4, envelope]` while
the target's original authenticated session and context are current. An
empty result is `[4]`. An already restored target can repeat verification
for a fresh context only if its existing sealed root derives the same
public key and address.

The private offline route uses mutual TLS 1.3 with ALPN `nu54-recovery/1`,
independent enrolled accessory credentials and no resumption. Each record
below is prefixed by its two-byte big-endian length inside TLS. The source
and target boards consume local PIN and captured physical authorization
before connecting. The accessory independently confirms the enrolled board
and request on its own panel.

- Source sends the public operation-6 context. The accessory sends
  `[10]` after independent panel confirmation. The source then sends the
  capsule `[3, stable claim, signature length, signature, 32-byte seed]`.
  The accessory independently derives and checks the account before sealing
  the capsule. It replies with `[7]` for sealed backup or
  `[8, earlier verified receipt]` after a previously completed restore.
- Target sends the public operation-5 context. After panel confirmation,
  the accessory sends the sealed backup's decrypted capsule inside this
  dedicated TLS session. The target independently derives the account and
  sends the fresh receipt body with its length-prefixed signature. The
  accessory returns the complete three-signature envelope. The target
  validates it, commits the immutable root, and sends
  `[9, SHA-256(envelope)]` within the same independently authenticated
  session. Only this last acknowledgement permits the accessory to persist
  the proof for later source acknowledgement.

The private route and its capsule are unavailable to management, payment,
USB diagnostics and logs.
