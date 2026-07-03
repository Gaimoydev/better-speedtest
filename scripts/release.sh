#!/usr/bin/env bash
# One-shot cross-compile of better-speedtest for all release platforms into
# dist/release/, with a SHA256SUMS.txt. Pure Go (CGO off) so every target
# builds from any host — no cross toolchains needed.
#
#   scripts/release.sh            # all platforms -> raw binaries + checksums
#   scripts/release.sh --archive  # also pack each (.tar.gz, or .zip on Windows if zip present)
#   scripts/release.sh --upx      # also UPX-compress each (if upx installed)
set -euo pipefail
cd "$(dirname "$0")/.."

OUTDIR="dist/release"
PKG="./cmd/better-speedtest"
VERSION="$(grep -oE 'Version = "[^"]+"' cmd/better-speedtest/main.go 2>/dev/null | grep -oE '[0-9][^"]*' | head -1 || true)"
VERSION="${VERSION:-dev}"

UPX=0; ARCHIVE=0
for a in "$@"; do case "$a" in
  --upx) UPX=1 ;;
  --archive) ARCHIVE=1 ;;
  *) echo "unknown flag: $a (use --archive and/or --upx)" >&2; exit 2 ;;
esac; done

# name          GOOS     GOARCH   extra-env (or -)
TARGETS=(
  "linux-amd64    linux    amd64    -"
  "linux-arm64    linux    arm64    -"
  "linux-armv7    linux    arm      GOARM=7"
  "linux-386      linux    386      -"
  "linux-mips     linux    mips     GOMIPS=softfloat"
  "linux-mipsle   linux    mipsle   GOMIPS=softfloat"
  "windows-amd64  windows  amd64    -"
  "windows-arm64  windows  arm64    -"
  "darwin-amd64   darwin   amd64    -"
  "darwin-arm64   darwin   arm64    -"
  "android-arm64  android  arm64    -"
)

rm -rf "$OUTDIR"; mkdir -p "$OUTDIR"
echo "better-speedtest v$VERSION  ->  $OUTDIR/"
echo

assets=(); ok=0; fail=0
for t in "${TARGETS[@]}"; do
  read -r NAME GOOS GOARCH EXTRA <<<"$t"
  bin="better-speedtest-$NAME"; [ "$GOOS" = windows ] && bin="$bin.exe"
  extraenv=""; [ "$EXTRA" != "-" ] && extraenv="$EXTRA"
  if env CGO_ENABLED=0 GOOS="$GOOS" GOARCH="$GOARCH" $extraenv \
       go build -trimpath -ldflags="-s -w" -o "$OUTDIR/$bin" "$PKG"; then
    # UPX only for linux/windows: darwin binaries are rejected by codesign,
    # and android/on-device binaries can misbehave or get flagged when packed.
    case "$GOOS" in linux|windows)
      { [ "$UPX" = 1 ] && command -v upx >/dev/null 2>&1 && upx -9q "$OUTDIR/$bin" >/dev/null 2>&1; } || true ;;
    esac
    printf "  OK  %-22s %6s\n" "$bin" "$(du -h "$OUTDIR/$bin" | cut -f1)"
    asset="$bin"
    assets+=("$asset")
    if [ "$ARCHIVE" = 1 ]; then
      if [ "$GOOS" = windows ] && command -v zip >/dev/null 2>&1; then
        asset="better-speedtest-$NAME.zip"
        ( cd "$OUTDIR" && zip -qj "$asset" "$bin" )
      else
        asset="better-speedtest-$NAME.tar.gz"
        ( cd "$OUTDIR" && tar czf "$asset" "$bin" )
      fi
      assets+=("$asset")
    fi
    ok=$((ok+1))
  else
    printf "  --  %-22s BUILD FAILED\n" "$bin"; fail=$((fail+1))
  fi
done

( cd "$OUTDIR" && sha256sum "${assets[@]}" > SHA256SUMS.txt )
echo
echo "SHA256SUMS.txt:"; sed 's/^/  /' "$OUTDIR/SHA256SUMS.txt"
echo
echo "done: $ok built, $fail failed  ->  upload $OUTDIR/* as GitHub release assets"
