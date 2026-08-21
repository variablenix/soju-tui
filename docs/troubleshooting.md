# Troubleshooting

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

## `soju-tui` command not found

The guided setup normally installs `/usr/local/bin/soju-tui`. Verify the file
and your command search path:

```sh
ls -l /usr/local/bin/soju-tui
command -v soju-tui
/usr/local/bin/soju-tui -version
```

If it is missing or stale, return to the repository and safely preview and
rerun setup:

```sh
./scripts/setup.sh --dry-run
./scripts/setup.sh
```

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

## Host certificate cannot be read

**View Soju host TLS certificate** reads the public certificate path from the
active config's `tls` line. Confirm the path exists and is readable by the TUI
user. The private key does not need to be readable.

## Git pull reports local changes

Inspect before changing anything:

```sh
git status
git diff
```

Commit work you want to keep or temporarily stash it, then use
`git pull --ff-only`. Avoid destructive resets unless the local changes are
known to be disposable.
