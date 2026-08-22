# GitHub repository and checks

GitHub is the writable source of truth for this project. Development, pull
requests, protected checks, tags, and releases happen there. A Gitea copy may
be maintained as a read-only downstream mirror, but it must never push back to
GitHub.

The repository ships GitHub Actions under `.github/workflows/` and no native
`.gitea/workflows/` workflow. Gitea installations may discover GitHub workflow
files, so disable Actions for the downstream repository. Every job also checks
that it is running on `https://github.com`.

## Checks

- `Verify` runs formatting, module verification, race tests, coverage,
  staticcheck, gosec, govulncheck, shellcheck, workflow validation, and both
  static Linux builds.
- `Soju Compatibility` starts isolated Soju v0.9.0 and v0.10.1 instances and
  verifies the `sojuctl` contract used by the TUI.
- `Analyze Go` runs CodeQL.

Dependabot checks Go modules and pinned GitHub Actions each Monday. Workflow
actions are pinned to full commit IDs, and Dependabot retains that protection
when proposing updates.

## Protect `main`

Under **Settings > Rules > Rulesets**, keep the active `main` ruleset configured
to block deletions and force pushes, require pull requests, require branches to
be current, and require these GitHub Actions checks:

- `Verify`
- `Soju Compatibility`
- `Analyze Go`

Keep the bypass list empty unless a narrowly scoped automation identity has a
documented need. Do not enable a broad administrator bypass. The repository
history is append-only; do not rewrite or force-push `main` again.

The one historical rewrite performed before GitHub became authoritative is
connected back to current history by a no-content merge. Existing clean clones
from either side of that rewrite can therefore fast-forward to current `main`.

## Automated releases

Use **Actions > Release > Run workflow** and enter the intended semantic
version without a `v` prefix. The workflow can run only from `main`. It repeats
the production gates, builds AMD64 and ARM64 artifacts, generates checksums and
build metadata, and publishes the tag and release as `github-actions[bot]`.

The publishing job has only `contents: write`; verification jobs remain
read-only. GitHub supplies a temporary repository-scoped token, so no personal
access token, repository secret, or GPG key is required. Existing tags and
releases are never replaced.

## Downstream Gitea mirror

Create or retain Gitea as a pull mirror of
`https://github.com/variablenix/soju-tui.git` with **This repository will be a
mirror** enabled. The Gitea copy should be treated as read-only and periodically
pull from GitHub. Disable Gitea repository Actions.

Do not configure a Gitea push mirror back to GitHub, do not make commits only
in Gitea, and do not run writable mirrors in both directions. On servers and
workstations, clone GitHub directly when you need to build, contribute, or
receive releases immediately.

## Dependabot and code scanning

Keep Dependabot alerts and security updates enabled under **Settings > Advanced
Security**. Use the committed `codeql.yml` as advanced CodeQL setup rather than
enabling default setup in parallel. Review and merge Dependabot pull requests
on GitHub after all protected checks pass.
