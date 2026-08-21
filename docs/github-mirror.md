# GitHub mirror and checks

The repository ships GitHub Actions under `.github/workflows/` and no native
`.gitea/workflows/` workflow. Current Gitea installations discover both
directories by default, so disable Actions for this repository in Gitea. Every
job also requires `github.server_url` to be `https://github.com`; this prevents
runner use if Gitea Actions is accidentally enabled, although Gitea may still
display the detected jobs as skipped. A GitHub mirror runs three stable checks:

- `Verify` runs formatting, module verification, race tests, coverage,
  staticcheck, gosec, govulncheck, shellcheck, and both static Linux builds.
- `Soju Compatibility` starts isolated Soju v0.9.0 and v0.10.1 instances and
  verifies the `sojuctl` contract used by the TUI.
- `Analyze Go` runs CodeQL. CodeQL must be available for the GitHub repository
  before this check can be required.

Dependabot checks Go modules and pinned GitHub Actions each Monday. The workflow
actions are pinned to full commit IDs; Dependabot retains that protection when
it proposes updates.

## Choose one source of truth

A Gitea push mirror force-pushes branches and tags to GitHub. Changes made only
on GitHub can therefore be overwritten by the next synchronization.

For a **Gitea-primary mirror**, make changes and releases in Gitea. Treat GitHub
as read-only, do not merge Dependabot or other GitHub pull requests, and allow
only the dedicated mirror identity to bypass the `main` ruleset. GitHub Actions
still report failures, but they run after Gitea has updated `main`; they cannot
gate the authoritative Gitea push.

For **GitHub-gated development**, make GitHub the write authority for `main` and
use pull requests there. Stop the Gitea-to-GitHub push mirror before merging
GitHub pull requests. A separate GitHub-to-Gitea synchronization can then keep
Gitea as a downstream copy. Do not run writable mirrors in both directions.

## Create the Gitea-primary push mirror

1. In the Gitea repository's **Settings > Actions**, disable repository Actions.
   This does not affect GitHub after mirroring.
2. Commit and push `.github/` to Gitea.
3. Create an empty GitHub repository named `soju-tui`; do not initialize it with
   a README, license, or `.gitignore`.
4. Create a dedicated, expiring GitHub token limited to the destination
   repository. It needs repository contents write access and permission to
   update workflow files. A classic token uses `public_repo` for a public
   destination (`repo` for private) plus `workflow`.
5. In Gitea, open **Settings > Repository > Mirror Settings**. Enter
   `https://github.com/OWNER/soju-tui.git`, expand **Authorization**, and enter
   the GitHub username and token. Do not put the token in the URL.
6. Enable **Sync when new commits are pushed**, add the push mirror, then use
   **Synchronize Now** for the first copy.
7. Rotate or revoke the token if it is ever exposed. Keep its repository scope
   and expiration as narrow as practical.

## Enable and run GitHub checks

1. On GitHub, open **Settings > Actions > General**. Allow actions created by
   GitHub and leave the default workflow token at read-only repository access.
   The workflows request only the additional CodeQL upload permission they use.
2. Do not enable CodeQL default setup in addition to this repository's advanced
   workflow. If the repository is eligible, use the committed `codeql.yml` as
   advanced setup.
3. Open **Actions**, select **CI**, and choose **Run workflow**. Repeat for
   **CodeQL**. Manual runs are available after the files exist on the default
   branch.
4. Confirm `Verify`, `Soju Compatibility`, and—when CodeQL is available—
   `Analyze Go` all complete successfully. Checks must run at least once before
   GitHub offers them in a ruleset.

## Protect `main` with a ruleset

Open **Settings > Rules > Rulesets > New ruleset > New branch ruleset** and use:

- Name: `Protect main`
- Enforcement: `Active`
- Target: the default branch (`main`)
- Rules: block deletions, block force pushes, require a pull request, resolve
  review conversations, and require status checks
- Required checks from GitHub Actions: `Verify`, `Soju Compatibility`, and
  `Analyze Go` when CodeQL is available
- Additional status setting: require branches to be up to date before merging

For a Gitea-primary push mirror, grant bypass only through the narrowest actor
GitHub offers, such as a dedicated GitHub App or organization team containing
only the mirror account. If the mirror identity cannot be isolated in the
bypass list, the ruleset and continuous force-push mirror are not a safe
combination; choose GitHub-gated development instead.

Do not require signed commits unless the complete mirrored history satisfies
that rule. Do not enable a broad administrator bypass merely to make mirroring
convenient.

## Dependabot

The committed `.github/dependabot.yml` enables version-update pull requests.
Enable Dependabot alerts and security updates under **Settings > Advanced
Security** where available. With a Gitea-primary mirror, review each proposal
on GitHub but reproduce and test the dependency change in Gitea; merging the
GitHub pull request would create history that the push mirror can overwrite.
