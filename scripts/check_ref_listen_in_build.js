#!/usr/bin/env node
/**
 * check_ref_listen_in_build.js — block ref.listen() calls inside Widget build()
 *
 * ref.listen() inside Widget build() re-registers a fresh closure on every
 * rebuild, stacking callbacks and causing side-effect handlers (lockNav,
 * unlockNav, hideNav, showNav, etc.) to fire multiple times per state change.
 * Use ref.listenManual() registered in initState instead.
 *
 * Behavior: walks app/lib/, detects ref.listen() calls inside any
 * `Widget build(BuildContext ...)` method body, and compares against
 * scripts/ref_listen_allowlist.txt. New violations (not in the allowlist)
 * cause exit 1. The allowlist can shrink but never grow on main — CI enforces
 * this ratchet.
 *
 * Usage:
 *   node scripts/check_ref_listen_in_build.js
 *   node scripts/check_ref_listen_in_build.js --update-baseline  # rewrite allowlist
 */

const fs = require('fs');
const path = require('path');

const ROOT = path.join(__dirname, '..');
const APP_LIB_DIR = path.join(ROOT, 'app', 'lib');
const ALLOWLIST_PATH = path.join(__dirname, 'ref_listen_allowlist.txt');

const EXEMPT_SUFFIXES = ['.g.dart', '.freezed.dart'];

function isExempt(filePath) {
  return EXEMPT_SUFFIXES.some((s) => filePath.endsWith(s));
}

