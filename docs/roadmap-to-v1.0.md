# Roadmap to v1.0 stable

LapGuard is an open-source alpha (currently **v0.9.x**). This document lists what
must ship before a **v1.0 stable** release, what could follow, and what the
project intentionally will not do.

It is organized around four priorities:

1. **Security** — API protection (GET routes or default auth)
2. **Stability** — API freeze, soak testing
3. **Usability** — installation experience, documentation
4. **Safety** — OS permission documentation, recovery plan

The production reference machine is a **Fujitsu Lifebook A3510** (BAT1, derived
power, charge thresholds `none`). v1.0 means “safe to run as a home-server power
manager on loopback + Tailscale,” not “hands-off datacenter automation.”

---

## Current baseline (alpha)

What exists today:

- Battery telemetry, AC watcher, SQLite outage log, notifications (ntfy / Telegram / Discord)
- Optional Bearer token auth (`lapguard auth generate`); **default is auth off**
- When auth is on, **POST/PUT require a token; GET routes stay readable**
- `protect_get` exists in config views but is always `false` (not wired)
- Experimental manual Docker drain / poweroff and smart **auto-drain** (all **off by default**)
- Safety controller records intended actions; it does **not** auto-execute host commands
- Loopback bind (`127.0.0.1:8585`); Tailscale Serve documented, not automated
- Prebuilt linux-amd64 / linux-arm64 binaries + `SHA256SUMS`; systemd unit templates under `contrib/`
- No `curl | sudo bash` installer; charge-threshold **writes** not wired

Known gaps called out in README / install docs:

- Real actions may return **503** after software gates pass (OS permission gap)
- Docs still reference older alpha versions in places (e.g. v0.9.3 checklist)
- Auto-drain and real actions lack long-running field validation
- Release workflow may require manual GitHub pre-release publish

---

## 1. Blockers (must ship before v1.0)

These are release gates. v1.0 should not ship until each item has a written
decision, implementation (where applicable), and verification.

### Security

| Item | Why it blocks v1.0 | Target outcome |
| --- | --- | --- |
| **Resolve GET vs auth policy** | With Tailscale Serve, unauthenticated GET exposes telemetry, config shape, discover (serials/hostname), safety/auto-drain state. Unauthenticated POST/PUT is blocked only when auth is enabled — but auth defaults **off**. | Ship **one** of: (A) `auth.enabled=true` by default for non-loopback or packaged installs, with documented localhost dev override; or (B) wire `protect_get` so all sensitive GET routes require Bearer when enabled; or (C) both. Document the chosen model in README and install.md. |
| **Secure deployment default** | Alpha explicitly warns that anyone who can reach the port can change settings if auth is off. | First-run / install path must prompt or fail closed when exposing beyond loopback (e.g. detect Tailscale Serve, print “run `lapguard auth generate`”). |
| **Auth migration story** | Existing alpha `config.json` files have `auth.enabled=false`. | Document upgrade steps; consider one-time startup warning when auth is off and listen is not loopback-only. |
| **Secret handling audit** | Token hashes, webhooks, and discover output are partially redacted today. | Confirm no regressions in logs, audit log, API, and `lapguard discover --report` for v1.0 contract tests. |

### Stability

| Item | Why it blocks v1.0 | Target outcome |
| --- | --- | --- |
| **HTTP API freeze** | Config keys and routes still change between alpha tags. | Declare **`/api/v1/*` stable** at v1.0: breaking changes only in `/api/v2`. Publish an API stability section (routes, config schema, deprecation policy). |
| **Config schema version** | Disk config may drift across alphas. | Add explicit config version field or documented migration table; test load/save round-trips from v0.9.x → v1.0. |
| **Soak testing on reference hardware** | CI uses mocks/fixtures; no multi-day battery + AC + notification run on A3510. | Minimum **7–14 day** soak on Fujitsu A3510: telemetry poll, AC debounce, notifications dry-run, auth on, dashboard over Tailscale Serve. Record results in `docs/` (similar to hardware validation doc). |
| **Real-action validation matrix** | Manual poweroff / docker-drain / auto-drain use fakes in CI only. | Document which combinations were exercised on real hardware (dry-run vs real) and which remain “operator opt-in only.” v1.0 label requires explicit scope, not implied safety. |
| **Release reproducibility** | Manual pre-release publish and version strings scattered across docs. | Single source of truth for version; CI release artifacts match tag; checklist for maintainer publish. |

