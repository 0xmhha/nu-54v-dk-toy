# Building a Stablecoin Travel Wallet with NU-54V-DK: A 12-Week Plan for a Three-Person Team

### Designing a payment device, mobile app, kiosk, and blockchain service for international travelers and independent cafés

I am preparing for a three-month team project. I started by exploring what could be built with the NU-54V-DK: a drone, a humanoid robot controller, a motion glove, a game controller, and a voice recorder. The direction is now much more specific: **a portable device and service that let international travelers pay with stablecoins at local cafés**.

A traveler can connect an existing wallet or create a temporary travel wallet. After placing an order at a café kiosk, the device shows the merchant and amount. The traveler reviews the details and approves the payment with a physical button. The kiosk, user app, and merchant view all resolve the result against the same order. During the rental, payments and visited places can form a travel trail. Before returning the device, the traveler moves any remaining balance and the operator clears the device for the next rental.

![Concept for an NU-54V-DK enclosure and café payment device shaped around the board's long form factor](assets/concepts/three-month-maker-team-hero-nu54v-v2.png)

*This is an AI-generated project concept image. It references NUCODE's official NU-54V-DK product photo to represent the long black PCB and its placement inside an enclosure. It is not a photograph of a finished product or a mechanical drawing. The final enclosure dimensions and port positions will be based on measurements of the physical board.*

The scope has already grown large. This article focuses on what three people must align before they can finish it in twelve weeks. The first practical step will be smaller: I have not used Zephyr before, so I will begin by bringing up the NU-54V-DK's basic peripherals and recording evidence that I can reproduce.

## 1. What needs to improve when this becomes a team project

The first priority is not the number of features. It is **one connected success path**:

> Place an order at the kiosk → send it to the device over BLE → review and approve it with a physical button → submit a StableNet testnet payment → confirm it through the indexer → show the receipt at the kiosk

This flow must work end to end on a real device and real apps. Before implementation, the team must agree on shared contracts such as the order ID, device ID, request version, expiration time, idempotency behavior, and error codes. A polished app is not useful if the firmware and server interpret the same state differently.

The second priority is reducing hardware uncertainty early. We need to confirm the NU-54V-DK revision, Zephyr board target, default LED and button pins, debugger and flashing process, available memory, BLE connection behavior, and measured throughput. Those results will determine when it is realistic to add a display, IMU, microphone, battery, and FOTA.

The initial technology choices are straightforward. The device firmware will use Zephyr. The merchant kiosk will be a React Native app that can run on an Android tablet. An existing Go indexer will be reused for payment confirmation. The contract layer will begin with StableNet testnet and a dummy USDC contract. The user app, backoffice, and travel service will consume the same identifiers and states established by the first payment path.

The third priority is keeping the two wallet roles clear. The portable device acts as the physical wallet where the user sees and approves a request. A social-login-based cloud wallet supports convenience features and recovery in the app. Receipts and audit records must identify which wallet approved each action.

Finally, the three team members need the same definition of done. A screen or an API is not complete by itself. A feature is complete when the same order ID passes through the kiosk, device, chain, indexer, and merchant view, and the system can recover the result after a relevant failure. The public article explains the architecture and user experience without exposing sensitive key-handling details or internal values that could weaken the system.

## 2. The whole product on one page

At first, I thought the project consisted of a portable device and a kiosk. Following the full user journey revealed separate lifecycles for the user app, merchant operations, rentals, blockchain event confirmation, and travel records. I divided the scope into **10 product modules, 57 cohesive work packages, and 104 implementation tasks**.

![Local, Cloud, and Blockchain architecture for the NU-54V-DK travel payment system](assets/diagrams/product-architecture-one-page.png)

*This system diagram was produced directly from the product WBS. Blue represents Local components that run near the traveler or merchant. Green represents Cloud services, operations, and data. Purple represents the StableNet testnet Blockchain layer. Orange represents external identity, place, and AI providers. C1 through C9 mark communication boundaries that the team must agree on, including BLE, HTTPS, chain RPC, and events.*

The diagram is meant to clarify ownership and recovery. Suppose a payment transaction reaches the chain but the kiosk loses its response. The kiosk must look up the existing outcome through the order ID and indexer instead of charging the traveler again. A BLE disconnect also does not reverse a transaction that has already been submitted, so device connectivity and blockchain payment state must remain separate.

## 3. Product modules and core features

### Local — products close to the traveler and merchant

- **P01 · NU-54V-DK firmware:** Zephyr-based hardware wallet, authenticated BLE session, payment display and physical approval, FOTA, passkeys, audio transfer, device finding, payment stamps, and resource arbitration when features compete for BLE or memory.
- **P02 · User app:** Google and Apple sign-in, device enrollment and configuration, FOTA progress, separate hardware and cloud wallet balances, payment approval and receipts, travel records, and DApp screens.
- **P04 · React Native kiosk:** Merchant sign-in, menu and stock management, orders, stablecoin payment, partial and full refunds, sales, and settlement on an Android tablet. It opens a temporary BLE payment session with the NU-54V-DK.

### Cloud — accounts, operations, data, and user services

- **P03 · Cloud MPC wallet:** Social-account-linked 2-of-3 distributed key generation, separate mobile approval, threshold signing, share refresh, and recovery without reconstructing the full private key in one place.
- **P05 · Backoffice:** Merchant approval, device rental and return, balance recovery checks, access revocation, FOTA campaigns, reconciliation, and auditable handling of delayed, duplicated, partial, or excess payments.
- **P07 · Indexer and query frontend:** StableNet block, transaction, and event ingestion with deduplication, restart recovery, backfill, and reorg handling. It exposes payment state to the kiosk and app, and sales, refund, and reconciliation state to operators.
- **P08 · Market service:** Test-scope DEX, TEST FX, and perpetual quotes and execution, together with oracle and keeper state. This module follows after the first payment path is stable.
- **P09 · Travel and AI service:** Place search, payment-backed reviews, travel footprints, audio transcription and summaries, AI-generated itineraries, follow-along challenges, and stamps. Consent and deletion for location, audio, and review data belong to this module as well.
- **P10 · Common platform:** APIs, authentication and authorization, shared IDs and states, configuration, audit logs, observability, CI, acceptance tests, and release rules. It prevents each product from inventing a different interpretation of an order or payment.

### Blockchain — verifiable payment and product state

- **P06 · StableNet contracts:** Dummy USDC and WKRC for the testnet, followed by smart accounts, DID, a restricted CafePass test asset, and x402 payments. DEX, FX, and perpetual events use the same indexer path.

The feature list is broad, but the first success criterion remains narrow: **one café order reviewed and approved on the NU-54V-DK must reach StableNet and return through the indexer as a kiosk receipt and user record**. The other modules reuse the identifiers, states, permissions, and recovery behavior proven by that path.

## 4. A 12-week WBS based on dependencies

Priorities describe the order in which dependencies are opened, rather than a ranking of product value.

- **C0 · Start immediately:** Board bring-up, shared communication contracts, StableNet and token baseline, indexer baseline, and app and kiosk shells.
- **C1 · First end-to-end path:** Hardware-wallet signing, order, payment and reconciliation, two-wallet UI, and receipts.
- **C2 · Required extensions:** FOTA, passkeys, recording and finding, MPC recovery, refunds and settlement, rental and return, market features, and travel features.
- **C3 · Integration and release:** Full regression, failure recovery, permissions and deletion, performance, backup, and operational handover.

### Month 1 — prove the device and the first payment path

Build and flash minimal Zephyr firmware on the NU-54V-DK. Verify the default LED, button, boot logs, timers, BLE advertising, and a small GATT write and notification. At the same time, build a kiosk with one menu item and one order, then connect the dummy USDC contract and existing indexer on StableNet testnet. The first month's goal is not visual polish. It is a reproducible order that travels from device approval to a testnet event.

### Month 2 — integrate the products and recover from failures

Connect device enrollment and wallet setup to the user app. Add orders, payment, partial refunds, sales, and settlement to the kiosk. Add display and button approval, FOTA, passkeys, device finding, audio transfer, and payment stamps to the firmware. Handle BLE disconnects, duplicate requests, expired requests, and delayed chain results as part of the same work.

### Month 3 — validate the café scenario and prepare the demonstration

Use the partner café scenario to repeat enrollment, ordering, approval, payment, refund, and device return for international travelers. Test the 3D-printed enclosure, clip, power, and portability. Record connection time, repeated success rate, recovery behavior, and remaining limitations instead of showing only successful screens.

The schedule is controlled through these integration gates:

- **Week 1 · M0 — Runnable baseline:** Board flash, app and kiosk launch, StableNet RPC, reproducible builds
- **Week 2 · M1 — Connectivity baseline:** Authenticated BLE, token deployment manifest, raw indexer event, order skeleton
- **Week 4 · M2 — First real payment:** NU approval → EOA submission → indexer confirmation → kiosk receipt
- **Week 6 · M3 — Core products:** FOTA, passkey, recording, finding, refund, settlement, MPC signing, core travel flow
- **Week 8 · M4 — First full-scope integration:** Every planned product module has connected once to its real device, account, testnet, or app target
- **Week 10 · M5 — Release candidate:** Café regression and failure, reconciliation, permission, deletion, performance, and recovery checks
- **Week 11 · M6 — Acceptance decision:** Device, wallet, merchant, on-chain, and travel suites with remaining defects recorded
- **Week 12 · M7 — Release and handover:** Reproducible artifacts, operations and recovery documents, demonstration, and evidence index

Code or a screen is not enough to mark a work package complete. The team must execute the normal path and relevant failure and recovery paths on the real target, then preserve versions, logs, photos, transactions, and cross-review evidence.

## 5. My technical strengths and areas of interest

My strengths are blockchain, backend systems, and the boundaries between products. I am interested in how contract events become app and operations state through an indexer, how payment and refund states prevent duplicate effects, and how operators reconcile outcomes after a failure. I want one user action to carry the same meaning across the app, server, contract, and indexer.

Zephyr and development-board firmware are new to me. I will address that gap with small, repeatable hardware tests. My first goal is to reproduce the build and flash process, LED and button behavior, logs, and BLE connection. I have less experience with circuit and power design, PCB work, battery safety, and enclosure design, so I will need review and help in those areas.

## 6. The role I want and where I need help

I want to own the shared system contracts, blockchain and indexer work, and the connection between backend and apps. The main topics are the data model from order to payment and refund, the separation between the physical wallet and cloud wallet, and the evidence that proves the whole flow. I also want to perform the first NU-54V-DK bring-up myself so that the software architecture reflects the physical constraints.

I need help with Zephyr board definitions and pin configuration, debugger use, battery and charging design, microphone, display and IMU integration, and the clip-style 3D-printed enclosure. BLE throughput and FOTA interruption recovery also require shared tests on real devices.

The three primary work streams are **device**, **app and merchant experience**, and **chain and data**. Boundary features will be reviewed by two streams: firmware and app owners review BLE messages together, while app and chain owners verify order and receipt states together.

## 7. The problem domain

The problem I want to explore is making an unfamiliar payment method safer and easier during travel. The main themes are international travelers paying at independent cafés, merchant order, refund, and settlement workflows, visible approval with a screen and physical button, rental-device reset and recovery, and travel footprints grounded in actual payments. Audio notes, AI summaries, suggested routes, and travel challenges can extend that experience.

This twelve-week project is a proof of concept using StableNet testnet and dummy USDC. It is not a commercial payment terminal or a production rental service using real assets. Real assets, personal data, tax, electronic finance, and virtual-asset obligations require separate legal, security, and operational review. This project focuses on a technically verifiable flow and an approval experience a user can understand.

## 8. Available time and resources

The team will align weekly availability and in-person work time before implementation. Integration sessions with the board and apps on the same desk need a separate slot from regular meetings.

The available resources include a physical NU-54V-DK, a Galaxy S25 Ultra and BLE test devices, a partner café, StableNet testnet RPC and Explorer, and existing contract, indexer, and platform repositories. Codex and other AI tools can support design review, documentation, and test-data preparation.

Additional hardware includes a small display, IMU, physical buttons, microphone, battery and charging module, Android tablet for the kiosk, and enclosure tools. Components will be selected after basic peripheral tests confirm interfaces and power requirements.

## 9. The first experiment: bring up the NU-54V-DK peripherals

![Official product image of the NU-54V-DK development board](assets/nu-54v-dk-official.jpg)

*This is a public product image of the NU-54V-DK from the manufacturer. Source: [NUCODE product page](https://nucode.store/product/nu-54v-dk-nucode-nrf54l15-ble-60-mcu-kcfcccemic/36/category/25/display/1/). It is not my own photograph. After the experiment, I will add a photo of the actual board setup and running output.*

The first experiment will follow this order:

1. Record the physical board model, revision, onboard debugger, and connection method with photos.
2. Confirm the supported Zephyr board target and SDK and toolchain versions from official material.
3. Build and flash a minimal sample, then save the boot log.
4. Test the default LED and button defined by the board configuration.
5. Repeat timer and reboot tests to confirm reproducibility.
6. Enable BLE advertising, find and connect from the Galaxy S25 Ultra, then test a small GATT write and notification.
7. Record both successes and failures with commands, configuration, logs, and photos.

The experiment has not been completed yet, so I will not present planned results as facts. I will fill in the following evidence after the work:

- **Board and development environment:** physical revision, SDK, Zephyr, toolchain, and board target
- **Build and flash:** minimal sample, success or failure, and repetition count
- **LED and button:** peripherals in the board definition and observed behavior
- **Boot and button logs:** the minimum logs and screenshots needed to reproduce the result
- **BLE:** advertising, connection, GATT write, and notification results with the Galaxy S25 Ultra
- **Problems and fixes:** symptoms, identified causes, fixes, or the next experiment

After completing this work, I will add photos of the actual board, terminal logs, and the phone's BLE screen. A beginner's record of what failed should also help the team reproduce the environment.

## 10. The scene I want to demonstrate after twelve weeks

A traveler chooses a drink at the café kiosk. The portable device displays the merchant and amount. The traveler reviews the details and presses the button to approve a testnet payment. The kiosk shows a receipt, the user app records the payment and travel footprint, and the merchant view shows the sale and refundable transaction. Firmware, apps, services, contracts, and the indexer all meet in this one scene.

The plan is detailed enough. The starting point is smaller and concrete: flash the first firmware to the NU-54V-DK, record the LED and button logs, and find the BLE advertisement from the phone. I will build the twelve-week result from that first piece of evidence.

`#NUCODE` `#NU54VDK` `#Nucoders` `#EmbeddedSystems` `#Blockchain`
