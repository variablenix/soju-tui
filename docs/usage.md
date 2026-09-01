# Using the TUI

## Menu and compatibility

At startup, the TUI asks BouncerServ for commands available globally and in a
user context. Only supported actions are shown. An older Soju release therefore
does not display operations it cannot perform.

The shared administration surface of Soju v0.9.0 and v0.10.1 is explicitly
covered by regression fixtures. See [Soju version compatibility](compatibility.md)
for the tested package versions and limits.

Certificate-related menu entries use blue as an informational category color.
Confirmation dialogs use green for additions, blue for changes, and red only
for destructive actions such as deletion, clearing, resetting, or replacement.
Outside confirmations, red remains reserved for actual errors.

The sidebar identifies the program as **Managing Soju via sojuctl**. This
describes the active control path accurately without implying that the TUI opens
or monitors an IRC chat connection. When the output pane has unused room, it
displays a Soju-TUI wordmark and ASCII bottle. A compact mark is used on medium
terminals, and all decorative branding is omitted when command output or
terminal dimensions need the space.

## Built-in help

Press `?` or `F1`, or select **Soju-TUI help & documentation**, for a concise
guide to navigation, users, networks, channels, certificates, SASL, server
operations, and confirmation behavior. This local help is separate from
**BouncerServ command help**, which queries the command list exposed by the
running Soju version.

The help screen includes the upstream Soju project, manual, `sojuctl` manual,
and source URLs as plain text. It never launches a browser, invokes a shell, or
fetches remote content.

Inside help, use `↑`/`↓`, Vim `k`/`j`, or `w`/`s` for line scrolling;
`Page Up`/`Page Down` for larger steps; `Home`/`End` to jump; and `←`, `h`,
`a`, `Esc`, or `?` to close it. Scrolling stops at the first or last full page,
so the help pane does not advance into blank space.

On the main output screen, use `Page Up` to view older output and `Page Down`
to move toward the newest output. `Shift`+`↑`/`↓` provides an additional
scroll shortcut in terminals that preserve modified arrow keys. The footer
changes to `PgUp older`/`PgDn newer` while viewing history. The menu's ordinary
`↑`/`↓`, `k`/`j`, and `w`/`s` selection remains available at all times.

Vim `h`/`j`/`k`/`l` and WASD `a`/`s`/`w`/`d` are case-insensitive navigation
aliases on menus and the help viewer. On the main menu, left (`←`, `h`, or
`a`) goes back and right (`→`, `l`, or `d`) opens the selected action. These
letter aliases are deliberately inactive in forms and confirmation dialogs, so
usernames, passwords, raw commands, notices, and confirmation phrases are
entered literally. Up/Down arrows, `Tab`, and `Shift-Tab` continue to navigate
forms.

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

Soju prints usernames unquoted in `user status`. The TUI parser supports spaces
and colons, but new usernames may not begin/end with whitespace or end in a
status marker such as `(admin)` or `(disabled)`, because those forms cannot be
rediscovered unambiguously from Soju's output. An unusual pre-existing account
can still be entered manually.

## Operations

The menu covers the administration commands reported by the running server:

- server status, notices, debug logging, and BouncerServ help;
- all-user and specific-user status, dedicated password changes and resets,
  account updates, IRC identity updates, and deletion;
- per-user networks and channels;
- upstream SASL PLAIN and EXTERNAL/CertFP configuration;
- downstream client device certificates when supported by Soju;
- local inspection of the configured Soju host TLS certificate.

Read-only operations run immediately. Mutations first show the exact redacted
argument preview. Ordinary changes require `y`; destructive or high-risk
changes require the displayed phrase exactly.

### Runtime ages in status output

On Linux/systemd installations, **Server status** appends the current Soju
process age and UTC start time obtained from the configured service unit. A
user's **Network status** appends the age and UTC timestamp of the latest
matching `connection registered` event retained by that unit's journal:

```text
2/2 users, 6 downstreams, 6 upstreams, 6 networks, 39 channels; uptime 31d 4h (since 2026-07-25T05:41:02Z)
AlienIRCd (ircs://irc.example.net:6697) [connected]: 2 channels; connected for 31d 4h (since 2026-07-25T05:41:02Z)
```

The query is read-only and bounded. It is restricted to the current service
start so a stale event from an earlier Soju process cannot be presented as the
current connection. Disconnected or disabled networks never receive a
connection age. Missing commands, insufficient journal access, rotated events,
clock inconsistencies, and unsupported platforms all fail closed: the TUI
shows the unmodified `sojuctl` line and never substitutes a first-observed or
estimated timestamp.

## Password changes

