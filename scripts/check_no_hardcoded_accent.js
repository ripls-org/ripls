#!/usr/bin/env node
/**
 * check_no_hardcoded_accent.js — block hardcoded accent / coral / sage color
 * references in *.dart sources outside app/lib/core/theme/.
 *
 * After #1946 the app's accent color became Heritage Sage (#5B8268). Every
 * accent-tinted pixel must resolve through `AppColors` so the next reskin is
 * a one-file change. This script catches:
 *
 *   - Material color shortcuts that bypass AppColors:
 *       Colors.orange, Colors.deepOrange, Colors.amber
 *
 *   - Direct hex literals for the coral palette (pre-migration values) and
 *     the new sage palette (current AppColors values) — using either set as
 *     a literal bypasses the theme indirection:
 *       coral: 0xFFE07A5F, 0xFFE89B7E, 0xFFD4714E, 0xFFD97052, 0xFFD35E3A,
 *              0xFFE8956F, 0xFFB35A3B, 0xFFFF8A80
 *       sage:  0xFF5B8268, 0xFF7BA088, 0xFF9DBFA8, 0xFFDDE6DC
 *
 * Scope: app/lib/, *.dart files only.
 * Exclusions: app/lib/core/theme/ (where the literals legitimately live),
 *             /gen/ path segments, generated Dart files, l10n localizations,
 *             test files, and this script itself.
 *
 * Allowlist: scripts/color_allowlist.txt — one entry per line, format
 *   "<rule_name> <relative_path_from_repo_root>". The list is a CI-enforced
 *   ratchet: entries may shrink but never grow on main. Currently empty.
 *
 * Usage:
 *   node scripts/check_no_hardcoded_accent.js
 *   node scripts/check_no_hardcoded_accent.js --update-baseline  # regenerate allowlist
 */

'use strict';

const fs = require('fs');
const path = require('path');

const ROOT = path.join(__dirname, '..');

const SCAN_DIRS = [path.join(ROOT, 'app', 'lib')];

const EXCLUDED_PATH_SEGMENTS = [
  path.sep + 'gen' + path.sep,
  path.sep + 'core' + path.sep + 'theme' + path.sep,
];

const EXCLUDED_SUFFIXES = [
  '_test.dart',
  '.g.dart',
  '.freezed.dart',
  '.mocks.dart',
];

const EXCLUDED_NAME_PREFIXES = ['app_localizations'];

const THIS_SCRIPT = path.resolve(__filename);

const ALLOWLIST_PATH = path.join(__dirname, 'color_allowlist.txt');

// Word-boundary match so Colors.orange[700] / Colors.orange.shade400 are
// caught but Colors.orangeRed (not a real color, hypothetically) is not.
const MATERIAL_COLOR_RE = /\bColors\.(orange|deepOrange|amber)\b/;

// Hex literals to forbid outside the theme directory. Case-insensitive on the
// hex digits. Order matters only for diagnostics; all patterns are checked.
const FORBIDDEN_HEX = [
  // Pre-migration coral palette.
  '0xFFE07A5F',
  '0xFFE89B7E',
  '0xFFD4714E',
  '0xFFD97052',
  '0xFFD35E3A',
  '0xFFE8956F',
  '0xFFB35A3B',
  '0xFFFF8A80',
  // Post-migration sage palette — listed here so widgets cannot bypass
  // AppColors by hardcoding the new accent values.
  '0xFF5B8268',
  '0xFF7BA088',
  '0xFF9DBFA8',
  '0xFFDDE6DC',
  // Ink & Sage token values (#2441, design/tokens.json) — brand, neutral
  // ramp, and per-theme status colors. Hardcoding any of these bypasses
  // DesignTokens/AppColors.
  '0xFF3E5A47',
  '0xFF2F4636',
  '0xFFB1CFBA',
  '0xFF141714',
  '0xFF1F2421',
  '0xFF323832',
  '0xFFF2F2EE',
  '0xFFDDDDD4',
  '0xFF2E7041',
  '0xFF8A6200',
  '0xFFB5492B',
  '0xFF3E6471',
  '0xFFDB7F63',
  '0xFF7FA0A9',
];
const HEX_RE = new RegExp(
  '(?:' + FORBIDDEN_HEX.map((h) => h.replace(/0x/i, '0[xX]')).join('|') + ')',
  'i',
);

// Font-family string literals outside the theme directory bypass the token
// fonts (DesignTokens.serifFamily / AppTheme.headingFont — #2441). Detection
// rides on the string stripper: in stripped code a literal-valued fontFamily
// is left with an EMPTY value (`fontFamily: ,`), while a constant-valued one
// keeps its identifier (`fontFamily: AppTheme.headingFont,`).
const FONT_LITERAL_RE = /\bfontFamily:\s*(?:,|\)|$)/;

// ---------------------------------------------------------------------------
// File collection
// ---------------------------------------------------------------------------

