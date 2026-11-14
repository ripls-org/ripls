#!/usr/bin/env node
/**
 * oss_scan.js — does a tracked file name something that must stay private?
 *
 * The repository is public. This scans it for the values listed in a denylist
 * that is itself not public, and fails the pull request that adds one. gitleaks covers credentials; this
 * covers what gitleaks has no rule for: people's names, addresses and phone
 * numbers, and identifiers the instance keeps to itself.
 *
 * REDACTION. Matched text is never printed, only `path:line  [category]`. The
 * denylist is a list of things we are trying not to publish; echoing a match
 * into a CI log (or a PR comment) republishes it. `--json` follows the same
 * rule. Do not add a --show-matches flag.
 *
 * Usage:
 *   node scripts/oss_scan.js --gate --tree [--dir <checkout>]  # exit 1 on any hit
 *   node scripts/oss_scan.js --report --tree [--json]          # summary, exit 0
 *   node scripts/oss_scan.js --gate --dir <directory>          # every file under it
 *
 * `--tree` scans the files `git ls-files` lists in a checkout (this one by
 * default). It is what CI runs (`npm run oss:gate:tree`,
 * .github/workflows/check_denylist.yaml). Without it, `--dir` names a plain
 * directory and every file under it is scanned.
 *
 * The denylist is read from $OSS_DENYLIST_FILE when set, else from
 * scripts/oss_denylist.txt. It is not in git: it names the values it exists to
 * keep out, so maintainers keep it privately and CI writes it from the
 * OSS_DENYLIST secret. A local copy goes at the default path, which .gitignore
 * covers. A missing denylist exits 2 rather than scanning with no rules, which
 * would pass everything. Without the denylist (a fork, an outside contributor)
 * the gate cannot run; CI skips it with a notice and a maintainer's run
 * catches what it would have.
 *
 * History: until the repository itself became public, this was the second
 * gate of an export step that copied an allowlisted subset of a private
 * repository into this one, hence the name.
 *
 * The scanning core is pure and exported for scripts/oss_scan.test.js; only
 * collectFiles()/listTrackedFiles()/main() touch the filesystem.
 */

'use strict';

const { execFileSync } = require('child_process');
const fs = require('fs');
const path = require('path');

const ROOT = path.join(__dirname, '..');
const DENYLIST_REL = path.join('scripts', 'oss_denylist.txt');

// Files whose whole point is to name the product or the org. Exempting by path
// keeps the patterns strong; weakening a pattern to accommodate one file blinds
// it everywhere.
const EXEMPT_PATHS = new Set([
  'TRADEMARK.md', // reserves the marks — must name them
  'NOTICE', // attribution
  'LICENSE',
  // A negative guard: it asserts the SSR pages contain NONE of these, so it
  // has to spell them out. Same reason the denylist itself is exempt.
  'server/services/web/branding_render_test.go',
  // Domain association files. The Apple Team ID and the Android signing-cert
  // fingerprints in them are served to the public internet at a fixed URL —
  // every device that opens one of our links fetches them — so there is no
  // confidentiality to protect, and the alternatives all cost something real.
  // The reasoning is in the README beside them.
  'server/services/web/static/.well-known/apple-app-site-association',
  'server/services/web/static/.well-known/assetlinks.json',
  'scripts/oss_denylist.txt', // the list itself
  'scripts/oss_scan.js', // this file's prose
  'scripts/oss_scan.test.js',
]);

// Path prefixes that are allowed to carry reserved test phone numbers and
// synthetic credentials. Kept narrow and enumerated.
const FIXTURE_PREFIXES = ['server/test_data/flyers/'];

// Extensions we do not scan as text.
const BINARY_EXTS = new Set([
  '.png', '.jpg', '.jpeg', '.webp', '.gif', '.heic', '.ico', '.svg',
  '.mp4', '.mov', '.webm', '.mp3', '.wav',
  '.ttf', '.otf', '.woff', '.woff2',
  '.onnx', '.zip', '.gz', '.jar', '.keystore', '.jks', '.pdf',
]);

// Reserved-for-fiction numbers and the canonical Twilio example. Legitimate in
// test fixtures anywhere, so they are subtracted from a line before the phone
// pattern runs rather than exempted by path.
//
// Two shapes, because this repo uses both: 555 as the AREA code (`+15551234567`,
// the dominant fixture style) and 555 as the EXCHANGE (`+14155551234`, the NANP
// 555-01xx convention). Matching only the second lets every `+1555…` fixture
// read as a real number.
const RESERVED_PHONE_RE = /\+1555[0-9]{7}|\+1[0-9]{3}555[0-9]{4}|\+14158675309/g;

