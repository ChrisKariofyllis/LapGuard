# Native Linux installation (alpha)

LapGuard is a local daemon. There is no `curl | sudo bash` installer and no
sudoers file. The battery safety controller is **dry-run only**: even after
install, LapGuard will not stop Docker containers or shut down the host.

Prefer the **user systemd unit** for alpha. The system unit is for machines
that should run LapGuard without a login session.

## Paths

| Layout | Config | Outage log | Binary |
| --- | --- | --- | --- |
| User (recommended) | `~/.config/lapguard/config.json` | `~/.config/lapguard/events.db` | `~/.local/bin/lapguard` or `/usr/local/bin/lapguard` |
| System | `/etc/lapguard/config.json` | `/var/lib/lapguard/events.db` | `/usr/local/bin/lapguard` |

You own the config file. Mode `0600` is required so webhook URLs and tokens
are not world-readable. Do not put secrets in unit files or the environment.

## Prebuilt binary

1. Download `lapguard_<version>_linux-amd64` or `linux-arm64` plus `SHA256SUMS`
   from a **published** GitHub Release (tagging only creates a draft; a
   maintainer must publish it).
2. Verify:

   ```bash
   sha256sum -c SHA256SUMS --ignore-missing
   ```

3. Install the binary (example: amd64, user install):

   ```bash
   install -m 0755 lapguard_*_linux-amd64 "$HOME/.local/bin/lapguard"
   ```

   Or system-wide:

   ```bash
   sudo install -o root -g root -m 0755 lapguard_*_linux-amd64 /usr/local/bin/lapguard
   ```

4. Run once to write the default config, then stop it (`Ctrl+C`):

   ```bash
   lapguard
   ```

5. Open `http://127.0.0.1:8585`. Do not change the bind to `0.0.0.0`.

## User systemd unit

The template is [`contrib/systemd/lapguard.user.service`](../contrib/systemd/lapguard.user.service).
It expects the binary at `~/.local/bin/lapguard`. If you installed to
`/usr/local/bin`, edit `ExecStart`.

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
sudo install -d -o root -g lapguard -m 0750 /etc/lapguard
sudo install -d -o lapguard -g lapguard -m 0750 /var/lib/lapguard
sudo install -o root -g root -m 0644 contrib/systemd/lapguard.service /etc/systemd/system/lapguard.service
sudo systemctl daemon-reload
sudo systemctl enable --now lapguard.service
```

Optional helpers (do not grant extra privileges):

- [`contrib/sysusers.d/lapguard.conf`](../contrib/sysusers.d/lapguard.conf)
- [`contrib/tmpfiles.d/lapguard.conf`](../contrib/tmpfiles.d/lapguard.conf)

`ProtectSystem=strict` and `ProtectHome=true` still allow reading
`/sys/class/power_supply`. The service user must be able to write
`/etc/lapguard/config.json` so the dashboard can save settings.

There is **no** sudoers drop-in in this repository. Do not add
`lapguard ALL=(ALL) NOPASSWD: ALL` or equivalent.

## What this alpha will not do

- Execute `docker stop` / `docker kill`
- Execute `systemctl poweroff`, `shutdown`, `reboot`, or `sync`
- Install via a remote shell pipe
- Expose the API on a public address
