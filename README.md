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
- read-only inspection of the TLS certificate configured for the Soju host;
- downstream client-device certificate registration when Soju enables it.

Every mutating operation stops at a confirmation screen showing the exact
redacted `sojuctl` command. Ordinary changes require `y`; destructive and
high-risk changes require an exact displayed phrase such as `RESET SASL`.
Read-only operations execute immediately and never require confirmation.

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

The first TUI run then reads `/etc/soju/config` and displays the discovered
local Linux user, Soju hostname/title, admin socket, server certificate,
configuration path, and `sojuctl` path. Nothing is saved until the user confirms
the review. Run with `-setup` to review and replace the local profile later.
`-accept-config` exists for deliberate non-interactive provisioning.

Passwords are not saved in the TUI profile. They are passed to `sojuctl` only
for the operation that needs them and are redacted from the TUI preview and
captured output. Because the upstream `sojuctl` interface accepts passwords as
command arguments, the password can be visible briefly to same-host process
inspection while that operation is running; use the socket permissions and
host access controls accordingly.

This follows soju's documented `sojuctl` and BouncerServ model:

- [sojuctl manual](https://soju.im/doc/sojuctl.1.html)
- [soju manual — configuration and IRC service](https://soju.im/doc/soju.1.html)

## Run

The normal invocation on a Debian VPS is:

```sh
./dist/soju-tui-linux-amd64
```

On first run, the TUI uses `/etc/soju/config` and locates `sojuctl` in
`PATH`. After confirmation, it saves only those non-secret paths to:

```text
~/.config/soju-tui/admin.json
```

The profile is created with mode `0600`. Future runs use the saved settings,
so there is no server, port, Soju account name, or IRC login command to type.
Administration is authorized by the current Linux user's access to the local
admin socket; no administrator username is hard-coded.

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

## Network updates and certificates

Selecting **Update network for user** first asks for the user and target
network, then queries `sojuctl network status` and opens the editor with the
network name, address, and enabled state filled in. Only changed fields are
submitted. Soju does not expose saved upstream passwords, nicknames, usernames,
real names, CertFP values, or connect commands through this status command;
those fields say **blank keeps current** and passwords are never displayed.

Soju uses three separate certificate concepts:

- **Server TLS certificate** is the certificate from the `tls` line in
  `/etc/soju/config`, such as a Let's Encrypt certificate for
  `soju.example.com`. The TUI shows its subject, issuer, SANs, validity, hostname
  match, and SHA-256 fingerprint. It never reads the private key.
- **Upstream SASL certificate / CertFP** authenticates a user's network
  connection to an upstream IRC server. Generating it can replace an existing
  key and therefore requires a typed confirmation.
- **Client device certificates** authenticate downstream IRC clients to Soju.
  Registration requires `client-cert-auth true` in the Soju configuration.
  They are unrelated to the certificate presented by the Soju host.

## Controls

- `↑`/`↓` — select an administration action;
- `Enter` — open an action or advance a form;
- `Tab`/`Shift-Tab` — move between form fields;
- `Space` — cycle booleans and select fields;
- `Ctrl-S` — preview/submit a form;
- `y` — approve an ordinary mutation on its confirmation screen;
- type the displayed phrase and press `Enter` — approve a destructive or
  high-risk mutation;
- `n` or `Esc` — cancel an ordinary confirmation; `Esc` cancels any form or
  typed confirmation;
- `r` — repeat the last read-only refresh;
- `q`, `Q`, `Ctrl-C`, or `Ctrl-Q` — open the exit confirmation;
- `y` — confirm exit, or `n`/`Esc` — remain in the TUI.

## Build

The source requires Go 1.26.6 or newer so release builds include the standard
library fix for GO-2026-5972. The helper runs tests and vet before building and
writes binaries under `dist/`.

For normal source updates on your x86_64 VPS (no version bump required):

```sh
./scripts/build.sh --target linux-amd64
```

To pull the latest fast-forwardable commit and build:

```sh
./scripts/build.sh --pull --target linux-amd64
```

Use `--version X.Y.Z` only when intentionally producing a numbered release.

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
