#!/bin/sh
set -eu

TARGET_USER=${SUDO_USER:-}
SOCKET_PATH=/run/soju/admin
ASSUME_YES=0
DRY_RUN=0
UNIT_NAME=soju-tui-admin-access
SYSTEMD_DIR=/etc/systemd/system

usage() {
	cat <<'EOF'
Usage: sudo scripts/grant-admin-access.sh --user USER [--socket PATH] [--dry-run] [--yes]

Grants one trusted local user access to soju's mode-0600 admin socket with a
per-user ACL. A systemd path unit reapplies the ACL whenever the socket is
created, so access survives soju restarts without making the socket public.
EOF
}

fail() {
	printf 'grant-admin-access: %s\n' "$1" >&2
	exit 1
}

while [ "$#" -gt 0 ]; do
	case "$1" in
	--user)
		[ "$#" -ge 2 ] || fail "--user requires a value"
		TARGET_USER=$2
		shift 2
		;;
	--socket)
		[ "$#" -ge 2 ] || fail "--socket requires a value"
		SOCKET_PATH=$2
		shift 2
		;;
	--yes)
		ASSUME_YES=1
		shift
		;;
	--dry-run)
		DRY_RUN=1
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		fail "unknown argument: $1"
		;;
	esac
done

[ "$(id -u)" -eq 0 ] || fail "run this helper with sudo"
[ -n "$TARGET_USER" ] || fail "specify the local login with --user USER"
case "$TARGET_USER" in
*[!A-Za-z0-9._-]*) fail "the username contains unsupported characters" ;;
esac
id "$TARGET_USER" >/dev/null 2>&1 || fail "user $TARGET_USER does not exist"

case "$SOCKET_PATH" in
/*) ;;
*) fail "the socket path must be absolute" ;;
esac
case "$SOCKET_PATH" in
*[!A-Za-z0-9._/-]*) fail "the socket path contains unsupported characters" ;;
esac
[ ! -L "$SOCKET_PATH" ] || fail "refusing a symbolic-link socket path"
[ -S "$SOCKET_PATH" ] || fail "$SOCKET_PATH is not a Unix socket"

command -v setfacl >/dev/null 2>&1 || fail "setfacl is required; on Debian run: sudo apt install acl"
command -v systemctl >/dev/null 2>&1 || fail "systemd is required for persistent socket access"
command -v runuser >/dev/null 2>&1 || fail "runuser is required"
command -v mktemp >/dev/null 2>&1 || fail "mktemp is required"
command -v install >/dev/null 2>&1 || fail "install is required"
command -v cmp >/dev/null 2>&1 || fail "cmp is required"

SETFACL_PATH=$(command -v setfacl)
case "$SETFACL_PATH" in
/*) ;;
*) fail "setfacl did not resolve to an absolute path" ;;
esac
if [ -x /usr/bin/test ]; then
	TEST_PATH=/usr/bin/test
elif [ -x /bin/test ]; then
	TEST_PATH=/bin/test
else
	fail "an external test executable is required"
fi
SOCKET_DIR=${SOCKET_PATH%/*}
[ -d "$SOCKET_DIR" ] || fail "$SOCKET_DIR is not a directory"

SERVICE_PATH=$SYSTEMD_DIR/$UNIT_NAME.service
PATH_PATH=$SYSTEMD_DIR/$UNIT_NAME.path
TEMP_DIR=$(mktemp -d /tmp/soju-tui-admin-access.XXXXXX)
[ -n "$TEMP_DIR" ] && [ -d "$TEMP_DIR" ] || fail "could not create a temporary directory"
cleanup() {
	rm -f "$TEMP_DIR/$UNIT_NAME.service" "$TEMP_DIR/$UNIT_NAME.path"
	rmdir "$TEMP_DIR" 2>/dev/null || true
}
trap cleanup EXIT HUP INT TERM

SERVICE_TEMP=$TEMP_DIR/$UNIT_NAME.service
PATH_TEMP=$TEMP_DIR/$UNIT_NAME.path
{
	printf '%s\n' '[Unit]'
	printf '%s\n' 'Description=Grant soju-tui administrator access to the soju admin socket'
	printf '\n'
	printf '%s\n' '[Service]'
	printf '%s\n' 'Type=oneshot'
	printf 'ExecCondition=%s -S %s\n' "$TEST_PATH" "$SOCKET_PATH"
	printf 'ExecStart=%s -m u:%s:rx %s\n' "$SETFACL_PATH" "$TARGET_USER" "$SOCKET_DIR"
	printf 'ExecStart=%s -m u:%s:rw %s\n' "$SETFACL_PATH" "$TARGET_USER" "$SOCKET_PATH"
	printf '%s\n' 'NoNewPrivileges=true'
	printf '%s\n' 'PrivateTmp=true'
} >"$SERVICE_TEMP"
{
	printf '%s\n' '[Unit]'
	printf '%s\n' 'Description=Watch for the soju admin socket'
	printf '\n'
	printf '%s\n' '[Path]'
	printf 'PathChanged=%s\n' "$SOCKET_PATH"
	printf 'Unit=%s.service\n' "$UNIT_NAME"
	printf '\n'
	printf '%s\n' '[Install]'
	printf '%s\n' 'WantedBy=multi-user.target'
} >"$PATH_TEMP"

for PAIR in "$SERVICE_TEMP:$SERVICE_PATH" "$PATH_TEMP:$PATH_PATH"; do
	SOURCE_PATH=${PAIR%%:*}
	DESTINATION_PATH=${PAIR#*:}
	if [ -e "$DESTINATION_PATH" ] && ! cmp -s "$SOURCE_PATH" "$DESTINATION_PATH"; then
		fail "$DESTINATION_PATH already exists with different content; review it manually"
	fi
done

printf 'This grants %s full soju administration through %s.\n' "$TARGET_USER" "$SOCKET_PATH"
printf 'Persistent systemd units:\n  %s\n  %s\n' "$SERVICE_PATH" "$PATH_PATH"
printf '\nProposed service unit:\n'
cat "$SERVICE_TEMP"
printf '\nProposed path unit:\n'
cat "$PATH_TEMP"
if [ "$DRY_RUN" -eq 1 ]; then
	printf '\nDry run complete; no system files were changed.\n'
	exit 0
fi
if [ "$ASSUME_YES" -ne 1 ]; then
	printf 'Continue? [y/N] '
	IFS= read -r ANSWER
	case "$ANSWER" in
	y | Y | yes | YES) ;;
	*) fail "cancelled" ;;
	esac
fi

install -m 0644 "$SERVICE_TEMP" "$SERVICE_PATH"
install -m 0644 "$PATH_TEMP" "$PATH_PATH"
systemctl daemon-reload
systemctl enable --now "$UNIT_NAME.path"
systemctl start "$UNIT_NAME.service"

if runuser -u "$TARGET_USER" -- test -w "$SOCKET_PATH"; then
	printf '%s can now access %s without sudo.\n' "$TARGET_USER" "$SOCKET_PATH"
else
	fail "$TARGET_USER still cannot write $SOCKET_PATH; inspect permissions with: namei -l $SOCKET_PATH"
fi
