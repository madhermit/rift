#!/usr/bin/env bash
set -euo pipefail

REPO="madhermit/rift"
INSTALL_DIR="${HOME}/.local/bin"

# Detect platform
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

BINARY="rift-${OS}-${ARCH}"

# Get latest release
LATEST=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep '"tag_name"' | cut -d'"' -f4)
if [[ -z "$LATEST" ]]; then
  echo "Could not determine latest version"
  exit 1
fi

BASE="https://github.com/${REPO}/releases/download/${LATEST}"

echo "Installing rift ${LATEST} (${OS}/${ARCH})..."

# Download the binary to a temp file so a failed checksum never leaves a
# half-installed or unverified binary on the PATH.
TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT
curl -fsSL "${BASE}/${BINARY}" -o "$TMP"

# Verify the checksum against the release's SHA256SUMS. Older releases may not
# have one, so a missing file degrades to a warning; a mismatch aborts.
if command -v sha256sum >/dev/null 2>&1; then
  SHA_CMD="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
  SHA_CMD="shasum -a 256"
else
  SHA_CMD=""
fi

SUMS=$(curl -fsSL "${BASE}/SHA256SUMS" 2>/dev/null || true)
if [[ -z "$SUMS" ]]; then
  echo "Warning: no SHA256SUMS published for ${LATEST}; skipping checksum verification"
elif [[ -z "$SHA_CMD" ]]; then
  echo "Warning: no sha256sum/shasum tool found; skipping checksum verification"
else
  EXPECTED=$(printf '%s\n' "$SUMS" | awk -v f="$BINARY" '$2 == f || $2 == "*"f {print $1; exit}')
  # Compare hash strings directly rather than using `-c` check mode, whose
  # stdin/format behavior differs between GNU sha256sum and macOS's perl shasum
  # (a matching hash could still exit non-zero on some macOS versions).
  GOT=$($SHA_CMD "$TMP" | awk '{print $1}')
  if [[ -z "$EXPECTED" ]]; then
    echo "Warning: no checksum for ${BINARY} in SHA256SUMS; skipping verification"
  elif [[ "$GOT" != "$EXPECTED" ]]; then
    echo "Checksum verification FAILED for ${BINARY}" >&2
    echo "  expected: ${EXPECTED}" >&2
    echo "  got:      ${GOT}" >&2
    exit 1
  else
    echo "Checksum verified."
  fi
fi

mkdir -p "$INSTALL_DIR"
mv "$TMP" "${INSTALL_DIR}/rift"
trap - EXIT
chmod +x "${INSTALL_DIR}/rift"

echo "Installed to ${INSTALL_DIR}/rift"

# Check if in PATH
if ! echo "$PATH" | grep -q "$INSTALL_DIR"; then
  echo ""
  echo "Add to your shell profile:"
  echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
fi

echo "Shell completion: run 'rift completion --help' for bash/zsh/fish setup."
