# P06 settlement contracts: Solidity + Foundry ([N07][N25]).
.PHONY: setup build test lint run docker

setup:
	forge --version

build:
	forge build

test:
	forge test

lint:
	forge fmt --check src test script

run:
	anvil --chain-id 8283 --block-time 1

docker:
	@echo "P06 deploys to a chain; it has no container image"