**Change my password** replaces the bouncer-login password for one discovered
Soju account. The selector prefers an exact, case-sensitive match for the
current local Unix username when one exists, but the operator must confirm the
actual Soju account before continuing. This preference is not persisted and
does not define the administrator identity.

Soju authorizes this operation through access to its administrative socket, so
there is no old-password prompt or old-password verification. The new password
must be entered twice, is redacted from the preview and captured output, and is
never written to the local profile.

**Reset user password** provides the same replacement for any discovered user,
but treats it as a destructive administrator action and requires the exact
phrase `RESET USER PASSWORD USERNAME`. Existing clients for that account must
be updated with the replacement password. Both workflows use Soju's supported
`user update USER -password ...` command; they are separate TUI actions to make
the target and operational intent unmistakable.

## Network updates

Actions that target an existing network use a guided user → network flow. After
the user is chosen, the TUI runs `network status` and turns the saved names and
addresses into a `Space`-selectable field. Manual entry remains available for a
listing truncated by Soju.

**Update network for user** then fills the address, name, and enabled state
exposed by Soju. Only changed values are submitted.

**Reconnect upstream network** selects a saved network and runs Soju's
`network update NETWORK` command without changing its settings. Soju
disconnects and reconnects the upstream network, which applies newly generated
CertFP certificates or changed SASL credentials. This is a mutating operation
and requires the normal confirmation prompt.

Soju does not return saved upstream passwords, nicknames, usernames, real
names, CertFP values, or connect commands in network status. Those fields remain
blank to preserve the existing value, and saved passwords are never displayed.

The network form labels `-certfp` as **Server TLS fingerprint** because it pins
the upstream IRC server certificate; it is not SASL CertFP. It accepts SHA-256
or SHA-512. Up to 20 connect commands can be supplied using the simple first
field plus the JSON array for additional commands. Updating that list replaces
it. Because Soju does not disclose several saved values, **Explicitly clear**
can remove one password, server pin, identity value, or connect-command list per
reviewed operation. Clearing uses an exact confirmation phrase. Connect and raw
network commands are treated as potentially secret and redacted from the
confirmation preview.

Channel operations use the same network selector. Update and delete actions
also run `channel status` for the chosen network and offer the saved channels as
a selectable list. A channel update loads the current detached state. Soju does
not expose the saved relay-detached, reattach-on, detach-after, or detach-on
values in channel status, so those fields are labeled **blank keeps current**
and are sent only when the operator supplies a replacement.

Channel status defaults to **All networks**. Leaving its network field blank
has the same meaning; choosing a saved network filters the output.

## Certificate types

Soju uses three unrelated certificate concepts:

- **View Soju host TLS certificate** reads the public certificate from the
  config's `tls` line. It shows the configured domain (for example
  `soju.example.com`), subject, issuer such as Let's Encrypt, DNS names, validity,
  hostname match, SHA-256 fingerprint, and chain length. The private key is
  never read.
- **Upstream SASL certificate / CertFP** authenticates one user's connection to
  an upstream IRC network. Before generation, the TUI checks for an existing
  CertFP and displays its fingerprints. Creating and replacing use different
  exact confirmation phrases; an inconclusive preflight blocks the mutation.
  The fingerprint viewer defaults to **All networks** and performs one
  read-only Soju request per saved network, displaying grouped network headers
  and clearly marking networks where CertFP is not configured.
- **Client device certificates** authenticate downstream IRC clients to Soju.
  Registration also requires `client-cert-auth true`. These actions are omitted
  when the running Soju version lacks the command.

See [Certificate safety](certificates.md) for the complete mutation boundaries
and production Let's Encrypt guidance.

## Controls

- `↑`/`↓`, `k`/`j`, or `w`/`s` — move through actions and wrap at either end
- `←`/`→`, `h`/`l`, or `a`/`d` — go back or open the selected menu action
- `Page Up`/`Page Down` — scroll the main output older/newer
- `Shift`+`↑`/`↓` — scroll main output when supported by the terminal
- `Home`/`End` — jump to the first or last action
- `Enter` — open an action or advance a form
- `Tab`/`Shift-Tab` — move between fields
- `Space` — cycle discovered users, networks, channels, booleans, and choices
- `Ctrl-S` — preview or submit a form
- `y` — approve an ordinary mutation
- exact phrase + `Enter` — approve a destructive/high-risk mutation
- `n` or `Esc` — cancel a confirmation; `Esc` also cancels forms
- `r` — repeat the last read-only refresh
- `?` or `F1` — open built-in Soju-TUI help
- `q`, `Q`, `Ctrl-C`, or `Ctrl-Q` — open exit confirmation

Vim and WASD letter aliases apply only to menus and help. Forms and
confirmations always treat those letters as input.
