# Native Linux installation (alpha)

LapGuard is a local daemon. There is no `curl | sudo bash` installer and no
sudoers file. The battery safety controller is **dry-run only**: even after
install, LapGuard will not stop Docker containers or shut down the host.
Charge-threshold writes are **not** performed.

Prefer the **user systemd unit** for alpha. The system unit is for machines
that should run LapGuard without a login session.

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
listen flags still work; leave them on loopback. LapGuard never runs
`tailscale` or `sudo` for you.

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

### Security

LapGuard has **no application-level authentication**. With this setup,
**Tailscale identity and ACLs are the security boundary**. Only trusted
Tailscale users and devices should be allowed to reach the dashboard. Do not
use Funnel or any other public Internet exposure.

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

## What this alpha will not do

- Execute `docker stop` / `docker kill`
- Execute `systemctl poweroff`, `shutdown`, `reboot`, or `sync`
- Write charge start/stop thresholds to sysfs or via `tlp setcharge`
- Install via a remote shell pipe
- Bind the HTTP server to `0.0.0.0`, a Tailscale `100.x` address, or a public interface
- Run `tailscale`, Funnel, or `sudo` on your behalf
- Expose the API on a public address
