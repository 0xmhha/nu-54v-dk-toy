# P10 platform: shared-vector conformance and week-12 acceptance ([N18][N21][N25]).
.PHONY: setup build test lint run docker

setup build lint run docker:
	@echo "P10 has no $@ step; its test target runs the conformance harness and the acceptance tool tests"

test:
	python3 harness/run_conformance.py
	python3 acceptance/test/test_w12.py
