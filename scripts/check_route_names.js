#!/usr/bin/env node
/**
 * check_route_names.js — enforce name: on every GoRoute declaration.
 *
 * Every GoRoute() in app/lib/ must include a name: argument so that
 * AnalyticsRouteObserver._extractScreenName uses the explicit name
 * instead of falling back to the route's runtime type string (#1809).
 *
 * Behavior: walks app/lib/, matches GoRoute( declarations, verifies that
 * the argument list (parenthesis-balanced) contains name:. Redirect-only
 * routes (those with only a redirect: argument and no builder:/pageBuilder:)
 * are still required to have name: to keep the convention consistent.
 *
 * Compares violations against scripts/route_names_allowlist.txt.
 * New violations cause exit 1. The allowlist can only shrink on main.
 *
 * Usage:
 *   node scripts/check_route_names.js
 *   node scripts/check_route_names.js --update-baseline
 */

const fs = require('fs');
const path = require('path');

const ROOT = path.join(__dirname, '..');
const APP_LIB_DIR = path.join(ROOT, 'app', 'lib');
const ALLOWLIST_PATH = path.join(__dirname, 'route_names_allowlist.txt');

const EXEMPT_SUFFIXES = ['.g.dart', '.freezed.dart'];
// Exempt generated code directories
const EXEMPT_DIR_SEGMENTS = [path.sep + 'gen' + path.sep, path.sep + 'gen' + path.sep];

function isExempt(filePath) {
  if (EXEMPT_SUFFIXES.some((s) => filePath.endsWith(s))) return true;
  const rel = path.relative(APP_LIB_DIR, filePath);
  if (rel.startsWith('gen' + path.sep) || rel.startsWith('data' + path.sep + 'gen' + path.sep)) return true;
  return false;
}

function walkDart(dir, out = []) {
  if (!fs.existsSync(dir)) return out;
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      walkDart(full, out);
    } else if (entry.isFile() && entry.name.endsWith('.dart')) {
      out.push(full);
    }
  }
  return out;
}

function stripComments(src) {
  const out = [];
  let i = 0;
  while (i < src.length) {
    if (src[i] === '/' && src[i + 1] === '/') {
      while (i < src.length && src[i] !== '\n') i++;
      continue;
    }
    if (src[i] === '/' && src[i + 1] === '*') {
      i += 2;
      while (i < src.length && !(src[i] === '*' && src[i + 1] === '/')) {
        if (src[i] === '\n') out.push('\n');
        i++;
      }
      i += 2;
      continue;
    }
    out.push(src[i]);
    i++;
  }
  return out.join('');
}

function extractArgs(src, openParenIdx) {
  let depth = 1;
  let i = openParenIdx + 1;
  while (i < src.length && depth > 0) {
    const c = src[i];
    if (c === '(') depth++;
    else if (c === ')') depth--;
    i++;
  }
  return src.slice(openParenIdx + 1, i - 1);
}

function lineOf(src, idx) {
  return src.slice(0, idx).split('\n').length;
}

function findViolations(filePath) {
  const src = fs.readFileSync(filePath, 'utf8');
  const stripped = stripComments(src);
  const violations = [];
  const needle = 'GoRoute(';
  let searchFrom = 0;
  while (true) {
    const callIdx = stripped.indexOf(needle, searchFrom);
    if (callIdx === -1) break;
    // Make sure GoRoute is a standalone token (not part of a longer identifier)
    const before = callIdx > 0 ? stripped[callIdx - 1] : ' ';
    if (/[a-zA-Z0-9_]/.test(before)) {
      searchFrom = callIdx + 1;
      continue;
    }
    const openParen = callIdx + needle.length - 1;
    const args = extractArgs(stripped, openParen);
    if (!args.includes('name:')) {
      const line = lineOf(stripped, callIdx);
      const rel = path.relative(ROOT, filePath);
      violations.push(`${rel}:${line}`);
    }
    searchFrom = openParen + 1;
  }
  return violations;
}

function loadAllowlist() {
  if (!fs.existsSync(ALLOWLIST_PATH)) return new Set();
  return new Set(
    fs.readFileSync(ALLOWLIST_PATH, 'utf8')
      .split('\n')
      .map((l) => l.trim())
      .filter((l) => l && !l.startsWith('#')),
  );
}

function writeAllowlist(entries) {
  const header = `# route-names allowlist.
#
# Each entry is a file:line path that contains a GoRoute() declaration
# without a name: argument. The allowlist can SHRINK but never GROW on
# main — CI enforces this ratchet.
#
# Fix: add name: '<screen_name>' to each listed GoRoute, using the
# conceptual screen name in snake_case (e.g. name: 'gear_detail').
#
# Regenerate from current state with:
#   node scripts/check_route_names.js --update-baseline\n`;
  fs.writeFileSync(ALLOWLIST_PATH, header + [...entries].sort().join('\n') + '\n');
}

const updateBaseline = process.argv.includes('--update-baseline');

const files = walkDart(APP_LIB_DIR).filter((f) => !isExempt(f));
const allViolations = [];
for (const f of files) {
  allViolations.push(...findViolations(f));
}

if (updateBaseline) {
  writeAllowlist(allViolations);
  console.log(`Wrote ${allViolations.length} entries to ${path.relative(ROOT, ALLOWLIST_PATH)}`);
  process.exit(0);
}

const allowlist = loadAllowlist();
const newViolations = allViolations.filter((v) => !allowlist.has(v));
const fixedEntries = [...allowlist].filter((e) => !allViolations.includes(e));

if (fixedEntries.length > 0) {
  console.log(`\nFixed entries (can be removed from allowlist):`);
  for (const e of fixedEntries) {
    console.log(`  ${e}`);
  }
}

if (newViolations.length > 0) {
  console.error(`\nError: ${newViolations.length} GoRoute() declaration(s) missing name::`);
  for (const v of newViolations) {
    console.error(`  ${v}`);
  }
  console.error(`\nFix: add name: '<screen_name>' to each GoRoute, using snake_case.`);
  console.error(`Example: name: 'gear_detail' for /gear/:id`);
  process.exit(1);
}

console.log(`route-names: ${allViolations.length} in allowlist, 0 new violations.`);
