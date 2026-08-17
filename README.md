# LapGuard

Lightweight Linux laptop power manager for machines that stay on as 24/7 home servers.

Milestone 1 is read-only battery telemetry: a Go API, a sysfs/mock battery layer, and a mobile-first Svelte dashboard. It binds to `127.0.0.1:8585`. Remote access is intended to go through Tailscale later, not a public bind.

## Layout

```
cmd/lapguard/main.go
internal/battery/provider.go
internal/battery/sysfs_provider.go
internal/battery/mock_provider.go
internal/api/handlers.go
testdata/sysfs/BAT0/
web/
```

## Development (no root)

The development box is an HP ProDesk without a battery. The production box is a Fujitsu Lifebook with a real pack. `auto` tries sysfs and falls back to the mock provider when no battery is present.

```bash
export PATH="$HOME/.local/go/bin:$HOME/.local/node/bin:$PATH"

# tests use testdata/sysfs, not hardware
go test ./...

# API (mock on a batteryless machine)
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

## API

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/api/v1/telemetry` | Battery snapshot, power (W), health (%) |
| GET | `/api/v1/capabilities` | Provider, available sysfs fields, feature flags |

Power is `power_now` when present, otherwise `voltage_now * current_now`. Sign is negative while discharging. Health is `energy_full / energy_full_design * 100`. Missing sysfs files are omitted from the payload and listed in `missing_fields`.

## Production notes

- Default bind is loopback only.
- Do not run the process as root during development.
- Build the UI with `cd web && npm run build`, then `go run ./cmd/lapguard` serves `web/dist`.
- Shutdown, Docker control, charge-threshold writes, notifications and authentication are out of scope for milestone 1.
