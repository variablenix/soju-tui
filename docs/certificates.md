# Certificate safety

Soju uses three certificate systems with different owners and purposes. They
must not be treated as interchangeable.

| Certificate | Purpose | Managed by | `soju-tui` behavior |
| --- | --- | --- | --- |
| Soju host TLS | Encrypts client connections to the Soju hostname | Certbot or another ACME/operator workflow | Read-only inspection |
| Upstream SASL CertFP | Authenticates one Soju user/network to upstream IRC | Soju through `certfp` | Preflight plus typed create/replace confirmation |
| Downstream device certificate | Authenticates an IRC client device to Soju | Soju through `device-certificate` | Explicit registration/deletion confirmations |

## Production Soju host TLS

Production clients should connect to a hostname covered by a certificate from a
trusted CA such as Let's Encrypt. Configure the resulting public chain and
private key in Soju, for example:

```text
hostname soju.example.net
tls /etc/soju/tls/fullchain.pem /etc/soju/tls/privkey.pem
```

Use Certbot or another ACME client to obtain and renew that certificate. A
deployment hook can copy renewed files into a Soju-owned directory with strict
permissions and reload Soju only after successful renewal. Follow the ACME
client and distribution documentation for the exact service account, paths,
ownership, and reload command.

Useful references:

- [Certbot instructions](https://certbot.eff.org/instructions)
- [Certbot DNS plugins and renewal hooks](https://eff-certbot.readthedocs.io/en/stable/using.html#dns-plugins)

The TUI's **View Soju host TLS certificate** action reads the public certificate
configured above and reports its hostname match, issuer, names, validity,
fingerprint, and chain length. It does not:

- invoke Certbot or any ACME client;
- write, copy, renew, or delete host certificate files;
- read the private key;
- reload or restart Soju.

This makes it safe to use alongside an automated Let's Encrypt renewal hook.

## Upstream SASL CertFP

**Generate upstream IRC CertFP (self-signed)** creates a certificate stored by
Soju for one user and one upstream IRC network. It does not use the hostname in
the Soju config and never touches the host TLS files.

Generation follows a fail-closed workflow:

1. Run `certfp fingerprint` for the selected user and network.
2. If a certificate exists, display its fingerprints and require the exact
   phrase `REPLACE EXISTING UPSTREAM CERTIFICATE`.
3. If Soju explicitly reports `CertFP not set up`, require the exact phrase
   `GENERATE UPSTREAM CERTIFICATE`.
4. For permission, connection, parsing, or unexpected command errors, stop
   without running `certfp generate`.

Replacing an existing CertFP changes upstream SASL EXTERNAL credentials. The
IRC account may need the new fingerprint registered with NickServ, and the
network may reconnect. Keep the previous fingerprint and a database backup
until the new authentication is verified.

## Downstream device certificates

Device certificates authenticate IRC clients to Soju; they do not encrypt the
host connection and are unrelated to CertFP. Registration requires
`client-cert-auth true` and a Soju version exposing `device-certificate`.
Unsupported actions are omitted from the menu.
