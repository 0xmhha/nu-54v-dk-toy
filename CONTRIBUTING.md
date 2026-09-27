# Contributing

Thank you for your interest in this project. This guide explains how to report problems,
set up a development environment and send changes.

## Reporting bugs and asking questions

Open a [GitHub issue](https://github.com/0xmhha/nu-54v-dk-toy/issues). Please include:

- what you did, what you expected, and what happened instead;
- the folder you were working in (for example `products/p07-indexer`);
- tool versions (`go version`, `node --version`, `forge --version`, and for firmware the
  nRF Connect SDK version and board revision);
- logs or error output, with any private data removed.

For a larger change, open an issue first so we can agree on the approach before you
write the code.

## Reporting security issues

Do not open a public issue for a security problem. Report it privately through
[GitHub security advisories](https://github.com/0xmhha/nu-54v-dk-toy/security/advisories/new).
Include the steps to reproduce and the impact you expect. We will reply in the advisory.

## Development setup

Install the tools listed in the [README](README.md#prerequisites), then run from the
repository root:

```bash
make setup
make build
make test
```

Every project folder has a `Makefile` with the same targets:

| Target | What it does |
|---|---|
| `setup` | Install dependencies |
| `build` | Compile, or generate code |
| `test` | Run unit tests |
| `lint` | Check formatting and run static analysis |
| `run` | Start the project locally, where that makes sense |
| `docker` | Build the container image, for projects that are deployed |

Run a target for one project with `make <target> P=<folder prefix>`, for example
`make test P=p06`.

## Making changes

- Keep each pull request focused on one change. Split unrelated changes.
- Add or update tests with the code they cover.
- The message format and signature types are defined in
  [`payment-protocol.schema.json`](docs/content/specifications/protocol/payment-protocol.schema.json).
  If you change the schema, run `make build P=protocol` to regenerate the code in
  `packages/protocol`, and commit the schema and the generated files together. Every
  implementation must still pass `make test P=p10`, which checks the shared signature
  test vectors.
- Do not commit private keys, mnemonics, API secrets or real user data. Put local
  settings in a `.env` file (ignored by Git) and document them in `.env.example`.
- Code from other projects must keep its license. Add a `VENDORED.md` file that records
  the source, the version or commit, and the license.

## Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org/):

```text
<type>(<scope>): <summary in the imperative mood>
```

| Type | Use it for |
|---|---|
| `feat` | A new feature |
| `fix` | A bug fix |
| `docs` | Documentation only |
| `refactor` | A code change that does not change behavior |
| `test` | Adding or fixing tests |
| `build` | Build system or dependencies |
| `ci` | Continuous integration |
| `chore` | Other maintenance |

The scope names the part of the repository you changed:

| Scope | Folder |
|---|---|
| `firmware` | `products/p01-device-firmware` |
| `kiosk` | `products/p04-merchant-kiosk` |
| `backoffice` | `products/p05-operations-backoffice` |
| `contracts` | `products/p06-stablenet-contracts` |
| `indexer` | `products/p07-indexer` |
| `conformance` | `products/p10-platform` |
| `protocol` | `packages/protocol` and the protocol schema |
| `sandbox` | `sandbox` |
| `docs` | `docs` |

Keep the summary under 72 characters and do not end it with a period. Use the body to
explain why the change is needed.

```text
fix(indexer): resume from the last finalized block after a restart
```

### Sign-off

Every commit must be signed off under the
[Developer Certificate of Origin](https://developercertificate.org/). The sign-off states
that you have the right to submit the code under the project license.

```bash
git commit -s -m "fix(indexer): resume from the last finalized block after a restart"
```

## Pull requests

1. Fork the repository and create a branch from `main`, for example
   `fix/indexer-restart`.
2. Run `make lint test` from the repository root.
3. Open a pull request and fill in the template: the problem, the resulting behavior,
   the affected folders, and how you tested the change.
4. Continuous integration must pass before the pull request can be merged.

## License

By contributing, you agree that your contributions are licensed under the
[Apache License 2.0](LICENSE).
