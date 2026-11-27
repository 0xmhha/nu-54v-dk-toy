# Non-EVM wallet laboratory profiles

The P01/P02 laboratory implements Bitcoin regtest native SegWit, Solana on a local Agave validator, and XRP Ledger in standalone mode. The authenticated owner wallet uses the backup-verified seed already protected by the board HUK and KMU witness. The legacy rental protocol and the EVM backup/profile identifiers remain unchanged.

## Supported profiles

| Profile | Device child | Review and broadcast |
| --- | --- | --- |
| Bitcoin regtest, network selector 1 | BIP32 secp256k1 `m/84'/1'/account'/branch/index`, accounts 0–99, branch 0 or 1, index 0–9999 | PSBT v0 with up to four inputs and outputs. Each input carries the complete stripped previous transaction and a matching public BIP84 path. A witness UTXO, when supplied, must equal the authenticated previous output. Every output, change path, amount, locktime, RBF flag, fee and `SIGHASH_ALL` are reviewed. P01 emits the canonical SegWit transaction. The authority gateway accepts an optional exact outpoint for coin selection and creates a two-output native SegWit draft. |
| Solana local, network selector 1 | SLIP-0010 Ed25519 `m/44'/501'/account'/0'`, accounts 0–99 | One-signer System Program transfer or original SPL Token Program `TransferChecked` with configured mint and precision. Legacy and static v0 messages supported. Payer, source and destination accounts, mint, amount, blockhash and attested fee are bound to the exact message. An expired or invalid blockhash blocks submission. |
| XRP Ledger standalone, network selector 1 | BIP32 secp256k1 `m/44'/144'/account'/0/0`, accounts 0–99 | Canonical single-signature Payment. XRP in drops or allowlisted currency/issuer with exact mantissa and exponent. An issued payment from a holder requires an identical-asset, equal-value SendMax. Canonical flags, Fee, Sequence or explicit Ticket, LastLedgerSequence and optional DestinationTag including zero are reviewed. The adapter compares validated destination, tag and delivered amount before showing completion. |

Policy rejects unsupported scripts and sighashes, alternate Solana programs, lookup tables, v1 messages, Token-2022 extensions, paths, partial XRPL Payments and unapproved asset variants. BIP84 watch-only export returns account-level public material after independent approval. The BIP48 2-of-3 profile is scoped separately for Bitcoin regtest and commissioned Signet. Each wire profile has its own write-once registered policy and returns canonical partial PSBTv2 signatures. The P02 watch export gives both BIP380-checksummed `wpkh(...)` descriptors. The app independently derives the first receive and change addresses from the exported account public data and compares them with authenticated board replies before offering an explicit share action.

