#!/usr/bin/env node
/**
 * check_web_colors.js — block raw color literals and off-token font families
 * in website content (#2441).
 *
 * design/tokens.json is the single source of truth for brand/neutral/semantic
 * colors and font families; the web consumes them via the generated
 * css/gen/tokens.gen.css (and the derived aliases in tokens.css /
 * marketing.css). This script catches web content bypassing the tokens:
 *
 *   no_raw_web_color   — hex (#RGB/#RRGGBB/#RRGGBBAA), rgb()/rgba(), hsl()
 *                        literals in CSS files, <style> blocks, inline
 *                        style="" attributes, and SVG fill=/stroke=
 *                        attributes. Pure black/white at any alpha is exempt
 *                        (overlays/scrims are functional, not palette).
 *
 *   no_off_token_font  — font-family declarations naming families outside
 *                        the token families + their declared fallback stacks.
 *
 * Scope: website/content/ — *.css and *.html.
 * Exclusions: css/gen/ (generated from tokens.json), css/fonts.css
 * (@font-face for the token families), choice-sheet/ (the #2441 decision
 * artifact renders candidate palettes by design).
 *
 * Allowlist: scripts/web_style_allowlist.txt — "<rule_name> <relative_path>"
 * per line, a CI-enforced ratchet: entries may shrink but never grow on main.
 *
 * Usage:
 *   node scripts/check_web_colors.js
 *   node scripts/check_web_colors.js --update-baseline  # regenerate allowlist
 */

'use strict';

const fs = require('fs');
const path = require('path');

const ROOT = path.join(__dirname, '..');
const SCAN_DIR = path.join(ROOT, 'website', 'content');
const ALLOWLIST_PATH = path.join(__dirname, 'web_style_allowlist.txt');

const EXCLUDED_PATH_SEGMENTS = [
  ['css', 'gen'].join(path.sep),
  'choice-sheet',
];
const EXCLUDED_FILES = new Set([
  'css/fonts.css',
  // Generated off-app notification copy review page (cmd/notification_examples);
  // its inline CSS + embedded email previews are a dev artifact, not site styling.
  'dev/notification-examples.html',
]);

// Families the tokens define (design/tokens.json) plus their fallback stacks
// and CSS-generic/keyword values. Anything else is off-token.
const ALLOWED_FONT_TOKENS = new Set([
  'libre baskerville', 'public sans',
  'baskerville', 'times new roman',
  '-apple-system', 'blinkmacsystemfont', 'segoe ui', 'roboto',
  'serif', 'sans-serif', 'system-ui', 'monospace', 'ui-monospace', 'menlo',
  'courier new', 'courier',
  'inherit', 'initial', 'unset',
]);

// Hex colors. Word-boundary'd so anchors in URLs (#section) don't match —
// require 3, 6, or 8 hex digits exactly.
const HEX_COLOR_RE = /#([0-9a-fA-F]{8}|[0-9a-fA-F]{6}|[0-9a-fA-F]{3})\b/g;
// rgb()/rgba()/hsl()/hsla() functional colors.
const FN_COLOR_RE = /\b(?:rgba?|hsla?)\(\s*([^)]*)\)/g;

function isPureBlackOrWhiteHex(hex) {
  const h = hex.toLowerCase();
  return ['fff', 'ffffff', '000', '000000'].includes(h) ||
    /^(?:ffffff|000000)[0-9a-f]{2}$/.test(h);
}

function isPureBlackOrWhiteFn(args) {
  const nums = args.split(/[\s,/]+/).filter(Boolean);
  if (nums.length < 3) return true; // var()-based or malformed — not a literal color
  const rgb = nums.slice(0, 3);
  return rgb.every((n) => n === '0') || rgb.every((n) => n === '255');
}

// ---------------------------------------------------------------------------
// Extraction
// ---------------------------------------------------------------------------

// For HTML, only scan style-bearing regions: <style> blocks, style=""
// attributes, and SVG fill=/stroke= attributes. Prose hex (e.g. a hex code
// quoted in copy) is not a violation.
function styleRegions(content, isHtml) {
  if (!isHtml) return [{ text: content, offset: 0 }];
  const regions = [];
  const styleBlock = /<style[^>]*>([\s\S]*?)<\/style>/g;
  const inlineAttr = /\b(?:style|fill|stroke)="([^"]*)"/g;
  let m;
  while ((m = styleBlock.exec(content)) !== null) {
    regions.push({ text: m[1], offset: m.index + m[0].indexOf(m[1]) });
  }
  while ((m = inlineAttr.exec(content)) !== null) {
    regions.push({ text: m[1], offset: m.index + m[0].indexOf(m[1]) });
  }
  return regions;
}