### Usability

| Item | Why it blocks v1.0 | Target outcome |
| --- | --- | --- |
| **Installation path for non-developers** | Today: download binary, chmod, optional systemd copy. High friction vs “install and forget.” | Ship a **supported install method**: e.g. documented `install.sh` (no remote pipe), `.deb`/`.rpm`, or improved `contrib/systemd` + `docs/install.md` quick path with copy-paste units. |
| **First-run experience** | Blank dashboard; auth optional; easy to expose unauthenticated API. | Minimal first-run: health check, auth recommendation, notification test, link to safety gates. CLI `lapguard doctor` or expanded `healthz` hints acceptable. |
| **Documentation pass for v1.0** | Alpha-testing, CONTRIBUTING, README reference mixed versions; auto-drain not in all checklists. | One **v1.0 operator guide**: install, auth, Tailscale, notifications, enabling real actions (with warnings), auto-drain. Retire or retarget alpha-only language. |
| **Compatibility baseline** | COMPATIBILITY.md lists one verified laptop + mocks. | v1.0 lists **minimum** supported environments (kernel, systemd user vs system unit, Docker optional) and tested distros for reference hardware. |

### Safety

| Item | Why it blocks v1.0 | Target outcome |
| --- | --- | --- |
| **OS permission documentation** | Poweroff/Docker often fail with 503 for normal users; project refuses to install sudoers/polkit. | Dedicated **“Host permissions”** doc: what LapGuard runs (`systemctl poweroff`, `docker`, `sync`), what fails without policy, **optional** polkit/logind/sudo snippets the **operator** may apply (clearly external, not installed by LapGuard). |
| **Recovery plan** | Operators need to know how to recover from misconfiguration or a stuck sequence. | Document: lost API token (`auth rotate` / `auth disable`), disabling real actions via config on disk, stopping systemd unit, AC plug-in aborting auto-drain, manual `docker start` after drain, “machine still up after failed poweroff.” |
| **Auto-drain operator contract** | v0.9.5 adds state machine + notification gate; not field-proven. | v1.0 doc: exact gates, timeout = YES behavior, NO = continue on battery, no ntfy callback URLs, when `commands_executed` is false vs true. Soak test includes at least one dry-run auto-drain cycle. |
| **Safe defaults invariant** | Trust is lost if an upgrade enables real execution. | Contract test + release checklist: `actions.real_enabled=false`, `safety.dry_run=true`, `docker.stop_enabled=false`, `auto_drain.enabled=false` on fresh install and after upgrade unless explicitly set. |

---

## 2. Nice-to-haves (could ship before or after v1.0)

Valuable but not required to call the release “stable.”

### Security & API

- Read-only API role (token that can GET but not POST)
- Rate limiting on auth failures and action POSTs
- CSP / security headers for embedded dashboard
- Optional mTLS or Tailscale identity headers (document-only integration)

### Stability & quality

- Config file watch / reload without full restart (today: disk edits need restart)
- Broader hardware matrix (ThinkPad, Dell, ASUS) with automated fixture tests
- Longer CI: race tests, fuzz on config JSON, `-race` on critical packages
- Automated GitHub Release publish from CI
- Prometheus `/metrics` or structured health beyond `healthz`

### Usability

