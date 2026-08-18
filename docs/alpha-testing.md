# Public alpha testing checklist

This is the tester checklist for **LapGuard v0.9.3-alpha**.

Real Docker drain and host poweroff are **experimental and disabled by default**.
They are **not** safe for production. Automatic low-battery shutdown is **not
implemented**. Do **not** set `actions.real_enabled=true` or
`safety.dry_run=false` on an important machine.

Do not run `docker stop`, `systemctl poweroff`, `shutdown`, `reboot`, or `sync`
as part of this checklist.

## Automated smoke test

From a built binary (never enables real actions; dangerous POSTs run only
against a local mock process):

```bash
make build
sh scripts/smoke-test.sh bin/lapguard
```

Against an already running local daemon (GET + preview only; refuses to
continue if `real_enabled` is not false):

```bash
LAPGUARD_URL=http://127.0.0.1:8585 sh scripts/smoke-test.sh
# if you enabled auth:
LAPGUARD_URL=http://127.0.0.1:8585 LAPGUARD_TOKEN='…' sh scripts/smoke-test.sh
```

## Manual checklist

### Native binary

- [ ] Download `lapguard_<version>_linux-amd64` or `linux-arm64` from a
      **published** [GitHub Release](https://github.com/ChrisKariofyllis/LapGuard/releases)
- [ ] Download `SHA256SUMS` from the same release
- [ ] Verify: `sha256sum -c SHA256SUMS --ignore-missing`
- [ ] `chmod +x` the binary (or `install` it as in [install.md](install.md))
- [ ] There is no `curl | sudo bash` installer

### Localhost dashboard

- [ ] Start LapGuard without extra privileges: `./lapguard` or `./lapguard -web-dir none`
- [ ] Listen address stays **`127.0.0.1:8585`** (do not bind `0.0.0.0` or a Tailscale `100.x` address)
- [ ] Open `http://127.0.0.1:8585`
- [ ] `curl http://127.0.0.1:8585/api/v1/healthz` returns `"status":"ok"`
- [ ] Dashboard shows battery telemetry (or mock telemetry on a machine without a pack)

### Read-only API

- [ ] `GET /api/v1/healthz`
- [ ] `GET /api/v1/telemetry`
- [ ] `GET /api/v1/capabilities`
- [ ] `GET /api/v1/discover` (do **not** share this JSON; use `lapguard discover --report` for GitHub)
- [ ] `GET /api/v1/actions/status` shows `real_enabled: false`, `safety_dry_run: true`, `commands_executed: false`, `executor: "recording"`
- [ ] `GET /api/v1/actions/preflight` is read-only (`commands_executed: false`) and says a restart is required after editing `config.json` on disk

### Authentication

Auth is **off** by default so localhost development works. Enable it before
Tailscale Serve.

- [ ] `lapguard auth generate` prints a token **once**
- [ ] Store the token in a password manager (not in the unit file or a gist)
- [ ] `lapguard auth status` shows enabled; `config.json` mode `0600` holds a hash only
- [ ] `GET /api/v1/telemetry` still works **without** a token
- [ ] `POST /api/v1/actions/preview` without `Authorization: Bearer` returns **401**
- [ ] The same POST with a valid Bearer token succeeds
- [ ] HTTP `GET /api/v1/config` and `GET /api/v1/auth/status` never show the token or `token_hash`

### Tailscale Serve

- [ ] Install and authenticate Tailscale yourself (outside LapGuard)
- [ ] LapGuard still listens on `127.0.0.1:8585`
- [ ] `sudo tailscale serve --bg http://127.0.0.1:8585` (or the syntax from `tailscale serve --help`)
- [ ] Open the dashboard from another device **on the same tailnet**
- [ ] `ss -ltn | grep 8585` shows loopback, not `0.0.0.0`
- [ ] **Do not** enable Tailscale Funnel
- [ ] Restrict access with Tailscale ACLs; treat ACLs **plus** the Bearer token as the security boundary
- [ ] Optional: `lapguard tailscale check --pretty` (read-only; never runs `sudo` or Serve)

### ntfy (optional)

- [ ] Configure ntfy with **`dry_run: true`** first:

  ```json
  {
    "provider": "ntfy",
    "enabled": true,
    "dry_run": true,
    "webhook_url": "https://ntfy.sh/your-topic-name"
  }
  ```

- [ ] Dashboard **Send test notification** (or `POST /api/v1/actions/test-notification`) reports dry-run OK and **no HTTP** is sent
- [ ] `GET /api/v1/config` does **not** return the webhook URL (only `webhook_configured: true`)
- [ ] After dry-run looks right, turn `dry_run` off and send a real ntfy test to your topic
- [ ] Treat the topic URL as a secret; never commit `config.json`

### Action preview (safe)

- [ ] `POST /api/v1/actions/preview` with `Content-Type: application/json` and body `{}`
- [ ] Response has `commands_executed: false`
- [ ] Intended plan is labels such as `sync` / `poweroff` (and `stop_docker` only if that intent is stored)
- [ ] Response does not include command strings (`systemctl`, `docker stop`, shell interpolation)
- [ ] Dashboard shows **Real actions disabled** and keeps Shut down / Stop Docker buttons disabled

### Confirm no real actions by default

- [ ] `GET /api/v1/config` → `actions.real_enabled` is false, `safety.dry_run` is true
- [ ] `POST /api/v1/actions/poweroff` with `{"confirm":"POWER_OFF"}` is **rejected** (409) and `commands_executed` is false
- [ ] `POST /api/v1/actions/docker-drain` with `{"confirm":"STOP_DOCKER"}` is **rejected** (409) and `commands_executed` is false
- [ ] Leave `actions.real_enabled=false` and `safety.dry_run=true`
- [ ] Do **not** perform a real poweroff or Docker stop as part of alpha testing

## Hardware notes (if you file a compatibility report)

- Fujitsu Lifebook A3510: **BAT1**, `charge_*`, no `power_now`, no `temp`, no charge thresholds, derived battery-side power, TLP present but cannot set thresholds. See [COMPATIBILITY.md](../COMPATIBILITY.md).
- Gigabyte Aero 16 (Zorin OS): **BAT1**, `charge_*`, `asus_wmi`, no thresholds on the tested machine, derived power, runtime estimate while discharging. See [COMPATIBILITY.md](../COMPATIBILITY.md).

Use `lapguard discover --report` (privacy-safe) instead of pasting `GET /api/v1/discover`.
