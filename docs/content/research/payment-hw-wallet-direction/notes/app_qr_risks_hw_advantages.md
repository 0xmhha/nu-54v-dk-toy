# App-based (Smartphone Hot Wallet) QR Crypto/Stablecoin Payments: Failure Modes, and What Hardware Wallets Do and Do Not Fix

Research date: 2026-09-23. Scope: 2023-2026. Every figure carries a URL and a date. Items marked **[UNVERIFIED]** come from secondary/aggregator sources or snippets that were not checked against a primary source.

## Q1. Hot wallet key compromise on phones (malware, infostealers, fake apps, OCR of seed photos) and loss statistics

### Takeaway
Theft from individuals is now a high-volume, low-ticket problem: Chainalysis counted 158,000 personal wallet compromises (80,000 victims, $713M) in 2025, and signature-phishing drainers alone took $83.85M from 106,000 victims. Mobile-specific vectors are proven in the wild: OCR trojans in the official App Store/Google Play that scan photo galleries for seed phrases, clippers, fake wallet apps, and even a 2026 Android chipset flaw that lets someone with physical access extract seeds from hot wallet apps in about 45 seconds.

### Cited Findings
- Chainalysis (published 2025-12-18): 2025 personal wallet compromises = 158,000 incidents, at least 80,000 unique victims, up from 54,000 incidents in 2022. Value stolen from individuals fell from $1.5B (2024) to $713M (2025); share of total stolen value fell from 44% (2024) to 20% (2025), distorted by the $1.5B Bybit hack. Chainalysis sums it up as "attackers are targeting more users, but stealing smaller amounts per victim." — [Chainalysis, "2025 Crypto Theft Reaches $3.4 Billion", 2025-12-18](https://www.chainalysis.com/blog/crypto-hacking-stolen-funds-2026/)
- Average loss per personal-wallet victim is about $4,500 (derived: $713M / ~158k incidents or ~80k victims; the aggregator's own arithmetic, not a Chainalysis-stated figure) **[UNVERIFIED derivation]** — [shattered.io summary](https://shattered.io/crypto-wallet-security-12-steps-2026/)
- Scam Sniffer 2025 annual report (published ~Jan 2026): wallet-drainer phishing losses were $83.85M in 2025, down 83% from ~$494M in 2024. Victims numbered 106,000 (down 68%). Q3 2025 was the worst quarter ($31M) and tracked the ETH rally. There were 11 cases above $1M (30 in 2024). The largest single loss was $6.5M through a malicious Permit signature (Sept 2025). Permit/Permit2 caused 38% of losses in cases above $1M. Two EIP-7702 cases in Aug 2025 cost $2.54M. — [Scam Sniffer 2025 report](https://drops.scamsniffer.io/scam-sniffer-2025-crypto-phishing-losses-fall-83-to-84-million/); [Cointelegraph coverage](https://cointelegraph.com/news/crypto-phishing-losses-fell-83-percent-2025-wallet-drainers)
- SparkCat (Kaspersky, reported Jan/Feb 2025): the first known OCR trojan stealer to reach the Apple App Store. It scanned device images for text such as seed phrases. — [Kaspersky blog, SparkCat](https://www.kaspersky.com/blog/ios-android-ocr-stealer-sparkcat/52980/)
- SparkKitty (Kaspersky Securelist, 2025-06-23): a trojan spy found in both the App Store (e.g., the "币coin" crypto tracker) and Google Play (a messaging app with crypto features that had more than 10,000 installs), plus modded TikTok builds and enterprise provisioning profiles. It exfiltrates gallery photos. One cluster used Google ML Kit OCR to upload only images containing text ("at least three lines containing a word with a minimum of three letters"). Targets were mainly China and Southeast Asia. It is linked to SparkCat through shared frameworks and debug paths, and has been active since at least Feb 2024. Apple and Google removed the apps. — [Securelist, 2025-06-23](https://securelist.com/sparkkitty-ios-android-malware/116793/); [Kaspersky press release](https://www.kaspersky.com/about/press-releases/kaspersky-has-discovered-sparkkitty-a-new-trojan-spy-on-app-store-and-google-play)
- Clippers: malware that replaces a copied crypto address with the attacker's address. The first one found on Google Play (ESET, Feb 2019) impersonated MetaMask and also stole credentials and private keys. This is older than the 2023-2026 window but is the reference case for the technique. — [SecurityWeek](https://www.securityweek.com/clipper-malware-slips-google-play/); [Immunefi explainer](https://medium.com/immunefi/the-malware-that-swaps-your-address-and-drains-your-wallet-552915fba542)
- Fake wallet apps: Cyble reported 20 Android apps on Google Play (2025) that impersonated popular wallets to phish credentials and seeds, some with hundreds of thousands of downloads; all have since been removed. **[UNVERIFIED: seen only via secondary summary]** — [Yellow.com roundup](https://yellow.com/news/top-10-crypto-malware-threats-of-2025-how-to-keep-your-mobile-wallet-safe); [Yahoo Tech](https://tech.yahoo.com/cybersecurity/articles/delete-android-crypto-apps-now-133608953.html)
- Ledger Donjon, CVE-2025-20435 (reported 2026-03-11): on MediaTek-based Android phones using Trustonic's TEE (estimated about 25% of Android devices), an attacker with physical USB access before the OS boots can extract the full-disk-encryption root keys. The attacker then recovers the PIN and decrypts storage, and pulls seed phrases from hot wallet apps (Trust Wallet, Kraken Wallet, Phantom; secondary sources also list Base, Rabby, Tangem app). This takes about 45 seconds and works even with the phone turned off. Patches exist but have not reached all devices. — [The Block, 2026-03-11](https://www.theblock.co/post/393154/ledger-researchers-expose-android-flaw-enabling-theft); [The Defiant](https://thedefiant.io/news/hacks/ledger-donjon-team-finds-android-vulnerability). Note: Ledger publicly called a separate "LDN-2026-0301" report fake and pointed to CVE-2025-20435 as the real research — [Ledger on X](https://x.com/Ledger/status/2032466333745484093)
- Trust Wallet (Dec 2025): about $7M stolen through a malicious Chrome extension update that harvested seed phrases. This is browser-based, not mobile, but it shows the software-wallet update channel acting as a supply-chain risk. **[UNVERIFIED: secondary snippet only]** — [The Defiant](https://thedefiant.io/news/hacks/ledger-donjon-team-finds-android-vulnerability)

### Inferences
- For a payment use case (small, frequent amounts), the threat that matters is volume-oriented. It needs no targeting, so drainers, clippers, OCR stealers and fake apps each reach thousands of cheap victims. Any wallet whose key lives in phone storage inherits the phone's full malware and physical-access attack surface.
- Photographing the seed phrase is common on low-end or shared phones, and OCR stealers make that habit a direct key compromise. Hardware form factors that never show the seed (e.g., Tangem's no-seed card mode) remove that path entirely.

### Gaps
- CertiK, Immunefi, and SlowMist 2025 annual figures for phishing, drainers, and mobile losses were not retrieved in this pass.
- No reliable per-platform breakdown (mobile vs desktop vs extension) of personal-wallet losses was found.

---

## Q2. Address poisoning, QR tampering/replacement stickers, clipboard swapping

### Takeaway
Address poisoning is industrial-scale and well measured academically: 270M+ attempts against 17M+ victims on Ethereum/BSC. QR sticker overlays are a documented, spreading fraud against parking and payment QR codes in 2025-2026, but the documented cases send victims to card-phishing web pages. I found no well-documented crypto-address QR-sticker case with loss figures.

### Cited Findings
- USENIX Security 2025, "Blockchain Address Poisoning" (Tsuchiya et al.): a two-year measurement on Ethereum and BSC found more than 270 million poisoning attempts against more than 17 million victims. Attackers successfully received 6,633 transfers totaling more than $83.8M. The paper also covers tiny-value and zero-value transfers of stablecoins. — [USENIX paper PDF](https://www.usenix.org/system/files/usenixsecurity25-tsuchiya.pdf); [arXiv 2501.16681](https://arxiv.org/html/2501.16681v1)
- A separate USENIX 2025 study on "address misuse" found 65,340 high-risk misuse cases across about 2.5M transactions, with losses of more than $574.8M (valued at May 2025 prices). This is a different phenomenon from poisoning and should not be summed with it. — [CryptoSlate](https://cryptoslate.com/study-finds-65340-risky-crypto-addresses-tied-to-574-million-in-losses/)
- A large stablecoin loss through combined address poisoning and phishing was reported in May 2025 (FXStreet headline "Crypto investor loses $2.6M in stablecoins in double phishing scam"; the exact figure was not verified) **[UNVERIFIED]** — [FXStreet](https://www.fxstreet.com/amp/cryptocurrencies/news/crypto-investor-loses-26m-in-stablecoins-in-double-phishing-scam-202505261114)
- QR sticker overlays: in 2025-2026 the FTC and multiple cities warned about fake "scan to pay" stickers placed over legitimate parking QR codes. Redondo Beach found about 150 on meters. NYC (June 2025) and Asheville NC (Feb 2026, about 20 stickers) issued alerts. In one case a victim lost $2,000. The stickers lead to card-phishing sites, not crypto addresses. — [NY1, 2025-06-09](https://ny1.com/nyc/all-boroughs/news/2025/06/09/nyc-parking-meter-scam-fake-qr-codes); [KTLA](https://ktla.com/news/california/why-parking-meter-qr-codes-cant-always-be-trusted/); [ScamWatchHQ on FTC warning, 2026](https://scamwatchhq.com/parking-meter-qr-code-sticker-scam-ftc-2026/)
- Clipboard swapping: see the clipper findings in Q1 — [SecurityWeek](https://www.securityweek.com/clipper-malware-slips-google-play/)

### Inferences
- A static merchant QR code that encodes a bare address (EIP-681 or BIP-21 style) can be overlaid physically as easily as a parking QR code, and nothing in the payment URI authenticates the merchant. Compared with a card-phishing page, a crypto overlay is simpler (swap only the address) and its losses are irreversible.
- A hardware wallet's trusted display does not by itself stop QR substitution or poisoning. It faithfully shows the attacker's address, and the user has no reference to compare it against. The display only helps when combined with (a) an address book or allowlist stored on the device, (b) a merchant identity bound to the request (a signed payment request or a name-resolution check), or (c) dynamic per-order QR codes shown on a merchant-controlled screen.

### Gaps
- No primary-source incident with loss figures of a *crypto* payment QR sticker swap was found in 2023-2026.
- No data on how often poisoning succeeds against mobile wallets versus other wallet types.

---

## Q3. Blind signing and malicious approvals; the Bybit Feb 2025 lesson

### Takeaway
Signature-based theft (approve/Permit/Permit2, and now EIP-7702 delegations) is the dominant drainer technique. The Bybit hack (Feb 2025, about $1.5B) showed that hardware signers do not protect you when the device displays opaque data and the host UI lies: the keys were never extracted, but the signers approved a malicious Safe upgrade.

### Cited Findings
- Bybit, 2025-02-21: about $1.5B stolen (401,000+ ETH plus staked tokens). The Lazarus Group compromised a Safe{Wallet} developer machine and injected JavaScript into Safe's front end, served from an AWS S3 bucket. The script activated only for Bybit's cold-wallet Safe, changed the transaction in real time while showing the original values, and turned the transaction into a malicious contract upgrade. Sygnia found no compromise of Bybit's own infrastructure. — [Sygnia investigation](https://www.sygnia.co/blog/sygnia-investigation-bybit-hack/); [BleepingComputer](https://www.bleepingcomputer.com/news/security/lazarus-hacked-bybit-via-breached-safe-wallet-developer-machine/); [Chainalysis, 2025-12-18](https://www.chainalysis.com/blog/crypto-hacking-stolen-funds-2026/)
- Ledger's post-mortem (2025-03-27): "the private keys themselves were not compromised". The malicious code "modified transactions in real-time but displayed original values to signers". Ledger calls this blind signing and argues for Clear Signing of every transaction. It also notes that "all enterprise-grade self-custody solutions on the market rely on non-secured intent/approval verification." — [Ledger blog, 2025-03-27](https://www.ledger.com/blog-learning-from-the-bybit-safe-attack)
- After the hack, Safe{Wallet} restored service in phases and temporarily removed its native Ledger integration, the signing method used in the heist. — [Huntress summary](https://www.huntress.com/threat-library/data-breach/bybit-cryptocurrency-exchange-data-breach); [The Block](https://www.theblock.co/post/343530/lazarus-appears-to-compromise-safe-developer-machine-in-lead-up-to-1-5-billion-bybit-hack-report)
- Permit/Permit2 caused 38% of drainer losses in cases above $1M in 2025, and the largest single phishing loss was $6.5M through a Permit signature. — [Scam Sniffer 2025](https://drops.scamsniffer.io/scam-sniffer-2025-crypto-phishing-losses-fall-83-to-84-million/)
- EIP-7702 delegation phishing: two cases in Aug 2025 lost $2.54M. — [Scam Sniffer 2025](https://drops.scamsniffer.io/scam-sniffer-2025-crypto-phishing-losses-fall-83-to-84-million/)

### Inferences
- "What you see is what you sign" (WYSIWYS) holds only when the secure display decodes the actual bytes being signed into human-meaningful intent. A hardware wallet that shows a hash, or "data present", gives no protection against a compromised host. Bybit is the canonical proof at $1.5B scale.
- For a payment product, the signing surface should be kept deliberately narrow: plain ERC-20 `transfer` to a merchant, or a typed payment struct. Allowing arbitrary approve/permit or 7702 delegation from the payment key widens the drainer surface for no payment benefit.

### Gaps
- No quantitative data on how many drainer victims used hardware wallets.

---

## Q4. Payment-specific UX problems of phone-based QR crypto payments

### Takeaway
Beyond security, QR crypto payments lack the primitives card payments take for granted: chain and token disambiguation, gas abstraction, fast finality at the till, refunds or chargebacks, and receipts or reconciliation. Standards cover some of these (Solana Pay's `reference` for reconciliation, EIP-681 for chain ID and token), but most are left to each wallet or merchant stack.

### Cited Findings
- EIP-681 encodes ETH and ERC-20 payment requests as URLs (target address, optional `@chain_id`, function such as `transfer`, and parameters). The URI carries no merchant authentication and no order or reference semantics. — [EIP-681](https://eips.ethereum.org/EIPS/eip-681)
- Solana Pay (inspired by BIP-21 and EIP-681) defines a transfer request with recipient, amount, `spl-token`, label, message, memo, and `reference`. `reference` should be unique per customer session and lets the merchant find and validate the payment before knowing the transaction signature, via `getSignaturesForAddress`. Solana Pay also defines *transaction requests*, where the wallet fetches a merchant-built transaction over HTTPS. — [Solana Pay spec](https://docs.solanapay.com/spec); [Solana docs v1.1](https://solana.com/docs/tools/solana-pay/specification/version1-1)
- MetaMask Card requires the user to sign on-chain approvals to set spending limits, and each approval costs gas. This illustrates the native-gas and approval friction even in "self-custody" card products. — [CoinMarketCap Academy](https://coinmarketcap.com/academy/article/metamask-and-mastercard-launch-self-custody-crypto-card-across-us); [Consensys](https://consensys.io/blog/metamask-mastercard-and-baanx-unveil-revolutionary-way-to-pay-with-crypto-pr)

### Inferences (no single primary source; derived from protocol properties)
- Wrong network or token: the same address format exists across EVM chains, and USDC/USDT are deployed on many chains. A QR code without chain ID and token contract, or a wallet that ignores them, leads to funds sent on the wrong chain. Recovery depends on the merchant controlling the same address on that chain.
- Native gas: a user holding only stablecoins cannot pay on most EVM chains without the native token. ERC-4337 paymasters or chain-level fee abstraction solve this, at the cost of relying on a sponsor.
- Latency and finality at POS: the merchant must choose between accepting on mempool or first inclusion (reorg or replacement risk) and waiting for finality (checkout delay). This cost depends on the chain.
- No chargeback or refund primitive: refunds are new outbound transfers from the merchant. They need the customer's refund address, and the customer's sending address may be an exchange hot wallet.
- Overpayment or underpayment and reconciliation: without a per-order reference (Solana Pay) or per-order deposit address, merchants must match on amount and time, which becomes ambiguous with concurrent orders.
- Offline or low connectivity: the phone needs RPC access to build, sign, and broadcast, and the merchant needs indexer access to confirm.
- Low-end or shared phones: these combine the weakest security (unpatched OS, e.g., CVE-2025-20435 on MediaTek devices, sideloaded apps) with multi-user exposure of the wallet app.

### Gaps
- No quantified data found on wrong-chain loss rates, POS abandonment due to confirmation latency, or merchant reconciliation error rates.

---

## Q5. Standards that address these problems

### Takeaway
ERC-7730 (clear signing metadata, contributed by Ledger with governance moving to the Ethereum Foundation) is the most direct fix for blind signing on hardware displays. EIP-712 structures off-chain messages. EIP-681 and Solana Pay structure payment requests. ERC-4337 paymasters remove the native-gas requirement. No widely deployed standard authenticates the merchant behind a QR code.

### Cited Findings
- ERC-7730: a protocol author publishes a JSON descriptor that maps contract functions and messages to human-readable fields in an open registry. At signing time the wallet fetches the descriptor and renders the transaction in plain language on the Secure Screen. Ledger released it in 2025, open-sourced tooling (Builder, Linter, Tester, Analyzer), published "ERC-7730 v2", and is transferring governance to the Ethereum Foundation. The registry now lives at github.com/ethereum/clear-signing-erc7730-registry. — [Ledger, "ERC-7730 v2 & The Evolution of Clear Signing"](https://www.ledger.com/blog-the-evolution-of-clear-signing); [Ledger developer docs](https://developers.ledger.com/docs/clear-signing/overview); [Ethereum registry](https://github.com/ethereum/clear-signing-erc7730-registry)
- EIP-681 payment URIs — [EIP-681](https://eips.ethereum.org/EIPS/eip-681); Solana Pay — [spec](https://docs.solanapay.com/spec)

### Inferences
- ERC-7730 moves trust from the host UI to the descriptor registry and its curation. A malicious or missing descriptor falls back to blind signing, or to misleading clear signing if the registry is poisoned.
- For payments, the cleanest combination is: dynamic per-order QR code (EIP-681 or Solana Pay with a reference) + device-side decoding of token, amount, recipient and chain + a merchant identity check + sponsored gas.

### Gaps
- I did not retrieve EIP-712, ERC-4337, or transaction-simulation vendor sources in this pass. Their descriptions above come from general protocol knowledge and should be verified before being cited.
- No standard was found for signed merchant payment requests on EVM, comparable to the old BIP-70 in Bitcoin (itself deprecated).

---

## Q6. Hardware wallet advantages for payments, and limits

### Takeaway
Hardware wallets directly neutralize key extraction by phone malware, OCR seed theft (if the seed never touches the phone), and physical phone-storage attacks such as CVE-2025-20435. With clear signing they also neutralize host-side transaction tampering. They do not fix address poisoning or QR substitution, social-engineering seed phishing, blind-signed approvals, or smart-contract bugs in the payment rail, and they add cost and friction for small payments.

### Cited Findings (advantages)
- Key isolation: in Bybit the keys were never compromised, which shows the key-extraction path was closed. The failure was intent verification. — [Ledger, 2025-03-27](https://www.ledger.com/blog-learning-from-the-bybit-safe-attack)
- Phone storage is not a safe key store: Donjon extracted seeds from hot wallet apps in about 45 seconds through a chipset boot flaw. Ledger's CTO framed this as smartphones lacking an adequate security architecture for crypto keys. (Ledger is an interested party.) — [The Block, 2026-03-11](https://www.theblock.co/post/393154/ledger-researchers-expose-android-flaw-enabling-theft)
- Trusted display plus clear signing is the fix Ledger proposes for host-UI manipulation. — [Ledger ERC-7730 blog](https://www.ledger.com/blog-the-evolution-of-clear-signing)

### Cited Findings (limits)
- Blind signing defeats hardware signers (Bybit, about $1.5B). — [Ledger](https://www.ledger.com/blog-learning-from-the-bybit-safe-attack); [Sygnia](https://www.sygnia.co/blog/sygnia-investigation-bybit-hack/)
- Seed phishing aimed at hardware-wallet owners: physical letters impersonating Ledger with a fake "Quantum Resistance" update notice ask victims to enter their seed. — [HackRead](https://hackread.com/scammers-physical-phishing-letters-ledger-wallet-seed/)
- Payment-rail contract risk is independent of the key device. Gnosis Pay (Safe + Delay/Roles modules) was exploited on 2026-06-01 through a missing success check on a `staticcall` in zodiac-core, introduced by a 2023-10-28 commit and silently fixed on 2026-02-27. Gnosis said about $1.5M was extracted plus about $300k made inaccessible; CertiK-reported coverage cited about $265K. **These figures conflict.** Gnosis reimbursed users. — [Gnosis Pay post-mortem](https://gnosispay.com/blog/post-mortem-gnosis-pay-vulnerability-exploit); [Verichains](https://blog.verichains.io/p/gnosis-pay-exploit-the-devs-discovered); [CryptoTimes citing CertiK, 2026-06-05](https://www.cryptotimes.io/2026/06/05/delay-module-trick-costs-gnosispay-265k-reports-certik/)

### Inferences
- Mapping each failure mode to hardware-wallet mitigation:
  - Phone malware or infostealer key theft: mitigated (key never on the phone).
  - OCR theft of seed photos: mitigated only if the device never exposes a seed (seedless or backup-card model), or if users stop photographing seeds.
  - Physical phone-storage extraction (CVE-2025-20435): mitigated.
  - Host UI or transaction tampering: mitigated only with clear signing of every field. Not mitigated under blind signing.
  - Clipper, poisoning, QR sticker: only partly mitigated. The device shows the wrong address faithfully. It needs an on-device allowlist or a merchant-identity binding.
  - Approval/permit/7702 phishing: mitigated only if the device decodes these and warns, or refuses them, on a payment key.
  - Seed phishing and social engineering: not mitigated.
  - Payment-rail contract bugs (Gnosis Pay case): not mitigated.
  - Shared-phone environment: mitigated. PIN on the device, key off the phone, and a physical tap or button per payment.
  - Cost and friction for small payments: a real drawback. Needs tap-style (NFC card) UX and possibly pre-authorized allowances.
- Pre-authorized allowances (the MetaMask Card, Gnosis Pay, and ether.fi model) move per-payment authorization away from the hardware key. The hardware key protects only allowance setup and withdrawal.

### Gaps
- No independent statistic comparing loss rates of hardware-wallet users with hot-wallet users.

---

## Q7. Existing payment-oriented hardware form factors: does a hardware key authorize the POS payment?

### Takeaway
None of the mainstream "self-custody card" products has the user's hardware key sign each POS payment. Tangem Pay uses a 2-key scheme where the issuer's scoped key co-authorizes card spends. Gnosis Pay uses a Safe Roles Module allowance plus a 3-minute Delay Module. MetaMask Card uses on-chain token approvals. ether.fi uses a Safe vault with a spending limit, and its keys are managed via Turnkey. In all of them the user key sets limits and the issuer's rail executes payments over Visa or Mastercard.

### Cited Findings
- Tangem Pay: a non-custodial payment account in the Tangem app that spends USDC on Polygon through a Visa card, added to Apple Pay or Google Pay for contactless payment. It uses a "two-key architecture": one key on the Tangem chip, and a second held by the issuing partner (commonly referenced as Rain) that is "tightly restricted in scope" and can only authorize payments meeting predefined conditions. It launched in phases from late 2024 through 2025 in the US (partial), LatAm, and APAC. The Tangem card itself signs only through NFC tap to the phone. — [Coin Bureau](https://coinbureau.com/education/what-is-tangem-pay); [Tangem Pay page](https://tangem.com/en/tangem-pay/); [The Defiant, rollout](https://thedefiant.io/news/infrastructure/tangem-announces-global-rollout-of-tangem-pay)
- Gnosis Pay: a Safe smart account with a Roles Module, which authorizes the card-settlement address within a user-set daily limit for a chosen stablecoin, and a Delay Module, which puts a roughly 3-minute delay on user-initiated outgoing transactions so they can be cancelled. — [Gnosis Pay help center](https://help.gnosispay.com/hc/en-us/articles/39400440331668-How-the-Delay-and-Roles-Modules-Secure-Your-Card); [Gnosis Pay docs](https://docs.gnosispay.com/account)
- MetaMask Card: spends from a self-custodial wallet using token permissions (on-chain approvals, which cost gas). It is issued by Cross River Bank with Monavate (formerly Baanx) on Mastercard, and supports USDC/USDT and others on Linea, Solana, and Base. — [CoinMarketCap Academy](https://coinmarketcap.com/academy/article/metamask-and-mastercard-launch-self-custody-crypto-card-across-us); [Consensys](https://consensys.io/blog/metamask-mastercard-and-baanx-unveil-revolutionary-way-to-pay-with-crypto-pr)
- ether.fi Cash: funds sit in a Gnosis Safe vault on Scroll. Spending is capped by a user-set limit, and limit changes take effect after a security delay. Key management is via Turnkey. — [ether.fi help center](https://help.ether.fi/en/articles/262378-frequently-asked-questions); [HackerNoon](https://hackernoon.com/can-you-spend-crypto-without-selling-it-inside-the-etherfi-cash-cards-never-sell-revolution) **[secondary sources partly UNVERIFIED]**

### Inferences
- The industry has converged on the pattern "hardware or self-custody key sets a bounded allowance, and a scoped issuer key executes card-rail payments." This pattern fits card-network latency and chargeback rules. It is not a design in which the hardware wallet directly signs a merchant QR payment.
- A product in which a hardware key signs each merchant payment (a QR or NFC request shown on the device, then a physical confirmation) appears to be an open niche. Its hard parts are merchant authentication, finality at the till, and gas abstraction, not key security.

### Gaps
- Ledger Stax/Flex: no source found for a native merchant-payment flow (POS QR or NFC) as of 2026-09.
- No source found for crypto smart cards other than Tangem, or for merchant-side POS hardware signers, being used for direct on-chain POS payments.
- Whether Tangem Pay requires a card tap for every purchase or only for setup and top-up was not confirmed from a primary Tangem document.
