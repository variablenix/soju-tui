#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
SCRIPT_PATH=$ROOT_DIR/scripts/setup.sh
GRANT_SCRIPT=$ROOT_DIR/scripts/grant-admin-access.sh
TARGET_USER=${SUDO_USER:-}
CONFIG_PATH=/etc/soju/config
SOCKET_PATH=
SOJUCTL_PATH=
TUI_BINARY=
INSTALL_PATH=/usr/local/bin/soju-tui
INSTALL_BINARY=1
DRY_RUN=0
ASSUME_YES=0
CHECKSUMS_PATH=
ALLOW_DEVELOPMENT_BUILD=0
DEFAULT_BINARY=0
RELEASE_VERSION=latest
RELEASE_EXPLICIT=0
USE_RELEASE=1
RELEASE_TEMP_DIR=
RELEASE_ASSET=
TEMP_BINARY=

cleanup() {
	if [ -n "$TEMP_BINARY" ]; then
		case "$TEMP_BINARY" in
		/*/.soju-tui.install.*) rm -f -- "$TEMP_BINARY" ;;
		esac
	fi
	if [ -n "$RELEASE_TEMP_DIR" ]; then
		case "$RELEASE_TEMP_DIR" in
		/tmp/soju-tui-release.*) rm -rf -- "$RELEASE_TEMP_DIR" ;;
		esac
	fi
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

usage() {
	cat <<'EOF'
Usage: scripts/setup.sh [options]

Initial and repeatable setup for a local soju-tui administrator. The wizard
discovers the admin socket from the soju config, installs the ACL utility when
approved, previews persistent socket access, applies it, installs the matching
TUI binary, and verifies both sojuctl and the installed command.

Options:
  --user USER       Local Linux account to authorize (default: sudo caller)
  --config PATH     Soju config path (default: /etc/soju/config)
  --socket PATH     Override the admin socket discovered from the config
  --sojuctl PATH    Override the sojuctl executable
  --release VERSION Install a GitHub release (default: latest stable)
  --binary PATH     Use a local architecture-matched binary instead
  --checksums PATH  SHA256SUMS manifest for a local binary
  --install-path P  Command path (default: /usr/local/bin/soju-tui)
  --allow-development-build
                    Permit a dev/dirty/unknown build (not for production)
  --no-install      Configure socket access without installing the command
  --dry-run         Show detected settings and proposed changes only
  --yes             Accept package, replacement, and ACL confirmation prompts
  -h, --help        Show this help

Run it directly; the script requests sudo when needed:
  ./scripts/setup.sh

Pin a release when required:
  ./scripts/setup.sh --release 0.3.2

Use checked-out or locally built artifacts explicitly:
  ./scripts/setup.sh --binary /absolute/path/to/soju-tui-linux-amd64 \
    --checksums /absolute/path/to/SHA256SUMS
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
	command -v sudo >/dev/null 2>&1 || fail "sudo is required for system setup"
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
	--release)
		[ "$#" -ge 2 ] || fail "--release requires a version or latest"
		[ -z "$TUI_BINARY" ] || fail "--release cannot be combined with --binary"
		RELEASE_VERSION=$2
		RELEASE_EXPLICIT=1
		USE_RELEASE=1
		shift 2
		;;
	--binary)
		[ "$#" -ge 2 ] || fail "--binary requires a value"
		[ "$RELEASE_EXPLICIT" -eq 0 ] || fail "--binary cannot be combined with --release"
		TUI_BINARY=$2
		USE_RELEASE=0
		shift 2
		;;
	--checksums)
		[ "$#" -ge 2 ] || fail "--checksums requires a value"
		CHECKSUMS_PATH=$2
		shift 2
		;;
	--install-path)
		[ "$#" -ge 2 ] || fail "--install-path requires a value"
		INSTALL_PATH=$2
		shift 2
		;;
	--no-install)
		INSTALL_BINARY=0
		shift
		;;
	--dry-run)
		DRY_RUN=1
		shift
		;;
	--yes)
		ASSUME_YES=1
		shift
		;;
	--allow-development-build)
		ALLOW_DEVELOPMENT_BUILD=1
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

case "$RELEASE_VERSION" in
latest) ;;
*[!A-Za-z0-9._+-]*) fail "release version contains unsupported characters" ;;
esac
case "$RELEASE_VERSION" in
latest) ;;
*)
	printf '%s\n' "$RELEASE_VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9][A-Za-z0-9.-]*)?$' ||
		fail "release version must look like 0.3.2 or 0.3.2-rc.1"
	;;
esac

if [ "$USE_RELEASE" -eq 1 ] && [ -n "$CHECKSUMS_PATH" ]; then
	fail "--checksums is only valid with --binary"
fi

if [ "$USE_RELEASE" -eq 1 ]; then
	case "$(uname -m)" in
	x86_64 | amd64) RELEASE_ASSET=soju-tui-linux-amd64 ;;
	aarch64 | arm64) RELEASE_ASSET=soju-tui-linux-arm64 ;;
	*) fail "unsupported architecture $(uname -m); use --binary with a native executable" ;;
	esac
	command -v curl >/dev/null 2>&1 || fail "curl is required to download releases; use --binary for a local installation"
	RELEASE_TEMP_DIR=$(mktemp -d /tmp/soju-tui-release.XXXXXX) || fail "cannot create a temporary release directory"
	chmod 0755 "$RELEASE_TEMP_DIR"
	RELEASE_BASE_URL=https://github.com/variablenix/soju-tui/releases
	if [ "$RELEASE_VERSION" = latest ]; then
		RELEASE_BASE_URL=$RELEASE_BASE_URL/latest/download
	else
		RELEASE_BASE_URL=$RELEASE_BASE_URL/download/v$RELEASE_VERSION
	fi
	printf 'Downloading Soju-TUI release (%s, %s) from GitHub...\n' "$RELEASE_VERSION" "$RELEASE_ASSET"
	curl --fail --silent --show-error --location --retry 3 --retry-delay 1 \
		--connect-timeout 15 --max-time 300 --proto '=https' --proto-redir '=https' --tlsv1.2 \
		"$RELEASE_BASE_URL/SHA256SUMS" --output "$RELEASE_TEMP_DIR/SHA256SUMS" ||
		fail "cannot download the release checksum manifest"
	curl --fail --silent --show-error --location --retry 3 --retry-delay 1 \
		--connect-timeout 15 --max-time 300 --proto '=https' --proto-redir '=https' --tlsv1.2 \
		"$RELEASE_BASE_URL/$RELEASE_ASSET" --output "$RELEASE_TEMP_DIR/$RELEASE_ASSET" ||
		fail "cannot download the release binary"
	chmod 755 "$RELEASE_TEMP_DIR/$RELEASE_ASSET"
	TUI_BINARY=$RELEASE_TEMP_DIR/$RELEASE_ASSET
	CHECKSUMS_PATH=$RELEASE_TEMP_DIR/SHA256SUMS
else
	if [ -z "$TUI_BINARY" ]; then
		DEFAULT_BINARY=1
		case "$(uname -m)" in
		x86_64 | amd64) TUI_BINARY=$ROOT_DIR/dist/soju-tui-linux-amd64 ;;
		aarch64 | arm64) TUI_BINARY=$ROOT_DIR/dist/soju-tui-linux-arm64 ;;
		*) TUI_BINARY=$ROOT_DIR/dist/soju-tui ;;
		esac
	fi
fi
case "$TUI_BINARY" in
/*) ;;
*) fail "the TUI binary path must be absolute" ;;
esac
[ -f "$TUI_BINARY" ] || fail "$TUI_BINARY is missing; build or download the matching binary first"
[ ! -L "$TUI_BINARY" ] || fail "refusing a symbolic-link source binary"
[ -x "$TUI_BINARY" ] || fail "$TUI_BINARY is not executable"
command -v runuser >/dev/null 2>&1 || fail "runuser is required"
if command -v sha256sum >/dev/null 2>&1; then
	SHA256_COMMAND=sha256sum
elif command -v shasum >/dev/null 2>&1; then
	SHA256_COMMAND=shasum
else
	fail "sha256sum or shasum is required"
fi
binary_sha256() {
	FINGERPRINT_PATH=$1
	if [ "$SHA256_COMMAND" = sha256sum ]; then
		FINGERPRINT=$(sha256sum "$FINGERPRINT_PATH" | awk '{ print $1 }') || fail "cannot hash $FINGERPRINT_PATH"
	else
		FINGERPRINT=$(shasum -a 256 "$FINGERPRINT_PATH" | awk '{ print $1 }') || fail "cannot hash $FINGERPRINT_PATH"
	fi
	case "$FINGERPRINT" in
	????????????????????????????????????????????????????????????????) ;;
	*) fail "unexpected SHA-256 output for $FINGERPRINT_PATH" ;;
	esac
	case "$FINGERPRINT" in
	*[!0-9A-Fa-f]*) fail "unexpected SHA-256 output for $FINGERPRINT_PATH" ;;
	esac
	printf '%s' "$FINGERPRINT"
}
if ! SOURCE_VERSION=$(runuser -u "$TARGET_USER" -- "$TUI_BINARY" -version 2>/dev/null); then
	fail "$TUI_BINARY cannot run as $TARGET_USER; verify its architecture and permissions"
fi
[ -n "$SOURCE_VERSION" ] || fail "$TUI_BINARY returned an empty version"
SOURCE_VERSION_LOWER=$(printf '%s' "$SOURCE_VERSION" | tr '[:upper:]' '[:lower:]')
if [ "$ALLOW_DEVELOPMENT_BUILD" -ne 1 ]; then
	case "$SOURCE_VERSION_LOWER" in
	*dev* | *dirty* | *unknown*) fail "$TUI_BINARY is a development or unverifiable build ($SOURCE_VERSION); rebuild a tagged release or use --allow-development-build for testing" ;;
	esac
fi
SOURCE_SHA256=$(binary_sha256 "$TUI_BINARY")

if [ "$USE_RELEASE" -eq 1 ] && [ "$RELEASE_VERSION" != latest ]; then
	case "$SOURCE_VERSION" in
	"$RELEASE_VERSION" | "$RELEASE_VERSION ("*) ;;
	*) fail "downloaded binary reports $SOURCE_VERSION, expected release $RELEASE_VERSION" ;;
	esac
fi

if [ -z "$CHECKSUMS_PATH" ] && [ "$DEFAULT_BINARY" -eq 1 ]; then
	CHECKSUMS_PATH=$ROOT_DIR/dist/SHA256SUMS
fi
if [ -n "$CHECKSUMS_PATH" ]; then
	case "$CHECKSUMS_PATH" in
	/*) ;;
	*) fail "the checksums path must be absolute" ;;
	esac
	if [ ! -f "$CHECKSUMS_PATH" ] || [ -L "$CHECKSUMS_PATH" ]; then
		fail "$CHECKSUMS_PATH is missing, not regular, or a symbolic link"
	fi
	EXPECTED_SHA256=$(awk -v name="${TUI_BINARY##*/}" '$2 == name { print $1; found++ } END { if (found != 1) exit 1 }' "$CHECKSUMS_PATH") || fail "$CHECKSUMS_PATH must contain exactly one entry for ${TUI_BINARY##*/}"
	[ "$EXPECTED_SHA256" = "$SOURCE_SHA256" ] || fail "$TUI_BINARY does not match $CHECKSUMS_PATH"
fi

INSTALL_STATUS=disabled
if [ "$INSTALL_BINARY" -eq 1 ]; then
	for REQUIRED_COMMAND in chmod chown cmp install mktemp mv stat; do
		command -v "$REQUIRED_COMMAND" >/dev/null 2>&1 || fail "$REQUIRED_COMMAND is required to install the TUI command"
	done
	case "$INSTALL_PATH" in
	/*) ;;
	*) fail "the install path must be absolute" ;;
	esac
	case "$INSTALL_PATH" in
	*[!A-Za-z0-9._/-]*) fail "the install path contains unsupported characters" ;;
	esac
	[ "$TUI_BINARY" != "$INSTALL_PATH" ] || fail "the source and install paths must differ; use --no-install to run in place"
	[ ! -L "$INSTALL_PATH" ] || fail "refusing to replace a symbolic-link install path"
	if [ -e "$INSTALL_PATH" ] && [ ! -f "$INSTALL_PATH" ]; then
		fail "$INSTALL_PATH exists but is not a regular file"
	fi
	INSTALL_DIR=${INSTALL_PATH%/*}
	[ -n "$INSTALL_DIR" ] || INSTALL_DIR=/
	validate_install_directory() {
		[ -d "$INSTALL_DIR" ] || fail "$INSTALL_DIR is not a directory"
		[ ! -L "$INSTALL_DIR" ] || fail "refusing a symbolic-link install directory: $INSTALL_DIR"
		[ "$(stat -c '%u' "$INSTALL_DIR")" = 0 ] || fail "install directory must be owned by root: $INSTALL_DIR"
		INSTALL_DIR_MODE=$(stat -c '%a' "$INSTALL_DIR")
		case "$INSTALL_DIR_MODE" in
		*[2367][0-7] | *[0-7][2367]) fail "install directory must not be group- or world-writable: $INSTALL_DIR" ;;
		esac
	}
	if [ -e "$INSTALL_DIR" ]; then
		validate_install_directory
	fi
	installed_binary_is_current() {
		[ -f "$INSTALL_PATH" ] &&
			[ ! -L "$INSTALL_PATH" ] &&
			cmp -s "$TUI_BINARY" "$INSTALL_PATH" &&
			[ "$(stat -c '%u:%g:%a:%h' "$INSTALL_PATH")" = '0:0:755:1' ]
	}
	if installed_binary_is_current; then
		INSTALL_STATUS='already current'
	elif [ -f "$INSTALL_PATH" ] && cmp -s "$TUI_BINARY" "$INSTALL_PATH"; then
		INSTALL_STATUS='will repair ownership or mode'
	elif [ -e "$INSTALL_PATH" ]; then
		INSTALL_STATUS='will update'
	else
		INSTALL_STATUS='will install'
	fi
fi

printf 'soju-tui system setup\n\n'
printf '  Local administrator: %s\n' "$TARGET_USER"
printf '  Soju config:         %s\n' "$CONFIG_PATH"
printf '  Admin socket:        %s\n' "$SOCKET_PATH"
printf '  sojuctl:             %s\n' "$SOJUCTL_PATH"
printf '  TUI binary:          %s\n' "$TUI_BINARY"
printf '  TUI build:           %s\n' "$SOURCE_VERSION"
printf '  TUI SHA-256:         %s\n' "$SOURCE_SHA256"
if [ "$USE_RELEASE" -eq 1 ]; then
	printf '  Release source:      GitHub %s\n' "$RELEASE_VERSION"
fi
printf '  Checksums manifest:  %s\n' "${CHECKSUMS_PATH:-not supplied (custom build)}"
if [ "$INSTALL_BINARY" -eq 1 ]; then
	printf '  Installed command:   %s (%s)\n\n' "$INSTALL_PATH" "$INSTALL_STATUS"
else
	printf '  Installed command:   disabled (--no-install)\n\n'
fi

install_tui_binary() {
	[ "$INSTALL_BINARY" -eq 1 ] || return 0
	if installed_binary_is_current; then
		printf 'Installed command is already current: %s\n' "$INSTALL_PATH"
	else
		if { [ "$INSTALL_STATUS" = 'will update' ] || [ "$INSTALL_STATUS" = 'will repair ownership or mode' ]; } &&
			[ "$ASSUME_YES" -ne 1 ]; then
			printf 'Replace the existing regular file at %s now? [y/N] ' "$INSTALL_PATH"
			IFS= read -r ANSWER
			case "$ANSWER" in
			y | Y | yes | YES) ;;
			*) fail "cancelled before replacing $INSTALL_PATH" ;;
			esac
		fi
		if [ -e "$INSTALL_DIR" ]; then
			validate_install_directory
		else
			install -d -m 0755 "$INSTALL_DIR"
			validate_install_directory
		fi
		TEMP_BINARY=$(mktemp "$INSTALL_DIR/.soju-tui.install.XXXXXX") || fail "cannot create a temporary install file in $INSTALL_DIR"
		install -m 0755 "$TUI_BINARY" "$TEMP_BINARY"
		chown 0:0 "$TEMP_BINARY"
		chmod 0755 "$TEMP_BINARY"
		mv -f "$TEMP_BINARY" "$INSTALL_PATH"
		TEMP_BINARY=
		[ -x "$INSTALL_PATH" ] || fail "installed command is not executable: $INSTALL_PATH"
		installed_binary_is_current || fail "installed command failed content, ownership, mode, or link-count verification"
		printf 'Installed command: %s\n' "$INSTALL_PATH"
	fi
	INSTALLED_SHA256=$(binary_sha256 "$INSTALL_PATH")
	[ "$INSTALLED_SHA256" = "$SOURCE_SHA256" ] || fail "installed command SHA-256 does not match the selected source binary"
	printf 'Verified installed SHA-256: %s\n' "$INSTALLED_SHA256"
}

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

install_tui_binary

printf '\nVerifying sojuctl as local user %s...\n' "$TARGET_USER"
runuser -u "$TARGET_USER" -- "$SOJUCTL_PATH" -config "$CONFIG_PATH" server status

if [ "$INSTALL_BINARY" -eq 1 ]; then
	printf '\nVerifying installed command as local user %s...\n' "$TARGET_USER"
	INSTALLED_VERSION=$(runuser -u "$TARGET_USER" -- "$INSTALL_PATH" -version)
	[ "$INSTALLED_VERSION" = "$SOURCE_VERSION" ] || fail "installed command reports a different build than $TUI_BINARY"
	printf '%s\n' "$INSTALLED_VERSION"
	if [ "$INSTALL_PATH" = /usr/local/bin/soju-tui ]; then
		printf '\nSetup complete. Run as %s:\n  soju-tui\n' "$TARGET_USER"
	else
		printf '\nSetup complete. Run as %s:\n  %s\n' "$TARGET_USER" "$INSTALL_PATH"
	fi
else
	if [ "$USE_RELEASE" -eq 1 ]; then
		printf '\nRelease validation complete; no command was installed.\n'
		printf 'Rerun setup without --no-install to install soju-tui for %s.\n' "$TARGET_USER"
	else
		printf '\nSetup complete without command installation. Run as %s:\n  %s\n' "$TARGET_USER" "$TUI_BINARY"
	fi
fi
printf 'The first TUI run will show the discovered hostname, admin socket, TLS certificate, config, and sojuctl paths for confirmation.\n'
