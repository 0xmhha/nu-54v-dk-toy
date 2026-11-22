# Product

<!-- impeccable:product-schema 1 -->

## Platform

android

## Users

- **Pilot user** (primary): someone holding an NU54DK hardware wallet board for the first time. They set it up alone from the phone, from board enrollment to the first test payment, with no developer beside them.
- **Demo audience**: investors and partners watching a presenter operate the app with a real board. They need to grasp what is happening at a glance: the amount, who is paid and the moment the board approves.
- **Demo visitors**: several people at an event who try the payment themselves. Each installs the app from a QR code (APK download), connects to the shared demo board, receives testnet funds and pays a test merchant.

The design baseline is the pilot user; the demo path gets extra polish so it also reads well to an audience.

## Product Purpose

The owner mobile app is the screen of a screenless hardware wallet. The board holds the keys and gives the final physical approval; the app shows the request, tells the user which board step is running and which button to press, and keeps the records. Success for the pilot: a first-time user finishes setup and a first test payment without help, and can decline with large text and without color.

## Positioning

The phone never decides what is approved. Every approval is bound to the board's physical buttons and PIN, and the content check belongs to an independent display; the phone guides the steps and shows progress. No other wallet app in this product line can claim that, because the board has no screen and the app is its only guide.

## Operating Context

- Navigation follows the IA: bottom tabs 지갑 · 결제 · 둘러보기 · 설정 (`docs/content/commercial/products/p02-user-app/ia.md`); the same board approval screen is used from every entry.
- The board is the NU54DK v2 (NU-54DK-C) with four buttons (SW1–SW4) and four LEDs. The app must show the board step, the sequence and the buttons whenever it waits on the board (`docs/content/commercial/screens/board.json`).
- Demo flows that must work: board connection and enrollment (W04), shop payment and receipt (W09–W11), balance, receive and send (W01, W07, W08), travel and DApp browsing (W13, T01).
- Networks are testnets (StableNet 8283 TestUSDC and others); nothing moves real money.

## Capabilities and Constraints

- React Native 0.87 app, Android first (Galaxy demo phone and emulator); iOS follows from the same code later.
- Personal and rental wallets share the app; private screens block screen capture.
- The verified wallet home needs a completed backup, which needs a second board and the HD build; the lab has one board, so this state cannot be reached on real hardware today.
- Open decisions (not settled; do not design as if decided):
  - **Testnet funds for visitors.** There is no faucet; TestUSDC can be minted only by its owner. With the rental route the operator funds one rental deposit; a per-visitor faucet is not planned.
- Decided 2026-11-23: **visitors use the rental route.** The board keeps one owner (the presenter). A visitor installs the app, opens a rental from the operator's QR and pays from the operator-funded rental deposit; the operator closes the rental afterwards (O09). Visitors do not enroll as owners.
  - **APK distribution by QR.** Signing, hosting and update policy for visitor installs are undecided.

## Brand Commitments

- Name in the app: NU Wallet. Korean first, English available.
- The screen design book is the visual reference (`docs/content/commercial/screens/book.html`, `wallet.png`, `personal.png`); tokens are mirrored in `src/ui/theme.ts`.
- Pretendard is the bundled typeface on Android (`third_party/pretendard`).

## Evidence on Hand

- Real board enrollment and owner registration on hardware, 2026-11-22 (`docs/content/commercial/screens/runtime-acceptance.md`).
- Screen design book with 86 pages and 461 states; runtime acceptance ledger at 447/461.
- No customer testimonials, metrics or production deployments exist; none may be invented.

## Product Principles

1. The board decides, the phone guides: every board wait shows the step and buttons, never a phone-side "looks right, approve".
2. One task per screen: tabs and pages follow the IA; developer and recovery tools stay out of the first screens.
3. A first-time user finishes alone: plain words, a visible next step and a recovery path on every state.
4. Demo truth: fixture or demo states are labeled as such and can never approve, sign or send.

## Accessibility & Inclusion

Large text up to 200%, screen readers (TalkBack first), decline possible without relying on color, reduced motion honored for board animations.
