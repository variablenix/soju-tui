# soju-tui

`soju-tui` is an administration-only terminal frontend for the [soju IRC
bouncer](https://soju.im/). It is not an IRC client: it does not connect to
IRC, open chat buffers, display messages, or manage live conversations. Channel
status and configuration are exposed only as administrative data.

It provides a keyboard-driven interface for managing the running soju
instance through the local `sojuctl` command:

- server status, notices, debug logging, and BouncerServ help;
- user status, creation, updates, and deletion;
- per-user network status, creation, updates, deletion, and raw upstream commands;
- per-user channel status, creation, updates, and deletion;
- upstream certificate generation and fingerprints;
- upstream SASL status, PLAIN credentials, and reset.

Every mutating operation stops at a confirmation screen showing the exact
redacted `sojuctl` command. Press `y` to execute it, or `n`/`Esc` to cancel.
Read-only operations execute immediately.

## How it works

The TUI executes `sojuctl` with an argument vector. It never invokes a shell
and never interpolates form values into shell syntax. `sojuctl` sends the
administrative BouncerServ command through soju's `unix+admin://` socket.

`sojuctl` itself requires write permission on the soju admin socket. Run the
TUI as a user with that permission, or configure the service socket's group or
ACL for the administrator who will run the TUI. The program does not silently
invoke `sudo`.

Soju deliberately creates the admin socket with mode `0600`. On first use,
run the setup wizard from the repository:

```sh
./scripts/setup.sh
```

The wizard requests `sudo`, detects the invoking Linux user, reads the admin
socket from `/etc/soju/config`, offers to install the ACL utility if needed,
previews the persistent systemd configuration, asks for confirmation, and
verifies `sojuctl` as the local user. It applies a per-user ACL and installs a
small systemd path unit that reapplies the ACL when soju recreates the socket.
It never makes the socket world-writable. Use `./scripts/setup.sh --dry-run` to
preview the setup without changing the host.

The generated service is intentionally `Type=oneshot`: after a successful run,
`soju-tui-admin-access.service` is inactive while
`soju-tui-admin-access.path` remains active and watches the socket. Check the
persistent watcher with:

```sh
systemctl status soju-tui-admin-access.path
```

`scripts/grant-admin-access.sh` remains available as the lower-level helper for
custom automation.

Passwords are not saved in the TUI profile. They are passed to `sojuctl` only
for the operation that needs them and are redacted from the TUI preview and
captured output. Because the upstream `sojuctl` interface accepts passwords as
command arguments, the password can be visible briefly to same-host process
inspection while that operation is running; use the socket permissions and
host access controls accordingly.

This follows soju's documented `sojuctl` and BouncerServ model:

- [sojuctl manual](https://manpages.debian.org/unstable/soju/sojuctl.1.en.html)
- [soju manual — configuration and IRC service](https://manpages.ubuntu.com/manpages/stonking/man1/soju.1.html)

## Run

The normal invocation on a Debian VPS is:

```sh
./dist/soju-tui-linux-amd64
```

On first run, the TUI uses `/etc/soju/config` and locates `sojuctl` in
`PATH`. It saves those non-secret paths to:

```text
~/.config/soju-tui/admin.json
```

The profile is created with mode `0600`. Future runs use the saved settings,
so there is no server, port, or IRC login command to type.

Useful overrides:

```sh
./dist/soju-tui-linux-amd64 -config /etc/soju/config
./dist/soju-tui-linux-amd64 -sojuctl /usr/bin/sojuctl
./dist/soju-tui-linux-amd64 -profile ~/.config/soju-tui/admin.json
./dist/soju-tui-linux-amd64 -timeout 60s
./dist/soju-tui-linux-amd64 -setup
```

The soju daemon configuration should contain an admin listener, for example:

```text
listen unix+admin://
```

The default admin socket is `/run/soju/admin`. The TUI does not use the IRC
TLS listener such as `172.32.0.1:6697`; that listener is intentionally outside
this application's scope.

## Controls

- `↑`/`↓` — select an administration action;
- `Enter` — open an action or advance a form;
- `Tab`/`Shift-Tab` — move between form fields;
- `Space` — cycle booleans and select fields;
- `Ctrl-S` — preview/submit a form;
- `y` — approve a mutation on the confirmation screen;
- `n` or `Esc` — cancel or go back;
- `r` — repeat the last read-only refresh;
- `q`, `Q`, `Ctrl-C`, or `Ctrl-Q` — open the exit confirmation;
- `y` — confirm exit, or `n`/`Esc` — remain in the TUI.

## Build

The source requires Go 1.23 or newer. The helper runs tests and vet before
building and writes binaries under `dist/`.

For your x86_64 VPS:

```sh
./scripts/build.sh --target linux-amd64 --version 0.2.2
```

To pull the latest fast-forwardable commit and build:

```sh
./scripts/build.sh --pull --target linux-amd64 --version 0.2.2
```

The helper prints each phase and defaults to `GOTOOLCHAIN=local`, so an old Go
installation fails clearly instead of silently downloading another toolchain.
Use `GOTOOLCHAIN=auto` explicitly if automatic toolchain downloads are wanted.

Other targets:

```sh
./scripts/build.sh --target linux-arm64
./scripts/build.sh --target all
./scripts/build.sh --target host
```

The Linux binaries are statically linked with `CGO_ENABLED=0`; they do not
require Go or ncurses at runtime. A normal terminal and its terminfo entry are
sufficient.

The Makefile provides equivalent targets:

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

The TUI operates on the documented running-instance administration surface.
It does not edit `/etc/soju/config`, manage systemd, change listeners, or
modify soju's database files directly.
