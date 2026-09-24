# Hardware Wallet Incidents, Attack Classes, Structural Limits, and nRF54L15 as a Wallet Platform

Research date: 2026-09-23. Sources were gathered through web search and page fetches. Items marked **[unverified]** come only from a search snippet or a secondary source that was not cross-checked. Nordic documentation pages (academy.nordicsemi.com, nrfconnectdocs.nordicsemi.com) returned HTTP 403 when fetched, so the nRF54L details below come from search snippets of those pages and from third-party summaries.

## A1. Ledger incidents (2020-2026)

### Takeaway
Most Ledger losses did not come from breaking the secure element. They came from the things around the device: a leaked customer database that fed years of phishing, fake-device mailings and physical threats; a compromised npm dependency (Connect Kit) that made users sign a different transaction from the one shown; and a co-founder kidnapping. Blind signing on the device turned host-side compromises into real losses.

### Cited Findings
- **Customer database leak (June 2020, dumped publicly December 2020).** Attackers took about 1.1M email addresses and 272,000 records with full name, phone and home address from Ledger's e-commerce and marketing database. Forensics found that a third party's API key had been abused. — [Ryder summary](https://ryder.id/blogs/post/every-ledger-hack-since-2020-and-what-each-changed); [Ledger leadership message](https://www.ledger.com/addressing-the-july-2020-e-commerce-and-marketing-data-breach); [HIBP](https://haveibeenpwned.com/Breach/Ledger); [Bitdefender (270k addresses published)](https://www.bitdefender.com/en-us/blog/hotforsecurity/hacker-publishes-stolen-email-and-mailing-addresses-of-270000-ledger-cryptocurrency-wallet-users)
- The leak led to phishing emails and SMS posing as Ledger support, physical letters, and extortion threats demanding $700-$1,000 in BTC. — [Ryder](https://ryder.id/blogs/post/every-ledger-hack-since-2020-and-what-each-changed)
- **Tampered Ledger devices sent by mail (May-June 2021).** Leaked addresses received fake Nano devices in shrink-wrapped packaging. A flash drive was soldered inside, and a letter told the recipient to run a malicious file and enter their 24-word phrase. — [CoinDesk, 2021-06-17](https://www.coindesk.com/tech/2021/06/17/scammers-are-sending-ledger-users-fake-hardware-wallets); [BleepingComputer](https://www.bleepingcomputer.com/news/cryptocurrency/criminals-are-mailing-altered-ledger-devices-to-steal-cryptocurrency/)
- **Ledger Recover (announced 2023-05-16, paused 2023-05-23).** A paid service that splits an encrypted copy of the seed across three custodians (Ledger, Coincover, EscrowTech), unlocked with government-ID verification. The backlash came from the fact that shipped firmware could export the seed through an approved code path, which contradicted the "keys never leave the SE" message. Ledger paused the rollout and promised to open-source the code. — [CoinDesk 2023-05-16](https://www.coindesk.com/tech/2023/05/16/ledger-bats-back-criticism-of-new-wallet-recovery-service); [CoinDesk 2023-05-24](https://www.coindesk.com/tech/2023/05/24/ledger-recover-fiasco-exposes-gap-between-blockchain-ideals-and-technical-reality); [Blockworks](https://blockworks.com/news/ledger-delays-recover-service)
- **Ledger Connect Kit supply-chain attack (2023-12-14).** A former employee was phished and the attacker hijacked their NPMJS session token, which bypassed 2FA. The attacker published malicious versions 1.1.5, 1.1.6 and 1.1.7 containing a wallet drainer that affected every dApp loading the library. Ledger shipped a fix within 40 minutes of learning of it. The malicious file was live for about 5 hours, and funds were drained for less than 2 hours. — [Ledger incident report](https://www.ledger.com/blog/security-incident-report); [SlowMist](https://slowmist.medium.com/supply-chain-attack-on-ledger-connect-kit-analyzing-the-impact-and-preventive-measures-1005e39422fd)
- Reported losses vary: CoinDesk put them at about $484K across a few hundred wallets, and later reports say at least $600K. Ledger committed to reimbursement. — [CoinDesk 2023-12-14](https://www.coindesk.com/business/2023/12/14/ledger-exploit-drained-484k-upended-defi-former-staffer-linked-to-malicious-code); [SlowMist](https://slowmist.medium.com/supply-chain-attack-on-ledger-connect-kit-analyzing-the-impact-and-preventive-measures-1005e39422fd)
- **Co-founder David Balland kidnapped (January 2025).** Balland and his wife were abducted from their home in central France for a €10M ransom. His finger was cut off, and a video of it was sent to co-founder Eric Larchevêque. The GIGN tactical unit freed both after about 48 hours, and at least 10 suspects were arrested. — [DL News](https://www.dlnews.com/articles/regulation/ledger-cofounder-david-balland-and-wife-kidnapped-in-france/); [The Block](https://www.theblock.co/post/336573/ledger-co-founder-david-balland-released-after-kidnapping); [CoinDesk 2025-01-24](https://www.coindesk.com/policy/2025/01/24/ledger-co-founder-s-kidnapping-sheds-light-on-soaring-crypto-robberies)
- **Physical phishing letters (late April 2025 onward).** Letters with Ledger's logo, business address and a reference number asked recipients to scan a QR code for a "critical security update" and enter their recovery phrase. Ledger confirmed it was a scam. Months later the letters were still arriving, and Ledger lists physical mail as the top scam threat. — [The Block 2025-04-30](https://www.theblock.co/news/ecosystems/2025-04-30-ledger-confirms-physical-scam-letters-requesting-seed-phrase-352479); [Cointelegraph](https://cointelegraph.com/news/ledger-scammers-send-letters-steal-recovery-seed-phrases); [Ledger support](https://support.ledger.com/article/scams-targeting-crypto-holders)
- **[unverified]** A counterfeit Nano S Plus sold on an online marketplace reportedly carried extra Wi-Fi and Bluetooth antennas. The only source is an aggregator snippet (mexc.com), so no primary source was confirmed. — [search snippet via mexc](https://www.mexc.com/news/732078)
- **[unverified]** A September 2026 report says Ledger faces a $500M class action over a $1.9M wallet theft said to be linked to a breach. The article was not fetched. — [CryptoTimes 2026-09-03](https://www.cryptotimes.io/2026/09/03/ledger-hit-with-500m-class-action-over-1-9m-wallet-theft/)

### Inferences
- The largest Ledger-linked harms came from customer PII (personal data) and from host software supply chains, not from the chip. A project that ships devices must treat order and shipping data as security-critical assets and must not rely on any single npm or JS dependency to display transaction data.
- Recover shows that a firmware update path able to export the seed breaks user trust even when the export is opt-in. The ability of firmware to export secrets is a policy decision that needs to be public.

### Gaps
- No primary source was found for specific Ledger Nano X Bluetooth vulnerabilities. Ledger's position is that BLE only carries public or already-signed data, but no document stating this was fetched.

---

## A2. Trezor incidents and physical extraction attacks

### Takeaway
Trezor devices without a secure element, which hold the seed in STM32 flash, fell to voltage glitching repeatedly: wallet.fail in 2018, Kraken in 2020, Ledger Donjon in 2020, and Unciphered in 2023. The flaw sits in the MCU silicon and cannot be patched by software. The mitigations are a strong passphrase, or moving secrets into an SE (Safe 3/5 with Infineon OPTIGA Trust M; Safe 7 with TROPIC01).

### Cited Findings
- **wallet.fail (35C3, December 2018).** Nedospasov, Datko and Roth showed attacks on Trezor One, Ledger Nano S and Ledger Blue. The attacks were: Trezor One key extraction by glitching (fails if the user set a passphrase); custom firmware on the Nano S used together with host malware; and TEMPEST-style RF leakage from the Ledger Blue touchscreen. Ledger called the attacks impractical. — [Bitcoin Magazine](https://bitcoinmagazine.com/culture/security-researchers-reveal-wallet-vulnerabilities-stage-35c3); [Threatpost](https://threatpost.com/cryptocurrency-wallet-hacks-spark-dustup/140445/)
- **Kraken Security Labs (2020-01-31).** Voltage glitching during boot let the STM32 factory bootloader read the protected flash 256 bytes per glitch, which exposed the encrypted seed on Trezor One and Model T. The attack needs about 15 minutes of physical access. The PIN (1-9 digits) is trivial to brute-force, and the flaw is inherent to the MCU, so it cannot be fixed. The recommended mitigation is a BIP39 passphrase. — [Kraken blog](https://blog.kraken.com/product/security/kraken-identifies-critical-flaw-in-trezor-hardware-wallets); [The Block](https://www.theblock.co/news/markets/2020-01-31-kraken-security-labs-hackers-can-exploit-trezor-hardware-wallets-with-only-15-minutes-of-physical-access-to-the-device-54631)
- **Ledger Donjon, "Unfixable Seed Extraction on Trezor" (2020).** A practical and reliable attack on Trezor's STM32. — [Ledger blog](https://www.ledger.com/blog/unfixable-key-extraction-attack-on-trezor)
- **Unciphered (May 2023).** Extracted the seed from a Trezor T in the attacker's possession using an in-house exploit plus GPU PIN cracking. Trezor said this resembled the RDP-downgrade flaw Kraken reported. — [The Block](https://www.theblock.co/post/232085/cybersecurity-firm-claims-it-hacked-private-key-from-a-trezor-t-hardware-wallet)
- **Ledger Donjon on Trezor Safe 3 (Trezor advisory dated 2024-11-12; public March 2025).** Donjon reused a known voltage-glitching attack on the Safe 3 MCU to bypass the supply-chain countermeasures, meaning the firmware authenticity check. It could not extract the PIN or keys, which stay protected by the SE. The attack matters mainly for devices bought through third parties. Trezor Safe 5 is not affected because it uses a newer MCU. — [Trezor advisory](https://trezor.io/vulnerability/donjon-s-trezor-safe-3-evaluation); [The Block 2025-03-13](https://www.theblock.co/news/regulation/2025-03-13-trezor-discloses-vulnerability-safe-3-crypto-wallet-rival-ledger-346018)
- **Support-form phishing (June 2025).** Anyone could open a Trezor support ticket with any email address and subject. The auto-reply came from the real help@trezor.io and carried the attacker's subject line (for example "[URGENT]: vault.trezor.guide …"), which led to a page that phished the seed. Trezor said there was no email breach and that it was changing its processes. — [BleepingComputer](https://www.bleepingcomputer.com/news/security/trezors-support-platform-abused-in-crypto-theft-phishing-attacks/); [The Block 2025-06-23](https://www.theblock.co/news/regulation/2025-06-23-trezor-phishing-scam-359172)
- **Customer data exposure (2026).** Data on 13,689 Trezor customers was exposed through the shipping provider ShipMonk. — [FinanceFeeds](https://financefeeds.com/bitbox-wallet-flaws-emerge-after-coldcard-exploit-led-to-112m-in-losses/)
- Current Trezor secure elements: Safe 5 uses Infineon OPTIGA Trust M (EAL6+), and Safe 7 uses TROPIC01. — [Trezor KB](https://trezor.io/learn/security-privacy/how-trezor-keeps-you-safe/secure-elements-in-trezor-safe-devices); [Trezor Safe 7 / TROPIC01](https://trezor.io/guides/trezor-devices/trezor-safe-7/what-is-the-tropic-01-chip)

### Inferences
- A wallet built on a general-purpose MCU like the nRF54L15 is in the same threat class as Trezor One/T: a physical attacker with glitching equipment. That class has been broken repeatedly, and the only software mitigation is a secret that is not on the device (a passphrase, or PIN-derived encryption that the device cannot brute-force alone because an SE enforces the retry limit).

### Gaps
- No specific fake-Trezor device campaign with a primary source was fetched.

---

## A3. Other vendors (Coldcard, BitBox, Keystone, SafePal) and the Dark Skippy attack

### Takeaway
The largest hardware-wallet loss by value was not a physical attack. It was a Coldcard entropy bug: seeds were generated from a PRNG seeded only by the chip UID and timers from March 2021 until 2026, and remotely brute-forced seeds were drained for more than $88M-$112M in July-August 2026. Dark Skippy (August 2024) showed that malicious firmware can leak a seed in two signatures. Anti-exfil (anti-klepto) is the known countermeasure.

### Cited Findings
- **Coldcard RNG flaw (drained 2026-07-30; emergency firmware 2026-07-31).** A March 2021 build set `MICROPY_HW_ENABLE_RNG` to 0, so seed generation fell back to MicroPython's Yasmarang PRNG, seeded from the chip UID and timer registers. Affected: Mk2/Mk3 4.0.0-4.1.9, Mk4/Mk5 before 5.6.0, Q before 1.5.0Q, and edge builds before 6.6.0X/6.6.0QX. The first wave took 1,082.65 BTC (about $70.2M) from 1,196 addresses in 41 minutes. An update put it at 1,367.05 BTC (about $88.6M) from 4,585 addresses. The patch does not repair seeds already generated; users must make new seeds and move funds. — [The Hacker News, Aug 2026](https://thehackernews.com/2026/08/coldcard-hardware-wallet-flaw-linked-to.html)
- A higher, conflicting figure: Galaxy Research, as cited by FinanceFeeds, reports more than $112M, about 1,778.6 BTC, from more than 8,600 addresses. — [FinanceFeeds](https://financefeeds.com/bitbox-wallet-flaws-emerge-after-coldcard-exploit-led-to-112m-in-losses/)
- **BitBox02 / BitBox02 Nova (2026).** Two flaws were patched. The first is memory corruption before setup: a malicious host could execute code and potentially install malicious firmware. The second is a Silent Payments bug that could send BTC to unintended addresses. BitBox found no evidence of exploitation. BitBox also states it is not affected by the Coldcard RNG flaw. — [FinanceFeeds](https://financefeeds.com/bitbox-wallet-flaws-emerge-after-coldcard-exploit-led-to-112m-in-losses/); [BitBox blog](https://blog.bitbox.swiss/en/bitbox-is-not-affected-by-the-coldcard-rng-vulnerability/)
- **SafePal (2026).** An authorization flaw in an order-tracking plugin exposed the data of 39,798 customers. — [FinanceFeeds](https://financefeeds.com/bitbox-wallet-flaws-emerge-after-coldcard-exploit-led-to-112m-in-losses/)
- **Dark Skippy (disclosed 2024-08-05; Nick Farrow of Frostsnap and co-researchers).** Malicious signer firmware writes seed entropy into low-entropy nonces: 8 bytes per signature, so two signatures leak a 12-word seed and four leak a 24-word seed. The attacker recovers the seed from the chain using Pollard's kangaroo algorithm. — [darkskippy.com](https://darkskippy.com/); [FAQ](https://darkskippy.com/faq.html); [Mitigations](https://darkskippy.com/mitigations.html)
- **Anti-exfil / anti-klepto.** Sign-to-contract makes the device commit its nonce to randomness supplied by the host, so malicious firmware cannot choose the nonce freely. BitBox02 and Blockstream Jade implement it for ECDSA; Schnorr support is still in progress. — [BitBox anti-klepto](https://bitbox.swiss/blog/anti-klepto-explained-protection-against-leaking-private-keys/); [Blockstream Anti-Exfil](https://blog.blockstream.com/anti-exfil-stopping-key-exfiltration/)
- RFC 6979 deterministic nonces are recommended, but on their own they do not stop malicious firmware, which can ignore the RFC. That is why the external verification anti-exfil provides is needed. — [darkskippy mitigations](https://darkskippy.com/mitigations.html)

### Inferences
- For a DIY wallet, "the TRNG actually feeds seed generation in the production build" is a critical test. The Coldcard bug survived five years because nobody tested the entropy source in release builds. A build-time assertion plus statistical output tests on real hardware would have caught it.
- Anti-exfil protects users from the vendor's own firmware and from firmware-update compromise. It is a strong trust signal for a new, unknown vendor.

### Gaps
- No specific Keystone vulnerability disclosure was found in this pass.
- No CVE IDs were found for the 2026 BitBox flaws.

---

## A4. Blind signing, physical coercion and backup failures

### Takeaway
Blind signing turned the Bybit Safe{Wallet} UI compromise (2025-02-21, about $1.4-1.5B) into the largest crypto theft on record, even though the signers used Ledger devices. Physical "$5 wrench" attacks rose sharply in 2025. For a payments wallet, clear signing on a trusted display, together with duress and spending-limit features, matters more than resistance to chip-level attacks.

### Cited Findings
- **Bybit (2025-02-21).** More than $1.4B was taken, including 401,347 ETH. Lazarus/TraderTraitor injected malicious JS into Safe{Wallet}'s AWS S3 bucket, so signers saw a benign transaction. The Ledger devices did not show the real payload or recipient, the signers blind-signed, and the multisig required 3 signers. — [NCC Group](https://www.nccgroup.com/research/in-depth-technical-analysis-of-the-bybit-hack/); [Ledger's lessons](https://www.ledger.com/blog-learning-from-the-bybit-safe-attack); [Blockaid](https://blockaid.io/blog/how-to-prevent-the-next-15b-bybit-hack-a-strategic-approach-to-solving-blind-signing)
- **Physical attacks.** Jameson Lopp maintains a GitHub list and dashboard of physical attacks on crypto holders; one report counts 360 incidents. 2025 was called a record year, with an increase of about 169% cited in some reports and on average more than one attack per week. The exact 2025 count differs by source (about 25 in the first 21 weeks versus about 70). — [jlopp/physical-bitcoin-attacks](https://github.com/jlopp/physical-bitcoin-attacks); [Crypto Briefing](https://cryptobriefing.com/lopp-bitcoin-attacks-interactive-dashboard/); [The Block](https://www.theblock.co/post/384018/record-year-wrench-attacks-how-crypto-holders-maintain-physical-security-rising-risks); [CoinDesk 2025-12-10](https://www.coindesk.com/coindesk-news/2025/12/10/most-influential-the-wrench-attackers)

### Inferences
- Leaked customer databases (Ledger 2020, Trezor/ShipMonk 2026, SafePal 2026) give attackers target lists for letters, fake devices and wrench attacks. Keeping as little shipping PII as possible reduces physical risk to customers.

### Gaps
- No reliable, source-backed statistic was found for loss due to seed backup failure (for example "X% of BTC lost"). Chainalysis-style estimates exist but were not fetched.

---

## A5. Structural limitations of hardware wallets

### Takeaway
Across the incidents above, the weak points recur: the seed phrase is a bearer secret that people can be tricked into typing; the host, app or web UI decides what is displayed unless the device can clear-sign; firmware updates are a trusted channel that can export keys (Recover) or leak them (Dark Skippy); closed secure elements require trusting the vendor; and PII leaks create physical risk.

### Cited Findings
- Seed-phrase social engineering through letters, fake devices and support-form abuse — see the Ledger 2021/2025 and Trezor 2025 citations above.
- Host dependence and blind signing — [Ledger Connect Kit report](https://www.ledger.com/blog/security-incident-report); [NCC Group on Bybit](https://www.nccgroup.com/research/in-depth-technical-analysis-of-the-bybit-hack/)
- Firmware can exfiltrate secrets — [Dark Skippy](https://darkskippy.com/); [BitBox: "How almost all hardware wallets can steal your seed"](https://bitbox.swiss/blog/how-almost-all-hardware-wallets-can-steal-your-seed/)
- Trust in closed secure elements: ST and Infineon designs are proprietary. TROPIC01 is presented as the first open-architecture secure element. — [Trezor KB](https://trezor.io/guides/trezor-devices/trezor-safe-7/what-is-the-tropic-01-chip); [Tropic Square](https://www.tropicsquare.com/tropic01)
- An MCU without an SE cannot be fixed against glitching by software — [Kraken](https://blog.kraken.com/product/security/kraken-identifies-critical-flaw-in-trezor-hardware-wallets)

### Inferences
- A Bluetooth link (the nRF54L15's main purpose) adds a remote attack surface for the pairing and GATT stack. The trusted display and button on the device must be the only place where the user confirms a transaction.

### Gaps
- No sourced Bluetooth-specific hardware-wallet exploit was found.

---

## B1. nRF54L15 security capabilities

### Takeaway
The nRF54L15 has strong features for an MCU: a Cortex-M33 with TrustZone; CRACEN, a crypto accelerator and TRNG that Nordic says are hardened against side-channel attacks; KMU key slots; secure boot; secure AP-Protect; and TAMPC with glitch detectors and an active shield. Nordic markets it as "designed for PSA Certified Level 3". However, the PSA Certified database lists only **Level 1** (issued 2026-01-15). The chip is not a CC EAL5+/6+ secure element and has no independent evaluation against physical attacks.

### Cited Findings
- Nordic says the chip is "designed for PSA Certified Level 3" and offers secure boot, secure firmware update, secure storage, integrated tamper sensors, and crypto accelerators hardened against side-channel attacks. — [Nordic nRF54L announcement 2023-10](https://www.nordicsemi.com/Nordic-news/2023/10/Nordic-announces-nRF54L-Series-expanding-industrys-most-efficient-Bluetooth-LE-portfolio); [Product page](https://www.nordicsemi.com/Products/nRF54L15)
- **The actual certificate** is PSA Certified Level 1 v3.1, certificate 0632793520627-13101, issued 2026-01-15 by SERMA. It covers HW rev 2 (AA, C00) running NCS v3.1, TF-M v2.1.2 and MCUboot v2.2.0-c2. No Level 2 or Level 3 is listed. (Nordic's nRF9160 and nRF5340 hold Level 2.) — [PSA Certified database](https://products.psacertified.org/products/nrf54l15-nrf54l10); [Nordic L2 news for nRF9160/nRF5340](https://www.nordicsemi.com/Nordic-news/2023/08/nRF9160-SiP-and-nRF5340-SoC-achieve-PSA-Certified-Level-2-for-enhanced-IoT-security-assurance)
- **CRACEN** is the hardware crypto and entropy peripheral that replaces CryptoCell-312. It is used through the PSA Crypto API (`CONFIG_PSA_CRYPTO_DRIVER_CRACEN`). **KMU** is hardware key storage with push and read protection per slot. — [NCS KMU/CRACEN overview](https://nrfconnectdocs.nordicsemi.com/ncs/latest/nrf/app_dev/device_guides/kmu_guides/kmu_cracen_overview.html) (snippet); [mbedded.ninja](https://blog.mbedded.ninja/programming/microcontrollers/nordic/nrf54/)
- CRACEN supports ECDSA and ECDH on secp256r1 (P-256). secp192k1/r1 are not supported. **Hardware support for secp256k1 is not confirmed** in the sources found. — [NCS crypto drivers](https://nrfconnectdocs.nordicsemi.com/ncs/latest/nrf/security/crypto/drivers.html); [DevZone secp192r1 thread](https://devzone.nordicsemi.com/f/nordic-q-a/127239/clarification-needed-secp192r1-support-for-nrf54l15/562487)
- A known limitation: `psa_generate_key` into the KMU failed with PSA_ERROR_NOT_SUPPORTED on NCS 2.9.1 (one DevZone report). — [DevZone](https://devzone.nordicsemi.com/f/nordic-q-a/124560/psa_generate_key-into-kmu-fails-on-nrf54l15-ncs-2-9-1-with-psa_error_not_supported)
- **TAMPC (tamper controller).** Glitch detectors catch timing violations in internal logic, defending against voltage spikes and EMFI. GLITCHDET watches the supply and resets the chip. External tamper detection uses an active shield: a PRBS on output pins, checked on input pins. The nRF54L15 has 4 active-shield pin pairs. — [Nordic Academy: Tamper detection](https://academy.nordicsemi.com/courses/nrf54l-series-express-course/lessons/lesson-4-security-features/topic/tamper-detection/) (snippet); [element14](https://community.element14.com/learn/learning-center/the-tech-connection/b/blog/posts/nrf54l-series-soc-beats-wireless-mcu-barriers)
- In NCS, AP-Protect on nRF54L has both a hardware form and a software form (`CONFIG_NRF_APPROTECT_*`, `CONFIG_NRF_SECURE_APPROTECT_*`). — [NCS 3.2.0 ap_protect](https://docs.nordicsemi.com/bundle/ncs-3.2.0/page/nrf/security/ap_protect.html)

### Inferences
- "Designed for Level 3" is a marketing target, not an evaluated result. For a wallet the relevant evidence is resistance to physical attacks (PSA L3 or CC AVA_VAN.5), and the nRF54L15 does not have it as of 2026-09.
- If CRACEN lacks secp256k1, Bitcoin and Ethereum signing will run in software (for example libsecp256k1 or trezor-crypto) on the M33. The private key then sits in general RAM and flash, not in the KMU, so side-channel and glitch resistance rests on the software implementation.

### Gaps
- The authoritative nRF54L15 datasheet sections on the KMU slot count, CRACEN algorithm list and side-channel claims could not be fetched (403 or PDF not retrieved). The team should read the official datasheet v1.0 ([Mouser mirror](https://www.mouser.lt/datasheet/3/926/1/nRF54L15_nRF54L10_nRF54L05_Datasheet_v1.0.pdf)).
- No public SESIP certification for the nRF54L15 was found.

---

## B2. Known attacks on nRF52/nRF54 and whether nRF54L fixes them

### Takeaway
Every nRF52 revision before the hardened ones is vulnerable to APPROTECT bypass by voltage glitching (LimitedResults, 2020, CVE-2020-27211). The attack is cheap and has been reused widely, for example to dump AirTag firmware. No public glitch attack on the nRF54L was found. Nordic's countermeasure is TAMPC plus glitch detectors, but no independent evaluation confirms it is effective.

### Cited Findings
- LimitedResults (published 2020) re-enabled SWD on a protected nRF52840 with a home-made voltage glitcher, giving full debug access. All nRF52 variants were affected, and the flaw cannot be fixed without a silicon change. The CVE is CVE-2020-27211. — [LimitedResults](https://www.limitedresults.com/results/nrf52-debug-resurrection-approtect-bypass); [CNX Software 2020-06-15](https://www.cnx-software.com/2020/06/15/nordic-semi-nrf52-wisocs-are-susceptible-to-debug-resurrection-using-approtect-bypass/); [CVE-2020-27211 write-up](https://cvereports.hashnode.dev/cve-2020-27211-voltage-glitching-the-nordic-nrf52-how-a-zap-resurrected-the-debugger)
- Nordic's response was an "improved APPROTECT" in later nRF52 revisions (for example nRF52840 Rev 3), which firmware must keep enabled. — [Nordic DevZone blog](https://devzone.nordicsemi.com/nordic/nordic-blog/b/blog/posts/working-with-the-nrf52-series-improved-approtect)
- The attack became a commodity: courses teach it and cheap tools use it (for example the AirTag dump and an ESP32-based nRF52 unlocker). — [Hextree course](https://app.hextree.io/courses/fault-injection-nrf52-approtect); [airtag-dump](https://github.com/pd0wm/airtag-dump); [ESP32_nRF52_SWD](https://github.com/atc1441/ESP32_nRF52_SWD)

### Inferences
- The nRF54L's glitch detectors and hardware and software AP-Protect directly target this attack class. Without third-party testing, however, the team should assume a skilled attacker with EMFI or voltage-glitching equipment might still succeed. Design so that dumping flash does not reveal the seed: encrypt it under a key that needs the user's PIN or passphrase plus an external secure element that enforces the retry counter.

### Gaps
- No public research (Donjon, LimitedResults, Hextree, academic) on fault injection against the nRF54L was found up to 2026-09.

---

## B3. External secure element options vs the nRF54L15 alone

### Takeaway
CC-certified secure elements add the evaluated physical-attack resistance the nRF54L15 lacks, plus hardware-enforced PIN retry counters. The main trade-offs are closed or NDA designs, limited curve support (many IoT SEs lack secp256k1), and I2C/SPI bus exposure unless the link is paired and encrypted.

### Cited Findings
- Hardware wallets use these secure elements: Ledger Stax/Flex use ST33K1M5 (EAL6+); Nano X/S Plus use ST33J2M0 (EAL5+); Trezor Safe 5 uses Infineon OPTIGA Trust M (EAL6+); Trezor Safe 7 uses TROPIC01. — [Cold Storage Crypto](https://www.coldstoragecrypto.com/guides/what-is-a-secure-element) (secondary); [Trezor KB](https://trezor.io/learn/security-privacy/how-trezor-keeps-you-safe/secure-elements-in-trezor-safe-devices)
- TROPIC01 is an open-architecture RISC-V secure element from Tropic Square, a SatoshiLabs company. It is in full production and available worldwide, and is described as meeting "EAL5+ equivalent" standards (not a formal CC certificate, per that wording). — [Tropic Square](https://www.tropicsquare.com/tropic01); [SatoshiLabs](https://satoshilabs.com/news/tropic01-the-future-proof-secure-element-now-in-full-production-and-available-worldwide)
- Even with an SE, the Trezor Safe 3 MCU could be glitched to bypass the firmware check, although the SE kept the PIN and keys safe. — [Trezor advisory](https://trezor.io/vulnerability/donjon-s-trezor-safe-3-evaluation)

### Inferences
- A realistic design for the NU-54V-DK: the nRF54L15 handles BLE, the UI and the signing logic, and an external SE (OPTIGA Trust M, SE050 or TROPIC01) stores a PIN-gated wrapping secret with a hardware retry counter. The seed stays encrypted in RRAM. This follows the Trezor Safe model, where the MCU signs and the SE gates the secret. It does not protect against a compromised MCU during an unlocked session.

### Gaps
- Certification levels for NXP SE050 (vendor claims CC EAL6+), Microchip ATECC608 (widely cited as having no CC certificate) and ST33 variants were **not verified against primary certificates** in this pass. Check the CC portal or BSI/ANSSI certificate lists.
- secp256k1 support per SE (for example ATECC608 supports only P-256; SE050 is said to support secp256k1) is unverified.
