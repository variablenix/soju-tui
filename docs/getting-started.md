# Getting started

## Requirements

The project is written in Go, but the Go toolchain is a build dependency—not a
dependency of the prebuilt Linux executables. The TUI uses the pure-Go `tcell`
terminal library and does not use ncurses.

| Component | Running a prebuilt Linux executable | Building from source |
| --- | --- | --- |
| Running Soju instance | Required | Required for live administration only |
| Matching `sojuctl` executable | Required | Required for live administration only |
| Soju config and writable admin socket | Required | Required for live administration only |
| Usable terminal/terminfo | Required | Required when running the result |
| Go toolchain | Not required | Go 1.26.6 or newer required |
| ncurses development/runtime library | Not required | Not required |
| HTTPS access and `curl` | Required for the default release install | Required only when downloading dependencies or releases |
| systemd and ACL tools | Not required by the TUI | Used only by the guided Linux socket-access setup |

The Linux AMD64 and ARM64 artifacts produced by release builds use
`CGO_ENABLED=0` and are statically linked. Select the artifact matching the
host architecture.

## Production host TLS

For production, configure Soju with a certificate from a publicly trusted CA,
such as Let's Encrypt, whose DNS name matches the hostname used by clients.
Use Certbot or another ACME client outside `soju-tui`, and automate renewal plus
Soju reload with a deploy hook appropriate for the host.

`soju-tui` only inspects the public certificate from the config's `tls` line.
It never runs Certbot, replaces host TLS files, reads the private key, or reloads
Soju. See [Certificate safety](certificates.md) before generating any upstream
CertFP certificate.

## Guided Linux setup

From the repository, preview the setup first:

```sh
./scripts/setup.sh --dry-run
```

Apply it when the discovered user, config, and socket are correct:

```sh
./scripts/setup.sh
```

The wizard requests `sudo`, downloads the latest stable architecture-matched
release from GitHub, verifies its SHA-256 manifest, installs ACL tools only with
approval, grants the invoking account access to the admin socket, verifies
`sojuctl`, and installs a systemd path unit that reapplies the ACL when Soju
recreates the socket. It copies the verified binary to
`/usr/local/bin/soju-tui` and verifies that command as the regular user. It
never makes the socket world-writable.

The installed command is a root-owned copy rather than a symlink into the Git
checkout, so moving or deleting the checkout does not break it. Installation is
atomic. The wizard skips the copy when its content and security metadata are
already current, prompts immediately before replacing an existing regular
file, and refuses symbolic-link, non-file, or insecure install destinations.
It prints the selected build revision and verifies the selected and installed
files with SHA-256. A normal release install does not depend on checked-in
`dist/` binaries and therefore does not require an artifact-sync commit after
each release.

Production setup rejects builds whose version reports `dev`, `dirty`, or
`unknown`. For local testing only, explicitly opt in with
`--allow-development-build`. This prevents an accidental development artifact
from silently replacing the installed production command.

### Setup choices

Use the path that matches the host:

- Standard first installation: preview with `./scripts/setup.sh --dry-run`,
  apply with `./scripts/setup.sh`, then run `soju-tui`.
- Repeatable release installation: use `./scripts/setup.sh --release VERSION`
  to pin an exact GitHub release instead of following the latest stable one.
- Existing managed installation: pull changes and rerun the same setup wizard;
  it downloads and verifies the selected release, skips an already-current
  installed binary, and re-verifies socket access.
- Locally built binary: build the matching target, then run
  `./scripts/setup.sh --binary /absolute/path/to/soju-tui`. Add
  `--checksums /absolute/path/to/SHA256SUMS` when a manifest accompanies it.
- Repository-only operation: use `./scripts/setup.sh --no-install`; completion
  validates the selected release without installing a global command. Use
  `--binary` as well when you want to validate a local `dist/` artifact.
- Custom command location: use
  `./scripts/setup.sh --install-path /opt/local/bin/soju-tui` with a trusted,
  root-owned directory already included in the user's `PATH`.
- Profile rediscovery only: run `soju-tui -setup` as the regular user. This
  does not reinstall the command or change socket permissions.
- Host without systemd: follow [Other Unix-like systems](#other-unix-like-systems)
  and configure equivalent admin-socket access with the native service manager.

The setup path is tested on Debian. Package discovery also supports APT, DNF,
YUM, Zypper, and Pacman environments. Only the persistent admin-socket ACL
helper requires systemd, `setfacl`, `runuser`, and standard Unix utilities; the
TUI executable itself does not.

## Other Unix-like systems

The source is portable Go code. On systems without a matching prebuilt Linux
artifact, install Go 1.26.6 or newer and build a native executable on the target
system with:

```sh
./scripts/build.sh --target host
```

If the host does not use systemd, configure equivalent persistent access to the
Soju admin socket with the platform's service manager. Keep access limited to a
trusted administrator account or group; do not use mode `0666`.

Verify access before starting the TUI:

```sh
sojuctl -config /etc/soju/config server status
```

## First launch

Run the installed command as the authorized regular user:

```sh
soju-tui
```

The first launch displays the discovered Soju hostname/title, config, admin
socket, TLS certificate, and `sojuctl` path. After confirmation, only the
non-secret paths are stored in:

```text
~/.config/soju-tui/admin.json
```

The profile is mode `0600`.

Useful overrides:

```sh
soju-tui -config /etc/soju/config
soju-tui -sojuctl /usr/bin/sojuctl
soju-tui -profile ~/.config/soju-tui/admin.json
soju-tui -timeout 60s
soju-tui -setup
```

## Updating or rerunning setup

For a normal production update, pull the setup script and rerun the same wizard.
The script obtains the release binary from GitHub, so no binary-sync PR is
needed:

```sh
git pull --ff-only
./scripts/setup.sh --dry-run
./scripts/setup.sh
```

The dry run changes nothing. The apply run updates `/usr/local/bin/soju-tui`
only when its contents, owner, mode, or link count differ and re-verifies the
existing socket-access setup.
If you changed source locally, build the target first and explicitly select the
local binary:

```sh
GOTOOLCHAIN=auto ./scripts/build.sh --target linux-amd64
./scripts/setup.sh --binary "$PWD/dist/soju-tui-linux-amd64" \
  --checksums "$PWD/dist/SHA256SUMS" --allow-development-build
```

Use `linux-arm64` instead on an ARM64 host. A normal setup rerun does not alter
Soju users, networks, channels, database contents, configuration, or TLS files.
