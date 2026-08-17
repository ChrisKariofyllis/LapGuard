# LapGuard

Lightweight Linux laptop power manager for machines that stay on as 24/7 home servers.

LapGuard **auto-discovers** what the hardware supports at runtime and enables only those
features. Milestone 1 is read-only battery telemetry. Milestone 2 adds the discovery
system, dual `energy_*` / `charge_*` sysfs support, and a capabilities panel.

The API binds to `127.0.0.1:8585`. Remote access is intended to go through Tailscale later, not a public bind.

See [COMPATIBILITY.md](COMPATIBILITY.md) for tested laptops and charge-threshold behaviour.

## Layout

```
cmd/lapguard/main.go
internal/discovery/          # hardware / module / tool / threshold detection
internal/battery/provider.go
internal/battery/sysfs_provider.go
internal/battery/mock_provider.go
internal/thresholds/         # sysfs writes or `tlp setcharge` when supported
internal/api/handlers.go
internal/config/             # flags + ~/.config/lapguard/config.json
testdata/sysfs/BAT0/
web/
```

## Development (no root)

The development box is an HP ProDesk without a battery. The production box is a Fujitsu Lifebook with a real pack. `auto` tries sysfs and falls back to the mock provider when no battery is present. Discovery always runs against the real (or `-sysfs-root`) tree.

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

To exercise the sysfs reader against the checked-in fixture:

```bash
go run ./cmd/lapguard -provider sysfs -sysfs-root testdata/sysfs
```

On the Lifebook, the default `auto` provider reads `/sys/class/power_supply` as an unprivileged user.

First start writes `~/.config/lapguard/config.json` (listen, provider, `threshold_method: auto`). Flags override the file. Features are re-detected every launch; `threshold_method` stays `auto` unless you pin `sysfs`, `tlp`, or `none`.

## API

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/v1/telemetry` | Battery snapshot, power (W), health (%) |
| GET | `/api/v1/capabilities` | UI capabilities: feature list with `detection_method`, `recommendation`, `why_not` |
| GET | `/api/v1/discover` | Full `CapabilityReport` (re-runs detection) |

Power is `power_now` when present, otherwise `voltage_now * current_now`. Sign is negative while discharging. Health is `energy_full / energy_full_design` or `charge_full / charge_full_design`. Missing sysfs files are omitted from the payload and listed in `missing_fields` unless an energy/charge equivalent exists.

Charge thresholds, when the hardware allows them:

- **sysfs** — write start/stop to `/sys/class/power_supply/BATx/charge_control_*`
- **tlp** — `tlp setcharge START STOP BATX`
- **none** — feature disabled (Fujitsu Lifebook is the reference case)

Writes are implemented in `internal/thresholds` but are not exposed over HTTP in this milestone.

## Production notes

- Default bind is loopback only.
- Do not run the process as root during development.
- Build the UI with `cd web && npm run build`, then `go run ./cmd/lapguard` serves `web/dist`.
- Shutdown, Docker drain, notifications and authentication remain out of scope; discovery will still report whether Docker is present.
