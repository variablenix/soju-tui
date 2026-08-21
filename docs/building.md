# Building and releases

## Build helper

`soju-tui` is implemented in Go with the pure-Go `tcell` terminal library. It
does not use ncurses. Go is required only when building from source; operators
running a tracked prebuilt Linux artifact do not need a Go installation.

Source builds require Go 1.26.6 or newer so they contain the standard-library
fix for GO-2026-5972. The helper checks scripts, downloads modules, runs tests
and vet, then writes executables to `dist/`.

Build the common Linux target after source updates:

```sh
./scripts/build.sh --target linux-amd64
```

Other targets:

```sh
./scripts/build.sh --target linux-arm64
./scripts/build.sh --target all
./scripts/build.sh --target host
```

On a configured Linux host, install or update the short command after a
successful build:

```sh
./scripts/setup.sh
soju-tui -version
```

The setup wizard copies the architecture-matched artifact to
`/usr/local/bin/soju-tui`. It does not copy an unchanged artifact. Build output
includes the source revision, and setup prints content fingerprints before and
after installation so an update can be verified. Use
`--binary /absolute/path` for a custom build or `--no-install` to keep running
from `dist/`.

To fast-forward from Git before building:

```sh
./scripts/build.sh --pull --target linux-amd64
```

The helper prints each phase and defaults to `GOTOOLCHAIN=local`. Set
`GOTOOLCHAIN=auto` explicitly if automatic toolchain downloads are acceptable.
If the local toolchain is below the version required by `go.mod`, the helper
stops before tests or builds and prints that exact opt-in command.

## Versions

Ordinary commits and local rebuilds do not require a version bump. Use
`--version X.Y.Z` only when intentionally producing a numbered release:

```sh
./scripts/build.sh --target all --version 0.3.0
```

Development artifacts may use a suffix such as `0.3.0-dev`.

## Make targets

```sh
make linux-amd64
make linux-arm64
make test
make vet
```

## Verification

```sh
go test ./...
go test -race ./...
go vet ./...
sh -n scripts/build.sh scripts/grant-admin-access.sh scripts/setup.sh
```

Linux release artifacts use `CGO_ENABLED=0` and are statically linked. They do
not need a separate Go runtime, C runtime, or ncurses library. They still invoke
the host's `sojuctl` executable to administer the running Soju instance. Confirm
the downloaded architecture with `file dist/soju-tui-linux-*` when diagnosing
an execution-format error.
