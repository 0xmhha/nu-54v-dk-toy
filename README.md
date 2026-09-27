# NU-54V-DK Hardware Wallet and Stablecoin Payment System

A hardware wallet built on the NU-54V-DK board, and the stablecoin payment system around
it: a shop tablet app, a settlement smart contract, an operator tool and a receipt service.
The wallet is made for paying at a shop counter. It signs only payment approvals and
spending-limit changes, not arbitrary transactions.

A traveler rents the device. At checkout, the shop's tablet app connects to it over
Bluetooth Low Energy (BLE) and sends the order. The device checks that the shop is
registered, shows the amount on its screen, and waits for the traveler to press a button.
Only then does it sign the payment. The tablet submits the signed payment to a
settlement smart contract, which moves the stablecoin to the shop's payout address.

The signing key never leaves the device. A compromised tablet can only collect payments
that the traveler approved on the device, paid to a registered shop address, and only up
to the traveler's spending limits, which the contract enforces.

> **Status: early development.** This project runs on a public test network with test
> tokens only. The firmware, apps and contracts are incomplete and have not been audited.
> Do not use them with real funds.

## How it works

```text
 Traveler's device              Shop tablet app                  Test network
 (NU-54V-DK board)              (Android)                        (EVM chain)
 ┌──────────────────┐  BLE     ┌───────────────────┐  JSON-RPC   ┌─────────────────────┐
 │ shows amount     │◄────────►│ creates the order │────────────►│ settlement contract │
 │ button approval  │  order / │ submits payment   │             │ pays the shop       │
 │ signs (EIP-712)  │ signature│ shows the receipt │◄──────┐     └──────────┬──────────┘
 └────────▲─────────┘          └───────────────────┘       │                │ events
          │ BLE (setup)                                    │     ┌──────────▼──────────┐
 ┌────────┴─────────┐                                      └─────│ receipt indexer     │
 │ operator tool    │  registers shops, sets up rentals          └─────────────────────┘
 └──────────────────┘
```

- Payments are signed as [EIP-712](https://eips.ethereum.org/EIPS/eip-712) typed data,
  so the device, the apps and the contract all check the same fields.
- The message format, signature types and error codes are defined once in a JSON
  schema. Code for Go, TypeScript, Python and C is generated from that schema, and a
  shared set of test vectors checks that every implementation produces the same signatures.
- The target chain is the StableNet test network (chain ID 8283). For local work, a
  local EVM node (anvil) runs with the same chain ID.

## Repository layout

| Path | What it contains | Language |
|---|---|---|
| [`products/p01-device-firmware`](products/p01-device-firmware) | Device firmware for the NU-54V-DK board (Nordic nRF54L15) | C, Zephyr RTOS |
| [`products/p04-merchant-kiosk`](products/p04-merchant-kiosk) | Shop tablet app with a native BLE module | React Native, TypeScript, Kotlin, Swift |
| [`products/p05-operations-backoffice`](products/p05-operations-backoffice) | Operator tool: shop registration, device setup, rentals | Go, React |
| [`products/p06-stablenet-contracts`](products/p06-stablenet-contracts) | Settlement smart contract and signature types | Solidity, Foundry |
| [`products/p07-indexer`](products/p07-indexer) | Receipt service that reads settlement events | Go, PostgreSQL |
| [`products/p10-platform`](products/p10-platform) | Cross-language conformance tests | Python |
| [`packages/protocol`](packages/protocol) | Code generated from the protocol schema | Go, TypeScript, Python, C |
| [`sandbox`](sandbox) | Local Docker environment: EVM node and PostgreSQL | Docker Compose |
| [`docs`](docs) | Design notes, specifications and plans (working documents, mostly in Korean) | Markdown |

Other folders under `products/` are placeholders for later work.

## Getting started

### Prerequisites

| Tool | Version | Needed for |
|---|---|---|
| Go | 1.25+ | operator tool, indexer, generated Go code |
| Node.js and pnpm | Node 22, pnpm 10 | tablet app, operator web UI, generated TypeScript code |
| Python and [uv](https://docs.astral.sh/uv/) | Python 3.12+ | code generator, conformance tests |
| [Foundry](https://book.getfoundry.sh/) | stable | smart contracts |
| CMake | 3.20+ | firmware unit tests on your computer |
| Docker | with Compose | local sandbox |

Optional, depending on what you work on:

- **Firmware on hardware:** [nRF Connect SDK](https://www.nordicsemi.com/Products/Development-software/nRF-Connect-SDK)
  v3.4.1, installed with `nrfutil`, and an NU-54V-DK board.
- **Tablet app on a device:** Android SDK (and Xcode for iOS).

### Build and test

The root `Makefile` runs the same targets in every project.

```bash
git clone https://github.com/0xmhha/nu-54v-dk-toy.git
cd nu-54v-dk-toy

make setup      # install dependencies for all projects
make build      # build or generate code
make test       # unit tests and cross-language conformance tests
make lint       # formatting and static checks
```

To work on one project, pass its folder prefix with `P`:

```bash
make test P=p07          # products/p07-indexer only
make build P=protocol    # packages/protocol only
make run P=p05           # start the operator tool
```

### Local sandbox

```bash
make sandbox-up      # EVM node on :8545 (chain ID 8283), PostgreSQL on :55432
make sandbox-down
```

The sandbox passwords are for local use only. Each project has an `.env.example` file
that lists its settings.

### Firmware

```bash
cd products/p01-device-firmware
make test            # unit tests on your computer, no SDK needed
make fw              # build for the NU-54V-DK board
make flash           # flash over the board's USB debugger
```

## Documentation

- [Payment protocol](docs/content/specifications/protocol/payment-protocol.md): BLE
  messages, signature types and error codes. The machine-readable versions are
  [`payment-protocol.schema.json`](docs/content/specifications/protocol/payment-protocol.schema.json)
  and [`eip712-vectors.json`](docs/content/specifications/protocol/eip712-vectors.json).
- Each project's README describes its folders and commands.
- The [`docs`](docs) folder holds the team's working design documents. They are written
  in Korean and use internal planning terms. Start from the
  [development overview](docs/development-overview.md) if you want to read them.

## Contributing

Contributions are welcome. Please read [CONTRIBUTING.md](CONTRIBUTING.md) before you open
a pull request. In short:

- Open an issue first for larger changes.
- Use [Conventional Commits](https://www.conventionalcommits.org/) and sign off each
  commit (`git commit -s`) under the [Developer Certificate of Origin](https://developercertificate.org/).
- Run `make lint test` before you push.

Please do not report security problems in public issues. See
[CONTRIBUTING.md](CONTRIBUTING.md#reporting-security-issues).

## License

This project is licensed under the [Apache License 2.0](LICENSE).

Some folders contain third-party code under their own licenses. Each one has a
`VENDORED.md` file that records its source and license:

- [`products/p01-device-firmware/boards/nucode/nu54v_dk`](products/p01-device-firmware/boards/nucode/nu54v_dk): NU-54V-DK board support files (MIT)
- [`products/p06-stablenet-contracts/lib/forge-std`](products/p06-stablenet-contracts/lib/forge-std): Foundry standard library (MIT or Apache-2.0)
