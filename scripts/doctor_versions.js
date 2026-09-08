#!/usr/bin/env node
/**
 * doctor_versions.js — compare the developer's local toolchain against
 * versions.env (the single source of truth) and report mismatches, so a
 * "works on my machine" failure from running a different Flutter/Go than CI is
 * caught locally instead of on a self-hosted runner. See #2287.
 *
 * Usage:
 *   node scripts/doctor_versions.js            # exit 1 on any mismatch
 *   node scripts/doctor_versions.js --advisory # always exit 0 (postinstall use)
 *
 * The version-comparison core (parse + extract + verdict) is pure and exported
 * for unit tests (doctor_versions.test.js); only run()/main() touch the system.
 */

'use strict';

const fs = require('fs');
const path = require('path');
const { execSync } = require('child_process');

const repoRoot = path.resolve(__dirname, '..');

// ─── Pure core (unit-tested) ────────────────────────────────────────────────

// Parse versions.env (KEY=VALUE, ignoring comments/blanks).
function parseVersionsEnv(text) {
  const out = {};
  for (const line of text.split('\n')) {
    const m = line.match(/^([A-Z0-9_]+)=(.+)$/);
    if (m) out[m[1]] = m[2].trim();
  }
  return out;
}

// Extractors turn raw `--version` output into a bare version string, or null
// when the tool is absent / output is unrecognized.
function extractGo(out) {
  const m = out.match(/go version go([0-9]+\.[0-9.]+)/);
  return m ? m[1] : null;
}
function extractNode(out) {
  const m = out.match(/v?([0-9]+\.[0-9]+\.[0-9]+)/);
  return m ? m[1] : null;
}
function extractFlutter(out) {
  const m = out.match(/Flutter\s+([0-9]+\.[0-9]+\.[0-9]+)/);
  return m ? m[1] : null;
}
function extractJava(out) {
  // `java -version` prints e.g. openjdk version "17.0.13" (to stderr).
  const m = out.match(/version "?([0-9]+)(?:\.[0-9.]+)?/);
  return m ? m[1] : null;
}
function extractXcode(out) {
  const m = out.match(/Xcode\s+([0-9]+\.[0-9.]+)/);
  return m ? m[1] : null;
}
function extractCocoaPods(out) {
  const m = out.match(/([0-9]+\.[0-9]+\.[0-9]+)/);
  return m ? m[1] : null;
}
function extractRuby(out) {
  const m = out.match(/ruby\s+([0-9]+\.[0-9]+\.[0-9]+)/);
  return m ? m[1] : null;
}
function extractGolangciLint(out) {
  // `golangci-lint --version` prints e.g.
  //   golangci-lint has version v1.64.8 built with go1.26.4 from ...
  const m = out.match(/has version v?([0-9]+\.[0-9]+\.[0-9]+)/);
  return m ? m[1] : null;
}
function extractGofumpt(out) {
  // `gofumpt --version` prints e.g. `v0.9.1 (go1.25.1)`.
  const m = out.match(/^v?([0-9]+\.[0-9]+\.[0-9]+)/);
  return m ? m[1] : null;
}

function major(v) {
  return String(v).split('.')[0];
}

// verdict compares an expected versions.env value against a detected version.
// mode 'exact' requires a full string match; mode 'major' compares only the
// leading major component (for values pinned at major granularity, e.g. Node).
// Returns one of: 'ok', 'mismatch', 'undetected'.
function verdict(mode, expected, detected) {
  if (detected == null) return 'undetected';
  if (mode === 'major') {
    return major(expected) === major(detected) ? 'ok' : 'mismatch';
  }
  return String(expected) === String(detected) ? 'ok' : 'mismatch';
}

// ─── Checks (declarative) ───────────────────────────────────────────────────

// Each check: which versions.env key it validates, how to read the local tool,
// how to extract its version, the comparison mode, and an optional platform
// gate ('darwin' for Mac-only iOS toolchain checks).
const CHECKS = [
  { key: 'GO_VERSION', label: 'Go', cmd: 'go version', extract: extractGo, mode: 'exact' },
  { key: 'NODE_MAJOR', label: 'Node', cmd: 'node --version', extract: extractNode, mode: 'major' },
  { key: 'FLUTTER_VERSION', label: 'Flutter', cmd: 'flutter --version', extract: extractFlutter, mode: 'exact' },
  { key: 'JAVA_VERSION', label: 'Java (JDK)', cmd: 'java -version', extract: extractJava, mode: 'major' },
  // Go lint toolchain. `npm run lint:go` is a required CI check (#1611), so a
  // local golangci-lint/gofumpt that differs from the runner's is the classic
  // "green locally, red in CI" (or worse, the reverse) — and a gofumpt bump can
  // reformat files under you.
  { key: 'GOLANGCI_LINT_VERSION', label: 'golangci-lint', cmd: 'golangci-lint --version', extract: extractGolangciLint, mode: 'exact' },
  { key: 'GOFUMPT_VERSION', label: 'gofumpt', cmd: 'gofumpt --version', extract: extractGofumpt, mode: 'exact' },
  // iOS/macOS toolchain — only meaningful on a Mac that builds iOS (the build
  // machine and dev Macs). Skipped on Linux.
  { key: 'XCODE_VERSION', label: 'Xcode', cmd: 'xcodebuild -version', extract: extractXcode, mode: 'exact', platform: 'darwin' },
  { key: 'COCOAPODS_VERSION', label: 'CocoaPods', cmd: 'pod --version', extract: extractCocoaPods, mode: 'exact', platform: 'darwin' },
  { key: 'RUBY_VERSION', label: 'Ruby', cmd: 'ruby --version', extract: extractRuby, mode: 'exact', platform: 'darwin' },
];

// ─── Impure shell + report ──────────────────────────────────────────────────

function run(cmd) {
  try {
    // Capture stderr too (java -version, xcodebuild print there).
    return execSync(cmd, { stdio: ['ignore', 'pipe', 'pipe'], encoding: 'utf8' });
  } catch {
    return null;
  }
}

function main() {
  const advisory = process.argv.includes('--advisory');
  const versions = parseVersionsEnv(fs.readFileSync(path.join(repoRoot, 'versions.env'), 'utf8'));

  const rows = [];
  let mismatches = 0;

  for (const c of CHECKS) {
    if (c.platform && c.platform !== process.platform) {
      rows.push({ label: c.label, expected: versions[c.key] ?? '(unset)', detected: '—', status: `skipped (${c.platform}-only)` });
      continue;
    }
    const expected = versions[c.key];
    if (expected === undefined) {
      rows.push({ label: c.label, expected: '(not in versions.env)', detected: '—', status: 'skipped' });
      continue;
    }
    const out = run(c.cmd);
    const detected = out == null ? null : c.extract(out);
    const v = verdict(c.mode, expected, detected);
    if (v === 'mismatch') mismatches += 1;
    rows.push({
      label: c.label,
      expected,
      detected: detected ?? 'not found',
      status: v === 'ok' ? 'ok' : v === 'mismatch' ? 'MISMATCH' : 'not installed',
    });
  }

  const width = Math.max(...rows.map((r) => r.label.length));
  console.log('Toolchain vs versions.env:\n');
  for (const r of rows) {
    const mark = r.status === 'ok' ? '✓' : r.status === 'MISMATCH' ? '✗' : '·';
    console.log(`  ${mark} ${r.label.padEnd(width)}  expected ${r.expected}  /  local ${r.detected}  [${r.status}]`);
  }

  if (mismatches > 0) {
    console.error(`\n${mismatches} toolchain mismatch(es) vs versions.env. Align your local tools (or update versions.env if intentional).`);
    if (!advisory) process.exit(1);
  } else {
    console.log('\nAll detected tools match versions.env.');
  }
}

if (require.main === module) {
  main();
}

module.exports = {
  parseVersionsEnv,
  extractGo,
  extractNode,
  extractFlutter,
  extractJava,
  extractXcode,
  extractCocoaPods,
  extractRuby,
  verdict,
  major,
  CHECKS,
};
