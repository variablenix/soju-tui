#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
MINIMUM=${COVERAGE_MINIMUM:-59.0}
GO_BIN=${GO_BIN:-go}
GO_TOOLCHAIN=${GOTOOLCHAIN:-local}
PROFILE=$(mktemp "${TMPDIR:-/tmp}/soju-tui-coverage.XXXXXX")
trap 'rm -f -- "$PROFILE"' EXIT HUP INT TERM

case "$MINIMUM" in
'' | *[!0-9.]*) echo "COVERAGE_MINIMUM must be numeric" >&2; exit 2 ;;
esac

cd "$ROOT_DIR"
GOTOOLCHAIN="$GO_TOOLCHAIN" "$GO_BIN" test -count=1 -covermode=atomic -coverprofile="$PROFILE" ./...
TOTAL=$(GOTOOLCHAIN="$GO_TOOLCHAIN" "$GO_BIN" tool cover -func="$PROFILE" | awk '/^total:/ { gsub(/%/, "", $3); print $3 }')
[ -n "$TOTAL" ] || { echo "could not determine total coverage" >&2; exit 1; }
awk -v total="$TOTAL" -v minimum="$MINIMUM" 'BEGIN { if (total + 0 < minimum + 0) exit 1 }' || {
	printf 'coverage %s%% is below the required %s%%\n' "$TOTAL" "$MINIMUM" >&2
	exit 1
}
printf 'coverage gate passed: %s%% (minimum %s%%)\n' "$TOTAL" "$MINIMUM"
