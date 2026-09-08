#!/usr/bin/env node
/**
 * check_todo_refs.js — every TODO in source must reference a GitHub issue.
 *
 * CLAUDE.md ("TODO Comments") requires the form `TODO(#NNNN):`, forbids `FIXME`
 * outright, and reserves bare `TODO:` for in-PR scratch notes that must be
 * resolved or converted before merge. This gate is what makes that last part
 * true: an unresolved scratch note fails CI instead of quietly becoming
 * permanent. A TODO nobody is watching rots — the repo had 12 of them, some
 * years old, until #1611 cleared the backlog.
 *
 * Violations:
 *   - todo-no-issue   `TODO` without a `(#NNNN)` reference, including
 *                     non-numeric tags like `TODO(some-slug)`.
 *   - fixme           any `FIXME`. Standardize on `TODO(#NNNN):`.
 *
 * Scopes: app/lib/ and server/, *.go and *.dart. Unlike
 * check_ephemeral_comments.js this DOES scan test files — CLAUDE.md's rule has
 * no test exemption, and two of the twelve originals lived in integration tests.
 * Exclusions: /gen/ path segments, generated Dart, l10n, and this script plus
 * its test (their prose necessarily contains bare TODO/FIXME).
 *
 * Usage:
 *   node scripts/check_todo_refs.js
 *
 * The scanning core is pure and exported for check_todo_refs.test.js; only
 * collectFiles()/main() touch the filesystem.
 */

'use strict';

const fs = require('fs');
const path = require('path');

const ROOT = path.join(__dirname, '..');

const SCAN_DIRS = [path.join(ROOT, 'app', 'lib'), path.join(ROOT, 'server')];

// Path segments that make a file exempt.
const EXCLUDED_PATH_SEGMENTS = [path.sep + 'gen' + path.sep];

// File-name suffixes that make a file exempt (generated output only — test
// files are deliberately NOT exempt here).
const EXCLUDED_SUFFIXES = ['.g.dart', '.freezed.dart', '.mocks.dart', '.pb.go', '_connect.go'];

// File base-name prefixes that make a file exempt.
const EXCLUDED_NAME_PREFIXES = ['app_localizations'];

// This script and its test describe the rule, so their prose contains the very
// markers being banned.
const SELF_EXEMPT = new Set([
  path.resolve(__filename),
  path.resolve(__dirname, 'check_todo_refs.test.js'),
]);

// A TODO is compliant only when immediately followed by (#NNNN).
const TODO_RE = /\bTODO\b/;
const TODO_WITH_ISSUE_RE = /\bTODO\(#\d+\)/;
const FIXME_RE = /\bFIXME\b/;

// ---------------------------------------------------------------------------
// Comment extraction (same strategy as check_ephemeral_comments.js: find the
// comment portion of a line, tracking /* */ state across lines, then test only
// that portion so a TODO inside a string literal is not flagged).
// ---------------------------------------------------------------------------

function extractCommentPortion(line, inBlockComment) {
  if (inBlockComment) {
    const close = line.indexOf('*/');
    return close >= 0 ? line.slice(0, close + 2) : line;
  }

  const trimmed = line.trimStart();
  if (trimmed.startsWith('///') || trimmed.startsWith('//') || trimmed.startsWith('/*')) {
    return line;
  }

  const inlineIdx = line.indexOf('//');
  if (inlineIdx > 0) return line.slice(inlineIdx);

  const blockIdx = line.indexOf('/*');
  if (blockIdx >= 0) return line.slice(blockIdx);

  return null;
}

function updateBlockCommentState(line, inBlockComment) {
  if (inBlockComment) {
    const close = line.indexOf('*/');
    if (close >= 0) {
      const reopen = line.indexOf('/*', close + 2);
      return reopen >= 0 && line.indexOf('*/', reopen) < 0;
    }
    return true;
  }
  const open = line.indexOf('/*');
  if (open >= 0) return line.indexOf('*/', open + 2) < 0;
  return false;
}

/**
 * violationsForComment returns the reason codes a single comment string earns.
 * Pure — this is what the unit test drives.
 */
function violationsForComment(commentText) {
  const reasons = [];
  if (FIXME_RE.test(commentText)) reasons.push('fixme');
  if (TODO_RE.test(commentText) && !TODO_WITH_ISSUE_RE.test(commentText)) {
    reasons.push('todo-no-issue');
  }
  return reasons;
}

/**
 * scanSource scans one file's text and returns violations. Pure over its input,
 * so the test can exercise multi-line block-comment behaviour without touching
 * the filesystem.
 */
function scanSource(relPath, src) {
  const lines = src.split('\n');
  const violations = [];
  let inBlockComment = false;

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    const commentText = extractCommentPortion(line, inBlockComment);
    inBlockComment = updateBlockCommentState(line, inBlockComment);
    if (!commentText) continue;

    // A multi-line TODO's continuation lines carry no marker of their own, so
    // only the line bearing TODO/FIXME is reported — that is where the fix goes.
    for (const reason of violationsForComment(commentText)) {
      violations.push({ path: relPath, line: i + 1, reason, text: line.trim() });
    }
  }

  return violations;
}

// ---------------------------------------------------------------------------
// File collection
// ---------------------------------------------------------------------------

function isExcluded(filePath) {
  if (SELF_EXEMPT.has(filePath)) return true;
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
  for (const dir of SCAN_DIRS) walkFiles(dir, files);
  return files.filter((f) => !isExcluded(f));
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

function main() {
  const files = collectFiles();
  const allViolations = [];
  for (const file of files) {
    allViolations.push(...scanSource(path.relative(ROOT, file), fs.readFileSync(file, 'utf-8')));
  }

  if (allViolations.length === 0) {
    console.log(`check_todo_refs passed: ${files.length} file(s) scanned, 0 violations.`);
    return;
  }

  console.error(`check_todo_refs: ${allViolations.length} violation(s) found:`);
  for (const v of allViolations) {
    console.error(`  ${v.path}:${v.line}:${v.reason} -- ${v.text}`);
  }
  console.error('');
  console.error('Every TODO must reference an open issue: TODO(#NNNN): <what and why>.');
  console.error('FIXME is not used — standardize on TODO(#NNNN).');
  console.error('');
  console.error('If no issue exists, file one first (gh issue create); the issue is where the');
  console.error('context lives. The TODO body itself must still say what the code does today,');
  console.error('what would let the TODO be removed, and what blocked doing it now — an');
  console.error('issue-only TODO is not enough. See CLAUDE.md "TODO Comments".');
  console.error('');
  console.error('A bare TODO: is an in-PR scratch note. Resolve it or convert it before merge.');
  process.exit(1);
}

if (require.main === module) {
  main();
}

module.exports = {
  extractCommentPortion,
  updateBlockCommentState,
  violationsForComment,
  scanSource,
  isExcluded,
};