/**
 * parseDenylist turns denylist text into [{ category, source, re }].
 * Pure — exported for the unit test.
 */
function parseDenylist(text) {
  const rules = [];
  let category = 'uncategorized';
  let gateAccepted = false;
  let allowUnder = [];

  text.split('\n').forEach((rawLine) => {
    const line = rawLine.trim();
    if (line === '' || line.startsWith('#')) return;

    const section = line.match(/^\[([^\]]+)\]$/);
    if (section) {
      category = section[1];
      gateAccepted = false;
      allowUnder = [];
      return;
    }

    // `!allow-under <dir>/` exempts the section's patterns in files under one
    // directory, for text that is meant to be public there (the website names
    // the people who run it). Narrower than EXEMPT_PATHS: other sections still
    // scan those files, and this section still scans everything else.
    if (line.startsWith('!allow-under')) {
      const prefix = line.slice('!allow-under'.length).trim();
      if (!prefix.endsWith('/')) {
        throw new Error(`denylist: !allow-under needs a directory ending in "/", got "${prefix}"`);
      }
      allowUnder = [...allowUnder, prefix];
      return;
    }

    // `!gate-accepted` marks the whole section as expected-to-hit: --gate
    // reports it but does not fail on it. Declared here rather than in the
    // script so the reason sits beside the patterns it excuses, and so adding
    // a NEW category is fail-closed — anything unmarked gates.
    if (line === '!gate-accepted') {
      gateAccepted = true;
      return;
    }

    rules.push({ category, gateAccepted, allowUnder, source: line, re: new RegExp(line, 'i') });
  });

  return rules;
}

/**
 * scanText returns one finding per (line, category) — never the matched text.
 * Pure — exported for the unit test.
 */
function scanText(filePath, text, rules) {
  const findings = [];
  const lines = text.split('\n');
  const active = rules.filter(
    (rule) => !(rule.allowUnder || []).some((prefix) => filePath.startsWith(prefix)),
  );

  lines.forEach((line, idx) => {
    const scrubbed = line.replace(RESERVED_PHONE_RE, '');
    const seen = new Set();
    active.forEach((rule) => {
      if (seen.has(rule.category)) return;
      if (!rule.re.test(scrubbed)) return;
      seen.add(rule.category);
      findings.push({ file: filePath, line: idx + 1, category: rule.category });
    });
  });

  return findings;
}

/** isExempt reports whether a repo-relative path is excused from scanning. */
function isExempt(filePath) {
  if (EXEMPT_PATHS.has(filePath)) return true;
  return FIXTURE_PREFIXES.some((prefix) => filePath.startsWith(prefix));
}

/** isScannable reports whether a path looks like text worth scanning. */
function isScannable(filePath) {
  return !BINARY_EXTS.has(path.extname(filePath).toLowerCase());
}

/**
 * summarize groups findings into { category -> { hits, files } }.
 * Pure — exported for the unit test.
 */
function summarize(findings) {
  const byCategory = new Map();
  findings.forEach((finding) => {
    const entry = byCategory.get(finding.category) || { hits: 0, files: new Set() };
    entry.hits += 1;
    entry.files.add(finding.file);
    byCategory.set(finding.category, entry);
  });
  return byCategory;
}

// ---------------------------------------------------------------------------
// Filesystem side
// ---------------------------------------------------------------------------

function collectFiles(absDir) {
  const out = [];
  const walk = (dir) => {
    fs.readdirSync(dir, { withFileTypes: true }).forEach((entry) => {
      const abs = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        if (entry.name === '.git' || entry.name === 'node_modules') return;
        walk(abs);
        return;
      }
      out.push(path.relative(absDir, abs).split(path.sep).join('/'));
    });
  };
  walk(absDir);
  return out.sort();
}

/**
 * listTrackedFiles returns the files git tracks in the checkout at absDir.
 * Untracked and ignored files are left out: they are not part of any change
 * that could be pushed.
 */
function listTrackedFiles(absDir) {
  const out = execFileSync('git', ['-C', absDir, 'ls-files', '-z'], { encoding: 'utf8' });
  return out
    .split('\0')
    .filter((f) => f !== '')
    .sort();
}

/** denylistPath resolves where the denylist is read from. */
function denylistPath(env) {
  return env.OSS_DENYLIST_FILE || path.join(ROOT, DENYLIST_REL);
}

