#!/bin/sh
# Validate a linux/amd64 LapGuard binary: build metadata + embedded UI.
# Does not require root, a battery, Docker, TLP, or external network.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$ROOT"

BIN=${1:-bin/lapguard}

test -s "$BIN"

echo "go version -m $BIN"
go version -m "$BIN"

go version -m "$BIN" | grep -q 'GOOS=linux'
go version -m "$BIN" | grep -q 'GOARCH=amd64'
go version -m "$BIN" | grep -q 'embedui'

if go version -m "$BIN" | grep -E 'vcs\.modified=true|-dirty'; then
	echo "binary must not contain vcs.modified=true or -dirty metadata" >&2
	exit 1
fi

port=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')
tmpdir=$(mktemp -d)
pid=
cleanup() {
	if [ -n "${pid:-}" ]; then
		kill "$pid" 2>/dev/null || true
		wait "$pid" 2>/dev/null || true
	fi
	rm -rf "$tmpdir"
}
trap cleanup EXIT INT TERM

"$BIN" \
	-web-dir none \
	-listen "127.0.0.1:${port}" \
	-provider mock \
	-config "$tmpdir/config.json" \
	-events-db "$tmpdir/events.db" \
	-sysfs-root "$ROOT/testdata/sysfs" \
	>"$tmpdir/lapguard.log" 2>&1 &
pid=$!

export LAPGUARD_VERIFY_URL="http://127.0.0.1:${port}/"
if ! python3 - <<'PY'
import os
import sys
import time
import urllib.error
import urllib.request

url = os.environ["LAPGUARD_VERIFY_URL"]
body = ""
status = 0
last_err = None
for _ in range(50):
    try:
        with urllib.request.urlopen(url, timeout=1) as resp:
            status = resp.getcode()
            body = resp.read().decode("utf-8", "replace")
        break
    except (urllib.error.URLError, TimeoutError, ConnectionError, OSError) as err:
        last_err = err
        time.sleep(0.1)
else:
    sys.stderr.write(f"server did not become ready: {last_err}\n")
    sys.exit(1)

if status != 200:
    sys.stderr.write(f"GET / returned HTTP {status}\n")
    sys.exit(1)
if "<title>LapGuard</title>" not in body:
    sys.stderr.write("GET / missing <title>LapGuard</title>\n")
    sys.exit(1)
if "/assets/" not in body:
    sys.stderr.write("GET / missing /assets/ reference\n")
    sys.exit(1)
print("embedded UI ok")
PY
then
	echo "lapguard log:" >&2
	cat "$tmpdir/lapguard.log" >&2 || true
	exit 1
fi
