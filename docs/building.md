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

## Versions and automated releases

Ordinary commits and local rebuilds do not require a version bump. Development
builds remain explicitly marked and are not accepted by production setup unless
the operator supplies `--allow-development-build`.

Routine commits do not create releases. When a version is ready, use the
protected GitHub workflow:

1. Merge the intended source to `main` and wait for its checks to pass.
2. Open **Actions > Release > Run workflow** on GitHub.
3. Select the `main` branch, enter a version without the `v` prefix, such as
   `0.4.0`, and choose whether it is a prerelease.
4. Run the workflow.

The workflow validates the version and branch, repeats the test, security, and
real-Soju compatibility gates, builds both static Linux targets, verifies the
checksums, and creates the tag and GitHub release. The release is published by
`github-actions[bot]`, not a personal account. It uses GitHub's temporary,
repository-scoped token; no personal access token, repository secret, or GPG
key is required.

The version is intentionally chosen by a maintainer instead of being inferred
from every merge. Use semantic versioning: patch for compatible fixes, minor
for compatible features, and major for incompatible changes. A failed run does
not expose a partially uploaded public release, and an existing tag or release
cannot be overwritten.

## Optional local release build

A local production build requires a clean working tree and an exact signed
`vVERSION` Git tag:

```sh
git tag -s v0.3.0 -m "soju-tui v0.3.0"
./scripts/build.sh --target all --version 0.3.0 --release
```

This optional path requires a locally configured GPG key. The helper refuses
an unsigned, untagged, dirty, unknown, or development revision.
`--allow-unsigned-tag` remains limited to private test builds.
`--github-actions-release` is accepted only in the manually dispatched GitHub
release workflow and is not a general local bypass. `SHA256SUMS` protects
artifact integrity, while `BUILDINFO` binds artifacts to their source revision
and Go toolchain. Distribute all four release files together.

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

The GitHub Actions workflows repeat unit, race, vet,
coverage, static-analysis, security, vulnerability, shell, workflow-validation,
cross-build, and real-Soju compatibility gates. The release workflow repeats
these gates before publishing. A downstream Gitea mirror intentionally has no
Gitea Actions workflow. Because Gitea may discover `.github/workflows/`, keep
repository Actions disabled there; each job also has a GitHub-host guard.

The compatibility job starts isolated temporary v0.9.0 and v0.10.1 instances;
it never touches the host's Soju service or data.

See [GitHub repository and checks](github-mirror.md) before configuring a
downstream mirror or branch ruleset. GitHub is the writable source of truth;
do not configure a Gitea-to-GitHub push mirror.

Linux release artifacts use `CGO_ENABLED=0` and are statically linked. They do
not need a separate Go runtime, C runtime, or ncurses library. They still invoke
the host's `sojuctl` executable to administer the running Soju instance. Confirm
the downloaded architecture with `file dist/soju-tui-linux-*` when diagnosing
an execution-format error.
