# P02 renter phone app: React Native 0.87 + TypeScript, Kotlin Turbo Modules, Android only ([N25]).
.PHONY: setup build test lint run docker

setup:
	pnpm install

build:
	pnpm --filter @nu54/p02-renter typecheck

test:
	npx jest

lint:
	pnpm --filter @nu54/p02-renter typecheck
	npx eslint src test

run:
	npx react-native run-android

docker:
	@echo "P02 is a mobile app; it has no container image"
