#!/bin/sh
# Cross-compile the static arm64 binary for UFI-TOOLS devices (OpenWrt aarch64, CGO off).
# Usage: scripts/build.sh [output-path]
set -e
cd "$(dirname "$0")/.."
OUT="${1:-better-speedtest-linux-arm64}"
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags="-s -w" -o "$OUT" ./cmd/better-speedtest
echo "built: $OUT ($(du -h "$OUT" 2>/dev/null | cut -f1))"
if command -v upx >/dev/null 2>&1; then
	upx -9 "$OUT" >/dev/null 2>&1 && echo "upx: $(du -h "$OUT" | cut -f1)"
else
	echo "upx: not installed (skipped)"
fi
