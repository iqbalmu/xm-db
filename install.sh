#!/bin/sh
# xm-db installer: installs the latest release binary from GitHub.
#
#   curl -fsSL https://raw.githubusercontent.com/iqbalmu/xm-db/main/install.sh | sh
#
# Environment overrides:
#   XMDB_INSTALL_DIR  target directory (default: /usr/local/bin, falls back to
#                    ~/.local/bin when it is not writable)
#   VERSION          pin a release tag (default: latest, e.g. VERSION=v1.0.0)

set -eu

REPO="iqbalmu/xm-db"

msg() { printf '%s\n' "$*"; }
fail() { printf 'error: %s\n' "$*" >&2; exit 1; }
need() { command -v "$1" >/dev/null 2>&1 || fail "$1 is required"; }

need curl
need tar
need grep
need awk
need uname

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
[ "$OS" = "linux" ] || [ "$OS" = "darwin" ] ||
	fail "unsupported OS: $OS (grab a binary from https://github.com/$REPO/releases)"

ARCH=$(uname -m)
case "$ARCH" in
	x86_64 | amd64) ARCH="amd64" ;;
	aarch64 | arm64) ARCH="arm64" ;;
	*) fail "unsupported architecture: $ARCH" ;;
esac

TAG="${VERSION:-}"
if [ -z "$TAG" ]; then
	msg "==> Finding the latest release…"
	TAG=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
		grep -o '"tag_name":[[:space:]]*"[^"]*"' | head -1 | cut -d '"' -f4)
	[ -n "$TAG" ] || fail "could not determine the latest release"
fi

VER=${TAG#v}
ASSET="xm-db_${VER}_${OS}_${ARCH}.tar.gz"
BASE="https://github.com/$REPO/releases/download/$TAG"

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

msg "==> Downloading xm-db $TAG for $OS/$ARCH…"
curl -fsSL -o "$TMP/$ASSET" "$BASE/$ASSET" ||
	fail "download failed: $BASE/$ASSET"
curl -fsSL -o "$TMP/checksums.txt" "$BASE/checksums.txt" ||
	fail "download failed: checksums.txt"

msg "==> Verifying checksum…"
WANT=$(grep "$ASSET" "$TMP/checksums.txt" | awk '{print $1}')
[ -n "$WANT" ] || fail "no checksum found for $ASSET"
if command -v sha256sum >/dev/null 2>&1; then
	GOT=$(sha256sum "$TMP/$ASSET" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
	GOT=$(shasum -a 256 "$TMP/$ASSET" | awk '{print $1}')
else
	fail "sha256sum or shasum is required for checksum verification"
fi
[ "$GOT" = "$WANT" ] || fail "checksum mismatch (want $WANT, got $GOT)"

tar -xzf "$TMP/$ASSET" -C "$TMP"

DEST="${XMDB_INSTALL_DIR:-/usr/local/bin}"
SUDO=""
if [ -n "${XMDB_INSTALL_DIR:-}" ]; then
	# Explicit override: create it and use it as-is.
	mkdir -p "$DEST"
elif [ ! -w "$DEST" ] && [ "$(id -u)" != "0" ]; then
	if command -v sudo >/dev/null 2>&1 && sudo -v 2>/dev/null; then
		SUDO="sudo"
	else
		DEST="$HOME/.local/bin"
		mkdir -p "$DEST"
	fi
fi

msg "==> Installing to $DEST…"
$SUDO install -m 0755 "$TMP/xmdb" "$DEST/xmdb"

case ":$PATH:" in
	*":$DEST:"*) ;;
	*) msg "NOTE: $DEST is not on your PATH. Add it with:"
	   msg "       export PATH=\"$DEST:\$PATH\"" ;;
esac

msg "==> Installed: $($DEST/xmdb version | head -1)"
msg "Get started: xmdb --help, or run xmdb for the interactive menu"
msg "Upgrade any time by re-running this installer."
