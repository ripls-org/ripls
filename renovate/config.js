// Self-hosted Renovate ADMIN config (the pieces that cannot live in the repo's
// renovate.json). Mounted into the container by run_renovate.sh. Contains NO
// secrets: the GitHub App credentials arrive via the environment.
//
// The repo-level rules (managers, packageRules, customManagers, the gomod
// postUpgradeTasks) live in renovate.json on the default branch and are read
// from there at run time.
module.exports = {
  platform: 'github',

  // Auth: RENOVATE_TOKEN (read from the environment by Renovate) is an
  // installation access token minted from the ripls-renovate GitHub App by
  // run_renovate.sh. Never hard-code it here.

  // Only this repo; do not crawl the org.
  autodiscover: false,
  repositories: ['Type2Fun-Foundation/ripls'],

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
