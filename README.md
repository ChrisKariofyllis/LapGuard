# LapGuard

Lightweight Linux laptop power manager for machines that stay on as 24/7 home servers.

The API binds to `127.0.0.1:8585`. Remote access is intended to go through Tailscale later, not a public bind.

**Milestone 2 is complete:** hardware auto-discovery and a capabilities UI. LapGuard probes the machine at runtime and enables only what it actually supports. See [COMPATIBILITY.md](COMPATIBILITY.md) for tested laptops and charge-threshold behaviour.

## Milestone 2 — Hardware Auto-Discovery & Capabilities

On startup (and on `GET /api/v1/discover`) LapGuard scans:

- `/sys/class/power_supply/` — batteries, AC adapters, available sysfs attributes
- `/proc/modules` — vendor modules (`thinkpad_acpi`, `fujitsu_laptop`, `asus_wmi`, …)
- Userspace tools — TLP (plus version / `tlp-stat` threshold support), UPower, ACPI, and related helpers

Charge-threshold method is chosen in order: **sysfs → tlp → none**. Unsupported hardware is not an error; the report includes `why_not`. The Fujitsu Lifebook is the reference edge case: `fujitsu_laptop` may load and TLP may be installed, yet charge control never registers, so the method is `none`.

Telemetry accepts both sysfs naming conventions (`energy_*` and `charge_*`). A missing file is omitted, not a failure.

Power semantics:

- `raw_power_now_supported` — sysfs `power_now` is present
- `derived_power_supported` — `current_now` and `voltage_now` can be multiplied
- Displayed watts prefer `power_now`, otherwise `current_now × voltage_now` (negative while discharging)
- `0 W` is a real reading when `current_now` is zero, not a missing capability

Health is `energy_full / energy_full_design` or `charge_full / charge_full_design`.

Threshold **writes** are wired in `internal/thresholds` (sysfs `charge_control_*` or `tlp setcharge START STOP BATX`) but are **not** exposed over HTTP yet.

## API

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/v1/telemetry` | Battery snapshot, power (W), health (%) |
| GET | `/api/v1/discover` | Re-run detection; full `CapabilityReport` |
| GET | `/api/v1/capabilities` | UI payload: per-feature `detection_method`, `recommendation`, `why_not` |

## UI

The dashboard Capabilities panel lists each feature as enabled or not supported, with the detection method and fallback reason. Tools and kernel modules are shown as chips. **Re-scan** calls `/api/v1/discover` and refreshes the panel.

## Layout

```
cmd/lapguard/main.go
internal/discovery/          # sysfs / modules / tools / threshold detection
internal/battery/            # sysfs + mock telemetry (energy_* and charge_*)
internal/thresholds/         # sysfs writes or tlp setcharge (no HTTP yet)
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

```bash
go run ./cmd/lapguard -provider sysfs -sysfs-root testdata/sysfs
```

On the Lifebook, the default `auto` provider reads `/sys/class/power_supply` as an unprivileged user.

First start writes `~/.config/lapguard/config.json` (listen, provider, `threshold_method: auto`). Flags override the file. Features are re-detected every launch; `threshold_method` stays `auto` unless you pin `sysfs`, `tlp`, or `none`.

## Production notes

- Default bind is loopback only.
- Do not run the process as root during development.
- Build the UI with `cd web && npm run build`, then `go run ./cmd/lapguard` serves `web/dist`.
- Shutdown, Docker drain, notifications and authentication remain out of scope; discovery still reports whether Docker is present.