Canonical profile references are [BIP84](https://bips.dev/84/), [BIP143](https://bips.dev/143/), [BIP174](https://bips.dev/174/), [Solana message format](https://solana.com/docs/core/transactions/transaction-structure), [TransferChecked](https://solana.com/docs/tokens/basics/transfer-tokens), [SLIP-0010](https://github.com/satoshilabs/slips/blob/master/slip-0010.md), [XRPL Payment](https://xrpl.org/docs/references/protocol/transactions/types/payment), and [XRPL binary format](https://xrpl.org/docs/references/protocol/binary-format).

## Secure owner commands

The C1 request body uses integers in big-endian order. Existing pinned HTTPS/BLE TLS and owner enrollment bind the issuer, physical board and owner generation. These commands are enabled only in a custom laboratory firmware with a separately provisioned independent recovery UART.

| Opcode | Request and response |
| --- | --- |
| 10 | Request `op, chain, network, account:u32, branch:u32`. Bitcoin branch 0 is receive, branch 1 is change. Bitcoin may append a nonzero address index as a trailing `u32` (1–9999). The indexed operation echoes the complete 15-byte request and returns the same suffix; the 11-byte request retains index zero. Solana and XRP Ledger accept branch 0. Response echoes the 11-byte request, followed by a 33-byte public key slot, 20-byte HASH160 and four-byte Bitcoin master fingerprint. Solana pads the 32-byte public key with zero and leaves the remaining fields zero. |
| 11 | 153-byte authority header, 64-byte compact P-256 authority signature, and chain-specific unsigned bytes. Success returns exact native signed bytes. The one-byte zero response denotes physical refusal. |
| 13 | Request `op, 1, network, account:u32` for BIP84 public export. Network 1 is regtest. Network 2 is Signet and requires the commissioned firmware profile and the exact seven-byte BIP84 request. Success echoes the request and adds master fingerprint (4), parent fingerprint (4), chain code (32), and compressed public key (33). P02 builds BIP32 `tpub`, external and internal watch-only descriptors. Refusal returns one-byte zero. |

The transfer header starts with `op, chain, network, account:u32, expiry:u64, maxFee:u64`, followed by 90 bytes of chain policy, 32 bytes of SHA-256 of the canonical ASCII network anchor, and the current lease revision as `u64`. Bitcoin policy bytes are zero and the digest must identify the pinned Bitcoin regtest genesis before review. Solana policy carries blockhash, observed height, last valid height, quoted fee, optional mint, decimals and a token flag. XRP Ledger policy carries current ledger, bounded ledger delta, Sequence and Ticket, exact currency and issuer, the issued-asset flag and reserved zeros. The issuer signs `N54M || boardFingerprint || ownerGeneration:u64 || header || SHA256(unsigned)`. The complete request is at most 1,800 bytes. The board verifies the issuer proof before invoking the bounded parser.

The separately provisioned display reads the 32-byte challenge from the C1 display characteristic on its own BLE connection. It parses the grant raw bytes afresh and shows the chain and full recipient, amount, fee, and chain-specific fields. The display signs `N54D || challenge || SHA256(requestBytes)`. The board checks the registered display proof before starting the physical approval prompt. The same request ID, session epoch, generation and lease must be present when local PIN and SW1 approval complete. Refusal, expiry, disconnection or a failed generation/lease check clears the pending state. An xpub export reads public material only after the same physical gate. The board deletes ephemeral private buffers and Ed25519 handles after use.

The phone receives public metadata and signed bytes over the owner link. It verifies authority scope and the draft before staging, then compares the returned canonical wire and validates the chain signature before creating a submission checkpoint. The app requires a fresh P-256-signed authority timestamp before the first submission, after chain checks, and after recording the attempt. The proof binds a fresh 32-byte nonce, board, owner generation and installation lease to domain `N54C`. The authority retains its last clock floor across process restarts, and the phone uses a monotonic elapsed-time bound. Expired grants and older checkpoints without trusted timing can still be queried. The app writes the submission attempt to protected storage before calling the local RPC gateway. An unknown result remains queryable by the original txid/signature/hash across a restart. The previous owner generation's checkpoint remains query-only if moved during phone replacement.

## Public testnet observation in P02

The owner wallet provides read-only profiles for StableNet Testnet, Ethereum Sepolia, Base Sepolia, Bitcoin Signet, Solana Devnet and XRP Ledger Testnet. The app verifies the configured HTTPS endpoint before storing it. The app limits public queries to the chosen account and asset after an authenticated board account read. The screen shows the observation time and resolution point; current foreground refresh is opt-in. A new observation after relaunch requires fresh board account verification. Changing an endpoint clears cached observations for that network.

Bitcoin Signet can use a synchronized user-operated Bitcoin Core watch wallet. Its bounded receive-address recovery examines historical confirmed usage even after outputs are spent. Alternatively, after agreeing to send one receiving address to the fixed public indexer, the user can see its reported history and balance. The public route validates each included six-confirmation output against raw transaction bytes, the Merkle branch, block header and indexed height. The indexer supplies the active-chain view, unspent status and completeness. Public lookup sends no xpub and does not do multi-address discovery. Both tracks require the default Signet checkpoint at height 321295 before disclosing an address. [BIP 325](https://bips.dev/325/#genesis-block-and-message-start) · [Bitcoin Core v32.1 default Signet parameters](https://github.com/bitcoin/bitcoin/blob/v32.1/src/kernel/chainparams.cpp#L428-L434) · [Esplora HTTP API](https://github.com/Blockstream/esplora/blob/master/API.md)

XRP Ledger Testnet is pinned to network ID and a canonical validated checkpoint. Each account lookup uses the exact current validated ledger hash and requires the account result and an independent ledger call to agree. A provider error or a mismatched ledger fails the read. A matching `actNotFound` reports the account awaiting activation. Other built-in non-EVM public asset observations currently use native amounts. EVM token reads verify token precision and the amount at the same finalized block.

The custom-network editor accepts user-defined EVM read-only profiles with an immutable chain ID and genesis, an HTTPS RPC and explorer, native unit, decimal precision, fee limit and finality rule. Custom descriptors grant no signing authority. Public Bitcoin Signet, Solana Devnet and XRPL Testnet are observation-only in this build. The signer, multisig and replacement workflows still use their separately provisioned laboratory profiles.

## Reproducible verification

Use disposable paths and synthetic fixtures. The commands below build artifacts and perform local software checks without programming a board.

```sh
make -C products/p01-device-firmware test
cmake --build products/p01-device-firmware/out/host --target wallet_chain_node_oracle
pnpm --dir products/p02-user-app exec tsc --noEmit
pnpm --dir products/p02-user-app exec jest --runInBand chainWallet
```

Use official Bitcoin Core 32.1 and Agave 4.4.1 arm64 binaries from their verified release archives. Pin the XRP Ledger standalone container to the digest in the test script. The harness launches Bitcoin with networking disabled and configures Solana to bind to loopback. It runs XRP Ledger with Docker networking disabled and calls its internal loopback RPC through `docker exec`. The Docker image must already be verified and present.

```sh
oracle=products/p01-device-firmware/out/host/libwallet_chain_node_oracle.so
scripts=products/p01-device-firmware/test
python3 "$scripts/run_wallet_chain_nodes.py" bitcoin --oracle "$oracle" --bitcoind "$BITCOIND_32_1"
python3 "$scripts/run_wallet_chain_nodes.py" solana --oracle "$oracle" --validator "$AGAVE_4_4_1_TEST_VALIDATOR"
python3 "$scripts/run_wallet_chain_nodes.py" xrpl --oracle "$oracle"
```

Observed on the isolated nodes:

- Bitcoin Core 32.1 accepted the portable firmware's P2WPKH transaction, then accepted a freshly reviewed same-input RBF version with fee raised from 1,000 to 2,000 sats. It mined the replacement and returned the expected funding txid and destination. A wrong genesis and a fee below the prescribed cap rejected locally before signature or submit.
- Agave 4.4.1 finalized 10,000,000 lamports with a 5,000-lamport fee, then finalized a static v0 SPL transfer of 1,234,567 base units at six decimals using the exact allowed mint. The host rejected a stale block height.
- `xrpld` 3.3.0 validated the XRP Payment with DestinationTag 0 and 5,000,000 drops delivered. It separately validated an issued USD Payment with tag 1234 and exactly 1.234567890123456 delivered from the configured issuer. The host rejected the partial-payment flag.
- Both the host harness and the portable HD regression verify first-account restoration and distinct next-account public material. The native service's PIN, physical buttons, UART and BLE display link need the independent hardware acceptance pass.

The custom ARM build passed with NCS v3.4.1, using 365,036 of 712,560 flash bytes and 172,460 of 258,048 RAM bytes. The build uses a disposable MCUboot signing key, install authority public point and explicit independent recovery UART overlay.

```sh
NU54_LAB_WALLET_CHAINS=1 \
NU54_RECOVERY_OVERLAY="$LAB_RECOVERY_UART_OVERLAY" \
NU54_SIGNING_KEY="$LAB_MCU_BOOT_KEY" \
NU54_INSTALL_AUTHORITY_HEX="$LAB_P256_AUTHORITY_POINT" \
P01_BUILD_DIR="$LAB_BUILD_DIR" \
python3 products/p01-device-firmware/scripts/fw.py build --release --approtect --security-counter=1 --pristine
```

## Local phone and independent review

Start separate disposable local nodes with the same parameters as the verification harness. For Bitcoin, import the board's public receiving descriptor into a local watch-only descriptor wallet and fund it after mining regtest blocks. The local gateway reads the regtest wallet RPC with its cookie. For Solana, fund the payer from the local fixture and provision an eligible token account and mint when testing SPL. Use standalone XRP funding and trust lines from the reference test.

Start `scripts/wallet_chain_lab.py serve` with the laboratory install authority key, local TLS certificate/key, out-of-checkout review directory, board SPKI fingerprint, generation, current lease revision and the independently selected local node RPC URLs. Use the production-like network TLS policy in the phone simulator by installing the disposable lab CA in its trusted root store. Forward local port 8443 to the isolated gateway when the simulator uses a separate loopback, then enter the configured loopback HTTPS endpoint and network anchor. The gateway has per-chain `/bitcoin`, `/solana`, and `/xrpl` endpoints and no external routing. Optional SPL and XRPL asset switches pin the allowed mint/precision or currency/issuer.

In the personal wallet, select the chain and account. **Receive** obtains the authenticated public address before enabling QR/share. The Solana and XRPL QR payloads include the local network label, and the XRP payload carries the optional destination tag. **Send** takes the address, human amount and chain-specific options, prepares a raw transaction using the configured local node, verifies the signed grant and requests board confirmation. The phone does not require a pasted transfer policy. If the user chooses to submit after signing, the same checkpoint supports requery. BTC finality uses six confirmations, Solana uses finalized status, and XRPL requires a validated receipt with exact delivered amount.

On a separately provisioned display host, use `wallet_chain_lab.py display --review <digest.json> --challenge <gatt-challenge> --display-key <show-key>`. Confirm the raw-decoded display summary and send the 64-byte proof on the independent GATT connection. For read-only Bitcoin export, use `display-export --account <zero-based-index> --challenge <gatt-challenge> --display-key <show-key>`. Private authority and display keys are confined to their separate laboratory roles. The normal app receives neither key.

## Bitcoin fee replacement

The normal send screen selects up to four confirmed inputs belonging to the authenticated BIP84 receive key. When the user selects an exact outpoint, that draft uses only that input. A replacement starts from a submitted RBF transaction. The app reads its original recipient and amount, accepts a new total fee in satoshis, and verifies that the draft retains the same ordered outpoints and scripts. It requires the change delta to equal the increase in fee. The gateway reconstructs the same inputs from Core, checks that the original remains unconfirmed and has no unconfirmed descendants, then creates a new authorization.

For a replacement, the signed Bitcoin policy context contains one byte of value 1, the 32-byte original txid in displayed byte order, the previous fee as a big-endian uint64 and 49 zero bytes. This binds the original identifier and previous fee to both the authority and the independent display. The portable firmware checks the exact higher fee, the declared RBF sequence and one independently derived change output. Every replacement needs a fresh local approval and PIN. Before first submit, the app requires the exact original txid in the local Core mempool. The dedicated gateway compares the signed wire with the stored approval, checks Core preflight and expiry, then repeats live original membership immediately before sending. Confirmed, absent, conflicted, ineligible or unknown results block first submission. Core does not expose an atomic compare-and-submit RPC, so the final membership read and the local submit are separate calls. The protected checkpoint carries the ordered chain of prior IDs and re-queries those IDs after restart.

The real-node harness verified two independently funded inputs and six-confirmation incoming balance, rejected changed recipient amount and a context with no fee increase, then mined the exact 1,000-to-2,000-satoshi replacement. Both inputs and the original external payment were unchanged.

## Bitcoin registered multisignature

W17 loads two independent-coordinator descriptors using `wsh(sortedmulti(2,...))` for the external and internal branches. The configured roots contain a master fingerprint, BIP48 account origin and an account-level Bitcoin test `tpub`, using `m/48'/1'/account'/2'`. The board rederives and matches its slot before writing the policy into HUK-sealed write-once storage. It supports one registered policy per network namespace on a board. Signet has a separate policy slot and binds the published Signet checkpoint in its grants. The other two private cosigner keys are kept by their owners.

- Op 13 with trailing byte `48` exports only the approved BIP48 participant public material. Append `--bip48` to the independent `display-export` command for that scope.
- Op 17 body is `17, 1, network, account:u32, maxFee:u64, ownSlot:u8, threshold:u8, roots`, where network is 1 for regtest and 2 for Signet. The threshold byte equals 2. Each of three ordered roots is fingerprint (4), BIP32 chain code (32) and compressed secp256k1 public key (33). The response is `17 || SHA256(body)`. The independent display reviews all three roots and this exact registration digest.
- Op 19 request is `19, 1, network, account:u32`. The 68-byte response begins with `19 || policyDigest` followed by zero padding. An empty digest means the storage has no registered policy.
- Op 18 uses the same signed transfer header as op 11 and carries the registered policy digest in its first 32 policy bytes. The remaining 58 policy bytes are zero. The complete stripped previous transaction must match the declared input txid, index, program and value. The signer derives all three branch/index keys from the registered policy to validate the native SegWit program and claimed change. It supports one or two distinct confirmed inputs, up to four native SegWit outputs and `SIGHASH_ALL`. The coordinator checks every exact prevout on the selected network before issuing the grant. Each input of a clean draft receives one device signature. A previously imported partial requires one consistent registered counterpart across all inputs. A partial missing from an input, a duplicate prevout, a foreign signer or an already-complete candidate yields no exported signature. The phone independently verifies every added signature over unchanged PSBT maps before offering export.

Set `--bip48-policy` on the local authority to a coordinator JSON object with `account`, `maxFee` (satoshis), zero-based `owner`, and the two `descriptors` in external/internal order. Use a dedicated local Core watch-only wallet with the descriptors imported. The phone normally builds a PSBT from the recipient and human amount, and can import an independent counterpart's partially signed PSBT. The coordinator validates and finalizes the exported PSBT. The coordinator can fund with one or two inputs. The profile caps raw PSBTv2 at 1,780 bytes and the returned partial at 1,998 bytes, within the C1 management frame budget. Independent cosigners should export PSBTv2 without finalizing to support the board's bounded profile.

For an independent signing review, run `wallet_chain_lab.py display` with a separately configured `--bip48-policy`, the matching local Core RPC via `--bitcoin` and its `--bitcoin-cookie`, plus the existing review file, challenge and enrolled display key. The reviewer binds the policy to the grant, rederives each previous output and internal change from public descriptors, and prints both totals, the previous and new output scripts, the exact fee and the rate lower bound. The local Core instance is used to derive script evidence. The issuing coordinator separately verifies that the prevouts are available on the selected network. The final rate may be higher because the actual DER signature lengths become known after signing.

The default P01 tests include an independent Core vector and verify rejection of foreign roots, truncation, duplicate identity and requests already meeting threshold. The real-node workflow produces a fresh grant with the normal local gateway and a P01 portable test runtime before separate Core signing:

```sh
python3 products/p01-device-firmware/test/run_bip48_node.py \
  --oracle products/p01-device-firmware/out/host/libwallet_chain_node_oracle.so \
  --bitcoind "$BITCOIND_32_1"
```

The observed regtest node accepted the two independent partials. Each bit alone left the PSBT incomplete. Core combine/finalize and counterpart-first import produced the same native SegWit witness and confirmed transaction.

## Component inventory

| Component | Pin | License or source |
| --- | --- | --- |
| libsecp256k1 | v0.8.0, `6e2c8bc4ecdc6e71dbe7a368f360d8d453ce435d` | MIT, [vendored inventory](third_party/README.md) |
| NCS and CRACEN SHA/HKDF/Ed25519 provider | v3.4.1, selected by firmware build | See the pinned SDK [manifest](https://github.com/nrfconnect/sdk-nrf/blob/v3.4.1/west.yml) and per-module license files |
| TF-PSA-Crypto RIPEMD-160 translation unit | NCS v3.4.1 checkout `fa92349590a54959ee2efc21b668e5afdb144726` | Apache-2.0 OR GPL-2.0-or-later, [TF-PSA-Crypto repository](https://github.com/Mbed-TLS/TF-PSA-Crypto) |
| Noble public-key verification and hashes | `@noble/curves@1.9.7`, `@noble/hashes@1.8.0` | MIT, pinned in the workspace package lock |
| qrcode-generator | 1.4.4 | MIT, pinned in the workspace package lock |
| Reference tools | Bitcoin Core 32.1, Agave 4.4.1, XRP Ledger 3.3.0 image digest `353d5e016bb93519e9fcac715cdc8c2205b96c4cfe2d1f0f1d22a22f6efaff70` | [Bitcoin Core checksum/signature](https://bitcoincore.org/bin/bitcoin-core-32.1/), [Agave releases](https://github.com/anza-xyz/agave/releases/tag/v4.4.1), [XRPL releases](https://github.com/XRPLF/rippled/releases/tag/3.3.0) |

Manual confirmed-balance refresh is available on the verified receive address. Automatic inbound subscriptions, gap-limit rediscovery, release network profiles and the physical recovery/display/QR scanner matrix remain follow-up work. The laboratory limits and lack of a hardware acceptance run keep this profile out of the official firmware path.

## Commissioned testnet profile

A custom enrolled build can enable C1 transfer wire network 2 with `NU54_COMMISSIONED_WALLET_CHAINS=1`. The device verifies the signed grant's fixed chain anchor and uses the same independent review and physical approval as the laboratory profile. Bitcoin binds the default Signet checkpoint and allows non-RBF native SegWit. Solana Devnet uses its genesis and accepts native SOL. XRPL Testnet binds its network ID and validated checkpoint and accepts native XRP with an optional destination tag. The XRPL Testnet raw format does not encode its network ID. The board uses a separate BIP44 hardened account range 100–199 for commissioned XRP and rejects transactions authored by its legacy XRP key. The displayed account indices stay 0–99. Standard external recovery must inspect BIP44 accounts 100–199 explicitly. Re-discover older XRP addresses by keeping the legacy account range 0–99. Both ranges remain deterministic from the same restored seed. Only the commissioned address should receive test funds. The gateway uses a separate scoped credential and stores the exact reviewed grant before relaying any matching signed bytes.

The approved board build channel, the private online testing environment and public testnet service acceptance are separate deliverables. See [mobile registration and gateway configuration](../p02-user-app/MOBILE-V2.md#commissioned-non-evm-testnet-transfers) and run `python3 test/test_commissioned_chains.py` for the offline gateway vectors. W17 uses the separate Signet write-once policy when explicitly commissioned. Signet W16 uses only the BIP84 watch-only op13 request. Use the independent display with `display-export --signet` to show and bind the exact network, account index and receiving/change scope before the owner confirms on the board.
