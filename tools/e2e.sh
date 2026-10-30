#!/bin/sh
# End-to-end run of the real window under a virtual X server with injected mouse and keyboard
# (python-xlib XTEST + ImageMagick `import`): title -> Singleplayer -> New World -> walk -> inventory -> pause.
# Software GL is slow, so the driver waits 2.5 s after every action. Screenshots go to $1.
set -e
out=${1:-/tmp/gocraft-e2e}
mkdir -p "$out"
Xvfb :99 -screen 0 1280x720x24 >/dev/null 2>&1 &
xvfb=$!
cd "$(dirname "$0")/.."
go build -o gocraft ./cmd/gocraft
XDG_DATA_HOME="$out/xdg" XDG_CONFIG_HOME="$out/cfg" DISPLAY=:99 ./gocraft -options /dev/null -view 4 >"$out/game.log" 2>&1 &
game=$!
trap 'kill $game $xvfb 2>/dev/null' EXIT
sleep 3
DISPLAY=:99 python3 tools/e2e_drive.py "$out"
grep -q 'opened "New World"' "$out/game.log" && echo "OK: the menu opened New World"
ls "$out"/xdg/gocraft/overlay/"New World"/player.json "$out"/xdg/gocraft/overlay/"New World"/c.*.bin >/dev/null && echo "OK: Save and Quit wrote the edited column and the player"
grep -Eq '^fov:0\.[89]' "$out/cfg/gocraft/options.txt" && echo "OK: a slider dragged in Options was saved ($(grep '^fov' "$out/cfg/gocraft/options.txt"))"
