#!/usr/bin/env node
/**
 * check_ephemeral_comments.js — block phase-number references in *.go and
 * *.dart source comments.
 *
 * CLAUDE.md ("Go Code Comments") forbids implementation-phase markers in source
 * comments: rollout phases (e.g. "Phase 3a", "Phase 1") rot the moment the
 * surrounding work ships. Issue and PR numbers (e.g. "#1157") are allowed —
 * they provide stable, retrievable context. This script enforces the phase-ref
 * rule mechanically. The cleanup is complete, so there is no allowlist — any
 * new violation is a regression.
 *
 * Scopes: app/lib/ and server/, *.go and *.dart files only.
 * Exclusions: /gen/ path segments, test files, generated Dart files,
 *             l10n localizations, and this script itself.
 *
 * Usage:
 *   node scripts/check_ephemeral_comments.js
 */

'use strict';

const fs = require('fs');
const path = require('path');

const ROOT = path.join(__dirname, '..');

const SCAN_DIRS = [
  path.join(ROOT, 'app', 'lib'),
  path.join(ROOT, 'server'),
];

// Path segments that make a file exempt.
const EXCLUDED_PATH_SEGMENTS = [
  path.sep + 'gen' + path.sep,
];

// Whole sub-trees that are excluded.
const EXCLUDED_PATH_PREFIXES = [];

// File-name suffixes that make a file exempt.
const EXCLUDED_SUFFIXES = [
  '_test.go',
  '_test.dart',
  '.g.dart',
  '.freezed.dart',
  '.mocks.dart',
];

// File base-name prefixes that make a file exempt.
const EXCLUDED_NAME_PREFIXES = ['app_localizations'];

// The script itself is exempt — its header prose legitimately mentions "Phase".
const THIS_SCRIPT = path.resolve(__filename);

// One violation pattern, matched only against comment text.
// phase-ref: \bPhase\s*\d+\b, case-insensitive.
const PHASE_RE = /\bphase\s*\d+\b/i;

// ---------------------------------------------------------------------------
// File collection
// ---------------------------------------------------------------------------

function isExcluded(filePath) {
  if (filePath === THIS_SCRIPT) return true;
  for (const prefix of EXCLUDED_PATH_PREFIXES) {
    if (filePath.startsWith(prefix)) return true;
  }
  for (const seg of EXCLUDED_PATH_SEGMENTS) {
    if (filePath.includes(seg)) return true;
  }
  const base = path.basename(filePath);
  for (const sfx of EXCLUDED_SUFFIXES) {
    if (base.endsWith(sfx)) return true;
  }
  for (const pfx of EXCLUDED_NAME_PREFIXES) {
    if (base.startsWith(pfx)) return true;
  }
  return false;
}

function walkFiles(dir, out = []) {
  if (!fs.existsSync(dir)) return out;
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      walkFiles(full, out);
    } else if (entry.isFile() && (entry.name.endsWith('.go') || entry.name.endsWith('.dart'))) {
      out.push(full);
    }
  }
  return out;
}

function collectFiles() {
  const files = [];
  for (const dir of SCAN_DIRS) {
    walkFiles(dir, files);
  }
  return files.filter((f) => !isExcluded(f));
}

// ---------------------------------------------------------------------------
// Comment extraction
//
// Strategy: extract the "comment portion" of each line, then check that
// portion for violations.  Block-comment state is tracked across lines.
//
// A line contributes comment text when:
//   1. The scanner is currently inside a /* … */ block (comment text = full line).
//   2. The line starts with ^\s*(//|///|/*) (comment text = full line).
//   3. The line contains an inline // after code (comment text = from // to EOL).
//
// We do NOT try to tokenize string literals inside comments; the plan notes that
// false positives in that edge case are acceptable and the fix (reword) is cheap.
// ---------------------------------------------------------------------------

/**
 * extractCommentPortion returns the comment text found on a line, or null if
 * the line has no comment content.  inBlockComment must be true if the scanner
 * is currently inside an open /* … * / block before this line is processed.
 */
function extractCommentPortion(line, inBlockComment) {
  if (inBlockComment) {
    // Inside a block comment: everything up to */ (or to EOL) is comment text.
    const close = line.indexOf('*/');
    return close >= 0 ? line.slice(0, close + 2) : line;
  }

  const trimmed = line.trimStart();

  // Full-line comment (///, //, /*).
  if (trimmed.startsWith('///') || trimmed.startsWith('//') || trimmed.startsWith('/*')) {
    return line;
  }

  // Inline comment: code ... // comment text
  const inlineIdx = line.indexOf('//');
  if (inlineIdx > 0) {
    return line.slice(inlineIdx);
  }

  // Inline block comment opening: code ... /* comment
  const blockIdx = line.indexOf('/*');
  if (blockIdx >= 0) {
    return line.slice(blockIdx);
  }

  return null;
}

