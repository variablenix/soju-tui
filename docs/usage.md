# Using the TUI

## Menu and compatibility

At startup, the TUI asks BouncerServ for commands available globally and in a
user context. Only supported actions are shown. An older Soju release therefore
does not display operations it cannot perform.

Certificate-related menu entries use blue as an informational category color.
Red is reserved for actual errors and confirmation warnings.

## All users or a specific user

Use **List users** to display Soju's current user listing. Before every
user-targeted workflow, the TUI refreshes that listing and prefills the user
field:

- press `Space` to cycle through discovered users;
- use `Backspace` and type to target a specific username manually;
- the field shows its position in the discovered list or `custom`.

Manual entry remains available because Soju may limit very large user listings.
Bulk destructive changes are intentionally not provided: each mutation targets
one reviewed user. Server notices are already server-wide.

## Operations

The menu covers the administration commands reported by the running server:

- server status, notices, debug logging, and BouncerServ help;
- user listing, creation, updates, and deletion;
- per-user networks and channels;
- upstream SASL PLAIN and EXTERNAL/CertFP configuration;
- downstream client device certificates when supported by Soju;
- local inspection of the configured Soju host TLS certificate.

Read-only operations run immediately. Mutations first show the exact redacted
argument preview. Ordinary changes require `y`; destructive or high-risk
changes require the displayed phrase exactly.

## Network updates

**Update network for user** asks for a user and network, runs network status,
then fills the address, name, and enabled state exposed by Soju. Only changed
values are submitted.

Soju does not return saved upstream passwords, nicknames, usernames, real
names, CertFP values, or connect commands in network status. Those fields remain
blank to preserve the existing value, and saved passwords are never displayed.

## Certificate types

Soju uses three unrelated certificate concepts:

- **View Soju host TLS certificate** reads the public certificate from the
  config's `tls` line. It shows the configured domain (for example
  `soju.kode.im`), subject, issuer such as Let's Encrypt, DNS names, validity,
  hostname match, SHA-256 fingerprint, and chain length. The private key is
  never read.
- **Upstream SASL certificate / CertFP** authenticates one user's connection to
  an upstream IRC network. Before generation, the TUI checks for an existing
  CertFP and displays its fingerprints. Creating and replacing use different
  exact confirmation phrases; an inconclusive preflight blocks the mutation.
- **Client device certificates** authenticate downstream IRC clients to Soju.
  Registration also requires `client-cert-auth true`. These actions are omitted
  when the running Soju version lacks the command.

See [Certificate safety](certificates.md) for the complete mutation boundaries
and production Let's Encrypt guidance.

## Controls

- `↑`/`↓` — move through actions and wrap at either end
- `Home`/`End` — jump to the first or last action
- `Enter` — open an action or advance a form
- `Tab`/`Shift-Tab` — move between fields
- `Space` — cycle users, booleans, and choices
- `Ctrl-S` — preview or submit a form
- `y` — approve an ordinary mutation
- exact phrase + `Enter` — approve a destructive/high-risk mutation
- `n` or `Esc` — cancel a confirmation; `Esc` also cancels forms
- `r` — repeat the last read-only refresh
- `q`, `Q`, `Ctrl-C`, or `Ctrl-Q` — open exit confirmation
