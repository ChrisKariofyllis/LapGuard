# Native Linux installation (alpha)

LapGuard is a local daemon. There is no `curl | sudo bash` installer and no
sudoers file.

> **Warning:** Real actions (Docker drain and host poweroff) are experimental
> and **disabled by default**. They are not safe for production yet. Do not
> enable them on an important machine. This alpha remains dry-run unless you
> explicitly set `actions.real_enabled=true` and `safety.dry_run=false`.
> Automatic low-battery shutdown is **not implemented**.
>
> Even then, a request must pass authentication (when enabled), exact
> confirmation (`POWER_OFF` / `STOP_DOCKER`), known AC-disconnected state, a
> discharging battery, cooldown, and an in-progress lock.
> `GET /api/v1/actions/status` reports those gates without running commands.

Charge-threshold writes are **not** performed. Prefer the **user systemd unit**
for alpha. The system unit is for machines that should run LapGuard without a
login session.

## Paths

| Layout | Config | Outage log | Binary |
| --- | --- | --- | --- |
| User (recommended) | `~/.config/lapguard/config.json` | `~/.config/lapguard/events.db` | `~/.local/bin/lapguard` or `/usr/local/bin/lapguard` |
| System | `/etc/lapguard/config.json` | `/var/lib/lapguard/events.db` | `/usr/local/bin/lapguard` |

You own the config file. LapGuard writes it mode `0600` so webhook URLs and
tokens are not world-readable. Do not put secrets in unit files or the
environment.

The service account must **own** `/etc/lapguard` (not merely be in the group)
so the dashboard can persist settings. A `0750 root:lapguard` directory is
not writable by user `lapguard`.

## Prebuilt binary

Release artifacts are `lapguard_<version>_linux-amd64` and
`lapguard_<version>_linux-arm64` plus `SHA256SUMS`. They are built with
`-tags embedui` (dashboard inside the binary). Tagging only creates a
**draft** GitHub Release; a maintainer must publish it.

