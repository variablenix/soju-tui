# Soju administration command reference

`soju-tui` builds these argument sequences directly and does not invoke a
shell. The examples below document the upstream Soju command grammar used by
the TUI. After the system setup, run the TUI as the authorized regular account;
routine administration does not need `sudo`.

Use the active config with direct `sojuctl` checks:

```sh
sojuctl -config /etc/soju/config server status
sojuctl -config /etc/soju/config user status
sojuctl -config /etc/soju/config user status USER
sojuctl -config /etc/soju/config user run USER network status
```

## Networks

```sh
sojuctl -config /etc/soju/config user run USER network create \
  -addr ircs://irc.example.net:6697 -name example -nick ExampleNick

sojuctl -config /etc/soju/config user run USER network update example \
  -enabled false

sojuctl -config /etc/soju/config user run USER network delete example
```

Valid connection schemes are `ircs://`, `irc+insecure://`, and
`irc+unix:///path` in the TUI. Soju v0.9.0 and v0.10.1 require the equivalent
`unix:///path` spelling in their service-command validator, so the TUI performs
that stable-release normalization before calling `sojuctl`. If a newer server
explicitly rejects `unix` and advertises `irc+unix`, the TUI retries only that
equivalent spelling. The
network `-certfp` option pins the upstream IRC **server's TLS certificate** and
current handlers accept a SHA-256 or SHA-512 fingerprint. It is unrelated to
the SASL CertFP identity certificate below.

Soju accepts up to 20 repeated `-connect-command` options. The TUI provides one
simple command field plus an optional JSON array for additional commands, and
an explicit clear selector for saved values that `network status` cannot
disclose. Empty-value clears require a typed confirmation.

## Upstream SASL CertFP

CertFP is scoped to one Soju user and one saved network:

```sh
sojuctl -config /etc/soju/config user run USER \
  certfp generate -network NETWORK
sojuctl -config /etc/soju/config user run USER \
  certfp fingerprint -network NETWORK
sojuctl -config /etc/soju/config user run USER \
  sasl status -network NETWORK
```

`certfp generate` creates the key and certificate and selects SASL EXTERNAL;
there is no `sasl set-external` command. Repeating generation replaces the
saved network certificate. The TUI checks first, displays an existing
fingerprint, and requires a different exact phrase for creation versus
replacement.

The `-network` value must be an actual saved network name or address. `*` does
not mean all networks. The TUI's **All networks** fingerprint view discovers
the real targets and invokes the read-only command separately for each one.

## Corrections to common invalid forms

| Invalid or incomplete | Correct form |
| --- | --- |
| `sojuctl status` | `sojuctl -config /etc/soju/config server status` |
| `sojuctl -config server status` | Supply the config path after `-config` |
| `sojuctl -config /etc/soju/config status` | Add the `server` command group |
| `user run USER sasl set-external ...` | Use `certfp generate -network NETWORK` |
| `user run USER network` | Add `status`, `create`, `update`, `delete`, or `quote` |
| `user run USER sasl status -network '*'` | Run once per real network name |
| `user run <SOJU_USER> ...` | Replace the placeholder before execution |

Deleting the same network twice is not idempotent: the first successful delete
removes it and the second correctly reports an unknown network. Likewise,
creating a duplicate network, channel, user, or device-certificate fingerprint
is expected to fail instead of overwriting the existing object.

## Version compatibility

Available commands depend on the running Soju version. Soju-TUI explicitly
supports v0.9.0 and v0.10.1; their administration command grammar is the same.
Inspect the active server with:

```sh
sojuctl -config /etc/soju/config help
sojuctl -config /etc/soju/config user run USER help
```

The TUI performs the same capability discovery and omits unsupported menu
actions. Command definitions and Debian packaging were audited at both tagged
releases; the running server remains authoritative. See
[Soju version compatibility](compatibility.md) for the detailed matrix and
boundary.
