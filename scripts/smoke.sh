#!/usr/bin/env bash
# Boots the BUSY Bar emulator on a scratch port and runs the smoke tests
# against it. Needs node, git and go.
#
#   scripts/smoke.sh                 # clones the emulator into .cache/ on first run
#   BUSYBAR_EMULATOR_DIR=~/src/busybar-emulator scripts/smoke.sh
#   PORT=18099 scripts/smoke.sh
set -euo pipefail
cd "$(dirname "$0")/.."

DIR="${BUSYBAR_EMULATOR_DIR:-.cache/busybar-emulator}"
PORT="${PORT:-18080}"
REVISION="$(cat scripts/emulator-revision)"

if [ ! -f "$DIR/server.js" ]; then
  git clone --no-checkout https://github.com/maxswinkels/busybar-emulator.git "$DIR"
  git -C "$DIR" checkout --detach "$REVISION"
fi
if [ "$(git -C "$DIR" rev-parse HEAD)" != "$REVISION" ] || ! git -C "$DIR" diff --quiet HEAD; then
  echo "emulator must be a clean checkout of $REVISION; set BUSYBAR_EMULATOR_DIR to that checkout" >&2
  exit 1
fi

LOG="$(mktemp -t busybar-emulator.XXXXXX)"
PORT="$PORT" node "$DIR/server.js" >"$LOG" 2>&1 &
PID=$!
trap 'kill "$PID" 2>/dev/null || true' EXIT

for _ in $(seq 1 50); do
  if curl -fs "http://127.0.0.1:$PORT/api/version" >/dev/null 2>&1; then break; fi
  sleep 0.1
done
if ! curl -fs "http://127.0.0.1:$PORT/api/version" >/dev/null; then
  echo "emulator did not start; log:" >&2
  cat "$LOG" >&2
  exit 1
fi
echo "emulator up on :$PORT (log: $LOG)"

BUSYBAR_EMULATOR_URL="http://127.0.0.1:$PORT" go test -count=1 -tags smoke -v ./smoke/ "$@"