/**
 * updateBlockCommentState returns the new inBlockComment state after processing
 * a line.
 */
function updateBlockCommentState(line, inBlockComment) {
  if (inBlockComment) {
    const close = line.indexOf('*/');
    if (close >= 0) {
      // Check if a new block comment opens after the close.
      const reopen = line.indexOf('/*', close + 2);
      return reopen >= 0 && line.indexOf('*/', reopen) < 0;
    }
    return true; // still inside
  }
  // Look for an opening /* that is not closed on the same line.
  const open = line.indexOf('/*');
  if (open >= 0) {
    const close = line.indexOf('*/', open + 2);
    return close < 0; // enters block comment only if not closed
  }
  return false;
}

// ---------------------------------------------------------------------------
// File scanning
// ---------------------------------------------------------------------------

function scanFile(filePath) {
  const src = fs.readFileSync(filePath, 'utf-8');
  const lines = src.split('\n');
  const relPath = path.relative(ROOT, filePath);
  const violations = [];
  let inBlockComment = false;

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const commentText = extractCommentPortion(line, inBlockComment);
    inBlockComment = updateBlockCommentState(line, inBlockComment);

    if (!commentText) continue;

    if (PHASE_RE.test(commentText)) {
      violations.push({ path: relPath, line: i + 1, reason: 'phase-ref', text: line.trim() });
    }
  }

  return violations;
}

// ---------------------------------------------------------------------------
// Self-tests — run before the file walk; exit non-zero on failure.
// ---------------------------------------------------------------------------

function selfTest(description, line, inBlock, expectedReasons) {
  const commentText = extractCommentPortion(line, inBlock);
  const found = [];
  if (commentText) {
    if (PHASE_RE.test(commentText)) found.push('phase-ref');
  }
  const expected = expectedReasons.slice().sort().join(',');
  const actual = found.slice().sort().join(',');
  if (expected !== actual) {
    console.error(`SELF-TEST FAILED: ${description}`);
    console.error(`  line:     "${line}"`);
    console.error(`  expected: [${expected || '(none)'}]`);
    console.error(`  got:      [${actual || '(none)'}]`);
    return false;
  }
  return true;
}

function runSelfTests() {
  let ok = true;

  // Positive cases — should flag.
  ok = selfTest('Go line comment with phase-ref', '  // Phase 2: foo bar', false, ['phase-ref']) && ok;
  ok = selfTest('Dart doc comment with phase-ref', '  /// Phase 2: foo bar', false, ['phase-ref']) && ok;
  ok = selfTest('TODO(phase1) inside comment', '  // TODO(phase1): bar', false, ['phase-ref']) && ok;
  ok = selfTest('phase-ref inside block comment', '  * Phase 2 of plan', true, ['phase-ref']) && ok;
  ok = selfTest('inline comment with phase-ref', '  x = 1; // Phase 2: set x', false, ['phase-ref']) && ok;
  ok = selfTest('lowercase phase-ref', '  // phase 1 is done', false, ['phase-ref']) && ok;

  // Negative cases — should NOT flag.
  ok = selfTest('issue-ref in comment not flagged', '  // see #1185', false, []) && ok;
  ok = selfTest('3-digit issue-ref not flagged', '  // see #853', false, []) && ok;
  ok = selfTest('hex color 6-digit no match', '  // color: #268080', false, []) && ok;
  ok = selfTest('hex color mixed alpha no match', '  // color: #786A50', false, []) && ok;
  ok = selfTest('phase without digit', '  // this is a phase of development', false, []) && ok;
  ok = selfTest('enum value name not in comment', '  LoanBorrowerPhase phase = x;', false, []) && ok;
  ok = selfTest('5-digit issue ref no match', '  // see #12345', false, []) && ok;
  ok = selfTest('non-comment line with Phase in name', '  enum LoanBorrowerPhase { a, b }', false, []) && ok;

  if (!ok) {
    console.error('Self-tests failed — fix the script before running against the tree.');
    process.exit(1);
  }
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

function main() {
  runSelfTests();

  const files = collectFiles();
  const allViolations = [];

  for (const file of files) {
    allViolations.push(...scanFile(file));
  }

  if (allViolations.length === 0) {
    console.log('check_ephemeral_comments passed: 0 violations.');
    return;
  }

  console.error(`check_ephemeral_comments: ${allViolations.length} violation(s) found:`);
  for (const v of allViolations) {
    console.error(`  ${v.path}:${v.line}:${v.reason} -- ${v.text}`);
  }
  console.error('');
  console.error('Implementation-phase markers in source comments rot when surrounding work ships.');
  console.error('Describe the durable invariant instead; put rollout-phase context in commit');
  console.error('messages, PR descriptions, or plan docs under docs/.');
  console.error('Issue and PR numbers (e.g. #1157) are allowed — they provide stable context.');
  process.exit(1);
}

main();
