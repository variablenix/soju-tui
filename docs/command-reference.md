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

On Linux/systemd hosts, Soju-TUI may append process and upstream connection
ages to the first and fourth outputs. These additions come from read-only
service/journal evidence and are display-only; the underlying `sojuctl`
commands and their compatibility contract are unchanged.

## Users

Soju uses the same administrative command to change the selected account's own
password or reset another user's password:

```sh
sojuctl -config /etc/soju/config user update USER -password NEW_PASSWORD
```

The distinction is a Soju-TUI safety boundary: **Change my password** prefers a
matching local Unix username and requires ordinary change approval, while
**Reset user password** requires the exact phrase
`RESET USER PASSWORD USERNAME`. In both cases the password is entered twice,
redacted from the preview and captured output, and never saved in the profile.
Administrative-socket access authorizes the replacement; the old password is
not supplied to `sojuctl`.

## Networks

```sh
sojuctl -config /etc/soju/config user run USER network create \
  -addr ircs://irc.example.net:6697 -name example -nick ExampleNick

sojuctl -config /etc/soju/config user run USER network update example \
  -enabled false

# Disconnect and reconnect without changing saved settings
sojuctl -config /etc/soju/config user run USER network update example

sojuctl -config /etc/soju/config user run USER network delete example
```

The Soju-TUI **Reconnect upstream network** action discovers the user's saved
networks and runs the no-options `network update NETWORK` form above. Soju
disconnects and reconnects that upstream connection, applying newly generated
CertFP certificates or changed SASL credentials without rewriting any network
settings.

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

For Soju v0.9.0 and v0.10.1, `sasl status` considers an upstream account
authenticated only after Soju records numeric 900 (`RPL_LOGGEDIN`). Some IRC
servers establish or later recognize an account without sending numeric 900,
including some CertFP flows. When Soju returns `Unauthenticated on upstream
network`, the TUI therefore presents the state as **Upstream account not
reported to Soju** and explains that the result does not by itself prove
authentication failure. Confirm the effective account with NickServ or WHOIS
and inspect the Soju log for an explicit SASL failure.

The `-network` value must be an actual saved network name or address. `*` does
not mean all networks. The TUI's **All networks** CertFP and SASL status views
discover the real targets and invoke the read-only command separately for each
one; selecting a specific network still runs only that network's status check.

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
