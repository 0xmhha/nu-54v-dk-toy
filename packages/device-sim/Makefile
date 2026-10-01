# packages/device-sim: software payment device for kiosk and firmware development (TypeScript).
.PHONY: setup build test lint run docker

setup:
	pnpm install

build lint:
	pnpm --filter @nu54/device-sim typecheck

test:
	pnpm --filter @nu54/device-sim test
	node --experimental-strip-types --no-warnings scripts/gen-session-vectors.ts --check

run:
	@echo "rehearsal: node --experimental-strip-types scripts/rehearse.ts --attestation <file> --anchor <file>"

docker:
	@echo "packages/device-sim has no container image"
