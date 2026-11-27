# P06 settlement contracts: Solidity + Foundry ([N07][N25]).
CONTRACT_FORGE := python3 ../../scripts/contract_forge.py p06

.PHONY: setup build test lint run docker snapshot

setup:
	forge --version

build:
	$(CONTRACT_FORGE) build

# Gas snapshots cover deterministic v1 and v2 scenarios. Fuzz and invariant gas varies per run.
SNAPSHOT_FILTER := --no-match-test testFuzz --no-match-contract SettlementInvariantTest

test:
	exec python3 -B ../../scripts/contract_test_workspace.py p06 -- $(SNAPSHOT_FILTER)

snapshot:
	$(CONTRACT_FORGE) snapshot $(SNAPSHOT_FILTER)

lint:
	forge fmt --check src test script

run:
	anvil --chain-id 8283 --block-time 1

docker:
	@echo "P06 deploys to a chain; it has no container image"
