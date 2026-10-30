#!/bin/bash
# Benchmark the renderer on your own screen: it flies a fixed route in one of your worlds and prints
# frames per second (average and the slowest 1%), where the game thread's time goes, and the GPU time
# of the world draw (timer queries). Nothing is saved: the world is read only, and the options and
# overlay go to a throw-away folder.
#
#   tools/bench.sh [world folder] [seconds] [render distance] [extra gocraft flags]
#
# Defaults: your "New World" in ~/mc-1.8.9-run/game/saves, 20 seconds, distance 10, 1280x720.
set -e
cd "$(dirname "$0")/.."
WORLD=${1:-$HOME/mc-1.8.9-run/game/saves/New World}
SECS=${2:-20}
VIEW=${3:-10}
shift 3 2>/dev/null || shift $# 2>/dev/null || true
BIN=${GOCRAFT_BIN:-./gocraft}
[ -x "$BIN" ] || go build -o gocraft ./cmd/gocraft
T=$(mktemp -d)
trap 'rm -rf "$T"' EXIT
XDG_DATA_HOME=$T/data XDG_CONFIG_HOME=$T/config "$BIN" -options /dev/null -world "$WORLD" \
	-bench "$SECS" -view "$VIEW" -width 1280 -height 720 "$@" 2>&1 | grep -E "bench:|opened"
