#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
SCRIPT_PATH=$ROOT_DIR/scripts/setup.sh
GRANT_SCRIPT=$ROOT_DIR/scripts/grant-admin-access.sh
TARGET_USER=${SUDO_USER:-}
CONFIG_PATH=/etc/soju/config
SOCKET_PATH=
SOJUCTL_PATH=
DRY_RUN=0
ASSUME_YES=0

usage() {
	cat <<'EOF'
Usage: scripts/setup.sh [options]

First-time setup for a local soju-tui administrator. The wizard discovers the
admin socket from the soju config, installs the ACL utility when approved,
previews persistent socket access, applies it, and verifies sojuctl.

Options:
  --user USER       Local Linux account to authorize (default: sudo caller)
  --config PATH     Soju config path (default: /etc/soju/config)
  --socket PATH     Override the admin socket discovered from the config
  --sojuctl PATH    Override the sojuctl executable
  --dry-run         Show detected settings and proposed changes only
  --yes             Accept package and ACL confirmation prompts
  -h, --help        Show this help

Run it directly; the script requests sudo when needed:
  ./scripts/setup.sh
EOF
}

fail() {
	printf 'soju-tui setup: %s\n' "$1" >&2
	exit 1
}

if [ "$#" -eq 1 ]; then
	case "$1" in
	-h | --help)
		usage
		exit 0
		;;
	esac
fi

if [ "$(id -u)" -ne 0 ]; then
	command -v sudo >/dev/null 2>&1 || fail "sudo is required for first-time setup"
	exec sudo -- "$SCRIPT_PATH" "$@"
fi

while [ "$#" -gt 0 ]; do
	case "$1" in
	--user)
		[ "$#" -ge 2 ] || fail "--user requires a value"
		TARGET_USER=$2
		shift 2
		;;
	--config)
		[ "$#" -ge 2 ] || fail "--config requires a value"
		CONFIG_PATH=$2
		shift 2
		;;
	--socket)
		[ "$#" -ge 2 ] || fail "--socket requires a value"
		SOCKET_PATH=$2
		shift 2
		;;
	--sojuctl)
		[ "$#" -ge 2 ] || fail "--sojuctl requires a value"
		SOJUCTL_PATH=$2
		shift 2
		;;
	--dry-run)
		DRY_RUN=1
		shift
		;;
	--yes)
		ASSUME_YES=1
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

[ -n "$TARGET_USER" ] || fail "cannot determine the invoking login; use --user USER"
case "$TARGET_USER" in
*[!A-Za-z0-9._-]*) fail "the username contains unsupported characters" ;;
esac
id "$TARGET_USER" >/dev/null 2>&1 || fail "local user $TARGET_USER does not exist"