function isExcluded(filePath) {
  if (filePath === THIS_SCRIPT) return true;
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
    } else if (entry.isFile() && entry.name.endsWith('.dart')) {
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
// Comment + string stripping
//
// To avoid false positives on "orange" appearing inside a doc comment or a
// string literal, strip comments and string content before applying regexes.
// Block-comment state is tracked across lines.
// ---------------------------------------------------------------------------

function stripCommentsAndStrings(line, inBlockComment) {
  let out = '';
  let i = 0;
  let inBlock = inBlockComment;
  let inSingle = false;
  let inDouble = false;
  let inRawSingle = false;
  let inRawDouble = false;

  while (i < line.length) {
    const c = line[i];
    const next = line[i + 1];

    if (inBlock) {
      if (c === '*' && next === '/') {
        inBlock = false;
        i += 2;
        continue;
      }
      i++;
      continue;
    }

    if (inSingle || inRawSingle) {
      if (!inRawSingle && c === '\\' && i + 1 < line.length) {
        i += 2;
        continue;
      }
      if (c === "'") {
        inSingle = false;
        inRawSingle = false;
      }
      i++;
      continue;
    }

    if (inDouble || inRawDouble) {
      if (!inRawDouble && c === '\\' && i + 1 < line.length) {
        i += 2;
        continue;
      }
      if (c === '"') {
        inDouble = false;
        inRawDouble = false;
      }
      i++;
      continue;
    }

    // Outside comments and strings.
    if (c === '/' && next === '/') {
      // Line comment — rest of the line is comment.
      break;
    }
    if (c === '/' && next === '*') {
      inBlock = true;
      i += 2;
      continue;
    }
    if (c === "'") {
      inSingle = true;
      i++;
      continue;
    }
    if (c === '"') {
      inDouble = true;
      i++;
      continue;
    }
    // Raw strings r'...' / r"..."
    if (c === 'r' && (next === "'" || next === '"')) {
      if (next === "'") inRawSingle = true;
      else inRawDouble = true;
      i += 2;
      continue;
    }
    out += c;
    i++;
  }

  return { code: out, inBlockComment: inBlock };
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
    const { code, inBlockComment: nextBlock } = stripCommentsAndStrings(
      line,
      inBlockComment,
    );
    inBlockComment = nextBlock;

    if (!code) continue;

    if (MATERIAL_COLOR_RE.test(code)) {
      violations.push({
        path: relPath,
        line: i + 1,
        rule: 'no_material_orange',
        text: line.trim(),
      });
    }
    if (HEX_RE.test(code)) {
      violations.push({
        path: relPath,
        line: i + 1,
        rule: 'no_hardcoded_accent_hex',
        text: line.trim(),
      });
    }
    if (FONT_LITERAL_RE.test(code)) {
      violations.push({
        path: relPath,
        line: i + 1,
        rule: 'no_font_family_literal',
        text: line.trim(),
      });
    }
  }

  return violations;
}

// ---------------------------------------------------------------------------
// Allowlist (ratchet)
// ---------------------------------------------------------------------------

function loadAllowlist() {
  if (!fs.existsSync(ALLOWLIST_PATH)) return new Set();
  const raw = fs.readFileSync(ALLOWLIST_PATH, 'utf-8');
  const entries = new Set();
  for (const rawLine of raw.split('\n')) {
    const line = rawLine.trim();
    if (!line || line.startsWith('#')) continue;
    entries.add(line);
  }
  return entries;
}

function violationKey(v) {
  return `${v.rule} ${v.path}`;
}

function writeAllowlist(violations) {
  const keys = new Set(violations.map(violationKey));
  const sorted = [...keys].sort();
  const header = [
    '# scripts/color_allowlist.txt — ratchet for check_no_hardcoded_accent.js.',
    '# Format: <rule_name> <relative_path>. One file/rule pair per line.',
    '# May shrink but never grow on main. PRs adding entries must remove',
    '# an equal-or-greater number of entries elsewhere. See #1946 for context.',
    '',
  ].join('\n');
  fs.writeFileSync(ALLOWLIST_PATH, header + sorted.join('\n') + (sorted.length ? '\n' : ''));
}

// ---------------------------------------------------------------------------
// Self-tests
// ---------------------------------------------------------------------------

