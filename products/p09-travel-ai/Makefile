.PHONY: setup build test lint run migrate travel-integration
setup:
	go mod download
build:
	go build ./...
test:
	go test ./...
lint:
	test -z "$$(gofmt -l cmd internal)"
	go vet ./...
run:
	go run ./cmd/placesd -trust-key "$${P09_TRUST_KEY:?set P09_TRUST_KEY}"
migrate:
	python3 ../p10-platform/database/migrate.py p09
travel-integration:
	python3 ../../scripts/test_travel_booking_integration.py

.PHONY: docker release-candidate
docker:
	docker build -f Dockerfile -t nu54-placesd:local ../..
release-candidate:
	python3 ../../scripts/package_placesd.py

.PHONY: run-private
run-private:
	go run ./cmd/recordsd -cert "$${P09_TLS_CERT:?set P09_TLS_CERT}" -key "$${P09_TLS_KEY:?set P09_TLS_KEY}" \
		-client-ca "$${P09_CLIENT_CA:?set P09_CLIENT_CA}" -grants "$${P09_PRIVATE_GRANTS:?set P09_PRIVATE_GRANTS}"
