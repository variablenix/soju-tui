# Building and releases

## Build helper

`soju-tui` is implemented in Go with the pure-Go `tcell` terminal library. It
does not use ncurses. Go is required only when building from source; operators
running a prebuilt Linux artifact do not need a Go installation.

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
includes the source revision, writes `dist/SHA256SUMS` and `dist/BUILDINFO`, and
setup verifies SHA-256 before and after installation. Use
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

Ordinary commits and local rebuilds do not require a version bump. Development
builds remain explicitly marked and are not accepted by production setup unless
the operator supplies `--allow-development-build`.

A production release requires a clean working tree, an exact signed
`vVERSION` Git tag, both Linux targets, embedded revision metadata, checksums,
and build information:

```sh
git tag -s v0.3.0 -m "soju-tui v0.3.0"
./scripts/build.sh --target all --version 0.3.0 --release
```

Choose the release number deliberately; routine code changes do not bump it.
The helper refuses `--release` from an unsigned, untagged, dirty, unknown, or
development revision. `--allow-unsigned-tag` exists only for private test
builds and should not be used for production. `SHA256SUMS` protects artifact
integrity and `BUILDINFO` binds the artifacts to their source revision and Go
toolchain. Distribute them together
through the trusted Gitea release or GitHub mirror artifact for that exact tag.

## Make targets

```sh
make linux-amd64
make linux-arm64
make test
make vet
make release VERSION=0.3.0
```

## Verification

```sh
go test ./...
go test -race ./...
go vet ./...
sh -n scripts/build.sh scripts/grant-admin-access.sh scripts/setup.sh
./scripts/check-coverage.sh
./scripts/test-soju-compat.sh
```

The GitHub Actions workflows on the GitHub mirror repeat unit, race, vet,
coverage, static-analysis, security, vulnerability, shell, cross-build, and
real-Soju compatibility gates. The Gitea repository intentionally has no Gitea
Actions workflow. Because Gitea also discovers `.github/workflows/` by default,
repository Actions must remain disabled in Gitea; each job has a GitHub-host
guard as a second layer.

The compatibility job starts isolated temporary v0.9.0 and v0.10.1 instances;
it never touches the host's Soju service or data.

See [GitHub mirror and checks](github-mirror.md) before configuring a push
mirror or branch ruleset. A Gitea push mirror force-pushes the GitHub copy, so
the two hosts must not both be treated as writable sources of truth.

Linux release artifacts use `CGO_ENABLED=0` and are statically linked. They do
not need a separate Go runtime, C runtime, or ncurses library. They still invoke
the host's `sojuctl` executable to administer the running Soju instance. Confirm
the downloaded architecture with `file dist/soju-tui-linux-*` when diagnosing
an execution-format error.
