# NU54 wire v2 transport

Each profile uses certificate-authenticated TLS 1.3 with an explicit trust store, a pinned peer SPKI SHA-256, and exactly one role URI in the peer certificate. Use a fresh handshake on every connection. PSK, session resumption, early data, and v1 fallback are outside this profile. The certificate role URI takes the form `urn:nu54:role:<role>`.

| TLS ALPN | Initiator | Responder | Initiator commands |
|---|---|---|---|
| `nu54-v2-management` | owner | board | manage, pay.prepare, status, clock |
| `nu54-v2-payment` | terminal | board | pay.prepare, status |
| `nu54-v2-nearby` | customer | terminal | nearby.submit, status |

Responders send `reply` and, for explicitly negotiated pending operations, the profile-specific progress described below. Role certificates establish the link policy. The application still establishes current ownership, terminal delegation, or order authority and validates the signed quote before acting. Both payment routes carry the [same signed quote bytes](README.md).

## TLS application bytes

All integers are unsigned big-endian. The TLS exporter uses the label `EXPORTER-NU54-wire2`, the selected ALPN bytes as context, and a 16-byte result. Each authenticated fragment carries that result. Reconnecting requires a new handshake and a fresh exporter. The profile and role are derived from authenticated TLS state.

| Field | Bytes |
|---|---:|
| Fragment length excluding this field | 2 |
| TLS exporter | 16 |
| Message sequence, starting at zero in each direction | 4 |
| Fragment index, starting at zero per message | 1 |
| Total fragment count | 1 |
| Fragment payload | 1–192 |

A message body is command code (1), nonzero request ID (16), expiry in UTC epoch seconds (8), and payload. Commands `manage`, `pay.prepare`, `status`, `nearby.submit`, and `reply` encode as 1 through 5. The C1 `approval.progress` and C2 `rental.review` encode as 6 and 7. The C1 `clock` encodes as 8. Bodies are limited to 2048 bytes and all non-final fragments carry exactly 192 bytes. The receiver closes on an epoch, sequence, ordering, size, role, five-second reassembly budget, or expiry violation. The sequence never wraps. Outer BLE framing must provide an ordered byte stream to TLS and discard connection-local state on teardown. The native TLS owner is responsible for handshake, socket, and inactivity deadlines, including timeouts when no more input arrives.

One request is outstanding per link. A reply carries the same request ID and expiry. Request expiry must be in the future and at most 300 seconds from the receiver's trusted time. `clock` is the one exception to the upper bound: the owner sends it to a board whose clock stood still without power, so the board accepts it however far its expiry lies ahead, and its handler binds the time it carries to that expiry (payload `0x81` and the time as eight bytes; the request must end within 95 seconds after that time). Every other rule applies to `clock` unchanged. Resume sends the original body again over fresh TLS with the same request ID. For the same profile and authenticated peer, the bounded journal distinguishes identical retries from conflicting reuse and retains entries through expiry plus 300 seconds. Journal saturation closes the request path. Durable order state, nonce consumption, and response recovery remain application responsibilities.

Share the journal's trusted-time watermark across live links and reconnects. A backwards clock closes the link before modifying any partial message. If the volatile journal or trusted-clock continuity is lost, recover durable time and operation state before authorizing a replay-sensitive command. A new TLS connection alone does not restore either.

## Owner payment review

Management opcode `0x75` takes the opcode, the nonzero 16-byte ID of the eventual `pay.prepare`, and the full 737-byte signed quote. The normal Wire2 request and reply retain their own separate frame ID and expiry. After authenticating the owner and verifying the signed quote against its installed policy, the board replies with `0x75`, SHA-256 over the entire opcode payload, and one status byte. `1` means the exact quote may proceed to `pay.prepare` once on the same TLS epoch. `0` means the board already recorded SW2 refusal for this original request and quote. The reply is neither an approval signature nor independent display or gas account evidence.

Invalid, expired, conflicting or cross-epoch requests never open a physical prompt. A verified independent display is still required before a board button can decide. SW2 records a refusal before replying, with no payer signature. A durable refusal rejects reuse of the original ID with a different quote, and the same quote with a different ID, until the signed expiry is certainly past. Storage exhaustion and time uncertainty fail closed.

## Implementations and validation

