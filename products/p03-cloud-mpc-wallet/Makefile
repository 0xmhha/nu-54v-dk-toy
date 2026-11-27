.PHONY: setup build test lint run real-test db-test

setup:
	go mod download

build:
	go build ./cmd/...

test:
	go test ./...

lint:
	go vet ./...

run:
	go run ./cmd/mpc-coordinator -config "$(P03_CONFIG)"

real-test:
	NU54_P03_REAL_DKG=1 go test ./internal/ceremony -run '^TestRealCeremonies$$' -count=1 -timeout 30m

db-test:
	python3 test/run_postgres.py

.PHONY: mobile-android mobile-ios-simulator

# Set P03_MOBILE_OUT to an absolute directory for native build output.
mobile-android:
	test -n "$(ANDROID_NDK_CLANG)" && test -n "$(P03_MOBILE_OUT)"
	GOOS=android GOARCH=arm64 CGO_ENABLED=1 CC="$(ANDROID_NDK_CLANG)" go build -buildmode=c-shared -o "$(P03_MOBILE_OUT)/android/arm64-v8a/libnu54_p03.so" ./cmd/mpc-mobile

mobile-ios-simulator:
	test -n "$(P03_MOBILE_OUT)"
	GOOS=ios GOARCH=arm64 CGO_ENABLED=1 CC="$$(go env GOROOT)/misc/ios/clangwrap.sh" go build -buildmode=c-archive -o "$(P03_MOBILE_OUT)/ios-simulator/libnu54_p03.a" ./cmd/mpc-mobile

.PHONY: mobile-ios-device

mobile-ios-device:
	test -n "$(P03_MOBILE_OUT)"
	GOOS=ios GOARCH=arm64 CGO_ENABLED=1 CC="$(CURDIR)/test/ios-device-clang.sh" go build -buildmode=c-archive -o "$(P03_MOBILE_OUT)/ios-device/libnu54_p03.a" ./cmd/mpc-mobile