case "$CONFIG_PATH" in
/*) ;;
*) fail "the config path must be absolute" ;;
esac
[ -f "$CONFIG_PATH" ] || fail "$CONFIG_PATH is not a regular file"
[ -r "$CONFIG_PATH" ] || fail "$CONFIG_PATH is not readable"

if [ -z "$SOCKET_PATH" ]; then
	LISTEN_URI=$(awk '$1 == "listen" && index($2, "unix+admin://") == 1 { print $2; exit }' "$CONFIG_PATH")
	[ -n "$LISTEN_URI" ] || fail "$CONFIG_PATH has no listen unix+admin:// directive"
	if [ "$LISTEN_URI" = "unix+admin://" ]; then
		SOCKET_PATH=/run/soju/admin
	else
		SOCKET_PATH=${LISTEN_URI#unix+admin://}
	fi
fi
case "$SOCKET_PATH" in
/*) ;;
*) fail "the discovered admin socket path is not absolute: $SOCKET_PATH" ;;
esac
case "$SOCKET_PATH" in
*[!A-Za-z0-9._/-]*) fail "the admin socket path contains unsupported characters" ;;
esac
[ ! -L "$SOCKET_PATH" ] || fail "refusing a symbolic-link socket path"
[ -S "$SOCKET_PATH" ] || fail "$SOCKET_PATH is not a Unix socket; make sure soju is running"

if [ -z "$SOJUCTL_PATH" ]; then
	SOJUCTL_PATH=$(command -v sojuctl || true)
fi
[ -n "$SOJUCTL_PATH" ] || fail "sojuctl was not found; install soju or use --sojuctl PATH"
case "$SOJUCTL_PATH" in
/*) ;;
*) fail "sojuctl did not resolve to an absolute path" ;;
esac
[ -x "$SOJUCTL_PATH" ] || fail "$SOJUCTL_PATH is not executable"
[ -x "$GRANT_SCRIPT" ] || fail "$GRANT_SCRIPT is missing or not executable"

case "$(uname -m)" in
x86_64 | amd64) TUI_BINARY=$ROOT_DIR/dist/soju-tui-linux-amd64 ;;
aarch64 | arm64) TUI_BINARY=$ROOT_DIR/dist/soju-tui-linux-arm64 ;;
*) TUI_BINARY=$ROOT_DIR/dist/soju-tui ;;
esac

printf 'soju-tui first-time setup\n\n'
printf '  Local administrator: %s\n' "$TARGET_USER"
printf '  Soju config:         %s\n' "$CONFIG_PATH"
printf '  Admin socket:        %s\n' "$SOCKET_PATH"
printf '  sojuctl:             %s\n' "$SOJUCTL_PATH"
printf '  TUI binary:          %s\n\n' "$TUI_BINARY"

install_acl_package() {
	if command -v apt-get >/dev/null 2>&1; then
		PACKAGE_COMMAND='apt-get install -y acl'
	elif command -v dnf >/dev/null 2>&1; then
		PACKAGE_COMMAND='dnf install -y acl'
	elif command -v yum >/dev/null 2>&1; then
		PACKAGE_COMMAND='yum install -y acl'
	elif command -v zypper >/dev/null 2>&1; then
		PACKAGE_COMMAND='zypper --non-interactive install acl'
	elif command -v pacman >/dev/null 2>&1; then
		PACKAGE_COMMAND='pacman --noconfirm -S acl'
	else
		fail "setfacl is missing and no supported package manager was found; install the ACL tools"
	fi
	printf 'The ACL utility is missing. Proposed package command: %s\n' "$PACKAGE_COMMAND"
	if [ "$DRY_RUN" -eq 1 ]; then
		return
	fi
	if [ "$ASSUME_YES" -ne 1 ]; then
		printf 'Install the acl package? [y/N] '
		IFS= read -r ANSWER
		case "$ANSWER" in
		y | Y | yes | YES) ;;
		*) fail "cancelled before package installation" ;;
		esac
	fi
	case "$PACKAGE_COMMAND" in
	'apt-get install -y acl') apt-get install -y acl ;;
	'dnf install -y acl') dnf install -y acl ;;
	'yum install -y acl') yum install -y acl ;;
	'zypper --non-interactive install acl') zypper --non-interactive install acl ;;
	'pacman --noconfirm -S acl') pacman --noconfirm -S acl ;;
	esac
	command -v setfacl >/dev/null 2>&1 || fail "acl package installation completed but setfacl is still unavailable"
}

if ! command -v setfacl >/dev/null 2>&1; then
	install_acl_package
fi

if [ "$DRY_RUN" -eq 1 ]; then
	if command -v setfacl >/dev/null 2>&1; then
		"$GRANT_SCRIPT" --user "$TARGET_USER" --socket "$SOCKET_PATH" --dry-run
	else
		printf 'ACL unit preview is available after the acl package is installed.\n'
	fi
	exit 0
fi

if [ "$ASSUME_YES" -eq 1 ]; then
	"$GRANT_SCRIPT" --user "$TARGET_USER" --socket "$SOCKET_PATH" --yes
else
	"$GRANT_SCRIPT" --user "$TARGET_USER" --socket "$SOCKET_PATH"
fi

printf '\nVerifying sojuctl as local user %s...\n' "$TARGET_USER"
runuser -u "$TARGET_USER" -- "$SOJUCTL_PATH" -config "$CONFIG_PATH" server status

if [ -x "$TUI_BINARY" ]; then
	printf '\nSetup complete. Run as %s:\n  %s\n' "$TARGET_USER" "$TUI_BINARY"
else
	printf '\nSocket access is configured, but %s is missing. Build or download the correct binary first.\n' "$TUI_BINARY"
fi
