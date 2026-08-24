# Soju version compatibility

## Supported releases

The administration contract is verified against these exact releases:

| Soju release | Packaging checked | Status |
| --- | --- | --- |
| v0.9.0 | Upstream tag and Debian `0.9.0-1` | Supported |
| v0.10.1 | Upstream tag and Debian `0.10.1-1` | Supported |

Soju v0.9.0 and v0.10.1 expose the same BouncerServ command names, usages,
flags, and status output used by Soju-TUI. The v0.10.1 `sojuctl` change in this
area fixes Go error construction; it does not change command syntax. Debian's
packages do not patch `service.go` or `cmd/sojuctl` in either checked release.

The compatibility suite contains regression fixtures for both command sets and
also builds and starts each upstream release in an isolated temporary directory.
It exercises the users (including password replacement), networks, channels,
CertFP, SASL, deletion-confirmation, and status contracts used by the TUI. The
script verifies each tag's pinned upstream commit before building it.

## Runtime capability detection

Soju does not expose a server version through `sojuctl`. Soju-TUI therefore
does not guess behavior from a package-version string. At startup it runs the
equivalent of:

```sh
sojuctl -config /etc/soju/config help
sojuctl -config /etc/soju/config user run USER help
```

Only commands reported by that running server are shown. Discovery fails
closed: if help cannot be read, Soju-TUI does not advertise unverified remote
operations. Local host-TLS inspection and built-in help remain available.

For example, `device-certificate` is newer than v0.10.1. Its actions are
automatically absent on v0.9.0 and v0.10.1 instead of being displayed and then
failing.

## Unix-socket upstream networks

Soju v0.9.0 and v0.10.1 accept `unix:///path` in the BouncerServ network
validator, while newer development source uses `irc+unix:///path`. The TUI
accepts the clearer `irc+unix:///path` form in its network form and first sends
the `unix:///path` spelling required by both supported stable releases. If a
newer server explicitly rejects `unix` while advertising `irc+unix`, the TUI
retries only that equivalent address spelling. It does not retry permission,
dial, or unrelated validation failures.

This concerns an IRC **upstream network address**. It is unrelated to Soju's
`listen unix+admin://` administration socket.

## Operator verification

After installing or upgrading Soju, verify the active server before changing
state:

```sh
sojuctl -config /etc/soju/config server status
sojuctl -config /etc/soju/config help
sojuctl -config /etc/soju/config user status
```

Then launch `soju-tui`. A missing action means the running server did not
report that command; see [Troubleshooting](troubleshooting.md#a-menu-action-is-missing).

Compatibility with a release means the TUI's command grammar and parsers are
covered and pass the project gates. It cannot guarantee behavior of locally
patched Soju builds, third-party database drivers, host permission policy, or
unavailable external IRC networks.
