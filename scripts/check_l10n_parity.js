#!/usr/bin/env node
/**
 * check_l10n_parity.js — enforce key parity across ARB locale files.
 *
 * Treats app/lib/l10n/app_en.arb as the source of truth. Every other
 * app/lib/l10n/app_<locale>.arb must contain exactly the same set of
 * non-`@`-prefixed keys: no missing keys, no extras. ARB metadata blocks
 * (keys starting with `@`) are ignored — they are optional in
 * non-template locales by Flutter convention.
 *
 * Run via: npm run lint:dart:l10n-parity
 * Exit 0 = clean, Exit 1 = drift detected.
 *
 * Issue: #1972
 */
const fs = require('fs');
const path = require('path');

const L10N_DIR = path.join(__dirname, '..', 'app', 'lib', 'l10n');
const TEMPLATE = 'app_en.arb';

function loadArb(file) {
  const raw = fs.readFileSync(path.join(L10N_DIR, file), 'utf-8');
  return JSON.parse(raw);
}

function main() {
  const files = fs.readdirSync(L10N_DIR).filter(
    (f) => f.startsWith('app_') && f.endsWith('.arb'),
  );
  if (!files.includes(TEMPLATE)) {
    console.error(`Template ARB not found: ${TEMPLATE}`);
    process.exit(1);
  }

  const template = loadArb(TEMPLATE);
  const templateKeys = new Set(
    Object.keys(template).filter((k) => !k.startsWith('@')),
  );

  let drift = 0;
  for (const file of files) {
    if (file === TEMPLATE) continue;
    const locale = loadArb(file);
    const localeKeys = new Set(
      Object.keys(locale).filter((k) => !k.startsWith('@')),
    );
    const missing = [...templateKeys].filter((k) => !localeKeys.has(k)).sort();
    const extra = [...localeKeys].filter((k) => !templateKeys.has(k)).sort();
    if (missing.length === 0 && extra.length === 0) continue;
    drift += missing.length + extra.length;
    console.error(`\n${file}: ${missing.length} missing, ${extra.length} extra`);
    if (missing.length > 0) {
      console.error('  Missing keys (present in app_en.arb, absent here):');
      missing.forEach((k) => console.error(`    - ${k}`));
    }
    if (extra.length > 0) {
      console.error('  Extra keys (present here, absent in app_en.arb):');
      extra.forEach((k) => console.error(`    - ${k}`));
    }
  }

  if (drift > 0) {
    console.error(
      `\n${drift} key parity violation(s). Every key in app_en.arb must ` +
        `exist in every other locale ARB (English placeholders are fine ` +
        `when a real translation is not yet available — see ` +
        `app/lib/l10n/README.md).`,
    );
    process.exit(1);
  }
  console.log(`l10n parity OK across ${files.length} ARB file(s).`);
}

main();
