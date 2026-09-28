# P06 settlement contracts: Solidity + Foundry ([N07][N25]).
.PHONY: setup build test lint run docker snapshot

setup:
	forge --version

build:
	forge build

# The gas snapshot excludes fuzz and invariant tests, whose gas varies run to run.
SNAPSHOT_FILTER := --no-match-test testFuzz --no-match-contract SettlementInvariantTest

test:
	forge test
	forge snapshot --check $(SNAPSHOT_FILTER)

snapshot:
	forge snapshot $(SNAPSHOT_FILTER)

lint:
	forge fmt --check src test script

run:
	anvil --chain-id 8283 --block-time 1

docker:
	@echo "P06 deploys to a chain; it has no container image"
