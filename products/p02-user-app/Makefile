# P02 renter phone app: React Native 0.87 + TypeScript, Kotlin and iOS Turbo Modules ([N25]).
.PHONY: setup build test lint run docker

setup:
	pnpm install

build:
	pnpm --filter @nu54/p02-renter typecheck

test:
	node --test ../../scripts/test-jest-isolation.test.cjs
	node ../../scripts/test-jest.cjs --runInBand

lint:
	pnpm --filter @nu54/p02-renter typecheck
	pnpm exec eslint src test

run:
	npx react-native run-android

docker:
	@echo "P02 is a mobile app; it has no container image"

.PHONY: test-ios-owner-registration
test-ios-owner-registration:
	../../scripts/ios/w0406-smoke/run-registration.sh "$(SIMULATOR_ID)" "$(EVIDENCE_DIR)"

.PHONY: test-ios-owner-backup
test-ios-owner-backup:
	../../scripts/ios/w0406-smoke/run-registration.sh "$(SIMULATOR_ID)" "$(EVIDENCE_DIR)" backup

.PHONY: test-ios-owner-backup-blocked
test-ios-owner-backup-blocked:
	../../scripts/ios/w0406-smoke/run-registration.sh "$(SIMULATOR_ID)" "$(EVIDENCE_DIR)" backup-blocked

.PHONY: test-ios-owner-replacement
test-ios-owner-replacement:
	../../scripts/ios/w0406-smoke/run-registration.sh "$(SIMULATOR_ID)" "$(EVIDENCE_DIR)" replacement

.PHONY: test-ios-owner-board-restore
test-ios-owner-board-restore:
	../../scripts/ios/w0406-smoke/run-registration.sh "$(SIMULATOR_ID)" "$(EVIDENCE_DIR)" board
