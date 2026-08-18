# LapGuard

[![CI](https://github.com/ChrisKariofyllis/LapGuard/actions/workflows/ci.yml/badge.svg)](https://github.com/ChrisKariofyllis/LapGuard/actions/workflows/ci.yml)
[![Latest Alpha Release](https://img.shields.io/github/v/release/ChrisKariofyllis/LapGuard?include_prereleases&label=latest%20alpha)](https://github.com/ChrisKariofyllis/LapGuard/releases)
[![License](https://img.shields.io/github/license/ChrisKariofyllis/LapGuard)](https://github.com/ChrisKariofyllis/LapGuard/blob/main/LICENSE)

Lightweight Linux laptop power manager for machines that stay on as 24/7 home servers.

Go API + Svelte dashboard. Default bind: **`127.0.0.1:8585`**.

## Alpha warning

**This is an open-source alpha.** Treat it as experimental software.

- APIs, config keys, and systemd paths may still change.
- There is **no authentication** on the HTTP API unless you enable an optional Bearer token (`lapguard auth generate`).
- GET telemetry stays readable without a token in this alpha; POST/PUT require the token when auth is enabled.
- Charge-threshold **writes are not wired** (helpers exist, daemon never calls them).
- CI and tests do **not** require a battery, Docker, root, or TLP.

### Real actions are experimental and disabled by default

**Do not enable real actions on an important machine.** This alpha remains
dry-run unless you explicitly change both `actions.real_enabled` and
`safety.dry_run`.

The battery safety controller classifies `NORMAL` / `WARNING` / `CRITICAL` /
`SHUTDOWN_PENDING` and can record an *intended* action plan. **Automatic
low-battery shutdown is not executed.**

Defaults:

- `actions.real_enabled=false`
- `safety.dry_run=true`
- `safety.require_ac_loss=true`
- `docker.stop_enabled=false`

Manual `POST /api/v1/actions/poweroff` and `POST /api/v1/actions/docker-drain`
are experimental. They stay behind authentication (when enabled), explicit
confirmation, `actions.real_enabled=true`, `safety.dry_run=false`, a known
disconnected AC adapter, a discharging battery, a cooldown, and an
in-progress lock. `GET /api/v1/actions/status` reports those gates without
executing anything.

CI and `go test` never run host `systemctl`, `poweroff`, `docker`, or `sync`.
Executor tests use temporary fake binaries that record argv and do not
touch the machine. LapGuard does **not** install sudoers, polkit, or root
permissions. A normal user may get **503** `action executor is unavailable`
after the gates pass, because OS permission to power off or talk to Docker
is outside this project. Real Docker drain and host poweroff are **not**
safe for production yet.

> **Warning:** Real Docker drain and host poweroff are experimental and off by default. Do not enable them on a machine you care about. The current alpha stays dry-run unless you change that configuration yourself.

### v0.9.3-alpha limitations

- Optional Bearer auth; GET routes stay readable when auth is on.
- Charge-threshold **writes are not wired**.
- Automatic low-battery shutdown is **not implemented**.
- Manual Docker drain and poweroff exist but stay **disabled** (`real_enabled=false`, `dry_run=true`).
- CI tests the real executor only against temp-dir fakes; it never powers off the host.
- Not production-safe; no sudoers/polkit; no UPS support. OS poweroff permission is
  external — a normal user may see HTTP 503 after the software gates pass.

Public alpha testers: follow [docs/alpha-testing.md](docs/alpha-testing.md)
(`make smoke` never enables real actions).

## Current features

- Battery telemetry from sysfs (`energy_*` and `charge_*`), with **battery-side**
  watts from `power_now` or derived `current_now × voltage_now`. While charging
  that value is power into the pack, not total laptop consumption
- Estimated time remaining while discharging (never while charging or on AC)
- Hardware auto-discovery: batteries, AC adapters, kernel modules, TLP/UPower
- Privacy-safe `lapguard discover --report` export for GitHub compatibility issues
- Read-only `lapguard tailscale instructions` / `lapguard tailscale check` for Serve setup
- Charge-threshold **detection** (`sysfs` → `tlp` → `none`)
- AC power-loss watcher with debounce and a local SQLite outage log
- Optional notifications (ntfy, Telegram, Discord) — **off by default**
- Battery safety state machine with simulate-warning / simulate-critical
- Experimental manual Docker drain and host poweroff, **disabled by default**
- Svelte dashboard for telemetry, capabilities, power events, config, and safety
- Optional Bearer API token (`lapguard auth generate`) protecting POST/PUT routes
- Loopback bind; remote access is meant to go through Tailscale Serve or SSH, not a public listen address

See [COMPATIBILITY.md](COMPATIBILITY.md) for tested machines. The production
reference laptop is a **Fujitsu Lifebook A3510** (BAT1, `charge_*`, derived
power, charge-threshold method `none`).

## Install from a prebuilt binary

Tagging `v*` builds **linux-amd64** and **linux-arm64** binaries with the
dashboard embedded and `SHA256SUMS`. The workflow **does not publish** the
GitHub Release; a maintainer must publish the draft by hand.

1. Download `lapguard_<version>_linux-amd64` (or `linux-arm64`) and `SHA256SUMS`
   from a published [GitHub Release](https://github.com/ChrisKariofyllis/LapGuard/releases).
2. Verify checksums: `sha256sum -c SHA256SUMS --ignore-missing`
3. `chmod +x lapguard_*_linux-amd64` (or `linux-arm64`) and run it (no root required):

   ```bash
   ./lapguard_<version>_linux-amd64
   # or: ./lapguard_<version>_linux-arm64
   ```

4. Open `http://127.0.0.1:8585`.

There is **no** `curl | sudo bash` installer. For a systemd unit and
`/etc/lapguard` vs `~/.config/lapguard` paths, see
[docs/install.md](docs/install.md).

First start writes `~/.config/lapguard/config.json` (mode `0600`) and uses
`~/.config/lapguard/events.db` for the outage log unless you pass `-config` /
`-events-db`. Release binaries embed the dashboard; systemd units pass
`-web-dir none` so they use that embed instead of a local `web/dist`.

**Config reload:** LapGuard reads `config.json` at process start. Editing the
file on disk while the daemon is running does **not** change the active
settings (`actions.real_enabled`, `safety.dry_run`, and the rest). Restart
the process after a disk edit (`systemctl --user restart lapguard` or
`systemctl restart lapguard`). Dashboard Save and `PUT /api/v1/config`
update the running process and rewrite the file. There is no automatic
reload. `GET /api/v1/actions/preflight` reports the **runtime** gates and
repeats this rule. Use `-config` if you intend a non-default file, then
restart after editing that same file.

Privacy-safe hardware report (no daemon required):

```bash
lapguard discover --report > lapguard-compatibility-report.json
```

Optional API token (shown once; hash only in `config.json`):

```bash
lapguard auth generate
lapguard auth status
```

## Development

No root, Docker, or TLP required.

```bash
export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$PATH"

make test          # go test ./...
make lint          # gofmt + go vet
make smoke         # local mock smoke test (never enables real actions)
make build-web     # Svelte production build
make build         # embed UI; version is "dev" or a clean git tag (override with VERSION=…)
make release-build # linux-amd64 + linux-arm64 + SHA256SUMS
make clean
```

Run the API (mock telemetry if the machine has no battery):

```bash
go run ./cmd/lapguard
```

Dashboard with hot reload:

```bash
cd web && npm install && npm run dev
```

Open `http://127.0.0.1:5173`. Vite proxies `/api` to the Go process.

Sysfs fixture:

```bash
make run-fixture
# or: go run ./cmd/lapguard -provider sysfs -sysfs-root testdata/sysfs
```

More in [CONTRIBUTING.md](CONTRIBUTING.md).

## Remote access (Tailscale Serve)

LapGuard listens only on loopback. **Tailscale Serve** is an external reverse
proxy in front of that local process. LapGuard does not bind to a Tailscale
`100.x.y.z` address, does not run `sudo`, and does not configure Serve or Funnel.

```
LapGuard                  ->  127.0.0.1:8585
Tailscale Serve           ->  proxies that localhost service to your tailnet
Remote Tailscale devices  ->  open the dashboard over the tailnet
```

Keep the default listen address **`127.0.0.1:8585`**. Do not bind `0.0.0.0`
just to use Tailscale. Do not expose port 8585 on the public Internet.
Existing `-listen` and `config.json` listen settings still apply; leave them
on loopback.

### Local process

Install and authenticate Tailscale yourself (outside LapGuard). Then start
LapGuard locally:

```bash
./lapguard -web-dir none
```

The dashboard stays available on the machine at:

```
http://127.0.0.1:8585
```

### Expose it through Tailscale Serve

Prefer an explicit localhost target. On current Tailscale CLIs:

```bash
sudo tailscale serve --bg http://127.0.0.1:8585
```

If that syntax is rejected, the installed Tailscale version uses a different
Serve CLI. Check:

```bash
tailscale serve --help
```

Useful status commands:

```bash
tailscale status
tailscale ip -4
tailscale serve status
```

Serve is reachable **only inside the tailnet** when configured normally. Use
Tailscale ACLs to restrict access to trusted users and devices.

**Do not use Tailscale Funnel.** Funnel publishes to the public Internet.

An SSH tunnel is an alternative that also leaves the process on loopback:

```bash
ssh -L 8585:127.0.0.1:8585 user@laptop
```

Install-oriented steps and troubleshooting are in
[docs/install.md](docs/install.md#remote-access-with-tailscale-serve).

LapGuard can print the same guidance and a privacy-safe JSON probe without
starting the daemon or changing Tailscale state:

```bash
lapguard tailscale instructions
lapguard tailscale check
lapguard tailscale check --pretty
```

`check` only looks up `tailscale` on `PATH` and, if present, may run
`tailscale status`, `tailscale ip -4`, `tailscale serve status`, and
`tailscale version`. It never runs `sudo`, Funnel, or `tailscale serve --bg`.

### Security

When `auth.enabled` is **false** (the default), LapGuard has **no application-level
authentication**. Anyone who can reach the HTTP port can change settings.
Enable a Bearer token before exposing the dashboard over Tailscale:

```bash
lapguard auth generate
```

Store the printed token in a password manager. It is shown once. Only a
SHA-256 hash is written to `config.json` (mode `0600`). Send it as
`Authorization: Bearer <token>` on POST/PUT. Do not put tokens in URLs.

GET telemetry, capabilities, discover, power, events, safety, healthz,
config, auth/status, actions/status, and actions/preflight stay readable
without a token in this alpha. Protecting GET is reserved for a later release.

With Tailscale Serve, **Tailscale identity/ACLs plus the API token** are
the security boundary. Only trusted tailnet users/devices should be allowed.
Do not use Funnel or expose port 8585 publicly.

If you lose the token, on the laptop run `lapguard auth rotate` or
`lapguard auth disable` (these edit the local config file; they do not need
the old token). Then generate a new one.

## Security limitations

- **Optional Bearer authentication.** Default is off for local development.
  When enabled, POST/PUT require `Authorization: Bearer`. GET telemetry remains
  readable. See [Remote access](#remote-access-tailscale-serve).
- Secrets in `config.json` (token hashes, webhook URLs, bot tokens, chat IDs)
  are stored mode `0600` and are **redacted** from API responses and logs.
  The plaintext API token is never stored or returned by HTTP. Never commit
  `config.json`.
- Notifications, when enabled, send battery/AC events to a third party.
  Treat the webhook URL as a secret.
- Discovery reports hostname, kernel, and battery model on the HTTP API.
  Use `lapguard discover --report` (not raw API JSON) when sharing a
  compatibility report.
- The process should not run as root for alpha. The systemd templates do
  not grant sudo or `CAP_SYS_BOOT`. LapGuard does not install sudoers or
  polkit rules. After every software gate passes, `systemctl poweroff` /
  `poweroff` still need OS permission; without it the API returns **503**.
- Dry-run safety means a low battery will **not** automatically stop
  workloads or power off the machine. Real actions stay disabled until you
  set `actions.real_enabled=true` and `safety.dry_run=false`. Do not do that
  on an important machine. CI never enables those flags against the host.

## Compatibility reports

Please file what your laptop actually exposes.

On the machine, generate a privacy-safe JSON report and attach it to a GitHub
**Compatibility report** issue, or include it in a PR that updates
[COMPATIBILITY.md](COMPATIBILITY.md):

```bash
lapguard discover --report > lapguard-compatibility-report.json
```

Pretty-print to the terminal:

```bash
lapguard discover --report --pretty
```

The export includes manufacturer, model, OS, kernel, battery names (`BAT0` /
`BAT1`), naming convention, available sysfs fields, power calculation method,
charge-threshold method, detected tools and kernel modules, and feature
capabilities. It **never** includes battery serial numbers, usernames, home
directory paths, IP or MAC addresses, UUIDs, webhook URLs, tokens, passwords,
or chat IDs.

Do not paste `config.json`. Prefer `lapguard discover --report` over
`GET /api/v1/discover` when sharing output: the API payload can include serials
and hostnames.

Record battery sysfs name (`BAT0` / `BAT1`), naming (`charge_*` vs `energy_*`),
whether watts come from `power_now` or `current_now × voltage_now`, charge-
threshold method (`sysfs` / `tlp` / `none`), and relevant vendor modules.

## Notifications (optional)

Delivery stays **disabled** until `notifications.enabled` is true and a real
provider is configured. `notifications.dry_run=true` logs the event type and
provider only — no HTTP.

**ntfy**

```json
{
  "provider": "ntfy",
  "enabled": true,
  "dry_run": true,
  "webhook_url": "https://ntfy.sh/your-topic-name"
}
```

**Telegram** — `webhook_url` is the Bot API `sendMessage` endpoint; `chat_id` is required:

```json
{
  "provider": "telegram",
  "enabled": true,
  "webhook_url": "https://api.telegram.org/bot<YOUR_BOT_TOKEN>/sendMessage",
  "chat_id": "<YOUR_CHAT_ID>"
}
```

**Discord**

```json
{
  "provider": "discord",
  "enabled": true,
  "webhook_url": "https://discord.com/api/webhooks/<id>/<token>"
}
```

Turn `dry_run` off after a successful test from the dashboard or
`POST /api/v1/actions/test-notification`.

## How it works (short)

**Battery power.** LapGuard reports **battery-side** watts, not total system
power consumption. `GET /api/v1/telemetry` keeps `power_w` (signed: negative
while discharging, positive while charging) for compatibility. Prefer
`battery_power_w` (magnitude) plus `power_direction` (`charge` / `discharge` /
`idle` / `unknown`) and `power_label`. On a Gigabyte Aero 16, charging at 5%
showed about 36 W from `current_now × voltage_now` — that was charging power
into the pack. Discharging is power drawn from the pack. `Full` / `Not charging`
with `current_now=0` is 0 W (`idle`). Estimated runtime is available only while
discharging.

**Power source.** LapGuard polls `/sys/class/power_supply/` for `type=Mains`
and reads `online`. Adapter names are discovered, not hardcoded. A new AC
state must stay stable for 10 seconds (default) before
`AC_CONNECTED` / `AC_DISCONNECTED` is stored. Events live in SQLite
(`0600`), pruned after 90 days or 1000 rows. No serials in the log.

**Safety states.** Warning/critical use the configured shutdown percents and
fire only while **Discharging**. On critical, the process records an intended
plan (`stop_docker` if configured, `sync`, `poweroff`) and **logs it**.
Automatic execution of that plan is not implemented. `POST /api/v1/safety/test`
with `{"scenario":"warning"}` or `"critical"` simulates a transition without
sysfs and without commands.

**Config.** `GET`/`PUT /api/v1/config` persist notifications, shutdown
percents, Docker *intent*, safety flags, and experimental action gates.
Critical percent must be lower than warning. Defaults keep
`safety.dry_run=true` and `actions.real_enabled=false`. `PUT` updates the
running process. Editing `config.json` on disk does not; restart afterward.

**Discovery.** Features are enabled only when the matching sysfs files,
modules, or tools exist. Missing hardware is `none` plus `why_not`. See
[COMPATIBILITY.md](COMPATIBILITY.md). Charge-threshold **writes are not
wired**: `internal/thresholds` can talk to sysfs or `tlp setcharge`, but the
daemon never calls those helpers and the HTTP API has no write endpoint.

## API

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/v1/telemetry` | Battery snapshot: `power_w`, `battery_power_w`, `power_direction`, `power_label`, health, runtime |
| GET | `/api/v1/discover` | Re-run detection; full `CapabilityReport` (includes serials/hostname — do not share) |
| GET | `/api/v1/capabilities` | UI payload: `detection_method`, `recommendation`, `why_not` |
| GET | `/api/v1/config` | Notifications, shutdown, Docker, safety settings |
| PUT | `/api/v1/config` | Merge and persist those settings |
| POST | `/api/v1/config/notifications` | Merge notifications |
| POST | `/api/v1/config/shutdown` | Merge shutdown percents (does not power off) |
| GET | `/api/v1/power` | `AC` / `BATTERY` / `UNKNOWN`, adapters, watcher |
| GET | `/api/v1/events` | Recent outage events (`limit`, optional `type`) |
| POST | `/api/v1/actions/test-notification` | Test message (enabled provider required) |
| POST | `/api/v1/actions/preview` | Intended plan only (`commands_executed=false`) |
| GET | `/api/v1/actions/status` | Read-only gates, safe config path/source, reload=`restart_required_for_disk_edits` |
| GET | `/api/v1/actions/preflight` | Read-only runtime action check (`commands_executed=false`); explains that disk edits need a restart |
| POST | `/api/v1/actions/poweroff` | Manual poweroff (experimental; confirm `POWER_OFF`) |
| POST | `/api/v1/actions/docker-drain` | Manual Docker drain (experimental; confirm `STOP_DOCKER`) |
| GET | `/api/v1/safety` | Controller state, thresholds, intended actions |
| POST | `/api/v1/safety/test` | Simulate warning or critical (dry-run) |
| GET | `/api/v1/healthz` | Liveness: `status`, `app`, `version`, `auth_enabled` |
| GET | `/api/v1/auth/status` | `auth_enabled`, `token_configured`, timestamps (never the token or hash) |
| POST | `/api/v1/auth/rotate` | Instructs you to use `lapguard auth rotate` (HTTP never returns a new token) |
| POST | `/api/v1/auth/disable` | Disable auth (Bearer required if enabled; loopback-only if disabled) |

`GET /api/v1/config` omits secret values and sets `webhook_configured` /
`chat_id_configured`, `auth_enabled`, and `token_configured`. It never returns
`token_hash`, the plaintext token, or host command strings.
`GET /api/v1/actions/status` never returns command arguments, executable
paths, or secrets. Home directories in `config.path` are shown as `~`.
`GET /api/v1/actions/preflight` is the same runtime snapshot plus an
explanation that on-disk `config.json` edits require a restart.
`execution.shutdown` and `execution.docker` are `disabled`
by default. When `auth.enabled` is true, POST/PUT need `Authorization: Bearer`.
GET routes listed above stay readable.

Manual poweroff and Docker drain are **not** production-ready. Automatic
low-battery shutdown is **not implemented**. Optional `Idempotency-Key` on
those POSTs is hashed in memory and rejected as a duplicate during the
cooldown window.

## Layout

```
cmd/lapguard/                # daemon + discover / tailscale / auth CLI
internal/auth/               # bearer token generate + SHA-256 verify
internal/discovery/          # sysfs / modules / tools / threshold detection + sanitized export
internal/tailscale/          # read-only Tailscale Serve diagnostics
internal/battery/            # sysfs + mock telemetry (energy_* and charge_*)
internal/power/              # mains scan, debounce watcher
internal/storage/            # SQLite outage + bounded audit log
internal/thresholds/         # unused write helpers (sysfs / tlp setcharge); not called
internal/notify/             # ntfy / Telegram / Discord delivery, retries, dry-run
internal/safety/             # battery safety state machine (recording executor)
internal/actions/            # gated real executor (disabled by default; refused in tests)
internal/webui/              # optional embed of web/dist (-tags embedui)
internal/api/
internal/config/             # flags + atomic config.json (mode 0600)
contrib/systemd/             # user and system unit templates
testdata/sysfs/BAT0/
web/
```

## License

LapGuard is licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE).
