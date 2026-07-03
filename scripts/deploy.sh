#!/bin/sh
# Build + scp the arm64 binary to the device for on-device testing.
# Usage: scripts/deploy.sh [user@host] [ssh-key-path]
#   scripts/deploy.sh root@192.168.0.1 ~/.ssh/u60_key
set -e
cd "$(dirname "$0")/.."
HOST="${1:-root@192.168.0.1}"
KEY="${2:-}"
OPT=""
[ -n "$KEY" ] && OPT="-i $KEY"

sh scripts/build.sh
ssh $OPT -o StrictHostKeyChecking=no "$HOST" 'mkdir -p /data/plugins/better-speedtest'
scp $OPT -o StrictHostKeyChecking=no better-speedtest-linux-arm64 "$HOST:/data/plugins/better-speedtest/better-speedtest"
ssh $OPT -o StrictHostKeyChecking=no "$HOST" 'chmod +x /data/plugins/better-speedtest/better-speedtest && /data/plugins/better-speedtest/better-speedtest version'
echo "deployed → /data/plugins/better-speedtest/better-speedtest"
