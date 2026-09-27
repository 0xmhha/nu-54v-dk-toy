# P07 minimal indexer: Go + PostgreSQL ([N19][N25]).
.PHONY: setup build test lint run docker

setup:
	go mod download

build:
	go build -o bin/indexer ./cmd/indexer

test:
	go test ./...

lint:
	test -z "$$(gofmt -l cmd internal)"
	go vet ./...

run:
	go run ./cmd/indexer

docker:
	docker build -f Dockerfile -t nu54/p07-indexer ../..
