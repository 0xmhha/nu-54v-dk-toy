# P01 payment signer firmware: C / Zephyr, NCS v3.4.2 ([D05][N25]).
# build/test/lint work on the host without the NCS toolchain (core/ only);
# fw/flash/reset need nrfutil + the NCS toolchain (see scripts/fw.py).
HOST_BUILD ?= out/host

.PHONY: setup build test _host-test lint run docker fw flash reset clean

setup:
	@command -v cmake >/dev/null || { echo "cmake is required"; exit 1; }
	@command -v nrfutil >/dev/null || test -x $$HOME/.local/bin/nrfutil || echo "note: nrfutil not found; fw/flash targets need it"

build:
	cmake -S test -B "$(HOST_BUILD)" -DCMAKE_BUILD_TYPE=Debug >/dev/null
	cmake --build "$(HOST_BUILD)"

test:
	exec python3 -B ../../scripts/host_test_workspace.py firmware "$(MAKE)" "$(origin HOST_BUILD)" "$(HOST_BUILD)"

_host-test: build
	ctest --test-dir "$(HOST_BUILD)" --output-on-failure
	python3 scripts/test_fw_release.py
	python3 scripts/test_fw_secure_key.py
	python3 -m unittest discover -s scripts -p test_verify_board_release.py
	go run test/evm_vectors.go "$(HOST_BUILD)/test_evm"
	python3 test/test_evm_service.py
	NU54_WALLET_CHAIN_HOST="$(HOST_BUILD)/libwallet_chain_node_oracle.so" python3 test/test_commissioned_chains.py
	python3 test/test_market_grant.py
	python3 test/test_operator_reset.py "$(HOST_BUILD)/spending_operator_host"
	python3 test/test_led_patterns.py
	python3 test/test_owner_policy_display.py
	python3 test/test_owner_finality_grant.py
	python3 test/test_btc_vectors.py
	python3 test/test_solana_vectors.py
	python3 test/test_xrpl_vectors.py

# Host build already uses -Wall -Wextra -Werror; this also checks the helper scripts.
lint: build
	python3 -m py_compile scripts/fw.py tools/bringup/*.py

run: flash

docker:
	@echo "p01: firmware has no container image; use 'make fw' and 'make flash'"

fw:
	python3 scripts/fw.py build

flash:
	python3 scripts/fw.py flash

reset:
	python3 scripts/fw.py reset

clean:
	rm -rf build out
