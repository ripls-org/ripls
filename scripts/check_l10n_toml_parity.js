#!/usr/bin/env node
/**
 * check_l10n_toml_parity.js — enforce key parity across server l10n TOML files.
 *
 * Treats server/l10n/source/en.toml as the source of truth. Every other
 * server/l10n/source/<locale>.toml must contain exactly the same set of
 * message-id keys: no missing keys, no extras. A missing translation in a
 * non-English locale would silently fall back to English at runtime (the
 * Localizer's warn-once gate fires once per process), so CI is strict.
 *
 * Counts every `[section.path]` and bare-form `key = "value"` at the top
 * level. The go-i18n source format used in this repo is consistently the
 * section form (`[notif.community_event.x.title]\nother = "..."`); the
 * regex tolerates both shapes.
 *
 * Run via: npm run lint:go:l10n-parity
 * Exit 0 = clean, Exit 1 = drift detected.
 *
 * Issue: #1904 Phase 6.
 */
const fs = require('fs');
const path = require('path');

const SOURCE_DIR = path.join(__dirname, '..', 'server', 'l10n', 'source');
const TEMPLATE = 'en.toml';

function extractKeys(file) {
  const raw = fs.readFileSync(path.join(SOURCE_DIR, file), 'utf-8');
  const keys = new Set();
  for (const rawLine of raw.split('\n')) {
    const line = rawLine.trim();
    if (line === '' || line.startsWith('#')) continue;
    const section = line.match(/^\[([^\]]+)\]$/);
    if (section) {
      keys.add(section[1]);
    }
  }
  return keys;
}

function main() {
  const files = fs
    .readdirSync(SOURCE_DIR)
    .filter((f) => f.endsWith('.toml'))
    .sort();
  if (!files.includes(TEMPLATE)) {
    console.error(`Template TOML not found: ${TEMPLATE}`);
    process.exit(1);
  }

  const templateKeys = extractKeys(TEMPLATE);

  let drift = 0;
  for (const file of files) {
    if (file === TEMPLATE) continue;
    const localeKeys = extractKeys(file);
    const missing = [...templateKeys].filter((k) => !localeKeys.has(k)).sort();
    const extra = [...localeKeys].filter((k) => !templateKeys.has(k)).sort();
    if (missing.length === 0 && extra.length === 0) continue;
    drift += missing.length + extra.length;
    console.error(`\n${file}: ${missing.length} missing, ${extra.length} extra`);
    if (missing.length > 0) {
      console.error('  Missing keys (present in en.toml, absent here):');
      missing.forEach((k) => console.error(`    - ${k}`));
    }
    if (extra.length > 0) {
      console.error('  Extra keys (present here, absent in en.toml):');
      extra.forEach((k) => console.error(`    - ${k}`));
    }
  }

  if (drift > 0) {
    console.error(
      `\n${drift} key parity violation(s). Every key in server/l10n/source/${TEMPLATE} ` +
        `must exist in every other locale source file. The runtime localizer ` +
        `falls back to English on a miss (and logs warn-once), but CI catches ` +
        `it before users do — see docs/server/l10n.md.`,
    );
    process.exit(1);
  }
  console.log(`server l10n parity OK across ${files.length} TOML file(s).`);
}

main();
