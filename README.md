# LapGuard

Lightweight Linux laptop power manager for machines that stay on as 24/7 home servers.

The API binds to `127.0.0.1:8585`. Remote access is intended to go through Tailscale later, not a public bind.

**Milestone 3C is complete:** notification delivery (ntfy, Telegram, Discord) is implemented and **disabled by default**. Docker stop and host shutdown are still **not executed**.

See [COMPATIBILITY.md](COMPATIBILITY.md) for tested laptops and charge-threshold behaviour.

## Milestone 3B — AC power-loss watcher

LapGuard polls `/sys/class/power_supply/` for supplies with `type=Mains` and reads `online`. Adapter names (`AC`, `ACAD`, `ADP1`, …) are discovered, not hardcoded.

- On AC if **any** mains adapter is online
- On battery if **all** detected mains adapters are offline
- `UNKNOWN` if there is no mains adapter, or `online` is missing/malformed

The watcher records a startup baseline **without** emitting an event. A new state must stay stable for **10 seconds** (configurable, `-power-debounce`) before `AC_CONNECTED`, `AC_DISCONNECTED`, or `AC_UNKNOWN` is stored. Poll interval defaults to **5 seconds** (`-power-poll`).

Events are stored in SQLite at `~/.config/lapguard/events.db` (mode `0600`). Rows older than 90 days or beyond 1000 events are pruned. The log never stores secrets or battery serial numbers. There is no SSE endpoint in this milestone.

This milestone does **not** run `systemctl poweroff`, `shutdown`, or `docker stop`. Notification HTTP calls are implemented in Milestone 3C and remain off until `notifications.enabled` is true.

## Milestone 3C — Notification delivery

LapGuard can send `AC_DISCONNECTED`, `AC_CONNECTED`, `BATTERY_WARNING`, and `BATTERY_CRITICAL` through ntfy, Telegram, or a Discord webhook. Delivery stays **disabled** until `notifications.enabled` is true and a real provider is configured. A webhook URL with `provider` `none` is not enough.

Events are rate-limited (5 minutes per type) so AC flapping cannot spam a channel. Failed deliveries retry up to 3 times with exponential backoff and a 5s HTTP timeout. `notifications.dry_run=true` logs only the event type and provider — it never makes an HTTP request.

Webhook URLs, bot tokens, chat IDs, and passwords are stored in `config.json` (mode `0600`) and are **never** returned by the API or written to logs. After save, the UI shows that a secret is stored without displaying it.

`POST /api/v1/actions/test-notification` sends a test message when notifications are enabled and a provider is configured. The response is `ok` / error only — no URL or token.

Battery warning/critical use the configured shutdown percents and fire only while the pack is discharging. Host shutdown is still not executed.

### Provider setup (placeholders only)

**ntfy** — create a topic and use its URL as `webhook_url`:

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

**Discord** — paste a channel webhook URL:

```json
{
  "provider": "discord",
  "enabled": true,
  "webhook_url": "https://discord.com/api/webhooks/<id>/<token>"
}
```

Turn `dry_run` off after a successful test. The estimate of remaining battery time and the outage log are unchanged.

## Milestone 3A — Secure configuration API

User-managed settings live alongside the existing process config (listen, provider, sysfs root, threshold method):

| Section | Fields | Runtime effect |
| --- | --- | --- |
| `notifications` | `provider`, `enabled`, `dry_run`, `webhook_url`, `chat_id` | Delivery when enabled (3C) |
| `shutdown` | `enabled`, `warning_threshold`, `critical_threshold` | Percents used for battery alerts; shutdown not executed |
| `docker` | `stop_enabled`, `timeout_seconds` | Persisted only |

Persistence rules:

- Path: `~/.config/lapguard/config.json` (or `-config`)
- Parent directory is created if missing
- Writes use a temporary file in the same directory, then `rename`
- File mode is `0600`
- Incoming JSON is validated; malformed bodies return HTTP 400
- Warning and critical percents must be `0..100`, and **critical must be lower than warning**
- Notification secrets, tokens, passwords, and webhook URLs are never written to logs

CLI flags still overlay the file for process settings (`listen`, `provider`, `sysfs-root`, …). The HTTP API updates the three user sections without restarting the process.