function walkDart(dir, out = []) {
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

// Strip // and /* */ comments and string literals, preserving newlines so
// that reported line numbers match the original file.
function stripCommentsAndStrings(src) {
  const out = [];
  let i = 0;
  let inLineComment = false;
  let inBlockComment = false;
  let inString = null; // null | "'" | '"'
  let stringIsTriple = false;
  while (i < src.length) {
    const c = src[i];
    const c2 = src[i + 1];
    if (inLineComment) {
      if (c === '\n') {
        inLineComment = false;
        out.push(c);
      }
      i++;
      continue;
    }
    if (inBlockComment) {
      if (c === '\n') out.push(c);
      if (c === '*' && c2 === '/') {
        inBlockComment = false;
        i += 2;
        continue;
      }
      i++;
      continue;
    }
    if (inString) {
      if (c === '\n' && !stringIsTriple) {
        out.push(inString);
        out.push(c);
        inString = null;
        i++;
        continue;
      }
      if (c === '\n') {
        out.push(c);
        i++;
        continue;
      }
      if (c === '\\' && c2 != null) {
        i += 2;
        continue;
      }
      if (stringIsTriple) {
        if (c === inString && src[i + 1] === inString && src[i + 2] === inString) {
          out.push(inString);
          inString = null;
          stringIsTriple = false;
          i += 3;
          continue;
        }
      } else if (c === inString) {
        out.push(inString);
        inString = null;
        i++;
        continue;
      }
      out.push('_');
      i++;
      continue;
    }
    if (c === '/' && c2 === '/') {
      inLineComment = true;
      i += 2;
      continue;
    }
    if (c === '/' && c2 === '*') {
      inBlockComment = true;
      i += 2;
      continue;
    }
    if (c === "'" || c === '"') {
      if (src[i + 1] === c && src[i + 2] === c) {
        inString = c;
        stringIsTriple = true;
        out.push(c);
        i += 3;
        continue;
      }
      inString = c;
      stringIsTriple = false;
      out.push(c);
      i++;
      continue;
    }
    out.push(c);
    i++;
  }
  return out.join('');
}

// Returns an array of [start, end) character offsets in `stripped` that
// correspond to the body of each `Widget build(BuildContext ...)` method
// (body = everything between the opening and closing brace, inclusive).
function findBuildMethodRanges(stripped) {
  const ranges = [];
  const BUILD_RE = /\bWidget\s+build\s*\(/g;
  let m;
  while ((m = BUILD_RE.exec(stripped)) !== null) {
    // Skip past the parameter list `(...)`.
    let i = m.index + m[0].length - 1; // points at the opening (
    let depth = 1;
    i++;
    while (i < stripped.length && depth > 0) {
      if (stripped[i] === '(') depth++;
      else if (stripped[i] === ')') depth--;
      i++;
    }
    // Skip whitespace / any `async` keyword to find the `{`.
    while (i < stripped.length && stripped[i] !== '{' && stripped[i] !== ';') {
      i++;
    }
    if (i >= stripped.length || stripped[i] !== '{') {
      // Abstract or forward declaration — no body.
      continue;
    }
    const bodyStart = i;
    let braceDepth = 1;
    i++;
    while (i < stripped.length && braceDepth > 0) {
      if (stripped[i] === '{') braceDepth++;
      else if (stripped[i] === '}') braceDepth--;
      i++;
    }
    ranges.push([bodyStart, i]);
  }
  return ranges;
}

function findViolations(filePath) {
  const src = fs.readFileSync(filePath, 'utf-8');
  const stripped = stripCommentsAndStrings(src);
  const ranges = findBuildMethodRanges(stripped);
  if (ranges.length === 0) return [];

  const violations = [];
  const LISTEN_RE = /\bref\.listen\s*\(/g;
  for (const [start, end] of ranges) {
    const body = stripped.slice(start, end);
    let lm;
    LISTEN_RE.lastIndex = 0;
    while ((lm = LISTEN_RE.exec(body)) !== null) {
      // Map offset back to line number in the original file.
      const absOffset = start + lm.index;
      const line = stripped.slice(0, absOffset).split('\n').length;
      violations.push({ file: filePath, line });
    }
  }
  return violations;
}

// Allowlist fingerprint: just the file path relative to ROOT, since we
// exempt entire files from this rule (each file should have at most one
// tracked legacy violation, and the fix is always "move to initState").
function fingerprint(violation) {
  return path.relative(ROOT, violation.file);
}

function loadAllowlist() {
  if (!fs.existsSync(ALLOWLIST_PATH)) return new Set();
  const lines = fs.readFileSync(ALLOWLIST_PATH, 'utf-8').split('\n');
  const set = new Set();
  for (const raw of lines) {
    const line = raw.replace(/#.*$/, '').trim();
    if (line) set.add(line);
  }
  return set;
}

function writeAllowlist(fingerprints) {
  const sorted = Array.from(fingerprints).sort();
  const header = [
    '# ref.listen-in-build allowlist.',
    '#',
    '# Each entry is a file path (relative to repo root) that contains a',
    '# legacy ref.listen() call inside Widget build(). The allowlist can',
    '# SHRINK but never GROW on main — CI enforces this ratchet.',
    '#',
    '# Fix: move ref.listen() to ref.listenManual() registered in initState.',
    '#',
    '# Regenerate from current state with:',
    '#   node scripts/check_ref_listen_in_build.js --update-baseline',
    '',
  ].join('\n');
  fs.writeFileSync(ALLOWLIST_PATH, header + sorted.join('\n') + '\n');
}

function main() {
  const args = process.argv.slice(2);
  const updateBaseline = args.includes('--update-baseline');

  const files = walkDart(APP_LIB_DIR).filter((f) => !isExempt(f));
  const allViolations = [];
  for (const file of files) {
    allViolations.push(...findViolations(file));
  }

  const currentFingerprints = new Set(allViolations.map(fingerprint));

  if (updateBaseline) {
    writeAllowlist(currentFingerprints);
    console.log(
      `Wrote ${currentFingerprints.size} fingerprint(s) to ${path.relative(ROOT, ALLOWLIST_PATH)}.`,
    );
    return;
  }

  const allowlist = loadAllowlist();
  const newViolations = allViolations.filter((v) => !allowlist.has(fingerprint(v)));
  const stale = Array.from(allowlist).filter((fp) => !currentFingerprints.has(fp));

  if (newViolations.length > 0) {
    console.error(
      `check_ref_listen_in_build: ${newViolations.length} new ref.listen() call(s) inside Widget build() found:\n`,
    );
    for (const v of newViolations) {
      console.error(`  ${path.relative(ROOT, v.file)}:${v.line}`);
    }
    console.error(
      '\nUse ref.listenManual() registered in initState instead (same pattern as',
    );
    console.error(
      'selectedCommunityProvider and contentCacheInvalidationProvider listeners).',
    );
    process.exit(1);
  }

  if (stale.length > 0) {
    console.warn(
      `${stale.length} stale allowlist entr${stale.length === 1 ? 'y' : 'ies'} (violation no longer present — allowlist can be tightened):`,
    );
    for (const fp of stale) console.warn(`  ${fp}`);
    console.warn(
      '\nRun: node scripts/check_ref_listen_in_build.js --update-baseline  to refresh.',
    );
  }

  console.log(
    `check_ref_listen_in_build passed: ${allViolations.length} violation(s), all in allowlist.`,
  );
}

main();
