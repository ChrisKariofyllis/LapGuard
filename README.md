# LapGuard

[![CI](https://github.com/ChrisKariofyllis/LapGuard/actions/workflows/ci.yml/badge.svg)](https://github.com/ChrisKariofyllis/LapGuard/actions/workflows/ci.yml)
[![Latest Alpha Release](https://img.shields.io/github/v/release/ChrisKariofyllis/LapGuard?include_prereleases&label=latest%20alpha)](https://github.com/ChrisKariofyllis/LapGuard/releases)
[![License](https://img.shields.io/github/license/ChrisKariofyllis/LapGuard)](https://github.com/ChrisKariofyllis/LapGuard/blob/main/LICENSE)

Lightweight Linux laptop power manager for machines that stay on as 24/7 home servers.

Go API + Svelte dashboard. Default bind: **`127.0.0.1:8585`**.

## Alpha warning

**This is an open-source alpha.** Treat it as experimental software.

- APIs, config keys, and systemd paths may still change.
- There is **no authentication** on the HTTP API.
- Charge-threshold **writes are not exposed** over HTTP yet.
- CI and tests do **not** require a battery, Docker, root, or TLP.

### Safety controller is dry-run only

The battery safety controller classifies `NORMAL` / `WARNING` / `CRITICAL` /
`SHUTDOWN_PENDING` and can record an *intended* action plan. **It does not
execute that plan.**

LapGuard will **not** stop Docker containers, sync the filesystem, or power
off the host. It will not run `docker stop`, `systemctl poweroff`, `shutdown`,
`reboot`, or `sync`. `safety.dry_run` is forced on. Real shutdown and Docker
execution are out of scope for this alpha.

> **Warning:** The safety controller is currently simulation/dry-run only. It will not stop containers, sync the filesystem, or power off the host.

## Current features

- Battery telemetry from sysfs (`energy_*` and `charge_*`), with derived watts
  from `current_now × voltage_now` when `power_now` is missing
- Estimated time remaining while discharging (when energy and power are usable)
- Hardware auto-discovery: batteries, AC adapters, kernel modules, TLP/UPower
- Privacy-safe `lapguard discover --report` export for GitHub compatibility issues
- Charge-threshold **detection** (`sysfs` → `tlp` → `none`)
- AC power-loss watcher with debounce and a local SQLite outage log
- Optional notifications (ntfy, Telegram, Discord) — **off by default**
- Battery safety state machine with simulate-warning / simulate-critical
- Svelte dashboard for telemetry, capabilities, power events, config, and safety
- Loopback bind; remote access is meant to go through Tailscale or SSH, not a public listen address

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
3. `chmod +x lapguard_*_linux-amd64` and run it (no root required):

   ```bash
   ./lapguard_<version>_linux-amd64
   ```

4. Open `http://127.0.0.1:8585`.

There is **no** `curl | sudo bash` installer. For a systemd unit and
`/etc/lapguard` vs `~/.config/lapguard` paths, see
[docs/install.md](docs/install.md).

First start writes `~/.config/lapguard/config.json` (mode `0600`) and uses
`~/.config/lapguard/events.db` for the outage log unless you pass `-config` /
`-events-db`.

## Development

No root, Docker, or TLP required.

```bash
export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$PATH"

make test          # go test ./...
make lint          # gofmt + go vet
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

## Tailscale access

Keep the default listen address **`127.0.0.1:8585`**. Do not bind `0.0.0.0`.

Options that leave the process on loopback:

- **SSH tunnel:** `ssh -L 8585:127.0.0.1:8585 user@laptop`
- **Tailscale Serve** (tailnet only, not the public internet):

  ```bash
  tailscale serve --bg http://127.0.0.1:8585
  ```

Use Tailscale ACLs / who is on the tailnet as the access-control layer.
**Do not use Tailscale Funnel** to publish LapGuard to the public internet.
The API has no login, CSRF tokens, or TLS of its own.

## Security limitations

- **No application authentication or authorization.** Anyone who can reach
  the port can read telemetry, change notification settings, and trigger
  dry-run safety tests.
- Secrets in `config.json` (webhook URLs, bot tokens, chat IDs) are stored
  mode `0600` and are **redacted** from API responses and logs. They are
  still present on disk. Never commit that file.
- Notifications, when enabled, send battery/AC events to a third party.
  Treat the webhook URL as a secret.
- Discovery reports hostname, kernel, and battery model on the HTTP API.
  Use `lapguard discover --report` (not raw API JSON) when sharing a
  compatibility report.
- The process should not run as root for alpha. The systemd templates do
  not grant sudo or `CAP_SYS_BOOT`.
- Dry-run safety means a low battery will **not** stop workloads or power
  off the machine. Do not rely on LapGuard to protect the pack yet.

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

**Power source.** LapGuard polls `/sys/class/power_supply/` for `type=Mains`
and reads `online`. Adapter names are discovered, not hardcoded. A new AC
state must stay stable for 10 seconds (default) before
`AC_CONNECTED` / `AC_DISCONNECTED` is stored. Events live in SQLite
(`0600`), pruned after 90 days or 1000 rows. No serials in the log.

**Safety states.** Warning/critical use the configured shutdown percents and
fire only while **Discharging**. On critical, the process records an intended
plan (`stop_docker` if configured, `sync`, `poweroff`) and **logs it**.
`POST /api/v1/safety/test` with `{"scenario":"warning"}` or `"critical"`
simulates a transition without sysfs and without commands.

**Config.** `GET`/`PUT /api/v1/config` persist notifications, shutdown
percents, Docker *intent*, and safety flags to the JSON file. Critical
percent must be lower than warning. `safety.dry_run` cannot be turned off
in this alpha.

**Discovery.** Features are enabled only when the matching sysfs files,
modules, or tools exist. Missing hardware is `none` plus `why_not`. See
[COMPATIBILITY.md](COMPATIBILITY.md). Threshold writes exist in
`internal/thresholds` but are not on the HTTP API.

## API

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/v1/telemetry` | Battery snapshot, power (W), health (%) |
| GET | `/api/v1/discover` | Re-run detection; full `CapabilityReport` |
| GET | `/api/v1/capabilities` | UI payload: `detection_method`, `recommendation`, `why_not` |
| GET | `/api/v1/config` | Notifications, shutdown, Docker, safety settings |
| PUT | `/api/v1/config` | Merge and persist those settings |
| POST | `/api/v1/config/notifications` | Merge notifications |
| POST | `/api/v1/config/shutdown` | Merge shutdown percents (does not power off) |
| GET | `/api/v1/power` | `AC` / `BATTERY` / `UNKNOWN`, adapters, watcher |
| GET | `/api/v1/events` | Recent outage events (`limit`, optional `type`) |
| POST | `/api/v1/actions/test-notification` | Test message (enabled provider required) |
| GET | `/api/v1/safety` | Controller state, thresholds, intended actions |
| POST | `/api/v1/safety/test` | Simulate warning or critical (dry-run) |

`GET /api/v1/config` omits secret values and sets `webhook_configured` /
`chat_id_configured`. `execution.shutdown` and `execution.docker` stay
`stored_only` in this alpha.

## Layout

```
cmd/lapguard/                # daemon + `discover --report` CLI
internal/discovery/          # sysfs / modules / tools / threshold detection + sanitized export
internal/battery/            # sysfs + mock telemetry (energy_* and charge_*)
internal/power/              # mains scan, debounce watcher
internal/storage/            # SQLite outage event log
internal/thresholds/         # sysfs writes or tlp setcharge (no HTTP yet)
internal/notify/             # ntfy / Telegram / Discord delivery, retries, dry-run
internal/safety/             # battery safety state machine (recording executor only)
internal/webui/              # optional embed of web/dist (-tags embedui)
internal/api/
internal/config/             # flags + atomic config.json
contrib/systemd/             # user and system unit templates
testdata/sysfs/BAT0/
web/
```

## License

LapGuard is licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE).
