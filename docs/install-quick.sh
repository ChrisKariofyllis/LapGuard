#!/bin/bash
# Quick user-space installer for LapGuard (linux-amd64 / linux-arm64).
# Review this file, then run it locally. There is no curl|sudo bash installer.
#
# Usage:
#   sh docs/install-quick.sh                 # newest GitHub release (incl. prereleases)
#   sh docs/install-quick.sh 0.9.6-alpha     # pin a tag (with or without leading v)
#
# Does not: run as root, enable real actions, overwrite an existing config.json,
# start the service, or expose port 8585.

set -euo pipefail

REPO="ChrisKariofyllis/LapGuard"
VERSION="${1:-latest}"
INSTALL_DIR="${HOME}/.local/bin"
CONFIG_DIR="${HOME}/.config/lapguard"
SYSTEMD_DIR="${HOME}/.config/systemd/user"
WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT

if [ "$(id -u)" -eq 0 ]; then
	echo "Refusing to run as root. Install as your login user." >&2
	exit 1
fi

for cmd in curl uname mkdir chmod cat; do
	if ! command -v "$cmd" >/dev/null 2>&1; then
		echo "missing required command: $cmd" >&2
		exit 1
	fi
done

ARCH="$(uname -m)"
case "$ARCH" in
x86_64) ARCH="amd64" ;;
aarch64 | arm64) ARCH="arm64" ;;
*)
	echo "unsupported architecture: $ARCH (want x86_64 or aarch64)" >&2
	exit 1
	;;
esac

resolve_version() {
	local raw="$1"
	if [ "$raw" != "latest" ]; then
		echo "${raw#v}"
		return
	fi
	# /releases/latest skips prereleases; LapGuard tags are currently prereleases.
	local json tag
	json="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases?per_page=10")"
	tag="$(printf '%s\n' "$json" | grep -o '"tag_name": *"[^"]*"' | head -n1 | cut -d'"' -f4)"
	if [ -z "$tag" ]; then
		echo "could not resolve latest GitHub release for ${REPO}" >&2
		exit 1
	fi
	echo "${tag#v}"
}

VERSION="$(resolve_version "$VERSION")"
TAG="v${VERSION}"
ASSET="lapguard_${VERSION}_linux-${ARCH}"
BASE="https://github.com/${REPO}/releases/download/${TAG}"

echo "Installing LapGuard ${TAG} (${ARCH}) to ${INSTALL_DIR}/lapguard"

curl -fL --retry 3 -o "${WORKDIR}/${ASSET}" "${BASE}/${ASSET}"
curl -fL --retry 3 -o "${WORKDIR}/SHA256SUMS" "${BASE}/SHA256SUMS"

if command -v sha256sum >/dev/null 2>&1; then
	(cd "$WORKDIR" && sha256sum -c SHA256SUMS --ignore-missing)
else
	echo "sha256sum not found; refusing to install an unverified binary" >&2
	exit 1
fi

mkdir -p "$INSTALL_DIR"
install -m 0755 "${WORKDIR}/${ASSET}" "${INSTALL_DIR}/lapguard"

mkdir -p "$CONFIG_DIR"
if [ -e "${CONFIG_DIR}/config.json" ]; then
	echo "Keeping existing ${CONFIG_DIR}/config.json"
	chmod 0600 "${CONFIG_DIR}/config.json" || true
else
	cat >"${CONFIG_DIR}/config.json" <<'EOF'
{
  "listen": "127.0.0.1:8585",
  "web_dir": "none",
  "auth": {
    "enabled": true,
    "allow_loopback_no_token": true
  },
  "actions": {
    "real_enabled": false
  },
  "safety": {
    "dry_run": true
  },
  "docker": {
    "stop_enabled": false
  },
  "auto_drain": {
    "enabled": false
  }
}
EOF
	chmod 0600 "${CONFIG_DIR}/config.json"
	echo "Wrote safe-default ${CONFIG_DIR}/config.json (mode 0600)"
fi

mkdir -p "$SYSTEMD_DIR"
cat >"${SYSTEMD_DIR}/lapguard.service" <<'EOF'
[Unit]
Description=LapGuard laptop power manager (user, dry-run safety)
Documentation=https://github.com/ChrisKariofyllis/LapGuard
After=network-online.target

[Service]
Type=simple
ExecStart=%h/.local/bin/lapguard -config %h/.config/lapguard/config.json -web-dir none
Restart=on-failure
RestartSec=5
NoNewPrivileges=true

[Install]
WantedBy=default.target
EOF

echo
echo "Done. Binary: ${INSTALL_DIR}/lapguard"
case ":${PATH}:" in
*":${INSTALL_DIR}:"*) ;;
*)
	echo "Add ${INSTALL_DIR} to PATH if 'lapguard' is not found."
	;;
esac
echo "A Bearer token is minted on first start if none exists — save it from the journal."
echo "Safe defaults: auth on, loopback may omit token, real actions off, auto-drain off."
echo
echo "  systemctl --user daemon-reload"
echo "  systemctl --user enable --now lapguard.service"
echo "  journalctl --user -u lapguard.service -e"
echo "  Dashboard: http://127.0.0.1:8585"
echo
echo "Do not bind 0.0.0.0. Do not enable Funnel. See docs/install.md and docs/security.md."
