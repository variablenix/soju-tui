# soju-tui

`soju-tui` is a terminal frontend for [soju](https://soju.im/): it provides the IRC conversation UI and an administrative UI for users, networks, channels, upstream SASL/certificates, server status, notices, and debug logging.

The administrative UI uses the authenticated `BouncerServ` interface over the existing IRC/TLS session. It does not invoke a shell, interpolate user input into shell syntax, or put passwords in a `sojuctl` process argument list. Read-only operations run immediately; every mutating operation stops at a confirmation screen showing the exact redacted operation before it is sent.

It is written in Go and builds as one Linux binary. The terminal layer uses [tcell](https://github.com/gdamore/tcell), so the executable does not link to `libncurses`; a normal terminal and its terminfo entry are sufficient. On a minimal Debian installation, `ncurses-base` can be installed if the terminal reports an unknown `TERM`, but it is not a runtime library dependency of this program.

## Build

The source requires Go 1.23 or newer to build. On a Debian VPS with Go already installed:

Use the helper script after pulling source updates:

```sh
./scripts/build.sh --target linux-amd64 --version 0.1.0
```

To fast-forward the checkout and then verify/build it:

```sh
./scripts/build.sh --pull --target linux-amd64
```

The script runs `go test ./...`, `go vet ./...`, downloads modules, and writes binaries under `dist/`. It never performs a non-fast-forward Git update.

The Makefile remains available:

```sh
make build
```

Build outputs are written under `dist/`, so a host-native binary cannot be
mistaken for the Linux deployment binary.

For a build machine targeting a typical VPS:

```sh
make linux-amd64
```

The resulting binary is `dist/soju-tui-linux-amd64`.

For ARM64 VPS hardware:

```sh
make linux-arm64
```

The resulting binary is `dist/soju-tui-linux-arm64`.

Copy the resulting binary to the server and make it executable. The binary is built with `CGO_ENABLED=0`, so it does not need Go, ncurses, or other build tools after it has been copied.

## Connect

On first run, `soju-tui` checks `/etc/soju/config` for the first non-admin IRC
listener. It ignores `unix+admin://`, asks whether to use the detected
listener, and saves the selected server, port, TLS hostname, and username in
the per-user profile. The password is never saved.

For the Linux amd64 binary built on the VPS:

```sh
./dist/soju-tui-linux-amd64
```

The profile is stored at `~/.config/soju-tui/config.json` on Linux with mode
`0600`. Use `-setup` to run the wizard again, `-soju-config PATH` to inspect a
different daemon configuration, or `-profile PATH` to use a different profile.

The program prompts for the password without echoing it. A remote TLS listener
can still be specified directly:

```sh
./dist/soju-tui-linux-amd64 -server irc.example.net:6697 -username alice
```

If soju listens on a local unencrypted port, explicitly disable TLS:

```sh
./dist/soju-tui-linux-amd64 -server 127.0.0.1:6667 -tls=false -username alice
```

If the listener is bound to a specific local address, such as
`172.32.0.1:6697`, the setup wizard reads that address from `/etc/soju/config`.
You can also provide it explicitly with `-server 172.32.0.1:6697`.

The password can also be supplied through `SOJU_PASSWORD`, but an interactive prompt is safer than putting it in shell history or the process command line. Useful environment variables are `SOJU_SERVER`, `SOJU_USERNAME`, `SOJU_NICK`, `SOJU_REALNAME`, `SOJU_CLIENT`, `SOJU_NETWORK`, and `SOJU_TLS`.

For a self-signed soju certificate, prefer installing/trusting the certificate. As a temporary alternative:

```sh
./dist/soju-tui-linux-amd64 -insecure-skip-verify
```

That option disables certificate verification and should not be used on an untrusted network.

## Administration

Log in with an administrator account without a `/network` suffix. Press `F2` to switch between chat and administration. The admin dashboard exposes:

- all-user status and account creation, update, and deletion;
- per-user network status, creation, update, deletion, and raw upstream quote;
- per-user channel status, creation, update, and deletion;
- upstream SASL PLAIN, certificate generation, and certificate fingerprints;
- server status, broadcast notices, debug logging, and BouncerServ help.

Use `↑`/`↓` and `Enter` to select a dashboard action, or the displayed shortcuts. Forms support `Tab`/`Enter` to advance, `Space` to cycle boolean/select fields, `Ctrl-S` to preview, and `Esc` to cancel. For a mutation, press `y` only after reviewing the redacted preview; `n` or `Esc` cancels it.

The admin view requires a global login because a username such as `alice/libera` is already bound to one network. The logged-in account must be a soju administrator for all-user operations; soju itself enforces those permissions.

Use TLS when the admin session crosses a host boundary. For a local plain listener, `-tls=false` is acceptable when the connection is confined to the VPS loopback interface.

This follows soju's documented BouncerServ service model: the service accepts shell-style quoting, and the same operations are what `sojuctl` sends through the optional `unix+admin://` socket. The TUI does not require an admin socket or a local `sojuctl` binary. See the [soju service documentation](https://manpages.ubuntu.com/manpages/stonking/man1/soju.1.html) and [sojuctl manual](https://manpages.debian.org/unstable/soju/sojuctl.1.en.html).

## soju setup

The TUI manages the running soju instance through BouncerServ. Create the first administrator with soju's normal installation tooling, then use the admin dashboard. The same operation can be performed by selecting “Create network for user” and filling in the form; no command typing is required.

```text
/msg BouncerServ network create -addr ircs://irc.libera.chat -name libera
```

The IRC chat view still supports the normal `/network list` command. Networks discovered through the soju extension appear in the sidebar automatically. The child sessions use `BOUNCER BIND`, so one TUI process can show multiple upstream networks.

If a client name is supplied, soju can keep per-client history separately:

```sh
./dist/soju-tui-linux-amd64 -client vps-tui -username alice
```

## Keys and commands

`Enter` sends chat input. `Tab` or `Ctrl-N` selects the next buffer; `Ctrl-P` selects the previous one; `PageUp` and `PageDown` scroll; `F2` opens the admin UI; `Ctrl-C` or `Ctrl-Q` exits. Up/down recall the chat input history.

Supported commands include:

```text
/join #channel [key]
/part [#channel] [reason]
/msg target text
/query nick
/notice target text
/me action text
/nick newnick
/topic [text]
/names [#channel]
/away [reason]       /back
/network [list|name]
/raw IRC COMMAND ...
/clear                /help       /quit
```

The bouncer sends backlog automatically when the session attaches. If the soju instance exposes `draft/chathistory` and message IDs, `PageUp` requests older history for the active channel or query.

## Current scope

The admin UI covers the documented running-instance BouncerServ operations. It does not edit `/etc/soju/config`, replace systemd, or change immutable process-level listeners/database/log directives; those are daemon deployment concerns and require the normal operating-system workflow. The chat UI does not yet implement file transfers, message editing/reactions, local logging, or automatic reconnect after a network outage.

## Verify

```sh
make test
make vet
```
