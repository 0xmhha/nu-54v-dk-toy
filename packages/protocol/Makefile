# packages/protocol: shared protocol types generated from the schema ([N21][N25]).
.PHONY: setup build test lint run docker

setup:
	uv sync --all-packages

build:
	python3 schema-gen/generate.py
	python3 cbor-gen/generate.py

test:
	python3 schema-gen/generate.py --check
	python3 cbor-gen/generate.py --check
	cd go && go test ./...
	pnpm --filter @nu54/protocol test
	uv run --package nu54-protocol pytest -q python/tests

lint:
	test -z "$$(gofmt -l go)"
	cd go && go vet ./...
	pnpm --filter @nu54/protocol typecheck

run docker:
	@echo "packages/protocol has no $@ target"
