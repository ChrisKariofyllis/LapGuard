# Contributing to LapGuard

LapGuard is an open-source **alpha**. The battery safety controller is dry-run
only: do not implement real Docker stop or host shutdown unless a maintainer
explicitly asks for that work.

## Development setup

You do not need a laptop battery, Docker, root, or TLP. The development box in
this project is an HP ProDesk without a pack; `provider=auto` falls back to mock
telemetry. Tests use fixtures under `testdata/sysfs` and in-memory trees in
`internal/discovery/laptops_test.go`.

Requirements:

- Go (see `go.mod`)
- Node 22+ (dashboard only)
- Make (optional)

```bash
export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$PATH"
go test ./...
cd web && npm ci && npm run dev
```

In another terminal:

```bash
go run ./cmd/lapguard
```

Open `http://127.0.0.1:5173`. Vite proxies `/api` to `127.0.0.1:8585`.

Sysfs fixture (no live hardware):

```bash
go run ./cmd/lapguard -provider sysfs -sysfs-root testdata/sysfs
```

Release-style binary with the dashboard embedded:

```bash
make build
./bin/lapguard
```

## Test commands

| Command | What it does |
| --- | --- |
| `make test` | `go test ./...` |
| `make lint` | fail if `gofmt -l .` is non-empty, then `go vet ./...` |
| `make build-web` | `npm ci` and Vite production build |
| `make build` | frontend + `go build -tags embedui` (version: git tag or `dev`) |
| `make release-build` | linux-amd64 and linux-arm64 plus `SHA256SUMS` |
| `make clean` | remove `bin/`, `dist/`, and staged embed files |

CI runs the same checks on Linux amd64. It does not talk to real batteries,
Docker, or TLP.

Please keep `gofmt` clean. Do not commit `web/dist`, `internal/webui/dist`
assets (except `.gitkeep`), or files that contain secrets.

## Mock hardware fixtures

Two layers exist:

1. **Checked-in sysfs tree** — `testdata/sysfs/` (`BAT0` + `AC`). Good for
   telemetry and `make run-fixture`.
2. **Per-test trees** — helpers in `internal/discovery/laptops_test.go`
   (`writeBattery`, `writeSupply`, `writeModules`, `fakeRunner`).

To add a laptop profile:

1. Add a case to `TestMockLaptops` (or a focused test) with the sysfs files
   that machine actually exposes.
2. Use **fake** identifiers only (`TEST-BAT-001`, `TP-BAT-1`, manufacturer
   strings that are already public). Never copy a real `serial_number`.
3. Include `type=Mains` + `online` if the machine has an AC adapter.
4. If TLP matters, stub `tlp` / `tlp-stat` through `fakeRunner` rather than
   calling the host binary.
5. Assert `features.charge_thresholds` (`sysfs` / `tlp` / `none`), naming
   (`charge` / `energy` / `both`), and `raw_power_now_supported` vs
   `derived_power_supported`.

A missing sysfs file is a capability gap, not a test failure. Match
[COMPATIBILITY.md](COMPATIBILITY.md): absence of `power_now` is fine when
`current_now` and `voltage_now` exist.

Example sketch:

```go
writeBattery(t, root, "BAT1", map[string]string{
    "type":       "Battery",
    "present":    "1",
    "status":     "Discharging",
    "capacity":   "61",
    "charge_now": "1848000",
    "charge_full":"3030000",
    "voltage_now":"11220000",
    "current_now":"1205000",
    "manufacturer": "FakeCo",
    "model_name":    "FixturePack",
    "serial_number": "TEST-BAT-001",
})
writeSupply(t, root, "ADP1", map[string]string{"type": "Mains", "online": "0"})
```

## Compatibility reports

On the laptop, generate a privacy-safe JSON report:

```bash
lapguard discover --report > lapguard-compatibility-report.json
# or:
lapguard discover --report --pretty
```

Attach the file to a GitHub issue using the **Compatibility report** template,
or include it in a PR that updates [COMPATIBILITY.md](COMPATIBILITY.md).

The command runs the same hardware discovery as the daemon, then prints
sanitized JSON on stdout. Serial numbers, usernames, home-directory paths, IP
and MAC addresses, UUIDs, webhook URLs, tokens, passwords, and chat IDs are
omitted. Manufacturer, model, OS, kernel, `BAT0`/`BAT1` names, naming
convention, sysfs fields, power and charge-threshold methods, tools, modules,
and feature capabilities are kept.

Do not change verified Fujitsu Lifebook A3510 facts unless you re-ran
discovery on that machine. Do not paste `config.json` or unsanitized
`GET /api/v1/discover` output.

See [COMPATIBILITY.md](COMPATIBILITY.md) for the field list to record
(battery name, naming convention, power path, threshold method, modules, TLP).

## Privacy rules

Before a report, log, screenshot, or PR, remove:

- Battery and device **serial numbers**
- **Usernames** and home-directory paths that identify a person
- **IP addresses**, MAC addresses, UUIDs, Tailscale names, and MagicDNS hostnames
- **Tokens**, passwords, cookie headers
- **Webhook URLs** (ntfy topics, Telegram bot URLs, Discord webhooks, generic HTTPS endpoints)

`config.json` must never be pasted. Prefer `lapguard discover --report` over
API JSON. The HTTP API still returns serials and hostnames for local use; the
CLI export is the supported public report. The API already redacts
`webhook_url` and `chat_id`; still treat `config.json` as secret (`0600`).

Fixture serials in this repo are synthetic and may stay.

## Pull requests

- Keep tests passing (`make test` and `make lint`).
- Do not add `curl | sudo bash` installers or unrestricted sudoers rules.
- Do not execute Docker commands or host shutdown/reboot.
- Do not log secrets. Use `internal/config` redaction helpers.
- Match existing Go and Svelte style. Prefer small, reviewable diffs.
