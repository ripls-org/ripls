#!/usr/bin/env node
// Decides whether a Renovate PR is *about* a dependency that renovate.json
// deliberately holds back — an `allowedVersions` cap or an `enabled: false`
// block. `claude_fix_renovate_ci.yaml` calls this before handing the PR to
// Claude: if the answer is yes, the failure is probably the documented hold
// doing its job, and a human should decide rather than an agent risking a
// "fix" that loosens the cap.
//
// WHY THIS IS A SCRIPT AND NOT INLINE SHELL. The gate used to be a bash loop
// that grepped the PR's title+body+branch for each held package name. It was
// wrong in both directions and neither failure was visible:
//
//   1. Failed OPEN on big PRs. The match was
//        `if printf '%s' "$HAYSTACK" | grep -qF -- "$needle"`
//      under `set -o pipefail`. `grep -q` exits at the first match, `printf`
//      then takes SIGPIPE, and the pipeline returns 141 — so a match was read
//      as "no match". It only worked when the body was small enough for printf
//      to finish first, which meant the gate silently stopped protecting the
//      holds on exactly the large grouped PRs most likely to touch one.
//   2. Failed CLOSED on small ones. Needles were bare substrings matched
//      against prose, so `golang` (the Docker base image) matched the string
//      "golang.org/x/text" in a changelog. PR #2655 bumped phonenumbers and was
//      told "This PR bumps `golang`, which is capped"; it was then closed, and
//      closing a major PR makes Renovate ignore every future 2.x release. That
//      dependency sat on v1 until it was upgraded by hand in #3007.
//
// The fix for both is to stop reading prose. A hold is about a *dependency in a
// manifest*, so this matches the PR's diff, and matches each held name only
// against the files its manager owns and only as a whole token. `golang` then
// matches `FROM golang:1.26` in a Dockerfile and does not match
// `golang.org/x/crypto` in go.mod.
//
// Usage:
//   node scripts/renovate_hold_gate.js --config renovate.json --diff pr.diff
// Prints the matched package name and exits 0; prints nothing and exits 0 when
// no hold matches. Exits 2 on bad input.

'use strict';

const fs = require('node:fs');

// Characters that can appear inside a dependency name. A needle only counts as
// a match when it is delimited by something outside this set, which is what
// keeps `protobuf` from matching `google.golang.org/protobuf`.
const NAME_CHARS = /[A-Za-z0-9._/@-]/;

// Which files each Renovate manager owns. A held name is only looked for in
// its own manager's manifests, so a pub `protobuf` cap can never be tripped by
// a Go module diff. Managers absent here fall back to "any changed file".
const MANAGER_PATHS = {
  gomod: [/(^|\/)go\.(mod|sum)$/],
  pub: [/(^|\/)pubspec\.(yaml|lock)$/],
  npm: [/(^|\/)package(-lock)?\.json$/],
  dockerfile: [/(^|\/)Dockerfile(\.|$)/, /(^|\/)[^/]*\.dockerfile$/i],
  'docker-compose': [/(^|\/)docker-compose[^/]*\.ya?ml$/],
  'github-actions': [/^\.github\/(workflows|actions)\//],
  gradle: [/(^|\/)(build|settings)\.gradle(\.kts)?$/, /(^|\/)gradle\.properties$/],
  'gradle-wrapper': [/gradle-wrapper\.properties$/],
  terraform: [/\.tf$/, /\.terraform\.lock\.hcl$/],
  bundler: [/(^|\/)Gemfile(\.lock)?$/],
  cocoapods: [/(^|\/)Podfile(\.lock)?$/],
};

/** Held package names from renovate.json, each with the managers that own it. */
function heldPackages(config) {
  const out = [];
  for (const rule of config.packageRules ?? []) {
    const held =
      Object.hasOwn(rule, 'allowedVersions') || rule.enabled === false;
    if (!held) continue;
    for (const name of rule.matchPackageNames ?? []) {
      // Renovate treats a leading "/" as a regex matcher; only literal names
      // can be looked for as tokens.
      if (name.startsWith('/')) continue;
      out.push({ name, managers: rule.matchManagers ?? null });
    }
  }
  return out;
}

/** Changed files in a unified diff, each with its added/removed lines. */
function parseDiff(diff) {
  const files = [];
  let current = null;
  for (const line of diff.split('\n')) {
    const header = /^diff --git a\/(.+?) b\/(.+)$/.exec(line);
    if (header) {
      current = { path: header[2], lines: [] };
      files.push(current);
      continue;
    }
    if (!current) continue;
    // +++/--- are file headers, not content.
    if (/^(\+\+\+|---)/.test(line)) continue;
    if (/^[+-]/.test(line)) current.lines.push(line.slice(1));
  }
  return files;
}

/** True when `needle` appears in `text` delimited by non-dependency-name characters. */
function hasToken(text, needle) {
  let from = 0;
  for (;;) {
    const i = text.indexOf(needle, from);
    if (i === -1) return false;
    const before = i === 0 ? '' : text[i - 1];
    const after = text[i + needle.length] ?? '';
    if (!NAME_CHARS.test(before) && !NAME_CHARS.test(after)) return true;
    from = i + 1;
  }
}

function ownsFile(managers, path) {
  if (!managers) return true; // no manager scope declared — check everywhere
  return managers.some((m) =>
    (MANAGER_PATHS[m] ?? []).some((re) => re.test(path)),
  );
}

/** The first held package this diff actually touches, or null. */
function matchHold(config, diff) {
  const files = parseDiff(diff);
  for (const { name, managers } of heldPackages(config)) {
    for (const file of files) {
      if (!ownsFile(managers, file.path)) continue;
      if (file.lines.some((l) => hasToken(l, name))) return name;
    }
  }
  return null;
}

function main(argv) {
  const arg = (flag) => {
    const i = argv.indexOf(flag);
    return i === -1 ? null : argv[i + 1];
  };
  const configPath = arg('--config');
  const diffPath = arg('--diff');
  if (!configPath || !diffPath) {
    console.error('usage: renovate_hold_gate.js --config <renovate.json> --diff <pr.diff>');
    process.exit(2);
  }
  const config = JSON.parse(fs.readFileSync(configPath, 'utf8'));
  const diff = fs.readFileSync(diffPath, 'utf8');
  const hit = matchHold(config, diff);
  if (hit) console.log(hit);
}

if (require.main === module) {
  main(process.argv.slice(2));
}

module.exports = { heldPackages, parseDiff, hasToken, matchHold };
