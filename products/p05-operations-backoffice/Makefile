# P05 operations back-office: Go core + opsctl now, opsd API and React web next cycle ([N20][N25]).
.PHONY: setup build test lint run docker

setup:
	go mod download
	pnpm install

build:
	go build -o bin/opsctl ./cmd/opsctl
	go build -o bin/opsd ./cmd/opsd
	pnpm --filter @nu54/p05-backoffice-web build

test:
	go test ./...

lint:
	test -z "$$(gofmt -l cmd internal)"
	go vet ./...
	pnpm --filter @nu54/p05-backoffice-web typecheck

run:
	go run ./cmd/opsd

docker:
	docker build -f Dockerfile -t nu54/p05-opsd ../..
