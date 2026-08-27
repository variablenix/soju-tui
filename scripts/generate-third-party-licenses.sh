#!/bin/sh
set -eu

GO_BIN=${GO_BIN:-go}
GO_TOOLCHAIN=${GOTOOLCHAIN:-local}

fail() {
	printf 'third-party license generation: %s\n' "$1" >&2
	exit 1
}

[ "$#" -ge 2 ] || fail "usage: $0 OUTPUT BINARY [BINARY ...]"
OUTPUT=$1
shift

case "$OUTPUT" in
/*) ;;
*) fail "the output path must be absolute" ;;
esac
command -v "$GO_BIN" >/dev/null 2>&1 || fail "$GO_BIN is required"

WORK_DIR=$(mktemp -d /tmp/soju-tui-licenses.XXXXXX) || fail "cannot create a temporary directory"
cleanup() {
	case "$WORK_DIR" in
	/tmp/soju-tui-licenses.*) rm -rf -- "$WORK_DIR" ;;
	esac
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

REFERENCE_MODULES=
INDEX=0
for BINARY in "$@"; do
	INDEX=$((INDEX + 1))
	[ -f "$BINARY" ] || fail "$BINARY is missing"
	[ ! -L "$BINARY" ] || fail "refusing a symbolic-link binary: $BINARY"
	MODULES=$WORK_DIR/modules-$INDEX
	GOTOOLCHAIN="$GO_TOOLCHAIN" "$GO_BIN" version -m "$BINARY" |
		awk '$1 == "dep" { print $2, $3 }' | LC_ALL=C sort -u >"$MODULES"
	[ -s "$MODULES" ] || fail "$BINARY has no embedded Go dependency metadata"
	if [ -z "$REFERENCE_MODULES" ]; then
		REFERENCE_MODULES=$MODULES
	elif ! cmp -s "$REFERENCE_MODULES" "$MODULES"; then
		fail "the packaged binaries do not contain the same dependency versions"
	fi
done

OUTPUT_DIR=${OUTPUT%/*}
[ -d "$OUTPUT_DIR" ] || fail "$OUTPUT_DIR is not a directory"
TEMP_OUTPUT=$(mktemp "$OUTPUT_DIR/.THIRD_PARTY_LICENSES.XXXXXX") || fail "cannot create an output file"
trap 'rm -f -- "$TEMP_OUTPUT"; cleanup' EXIT

{
	printf '%s\n' 'Third-party licenses for soju-tui'
	printf '%s\n' '================================='
	printf '\n'
	printf '%s\n' 'This file is generated from the dependency metadata embedded in the release binaries.'
	MODULE_NUMBER=0
	while read -r MODULE VERSION; do
		[ -n "$MODULE" ] || continue
		MODULE_NUMBER=$((MODULE_NUMBER + 1))
		MODULE_DIR=$(GOTOOLCHAIN="$GO_TOOLCHAIN" "$GO_BIN" list -m -f '{{.Dir}}' "$MODULE@$VERSION") ||
			fail "cannot locate $MODULE@$VERSION in the Go module cache"
		case "$MODULE_DIR" in
		/*) ;;
		*) fail "Go returned a non-absolute module directory for $MODULE@$VERSION" ;;
		esac
		LICENSE_LIST=$WORK_DIR/licenses-$MODULE_NUMBER
		find "$MODULE_DIR" -maxdepth 1 -type f \
			\( -iname 'LICENSE*' -o -iname 'COPYING*' -o -iname 'NOTICE*' \) \
			-print | LC_ALL=C sort >"$LICENSE_LIST"
		[ -s "$LICENSE_LIST" ] || fail "$MODULE@$VERSION has no top-level license or notice file"
		printf '\n%s\n' '=============================================================================='
		printf 'Module: %s %s\n' "$MODULE" "$VERSION"
		while IFS= read -r LICENSE_PATH; do
			printf 'Source file: %s\n\n' "${LICENSE_PATH##*/}"
			cat "$LICENSE_PATH"
			printf '\n'
		done <"$LICENSE_LIST"
	done <"$REFERENCE_MODULES"
} >"$TEMP_OUTPUT"

chmod 0644 "$TEMP_OUTPUT"
mv -f "$TEMP_OUTPUT" "$OUTPUT"
TEMP_OUTPUT=
printf '[soju-tui] generated %s\n' "$OUTPUT"