## Milestone 2 — Hardware Auto-Discovery & Capabilities

On startup (and on `GET /api/v1/discover`) LapGuard scans:

- `/sys/class/power_supply/` — batteries, AC adapters, available sysfs attributes
- `/proc/modules` — vendor modules (`thinkpad_acpi`, `fujitsu_laptop`, `asus_wmi`, …)
- Userspace tools — TLP (plus version / `tlp-stat` threshold support), UPower, ACPI, and related helpers

Charge-threshold method is chosen in order: **sysfs → tlp → none**. Unsupported hardware is not an error; the report includes `why_not`. The Fujitsu Lifebook A3510 is the reference edge case: **BAT1** uses `charge_*` (no `power_now` or `temp`); power is derived from `current_now × voltage_now`; `fujitsu_laptop` may load and TLP may be installed, yet charge control never registers, so the method is `none`.

Telemetry accepts both sysfs naming conventions (`energy_*` and `charge_*`). A missing file is omitted, not a failure.

Power semantics:

- `raw_power_now_supported` — sysfs `power_now` is present
- `derived_power_supported` — `current_now` and `voltage_now` can be multiplied
- Displayed watts prefer `power_now`, otherwise `current_now × voltage_now` (negative while discharging)
- `0 W` is a real reading when `current_now` is zero, not a missing capability
- Estimated time left is `energy_now_wh / abs(power_w)` while **discharging** with non-zero battery power. It is omitted on AC, when full, and when discharge power is 0 W. The value tracks current load and can move around.

Health is `energy_full / energy_full_design` or `charge_full / charge_full_design`.

Threshold **writes** are wired in `internal/thresholds` (sysfs `charge_control_*` or `tlp setcharge START STOP BATX`) but are **not** exposed over HTTP yet.