function printReport(byCategory, scanned, totalFindings) {
  process.stdout.write('\nOSS denylist scan (matches redacted by design)\n');
  process.stdout.write('---------------------------------------------\n');
  process.stdout.write(`scanned: ${scanned} files   findings: ${totalFindings}\n\n`);

  if (totalFindings === 0) {
    process.stdout.write('  clean — no org identifiers found.\n\n');
    return;
  }

  const rows = [...byCategory.entries()].sort((a, b) => b[1].hits - a[1].hits);
  process.stdout.write('  category                    hits   files\n');
  rows.forEach(([category, entry]) => {
    process.stdout.write(
      `  ${category.padEnd(26)} ${String(entry.hits).padStart(5)}   ${String(entry.files.size).padStart(5)}\n`,
    );
  });
  process.stdout.write(
    '\n  Locations are withheld from this summary on purpose. Re-run locally\n' +
      '  with --json to get path:line pairs (still without the matched text).\n\n',
  );
}

function main(argv, env = process.env) {
  const wantsJson = argv.includes('--json');
  const isGate = argv.includes('--gate');
  const isTree = argv.includes('--tree');
  const dirIdx = argv.indexOf('--dir');
  const dir = dirIdx !== -1 ? argv[dirIdx + 1] : '.';
  const absDir = path.isAbsolute(dir) ? dir : path.join(ROOT, dir);

  if (!fs.existsSync(absDir)) {
    process.stderr.write(
      `oss_scan: ${dir} does not exist.\n`,
    );
    return 2;
  }

  const listPath = denylistPath(env);
  if (!fs.existsSync(listPath)) {
    process.stderr.write(
      `oss_scan: no denylist at ${listPath}. Copy oss/oss_denylist.txt from the ` +
        'private notes repo there, or set OSS_DENYLIST_FILE to its path.\n',
    );
    return 2;
  }

  const rules = parseDenylist(fs.readFileSync(listPath, 'utf8'));
  const listed = isTree ? listTrackedFiles(absDir) : collectFiles(absDir);
  const files = listed.filter((f) => isScannable(f) && !isExempt(f));

  const findings = [];
  files.forEach((rel) => {
    let text;
    try {
      text = fs.readFileSync(path.join(absDir, rel), 'utf8');
    } catch {
      return; // unreadable / not valid UTF-8 — treated as binary
    }
    findings.push(...scanText(rel, text, rules));
  });

  const byCategory = summarize(findings);

  if (wantsJson) {
    process.stdout.write(
      `${JSON.stringify(
        {
          scanned: files.length,
          findings: findings.length,
          byCategory: Object.fromEntries(
            [...byCategory].map(([k, v]) => [k, { hits: v.hits, files: v.files.size }]),
          ),
          locations: findings.map((f) => `${f.file}:${f.line} [${f.category}]`),
        },
        null,
        2,
      )}\n`,
    );
  } else {
    printReport(byCategory, files.length, findings.length);
  }

  if (!isGate) return 0;

  // Gate on everything EXCEPT the categories the denylist marks
  // `!gate-accepted`. Those are hits we expect and have reasoned about; the
  // rest must stay at zero. Checked per category rather than as a total, so a
  // new leak cannot hide under an accepted category's count.
  const accepted = new Set(rules.filter((r) => r.gateAccepted).map((r) => r.category));
  const blocking = findings.filter((f) => !accepted.has(f.category));
  if (blocking.length === 0) {
    process.stdout.write(
      `\noss_scan: gate PASSED — ${findings.length} finding(s), all in ` +
        `gate-accepted categories (${[...accepted].sort().join(', ') || 'none'}).\n`,
    );
    return 0;
  }

  const blockingCategories = [...new Set(blocking.map((f) => f.category))].sort();
  const rerun = isTree
    ? 'node scripts/oss_scan.js --tree --report --json'
    : `node scripts/oss_scan.js --report --json --dir ${dir}`;
  process.stderr.write(
    `\noss_scan: gate FAILED — ${blocking.length} finding(s) in ` +
      `${blockingCategories.length} category/categories that must be empty:\n` +
      blockingCategories.map((c) => `    ${c}`).join('\n') +
      '\n\n  Locations are withheld here for the same reason the report withholds\n' +
      '  them — a CI log is a publication channel. Re-run locally:\n' +
      `    ${rerun}\n`,
  );
  return 1;
}

if (require.main === module) {
  process.exitCode = main(process.argv.slice(2));
}

module.exports = {
  denylistPath,
  parseDenylist,
  scanText,
  summarize,
  isExempt,
  isScannable,
};
