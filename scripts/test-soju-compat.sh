#!/bin/sh
set -eu

SOJU_SOURCE=${SOJU_SOURCE:-}
TAGS="v0.9.0 v0.10.1"
WORK_DIR=
SOJU_PID=

usage() {
	cat <<'EOF'
Usage: scripts/test-soju-compat.sh [--source PATH] [--tags "v0.9.0 v0.10.1"]

Builds each requested upstream Soju tag, starts an isolated temporary instance,
and verifies the sojuctl command grammar and output contracts used by Soju-TUI.
No host Soju config, database, socket, certificate, or service is touched.
EOF
}

while [ "$#" -gt 0 ]; do
	case "$1" in
	--source) [ "$#" -ge 2 ] || { echo "--source requires a value" >&2; exit 2; }; SOJU_SOURCE=$2; shift 2 ;;
	--tags) [ "$#" -ge 2 ] || { echo "--tags requires a value" >&2; exit 2; }; TAGS=$2; shift 2 ;;
	-h | --help) usage; exit 0 ;;
	*) echo "unknown argument: $1" >&2; usage >&2; exit 2 ;;
	esac
done

cleanup_instance() {
	if [ -n "$SOJU_PID" ]; then
		kill "$SOJU_PID" 2>/dev/null || true
		wait "$SOJU_PID" 2>/dev/null || true
		SOJU_PID=
	fi
}

cleanup() {
	cleanup_instance
	case "$WORK_DIR" in
	"${TMPDIR:-/tmp}"/soju-tui-compat.*) rm -rf -- "$WORK_DIR" ;;
	esac
}
trap cleanup EXIT HUP INT TERM

command -v git >/dev/null 2>&1 || { echo "git is required" >&2; exit 1; }
command -v go >/dev/null 2>&1 || { echo "Go is required" >&2; exit 1; }
WORK_DIR=$(mktemp -d "${TMPDIR:-/tmp}/soju-tui-compat.XXXXXX")

if [ -z "$SOJU_SOURCE" ]; then
	SOJU_SOURCE=$WORK_DIR/soju-source
	git clone --quiet --filter=blob:none https://codeberg.org/emersion/soju.git "$SOJU_SOURCE"
