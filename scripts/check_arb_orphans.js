#!/usr/bin/env node
/**
 * check_arb_orphans.js — fail when an ARB message key is never referenced.
 *
 * `flutter gen-l10n` turns every key in app/lib/l10n/app_en.arb into a getter
 * or method on AppLocalizations. Nothing warns when the last caller of one goes
 * away, so retired screens leave their copy behind — and translators keep
 * paying for it in every locale.
 *
 * A key counts as referenced if its exact name appears as an identifier-shaped
 * word anywhere under app/lib/ or app/test/, excluding generated output (the
 * l10n classes themselves and lib/data/gen/). That is deliberately generous:
 * a mention in a comment keeps a key alive, because erring toward "used" only
 * costs a stale string, while erring toward "unused" deletes shipped copy.
 *
 * Escape hatch: scripts/arb_orphan_allowlist.txt, one key per line. It may
 * SHRINK but never GROW — adding an entry requires justification in review.
 *
 * Run via:
 *   npm run lint:dart:arb-orphans          # check, exit 1 on orphans
 *   node scripts/check_arb_orphans.js --list    # print orphans, exit 0
 */
const fs = require('fs');
const path = require('path');

const ROOT = path.join(__dirname, '..');
const ARB = path.join(ROOT, 'app', 'lib', 'l10n', 'app_en.arb');
const ALLOWLIST = path.join(__dirname, 'arb_orphan_allowlist.txt');
const SCAN_ROOTS = [
  path.join(ROOT, 'app', 'lib'),
  path.join(ROOT, 'app', 'test'),
];
// Generated from the ARB itself, or from protos — a key "appearing" in these
// is not a use, it is the generator echoing the key back.
const EXCLUDED_DIRS = [
  path.join(ROOT, 'app', 'lib', 'l10n'),
  path.join(ROOT, 'app', 'lib', 'data', 'gen'),
];

const IDENTIFIER = /[A-Za-z_$][A-Za-z0-9_$]*/g;

function walk(dir, out) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (EXCLUDED_DIRS.some((d) => full === d || full.startsWith(d + path.sep))) {
        continue;
      }
      walk(full, out);
    } else if (entry.name.endsWith('.dart')) {
      out.push(full);
    }
  }
  return out;
}

function loadAllowlist() {
  if (!fs.existsSync(ALLOWLIST)) return new Set();
  return new Set(
    fs
      .readFileSync(ALLOWLIST, 'utf-8')
      .split('\n')
      .map((l) => l.trim())
      .filter((l) => l && !l.startsWith('#')),
  );
}

function main() {
  const listOnly = process.argv.includes('--list');

  const arb = JSON.parse(fs.readFileSync(ARB, 'utf-8'));
  const keys = Object.keys(arb).filter((k) => !k.startsWith('@'));

  const used = new Set();
  const files = SCAN_ROOTS.filter(fs.existsSync).reduce(
    (acc, root) => walk(root, acc),
    [],
  );
  for (const file of files) {
    const src = fs.readFileSync(file, 'utf-8');
    for (const m of src.matchAll(IDENTIFIER)) used.add(m[0]);
  }

  const allowlist = loadAllowlist();
  const orphans = keys.filter((k) => !used.has(k) && !allowlist.has(k));

  if (listOnly) {
    orphans.forEach((k) => console.log(k));
    process.exit(0);
  }

  // A stale allowlist entry is drift too — the ratchet only holds if entries
  // disappear once their key is either used or deleted.
  const stale = [...allowlist].filter(
    (k) => !keys.includes(k) || used.has(k),
  );

  if (orphans.length === 0 && stale.length === 0) {
    console.log(
      `arb orphans OK: all ${keys.length} keys in app_en.arb are referenced.`,
    );
    process.exit(0);
  }

  if (orphans.length) {
    console.error(
      `\n${orphans.length} ARB key(s) in app_en.arb are never referenced:\n`,
    );
    orphans.forEach((k) => console.error(`  ${k}`));
    console.error(
      '\nDelete them from every app/lib/l10n/app_*.arb (with their "@key"' +
        '\nmetadata block), then re-run `flutter gen-l10n`. If a key really is' +
        `\nreachable in a way this check cannot see, add it to\n  ${path.relative(ROOT, ALLOWLIST)}\n`,
    );
  }
  if (stale.length) {
    console.error(
      `\n${stale.length} allowlist entr(ies) are no longer needed — remove them:\n`,
    );
    stale.forEach((k) => console.error(`  ${k}`));
  }
  process.exit(1);
}

main();
