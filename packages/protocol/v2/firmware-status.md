# Owner read of the running firmware

W12 update preparation uses a live read from the enrolled board. C1 owner-management operation 119 (`0x77`) reads the running image. It neither opens an update session nor reads the candidate slot. The board accepts the request only on authenticated owner management while the current owner and generation match, the installation policy is current, and the board is available to receive a normal management request. The request is nine bytes: `0x77`, followed by the nonzero current owner generation as a big-endian `u64`. Existing C1 framing authenticates the board pin, TLS epoch, request ID, and original expiry. This read expires within ten seconds.

A successful reply is exactly 86 bytes. All integers in the reply are big-endian.

| Offset | Size | Value |
| --- | --- | --- |
| 0 | 1 | Operation `0x77` |
| 1 | 1 | Format version `1` |
| 2 | 1 | MCUboot image major version |
| 3 | 1 | Image minor version |
| 4 | 2 | Image revision |
| 6 | 4 | Image build number |
| 10 | 4 | Nonzero security counter from the measured protected MCUboot TLV |
| 14 | 8 | Current owner generation, equal to the request |
| 22 | 32 | Current registered board identity digest |
| 54 | 32 | SHA-256 of the running MCUboot header, payload, and protected TLV area |

The source reads the active slot through MCUboot, checks MCUboot confirmation before and after the read, and measures the same bytes that provide the version and counter. A missing or malformed counted-image header or protected TLV, inaccessible slot, unconfirmed trial image, or unavailable production adapter yields no positive fields. A supported command may return just `0x77` for unavailability. A legacy implementation may close the idle management link. Neither outcome authorizes a retry of an approval or a firmware write.

W12 clears the current reading on profile change, loss of the activity view, or failed read. It displays a version only for the same registered board/generation and preserves the separate authenticated post-boot campaign proof (`0x10`, N54F).

Candidate eligibility requires a signed release from the selected deployment channel with the correct model/revision, protected counter and image digests. P05 retains its operator and station role checks. Last campaign acknowledgement can report historical installation outcome. It cannot supply the live image, current power, live approval or passkey occupancy. This wire read supplies no candidate, power or idle claim. The phone keeps update preparation on hold for any required fact without a qualified source. Flashing and confirmation remain separate MCUboot and physical operator actions.

## Offline candidate verification for an enrolled owner

Candidate preparation needs three independently sourced facts. The owner's
authenticated running-image read supplies the current board, owner generation,
protected counter and measurement. A deployment trust record must supply its
channel, the independently established physical revision of that particular
board, and the trusted release and MCUboot keys. The detached P10 release
descriptor supplies the candidate version, revision, channel, counter and image
digests. The current setup receipt is historical and lacks physical revision and
release publisher authority. The MCUboot application target identifies a build,
and does not identify the specimen revision.

The owner handoff is a purpose-specific station-signed verification
receipt carrying the verified descriptor and bound to the current board,
generation, running measurement and counter. The custom station command `products/p10-platform/release/fota_candidate.py`
uses the detached publisher signature and the existing P05 `verify-firmware`
binary to verify one snapshot of the image and protected measurement before
signing. Supply the compiled verifier and public keys through the trusted
deployment process. The station checks the SPKI SHA-256 pins of the three
distinct Ed25519 identities against its deployment trust document. The phone must already trust the relevant
deployment and station. Document import supplies only untrusted input. Candidate
selection must reject another board, stale read, wrong channel or revision,
untrusted keys, altered descriptor or a counter that does not strictly advance.
An absent qualified trust record yields an unconfirmed candidate. The existing production
policy has no official release pin and the candidate verifier accepts only the
custom channel. Custom lab fixtures may establish
an isolated trust record using test keys. No automatic production enrollment or
pin registration is provided. The approved build would independently pin the
board fingerprint, exact physical revision, channel, station and publisher
public keys, and the MCUboot public key. A board-family name or labels from a
candidate document cannot establish revision trust. A historical verified W04
installation for the same board blocks a conflicting custom channel or
MCUboot pin. W04 stores a DER-SPKI SHA-256 digest. The app derives it from the
raw pinned boot public key before comparison. A different board's record has
no authority over this board and the historical generation does not supply
the current owner's generation.

The owner may explicitly share the running comparison context for this board
to the station. This context contains the board fingerprint, generation,
measurement and counter. The station uses those values as the comparison
target. The phone matches the signed station result against an independent
authenticated running read after the owner chooses the document or confirms its
contents. Files and clipboard input supply a bounded envelope containing exact
UTF-8 station payload and detached signature in lowercase hexadecimal. Its
payload `nu54-firmware-candidate-v1` carries the board, decimal-string
generation, current measurement and counter, model, MCUboot SPKI SHA-256,
exact P10 descriptor and its detached publisher signature, and integer UTC
issue/expiry. The verifier caps the lifetime at five minutes. Inspection
neither stores the document in the wallet profile nor sends it to a service.

Preparation is read-only. Cancelling, failed verification, leaving the screen,
changing owner or entering a pending approval removes the displayed candidate.
The candidate receipt authorizes no update, power claim, signer action, payment
retry, campaign action or firmware write. The existing post-installation campaign
request and receipt retain their separate wire and authority.
