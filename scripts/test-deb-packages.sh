#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
DIST_DIR=$ROOT_DIR/dist
RUN_CONTAINERS=0

fail() {
	printf 'Debian package integration test: %s\n' "$1" >&2
	exit 1
}

usage() {
	cat <<'EOF'
Usage: scripts/test-deb-packages.sh [--containers]

Validates both Debian package architectures, release checksums, installed
paths, permissions, documentation, and absence of maintainer or systemd side
effects. With --containers, also exercises install, upgrade, and removal in
clean Debian and Ubuntu containers.
EOF
}

while [ "$#" -gt 0 ]; do
	case "$1" in
	--containers)
		RUN_CONTAINERS=1
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

for REQUIRED_COMMAND in cmp dpkg-deb file gzip md5sum mktemp sha256sum stat; do
	command -v "$REQUIRED_COMMAND" >/dev/null 2>&1 || fail "$REQUIRED_COMMAND is required"
done

WORK_DIR=$(mktemp -d /tmp/soju-tui-deb-test.XXXXXX) || fail "cannot create a temporary directory"
cleanup() {
	case "$WORK_DIR" in
	/tmp/soju-tui-deb-test.*) rm -rf -- "$WORK_DIR" ;;
	esac
}
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

BUILD_VERSION=$(awk -F= '$1 == "version" { value = substr($0, 9); found++ } END { if (found != 1 || value == "") exit 1; print value }' "$DIST_DIR/BUILDINFO") ||
	fail "dist/BUILDINFO must contain exactly one non-empty version entry"
case "$BUILD_VERSION" in
*-*) DEBIAN_UPSTREAM_VERSION=${BUILD_VERSION%%-*}~${BUILD_VERSION#*-} ;;
*) DEBIAN_UPSTREAM_VERSION=$BUILD_VERSION ;;
esac
DEBIAN_VERSION=$DEBIAN_UPSTREAM_VERSION-1
AMD64_PACKAGE=$DIST_DIR/soju-tui_${DEBIAN_VERSION}_amd64.deb
ARM64_PACKAGE=$DIST_DIR/soju-tui_${DEBIAN_VERSION}_arm64.deb

(
	cd "$DIST_DIR"
	sha256sum -c SHA256SUMS
)
[ -f "$AMD64_PACKAGE" ] || fail "expected $AMD64_PACKAGE"
[ -f "$ARM64_PACKAGE" ] || fail "expected $ARM64_PACKAGE"
sha256sum "$AMD64_PACKAGE" "$ARM64_PACKAGE" >"$WORK_DIR/package-hashes-before"
"$ROOT_DIR/scripts/build-deb.sh" >/dev/null
sha256sum "$AMD64_PACKAGE" "$ARM64_PACKAGE" >"$WORK_DIR/package-hashes-after"
cmp -s "$WORK_DIR/package-hashes-before" "$WORK_DIR/package-hashes-after" ||
	fail "rebuilding the packages did not reproduce identical artifacts"

package_for_architecture() {
	ARCHITECTURE=$1
	PACKAGE_PATH=$DIST_DIR/soju-tui_${DEBIAN_VERSION}_${ARCHITECTURE}.deb
	[ -f "$PACKAGE_PATH" ] || fail "expected $PACKAGE_PATH"
	printf '%s\n' "$PACKAGE_PATH"
}

