#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR"

TARGET=${TARGET:-linux-amd64}
VERSION=${VERSION:-dev}
PULL=0
GO_BIN=${GO_BIN:-go}
GO_TOOLCHAIN=${GOTOOLCHAIN:-local}

log() {
	printf '[soju-tui] %s\n' "$1"
}

go_cmd() {
	GOTOOLCHAIN="$GO_TOOLCHAIN" "$GO_BIN" "$@"
}

usage() {
	cat <<'EOF'
Usage: scripts/build.sh [--target TARGET] [--version VERSION] [--pull]

TARGET values:
  host         Build for the current Go host.
  linux-amd64  Build a static Linux x86_64 binary.
  linux-arm64  Build a static Linux ARM64 binary.
  all          Build both supported Linux targets.

--pull performs a fast-forward-only git pull before verification and build.
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

command -v "$GO_BIN" >/dev/null 2>&1 || { echo "Go is required to build soju-tui" >&2; exit 1; }

log "using $($GO_BIN version)"
log "toolchain policy: GOTOOLCHAIN=$GO_TOOLCHAIN"

log "downloading Go modules"
go_cmd mod download
log "running tests"
go_cmd test ./...
log "running go vet"
go_cmd vet ./...

mkdir -p dist
LDFLAGS="-s -w -X main.version=$VERSION"

build_target() {
	target=$1
	log "building $target version $VERSION"
	case "$target" in
		host)
		go_cmd build -trimpath -ldflags "$LDFLAGS" -o "dist/soju-tui" .
		;;
		linux-amd64)
		CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go_cmd build -trimpath -ldflags "$LDFLAGS" -o "dist/soju-tui-linux-amd64" .
		;;
		linux-arm64)
		CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go_cmd build -trimpath -ldflags "$LDFLAGS" -o "dist/soju-tui-linux-arm64" .
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

echo "Built $TARGET version $VERSION in $ROOT_DIR/dist"
