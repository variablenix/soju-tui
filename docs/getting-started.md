# Getting started

## Requirements

- A running Soju instance and matching `sojuctl` executable.
- A Soju config containing `listen unix+admin://` or an explicit admin socket.
- A trusted local account allowed to read the config and write the admin socket.
- A normal terminal with a usable terminfo entry.

The Linux release binaries are static and do not require Go or ncurses at
runtime.

## Production host TLS

For production, configure Soju with a certificate from a publicly trusted CA,
such as Let's Encrypt, whose DNS name matches the hostname used by clients.
Use Certbot or another ACME client outside `soju-tui`, and automate renewal plus
Soju reload with a deploy hook appropriate for the host.

`soju-tui` only inspects the public certificate from the config's `tls` line.
It never runs Certbot, replaces host TLS files, reads the private key, or reloads
Soju. See [Certificate safety](certificates.md) before generating any upstream
CertFP certificate.

## Guided Linux setup

From the repository, preview the setup first:

```sh
./scripts/setup.sh --dry-run
```

Apply it when the discovered user, config, socket, and executable are correct:

```sh
./scripts/setup.sh
```

The wizard requests `sudo`, installs ACL tools only with approval, grants the
invoking account access to the admin socket, verifies `sojuctl`, and installs a
systemd path unit that reapplies the ACL when Soju recreates the socket. It
never makes the socket world-writable.

The setup path is tested on Debian. Package discovery also supports APT, DNF,
YUM, Zypper, and Pacman environments. The persistent ACL helper requires
systemd, `setfacl`, `runuser`, and standard Unix utilities.

## Other Unix-like systems

The TUI itself is portable Go code. Build it on the target system with:

```sh
./scripts/build.sh --target host
```

If the host does not use systemd, configure equivalent persistent access to the
Soju admin socket with the platform's service manager. Keep access limited to a
trusted administrator account or group; do not use mode `0666`.

Verify access before starting the TUI:

```sh
sojuctl -config /etc/soju/config server status
```

## First launch

Run the matching binary as the authorized regular user:

```sh
./dist/soju-tui-linux-amd64
```

The first launch displays the discovered Soju hostname/title, config, admin
socket, TLS certificate, and `sojuctl` path. After confirmation, only the
non-secret paths are stored in:

```text
~/.config/soju-tui/admin.json
```

The profile is mode `0600`. Re-run discovery at any time with `-setup`.

Useful overrides:

```sh
./dist/soju-tui-linux-amd64 -config /etc/soju/config
./dist/soju-tui-linux-amd64 -sojuctl /usr/bin/sojuctl
./dist/soju-tui-linux-amd64 -profile ~/.config/soju-tui/admin.json
./dist/soju-tui-linux-amd64 -timeout 60s
./dist/soju-tui-linux-amd64 -setup
```
