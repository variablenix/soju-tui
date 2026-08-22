#!/bin/sh
set -eu

fail() {
	printf 'release setup integration test: %s\n' "$1" >&2
	exit 1
}

if [ "$(uname -s)" != Linux ]; then
	printf 'release setup integration test skipped: Linux is required\n'
	exit 0
fi

for REQUIRED_COMMAND in curl python3 runuser sudo; do
	command -v "$REQUIRED_COMMAND" >/dev/null 2>&1 || fail "$REQUIRED_COMMAND is required"
done
sudo -n true >/dev/null 2>&1 || fail "passwordless sudo is required"

ROOT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
WORK_DIR=$(mktemp -d /tmp/soju-tui-release-test.XXXXXX) || fail "cannot create temporary directory"
SOCKET_PID=

cleanup() {
	if [ -n "$SOCKET_PID" ]; then
		kill "$SOCKET_PID" >/dev/null 2>&1 || true
		wait "$SOCKET_PID" 2>/dev/null || true
	fi
	case "$WORK_DIR" in
	/tmp/soju-tui-release-test.*) rm -rf -- "$WORK_DIR" ;;
	esac
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

TEST_REPOSITORY=$WORK_DIR/repository
TEST_SCRIPTS=$TEST_REPOSITORY/scripts
TEST_BIN=$WORK_DIR/bin
mkdir -p "$TEST_SCRIPTS" "$TEST_BIN"
cp "$ROOT_DIR/scripts/setup.sh" "$TEST_SCRIPTS/setup.sh"
chmod 0755 "$TEST_SCRIPTS/setup.sh"

cat >"$TEST_SCRIPTS/grant-admin-access.sh" <<'EOF'
#!/bin/sh
set -eu
printf 'stub admin socket access configured\n'
EOF
chmod 0755 "$TEST_SCRIPTS/grant-admin-access.sh"

cat >"$TEST_BIN/sojuctl" <<'EOF'
#!/bin/sh
set -eu
printf 'stub sojuctl status: ready\n'
EOF
cat >"$TEST_BIN/setfacl" <<'EOF'
#!/bin/sh
exit 0
EOF
chmod 0755 "$TEST_BIN/sojuctl" "$TEST_BIN/setfacl"

SOCKET_PATH=$WORK_DIR/admin.sock
python3 - "$SOCKET_PATH" <<'PY' &
import socket
import sys
import time

server = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
server.bind(sys.argv[1])
server.listen(1)
while True:
    time.sleep(60)
PY
SOCKET_PID=$!

attempt=0
while [ ! -S "$SOCKET_PATH" ]; do
	attempt=$((attempt + 1))
	[ "$attempt" -le 50 ] || fail "test admin socket was not created"
	sleep 0.1
done

CONFIG_PATH=$WORK_DIR/soju.conf
printf 'listen unix+admin://%s\n' "$SOCKET_PATH" >"$CONFIG_PATH"
TARGET_USER=$(id -un)
RELEASE_VERSION=${SOJU_TUI_TEST_RELEASE_VERSION:-0.3.2}
SYSTEM_PATH=$TEST_BIN:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

if ! OUTPUT=$(sudo -n env PATH="$SYSTEM_PATH" "$TEST_SCRIPTS/setup.sh" \
	--user "$TARGET_USER" \
	--config "$CONFIG_PATH" \
	--socket "$SOCKET_PATH" \
	--sojuctl "$TEST_BIN/sojuctl" \
	--release "$RELEASE_VERSION" \
	--no-install \
	--yes 2>&1); then
	printf '%s\n' "$OUTPUT" >&2
	fail "release setup failed"
fi
printf '%s\n' "$OUTPUT"

printf '%s\n' "$OUTPUT" | grep -F "Release validation complete; no command was installed." >/dev/null ||
	fail "completion output did not describe validation-only behavior"
RELEASE_BINARY=$(printf '%s\n' "$OUTPUT" | sed -n 's/^  TUI binary:          //p')
[ -n "$RELEASE_BINARY" ] || fail "could not identify downloaded release path"
case "$RELEASE_BINARY" in
/tmp/soju-tui-release.*/soju-tui-linux-*) ;;
*) fail "unexpected release path: $RELEASE_BINARY" ;;
esac
[ ! -e "${RELEASE_BINARY%/*}" ] || fail "temporary release directory was not removed"

printf 'release setup integration test passed for %s as %s\n' "$RELEASE_VERSION" "$TARGET_USER"
