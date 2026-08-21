#!/bin/sh
set -eu

ROOT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$ROOT_DIR"

TARGET=${TARGET:-linux-amd64}
VERSION=${VERSION:-dev}
PULL=0

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
	git pull --ff-only
fi

command -v go >/dev/null 2>&1 || { echo "Go is required to build soju-tui" >&2; exit 1; }

go mod download
go test ./...
go vet ./...

mkdir -p dist
LDFLAGS="-s -w -X main.version=$VERSION"

build_target() {
	target=$1
	case "$target" in
		host)
		go build -trimpath -ldflags "$LDFLAGS" -o "dist/soju-tui" .
		;;
		linux-amd64)
		CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$LDFLAGS" -o "dist/soju-tui-linux-amd64" .
		;;
		linux-arm64)
		CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$LDFLAGS" -o "dist/soju-tui-linux-arm64" .
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