validate_package() {
	DEBIAN_ARCH=$1
	BINARY_ARCH=$2
	RAW_BINARY=$3
	PACKAGE_PATH=$(package_for_architecture "$DEBIAN_ARCH")
	EXTRACT_ROOT=$WORK_DIR/extract-$DEBIAN_ARCH
	CONTROL_ROOT=$WORK_DIR/control-$DEBIAN_ARCH

	[ "$(dpkg-deb -f "$PACKAGE_PATH" Package)" = soju-tui ] || fail "$PACKAGE_PATH has an unexpected package name"
	[ "$(dpkg-deb -f "$PACKAGE_PATH" Architecture)" = "$DEBIAN_ARCH" ] || fail "$PACKAGE_PATH has an unexpected architecture"
	PACKAGE_VERSION=$(dpkg-deb -f "$PACKAGE_PATH" Version)
	[ -n "$PACKAGE_VERSION" ] || fail "$PACKAGE_PATH has an empty version"

	dpkg-deb -x "$PACKAGE_PATH" "$EXTRACT_ROOT"
	dpkg-deb -e "$PACKAGE_PATH" "$CONTROL_ROOT"
	[ -x "$EXTRACT_ROOT/usr/bin/soju-tui" ] || fail "$PACKAGE_PATH does not install /usr/bin/soju-tui as executable"
	[ "$(stat -c '%a' "$EXTRACT_ROOT/usr/bin/soju-tui")" = 755 ] || fail "$PACKAGE_PATH uses the wrong binary mode"
	cmp -s "$DIST_DIR/$RAW_BINARY" "$EXTRACT_ROOT/usr/bin/soju-tui" || fail "$PACKAGE_PATH binary differs from $RAW_BINARY"
	[ -x "$EXTRACT_ROOT/usr/sbin/soju-tui-setup" ] || fail "$PACKAGE_PATH is missing the explicit setup helper"
	[ "$(stat -c '%a' "$EXTRACT_ROOT/usr/sbin/soju-tui-setup")" = 755 ] || fail "$PACKAGE_PATH uses the wrong setup-helper mode"
	[ -x "$EXTRACT_ROOT/usr/lib/soju-tui/scripts/setup.sh" ] || fail "$PACKAGE_PATH is missing the guided setup implementation"
	[ -x "$EXTRACT_ROOT/usr/lib/soju-tui/scripts/grant-admin-access.sh" ] || fail "$PACKAGE_PATH is missing the ACL helper"
	[ -f "$EXTRACT_ROOT/usr/share/doc/soju-tui/README.md" ] || fail "$PACKAGE_PATH is missing README documentation"
	[ -f "$EXTRACT_ROOT/usr/share/doc/soju-tui/README.Debian" ] || fail "$PACKAGE_PATH is missing Debian guidance"
	[ -f "$EXTRACT_ROOT/usr/share/doc/soju-tui/BUILDINFO" ] || fail "$PACKAGE_PATH is missing build metadata"
	cmp -s "$DIST_DIR/BUILDINFO" "$EXTRACT_ROOT/usr/share/doc/soju-tui/BUILDINFO" || fail "$PACKAGE_PATH contains different build metadata"
	[ -f "$EXTRACT_ROOT/usr/share/doc/soju-tui/THIRD_PARTY_LICENSES" ] || fail "$PACKAGE_PATH is missing dependency license notices"
	[ -f "$EXTRACT_ROOT/usr/share/man/man1/soju-tui.1.gz" ] || fail "$PACKAGE_PATH is missing the soju-tui man page"
	[ -f "$EXTRACT_ROOT/usr/share/man/man8/soju-tui-setup.8.gz" ] || fail "$PACKAGE_PATH is missing the setup man page"
	gzip -t "$EXTRACT_ROOT/usr/share/man/man1/soju-tui.1.gz"
	gzip -t "$EXTRACT_ROOT/usr/share/man/man8/soju-tui-setup.8.gz"
	(
		cd "$EXTRACT_ROOT"
		md5sum -c "$CONTROL_ROOT/md5sums" >/dev/null
	) || fail "$PACKAGE_PATH contains a file that does not match DEBIAN/md5sums"
	[ ! -e "$EXTRACT_ROOT/usr/local" ] || fail "$PACKAGE_PATH must not install under /usr/local"
	[ ! -e "$EXTRACT_ROOT/etc" ] || fail "$PACKAGE_PATH must not install configuration under /etc"
	if find "$EXTRACT_ROOT" -type l -print -quit | grep -q .; then
		fail "$PACKAGE_PATH unexpectedly contains a symbolic link"
	fi
	if ! dpkg-deb -c "$PACKAGE_PATH" | awk 'NF && $2 != "root/root" { exit 1 }'; then
		fail "$PACKAGE_PATH contains a non-root-owned archive entry"
	fi

	FILE_DESCRIPTION=$(file -b "$EXTRACT_ROOT/usr/bin/soju-tui")
	case "$BINARY_ARCH:$FILE_DESCRIPTION" in
	amd64:*x86-64*) ;;
	arm64:*ARM\ aarch64*) ;;
	*) fail "$PACKAGE_PATH contains the wrong executable architecture: $FILE_DESCRIPTION" ;;
	esac

	for FORBIDDEN_SCRIPT in preinst postinst prerm postrm config triggers; do
		[ ! -e "$CONTROL_ROOT/$FORBIDDEN_SCRIPT" ] || fail "$PACKAGE_PATH unexpectedly contains $FORBIDDEN_SCRIPT"
	done
	if find "$EXTRACT_ROOT" -path '*/systemd/system/*' -print -quit | grep -q .; then
		fail "$PACKAGE_PATH unexpectedly installs a systemd unit"
	fi
	printf 'validated %s (%s)\n' "${PACKAGE_PATH##*/}" "$PACKAGE_VERSION"
}

validate_package amd64 amd64 soju-tui-linux-amd64
validate_package arm64 arm64 soju-tui-linux-arm64

