#!/usr/bin/env node
/**
 * check_i18n.js — detects hardcoded user-facing strings in the Flutter
 * presentation layer that should be replaced with context.l10n.* calls.
 *
 * Run via: npm run lint:dart:i18n
 * Exit 0 = clean, Exit 1 = violations found.
 */
const { execSync } = require('child_process');
const path = require('path');

const presentationDir = path.join(__dirname, '..', 'app', 'lib', 'presentation');

let violations = 0;
let output;

try {
  output = execSync(
    `grep -rn --include="*.dart" -E ` +
    `"Text\\('(Cancel|Save|Delete|Done|Close|Retry|Submit|Continue|Confirm)'\\)|` +
    `tooltip: '(Settings|Logout|Delete|Search|Cancel)'" ` +
    `"${presentationDir}"`,
    { encoding: 'utf-8', stdio: ['pipe', 'pipe', 'pipe'] },
  ).trim();
} catch (e) {
  // grep exits 1 when no matches found — that means we're clean.
  if (e.status === 1) {
    console.log('No hardcoded i18n violations found.');
    process.exit(0);
  }
  output = e.stdout ? e.stdout.trim() : '';
}

if (output) {
  // Filter out lines that are already using l10n or are comments.
  const lines = output.split('\n').filter((line) => {
    return (
      line &&
      !line.includes('l10n.') &&
      !line.includes('//') &&
      !line.includes('* ') &&
      !line.includes('test')
    );
  });

  if (lines.length > 0) {
    console.error(
      'Hardcoded i18n strings found (use context.l10n.* instead):',
    );
    lines.forEach((line) => console.error(' ', line));
    violations = lines.length;
  }
}

if (violations > 0) {
  console.error(
    `\n${violations} violation(s) found. Replace hardcoded strings with context.l10n.* calls.`,
  );
  process.exit(1);
} else {
  console.log('No hardcoded i18n violations found.');
  process.exit(0);
}
