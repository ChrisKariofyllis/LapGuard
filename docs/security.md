# LapGuard API authentication (v0.9.6-alpha)

This is the operator-facing auth model. GET routes stay readable for monitoring.
PUT/POST from a remote client (including Tailscale Serve) need a Bearer token.
Loopback (`127.0.0.1` / `localhost` / `::1`) may omit the token so the local
dashboard keeps working.

## Defaults

| Setting | Default |
| --- | --- |
| `auth.enabled` | **true** |
| `auth.allow_loopback_no_token` | **true** |
| GET routes | public (no token) |
| PUT/POST from loopback | allowed without a token |
| PUT/POST from remote | **401** without `Authorization: Bearer` |

Loopback is both the TCP peer **and** the `Host` header. Tailscale Serve
connects from `127.0.0.1` but sends a MagicDNS `Host`, so those requests are
**remote** and still require a token.

Tokens in query strings (`?token=` / `?access_token=`) are always rejected.

## First start

If auth is enabled and `config.json` has no `token_hash`, the daemon mints a
token with `crypto/rand` (32 bytes, `lg_` prefix), stores **only**
`sha256:<hex>` in the config file (mode `0600`), and prints the plaintext
**once** to stdout with a warning on stderr.

Store that value in a password manager. It is never written to slog, never
returned by HTTP, and never stored in plaintext.

Confirm flags (not the secret):

```bash
curl -sS http://127.0.0.1:8585/api/v1/auth/status
lapguard auth status
```

`GET /api/v1/auth/status` returns `auth_enabled`, `token_configured`,
`allow_loopback_no_token`, and timestamps. It **never** returns the token or
`token_hash`.

Rotate or recover on the laptop (file access; no old token required):

```bash
lapguard auth rotate    # prints a new token once; invalidates the previous hash
lapguard auth generate  # only if no hash exists yet
lapguard auth disable   # turns auth off (not recommended)
```

HTTP `POST /api/v1/auth/rotate` never returns a plaintext token.

## Remote clients (Tailscale)

Paste the token in the dashboard **Token setup** field (session storage only)
or send:

```http
Authorization: Bearer lg_…
Content-Type: application/json
```

Do **not** put the token in ntfy action URLs, unit files, or gists.

## Migration

| On-disk config | Result |
| --- | --- |
| No `auth` object | Defaults apply: `enabled=true`, `allow_loopback_no_token=true` |
| `"auth": { "enabled": false }` | **Kept off** (explicit alpha configs) |
| `"auth": { "enabled": true, "token_hash": "sha256:…" }` | Unchanged |

Restart after editing `config.json` on disk. Dashboard / `PUT /api/v1/config`
do not toggle these auth flags over HTTP.

## What this does not do

- Protect GET routes (`protect_get` stays false)
- Return the token from any HTTP handler
- Log plaintext tokens
- Install sudoers or bind `0.0.0.0`
