#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
DIST_DIR=$ROOT_DIR/dist
BUILDINFO=$DIST_DIR/BUILDINFO
PACKAGE_REVISION=${PACKAGE_REVISION:-1}

fail() {
	printf 'soju-tui Debian package build: %s\n' "$1" >&2
	exit 1
}

for REQUIRED_COMMAND in awk date dpkg-deb du file find grep gzip install md5sum mktemp sha256sum xargs; do
	command -v "$REQUIRED_COMMAND" >/dev/null 2>&1 || fail "$REQUIRED_COMMAND is required"
done

[ -f "$BUILDINFO" ] || fail "$BUILDINFO is missing; run scripts/build.sh first"
[ ! -L "$BUILDINFO" ] || fail "refusing a symbolic-link BUILDINFO file"
[ -f "$DIST_DIR/SHA256SUMS" ] || fail "$DIST_DIR/SHA256SUMS is missing; run scripts/build.sh first"
[ ! -L "$DIST_DIR/SHA256SUMS" ] || fail "refusing a symbolic-link SHA256SUMS file"
[ -f "$DIST_DIR/THIRD_PARTY_LICENSES" ] || fail "$DIST_DIR/THIRD_PARTY_LICENSES is missing; run scripts/build.sh first"
[ ! -L "$DIST_DIR/THIRD_PARTY_LICENSES" ] || fail "refusing a symbolic-link THIRD_PARTY_LICENSES file"
(
	cd "$DIST_DIR"
	sha256sum -c SHA256SUMS
) || fail "the raw build artifacts do not match dist/SHA256SUMS"

buildinfo_value() {
	KEY=$1
	awk -F= -v key="$KEY" '$1 == key { value = substr($0, length(key) + 2); found++ } END { if (found != 1 || value == "") exit 1; print value }' "$BUILDINFO"
}

VERSION=$(buildinfo_value version) || fail "$BUILDINFO must contain exactly one non-empty version entry"
REVISION=$(buildinfo_value revision) || fail "$BUILDINFO must contain exactly one non-empty revision entry"
printf '%s\n' "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+([.+-][A-Za-z0-9][A-Za-z0-9.+-]*)?$' ||
	fail "BUILDINFO version must be a release or CI version, not $VERSION"
case "$REVISION" in
*[!A-Za-z0-9._+-]*) fail "BUILDINFO revision contains unsupported characters" ;;
esac
printf '%s\n' "$PACKAGE_REVISION" | grep -Eq '^[1-9][0-9]*$' || fail "PACKAGE_REVISION must be a positive integer"

case "$VERSION" in
*-*)
	DEBIAN_UPSTREAM_VERSION=${VERSION%%-*}~${VERSION#*-}
	;;
*)
	DEBIAN_UPSTREAM_VERSION=$VERSION
	;;
esac
DEBIAN_VERSION=$DEBIAN_UPSTREAM_VERSION-$PACKAGE_REVISION

if [ -n "${SOURCE_DATE_EPOCH:-}" ]; then
	case "$SOURCE_DATE_EPOCH" in
	*[!0-9]*) fail "SOURCE_DATE_EPOCH must contain only decimal digits" ;;
	esac
else
	SOURCE_DATE_EPOCH=$(git -C "$ROOT_DIR" show -s --format=%ct HEAD 2>/dev/null) ||
		fail "SOURCE_DATE_EPOCH is unset and the source revision timestamp is unavailable"
fi
export SOURCE_DATE_EPOCH

install_source_file() {
	SOURCE_PATH=$1
	FILE_MODE=$2
	DESTINATION_PATH=$3
	[ -f "$SOURCE_PATH" ] || fail "$SOURCE_PATH is missing or not a regular file"
	[ ! -L "$SOURCE_PATH" ] || fail "refusing a symbolic-link package input: $SOURCE_PATH"
	install -m "$FILE_MODE" "$SOURCE_PATH" "$DESTINATION_PATH"
}

