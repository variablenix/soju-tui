#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR"

TARGET=${TARGET:-linux-amd64}
VERSION=${VERSION:-dev}
PULL=0
RELEASE=0
ALLOW_UNSIGNED_TAG=0
GITHUB_ACTIONS_RELEASE=0
GO_BIN=${GO_BIN:-go}
GO_TOOLCHAIN=${GOTOOLCHAIN:-local}
REVISION=${REVISION:-}
BUILT_ARTIFACTS=

log() {
	printf '[soju-tui] %s\n' "$1"
}

go_cmd() {
	GOTOOLCHAIN="$GO_TOOLCHAIN" "$GO_BIN" "$@"
}

usage() {
	cat <<'EOF'
Usage: scripts/build.sh [--target TARGET] [--version VERSION] [--pull] [--release]
                        [--allow-unsigned-tag] [--github-actions-release]

TARGET values:
  host         Build for the current Go host.
  linux-amd64  Build a static Linux x86_64 binary.
  linux-arm64  Build a static Linux ARM64 binary.
  all          Build both supported Linux targets.

--pull performs a fast-forward-only git pull before verification and build.
--release requires a clean exact vVERSION tag, builds both Linux targets, and
          refuses development version strings.
--allow-unsigned-tag permits an unsigned release tag for private test builds;
          production releases should not use it.
--github-actions-release permits the protected GitHub release workflow to use
          its repository-scoped identity instead of a personal GPG signature.
EOF
}

while [ "$#" -gt 0 ]; do
	case "$1" in
		--target)
			[ "$#" -ge 2 ] || { echo "--target requires a value" >&2; exit 2; }
			TARGET=$2
			shift 2
			;;
		--version)
			[ "$#" -ge 2 ] || { echo "--version requires a value" >&2; exit 2; }
			VERSION=$2
			shift 2
			;;
		--pull)
			PULL=1
			shift
			;;
		--release)
			RELEASE=1
			shift
			;;
		--allow-unsigned-tag)
			ALLOW_UNSIGNED_TAG=1
			shift
			;;
		--github-actions-release)
			GITHUB_ACTIONS_RELEASE=1
			shift
			;;
		-h|--help)
			usage
			exit 0
			;;
		*)
			echo "unknown argument: $1" >&2
			usage >&2
			exit 2
			;;
	esac
done

if [ "$PULL" -eq 1 ]; then
	log "pulling latest changes with fast-forward-only git pull"
	git pull --ff-only
fi

if [ "$GITHUB_ACTIONS_RELEASE" -eq 1 ] && [ "$RELEASE" -ne 1 ]; then
	echo "--github-actions-release requires --release" >&2
	exit 2
fi

command -v "$GO_BIN" >/dev/null 2>&1 || { echo "Go is required to build soju-tui" >&2; exit 1; }

case "$VERSION" in
*[!A-Za-z0-9._+-]*) echo "version contains unsupported characters" >&2; exit 2 ;;
esac
if [ -z "$REVISION" ]; then
	if command -v git >/dev/null 2>&1 && git rev-parse --is-inside-work-tree >/dev/null 2>&1; then
		REVISION=$(git rev-parse --short=12 HEAD)
		if [ -n "$(git status --porcelain --untracked-files=normal)" ]; then
			REVISION=$REVISION-dirty
		fi
	else
		REVISION=unknown
	fi
fi
case "$REVISION" in
*[!A-Za-z0-9._+-]*) echo "revision contains unsupported characters" >&2; exit 2 ;;
esac

if [ "$RELEASE" -eq 1 ]; then
	[ "$TARGET" = all ] || { echo "--release requires --target all" >&2; exit 2; }
	printf '%s\n' "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9][A-Za-z0-9.-]*)?$' || {
		echo "--release requires a numbered version such as 0.3.0" >&2
		exit 2
	}
	case "$VERSION:$REVISION" in
		*dev*|*dirty*|*unknown*) echo "release builds require a clean non-development revision" >&2; exit 2 ;;
	esac
	command -v git >/dev/null 2>&1 || { echo "git is required for a release build" >&2; exit 1; }
	[ "$(git describe --exact-match --match "v$VERSION" 2>/dev/null || true)" = "v$VERSION" ] || {
		echo "release build requires HEAD to be tagged v$VERSION" >&2
		exit 2
	}
	if [ "$GITHUB_ACTIONS_RELEASE" -eq 1 ]; then
		[ "${GITHUB_ACTIONS:-}" = true ] || {
			echo "--github-actions-release is restricted to GitHub Actions" >&2
			exit 2
		}
		[ "${GITHUB_SERVER_URL:-}" = https://github.com ] || {
			echo "--github-actions-release requires github.com" >&2
			exit 2
		}
		[ "${GITHUB_EVENT_NAME:-}" = workflow_dispatch ] || {
			echo "--github-actions-release requires a manually dispatched workflow" >&2
			exit 2
		}
		[ "${GITHUB_REF:-}" = refs/heads/main ] || {
			echo "--github-actions-release requires refs/heads/main" >&2
			exit 2
		}
		[ "${GITHUB_SHA:-}" = "$(git rev-parse HEAD)" ] || {
			echo "--github-actions-release requires GITHUB_SHA to match HEAD" >&2
			exit 2
		}
	fi
	if [ "$ALLOW_UNSIGNED_TAG" -ne 1 ] && ! git tag -v "v$VERSION" >/dev/null 2>&1; then
		if [ "$GITHUB_ACTIONS_RELEASE" -ne 1 ]; then
			echo "release tag v$VERSION is not a valid signature from a trusted local Git key" >&2
			echo "sign the production tag, or use the protected GitHub release workflow" >&2
			exit 2
		fi
	fi
	export SOURCE_DATE_EPOCH
	SOURCE_DATE_EPOCH=$(git show -s --format=%ct HEAD)
