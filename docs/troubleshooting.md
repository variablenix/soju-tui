# Troubleshooting

## Server or network age is not shown

Timing is optional and appears only when Soju runs as the configured systemd
unit and the current account can read the required evidence. Check the same
sources without elevation:

```sh
systemctl show -p ActiveState -p ExecMainStartTimestamp soju.service
journalctl -u soju.service --grep='connection registered with nick' -n 1 --no-pager
```

If Soju uses another unit, start the TUI with
`-soju-systemd-unit NAME.service` or set `SOJU_SYSTEMD_UNIT`. A network event
may also have rotated out of the journal. In every failure case, the TUI keeps
the underlying `sojuctl` output rather than displaying an inaccurate age.

The TUI intentionally does not invoke `sudo` and setup does not grant broader
journal-reader membership. Review the host-wide visibility implications before
changing journal permissions. Passing an empty unit value disables the probes.

## Admin socket permission denied

Verify the failure outside the TUI:

```sh
sojuctl -config /etc/soju/config server status
```

Preview and rerun the guided setup:

```sh
./scripts/setup.sh --dry-run
./scripts/setup.sh
```

With systemd, the persistent watcher should remain active even though its
oneshot service becomes inactive after success:

```sh
systemctl status soju-tui-admin-access.path
```

## Connection refused

Confirm Soju is running and the config contains an admin listener. The TUI uses
the Unix admin socket, not the public IRC TLS listener or port 6697.

```text
listen unix+admin://
```

## Exec format error

Use the binary matching the server architecture:

```sh
uname -m
file dist/soju-tui-linux-amd64 dist/soju-tui-linux-arm64
```

`x86_64` uses the AMD64 artifact; `aarch64` uses ARM64. A binary built with
`--target host` on macOS cannot run on Linux.

Installing Go or ncurses will not fix an execution-format error. The prebuilt
Linux executables do not require either at runtime; use the executable matching
the server's operating system and CPU architecture.

## `soju-tui` command not found

The guided setup normally installs `/usr/local/bin/soju-tui`. Verify the file
and your command search path:

```sh
ls -l /usr/local/bin/soju-tui
type -a soju-tui
/usr/local/bin/soju-tui -version
dist/soju-tui-linux-amd64 -version
```

If it is missing or stale, return to the repository and safely preview and
rerun setup. The normal rerun downloads the latest stable release; it does not
depend on a checked-in artifact-sync update:

```sh
./scripts/setup.sh --dry-run
./scripts/setup.sh
```

The apply run asks for replacement immediately before the atomic copy. A
successful run prints the selected build revision and matching source and
installed SHA-256 values. Verify the installed command directly if needed:

```sh
soju-tui -version
sha256sum /usr/local/bin/soju-tui
```

For an explicit local install, use the matching `dist/` artifact and manifest:

```sh
./scripts/setup.sh --binary "$PWD/dist/soju-tui-linux-amd64" \
  --checksums "$PWD/dist/SHA256SUMS"
```

Use the ARM64 artifact instead on an ARM64 host.

If the installed version looks correct but typing `soju-tui` behaves
differently, `type -a soju-tui` will reveal an alias, shell function, or earlier
executable in `PATH`. Start a fresh shell or run `hash -r` after correcting
that command-resolution issue.

If `/usr/local/bin` is not in the login shell's `PATH`, run the absolute path or
choose a trusted directory already in `PATH` with `--install-path`.

## A menu action is missing

The TUI intentionally omits commands not reported by the running Soju server.
Compare with:

```sh
sojuctl -config /etc/soju/config help
sojuctl -config /etc/soju/config user run USERNAME help
```

Upgrade Soju when a required administration command is unavailable.

## Older output or fingerprints are not visible

The main output pane follows the newest lines by default. Press `Page Up` to
scroll toward older output and `Page Down` to return toward the newest output.
The footer identifies the current direction while scrolled. The menu's
`↑`/`↓` keys still move the selected action; they do not scroll the output.

## Host certificate cannot be read

**View Soju host TLS certificate** reads the public certificate path from the
active config's `tls` line. Confirm the path exists and is readable by the TUI
user. The private key does not need to be readable.

Use absolute certificate and key paths in the `tls` directive. Relative paths
depend on Soju's service working directory, which can differ from the TUI's
directory. The viewer refuses such an ambiguous path instead of risking display
of an unrelated certificate.

## Setup rejects a development build

Production setup rejects a build reporting `dev`, `dirty`, or `unknown`. Build
from a clean exact release tag as described in [Building and releases](building.md).
For disposable testing only, rerun setup with `--allow-development-build`.

## Git pull reports local changes

Inspect before changing anything:

```sh
git status
git diff
```

Commit work you want to keep or temporarily stash it, then use
`git pull --ff-only`. Avoid destructive resets unless the local changes are
known to be disposable.