WORK_DIR=$(mktemp -d /tmp/soju-tui-deb.XXXXXX) || fail "cannot create a temporary build directory"
cleanup() {
	case "$WORK_DIR" in
	/tmp/soju-tui-deb.*) rm -rf -- "$WORK_DIR" ;;
	esac
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

build_package() {
	DEBIAN_ARCH=$1
	BINARY_ARCH=$2
	BINARY_NAME=$3
	SOURCE_BINARY=$DIST_DIR/$BINARY_NAME
	PACKAGE_ROOT=$WORK_DIR/$DEBIAN_ARCH
	PACKAGE_NAME=soju-tui_${DEBIAN_VERSION}_${DEBIAN_ARCH}.deb
	PACKAGE_TEMP=$WORK_DIR/$PACKAGE_NAME
	PACKAGE_OUTPUT=$DIST_DIR/$PACKAGE_NAME
	CHANGELOG_DATE=$(date -u -d "@$SOURCE_DATE_EPOCH" '+%a, %d %b %Y %H:%M:%S +0000') ||
		fail "cannot format SOURCE_DATE_EPOCH"

	[ -f "$SOURCE_BINARY" ] || fail "$SOURCE_BINARY is missing; build both Linux targets first"
	[ ! -L "$SOURCE_BINARY" ] || fail "refusing a symbolic-link binary: $SOURCE_BINARY"
	[ -x "$SOURCE_BINARY" ] || fail "$SOURCE_BINARY is not executable"
	FILE_DESCRIPTION=$(file -b "$SOURCE_BINARY")
	case "$BINARY_ARCH:$FILE_DESCRIPTION" in
	amd64:*x86-64*) ;;
	arm64:*ARM\ aarch64*) ;;
	*) fail "$SOURCE_BINARY does not contain the expected $BINARY_ARCH ELF executable: $FILE_DESCRIPTION" ;;
	esac

	install -d -m 0755 \
		"$PACKAGE_ROOT/DEBIAN" \
		"$PACKAGE_ROOT/usr/bin" \
		"$PACKAGE_ROOT/usr/sbin" \
		"$PACKAGE_ROOT/usr/lib/soju-tui/scripts" \
		"$PACKAGE_ROOT/usr/share/doc/soju-tui/docs" \
		"$PACKAGE_ROOT/usr/share/man/man1" \
		"$PACKAGE_ROOT/usr/share/man/man8"
	install_source_file "$SOURCE_BINARY" 0755 "$PACKAGE_ROOT/usr/bin/soju-tui"
	install_source_file "$ROOT_DIR/packaging/debian/soju-tui-setup" 0755 "$PACKAGE_ROOT/usr/sbin/soju-tui-setup"
	install_source_file "$ROOT_DIR/scripts/setup.sh" 0755 "$PACKAGE_ROOT/usr/lib/soju-tui/scripts/setup.sh"
	install_source_file "$ROOT_DIR/scripts/grant-admin-access.sh" 0755 "$PACKAGE_ROOT/usr/lib/soju-tui/scripts/grant-admin-access.sh"
	install_source_file "$ROOT_DIR/README.md" 0644 "$PACKAGE_ROOT/usr/share/doc/soju-tui/README.md"
	install_source_file "$ROOT_DIR/LICENSE" 0644 "$PACKAGE_ROOT/usr/share/doc/soju-tui/LICENSE"
	install_source_file "$BUILDINFO" 0644 "$PACKAGE_ROOT/usr/share/doc/soju-tui/BUILDINFO"
	install_source_file "$DIST_DIR/THIRD_PARTY_LICENSES" 0644 "$PACKAGE_ROOT/usr/share/doc/soju-tui/THIRD_PARTY_LICENSES"
	install_source_file "$ROOT_DIR/packaging/debian/README.Debian" 0644 "$PACKAGE_ROOT/usr/share/doc/soju-tui/README.Debian"
	install_source_file "$ROOT_DIR/packaging/debian/copyright" 0644 "$PACKAGE_ROOT/usr/share/doc/soju-tui/copyright"
	for DOCUMENT in "$ROOT_DIR"/docs/*.md; do
		install_source_file "$DOCUMENT" 0644 "$PACKAGE_ROOT/usr/share/doc/soju-tui/docs/${DOCUMENT##*/}"
	done
	if [ ! -f "$ROOT_DIR/packaging/debian/soju-tui.1" ] || [ -L "$ROOT_DIR/packaging/debian/soju-tui.1" ]; then
		fail "invalid soju-tui man-page source"
	fi
	if [ ! -f "$ROOT_DIR/packaging/debian/soju-tui-setup.8" ] || [ -L "$ROOT_DIR/packaging/debian/soju-tui-setup.8" ]; then
		fail "invalid setup man-page source"
	fi
	gzip -9n -c "$ROOT_DIR/packaging/debian/soju-tui.1" >"$PACKAGE_ROOT/usr/share/man/man1/soju-tui.1.gz"
	gzip -9n -c "$ROOT_DIR/packaging/debian/soju-tui-setup.8" >"$PACKAGE_ROOT/usr/share/man/man8/soju-tui-setup.8.gz"
	{
		printf 'soju-tui (%s) stable; urgency=medium\n\n' "$DEBIAN_VERSION"
		printf '  * Package upstream soju-tui release %s (%s).\n\n' "$VERSION" "$REVISION"
		printf ' -- VariableNix <variablenix@users.noreply.github.com>  %s\n' "$CHANGELOG_DATE"
	} | gzip -9n >"$PACKAGE_ROOT/usr/share/doc/soju-tui/changelog.Debian.gz"

	INSTALLED_SIZE=$(du -sk "$PACKAGE_ROOT/usr" | awk '{ print $1 }')
	cat >"$PACKAGE_ROOT/DEBIAN/control" <<EOF
