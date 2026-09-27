# Consumer Hardware Wallet Comparison (as of September 2026)

Scope note: specs come from vendor pages where fetched, otherwise from reviews and trade press (flagged). Items not confirmed by a source are placed under Gaps or marked "(unverified)". Research budget was limited (~20 tool calls), so several per-device fields (battery mAh, exact chain counts, clear-signing detail) remain unverified for some devices.

## 1. Security architecture (secure element, MCU, firmware openness, SE openness)

### Takeaway
The market has split into three camps: (a) closed-SE-centric designs where signing runs inside a certified SE with closed OS (Ledger, Tangem); (b) "open firmware on general MCU + SE as key vault" designs (Trezor Safe series, BitBox02/Nova, Coldcard, Keystone, OneKey, Passport); (c) Trezor Safe 7's new dual-SE with an open-design SE (TROPIC01) plus a certified Infineon Optiga. EAL6+ is now the norm for new devices; air-gapped devices (Keystone, Ellipal, SafePal S1) tend to use EAL5+ or older Microchip/Maxim parts. Coldcard's 2026 RNG vulnerability shows open firmware does not by itself prevent critical bugs.

### Cited Findings

Ledger
- Nano Gen5 (2025): ST33K1M5 secure element, CC EAL6+; 2.76" E-Ink touchscreen 400x300; USB-C, Bluetooth 5.2, NFC; $179 — [Ledger Academy: which Nano](https://www.ledger.com/academy/the-world-of-nano-which-ones-for-you); [Finder review](https://www.finder.com/cryptocurrency/wallets/ledger-nano-gen5-review)
- Nano S Plus (2022): ST33 SE, EAL6+; USB-C only, no battery; ~$79 — [Ledger Academy](https://www.ledger.com/academy/the-world-of-nano-which-ones-for-you)
- Nano X (2019): ST33 SE described as EAL5+; Bluetooth + USB-C; 100 mAh battery; ~$149 — [Ledger Academy](https://www.ledger.com/academy/the-world-of-nano-which-ones-for-you) (exact part number, commonly cited as ST33J2M0, is unverified)
- Nano S (2016): discontinued, replaced by Nano S Plus — [Ledger Academy](https://www.ledger.com/academy/the-world-of-nano-which-ones-for-you)
- Flex: launched 2024-07-26, $249, 2.84" E-Ink touchscreen, CC EAL6+ SE, screen driven directly by the SE, USB-C + Bluetooth + NFC — [Coin Bureau Stax vs Flex](https://coinbureau.com/analysis/ledger-stax-vs-ledger-flex); [Ledger Academy Flex](https://www.ledger.com/academy/topics/ledgersolutions/ledger-flex-ease-of-use-free-from-compromise)
- Stax: $399, 3.7" curved E-Ink, Qi wireless charging, same ST33K1M5 EAL6+ chip as Flex — [Coin Bureau](https://coinbureau.com/analysis/ledger-stax-vs-ledger-flex); [CryoVault Stax review](https://cryovaultsolutions.com/blog/ledger-stax-review-2026)
- Openness: Ledger's BOLOS OS on the SE is closed source; Ledger cites SE vendor NDA and CC certification as assurance — [Keycard blog comparison](https://docs.keycard.tech/en/blog/open-source-ledger-alternatives-compared) (secondary source)

Trezor
- Safe 7 (late 2025): dual SE = TROPIC01 (Tropic Square, open/auditable design, "Truly Open Integrated Circuit") + Infineon Optiga EAL6+ (NDA-free); MCU STM32U5G Cortex-M33 160 MHz; 2.5" color touchscreen 520x380, Gorilla Glass 3; USB-C + Bluetooth 5.0+; LiFePO4 330 mAh battery with Qi2 wireless charging; IP54; $249; post-quantum signature (SLH-DSA-128) for firmware updates / device auth / boot — [Trezor Safe 7 product page](https://trezor.io/trezor-safe-7); [Bitcoin.com News](https://news.bitcoin.com/new-trezor-safe-7-boasts-quantum-resistant-design-and-dual-secure-elements/); [CryptoSlate review](https://cryptoslate.com/crypto-wallets/trezor-safe-7-review/)
- TROPIC01 is itself not stated as CC-certified on the product page; the EAL6+ claim attaches to the Optiga — [Trezor Safe 7](https://trezor.io/trezor-safe-7)
- Safe 3: Optiga Trust M V3, EAL6+ — [CryptoSlate Safe 3 vs Safe 5](https://cryptoslate.com/crypto-wallets/trezor-safe-3-vs-safe-5/); price $59 (US offer checked Sept 12) vs $79 elsewhere — same source (prices conflict)
- Safe 5: price $129 in the same check — [CryptoSlate](https://cryptoslate.com/crypto-wallets/trezor-safe-3-vs-safe-5/); SE believed to be Optiga Trust M EAL6+ (unverified in this session)
- Model T: no longer sold; 180 MHz Cortex-M4, 1.54" color touchscreen 240x240, no secure element listed; software updates guaranteed until at least 2031, critical security patches until at least 2036 — [Trezor Model T page](https://trezor.io/trezor-model-t)
- Model T and Model One: official e-shop sales ended 2026-01-08; may remain at resellers — [CryptoSlate](https://cryptoslate.com/crypto-wallets/trezor-safe-3-vs-safe-5/) (secondary; Trezor page confirms Model T end but gives no date)
- Trezor firmware is fully open source with reproducible builds — [Keycard blog](https://docs.keycard.tech/en/blog/open-source-ledger-alternatives-compared)

Keystone 3 Pro
- Three SEs per Keystone's own blog: Microchip ATECC608B (seed), Maxim DS28S60 (recovery storage), Maxim MAX32520 (fingerprint) — [Keystone blog](https://blog.keyst.one/inside-the-vault-how-keystone-3-pro-secures-your-crypto-with-triple-se-chips); contradicted by reviews describing "three Infineon SEs, two CC EAL5+" — [hardware-wallets.net](https://www.hardware-wallets.net/keystone-3-pro-review/). Prefer the vendor blog.
- 4" touchscreen, fingerprint, QR-only air-gap (no BT/WiFi/NFC/USB data), firmware open source (MIT) and reproducible; ~$149 (another listing $129) — [Snout0x review](https://snout0x.com/keystone-3-pro-review/); [Keystone shop](https://shop.keyst.one/products/keystone-3-pro)

Tangem (card / ring)
- Samsung S3D350A SE, CC EAL6+ in all models — [Unfinished Man](https://www.unfinishedman.com/how-safe-is-the-tangem-wallet/) (secondary); EAL6+ confirmed on [Tangem pricing](https://tangem.com/en/pricing/)
- Ring: EAL6+ chip, zirconia ceramic shell, waterproof — [Tangem ring comparison](https://tangem.com/en/learning-hub/post/tangem-ring-vs-tangem-card/)
- Keys generated by on-chip TRNG (seedless) or optionally seed phrase — [Tangem pricing](https://tangem.com/en/pricing/)

GridPlus Lattice1
- Desktop device, 5" touchscreen, Wi-Fi + Ethernet; secure enclave with PUF, CLDS tamper-detection mesh; removable PIN-protected SafeCards hold keys — [GridPlus docs](https://docs.gridplus.io/lattice1/security-features); [Coin Bureau review](https://coinbureau.com/review/gridplus-lattice1-review)
- Price ~$397, starter ~$450 with SafeCards — [Blocklr guide](https://blocklr.com/guides/best-hardware-wallets-2026/) (secondary)

Coldcard (Bitcoin-only, Coinkite)
- Mk4: dual SE, Microchip ATECC608A + Maxim DS28C36B; NFC; microSD air-gap; price $177.94 (official store, per review) or $149.99 (conflicting) — [Coldcard Mk4](https://coldcard.com/mk4); [bitcoin.diy review](https://www.bitcoin.diy/wallets/coldcard-mk4-review)
- Q: same dual-SE model as Mk4; QWERTY keyboard, 320x240 LCD, QR scanner, dual microSD, 3xAAA battery; ~$249–260 — [Coldcard Q docs](https://coldcard.com/docs/coldcard-q/)
- Mk5 (launched 2026-03-10): 1.54" Gorilla Glass display, redesigned keypad, dual SE retained, NFC PushTX, $167 — [Coinkite blog](https://blog.coinkite.com/coldcard-mk5-launch/); [Bitcoin Magazine](https://bitcoinmagazine.com/business/coinkite-launches-coldcard-mk5-major-ux-upgrades-to-flagship-bitcoin-hardware-wallet)
- 2026 RNG vulnerability: firmware since 4.0.0 (March 2021) fell back to MicroPython's deterministic Yasmarang PRNG instead of STM32 hardware RNG; Mk4/Q/Mk5 seeds ~72 bits instead of 128, Mk3 worse; fixed in Mk4/Mk5 5.6.0+, Q 1.5.0Q+; 50+ dice rolls mitigate — [Coldcard security status](https://coldcard.com/security/status); [Block Engineering](https://engineering.block.xyz/blog/predictable-rng-fallback-and-32-bit-reseed-in-coldcard-firmware)
- Reported exploitation: 594 BTC drained in a 25-minute sweep; ~$100M hacked funds linked to the bug — [CoinDesk 2026-07-31](https://www.coindesk.com/tech/2026/07/31/major-bitcoin-wallet-flaw-drains-594-btc-in-25-minute-sweep); [CoinDesk 2026-08-17](https://www.coindesk.com/tech/2026/08/17/how-a-bug-in-coldcard-s-code-went-unnoticed-for-years-leading-to-usd100-million-in-hacked-funds)
- Firmware open source with reproducible Docker builds; full hardware schematics not released — [Keycard blog](https://docs.keycard.tech/en/blog/open-source-ledger-alternatives-compared)

BitBox02 / Nova (Shift Crypto, Swiss)
- Nova: Infineon OPTIGA Trust M V3 EAL6+, NDA-free; MCU ATSAMD51J20A Cortex-M4F 120 MHz; 128x64 OLED; capacitive touch sensors; USB-C + BLE (iPhone support); 13 g; €175 — [BitBox Nova blog](https://blog.bitbox.swiss/en/introducing-bitbox02-nova/); [BitBox Nova page](https://bitbox.swiss/bitbox02/nova/)
- BitBox02 firmware, schematics and BitBoxApp open source; reproducible builds verified by WalletScrutiny — [Keycard blog](https://docs.keycard.tech/en/blog/open-source-ledger-alternatives-compared)

SafePal
- S1 $49.99, X1 $69.99, S1 Pro $89.99; X1 SE CC EAL5+, S1/S1 Pro EAL6+; S1 1.8" mono 128x64, X1/S1 Pro 1.3" color 320x320; S1/S1 Pro QR-only, X1 QR + Bluetooth — [SafePal store](https://www.safepal.com/en/store); [practicalcrypto comparison](https://practicalcrypto.net/en/wallets/safepal-comparison.html)

Foundation Passport Prime
- ATECC608C SE + Microchip SAMA5D2 processor; seed split between SAMA5D2 and SE, XORed with PIN hash; 3.5" color touchscreen; NFC; "QuantumLink" post-quantum encrypted Bluetooth; 50 GB encrypted storage; FIDO keys/TOTP; $349 — [Foundation product page](https://foundation.xyz/products/passport-prime); [WalletInsights](https://walletinsights.io/en/hardware-wallets/foundation-passport-prime/); WalletScrutiny entry exists — [WalletScrutiny Passport Prime](https://walletscrutiny.com/hardware/passportprime/)

Ellipal Titan 2.0
- CC EAL5+ SE; QR-only air-gap (no WiFi/BT/USB data); 4" touchscreen; anti-disassembly wipe; $149 (street ~$135); reviewers describe it as "partly closed" — [Ellipal product page](https://www.ellipal.com/products/ellipal-titan); [CryptoSlate review](https://cryptoslate.com/crypto-wallets/ellipal-titan-2-0-wallet/)

OneKey
- Classic 1S: EAL6+ SE, Bluetooth + USB-C, OLED, $99; Classic 1S Pure (battery-free) $79; open-source firmware with verifiable builds — [OneKey Classic 1S](https://onekey.so/products/onekey-classic-1s-hardware-wallet/); [OneKey shop Pure](https://shop.onekey.so/products/onekey-classic-1s-pure-crypto-hardware-wallet-battery-free)
- Pro: 4x EAL6+ SEs, air-gap QR, fingerprint (per review) — [OneKey Pro page](https://onekey.so/products/onekey-pro/); [CryoVault review](https://cryovaultsolutions.com/blog/onekey-pro-review-2026)

2025–2026 newcomers
- Trezor Safe 7 (late 2025) and Ledger Nano Gen5 (2025) — see above
- Coldcard Mk5 (2026-03-10) — see above
- Bitkey (2026) by Block: announced at Bitcoin2026 (Apr 27–29, 2026); adds OLED touchscreen to fingerprint + NFC design; $250 (original 2024 model $150, no screen); 2-of-3 multisig model with app + server key (unverified in this session) — [WalletScrutiny Bitkey 2026](https://walletscrutiny.com/hardware/bitkey2026/); [Block announcement](https://block.xyz/inside/block-launches-bitkey-wallet-with-screen-automatic-bitcoin-earning-on-cash-app-and-proof-of-reserves)

### Inferences
- Trezor Safe 7 is the only mainstream consumer wallet found with an open-design SE; its security claim still leans on a closed certified SE (Optiga) as a second layer, so "fully open" is not accurate.
- Ledger's model (keys + UI driver in the SE, closed OS) remains the most payment-card-like architecture, which matters if a product aims at EMV-like certification.
- The Coldcard incident is a strong counterexample to "open source = safe": the bug was public for ~5 years. Reproducible builds verify binary provenance, not correctness.

### Gaps
- Safe 5 SE model/cert and Safe 5 MCU not confirmed from Trezor page in this session.
- Nano X exact SE part number not confirmed from a primary source.
- Keystone SE vendor conflict (Microchip/Maxim per vendor vs Infineon per reviews) unresolved; vendor source preferred.
- OneKey Pro, Ellipal, SafePal MCU models not found.
- Tangem firmware openness (closed, audited per Tangem) not verified from a primary source.

## 2. Connectivity, display, input, and backup features

### Takeaway
USB-C + Bluetooth is standard for mainstream devices; NFC has spread to Ledger (Gen5, Flex, Stax), Coldcard (Mk4/Mk5), Passport Prime, Bitkey and Tangem (NFC-only). Air-gapped QR devices (Keystone, Ellipal, SafePal S1) avoid radios entirely. Backup schemes diverge: BIP39, SLIP-39 multi-share (Trezor), Ledger Recover (paid sharded custody), and card-cloning backups (Tangem, Ledger NFC Recovery Key on Gen5).

### Cited Findings

| Device | USB | BT | NFC | QR | microSD | Display / input | Source |
|---|---|---|---|---|---|---|---|
| Ledger Nano S Plus | USB-C | No | No | No | No | small OLED, buttons | [Ledger Academy](https://www.ledger.com/academy/the-world-of-nano-which-ones-for-you) |
| Ledger Nano X | USB-C | Yes | No | No | No | small OLED, buttons | same |
| Ledger Nano Gen5 | USB-C | BT 5.2 | Yes | No | No | 2.76" E-Ink touch | same |
| Ledger Flex | USB-C | Yes | Yes | No | No | 2.84" E-Ink touch | [Coin Bureau](https://coinbureau.com/analysis/ledger-stax-vs-ledger-flex) |
| Ledger Stax | USB-C | Yes | Yes (unverified) | No | No | 3.7" curved E-Ink touch, Qi | same |
| Trezor Safe 7 | USB-C | BT 5.0+ | Not listed | No | No | 2.5" color touch 520x380 | [Trezor](https://trezor.io/trezor-safe-7) |
| Trezor Model T (disc.) | USB | No | No | No | microSD (unverified) | 1.54" color touch | [Trezor Model T](https://trezor.io/trezor-model-t) |
| Keystone 3 Pro | power only | No | No | Yes | microSD (unverified) | 4" touch + fingerprint | [Snout0x](https://snout0x.com/keystone-3-pro-review/) |
| Tangem card/ring | No | No | Yes (only) | No | No | none (phone is UI) | [Tangem pricing](https://tangem.com/en/pricing/) |
| GridPlus Lattice1 | USB (unverified) | No | No | No | No | 5" touch, Wi-Fi/Ethernet | [Coin Bureau](https://coinbureau.com/review/gridplus-lattice1-review) |
| Coldcard Mk4 | USB-C | No | Yes | No | Yes | small screen, keypad | [Coldcard Mk4](https://coldcard.com/mk4) |
| Coldcard Q | USB-C | No | Yes (unverified) | Yes (scanner) | 2x | 320x240 LCD, QWERTY | [Coldcard Q docs](https://coldcard.com/docs/coldcard-q/) |
| Coldcard Mk5 | USB-C | No | Yes (PushTX) | No | Yes | 1.54" display, keypad | [Coinkite blog](https://blog.coinkite.com/coldcard-mk5-launch/) |
| BitBox02 Nova | USB-C | BLE | No | No | No (unverified) | 128x64 OLED, touch sensors | [BitBox Nova](https://blog.bitbox.swiss/en/introducing-bitbox02-nova/) |
| SafePal S1 / S1 Pro | charge only (unverified) | No | No | Yes | No | 1.8" mono / 1.3" color | [SafePal store](https://www.safepal.com/en/store) |
| SafePal X1 | (unverified) | Yes | No | Yes | No | 1.3" color | same |
| Passport Prime | USB-C | Yes (PQ encrypted) | Yes | Yes (unverified) | (unverified) | 3.5" color touch | [Foundation](https://foundation.xyz/products/passport-prime) |
| Ellipal Titan 2.0 | No data | No | No | Yes | (unverified) | 4" touch | [Ellipal](https://www.ellipal.com/products/ellipal-titan) |
| OneKey Classic 1S | USB-C | Yes | No | No | No | OLED | [OneKey](https://onekey.so/products/onekey-classic-1s-hardware-wallet/) |
| Bitkey (2026) | (unverified) | No | Yes | No | No | OLED touch + fingerprint | [WalletScrutiny](https://walletscrutiny.com/hardware/bitkey2026/) |

- Trezor Safe 7 supports 12/20/24-word backups and "Advanced Multi-share Backup" (SLIP-39 family), passphrase, PIN up to 50 digits, FIDO2 — [Trezor Safe 7](https://trezor.io/trezor-safe-7)
- Ledger Nano X / S Plus / Gen5 compatible with paid opt-in Ledger Recover; Gen5 ships with an NFC Recovery Key and supports passkeys; S Plus small screen not listed as supporting full clear signing, Gen5 supports full human-readable transaction details — [Ledger Academy](https://www.ledger.com/academy/the-world-of-nano-which-ones-for-you)
- Tangem: 2- or 3-card sets are clones; with 3 cards, losing one still allows access; seed phrase optional — [Tangem pricing](https://tangem.com/en/pricing/)
- Passport Prime: multisig cosigning, Taproot, PSBT, SeedQR, passphrase — [WalletInsights](https://walletinsights.io/en/hardware-wallets/foundation-passport-prime/)
- Coldcard: duress wallets, brick-me PINs, dice entropy, BIP-85 — [Coinkite Mk5 blog](https://blog.coinkite.com/coldcard-mk5-launch/)

### Inferences
- For a payment-oriented device, NFC tap UX is proven at scale by Tangem (no battery, no screen) but Tangem relies on the phone as display, which weakens WYSIWYS (what you see is what you sign). Ledger Gen5/Flex and Bitkey 2026 combine NFC with an on-device secure screen — the closest existing template for a "tap-to-sign" payment signer.

### Gaps
- Per-device SLIP-39 support beyond Trezor, multisig support per device, and clear-signing coverage per chain were not systematically verified.
- Battery capacities for most devices other than Nano X (100 mAh) and Safe 7 (330 mAh) not found.

## 3. Supported chains, companion apps, price

### Takeaway
Price bands in 2026: entry $50–$100 (SafePal, OneKey, Tangem, Nano S Plus, Trezor Safe 3), mid $130–$180 (Safe 5, Nano X, Nano Gen5, Coldcard Mk4/Mk5, Keystone, Ellipal, BitBox Nova), premium $249–$400 (Flex, Safe 7, Coldcard Q, Bitkey 2026, Passport Prime, Stax, Lattice1). Bitcoin-only: Coldcard, Passport Prime, Bitkey; others multichain.

### Cited Findings
- Ledger devices: 15,000+ assets via Ledger Wallet app — [Ledger Academy](https://www.ledger.com/academy/the-world-of-nano-which-ones-for-you)
- Trezor Safe 7: thousands of coins, Trezor Suite, 30+ third-party wallets — [Trezor](https://trezor.io/trezor-safe-7); Bitcoin-only edition exists — [Trezor Safe 7 BTC-only](https://trezor.io/trezor-safe-7-bitcoin-only)
- Ellipal: 10,000+ tokens across 50+ chains — [CryptoSlate](https://cryptoslate.com/crypto-wallets/ellipal-titan-2-0-wallet/)
- SafePal: 200+ blockchains — [practicalcrypto](https://practicalcrypto.net/en/wallets/safepal-comparison.html)
- OneKey Classic 1S: 30,000+ currencies — [OneKey](https://onekey.so/products/onekey-classic-1s-hardware-wallet/)
- Keystone: Bitcoin-only firmware also offered — [Nasdaq](https://www.nasdaq.com/articles/crypto-wallet-maker-keystone-debuts-bitcoin-only-firmware-for-flagship-device)
- Tangem: $59.90 (2 cards), $69.90 (3 cards), Ring $160 incl. 2 cards — [Tangem pricing](https://tangem.com/en/pricing/)

### Inferences
- Prices shown are list prices from mixed dates; discounts are frequent (e.g., Safe 3 $59 vs $79).

### Gaps
- Companion app names for Keystone, SafePal, Ellipal, OneKey not verified in this session.

## 4. Payment-oriented features

### Takeaway
No major hardware wallet performs point-of-sale payment itself. Payment features are delivered by pairing the wallet with a custodial or semi-custodial card program (Ledger CL Card via Baanx; Tangem Pay virtual Visa). Tangem's NFC card form factor is the closest physical analogue to a payment card, but it is a signer, not an EMV payment instrument.

### Cited Findings
- Ledger + Baanx CL Card: Visa debit card funded from Ledger device via Ledger Wallet app (BTC, ETH, SOL, stablecoins); 1% cashback in BTC/USDC/USDT; available in most EU states and UK, and most US states except NY and VT — [Ledger CL Card page](https://shop.ledger.com/pages/cl-card-crypto-card); [Ledger blog US launch](https://www.ledger.com/blog-the-cl-card-comes-to-the-us); [TheStreet](https://www.thestreet.com/crypto/innovation/ledger-and-baanx-launch-bitcoin-rewards-debit-card-in-the-us)
- Tangem Pay: announced Nov 2025; virtual Visa in Apple Pay/Google Pay spending USDC on Polygon; rolling out US, LatAm, APAC, UK/EU in early 2026 under MiCA — [Webopedia Tangem review](https://www.webopedia.com/crypto/wallets/tangem-wallet-review/) (secondary; Tangem pricing page confirms Tangem Pay exists as separate product)
- Bitkey 2026 launch tied to Cash App automatic bitcoin earning and Proof of Reserves — [Block announcement](https://block.xyz/inside/block-launches-bitkey-wallet-with-screen-automatic-bitcoin-earning-on-cash-app-and-proof-of-reserves)
- Coldcard NFC PushTX lets a tap sign and broadcast via a phone — [Bitcoin Magazine](https://bitcoinmagazine.com/business/coinkite-launches-coldcard-mk5-major-ux-upgrades-to-flagship-bitcoin-hardware-wallet)

### Inferences
- The card programs move funds from self-custody into a card-issuer account before spend, so "self-custody payment" claims are partial: the spend step is custodial or smart-contract-controlled. A true "hardware wallet as payment instrument" niche appears open.

### Gaps
- Whether Tangem Pay's USDC stays in a user-controlled smart account vs issuer custody could not be verified from a primary source.
- No evidence found of any hardware wallet with EMV contactless merchant acceptance.

## 5. Market share / units sold

### Takeaway
Only Ledger publishes volume-scale figures (~7–8M devices sold, >$100M 2025 revenue, IPO plans). Trezor and others publish little.

### Cited Findings
- Ledger: over 7M devices sold, later communications ~8M; 2025 revenue >$100M (record); hardware sales +~31% in 2025 vs 2024; targeting 2026 IPO at >$4B valuation — [Yellow.com](https://yellow.com/news/ledger-4b-nyse-ipo-plans-2026); [MarketScreener](https://www.marketscreener.com/news/ledger-targets-ipo-of-over-4bn-in-2026-ce7e5bdbd889f321); [Bankless](https://www.bankless.com/read/news/ledger-eyes-ipo-raise-amid-record-year)
- Ledger's own 10-year device history — [Ledger Academy](https://www.ledger.com/academy/topics/ledgersolutions/ledger-hardware-devices-explained-the-10-year-evolution)

### Inferences
- Ledger likely holds the plurality of the market; Trezor second (unverified; aggregator sites like CoinLaw give numbers of uncertain provenance and were not used).

### Gaps
- No reliable primary figures for Trezor, Tangem, SafePal, Keystone, OneKey units sold.
- The "31% growth" figure is from secondary press and its original source was not traced.
