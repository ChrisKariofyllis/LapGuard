export PATH := $(HOME)/.local/go/bin:$(HOME)/.local/node/bin:$(PATH)

.PHONY: test build run web-install web-dev web-build tidy

test:
	go test ./...

tidy:
	go mod tidy

run:
	go run ./cmd/lapguard

run-fixture:
	go run ./cmd/lapguard -provider sysfs -sysfs-root testdata/sysfs

web-install:
	cd web && npm install

web-dev:
	cd web && npm run dev

web-build:
	cd web && npm run build

build: web-build
	mkdir -p bin
	go build -o bin/lapguard ./cmd/lapguard