else
	case "$SOJU_SOURCE" in
	/*) ;;
	*) SOJU_SOURCE=$(CDPATH='' cd -- "$SOJU_SOURCE" && pwd) ;;
	esac
	[ -d "$SOJU_SOURCE/.git" ] || { echo "$SOJU_SOURCE is not a Soju Git checkout" >&2; exit 1; }
fi

assert_contains() {
	case "$1" in
	*"$2"*) ;;
	*) printf 'expected output to contain %s, got:\n%s\n' "$2" "$1" >&2; exit 1 ;;
	esac
}

for tag in $TAGS; do
	case "$tag" in
	v0.9.0) expected_commit=8df1af5bd57b81f2f80d2bae44bf98ffe167ed06 ;;
	v0.10.1) expected_commit=ca81b9905e24aadd6a346a5ae72ae7e9d14db568 ;;
	*) echo "unsupported compatibility target: $tag" >&2; exit 2 ;;
	esac
	printf '[compat] testing Soju %s\n' "$tag"
	release_dir=$WORK_DIR/${tag#v}
	mkdir -p "$release_dir"
	git -C "$SOJU_SOURCE" checkout --quiet --detach "$tag"
	actual_commit=$(git -C "$SOJU_SOURCE" rev-parse HEAD)
	[ "$actual_commit" = "$expected_commit" ] || {
		printf 'Soju %s resolved to unexpected commit %s (expected %s)\n' "$tag" "$actual_commit" "$expected_commit" >&2
		exit 1
	}
	(
		cd "$SOJU_SOURCE"
		GOTOOLCHAIN=auto go build -trimpath -o "$release_dir/soju" ./cmd/soju
		GOTOOLCHAIN=auto go build -trimpath -o "$release_dir/sojuctl" ./cmd/sojuctl
	)
	socket=$release_dir/admin.sock
	config=$release_dir/soju.conf
	cat >"$config" <<EOF
db sqlite3 $release_dir/main.db
message-store memory
listen unix+admin://$socket
hostname compatibility.example.test
auth internal
EOF
	"$release_dir/soju" -config "$config" >"$release_dir/soju.log" 2>&1 &
	SOJU_PID=$!
	ready=0
	attempt=0
	while [ "$attempt" -lt 100 ]; do
		if [ -S "$socket" ]; then ready=1; break; fi
		if ! kill -0 "$SOJU_PID" 2>/dev/null; then break; fi
		sleep 0.1
		attempt=$((attempt + 1))
	done
	if [ "$ready" -ne 1 ]; then
		cat "$release_dir/soju.log" >&2
		echo "Soju $tag did not create its admin socket" >&2
		exit 1
	fi
	output=$("$release_dir/sojuctl" -config "$config" help)
	assert_contains "$output" "user create"
	output=$("$release_dir/sojuctl" -config "$config" user create -username compat-admin -disable-password -admin=true)
	assert_contains "$output" "created user"
	output=$("$release_dir/sojuctl" -config "$config" user status)
	assert_contains "$output" "compat-admin (admin):"
	output=$("$release_dir/sojuctl" -config "$config" user run compat-admin network create -addr ircs://127.0.0.1:9 -name compat -enabled false)
	assert_contains "$output" "created network"
	output=$("$release_dir/sojuctl" -config "$config" user run compat-admin network status)
	assert_contains "$output" "compat (ircs://127.0.0.1:9) [disabled]"
	output=$("$release_dir/sojuctl" -config "$config" user run compat-admin channel create '#compat/compat' -detached true)
	assert_contains "$output" "created channel"
	output=$("$release_dir/sojuctl" -config "$config" user run compat-admin channel status -network compat)
	assert_contains "$output" "#compat"
	output=$("$release_dir/sojuctl" -config "$config" user run compat-admin certfp generate -network compat -key-type ed25519)
	assert_contains "$output" "SHA-256 fingerprint:"
	output=$("$release_dir/sojuctl" -config "$config" user run compat-admin certfp fingerprint -network compat)
	assert_contains "$output" "SHA-512 fingerprint:"
	output=$("$release_dir/sojuctl" -config "$config" user run compat-admin sasl status -network compat)
	assert_contains "$output" "SASL EXTERNAL"
	output=$("$release_dir/sojuctl" -config "$config" user run compat-admin channel update '#compat/compat' -detached false)
	assert_contains "$output" "updated channel"
	output=$("$release_dir/sojuctl" -config "$config" user run compat-admin channel delete '#compat/compat')
	assert_contains "$output" "deleted channel"
	output=$("$release_dir/sojuctl" -config "$config" user run compat-admin network update compat -name compat-renamed)
	assert_contains "$output" "updated network"
	output=$("$release_dir/sojuctl" -config "$config" user run compat-admin network delete compat-renamed)
	assert_contains "$output" "deleted network"
	output=$("$release_dir/sojuctl" -config "$config" server status)
	assert_contains "$output" "users"
	output=$("$release_dir/sojuctl" -config "$config" user delete compat-admin)
	token=$(printf '%s\n' "$output" | sed -n 's/.*user delete compat-admin \([0-9a-f][0-9a-f]*\)".*/\1/p')
	[ "${#token}" -eq 6 ] || { echo "could not parse Soju deletion token" >&2; exit 1; }
	output=$("$release_dir/sojuctl" -config "$config" user delete compat-admin "$token")
	assert_contains "$output" "deleted user"
	cleanup_instance
	printf '[compat] Soju %s passed\n' "$tag"
done

printf '[compat] all requested Soju releases passed\n'
