# P01 payment signer firmware: C / Zephyr, NCS v3.4.1 ([D05][N25]).
# build/test/lint work on the host without the NCS toolchain (core/ only);
# fw/flash/reset need nrfutil + the NCS toolchain (see scripts/fw.py).
HOST_BUILD ?= out/host

.PHONY: setup build test lint run docker fw flash reset clean

setup:
	@command -v cmake >/dev/null || { echo "cmake is required"; exit 1; }
	@command -v nrfutil >/dev/null || test -x $$HOME/.local/bin/nrfutil || echo "note: nrfutil not found; fw/flash targets need it"

build:
	cmake -S test -B $(HOST_BUILD) -DCMAKE_BUILD_TYPE=Debug >/dev/null
	cmake --build $(HOST_BUILD)

test: build
	ctest --test-dir $(HOST_BUILD) --output-on-failure

# Host build already uses -Wall -Wextra -Werror; this also checks the helper scripts.
lint: build
	python3 -m py_compile scripts/fw.py

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
