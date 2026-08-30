# Debian and Ubuntu packages

GitHub releases provide native `amd64` and `arm64` Debian packages alongside
the standalone Linux executables. A package installs the interactive command at
`/usr/bin/soju-tui`, documentation under `/usr/share/doc/soju-tui`, manual pages
under `/usr/share/man`, and the optional guided access helper at
`/usr/sbin/soju-tui-setup`.

The optional Grafana dashboard, Prometheus scrape example, and observability
guidance are installed under `/usr/share/doc/soju-tui/grafana`. They configure
neither Soju nor Prometheus automatically.

The package also includes license and notice texts for the exact Go modules
embedded in the release binaries. The release workflow verifies that AMD64 and
ARM64 carry the same dependency versions before generating that notice file.

The package does not contain maintainer scripts or a soju-tui systemd service.
Installing, upgrading, or removing it does not edit Soju's configuration or
database, change users or networks, grant socket access, modify ACLs, or start a
background process. `systemctl` and `journalctl` remain optional read-only
runtime integrations.

## Install a release package

Select the package matching `uname -m`: `x86_64` uses `amd64`, while `aarch64`
uses `arm64`. For release `0.3.4`, download the package and the common checksum
manifest from the release page, then verify and install them:

```sh
sha256sum -c SHA256SUMS --ignore-missing
sudo apt install ./soju-tui_0.3.4-1_amd64.deb
```

Use `soju-tui_0.3.4-1_arm64.deb` on ARM64. The `-1` suffix is the Debian package
revision and is separate from the upstream `0.3.4` application version.

A standalone `.deb` downloaded from GitHub is not an APT repository. `apt`
tracks the installed files and supports clean upgrades and removal, but the
operator must download and install each new GitHub release package until an APT
repository is provided.

## Authorize a local administrator

The package cannot safely guess which local login should receive full Soju
administration. If that account does not already have access to the admin
socket, preview and run the explicit helper on a systemd host:

```sh
sudo soju-tui-setup --user "$(id -un)" --dry-run
sudo soju-tui-setup --user "$(id -un)"
sojuctl -config /etc/soju/config server status
soju-tui
```

The helper discovers the socket from `/etc/soju/config`, verifies the proposed
account and paths, and asks before granting a per-user ACL. Its systemd path
unit only reapplies that ACL when Soju recreates the socket; no unit runs the
interactive TUI. Use `--config`, `--socket`, or `--sojuctl` for non-default
locations.

On a host without systemd, grant the trusted account equivalent persistent
access through the host's service manager. The TUI itself does not require
systemd or ACL tools.

## Avoid shadowing the package

`/usr/local/bin` normally precedes `/usr/bin` in `PATH`. A previous manual
installation can therefore hide the package-managed command:

```sh
type -a soju-tui
/usr/bin/soju-tui -version
```

After verifying the package, move the old manual copy out of the command path:

```sh
sudo mv /usr/local/bin/soju-tui /usr/local/bin/soju-tui.manual-backup
hash -r
soju-tui -version
```

Do not run the repository's default `scripts/setup.sh` to update a package
installation because that path intentionally manages `/usr/local/bin`. Use a
new `.deb` for package upgrades and `soju-tui-setup` only for socket access.

## Upgrade and remove

Install a downloaded newer package with `apt`; Debian verifies the package
architecture and performs an in-place package upgrade:

```sh
sudo apt install ./soju-tui_NEWVERSION-1_amd64.deb
```

Remove all package-owned files with:

```sh
sudo apt remove soju-tui
```

Authorization created by an explicit `soju-tui-setup` run is host policy, not a
package-owned side effect, and is therefore not silently revoked on package
removal. Review and remove those ACL watcher units separately if the trusted
administrator should also lose Soju access.

## Build packages locally

Build both static Linux executables and then package them:

```sh
GOTOOLCHAIN=auto ./scripts/build.sh --target all --version 0.3.4
./scripts/build-deb.sh
./scripts/test-deb-packages.sh
```

The protected release workflow additionally runs install, upgrade, and removal
tests in clean Debian and Ubuntu containers before publishing either package.
