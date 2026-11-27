# P10 platform: shared-vector conformance and week-12 acceptance ([N18][N21][N25]).
.PHONY: setup build test lint run docker baseline baseline-sbom baseline-regression baseline-flow baseline-restart

POSTGRES_BIN ?=
postgres_option = $(if $(POSTGRES_BIN),--postgres-bin "$(POSTGRES_BIN)",)

setup build lint run docker:
	@echo "P10 has no $@ step; its test target runs the conformance harness and the acceptance tool tests"

test:
	python3 harness/run_conformance.py
	python3 acceptance/test/test_w12.py

baseline:
	python3 baseline/verify.py
	python3 baseline/test_baseline.py
	python3 baseline/test_inventory.py
	python3 baseline/test_regression.py

baseline-sbom:
	@python3 baseline/inventory.py

baseline-regression:
	python3 baseline/regression.py all $(postgres_option)

baseline-flow:
	python3 baseline/regression.py flow

baseline-restart:
	python3 baseline/regression.py restart $(postgres_option)

.PHONY: migrate
migrate:
	python3 database/migrate.py p10

.PHONY: release-test sdk-candidates

release-test:
	python3 release/test_inputs.py
	python3 release/test_release.py
	python3 release/test_ios_profile.py
	python3 release/test_android_profile.py
	python3 release/test_sdk_artifacts.py

sdk-candidates:
	test -n "$(SDK_OUT)"
	python3 release/build_sdk.py --smoke --out "$(SDK_OUT)"

.PHONY: sdk-mobile-candidate
sdk-mobile-candidate:
	@test -n "$(SDK_OUT)" || (echo 'Set SDK_OUT to a new output directory outside the checkout' && exit 2)
	python3 release/build_mobile_sdk.py --out "$(SDK_OUT)" $(MOBILE_SDK_FLAGS)
