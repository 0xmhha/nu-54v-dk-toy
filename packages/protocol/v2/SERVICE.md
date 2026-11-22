# v2 authenticated services

The [interface registry](interfaces.json), [service rules](service-contracts.json), and [fixtures](service-vectors.json) ship in the SDK. Check the Go and TypeScript bindings with `python3 packages/protocol/v2/generate_services.py --check`. Go provides the owner-side API in `packages/protocol/go/service`. TypeScript clients import from `@nu54/protocol/v2` and validate the request binding and authenticated reply. The Node TLS adapter is available from `@nu54/protocol/v2/node`.

## Boundary and retry

- The envelope carries a version, contract/operation, random UUIDv4 request ID, opaque domain and object UUIDv4, UTC expiry, and a closed set of typed fields. The gateway authenticates tenant, subject, role and exact domain/object/operation grant independently. The Go JSON entry rejects duplicates, unknown keys, embedded tenant claims, nested unrecognized objects and envelopes beyond 4096 bytes.
- Request TTL is capped by both the operation and the credential. Numeric command fields use canonical unsigned decimal strings at the declared width. A mutation needs a random UUIDv4 idempotency key. Reads, including recovery reads, omit it.
- SHA-256 over ordered UTF-8 length-prefixed fields binds scope, operation and body to the intent. A second binding adds the request ID and expiry. Retry with the same key and intent uses the owner’s persisted receipt; changing intent conflicts. Unknown results reconcile the original receipt and request digest before any new operation. The SDK validates bindings. Durable side effects and receipt writes belong to the owner’s transaction.
- Roles and context are product-specific. C3 forwarded payment requires trusted order approval. C4 matches the approved key generation and preimage. C5 matches verified quote/transaction bytes, payer, fee payer, asset, nonce and fee budget. A payer may pay their own fee; a sponsor must be a distinct fee payer. This validates metadata only; it supplies no new chain profile or submitter. C6 matches the trusted manifest/ABI, block parent, height, log and lease fence. C8 grants separate market and travel purposes. C9 scopes issuer/audience or the provider, purpose, consent version and expiry. Revoked optional consent blocks new invocation. A rejection does not change payment authority.
- A C7 finalized reply binds the scope, request digest, evidence ID, active generation, anchor hash, chain fact, order, merchant, transaction and bounded observation age. Unobserved means the reader supplied no finalized fact and gives no absence or cancellation authority. Revocation claims require their own typed evidence. The first P07 adapter issues finalized observations and safe unobserved replies from the active projection.

## Product paths

`go run ./cmd/indexer evidence` in P07 serves `POST /v2/evidence/read` on an independently configured TLS 1.3 listener. It needs `P07_EVIDENCE_DATABASE_URL`, `P07_EVIDENCE_CA`, `P07_EVIDENCE_CERT`, `P07_EVIDENCE_KEY`, and `P07_EVIDENCE_GRANTS`. The bind address defaults to `127.0.0.1:8443` and is overridden by `P07_EVIDENCE_HTTP_ADDR`. The database login is a member of `nu54_p07_read` and uses the migrated `chainview.active_evidence` view. The grants JSON maps the lowercase SHA-256 of each CA-verified client certificate’s DER bytes to a principal with `tenantRef`, `subjectRef`, `role`, `expiresAt`, and `grants`. Each grant binds a generation (`domainRef`) and opaque evidence reference (`objectRef`) to operations including `C7/evidence.read`. Certificate issuance and grant rotation are deployment controls.

P05's C7 client validates the request binding and exact merchant/order fact against authorized local state. The P06 Solidity boundary test consumes the same v2 quote fixture as C, TypeScript, Go and Python and detects a changed payout. The P02 and P04 portable client APIs come from the same TypeScript package. v1 firmware, user, kiosk, operations and StableNet settlement paths retain their existing entry points. New order creation, signing, transaction broadcasting, market, travel and provider execution require their planned owner integrations and approvals.

### C3 private merchant gas proof

A C3 private quote presentation can carry an optional `gasSponsor` with a lowercase 20-byte EVM `account`, integer `expiresAt` in Unix seconds, 32-byte hex `grant` commitment digest, and 64-byte hex Ed25519 `signature`. C2 conveys the same proof in the existing encrypted quote presentation after confirming the original customer. The v2 quote bytes and their EIP-712 merchant signature keep their existing format. Prior private presentations remain readable.

The presentation authority signs `NU54-merchant-gas-sponsor-v1` and a zero byte, SHA-256 of the existing private presentation message, SHA-256 of all 737 signed quote bytes, the relay account bytes, the grant digest, and the expiry as an 8-byte big-endian unsigned integer. The domain identifies the merchant relay as the gas sponsor. The account must differ from the quote payer. Proof expires at the earliest of the quote deadline, the enrolled grant deadline and the end of the 30-second presentation window. The board verifies the signed quote through its separate [payment review](transport.md#owner-payment-review) session.


```sh
go test ./packages/protocol/go/service
pnpm --filter @nu54/protocol test
go test ./products/p07-indexer/internal/evidencev2
make -C products/p10-platform sdk-candidates SDK_OUT=/tmp/nu54-sdk-v2-candidate
```

Set `P07_EVIDENCE_TEST_DATABASE_URL` to a migrated local scratch PostgreSQL socket to run the database integration test. It uses a rolled-back fixture to verify the actual read role can access active evidence and that inactive or wrong-generation evidence is excluded. Loopback TLS tests use ephemeral certificates.
