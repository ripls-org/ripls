#!/usr/bin/env node
/**
 * check_versions.js — keep the toolchain versions consumed across the repo in
 * sync with the single source of truth, versions.env.
 *
 * After #2287, most consumers read versions.env directly via build-args or by
 * sourcing it in a workflow step, so there are no literals to drift there. This
 * script now enforces two things:
 *
 *   1. SOURCE MIRRORS — a few files are themselves the source the build reads
 *      and so cannot take a build-arg: go.mod's `go` directive, and the prod
 *      image's ARG defaults (used by plain `docker build` in test_docker_build).
 *      These must equal the matching versions.env value.
 *
 *   2. NO STRAY LITERALS — the files migrated to consume versions.env must not
 *      re-introduce a hardcoded version. The runner Dockerfile must declare each
 *      baked tool as a bare `ARG VAR` (build-arg, no `=literal`); the Flutter
 *      workflows must source versions.env rather than pin FLUTTER_VERSION inline.
 *
 * To bump a version: edit versions.env. For source mirrors (go.mod, the prod
 * Dockerfile ARG defaults) also update the literal there — this script names the
 * exact file/line on failure. Runner/workflow consumers need no edit.
 */

'use strict';

const fs = require('fs');
const path = require('path');

const repoRoot = path.resolve(__dirname, '..');

function read(rel) {
  return fs.readFileSync(path.join(repoRoot, rel), 'utf8');
}

// Files that exist only in the private deploy repo. The open-source export
// excludes the runner fleet and every deploy/release workflow (#2953), so the
// mirrors they carry have nothing to check there — and this script is a
// REQUIRED CI check, so reading them unconditionally crashed the public repo's
// lint outright rather than degrading.
const skippedPrivate = [];
function readPrivate(rel) {
  const abs = path.join(repoRoot, rel);
  if (!fs.existsSync(abs)) {
    skippedPrivate.push(rel);
    return null;
  }
  return fs.readFileSync(abs, 'utf8');
}

// Parse versions.env (KEY=VALUE, ignoring comments/blanks).
function parseVersionsEnv() {
  const out = {};
  for (const line of read('versions.env').split('\n')) {
    const m = line.match(/^([A-Z0-9_]+)=(.+)$/);
    if (m) out[m[1]] = m[2].trim();
  }
  return out;
}

const versions = parseVersionsEnv();

// 1. Source mirrors: hand-edited files that must equal a versions.env value
//    because the build reads them directly (they cannot be a build-arg).
const mirrors = [
  { key: 'GO_VERSION', file: 'go.mod', re: /^go ([0-9][0-9.]*)/m, label: 'go.mod directive' },
  { key: 'GO_VERSION', file: 'Dockerfile', re: /ARG GO_VERSION=([0-9][0-9.]*)/, label: 'prod image ARG default' },
  { key: 'ONNXRUNTIME_VERSION', file: 'Dockerfile', re: /ARG ONNXRUNTIME_VERSION=([0-9][0-9.]*)/, label: 'prod image ARG default' },
];

// 2. Build-arg / sourced consumers: files that must take the version from
//    versions.env and must NOT hardcode it. Regression guard against re-pinning.
const runnerBakedKeys = [
  'GO_VERSION',
  'NODE_MAJOR',
  'FLUTTER_VERSION',
  'JAVA_VERSION',
  'NDK_VERSION',
  'ONNXRUNTIME_VERSION',
  // Go lint toolchain, pinned in #1611 once `npm run lint:go` became a required
  // check. Consumed as build args by runners/Dockerfile.
  'GOLANGCI_LINT_VERSION',
  'GOFUMPT_VERSION',
];
const flutterWorkflows = [
  { file: '.github/workflows/test_ios_build.yaml', mustSource: true },
  { file: '.github/workflows/release_app.yaml', mustSource: true },
  { file: '.github/workflows/release_build_android.yaml', mustSource: false },
];

// Keys this build-time guard does not verify here — the dev-local doctor
// (npm run doctor:versions) checks these against the Mac that builds iOS.
// IOS_DEPLOYMENT_TARGET is NOT here: it is a source mirror checked below.
const uncheckedKeys = ['XCODE_VERSION', 'COCOAPODS_VERSION', 'RUBY_VERSION'];

const violations = [];

// Every versions.env key must be accounted for by a mirror, a runner build-arg,
// or the unchecked allowlist — otherwise a new key silently goes unguarded.
const accountedKeys = new Set([
  ...mirrors.map((m) => m.key),
  ...runnerBakedKeys,
  ...uncheckedKeys,
  'IOS_DEPLOYMENT_TARGET', // checked against the Podfile + Xcode project below.
]);
for (const key of Object.keys(versions)) {
  if (!accountedKeys.has(key)) {
    violations.push(`versions.env defines ${key} but check_versions.js does not account for it — add a mirror, a consumer, or list it in uncheckedKeys.`);
  }
}

// Source-mirror checks.
for (const m of mirrors) {
  const want = versions[m.key];
  if (want === undefined) {
    violations.push(`${m.file} (${m.label}) mirrors ${m.key}, but versions.env does not define it.`);
    continue;
  }
  const found = read(m.file).match(m.re);
  if (!found) {
    violations.push(`${m.file} (${m.label}): could not find ${m.key} via ${m.re} — did the file move or change shape?`);
    continue;
  }
  if (found[1] !== want) {
    violations.push(`${m.file} (${m.label}): ${m.key} is ${found[1]} but versions.env says ${want}. Update one to match.`);
  }
}

