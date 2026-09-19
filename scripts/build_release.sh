#!/usr/bin/env bash
# ==============================================================================
# Nord Launcher - Linux Production Release Builder
# ==============================================================================
set -euo pipefail

VERSION="${1:-0.1.0}"
CLEAN_VERSION="${VERSION#v}"
CF_KEY="${2:-${CURSEFORGE_KEY:-}}"

if [ -n "$CF_KEY" ]; then
    if [ "${#CF_KEY}" -lt 16 ]; then
        echo "ERROR: Invalid CurseForge key length (expected at least 16 chars)!"
        exit 1
    fi
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
DIST_LINUX="$ROOT_DIR/dist/linux"

echo "==> [1/5] Preparing output directory: $DIST_LINUX"
rm -rf "$DIST_LINUX"
mkdir -p "$DIST_LINUX"

echo "==> [2/5] Building Frontend (SolidJS)..."
cd "$ROOT_DIR/frontend"
pnpm install
pnpm build

echo "==> [3/5] Compiling Launcher Executable with Go..."
cd "$ROOT_DIR"
BIN_PATH="$DIST_LINUX/nord-launcher"
go build -ldflags="-s -w -X main.version=$CLEAN_VERSION -X main.CurseForgeKey=$CF_KEY" -o "$BIN_PATH" ./cmd/launcher/main.go

if [ -n "$CF_KEY" ]; then
    if grep -Fq "$CF_KEY" "$BIN_PATH"; then
        echo "PASS: Built-in CurseForge API key verified in Linux executable binary."
    else
        echo "ERROR: Built-in CurseForge API key was not baked into Linux executable!"
        exit 1
    fi
fi

BIN_SIZE=$(stat -c%s "$BIN_PATH" 2>/dev/null || stat -f%z "$BIN_PATH")
BIN_MB=$(awk "BEGIN {printf \"%.2f\", $BIN_SIZE/1048576}")
echo "Binary compiled: $BIN_PATH (${BIN_MB} MB)"

MAX_BYTES=$((40 * 1024 * 1024))
if [ "$BIN_SIZE" -gt "$MAX_BYTES" ]; then
    echo "ERROR: Launcher binary exceeds 40 MB budget (${BIN_MB} MB)!"
    exit 1
fi

echo "==> [4/5] Packaging tar.gz distribution..."
cp "$ROOT_DIR/build/linux/nord-launcher.desktop" "$DIST_LINUX/"
TAR_PATH="$DIST_LINUX/nord-launcher-v${CLEAN_VERSION}-linux-amd64.tar.gz"
tar -czf "$TAR_PATH" -C "$DIST_LINUX" nord-launcher nord-launcher.desktop

echo "==> [5/5] Generating SHA256 Checksums..."
cd "$DIST_LINUX"
sha256sum nord-launcher "nord-launcher-v${CLEAN_VERSION}-linux-amd64.tar.gz" > SHA256SUMS.txt

echo "=================================================================="
echo " Production Release Built Successfully for Linux x64"
echo " Tarball: $TAR_PATH"
echo " SHA256: $(cat SHA256SUMS.txt)"
echo "=================================================================="