function stripCssComments(text) {
  // Preserve length so offsets keep mapping to line numbers.
  return text.replace(/\/\*[\s\S]*?\*\//g, (c) => c.replace(/[^\n]/g, ' '));
}

function lineOf(content, absOffset) {
  return content.slice(0, absOffset).split('\n').length;
}

function scanFile(filePath) {
  const content = fs.readFileSync(filePath, 'utf-8');
  const relPath = path.relative(ROOT, filePath).split(path.sep).join('/');
  return scanContent(content, relPath, filePath.endsWith('.html'));
}

function scanContent(content, relPath, isHtml) {
  const violations = [];

  for (const region of styleRegions(content, isHtml)) {
    const text = stripCssComments(region.text);

    let m;
    HEX_COLOR_RE.lastIndex = 0;
    while ((m = HEX_COLOR_RE.exec(text)) !== null) {
      if (isPureBlackOrWhiteHex(m[1])) continue;
      violations.push({
        path: relPath,
        line: lineOf(content, region.offset + m.index),
        rule: 'no_raw_web_color',
        text: m[0],
      });
    }

    FN_COLOR_RE.lastIndex = 0;
    while ((m = FN_COLOR_RE.exec(text)) !== null) {
      if (isPureBlackOrWhiteFn(m[1])) continue;
      violations.push({
        path: relPath,
        line: lineOf(content, region.offset + m.index),
        rule: 'no_raw_web_color',
        text: m[0].slice(0, 60),
      });
    }

    const fontRe = /font-family:\s*([^;}{]+)/g;
    while ((m = fontRe.exec(text)) !== null) {
      const families = m[1]
        .split(',')
        .map((f) => f.trim().replace(/^['"]|['"]$/g, '').toLowerCase())
        .filter((f) => f && !f.startsWith('var('));
      const bad = families.filter((f) => !ALLOWED_FONT_TOKENS.has(f));
      if (bad.length) {
        violations.push({
          path: relPath,
          line: lineOf(content, region.offset + m.index),
          rule: 'no_off_token_font',
          text: `font-family: ${bad.join(', ')}`,
        });
      }
    }
  }

  return violations;
}

// ---------------------------------------------------------------------------
// File collection
// ---------------------------------------------------------------------------

function walkFiles(dir, out = []) {
  if (!fs.existsSync(dir)) return out;
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    // statSync, not entry.isDirectory(): the latter reports false for a
    // symlink, and website/content/{css,fonts} are symlinks into
    // server/services/web/static/ (#2953). Descending on the Dirent alone
    // silently skipped every stylesheet while still reporting OK.
    const stat = fs.statSync(full, { throwIfNoEntry: false });
    if (!stat) continue; // dangling symlink
    if (stat.isDirectory()) {
      walkFiles(full, out);
    } else if (stat.isFile() && (entry.name.endsWith('.css') || entry.name.endsWith('.html'))) {
      out.push(full);
    }
  }
  return out;
}

function collectFiles() {
  return walkFiles(SCAN_DIR).filter((f) => {
    const relFromContent = path.relative(SCAN_DIR, f).split(path.sep).join('/');
    if (EXCLUDED_FILES.has(relFromContent)) return false;
    for (const seg of EXCLUDED_PATH_SEGMENTS) {
      if (relFromContent.includes(seg.split(path.sep).join('/'))) return false;
    }
    return true;
  });
}

// ---------------------------------------------------------------------------
// Allowlist (ratchet) — same mechanics as check_no_hardcoded_accent.js.
// ---------------------------------------------------------------------------

function loadAllowlist() {
  if (!fs.existsSync(ALLOWLIST_PATH)) return new Set();
  const entries = new Set();
  for (const rawLine of fs.readFileSync(ALLOWLIST_PATH, 'utf-8').split('\n')) {
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
  const keys = [...new Set(violations.map(violationKey))].sort();
  const header = [
    '# scripts/web_style_allowlist.txt — ratchet for check_web_colors.js.',
    '# Format: <rule_name> <relative_path>. One file/rule pair per line.',
    '# May shrink but never grow on main. PRs adding entries must remove',
    '# an equal-or-greater number of entries elsewhere. See #2441 for context.',
    '',
  ].join('\n');
  fs.writeFileSync(ALLOWLIST_PATH, header + keys.join('\n') + (keys.length ? '\n' : ''));
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

function main() {
  const violations = [];
  for (const f of collectFiles()) {
    for (const v of scanFile(f)) violations.push(v);
  }

  if (process.argv.includes('--update-baseline')) {
    writeAllowlist(violations);
    console.log(
      `Wrote ${path.relative(ROOT, ALLOWLIST_PATH)} with ${new Set(violations.map(violationKey)).size} entries.`,
    );
    return;
  }

  const allowlist = loadAllowlist();
  const newViolations = violations.filter((v) => !allowlist.has(violationKey(v)));

  if (newViolations.length === 0) {
    console.log('check_web_colors: OK');
    return;
  }

  console.error(`check_web_colors: ${newViolations.length} violation(s):`);
  for (const v of newViolations) {
    console.error(`  ${v.path}:${v.line}  [${v.rule}]  ${v.text}`);
  }
  console.error('');
  console.error('Use the token variables from css/gen/tokens.gen.css (see design/README.md).');
  console.error('If a violation is truly unavoidable, regenerate the allowlist with');
  console.error('  node scripts/check_web_colors.js --update-baseline');
  console.error('and remove an equal-or-greater number of entries elsewhere in the same PR.');
  process.exit(1);
}

if (require.main === module) {
  main();
}

module.exports = {
  ALLOWED_FONT_TOKENS,
  isPureBlackOrWhiteHex,
  isPureBlackOrWhiteFn,
  styleRegions,
  scanFile,
  scanContent,
  collectFiles,
  violationKey,
};