// Runner Dockerfile: each baked tool must be a bare `ARG VAR` (no literal).
const runnerDockerfile = readPrivate('runners/Dockerfile');
if (runnerDockerfile) {
  for (const key of runnerBakedKeys) {
    if (new RegExp(`(?:ENV|ARG)\\s+${key}\\s*=`).test(runnerDockerfile)) {
      violations.push(`runners/Dockerfile: ${key} is assigned a literal — it must be a bare \`ARG ${key}\` supplied as a build arg from versions.env.`);
    } else if (!new RegExp(`ARG\\s+${key}\\b`).test(runnerDockerfile)) {
      violations.push(`runners/Dockerfile: missing \`ARG ${key}\` declaration — the build won't receive ${key} from versions.env.`);
    }
  }
}

// Runner image builders: every baked tool must actually be SUPPLIED, not just
// declared. #2883: the lint toolchain was wired into build_runner_image.yaml but
// not into validate_runner_candidate.yaml, so every candidate build died on an
// empty `golangci-lint@v`. The two CI builders now share
// scripts/build_runner_image.sh, which derives its build-args from the
// Dockerfile's ARGs; compose keeps a hand-written list, so check that one.
for (const wf of ['.github/workflows/build_runner_image.yaml', '.github/workflows/validate_runner_candidate.yaml']) {
  const text = readPrivate(wf);
  if (!text) continue;
  if (!/scripts\/build_runner_image\.sh/.test(text)) {
    violations.push(`${wf}: builds the runner image without scripts/build_runner_image.sh — use the shared script so every versions.env build-arg is supplied.`);
  }
  if (/--build-arg/.test(text)) {
    violations.push(`${wf}: hand-rolls \`--build-arg\` for the runner image — that list drifts (#2883). Let scripts/build_runner_image.sh derive it.`);
  }
}
const runnerCompose = readPrivate('runners/docker-compose.yml');
if (runnerCompose) {
  for (const key of runnerBakedKeys) {
    if (!new RegExp(`^\\s*${key}:\\s*\\$\\{${key}\\}`, 'm').test(runnerCompose)) {
      violations.push(`runners/docker-compose.yml: build args must pass \`${key}: \${${key}}\` — the manual \`sync.sh --rebuild\` path would bake an empty ${key}.`);
    }
  }
}

// iOS deployment target: versions.env must match the Podfile platform line and
// EVERY build config in the Xcode project (debug/release/profile), so they
// can't silently drift apart.
const iosTarget = versions.IOS_DEPLOYMENT_TARGET;
if (iosTarget !== undefined) {
  const podfile = read('app/ios/Podfile');
  const podMatch = podfile.match(/platform :ios, '([0-9][0-9.]*)'/);
  if (!podMatch) {
    violations.push("app/ios/Podfile: could not find a `platform :ios, '<version>'` line.");
  } else if (podMatch[1] !== iosTarget) {
    violations.push(`app/ios/Podfile: platform :ios is ${podMatch[1]} but versions.env IOS_DEPLOYMENT_TARGET is ${iosTarget}.`);
  }

  const pbxproj = read('app/ios/Runner.xcodeproj/project.pbxproj');
  const targets = [...pbxproj.matchAll(/IPHONEOS_DEPLOYMENT_TARGET = ([0-9][0-9.]*);/g)].map((m) => m[1]);
  if (targets.length === 0) {
    violations.push('app/ios/Runner.xcodeproj/project.pbxproj: no IPHONEOS_DEPLOYMENT_TARGET found.');
  }
  for (const t of targets) {
    if (t !== iosTarget) {
      violations.push(`app/ios/Runner.xcodeproj/project.pbxproj: IPHONEOS_DEPLOYMENT_TARGET ${t} != versions.env IOS_DEPLOYMENT_TARGET ${iosTarget}.`);
    }
  }
}

// Flutter workflows: FLUTTER_VERSION must be sourced, not pinned as a literal.
for (const wf of flutterWorkflows) {
  const text = readPrivate(wf.file);
  if (!text) continue;
  if (/FLUTTER_VERSION\s*:\s*['"]?[0-9]/.test(text)) {
    violations.push(`${wf.file}: FLUTTER_VERSION is pinned inline — it must be sourced from versions.env instead.`);
  }
  if (wf.mustSource && !/versions\.env/.test(text)) {
    violations.push(`${wf.file}: installs Flutter but does not source versions.env — add a step that loads FLUTTER_VERSION from it.`);
  }
}

if (violations.length > 0) {
  console.error('check_versions FAILED — toolchain versions out of sync with versions.env:\n');
  for (const v of violations) console.error(`  - ${v}`);
  console.error('\nversions.env is the source of truth. Bump it, then update the mirrors above.');
  process.exit(1);
}

if (skippedPrivate.length > 0) {
  // Named rather than silent: "passed" should not be able to mean "checked
  // almost nothing because the files were not there".
  console.log(
    `check_versions: skipped ${skippedPrivate.length} private-repo mirror(s) not present here ` +
      `(${skippedPrivate.join(', ')}).`,
  );
}

console.log(`check_versions passed: ${mirrors.length} source mirror(s) + runner/workflow consumers match versions.env.`);
