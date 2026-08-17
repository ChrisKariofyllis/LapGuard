#!/bin/sh
# Cross-compile Linux amd64/arm64 binaries with the embedded dashboard.
# Does not publish a GitHub release.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$ROOT"

VERSION=$(sh ./scripts/version.sh "${1:-}")

if [ ! -f web/dist/index.html ]; then
	echo "web/dist is missing; run make build-web first" >&2
	exit 1
fi

rm -rf internal/webui/dist
mkdir -p internal/webui/dist
cp -a web/dist/. internal/webui/dist/

mkdir -p dist
export CGO_ENABLED=0
for arch in amd64 arm64; do
	out="dist/lapguard_${VERSION}_linux-${arch}"
	echo "building $out"
	GOOS=linux GOARCH="$arch" go build -tags embedui -trimpath -buildvcs=false \
		-ldflags "-s -w -X lapguard/internal/config.Version=${VERSION}" \
		-o "$out" ./cmd/lapguard
done

(
	cd dist
	sha256sum "lapguard_${VERSION}_linux-amd64" "lapguard_${VERSION}_linux-arm64" > SHA256SUMS
)

echo "artifacts in $ROOT/dist:"
cat dist/SHA256SUMS
