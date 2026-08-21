# Security model

## Administrative boundary

`soju-tui` is a local frontend for `sojuctl`. Authorization comes from the
current Unix account's write access to Soju's `unix+admin://` socket; no Soju
administrator username is hard-coded or stored.

The admin socket grants powerful control over all Soju users. Limit access to
trusted local administrators. The setup wizard applies a per-user ACL and never
makes the socket world-writable.

## Command execution

Every operation uses an argument vector with `exec.CommandContext`. User input
is not interpolated into shell syntax, and the program never invokes `sh -c`.
Inputs containing NUL, carriage return, or newline are rejected.

Read-only actions execute directly. Every mutation has a review screen;
destructive and high-risk changes require an exact typed phrase. Exiting also
requires confirmation, including while an operation is running.

## Secrets

The saved profile contains only config and executable paths and is created with
mode `0600`. Passwords are held only for the operation that needs them and are
redacted from previews and captured output.

`sojuctl` accepts some secrets as command arguments. They can therefore be
briefly visible to same-host process inspection while the process runs. Protect
the host, admin socket, and administrator account accordingly.

The host certificate viewer reads only the public certificate file. It reports
the configured private-key path for clarity but never opens that file.

## Scope

The TUI does not edit Soju's config, database, listeners, or service definition.
The explicit setup wizard is the only component that requests elevated access;
normal TUI operation never invokes `sudo`.
