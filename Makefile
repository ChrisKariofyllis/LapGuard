export PATH := $(HOME)/.local/go/bin:$(HOME)/.local/node/bin:$(PATH)

# Local: "dev", or the exact git tag when HEAD is tagged and the tree is clean.
# Override: make build VERSION=0.6.0-alpha
# Release tags: the workflow passes VERSION from the v* tag. Never embed "-dirty".
override VERSION := $(shell sh ./scripts/version.sh "$(VERSION)")
LDFLAGS := -s -w -X lapguard/internal/config.Version=$(VERSION)
GO_BUILD := CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags "$(LDFLAGS)"

.PHONY: test lint build-web web-build stage-web build release-build clean tidy run run-fixture web-install web-dev

test:
	go test ./...

lint:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed on:"; echo "$$unformatted"; exit 1; \
	fi
	go vet ./...

build-web:
	cd web && npm ci --no-audit --no-fund && npm run build

web-build: build-web

stage-web: build-web
	rm -rf internal/webui/dist
	mkdir -p internal/webui/dist
	cp -a web/dist/. internal/webui/dist/

build: stage-web
	mkdir -p bin
	$(GO_BUILD) -tags embedui -o bin/lapguard ./cmd/lapguard

release-build: stage-web
	./scripts/release-build.sh "$(VERSION)"

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

clean:
	rm -rf bin dist web/dist web/.vite
	rm -rf internal/webui/dist
	mkdir -p internal/webui/dist
	touch internal/webui/dist/.gitkeep
