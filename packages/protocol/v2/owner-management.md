# C1 authenticated owner management operations

The first byte of a C1 `manage` payload selects the owner operation. C1 binds the request ID, owner identity, session epoch and expiry. A pending mutation excludes other management operations. Public reads and proposals require the authenticated owner session. Physical mutations require the current owner generation and the corresponding trusted local confirmation. The dispatcher switch compiles the full registry together so a duplicate operation number is a build error.

| Operation | Purpose |
| --- | --- |
| 1 | Begin owner registration or recovery challenge |
| 2 | Complete owner challenge proof |
| 3 | Read owner state |
| 4 | Read backup or restoration receipt |
| 5 | Initiate the independent seed restore protocol on the target |
| 6 | Initiate the independent seed backup protocol on the source |
| 7 | Read the authenticated EVM account. Optionally append the 32-byte reserved grant digest and `expiry:u64` to verify expiry against the pinned board. A successful extended response appends the exact echoed digest and expiry followed by the board’s trusted lower time bound as `u64`. |
| 8 | Reclaim an archived receipt following the signed reconciliation decision |
| 9 | Review and sign an authorized EVM transfer |
| 10 | Read the derived Bitcoin, Solana or XRP Ledger public account. Bitcoin may append a nonzero `addressIndex:u32` (1–9999) to the legacy 11-byte request. The response echoes the complete request before public fields. |
| 11 | Review and sign an authorized non-EVM transfer |
| 12 | Read the current install authority public point and installation lease revision |
| 13 | Review and export the Bitcoin public extended key. The optional final byte 48 selects the registered-capable BIP48 participant key. A seven-byte command remains BIP84. |
| 14 | Review and revoke an EVM allowance |
| 15 | Review and sign a bound SIWE login |
| 16 | Read the signed acknowledgement for a bounded post-boot firmware campaign. |
| 17 | Review a new, write-once BIP48 2-of-3 policy and record its SHA-256 digest |
| 18 | Review and append the registered BIP48 participant's ECDSA signature to an exact PSBTv2 |
| 19 | Read the authenticated BIP48 policy digest for this account |
| 20 | Review an external application's EVM native transfer. Bind the application context and the decoded transaction to the board's approval. |
| 21 | Review a transaction under the registered test-market profile and its separate signing account. |
| 22 | Read the derived market account address within the owner session. |
| 23 | Read the experimental owner spending policy for the enrolled board and generation. Interpret the status and length according to the enabled profile. |
| 24 | Stage a single-destination cap change for local review and delayed activation. |
| 25 | Apply a pending single-destination change once the trusted time bounds permit it. |
| 26 | Initialize the single-destination policy for an eligible fresh personal key after local approval. |
| 27 | Reconcile a reservation using independently signed finality evidence and its associated raw transaction. |
| 28 | Start enrollment in the experimental finality-only policy using a signed grant. |
| 29 | Resume an already prepared finality-only enrollment for the same board, generation and signer. |
| 31 | Submit signed replacement evidence and the original and replacement transactions for reconciliation. |
| 32 | Read the active recipient set and any pending policy change. |
| 33 | Review a recipient set change with the original caps. |
| 34 | Review a cap change with the original recipient set. |
| 35 | Activate the identified pending change after the required hold. |
| 112–114 | Read locate capabilities, request a bounded LED indication, or stop that indication. The request is bound to the current owner and expires within ten seconds. |
| 115 | Read the authenticated idle policy. The five-byte response contains the operation, version, flags and a big-endian timeout in seconds. The current personal-board profile reports version 1, flags 3 and 120 seconds. |
| 116 | Attest the current protected rental registration under the enrolled installation identity. The owner sends an installation-authority-signed N54G statement. The board returns that statement with its own installation-key signature. |
| 117 (`0x75`) | Read the board’s decision on an exact signed commerce quote before preparing local approval. See [owner payment review](transport.md#owner-payment-review). |
| 118 (`0x76`) | Negotiate the optional read-only approval progress notification before sending the original approval. See [owner approval progress](transport.md#optional-owner-approval-progress). |
| 119 (`0x77`) | Read the confirmed running MCUboot version, protected counter, and image measurement for the current owner generation. See [running firmware](firmware-status.md). |
| 120 (`0x78`) | Observe the qualified charger's current VIN condition. See [firmware preparation power evidence](firmware-power.md). |

Operations 10, 11, 13, 17–19 are enabled in the multichain laboratory build. Numerical fields use big-endian order unless the embedded chain format specifies otherwise. On a physically refused pending request, the board returns the single byte zero. A reply is scoped to the live request. [Non-EVM wire definitions and operating profile](../../../products/p01-device-firmware/MULTICHAIN-LAB.md).

Policy operations depend on the enrolled profile and firmware configuration. Finality-only profiles refuse policy mutations 24–26 and 33–35. Operation 23 retains its legacy singleton format and returns unavailable when the active or proposed set cannot be expressed in it. Revision 2 reads represent up to eight sorted, distinct 20-byte addresses. The empty set blocks new transfers.

### Protected rental installation evidence

Operation 116 (hex `0x74`) is available on the experimental protected-rental profile with authenticated owner management, counted update and confirmed MCUboot image. The 440-byte request is the operation byte followed by the 375-byte N54G statement and the installation authority's 64-byte P-256 signature in fixed-width `r || s` form. The reply is the operation byte followed by the same statement and the board installation key's 64-byte signature. Both signatures cover SHA-256 of the statement.

The board requires a current installation policy and owner link, active measured rental generation, original first nonce, distinct enrolled freeze signer, and authorized collector BLE peer. A missing qualification, expired statement, wrong scope, recovery session or withdrawn owner is rejected. No payment signature or protected journal image is returned. The N54G statement fields and private P05 registration handoff are specified in [O09 generation evidence](../../../products/p05-operations-backoffice/O09-GENERATION-EVIDENCE.md#수집기-등록-확인과-연결별-발급).

### Revision 2 policy messages

A set occupies 161 bytes: one count followed by eight 20-byte address slots. Unused slots are zero. Address order is lexicographic on the unsigned bytes. The wire codec rejects unused nonzero bytes, duplicates and a count above eight.

| Operation | Request after the operation and version byte (2) |
| --- | --- |
| 32 | No additional bytes. |
| 33 | Active revision (u64), followed by the proposed set. |
| 34 | Active revision, proposed per-transfer cap, proposed daily cap (all u64). |
| 35 | Active revision (u64), followed by the 16-byte original pending request ID. |

An unavailable read returns `[32, 2, 0]`. An available read returns `[32, 2, 1]` followed by board (32 bytes), owner generation, revision, chain (each u64), asset (20 bytes), per-transfer cap, daily cap, and day spent (each u64), active set, pending kind (0 none, 1 legacy, 2 recipient, 3 cap), earliest activation, candidate per-transfer and daily caps (each u64), original request ID (16 bytes) and candidate set. The available reply has 466 bytes. An absent candidate is all zero. A successful proposal replies with operation, version 2, status 1 and earliest activation (u64). A successful activation replies with operation, version 2 and status 1. The authenticated app reads the policy again after a missing reply and verifies the committed revision and fields before confirming activation.

Activating a recipient or cap proposal preserves the day-spent value and every existing reservation. A legacy pending change retains its original reservation and day-rollover rules. A cap-only proposal carries both existing recipient sets unchanged. An existing pending change must be resolved before another can be proposed. The board compares the activation request with the original pending request ID and revision and checks its trusted time lower bound. [Firmware dispatcher](../../../products/p01-device-firmware/app/src/device_pilot.c) · [Wire sizes and codecs](../../../products/p01-device-firmware/core/include/nu54_owner_limits_wire.h) · [Owner policy and qualification](../../../products/p01-device-firmware/W27-POLICY-PROGRESS.md)

### Independent display for operations 24, 26, 28, 33 and 34

During a live review, the public display characteristic exposes only a random 32-byte challenge. Offset reads return a slice of the same challenge. The wallet can show the current operation, 16-byte authenticated session binding, 16-byte request ID and decimal Unix expiry as a single-use QR. The owner presents it locally to the registered display. The display accepts at most 110 seconds of remaining validity and reconstructs the statement from its independent policy or enrollment record, the request binding and the challenge. The display signs only after local review. The board checks the registered display's signature against its own statement and checks the original approval again at SW1. For operation 28 both the owner request and the issuer grant must still be live. The phone removes the QR when the request ends, expires, or the app leaves the foreground. [Wallet session](../../../products/p02-user-app/src/wallet/ownerSession.ts) · [Wallet display](../../../products/p02-user-app/src/wallet/WalletHome.tsx) · [Independent verifier](../../../products/p01-device-firmware/recovery-accessory/confirm_board.py) · [Board confirmation](../../../products/p01-device-firmware/app/src/device_pilot.c)

For operations 33 and 34, the display's independent policy record contains `board`, `generation`, `revision`, `chain`, `asset`, `previous_recipients`, `recipients`, `previous_per_transfer`, `previous_daily`, `per_transfer` and `daily`. The display shows both full address lists and both cap pairs in asset base units. `N54R` occupies 498 bytes and binds the board, TLS session, request, challenge, generation, revision, chain, asset, current caps, earliest activation, expiry and both sets. `N54M` uses the same fields with equal sets, then appends the two proposed caps for 514 bytes. Earliest activation equals request expiry plus 86,400 seconds for both statements. [Exact statement encoding](../../../products/p01-device-firmware/core/src/nu54_owner_limit_change.c)

### Expiry check for operation 7

The extended operation 7 keeps the legacy one-byte request and 97-byte response intact. The extended request is 41 bytes and the response is 145 bytes. It requires the active authenticated owner and an idle management session. The board writes the expired grant timestamp to the protected replay floor before replying. EVM and non-EVM transfer staging require expiration newer than that durable floor even after board restart. Any failure to observe or fence the requested expiration leaves the phone reservation in place.


The existing policy initialization (26), change (24) and finality enrollment
(28) use a public 32-byte random display challenge while the authenticated
request is pending. The independent display reconstructs the canonical
statement from that challenge, the in-person one-time owner session binding
and independently verified policy or grant fields. A registered display key
signs those exact bytes before the board enables its physical approval
prompt. [Independent display handoff](../../../products/p01-device-firmware/W27-RECONCILIATION-DESIGN.md#independent-display-handoff)
