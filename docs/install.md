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

5. Open `http://127.0.0.1:8585`. Do not change the bind to `0.0.0.0`.

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

## What this alpha will not do

- Execute `docker stop` / `docker kill`
- Execute `systemctl poweroff`, `shutdown`, `reboot`, or `sync`
- Write charge start/stop thresholds to sysfs or via `tlp setcharge`
- Install via a remote shell pipe
- Expose the API on a public address
