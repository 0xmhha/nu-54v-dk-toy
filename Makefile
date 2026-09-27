# Monorepo entry point ([N25]).
#
# Every product and shared package has a Makefile with the same targets:
#   setup  install tools and dependencies
#   build  compile or generate
#   test   run unit and conformance tests
#   lint   format and static checks
#   run    start the product locally (where it makes sense)
#   docker build the container image (deployable products only)
#
# Usage:
#   make test                # every project
#   make test P=p05          # one product (matches products/p05-*)
#   make build P=protocol    # one shared package (packages/protocol)
#   make sandbox-up          # local docker sandbox (anvil 8283, PostgreSQL)
#   make docs-check          # design register validators and link check

SHELL := /bin/bash

PACKAGES := packages/protocol
PRODUCTS := products/p01-device-firmware products/p04-merchant-kiosk \
            products/p05-operations-backoffice products/p06-stablenet-contracts \
            products/p07-indexer products/p10-platform
PROJECTS := $(PACKAGES) $(PRODUCTS)

ifdef P
PROJECTS := $(foreach p,$(PROJECTS),$(if $(filter $(P) $(P)-%,$(notdir $(p))),$(p)))
ifeq ($(PROJECTS),)
$(error no project matches P=$(P))
endif
endif

TARGETS := setup build test lint run docker

.PHONY: $(TARGETS) help sandbox-up sandbox-down docs-check conformance

help:
	@sed -n '2,18p' Makefile | sed 's/^# \{0,1\}//'
	@echo "projects: $(PROJECTS)"

setup:
	pnpm install
	@for p in $(PROJECTS); do echo "== $$p: setup"; $(MAKE) -C $$p setup || exit 1; done

build test lint:
	@for p in $(PROJECTS); do echo "== $$p: $@"; $(MAKE) -C $$p $@ || exit 1; done

run docker:
	@test -n "$(P)" || { echo "make $@ needs P=<product>"; exit 2; }
	@for p in $(PROJECTS); do $(MAKE) -C $$p $@ || exit 1; done

conformance:
	python3 products/p10-platform/harness/run_conformance.py

sandbox-up:
	docker compose -f sandbox/compose.yaml up -d

sandbox-down:
	docker compose -f sandbox/compose.yaml down

docs-check:
	python3 docs/content/planning/validate_design_freeze.py >/dev/null
	python3 docs/content/planning/validate_product_wbs.py >/dev/null
	python3 docs/content/planning/validate_design_freeze_02.py --check all
	python3 docs/content/planning/validate_design_freeze_02.py --self-test
	python3 scripts/check_markdown_links.py
