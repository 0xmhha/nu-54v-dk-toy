# P04 merchant kiosk: React Native 0.87 + TypeScript, Kotlin/Swift Turbo Modules ([N25]).
.PHONY: setup build test lint run docker

setup:
	pnpm install

build:
	pnpm --filter @nu54/p04-kiosk typecheck

test:
	npx jest

lint:
	pnpm --filter @nu54/p04-kiosk typecheck
	npx eslint src test

run:
	npx react-native run-android

docker:
	@echo "P04 is a mobile app; it has no container image"