fi

log "toolchain policy: GOTOOLCHAIN=$GO_TOOLCHAIN"
log "using $(go_cmd version)"
log "build revision: $REVISION"

log "checking shell helpers"
sh -n scripts/build.sh
sh -n scripts/grant-admin-access.sh
sh -n scripts/setup.sh
sh -n scripts/test-soju-compat.sh
sh -n scripts/check-coverage.sh
sh -n scripts/build-deb.sh
sh -n scripts/test-deb-packages.sh
sh -n packaging/debian/soju-tui-setup
sh -n scripts/generate-third-party-licenses.sh
log "downloading Go modules"
if ! go_cmd mod download; then
	if [ "$GO_TOOLCHAIN" = local ]; then
		printf '%s\n' 'The installed local Go toolchain may be older than the version required by go.mod.' >&2
		printf '%s\n' 'Upgrade Go, or explicitly allow Go to fetch the required toolchain:' >&2
		printf '%s\n' '  GOTOOLCHAIN=auto ./scripts/build.sh --target linux-amd64' >&2
	fi
	exit 1
fi
log "verifying Go modules"
go_cmd mod verify
log "running tests"
go_cmd test ./...
log "running go vet"
go_cmd vet ./...

mkdir -p dist
LDFLAGS="-s -w -X main.version=$VERSION -X main.revision=$REVISION"

build_target() {
	target=$1
	log "building $target version $VERSION"
	case "$target" in
		host)
		go_cmd build -buildvcs=false -trimpath -ldflags "$LDFLAGS" -o "dist/soju-tui" .
		BUILT_ARTIFACTS="$BUILT_ARTIFACTS dist/soju-tui"
		;;
		linux-amd64)
		CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go_cmd build -buildvcs=false -trimpath -ldflags "$LDFLAGS" -o "dist/soju-tui-linux-amd64" .
		BUILT_ARTIFACTS="$BUILT_ARTIFACTS dist/soju-tui-linux-amd64"
		;;
		linux-arm64)
		CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go_cmd build -buildvcs=false -trimpath -ldflags "$LDFLAGS" -o "dist/soju-tui-linux-arm64" .
		BUILT_ARTIFACTS="$BUILT_ARTIFACTS dist/soju-tui-linux-arm64"
		;;
		*)
			echo "unknown target: $target" >&2
			exit 2
		;;
	esac
}

case "$TARGET" in
	all)
		build_target linux-amd64
		build_target linux-arm64
		;;
	host|linux-amd64|linux-arm64)
		build_target "$TARGET"
		;;
	*)
		echo "unknown target: $TARGET" >&2
		exit 2
		;;
esac

case "$TARGET" in
all)
	./scripts/generate-third-party-licenses.sh "$ROOT_DIR/dist/THIRD_PARTY_LICENSES" \
		"$ROOT_DIR/dist/soju-tui-linux-amd64" "$ROOT_DIR/dist/soju-tui-linux-arm64"
	;;
host)
	./scripts/generate-third-party-licenses.sh "$ROOT_DIR/dist/THIRD_PARTY_LICENSES" "$ROOT_DIR/dist/soju-tui"
	;;
linux-amd64)
	./scripts/generate-third-party-licenses.sh "$ROOT_DIR/dist/THIRD_PARTY_LICENSES" "$ROOT_DIR/dist/soju-tui-linux-amd64"
	;;
linux-arm64)
	./scripts/generate-third-party-licenses.sh "$ROOT_DIR/dist/THIRD_PARTY_LICENSES" "$ROOT_DIR/dist/soju-tui-linux-arm64"
	;;
esac
BUILT_ARTIFACTS="$BUILT_ARTIFACTS dist/THIRD_PARTY_LICENSES"

sha256_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{ print $1 }'
	elif command -v shasum >/dev/null 2>&1; then
		shasum -a 256 "$1" | awk '{ print $1 }'
	else
		echo "sha256sum or shasum is required to create the artifact manifest" >&2
		exit 1
	fi
}

{
	printf 'version=%s\n' "$VERSION"
	printf 'revision=%s\n' "$REVISION"
	printf 'go=%s\n' "$(go_cmd version)"
	printf 'target=%s\n' "$TARGET"
} >dist/BUILDINFO
BUILT_ARTIFACTS="$BUILT_ARTIFACTS dist/BUILDINFO"

: >dist/SHA256SUMS
for artifact in $BUILT_ARTIFACTS; do
	printf '%s  %s\n' "$(sha256_file "$artifact")" "${artifact#dist/}" >>dist/SHA256SUMS
done

echo "Built $TARGET version $VERSION in $ROOT_DIR/dist with SHA256SUMS and BUILDINFO"