The portable C and TypeScript APIs take a TLS exporter and peer binding from the native TLS engine. The included OpenSSL and Node adapters verify the real TLS state before passing those inputs to the portable code. `make -C packages/protocol test-transport-v2` exchanges both payment routes and the nearby role between separate C/OpenSSL and TypeScript/Node endpoints using ephemeral certificates. Native board and mobile activation requires the provisioned peer identities, a supported TLS backend, and the BLE/device acceptance runs.

The P01 release build pins NCS v3.4.2 and stages Mbed TLS 4.1.2 from a checksum-verified archive. The custom ARM commissioning manifest identifies the linked TLS handshake, TLS 1.3 server and X.509 verification functions and their source hashes. From the firmware product directory, run `sh test/test_mbedtls_peer.sh` to exercise the board adapter with fresh certificates over a local socket pair. The test verifies the owner and terminal roles, compares the exporter at both endpoints, opens a second session and checks rejection of the wrong role and public-key pin. Physical acceptance still requires provisioning the intended board and phone, exercising both payment routes over BLE and verifying reconnection on the devices. [Firmware build and TLS test](../../../products/p01-device-firmware/README.md#authenticated-firmware-tls-dependency) · [Custom ARM build manifest](../../../products/p10-platform/patches/dependencies/firmware-qualification/ncs-3.4.2-custom-commissioning-manifest.json)


### Rental slot cleanup proof v1

`RCI1` and `RCR1` are standalone wire records. P05 durably records an operator's cleanup request after verifying the frozen generation and its final disposition. Starting with schema 095, a separately authorized station can retrieve a signed RCI1 for that original request while the operator's grant, station certificate, generation registration and evidence remain valid. P05 pins the first envelope before returning it. An authorized retry within its execution window recovers identical bytes. An older admission without a recorded finality anchor cannot produce a signed intent. The operator sees `board-receipt-required` after issuance. With schema 097, an authorized station can submit the 109-byte receipt or recover it through the same scoped endpoint. P05 verifies the signature against the evidence signer for the original intent, retains the first accepted bytes and exposes `receipt-authenticated-handoff-required` to the operator. The successor remains held pending independent qualification of retirement and handoff. [Operator admission and recovery](../../../products/p05-operations-backoffice/O09-GENERATION-EVIDENCE.md#protected-cleanup-request) · [Station issuance](../../../products/p05-operations-backoffice/O09-GENERATION-EVIDENCE.md#qualified-station-cleanup-intent-issuance) · [Station receipt and readback](../../../products/p05-operations-backoffice/O09-GENERATION-EVIDENCE.md#authenticated-station-receipt)

| Record | Exact bytes | Fields in wire order |
| --- | ---: | --- |
| `RCI1` | 181 | ASCII magic (4), registered generation binding digest (32), finalized disposition digest (32), immutable operations request ID (32), issued-at (8), expires-at (8), operator signature (65) |
| `RCR1` | 109 | ASCII magic (4), EIP-712 digest of the exact intent (32), retired-at (8), board evidence signature (65) |

Times are unsigned big-endian Unix seconds. Both signatures are secp256k1 low-S with recovery byte 27 or 28. The EIP-712 domain is `NU54 Payment Settlement`, version `1`, with the independently enrolled chain and settlement contract. The struct types are `RentalCleanupIntentV1(bytes32 binding,bytes32 disposition,bytes32 request,uint64 issuedAt,uint64 expiresAt)` and `RentalCleanupReceiptV1(bytes32 intent,uint64 retiredAt)`. The verifier resolves the independently registered operator and board evidence keys and requires distinct signers. Binding, disposition and request must match the independently resolved scope and be nonzero.

The finalized disposition digest commits to the authoritative close and withdrawal evidence for that same generation. Its preimage is ASCII `NU54-rental-cleanup-disposition-v1` followed by a zero byte, the generation binding digest, the authenticated freeze proof digest, the close transaction hash, the finalized evidence anchor block hash, the one-byte disposition kind, and the withdrawal evidence digest, in that order. The hash is Keccak-256. The kind is 1 for a verified withdrawal on the authorized route and 2 for a verified zero/no-transfer disposition. Each digest/hash is 32 bytes. The collector derives these fields from independently verified records, including the freeze proof signed by the enrolled board for the same generation. Resolve that immutable enrollment together with the expected disposition, including for historical readback. Pending, reverted or unverified dispositions cannot supply an expected digest. The host codec receives only the already resolved digest.

Future protected execution must verify a new intent at trusted time within `issuedAt <= now < expiresAt`, allowing at most 120 seconds. It must durably fence that exact rental generation, retire its rental-only material, and commit the same request's receipt before returning it. A failed or interrupted operation yields no signed retirement claim until durable retirement can be verified. Personal keys and other slots are outside this scope. Readback uses the existing authorized request after reconnect or reboot, returns the same previously committed bytes, and does not invoke retirement again. A receipt can be verified after intent expiry when `issuedAt <= retiredAt < expiresAt` and `retiredAt <= now`. P01 can already admit a pending fence with qualified inputs as described below. A releasable completion path also needs protected retirement and durable receipt readback on the target board.

On the authenticated collector connection, request
`rental.cleanup.challenge` and send `rental.cleanup.admit` with the zero session
ID, its active connection nonce, and the exact RCI1 as 362 lowercase hex
characters. Ordinary freeze authority does not confer cleanup station rights. The receiver
verifies the separately registered station, current
nonce and EIP-712 operator signature, then resolves a previously authenticated
local witness under the shared slot guard. The resolver must validate the
original frozen proof and final settlement evidence independently. It returns
the original root, exact registered binding, request, signed-envelope hash,
finalized disposition preimage and both current zero counts. The receiver
recomputes the disposition digest and the storage guard compares the durable
RJ03 projection. The local trusted time must place the witness inside its
freshness window. No external I/O is allowed while the guard is held.

The only successful response is `rental.cleanup.status` with `status: pending`.
A new fence needs an unexpired RCI1. Following reconnect, only the complete
matched existing fence can be recovered after expiry, with a new connection
nonce and a fresh authenticated witness for that same signed envelope. The
cleanup challenge uses the protected enrollment without restoring a signing
journal after reboot. Its nonce cannot authorize a freeze request. Unknown
storage, stale or altered evidence, and unsupported platform hooks give a
refusal. This command issues no RCR1 and performs no key writes.

A legacy `setup.ack`, credential receipt, open-slot receipt, freeze receipt, or finalized withdrawal on its own cannot satisfy this proof. A different generation, disposition, request, operator, board or deployment cannot consume a prior receipt.

The host journal model keeps a pending retirement fence beside the frozen RJ03 image, with a separate internal `RCF1` record. Under the slot guard it checks the durable frozen journal against the verified generation and projection root, and admits only a finalized disposition with zero pending payments and zero pending limits. An atomic create-if-absent pins the exact intent digest, disposition and request. An uncertain write yields an error; a fresh read distinguishes absent from the same committed fence. An absent record after expiry cannot be created. A later read of the same previously admitted fence is recovery bookkeeping only.

A completion receipt requires a protected, generation-scoped retirement adapter that can independently re-establish which material was retired and persist completion and the exact signed receipt across failure. The pending fence contains no completion marker or signature. The fence itself performs no key writes and exposes no customer command.

The experimental P01 PSA adapter allocates a separate rollback-protected WRITE_ONCE asset for the pending RCF1 fence. It uses the existing exact ACTIVE RA01 enrollment and running measurement, and reopens RJ03 read-only to pin or recover the fence. The journal and activation assets keep their previous layout and bytes. A committed fence blocks rental signing reactivation; missing or unreadable authority does not allow writes or fallback to a new generation. The adapter's separate platform-qualification hook defaults to refusal.

The enrolled rental-only signing credential is PSA key slot `0x5403`, whose public address is checked against the enrolled generation. The existing generic key destruction helper considers an absent key a successful result and cannot establish why it is absent, or whether a subsequent occupant of the fixed slot belongs to the original generation. The old wipe also changes shared PIN/nonce data and BLE bonds. A qualified completion path needs generation-bound compare-and-retire under the same secure guard, with trustworthy recovery evidence of that specific retirement even when acknowledgement is lost. The pending fence does not change the physical key or issue RCR1; receipt delivery remains separate work. The single WRITE_ONCE asset is intentionally limited to the guarded experiment. It cannot be reclaimed by the application for the next rental, and a present fence continues to refuse commissioning and signing. The existing RA01 API likewise provides no authenticated handoff to a successor generation. A reusable release needs an authenticated, rollback-protected handoff that atomically advances the active generation and its protected key binding while retaining the previous generation's completion proof for the required readback window. A successor may open only after the handoff verifies prior retirement; retries for the old generation must resolve against its retained record and must never act on the successor key. This adapter may be qualified only for the guarded experiment until that lifecycle is implemented and verified.

The independently gated reusable handoff stage stores one atomic `RH01` image in a separate rollback-protected mutable PSA asset. The image carries the exact predecessor and candidate successor bindings, original RCR1 bytes for the immediately preceding return, and the scoped server acknowledgement digest. A later handoff may replace this bounded history only after independent authorization to reclaim the exact previous receipt bytes, bound by their digest. The first handoff resolves the prior generation from ACTIVE RA01 and checks the matching intent against historical RCF1 when that asset exists. A later handoff compares against the exact durable candidate binding. It must independently prove that generation was enrolled and retired, since a candidate alone proves neither. The historical RCF1 stays readable in its original asset and is never overwritten by handoffs.

In production the trusted provider must independently verify protected retirement, the canonical board signature and matching enrolled intent, and a server-authenticated acknowledgement authorizing the exact prior and successor bindings, the exact terminal receipt, and any explicit reclaim authorization. It must resolve trusted time and evidence rather than accept caller-supplied digests or flags as authority. Its default rejects every input. The successor handoff has no production provider or collector command. The existing cleanup admission and P05 receipt endpoints cover the preceding steps. The host fixture supplies independently held test authority for exact inputs. For a lost write response, rerunning the identical authorized input reads the matching durable record and returns replay. A conflicting or superseded input cannot advance it. Both old and candidate generation signing stay closed; a later protected enrollment and key lifecycle will own opening the successor.

The shared serialized slot guard covers compare plus replace. PSA atomicity is per object and does not make separate asset or key operations atomic. Qualification requires the platform to show power-loss recovery and rollback protection for this mutable asset. [Arm PSA atomicity and storage flags](https://arm-software.github.io/psa-api/storage/1.0/overview/architecture.html#atomicity-of-operations) · [Arm PSA storage requirements](https://arm-software.github.io/psa-api/storage/1.0/overview/requirements.html)

### Optional owner approval progress

Wire command 6 is reserved for one board-to-owner progress notification on a negotiated management link. Commands 1 through 5 keep their existing values and semantics. A link starts with progress disabled. The owner and board enable it only after explicit opt-in on the authenticated idle link and confirmation of the enrolled owner generation. An implementation without that negotiation keeps the legacy request and reply exchange. Management opcode `0x76` is an explicit opt-in request while the authenticated owner link is idle. The request body is `0x76` followed by the current owner generation as eight unsigned big-endian bytes. After verifying current owner and generation, the board echoes this body in the final reply, then enables progress on that same TLS epoch. The owner enables progress after authenticating the exact final reply. This command never stages an approval. A legacy board may close the idle probe. Any legacy retry opens a fresh authenticated session before any `pay.prepare` was sent. Loss, timeout or a malformed response after `pay.prepare` never authorize retransmission.

Progress reports that the board has accepted an original `pay.prepare` into live local approval. It uses the existing request ID and exact expiry in its frame header. Its body is one byte with value 1 followed by the nonzero owner generation as an unsigned 64-bit big-endian integer. The authenticated TLS epoch, peer role, frame sequence, outstanding request, generation, and original deadline all have to match. The board can report progress once for a freshly accepted request. Receipt or refusal replay sends its original final reply.

Progress consumes a frame sequence but leaves the original request outstanding. The final reply alone resolves that request. Progress carries neither a signature nor a payment result, and gives no authorization to apply firmware. When a reply, expiry, disconnection or session replacement ends the observation, the phone stops showing live board progress. A still-unresolved local approval journal keeps maintenance on hold without asserting that the board is active. The phone verifies the same enrolled board and generation on a fresh authenticated maintenance read before enabling an evidence request.

## Rental payment review (C2, W10)

An authenticated C2 terminal may explicitly request a review receipt with
`pay.prepare` payload `W10Q || customerChallenge[32] || signedCommerceQuote[737]`.
The challenge must be nonzero. The Wire2 request uses the exact original
16-byte payment request ID and the expiry of the original signed quote.
A legacy endpoint rejects this payload before staging. A terminal must never
retry an unsupported or uncertain attempt as a legacy payment automatically.

An N54Q-commissioned renter board can issue `rental.review` (command 7) once
while the fresh matching request remains outstanding. It issues the receipt
only after checking the authenticated terminal, exact signed quote, current
deployment/payer/generation and absence of any durable prior approval or
refusal. A duplicate/resumed request cannot issue a new review. Its terminal
reply and the existing independent signed-display, PIN and physical-button
gates are unchanged.

The 296-byte W10R wire is `W10R || boardSPKI[32] || terminalSPKI[32] ||
epoch[16] || request[16] || customerChallenge[32] || quoteOrderKey[32] ||
signedQuoteSHA256[32] || payer[20] || generationBE[8] || expiresBE[8] ||
signature[64]`. Pins are SHA-256 over canonical DER SPKI and the epoch is
the 16-byte live C2 exporter from outer Wire2. The protected commissioning
INSTALL_ID identity signs SHA-256 of ASCII `NU54 W10 rental payment review v1`,
a NUL byte and the 232 unsigned receipt bytes. The signature is canonical
low-S P-256 P1363, independent of the secp256k1 payer signature. The terminal
may convey these bytes, but cannot manufacture authorization from a receipt
or its own callback. A customer verifier first obtains the currently accepted
handover, pinned installation key, exact original order and quote and
customer's one-time challenge from the authenticated presentation authority.
An unmatched or expired fact leaves the review unconfirmed. The receipt is
a pre-approval review fact and never authorizes signing, settlement or relay.


The customer reaches the terminal over the existing `nu54-v2-nearby` profile.
For a direct QR or W09 original, `rental-review-source` accepts the original
compact quote and classic presentation only after the terminal resolves its
current order status under its existing service role and verifies them against
the authenticated customer. It stores a bounded transient same-link reference.
A separate `rental-review` request presents the current short-lived P05
authority for the same customer, order, digest, request and challenge. This
authority and a fresh terminal order read precede the C2 board request, even
on a newly started terminal. The terminal returns the exact receipt bytes to
the authenticated customer on that same nearby link. Reconnecting discards
the old proof route. Source material and the receipt expire on timers and are
dropped immediately when the link retires. A terminal-local, bounded order
marker alone can remain until the original quote expiry to suppress another
board attempt, with no challenge or receipt attached. These pre-review commands never reserve, issue, re-issue, sign or
forward a payment. Existing display confirmation and handling of a subsequent
physical decision remain separate from this review.

The original C2 terminal reply for an opted-in, live reviewed attempt can
carry a 166-byte `W10D` instead of a bare payout signature or refusal byte.
`W10D || SHA256(W10R)[32] || decision[1] || payerApproval[65] || signature[64]`
uses decision 1 for approval and decision 2 for refusal. A refusal must carry
65 zero bytes and an approval must carry the existing canonical payer
signature for that exact quote. The installation ID uses canonical low-S
P-256 over SHA256 of ASCII `NU54 W10 rental payment decision v1`, a NUL byte
and the first 102 bytes. The board emits W10D only from its existing verified
physical branch after persisting the ordinary approval or refusal.

The nearby `rental-decision` command is an explicit read for the same
authenticated customer, original order, key, request, challenge, quote digest
and review digest. Before delivery the terminal checks current original order
status. Without a verified final receipt it returns pending. Reconnection,
expiry or lost scope cannot replay the old terminal delivery or start a
replacement board approval. When the original reaches P05, status and
receipts are recovered through the original customer scope. If the final
frame is lost after the decision commits locally, the ordinary protected
original payer receipt remains on the board. Republishing an independently
verified decision after that loss needs a future scoped read of the same
durable result and same prior W10R, without new signing authorization.

For a nearby rental review, the kiosk uses each unexpired scan handle only
for a TLS handshake until the authenticated board SPKI matches the signed
current authority. Names, RSSI and the order returned by the scanner confer
no authority. A mismatched or failed handshake receives zero Wire2 application
bytes and is closed before another candidate can be tried. A kiosk maintains
at most one active C2 connection. The first native application write clears
the remaining scan handles and fences the original order until its signed
expiry. After beginning that write, any error is an uncertain result and
the kiosk must not retry on another device. Unstarted or busy requests may
return pending, with the original order still available for an explicit retry.