- Package repositories (AUR, Copr, PPA) maintained by community or maintainer
- Desktop notification or tray helper (out of scope for core daemon today)
- Dashboard: guided “enable notifications” and “enable auth before Serve” wizard
- `lapguard discover --report` integrated into dashboard export button
- Charge-threshold **write** support (sysfs / `tlp setcharge`) behind explicit API + safety gates

### Safety & features

- Auto-drain: configurable “timeout means NO” (today: timeout = YES)
- Graceful shutdown of specific systemd user services before docker/sync/poweroff
- Battery health trending / export
- Webhook **outbound** status callbacks (not ntfy inbound actions — still no token in URLs)

---

## 3. Non-goals (intentionally out of scope)

These align with existing alpha rules in README, CONTRIBUTING, and install.md.
v1.0 will **not** promise them unless the project direction explicitly changes.

### Security & exposure

- Binding the HTTP server to `0.0.0.0` or a public interface by default
- Tailscale **Funnel** or automatic public exposure
- Embedding Bearer tokens in ntfy/Telegram action URLs
- Running the daemon as **root** or installing sudoers/polkit rules **on behalf of the user**
- Replacing Tailscale/SSH with a bespoke remote access protocol

### Safety & automation

- Automatic drain or poweroff **without** a successful user notification first
- UPS / PDU integration (USB, SNMP, NUT)
- Unattended “always power off at X%” with no confirmation path
- Guaranteeing poweroff succeeds without the operator understanding OS permissions
- CI or default install executing real `poweroff`, `docker stop`, or `sync` on the host

### Product scope

- `curl | sudo bash` one-line installer from the internet
- Windows / macOS battery management (Linux laptop home server only)
- Mobile native apps (dashboard over Tailscale is enough)
- Cloud-hosted multi-tenant LapGuard as a service
- Charge-threshold writes **silently enabled** on upgrade

### v1.0 label does not mean

- Production datacenter SLA or 24/7 vendor support
- Every laptop model works identically (discovery remains truth-based)
- Docker drain is safe on every compose stack without operator review
- Legal/compliance certification (SOC2, etc.)

---

## Suggested sequencing

```text
Phase A — Security & docs (blockers)
  auth/default policy → install.md + README → recovery + OS permissions docs

Phase B — Stability
  API freeze doc → config migration tests → reference hardware soak

Phase C — Usability polish
  install path → first-run/doctor → v1.0 operator guide → version doc sync

Phase D — Tag v1.0
  release checklist → hardware validation appendix → drop “alpha” badges where appropriate
```

---

## v1.0 release checklist (summary)

Use this as the final gate before tagging `v1.0.0`:

- [ ] Auth policy implemented and documented (GET protection and/or default auth)
- [ ] `/api/v1` stability policy published
- [ ] Fresh install safe defaults verified by CI contract tests
- [ ] Reference hardware soak completed (documented)
- [ ] `docs/install.md` + operator guide updated for v1.0
- [ ] OS permissions + recovery runbooks published
- [ ] Auto-drain and real-action scope explicitly stated in release notes
- [ ] `make lint`, `go test ./...`, `make build`, smoke test pass on release artifacts
- [ ] No known critical security issues on loopback + Tailscale deployment model

---

## Related documents

| Document | Purpose |
| --- | --- |
| [README.md](../README.md) | Feature overview, alpha warnings, API table |
| [docs/install.md](install.md) | Native install, systemd, Tailscale Serve |
| [docs/alpha-testing.md](alpha-testing.md) | Public tester checklist (to be superseded for v1.0) |
| [docs/v0.9.4-alpha-hardware-validation.md](v0.9.4-alpha-hardware-validation.md) | Example hardware validation format |
| [COMPATIBILITY.md](../COMPATIBILITY.md) | Machine matrix and sysfs conventions |
| [CONTRIBUTING.md](../CONTRIBUTING.md) | Dev setup; no real host commands in CI |

---

*Last updated: 2026-08-20 — reflects codebase through v0.9.5-alpha (smart auto-drain, uncommitted).*
