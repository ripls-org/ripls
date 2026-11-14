// Self-hosted Renovate ADMIN config (the pieces that cannot live in the repo's
// renovate.json). Mounted into the container by the host's run-renovate.sh.
// Contains NO secrets: the credentials arrive via the environment.
//
// The repo-level rules (managers, packageRules, customManagers, the gomod
// postUpgradeTasks) live in renovate.json on the default branch and are read
// from there at run time.
module.exports = {
  // The repository lives on a Forgejo instance (#3293).
  platform: 'forgejo',
  endpoint: 'https://code.overt.org/api/v1/',

  // Auth: RENOVATE_TOKEN (read from the environment by Renovate) is an access
  // token of the `ripls-renovate` account on that instance, kept in a
  // host-only file. Never hard-code it here.
  //
  // GITHUB_COM_TOKEN, when the host provides it, is a short-lived token
  // minted from the old ripls-renovate GitHub App by mint_app_token.js. It
  // only raises the rate limit for looking up releases and changelogs of
  // dependencies hosted on github.com; it cannot write anywhere Renovate
  // works.

  // Commits are authored as the bot account, so the forge shows them as its.
  gitAuthor: 'ripls-renovate <ripls-renovate@noreply.code.overt.org>',

  // Only this repo; do not crawl the instance.
  autodiscover: false,
  repositories: ['ripls-org/ripls'],

  // Config lives in the repo on the default branch — skip onboarding PRs and
  // tolerate repos without an inline config block.
  onboarding: false,
  requireConfig: 'optional',

  // The renovate/renovate base image has no npm and tool versions differ per
  // manager, so let containerbase install the correct tool per manager on
  // demand. The Go toolchain, buf, and the protoc plugins baked into
  // renovate/Dockerfile stay on PATH for the gomod go.sum postUpgradeTask.
  binarySource: 'install',

  // SECURITY GATE: authorize ONLY the go.sum regen postUpgradeTask declared in
  // renovate.json. This allowlist is admin-only and intentionally cannot be set
  // from the repo config — it is what lets postUpgradeTasks run any shell at all.
  allowedCommands: ['^bash scripts/renovate_gomod_regen\\.sh$'],
};