if command -v lintian >/dev/null 2>&1; then
	lintian --fail-on error "$AMD64_PACKAGE" "$ARM64_PACKAGE"
fi

if [ "$RUN_CONTAINERS" -eq 0 ]; then
	printf 'Debian package metadata and contents passed validation\n'
	exit 0
fi

command -v docker >/dev/null 2>&1 || fail "docker is required with --containers"
CURRENT_PACKAGE=$(package_for_architecture amd64)
CURRENT_ARM64_PACKAGE=$(package_for_architecture arm64)
CURRENT_VERSION=$(dpkg-deb -f "$CURRENT_PACKAGE" Version)
PREVIOUS_PACKAGE=$WORK_DIR/soju-tui_previous_amd64.deb
PREVIOUS_ARM64_PACKAGE=$WORK_DIR/soju-tui_previous_arm64.deb

make_previous_package() {
	CURRENT_PATH=$1
	PREVIOUS_PATH=$2
	PREVIOUS_ARCH=$3
	PREVIOUS_ROOT=$WORK_DIR/previous-root-$PREVIOUS_ARCH
	dpkg-deb -R "$CURRENT_PATH" "$PREVIOUS_ROOT"
	awk 'BEGIN { replaced = 0 } /^Version:/ { print "Version: 0.0.0~previous-1"; replaced = 1; next } { print } END { if (!replaced) exit 1 }' \
		"$PREVIOUS_ROOT/DEBIAN/control" >"$PREVIOUS_ROOT/DEBIAN/control.new"
	mv "$PREVIOUS_ROOT/DEBIAN/control.new" "$PREVIOUS_ROOT/DEBIAN/control"
	dpkg-deb --build --root-owner-group "$PREVIOUS_ROOT" "$PREVIOUS_PATH" >/dev/null
}

make_previous_package "$CURRENT_PACKAGE" "$PREVIOUS_PACKAGE" amd64
make_previous_package "$CURRENT_ARM64_PACKAGE" "$PREVIOUS_ARM64_PACKAGE" arm64

CONTAINER_PACKAGES=$WORK_DIR/container-packages
mkdir -p "$CONTAINER_PACKAGES"
cp "$CURRENT_PACKAGE" "$CONTAINER_PACKAGES/current.deb"
cp "$PREVIOUS_PACKAGE" "$CONTAINER_PACKAGES/previous.deb"
cp "$CURRENT_ARM64_PACKAGE" "$CONTAINER_PACKAGES/current-arm64.deb"
cp "$PREVIOUS_ARM64_PACKAGE" "$CONTAINER_PACKAGES/previous-arm64.deb"

for IMAGE in debian:12-slim ubuntu:24.04; do
	printf 'testing package lifecycle in %s\n' "$IMAGE"
	docker run --rm --network none \
		-e EXPECTED_VERSION="$CURRENT_VERSION" \
		-v "$CONTAINER_PACKAGES:/packages:ro" \
		"$IMAGE" sh -euxc '
			test ! -e /usr/bin/soju-tui
			test ! -e /etc/soju
			apt-get install -y /packages/previous.deb
			test "$(dpkg-query -W -f="\${Version}" soju-tui)" = "0.0.0~previous-1"
			/usr/bin/soju-tui -version
			/usr/sbin/soju-tui-setup --help >/dev/null
			test ! -e /etc/systemd/system/soju-tui.service
			test ! -e /etc/soju
			apt-get install -y /packages/current.deb
			test "$(dpkg-query -W -f="\${Version}" soju-tui)" = "$EXPECTED_VERSION"
			/usr/bin/soju-tui -version
			dpkg -r soju-tui
			test ! -e /usr/bin/soju-tui
			test ! -e /usr/sbin/soju-tui-setup
			test ! -e /usr/lib/soju-tui
			test ! -e /etc/systemd/system/soju-tui.service
			dpkg -P soju-tui
			dpkg --force-architecture -i /packages/previous-arm64.deb
			test "$(dpkg-query -W -f="\${Version}" soju-tui)" = "0.0.0~previous-1"
			test "$(dpkg-query -W -f="\${Architecture}" soju-tui)" = "arm64"
			dpkg --force-architecture -i /packages/current-arm64.deb
			test "$(dpkg-query -W -f="\${Version}" soju-tui)" = "$EXPECTED_VERSION"
			test "$(dpkg-query -W -f="\${Architecture}" soju-tui)" = "arm64"
			dpkg -r soju-tui
			test ! -e /usr/bin/soju-tui
			test ! -e /usr/sbin/soju-tui-setup
			test ! -e /etc/soju
		'
done

printf 'Debian and Ubuntu package lifecycle tests passed\n'