function selfTest(description, line, expectedRules) {
  const { code } = stripCommentsAndStrings(line, false);
  const found = [];
  if (code && MATERIAL_COLOR_RE.test(code)) found.push('no_material_orange');
  if (code && HEX_RE.test(code)) found.push('no_hardcoded_accent_hex');
  if (code && FONT_LITERAL_RE.test(code)) found.push('no_font_family_literal');
  const expected = expectedRules.slice().sort().join(',');
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

  // Positive cases.
  ok =
    selfTest('Colors.orange flagged', '  color: Colors.orange,', [
      'no_material_orange',
    ]) && ok;
  ok =
    selfTest('token hex 0xFF3E5A47 flagged', '  color: Color(0xFF3E5A47),', [
      'no_hardcoded_accent_hex',
    ]) && ok;
  ok =
    selfTest('fontFamily string literal flagged', "  fontFamily: 'Georgia',", [
      'no_font_family_literal',
    ]) && ok;
  ok =
    selfTest('fontFamily literal at line end flagged', "  fontFamily: 'Libre Baskerville'", [
      'no_font_family_literal',
    ]) && ok;
  ok =
    selfTest(
      'fontFamily via constant allowed',
      '  fontFamily: AppTheme.headingFont,',
      [],
    ) && ok;
  ok =
    selfTest(
      'fontFamily in comment ignored',
      "  // fontFamily: 'Georgia',",
      [],
    ) && ok;
  ok =
    selfTest('Colors.orange.shade400 flagged', '  return Colors.orange.shade400;', [
      'no_material_orange',
    ]) && ok;
  ok =
    selfTest('Colors.deepOrange flagged', '  color: Colors.deepOrange[700],', [
      'no_material_orange',
    ]) && ok;
  ok =
    selfTest('Colors.amber flagged', '  c = Colors.amber;', ['no_material_orange']) &&
    ok;
  ok =
    selfTest('coral hex 0xFFD4714E flagged', '  color: Color(0xFFD4714E),', [
      'no_hardcoded_accent_hex',
    ]) && ok;
  ok =
    selfTest('coral hex lowercase 0xffd4714e flagged', '  color: Color(0xffd4714e),', [
      'no_hardcoded_accent_hex',
    ]) && ok;
  ok =
    selfTest('sage hex 0xFF5B8268 flagged', '  color: Color(0xFF5B8268),', [
      'no_hardcoded_accent_hex',
    ]) && ok;
  // Negative cases — comments and strings should NOT trigger.
  ok =
    selfTest('// comment with Colors.orange not flagged', '  // use Colors.orange', []) &&
    ok;
  ok =
    selfTest('/// doc comment with hex not flagged', '  /// see Color(0xFFD4714E)', []) &&
    ok;
  ok =
    selfTest('string literal with hex not flagged', '  final s = "0xFFD4714E";', []) &&
    ok;
  ok =
    selfTest('string literal with Colors.orange not flagged', "  print('Colors.orange');", []) &&
    ok;
  ok =
    selfTest('non-forbidden color not flagged', '  color: Colors.red,', []) && ok;
  ok =
    selfTest('non-forbidden hex not flagged', '  color: Color(0xFF123456),', []) && ok;

  // Hex literal mixed with a non-forbidden Material color (Colors.red is
  // not on our list) — only the hex rule fires.
  ok =
    selfTest(
      'hex with non-forbidden Material color',
      '  [Color(0xFFE07A5F), Colors.red],',
      ['no_hardcoded_accent_hex'],
    ) && ok;

  return ok;
}

// ---------------------------------------------------------------------------
// Block-comment-aware self-test for stripCommentsAndStrings
// ---------------------------------------------------------------------------

function runBlockCommentTest() {
  const lines = [
    '/* a multi-line block comment',
    '   contains Colors.orange and 0xFFD4714E here',
    '   but should not flag */ color: Colors.red,',
  ];
  let inBlock = false;
  const found = [];
  for (let i = 0; i < lines.length; i++) {
    const { code, inBlockComment } = stripCommentsAndStrings(lines[i], inBlock);
    inBlock = inBlockComment;
    if (MATERIAL_COLOR_RE.test(code)) found.push(`line${i + 1}:material`);
    if (HEX_RE.test(code)) found.push(`line${i + 1}:hex`);
  }
  if (found.length !== 0) {
    console.error('SELF-TEST FAILED: block-comment stripping');
    console.error(`  found: ${found.join(', ')}`);
    return false;
  }
  return true;
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

function main() {
  if (!runSelfTests()) process.exit(2);
  if (!runBlockCommentTest()) process.exit(2);

  const files = collectFiles();
  const violations = [];
  for (const f of files) {
    for (const v of scanFile(f)) violations.push(v);
  }

  const args = process.argv.slice(2);
  if (args.includes('--update-baseline')) {
    writeAllowlist(violations);
    console.log(
      `Wrote ${path.relative(ROOT, ALLOWLIST_PATH)} with ${violations.length} entries.`,
    );
    return;
  }

  const allowlist = loadAllowlist();
  const newViolations = violations.filter((v) => !allowlist.has(violationKey(v)));

  if (newViolations.length === 0) {
    console.log('check_no_hardcoded_accent: OK');
    return;
  }

  console.error(`check_no_hardcoded_accent: ${newViolations.length} violation(s):`);
  for (const v of newViolations) {
    console.error(`  ${v.path}:${v.line}  [${v.rule}]  ${v.text}`);
  }
  console.error('');
  console.error(
    'Route the value through AppColors (see app/lib/core/theme/app_colors.dart).',
  );
  console.error('If a violation is truly unavoidable, regenerate the allowlist with');
  console.error(
    '  node scripts/check_no_hardcoded_accent.js --update-baseline',
  );
  console.error(
    'and remove an equal-or-greater number of entries elsewhere in the same PR.',
  );
  process.exit(1);
}

main();
