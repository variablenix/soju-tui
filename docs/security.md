# Security model

## Administrative boundary

`soju-tui` is a local frontend for `sojuctl`. Authorization comes from the
current Unix account's write access to Soju's `unix+admin://` socket; no Soju
administrator username is hard-coded or stored.

The admin socket grants powerful control over all Soju users. Limit access to
trusted local administrators. The setup wizard applies a per-user ACL and never
makes the socket world-writable.

The setup wizard installs a root-owned mode-`0755` copy in
`/usr/local/bin/soju-tui`; it does not create a link into a user-writable Git
checkout. It rejects symbolic links and install directories that are not
root-owned or that are writable by a group or other users. A multiple-link file
is never trusted as current. Updates use a temporary file in the destination
directory followed by an atomic rename, and replacing an existing file requires
confirmation.

Production setup rejects development, dirty, and unknown builds. The normal
installation downloads the architecture-matched release and its
`SHA256SUMS` manifest over HTTPS, verifies the downloaded binary before use,
and compares the installed copy by SHA-256 after atomic replacement. An
explicit `--binary` installation can be verified with `--checksums`, including
the manifest generated in `dist/`. Checksums detect corruption; release
authenticity also depends on trusted GitHub transport, branch protection,
workflow integrity, and release access controls. The manually dispatched
release workflow receives repository contents write permission only in its
publishing job and uses the short-lived GitHub Actions token. It does not
require a stored token or private GPG key. Local release builds continue to
require a signed tag.

CI exercises the release installer on Linux with a real unprivileged account.
Downloaded public release artifacts use a root-owned, traversable temporary
directory so the selected administrator can execute the candidate for version
verification. A single cleanup handler removes release and atomic-install
temporary files on success, failure, or interruption. Setup always creates its
release workspace under `/tmp` rather than trusting an inherited `TMPDIR`.

## Command execution

Every operation uses an argument vector with `exec.CommandContext`. User input
is not interpolated into shell syntax, and the program never invokes `sh -c`.
Inputs containing NUL, carriage return, or newline are rejected.

Built-in Soju-TUI help is compiled static text. Documentation URLs are shown
for copying only; the help action does not launch a browser, make a network
request, invoke `sojuctl`, or execute a shell.

Vim and WASD letter aliases are translated only after form and confirmation
input has been handled. Those letters therefore remain literal in usernames,
passwords, commands, notices, and exact confirmation phrases.

Read-only actions execute directly. Every mutation has a review screen;
destructive and high-risk changes require an exact typed phrase. Exiting also
requires confirmation, including while an operation is running.

Changing a network from SASL EXTERNAL/CertFP to SASL PLAIN requires the exact
phrase `SET SASL PLAIN`. Resetting and erasing all saved SASL credentials uses
the separate `RESET SASL` phrase.

Privilege grants, server TLS pin changes, connect commands, client-device trust,
server-wide notices, limit overrides, and explicit clears also use typed
phrases. Lower-risk creates and edits still require a `y` confirmation after
the redacted argument preview.

Confirmation color communicates the operation's effect, not whether approval
can be skipped: green adds an object or credential, blue changes state, and red
deletes, clears, resets, or replaces existing state. Every color still requires
explicit approval, and high-risk additions such as administrator or client
certificate registration retain their exact typed phrase.

## Secrets

The saved profile contains only config and executable paths and is created with
mode `0600`. Passwords are held only for the operation that needs them and are
redacted from previews and captured output.

Network connect commands and raw network commands are redacted in their
entirety because operators commonly use them for NickServ credentials. They
remain single argument-vector values and require high-risk confirmation before
execution.

`sojuctl` accepts some secrets as command arguments. They can therefore be
briefly visible to same-host process inspection while the process runs. Protect
the host, admin socket, and administrator account accordingly.

The host certificate viewer reads only an absolute public-certificate path. It
reports the configured private-key path for clarity but never opens that file.

Upstream CertFP generation is a separate per-user, per-network operation. The
TUI performs a read-only fingerprint preflight first. Existing fingerprints are
shown before replacement. The state is checked again after typed confirmation;
a concurrent change or any unexpected preflight error blocks generation instead
of guessing that no certificate exists.

## Scope

The TUI does not edit Soju's config, database, listeners, or service definition.
The explicit system setup wizard is the only component that requests elevated
access; normal TUI operation and `soju-tui -setup` never invoke `sudo`.
