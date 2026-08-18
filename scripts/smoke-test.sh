#!/bin/sh
# Public-alpha smoke test. Never enables real actions and never runs
# poweroff, shutdown, reboot, sync, or Docker.
#
# Usage:
#   sh scripts/smoke-test.sh [bin/lapguard]
#   LAPGUARD_URL=http://127.0.0.1:8585 sh scripts/smoke-test.sh
#
# Optional: LAPGUARD_TOKEN=...  Bearer token for POST preview on a live daemon.
# Dangerous POSTs (poweroff / docker-drain) run only against a local mock
# process started by this script.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$ROOT"

BIN=${1:-}
URL=${LAPGUARD_URL:-}
TOKEN=${LAPGUARD_TOKEN:-}
started=0
pid=
tmpdir=
cleanup() {
	if [ "$started" -eq 1 ] && [ -n "${pid:-}" ]; then
		kill "$pid" 2>/dev/null || true
		wait "$pid" 2>/dev/null || true
	fi
	if [ -n "${tmpdir:-}" ]; then
		rm -rf "$tmpdir"
	fi
}
trap cleanup EXIT INT TERM

if [ -z "$URL" ]; then
	if [ -z "$BIN" ]; then
		if [ -x "$ROOT/bin/lapguard" ]; then
			BIN=$ROOT/bin/lapguard
		else
			echo "usage: $0 [path-to-lapguard]  (or set LAPGUARD_URL)" >&2
			exit 2
		fi
	fi
	test -s "$BIN"
	port=$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')
	tmpdir=$(mktemp -d)
	"$BIN" \
		-web-dir none \
		-listen "127.0.0.1:${port}" \
		-provider mock \
		-config "$tmpdir/config.json" \
		-events-db "$tmpdir/events.db" \
		>"$tmpdir/lapguard.log" 2>&1 &
	pid=$!
	started=1
	URL="http://127.0.0.1:${port}"
	TOKEN=
fi

URL=${URL%/}
export LAPGUARD_SMOKE_URL="$URL"
export LAPGUARD_SMOKE_TOKEN="$TOKEN"
export LAPGUARD_SMOKE_LOCAL="$started"
export LAPGUARD_SMOKE_LOG="${tmpdir:-}/lapguard.log"

if ! python3 - <<'PY'
import json
import os
import sys
import time
import urllib.error
import urllib.request

base = os.environ["LAPGUARD_SMOKE_URL"]
token = os.environ.get("LAPGUARD_SMOKE_TOKEN") or ""
local = os.environ.get("LAPGUARD_SMOKE_LOCAL") == "1"
log_path = os.environ.get("LAPGUARD_SMOKE_LOG") or ""

def request(method, path, body=None):
    url = base + path
    data = None
    headers = {"Accept": "application/json"}
    if body is not None:
        data = json.dumps(body).encode("utf-8")
        headers["Content-Type"] = "application/json"
        if token:
            headers["Authorization"] = "Bearer " + token
    req = urllib.request.Request(url, data=data, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=5) as resp:
            raw = resp.read().decode("utf-8", "replace")
            return resp.getcode(), raw
    except urllib.error.HTTPError as err:
        raw = err.read().decode("utf-8", "replace")
        return err.code, raw

def wait_ready():
    last = None
    for _ in range(50):
        try:
            code, body = request("GET", "/api/v1/healthz")
            if code == 200:
                return body
        except (urllib.error.URLError, TimeoutError, ConnectionError, OSError) as err:
            last = err
        time.sleep(0.1)
    sys.stderr.write(f"server did not become ready: {last}\n")
    sys.exit(1)

health = wait_ready()
health_obj = json.loads(health)
if health_obj.get("status") != "ok":
    sys.stderr.write(f"healthz unexpected: {health}\n")
    sys.exit(1)

leaks = ("systemctl", "docker stop", "$(", "/bin/sh", "token_hash")

def check_get(path, *needles):
    code, body = request("GET", path)
    if code != 200:
        sys.stderr.write(f"GET {path} HTTP {code}: {body}\n")
        sys.exit(1)
    for needle in needles:
        if needle not in body:
            sys.stderr.write(f"GET {path} missing {needle!r}: {body}\n")
            sys.exit(1)
    for leak in leaks:
        if leak in body:
            sys.stderr.write(f"GET {path} leaked {leak!r}\n")
            sys.exit(1)
    return json.loads(body)

check_get("/api/v1/healthz", '"status":"ok"')
tel = check_get("/api/v1/telemetry", "battery")
if "battery" not in tel:
    sys.stderr.write("telemetry missing battery\n")
    sys.exit(1)
check_get("/api/v1/capabilities", "feature_flags")
check_get("/api/v1/discover", "available_fields")

status = check_get("/api/v1/actions/status", "real_enabled", "safety_dry_run")
if status.get("real_enabled") is not False:
    sys.stderr.write("real_enabled must be false for this smoke test\n")
    sys.exit(1)
if status.get("safety_dry_run") is not True:
    sys.stderr.write("safety_dry_run must be true for this smoke test\n")
    sys.exit(1)
if status.get("commands_executed") is not False:
    sys.stderr.write("actions/status claimed commands_executed\n")
    sys.exit(1)
if status.get("executor") != "recording":
    sys.stderr.write(f"executor {status.get('executor')!r}, want recording\n")
    sys.exit(1)

code, preview = request("POST", "/api/v1/actions/preview", {})
if code == 401 and not token:
    sys.stderr.write("POST preview returned 401; set LAPGUARD_TOKEN if auth is enabled\n")
    sys.exit(1)
if code != 200:
    sys.stderr.write(f"POST preview HTTP {code}: {preview}\n")
    sys.exit(1)
preview_obj = json.loads(preview)
if preview_obj.get("commands_executed") is not False:
    sys.stderr.write("preview claimed commands_executed\n")
    sys.exit(1)
for leak in leaks:
    if leak in preview:
        sys.stderr.write(f"preview leaked {leak!r}\n")
        sys.exit(1)

if local:
    for path, body in (
        ("/api/v1/actions/poweroff", {"confirm": "POWER_OFF"}),
        ("/api/v1/actions/docker-drain", {"confirm": "STOP_DOCKER"}),
    ):
        code, raw = request("POST", path, body)
        if code != 409:
            sys.stderr.write(f"POST {path} HTTP {code}, want 409: {raw}\n")
            sys.exit(1)
        obj = json.loads(raw)
        if obj.get("commands_executed") is True:
            sys.stderr.write(f"{path} claimed commands_executed\n")
            sys.exit(1)
        if obj.get("ok") is True:
            sys.stderr.write(f"{path} succeeded; smoke test must not execute actions\n")
            sys.exit(1)
        for leak in ("systemctl", "docker stop", "$(", "/bin/sh"):
            if leak in raw:
                sys.stderr.write(f"{path} leaked {leak!r}\n")
                sys.exit(1)
    if log_path:
        log = open(log_path, encoding="utf-8", errors="replace").read()
        for leak in ("systemctl poweroff", "docker stop", "/bin/sh -c"):
            if leak in log:
                sys.stderr.write(f"log mentioned executed command {leak!r}\n")
                sys.exit(1)

print("smoke test ok")
print(f"url={base}")
print("real_enabled=false")
print("safety_dry_run=true")
print("commands_executed=false")
PY
then
	if [ "$started" -eq 1 ] && [ -n "${tmpdir:-}" ]; then
		echo "lapguard log:" >&2
		cat "$tmpdir/lapguard.log" >&2 || true
	fi
	exit 1
fi
