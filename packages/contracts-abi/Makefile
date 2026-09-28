# packages/contracts-abi: settlement contract ABIs for Go and TypeScript consumers.
.PHONY: setup build test lint run docker

setup:
	cd go && go mod download

build:
	python3 tools/generate.py

test:
	python3 tools/generate.py --check
	cd go && go build ./...

lint:
	cd go && go vet ./...
	pnpm --filter @nu54/contracts-abi typecheck

run docker:
	@echo "packages/contracts-abi has no $@ target"
