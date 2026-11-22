SHELL := /bin/bash

CONTRACT_FORGE := python3 ../../scripts/contract_forge.py p08

.PHONY: setup build test lint run migrate market-integration
setup:
	go mod download
	$(CONTRACT_FORGE) build --quiet
build:
	$(CONTRACT_FORGE) build --quiet
	go build ./...
test:
	exec python3 -B ../../scripts/contract_test_workspace.py p08
lint:
	forge fmt --check
	test -z "$$(gofmt -l cmd internal)"
	go vet ./...
run:
	go run ./cmd/marketd serve
migrate:
	python3 ../p10-platform/database/migrate.py p08
market-integration:
	bash test/anvil.sh -run '^TestIsolatedMarketExecutionAndKeeperRestart$$' -v
