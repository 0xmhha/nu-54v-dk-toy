# P05 operations back-office: Go core + opsctl now, opsd mTLS and React operations web ([N20][N25]).
.PHONY: setup build test lint run docker

setup:
	go mod download
	pnpm install

build:
	go build -o bin/opsctl ./cmd/opsctl
	go build -o bin/opsd ./cmd/opsd
	go build -o bin/rental-collect ./cmd/rental-collect
	pnpm --filter @nu54/p05-backoffice-web build

test:
	go test ./...
	pnpm --filter @nu54/p05-backoffice-web test

lint:
	test -z "$$(gofmt -l cmd internal)"
	go vet ./...
	pnpm --filter @nu54/p05-backoffice-web typecheck

run:
	go run ./cmd/opsd

docker:
	docker build -f Dockerfile -t nu54/p05-opsd ../..

.PHONY: migrate
migrate:
	python3 ../p10-platform/database/migrate.py p05

.PHONY: commerce-test

commerce-test:
	@test -n "$$P05_COMMERCE_TEST_DATABASE_URL" || { echo "P05_COMMERCE_TEST_DATABASE_URL must name a local migrated nu54_commerce_* scratch DB"; exit 1; }
	go test -count=1 -v ./internal/commerce

.PHONY: rental-browser-test
rental-browser-test:
	sh test/settlement/anvil.sh --rental-browser