## API

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/v1/telemetry` | Battery snapshot, power (W), health (%) |
| GET | `/api/v1/discover` | Re-run detection; full `CapabilityReport` |
| GET | `/api/v1/capabilities` | UI payload: per-feature `detection_method`, `recommendation`, `why_not` |
| GET | `/api/v1/config` | Current notifications, shutdown, and Docker settings |
| PUT | `/api/v1/config` | Merge and persist those settings |
| POST | `/api/v1/config/notifications` | Merge and persist the notifications section |
| POST | `/api/v1/config/shutdown` | Merge and persist the shutdown section |
| GET | `/api/v1/power` | Current power source (`AC` / `BATTERY` / `UNKNOWN`), adapters, watcher status |
| GET | `/api/v1/events` | Recent outage events (`limit`, optional `type`) |
| POST | `/api/v1/actions/test-notification` | Send a test message (requires enabled provider) |

### `GET /api/v1/config`

Returns the in-memory settings (defaults if the file has no user sections yet):

```json
{
  "notifications": {
    "provider": "none",
    "enabled": false,
    "dry_run": false,
    "webhook_url": "",
    "chat_id": "",
    "webhook_configured": false,
    "chat_id_configured": false
  },
  "shutdown": {
    "enabled": false,
    "warning_threshold": 20,
    "critical_threshold": 10
  },
  "docker": {
    "stop_enabled": false,
    "timeout_seconds": 30
  },
  "execution": {
    "notifications": "unconfigured",
    "shutdown": "stored_only",
    "docker": "stored_only"
  }
}
```

`notifications.provider` is one of `none`, `ntfy`, `telegram`, `discord`, `webhook`. Secrets in `webhook_url` and `chat_id` are omitted from responses; use `webhook_configured` / `chat_id_configured`. `execution.notifications` is `unconfigured`, `disabled`, `dry_run`, or `ready`.

### `PUT /api/v1/config`

JSON object. Omitted sections are left unchanged. Round-tripping `execution` / `notes` is ignored.

```json
{
  "notifications": {
    "provider": "telegram",
    "enabled": false,
    "dry_run": true,
    "webhook_url": "https://api.telegram.org/bot<YOUR_BOT_TOKEN>/sendMessage",
    "chat_id": "<YOUR_CHAT_ID>"
  },
  "shutdown": {
    "enabled": true,
    "warning_threshold": 25,
    "critical_threshold": 8
  },
  "docker": {
    "stop_enabled": true,
    "timeout_seconds": 45
  }
}
```

Errors:

| Condition | Status | `error` |
| --- | --- | --- |
| Empty body, non-object, or invalid JSON | 400 | `malformed JSON` |
| Percent outside 0–100, or critical ≥ warning | 400 | `invalid config` |
| Unknown notification provider, missing webhook when enabled | 400 | `invalid config` |
| Persist failure | 500 | `failed to persist config` |

`POST /api/v1/config/notifications` and `POST /api/v1/config/shutdown` take the section object (not wrapped) and use the same validation. Shutdown POST still does not power off the host.

### `GET /api/v1/power`

Returns the current mains classification and watcher status. `source` is `AC`, `BATTERY`, or `UNKNOWN`. Adapter objects include `name`, `online`, and `readable` only — no serial numbers.

### `GET /api/v1/events`

Query parameters: `limit` (default 50) and optional `type` (`AC_CONNECTED`, `AC_DISCONNECTED`, `AC_UNKNOWN`). Newest first. Invalid `type` or `limit` returns HTTP 400.

## UI

The dashboard Capabilities panel lists each feature as enabled or not supported, with the detection method and fallback reason. Tools and kernel modules are shown as chips. **Re-scan** calls `/api/v1/discover` and refreshes the panel.

The Configuration panel edits notifications, shutdown thresholds, and Docker drain settings. **Save settings** calls `PUT /api/v1/config`. **Send test notification** calls `POST /api/v1/actions/test-notification` when notifications are enabled. **Shut down now** and **Stop Docker containers** stay disabled. Webhook URLs and chat IDs are not shown after save.

The Power source panel shows AC / battery / unknown, discovered mains adapters, and the recent outage log from `GET /api/v1/events`. The startup baseline is not listed as an event.

## Layout

```
cmd/lapguard/main.go
internal/discovery/          # sysfs / modules / tools / threshold detection
internal/battery/            # sysfs + mock telemetry (energy_* and charge_*)
internal/power/              # mains scan, debounce watcher
internal/storage/            # SQLite outage event log
internal/thresholds/         # sysfs writes or tlp setcharge (no HTTP yet)
internal/notify/             # ntfy / Telegram / Discord delivery, retries, dry-run
internal/api/handlers.go
internal/api/config.go       # GET/PUT config, POST notifications/shutdown
internal/api/notify.go       # POST /actions/test-notification
internal/api/power.go        # GET /power and GET /events
internal/config/             # flags + atomic ~/.config/lapguard/config.json
testdata/sysfs/BAT0/
web/
```

## Development (no root)

The development box is an HP ProDesk without a battery. The production box is a Fujitsu Lifebook A3510 with a real `charge_*` pack on BAT1. `auto` tries sysfs and falls back to the mock provider when no battery is present. Discovery always runs against the real (or `-sysfs-root`) tree.

```bash
export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$PATH"

# tests use testdata/sysfs and in-memory laptop mocks, not live hardware
go test ./...

# API (mock telemetry on a batteryless machine; discovery still inspects the host)
go run ./cmd/lapguard

# dashboard
cd web && npm install && npm run dev
```

Open `http://127.0.0.1:5173`. Vite proxies `/api` to the Go process.

```bash
go run ./cmd/lapguard -provider sysfs -sysfs-root testdata/sysfs
```

On the Lifebook, the default `auto` provider reads `/sys/class/power_supply` as an unprivileged user.

First start writes `~/.config/lapguard/config.json` (listen, provider, `threshold_method: auto`, plus notifications/shutdown/docker defaults). Flags override the file. Features are re-detected every launch; `threshold_method` stays `auto` unless you pin `sysfs`, `tlp`, or `none`.

## Production notes

- Default bind is loopback only.
- Do not run the process as root during development.
- Config file mode is `0600`; webhook URLs and tokens stay out of logs.
- Outage events live in `~/.config/lapguard/events.db` (mode `0600`). Older than 90 days or beyond 1000 rows are pruned.
- Build the UI with `cd web && npm run build`, then `go run ./cmd/lapguard` serves `web/dist`.
- Notification delivery is off by default. Docker drain, host shutdown, and authentication remain out of scope for execution. Discovery still reports whether Docker is present; the config API only stores the intended Docker/shutdown policy.