Package: soju-tui
Version: $DEBIAN_VERSION
Section: net
Priority: optional
Architecture: $DEBIAN_ARCH
Maintainer: VariableNix <variablenix@users.noreply.github.com>
Homepage: https://github.com/variablenix/soju-tui
Installed-Size: $INSTALLED_SIZE
Suggests: soju, acl, systemd
Description: terminal administration frontend for the soju IRC bouncer
 soju-tui is an interactive, administration-only terminal frontend for soju.
 It uses sojuctl and a local administrative Unix socket to manage users,
 networks, channels, SASL, certificates, and server status.
 .
 Package installation does not grant admin-socket access. A trusted local
 administrator can explicitly run soju-tui-setup when access is required.
EOF
	(
		cd "$PACKAGE_ROOT"
		find usr -type f -print | LC_ALL=C sort | xargs md5sum >DEBIAN/md5sums
	)
	find "$PACKAGE_ROOT" -print0 | xargs -0 touch -h -d "@$SOURCE_DATE_EPOCH"
	dpkg-deb --build --root-owner-group "$PACKAGE_ROOT" "$PACKAGE_TEMP" >/dev/null
	mv -f "$PACKAGE_TEMP" "$PACKAGE_OUTPUT"
	printf '[soju-tui] built %s\n' "$PACKAGE_OUTPUT"
}

build_package amd64 amd64 soju-tui-linux-amd64
build_package arm64 arm64 soju-tui-linux-arm64

CHECKSUM_TEMP=$WORK_DIR/SHA256SUMS
awk '$2 !~ /^soju-tui_.*_(amd64|arm64)\.deb$/ { print }' "$DIST_DIR/SHA256SUMS" >"$CHECKSUM_TEMP"
for PACKAGE_PATH in "$DIST_DIR"/soju-tui_"$DEBIAN_VERSION"_amd64.deb "$DIST_DIR"/soju-tui_"$DEBIAN_VERSION"_arm64.deb; do
	printf '%s  %s\n' "$(sha256sum "$PACKAGE_PATH" | awk '{ print $1 }')" "${PACKAGE_PATH##*/}" >>"$CHECKSUM_TEMP"
done
mv -f "$CHECKSUM_TEMP" "$DIST_DIR/SHA256SUMS"
printf '[soju-tui] Debian packages use version %s and are included in dist/SHA256SUMS\n' "$DEBIAN_VERSION"
