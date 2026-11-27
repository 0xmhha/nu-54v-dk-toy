# packages/protocol: shared protocol types generated from the schema ([N21][N25]).
.PHONY: setup build test test-transport-v2 lint run docker

setup:
	uv sync --all-packages

build:
	python3 v2/generate_services.py
	python3 schema-gen/generate.py
	python3 cbor-gen/generate.py
	python3 frame-gen/generate.py
	python3 v2/generate.py

test: test-transport-v2
	python3 -B schema-gen/generate.py --check
	python3 -B cbor-gen/generate.py --check
	python3 -B frame-gen/generate.py --check
	python3 -B v2/generate.py --check
	sh v2/testdata/test-c.sh
	cd go && go test ./...
	pnpm --filter @nu54/protocol test
	exec python3 -B ../../scripts/host_test_workspace.py protocol-python

test-transport-v2:
	sh v2/testdata/test-transport.sh

lint:
	test -z "$$(gofmt -l go)"
	cd go && go vet ./...
	pnpm --filter @nu54/protocol typecheck

run docker:
	@echo "packages/protocol has no $@ target"