1. Download the binary for your architecture and `SHA256SUMS` from a
   **published** [GitHub Release](https://github.com/ChrisKariofyllis/LapGuard/releases).
2. Verify:

   ```bash
   sha256sum -c SHA256SUMS --ignore-missing
   ```

3. Install the binary (user install, amd64 example):

   ```bash
   install -m 0755 lapguard_*_linux-amd64 "$HOME/.local/bin/lapguard"
   ```

   arm64:

   ```bash
   install -m 0755 lapguard_*_linux-arm64 "$HOME/.local/bin/lapguard"
   ```

   Or system-wide (replace `amd64` with `arm64` on ARM machines):

   ```bash
   sudo install -o root -g root -m 0755 lapguard_*_linux-amd64 /usr/local/bin/lapguard
   ```

4. For the **user** unit, run once as yourself to write
   `~/.config/lapguard/config.json`, then stop it (`Ctrl+C`):

   ```bash
   lapguard
   ```

   For the **system** unit, skip this step; the service user writes
   `/etc/lapguard/config.json` on first start.

5. Open `http://127.0.0.1:8585`. Leave the default listen address
   (`127.0.0.1:8585`). Do not bind `0.0.0.0` and do not bind a Tailscale
   `100.x.y.z` address. For tailnet access, use Tailscale Serve in front of
   that localhost port — see [Remote access with Tailscale Serve](#remote-access-with-tailscale-serve).

Privacy-safe hardware report (does not start the HTTP server):

```bash
lapguard discover --report > lapguard-compatibility-report.json
```

## User systemd unit

The template is [`contrib/systemd/lapguard.user.service`](../contrib/systemd/lapguard.user.service).
It expects the binary at `~/.local/bin/lapguard`. If you installed to
`/usr/local/bin`, edit `ExecStart`.

`ExecStart` passes `-web-dir none` so a release binary serves the **embedded**
dashboard instead of looking for `web/dist`.

```bash
mkdir -p ~/.config/systemd/user
cp contrib/systemd/lapguard.user.service ~/.config/systemd/user/lapguard.service
systemctl --user daemon-reload
systemctl --user enable --now lapguard.service
```

Linger is required if the service should survive logout:

```bash
loginctl enable-linger "$USER"
```

The unit sets `NoNewPrivileges=true` and does not request `CAP_SYS_BOOT`,
Docker, or sudo.

## System systemd unit (optional)

Do **not** run LapGuard as root. Create a dedicated account with no sudoers
entry:

```bash
sudo useradd --system --home /var/lib/lapguard --shell /usr/sbin/nologin lapguard
sudo install -o root -g root -m 0755 lapguard_*_linux-amd64 /usr/local/bin/lapguard
sudo install -d -o lapguard -g lapguard -m 0750 /etc/lapguard
sudo install -d -o lapguard -g lapguard -m 0750 /var/lib/lapguard
sudo install -o root -g root -m 0644 contrib/systemd/lapguard.service /etc/systemd/system/lapguard.service
sudo systemctl daemon-reload
sudo systemctl enable --now lapguard.service
```

On ARM, install `lapguard_*_linux-arm64` instead of the amd64 binary.

Optional helpers (do not grant extra privileges):

- [`contrib/sysusers.d/lapguard.conf`](../contrib/sysusers.d/lapguard.conf)
- [`contrib/tmpfiles.d/lapguard.conf`](../contrib/tmpfiles.d/lapguard.conf)

`ProtectSystem=strict` and `ProtectHome=true` still allow reading
`/sys/class/power_supply`. `/etc/lapguard` must be owned by `lapguard` so
`config.json` (mode `0600`) can be created and updated.

There is **no** sudoers drop-in in this repository. Do not add
`lapguard ALL=(ALL) NOPASSWD: ALL` or equivalent.

## Remote access with Tailscale Serve

LapGuard must keep listening on **`127.0.0.1:8585`**. Tailscale Serve is an
external reverse proxy. It is not built into the LapGuard process.

```
LapGuard                  ->  127.0.0.1:8585
Tailscale Serve           ->  proxies that localhost service to your tailnet
Remote Tailscale devices  ->  open the dashboard over the tailnet
```

Do **not** bind LapGuard to a Tailscale `100.x.y.z` address, to `0.0.0.0`, or
to any public interface just to use Tailscale. Existing `-listen` / config
listen flags still work; leave them on loopback. LapGuard never runs `sudo`
or changes Tailscale state. `lapguard tailscale check` may run read-only
`tailscale status` / `ip -4` / `serve status` / `version` only.

### 1. Install Tailscale separately

Install Tailscale and authenticate the laptop on your tailnet using Tailscale's
own docs. That is outside LapGuard.

### 2. Start LapGuard on localhost

```bash
./lapguard -web-dir none
```

(`-web-dir none` uses the dashboard embedded in a release binary. Systemd
units already pass this flag.)

Confirm the local dashboard:

```
http://127.0.0.1:8585
```

### 3. Proxy it with Tailscale Serve

Prefer an explicit localhost target. On current Tailscale CLIs:

```bash
sudo tailscale serve --bg http://127.0.0.1:8585
```

If that syntax is rejected, check the installed CLI:

```bash
tailscale serve --help
```

Then inspect:

```bash
tailscale status
tailscale ip -4
tailscale serve status
```

Serve is accessible **only inside the tailnet** when configured normally.
Use Tailscale ACLs to limit access to trusted users and devices.

**Do not use Tailscale Funnel.** Do not expose port 8585 on the public
Internet. Do not reconfigure LapGuard to listen on `0.0.0.0` for this.

LapGuard includes a read-only helper that does not start the HTTP server and
does not change Tailscale state:

```bash
lapguard tailscale instructions
lapguard tailscale check --pretty
```

`instructions` prints the setup text only. `check` looks up `tailscale` on
`PATH` and, if found, may run `tailscale status`, `tailscale ip -4`,
`tailscale serve status`, and `tailscale version`. It never runs `sudo`,
Funnel, or `tailscale serve --bg`. JSON always reports
`lapguard_listen` as `127.0.0.1:8585` and `recommended_access` as
`tailscale_serve`.

Serve was verified on a **Gigabyte Aero 16** (dashboard opened from a phone
on the same tailnet). That does not change the listen address.

### Security

When `auth.enabled` is **false** (the default), LapGuard has **no application-level
authentication**. Anyone who can reach the HTTP port can change settings.
Enable a Bearer token **before** exposing the dashboard over Tailscale:

```bash
lapguard auth generate
```

Store the printed token in a password manager. Send `Authorization: Bearer <token>`
on POST/PUT. GET telemetry stays readable without a token in this alpha.

With Tailscale Serve, **Tailscale identity/ACLs plus the API token** are the
security boundary. Only trusted tailnet users and devices should be allowed.
**Do not use Tailscale Funnel.** Do not expose port 8585 on the public Internet.

Tester steps (checksum, localhost, Serve, ntfy, preview) are in
[alpha-testing.md](alpha-testing.md).

### Troubleshooting

Verify LapGuard is listening:

```bash
curl http://127.0.0.1:8585/api/v1/healthz
```

Verify Tailscale:

```bash
tailscale status
tailscale ip -4
```

Inspect Serve:

```bash
tailscale serve status
```

Check the local listening socket (should be loopback, not `0.0.0.0`):

```bash
ss -ltn | grep 8585
```

If Serve cannot reach the app, confirm LapGuard is still bound to
`127.0.0.1:8585` and that you did not enable Funnel. To remove a background
Serve config, use `tailscale serve off` (see `tailscale serve --help`).

Or run the built-in probe (read-only; never executes sudo):

```bash
lapguard tailscale check --pretty
```

## Optional API token

Authentication is **off** by default so local development on `127.0.0.1:8585`
keeps working. Enable a Bearer token before Tailscale Serve:

1. Start LapGuard locally on `127.0.0.1:8585`.
2. Generate a token on the laptop:

   ```bash
   lapguard auth generate
   ```

3. Store the printed token in a password manager. It is shown **once**.
   Only a SHA-256 hash is saved in `config.json` (mode `0600`).
4. Authentication is enabled by that command. Confirm with
   `lapguard auth status`.
5. Configure Tailscale Serve as above. Do **not** use Funnel or expose
   port 8585 publicly.
6. Send `Authorization: Bearer <token>` on POST/PUT (settings, notification
   test, safety simulation, action preview). GET telemetry, capabilities,
   discover, power, events, safety, healthz, config, auth/status, and
   actions/status stay readable without a token in this alpha.

Rotate or recover without the old token (local config file access):

```bash
lapguard auth rotate    # prints a new token; invalidates the previous hash
lapguard auth disable   # turns auth off
```

HTTP `POST /api/v1/auth/rotate` never returns a plaintext token. Disable over
HTTP requires a valid Bearer token when auth is on, or a loopback request to
`127.0.0.1` when auth is off. Remote callers cannot enable or disable auth
without a token.

Real Docker drain and host poweroff are experimental, disabled by default, and
are **not** safe for production. Do not enable them on an important machine.
Automatic low-battery shutdown is not implemented. This alpha does not install
sudoers, polkit, or root permissions.

Even when `actions.real_enabled=true` and `safety.dry_run=false`, LapGuard
runs `systemctl poweroff` / `poweroff` (and optionally the Docker CLI) as the
service user. A normal Ubuntu/Debian user without an existing polkit or
logind policy typically cannot power the machine off. In that case the API
returns **503** (`action executor is unavailable`). That permission gap is
external to LapGuard; do not add sudoers or `CAP_SYS_BOOT` to “fix” it.

CI and unit/integration tests never execute host poweroff, shutdown, reboot,
sync, or Docker. They use temporary fake executables that only record argv.

`GET /api/v1/actions/status` is read-only. It reports `real_enabled`,
`safety_dry_run`, `require_ac_loss`, AC state, battery status/percent,
cooldown, and whether the recording or real executor would be selected.
It never exposes command arguments or secrets.

Public alpha testers should use [alpha-testing.md](alpha-testing.md).
`make smoke` / `scripts/smoke-test.sh` never enables real actions.

## What this alpha will not do

- Automatically stop Docker or power off the host on low battery
- Execute real actions unless every safety gate passes (`actions.real_enabled=true`,
  `safety.dry_run=false`, explicit confirmation, AC disconnected, battery
  discharging, cooldown clear)
- Treat Docker drain or host poweroff as production-safe
- Write charge start/stop thresholds to sysfs or via `tlp setcharge`
- Install via a remote shell pipe
- Bind the HTTP server to `0.0.0.0`, a Tailscale `100.x` address, or a public interface
- Run `sudo`, Funnel, or `tailscale serve --bg` on your behalf (`lapguard tailscale check` is read-only)
- Expose the API on a public address
- Add sudoers, polkit, or unrestricted root permissions
