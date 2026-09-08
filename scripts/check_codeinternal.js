#!/usr/bin/env node
/**
 * check_codeinternal.js — block bare connect.NewError(connect.CodeInternal, ...)
 *
 * The bare pattern serializes the underlying error's message to the wire, leaking
 * Postgres constraint text, JSON decoder messages, third-party API bodies, and
 * similar low-signal noise. Use server/connecterr.Internal(ctx, op, err, kv...)
 * instead — it logs server-side and returns a generic public message.
 *
 * Behavior: walks server/ and fails if any non-test, non-generated, non-helper
 * file contains the bare pattern. The migration is complete (issue #1340), so
 * there is no allowlist — any new violation is a regression.
 *
 * Usage:
 *   node scripts/check_codeinternal.js
 */

const fs = require('fs');
const path = require('path');

const ROOT = path.join(__dirname, '..');
const SERVER_DIR = path.join(ROOT, 'server');

// Files matching these conditions are exempt from the rule entirely:
//  - the connecterr package itself (the only legitimate caller)
//  - generated code under server/gen/
//  - test files (they legitimately construct CodeInternal errors for assertions)
const EXEMPT_PREFIXES = [
  path.join(SERVER_DIR, 'connecterr') + path.sep,
  path.join(SERVER_DIR, 'gen') + path.sep,
];
const EXEMPT_SUFFIXES = ['_test.go'];

const BARE_PATTERN = /connect\.NewError\(\s*connect\.CodeInternal\b/g;

function isExempt(filePath) {
  if (EXEMPT_PREFIXES.some((p) => filePath.startsWith(p))) return true;
  if (EXEMPT_SUFFIXES.some((s) => filePath.endsWith(s))) return true;
  return false;
}

function walkGo(dir, out = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      walkGo(full, out);
    } else if (entry.isFile() && entry.name.endsWith('.go')) {
      out.push(full);
    }
  }
  return out;
}

function findViolations() {
  const violations = []; // [{path, line}]
  for (const file of walkGo(SERVER_DIR).filter((f) => !isExempt(f))) {
    const src = fs.readFileSync(file, 'utf-8');
    const lines = src.split('\n');
    for (let i = 0; i < lines.length; i++) {
      if (BARE_PATTERN.test(lines[i])) {
        violations.push({ path: path.relative(ROOT, file), line: i + 1 });
      }
      BARE_PATTERN.lastIndex = 0;
    }
  }
  return violations;
}

function main() {
  const violations = findViolations();
  if (violations.length === 0) {
    console.log('check_codeinternal passed: 0 violations.');
    return;
  }
  console.error(
    `check_codeinternal: ${violations.length} bare connect.NewError(connect.CodeInternal, ...) site(s) found:`,
  );
  for (const v of violations) {
    console.error(`  ${v.path}:${v.line}`);
  }
  console.error('');
  console.error(
    'Use server/connecterr.Internal(ctx, op, err, kv...) instead — it logs',
  );
  console.error('the wrapped error server-side and returns a generic public message.');
  process.exit(1);
}

main();
