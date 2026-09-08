#!/usr/bin/env node
/**
 * check_glass_foreground.js — keep glass FILL tokens out of the positions where
 * they become foregrounds by accident (#2770 / #2764, corrected by #2806).
 *
 * ## The rule, and why it is narrower than it used to be
 *
 * This lint originally banned four tokens from every foreground position, on
 * the premise that painting them on the glass sheet measures ~1:1. That premise
 * described `glass.primary` when it was the deep sage #3E5A47 — 1.66:1 on the
 * sheet, which is what #2764 actually was. #2770 flipped it to the light sage
 * #9DBFA8, which measures ~6.3:1 there, and its own `$description` in
 * design/tokens.json now calls it "the on-glass action colour".
 *
 * So the token is genuinely dual-role, and the blanket ban had become false: it
 * flagged six call sites that paint an accent icon or label on glass at ~6.3:1
 * and are correct. #2806 split the list:
 *
 *   FILL_ONLY_TOKENS  (solid white)  — banned in every foreground position.
 *   DUAL_ROLE_TOKENS  (the sage)     — banned only in ColorScheme slots.
 *
 * The ColorScheme ban survives for all of them because it is a different
 * defect: one slot is resolved by Material into a fill AND a foreground at
 * once, and the author chooses neither. That was #2764's mechanism, and with
 * today's light sage the selected day cell would still be light sage under
 * white text at 2.01:1.
 *
 * Detected positions:
 *   - `ColorScheme(... primary: / secondary: / tertiary: ...)`  — all tokens
 *   - `foregroundColor:` / `iconColor:` / `labelColor:`         — fill-only
 *   - `color:` inside a `TextStyle(...)`                        — fill-only
 *   - `color:` inside an `Icon(...)`                            — fill-only
 *
 * Tokens reached through a one-hop local alias (`final c = <TOKEN>;` then
 * `color: c`) are resolved. #2798's chip evaded this scanner exactly that way
 * and was never reported at all.
 *
 * Apple's materials encode the same role separation: vibrancy vends label
 * colours, and the HIG tells you not to put custom brand colours on a vibrant
 * material. The difference is that a rule living only in a doc comment is how
 * #2764 shipped — so this fails the build instead.
 *
 * ## What this lint structurally cannot tell you (#2798)
 *
 * It reasons about a token's POSITION and never about what the element paints
 * underneath. When a control paints its OWN opaque fill, the correct foreground
 * is whatever contrasts with THAT fill — not the on-glass text ramp.
 *
 * #2798 was this lint's own remedy applied to such a control: the "Add
 * Screenshots" label moved off the sage token and onto `modalTextPrimary`
 * (white) while the element's own fill was #F2F2EE, taking it from 1.79:1 to
 * 1.12:1. Nothing here could tell that case from a label on the bare sheet.
 *
 * That gap is covered by measurement rather than by pattern-matching:
 *   - `app/test/helpers/contrast_helpers.dart` — opaque pairs, every
 *     `flutter test`. This is what catches the #2798 shape.
 *   - `e2e/scripts/contrast_surfaces.mjs` — translucent/composited pairs.
 *
 * Usage: node scripts/check_glass_foreground.js
 */

'use strict';

const fs = require('fs');
const path = require('path');

const ROOT = path.resolve(__dirname, '..');
const SCAN_DIR = path.join(ROOT, 'app', 'lib');
const ALLOWLIST_REL = 'scripts/glass_foreground_allowlist.txt';

/**
 * Fill-ONLY tokens: solid white, banned in every foreground position.
 *
 * Solid white as a "foreground" is `textPrimary`'s job. Reaching for the chip
 * FILL token to colour text is pure role confusion with no upside — the value
 * is identical, so the only thing the wrong name buys you is a silent break the
 * day one of the two moves.
 */
const FILL_ONLY_TOKENS = ['GlassTokens.fillStrong', 'AppColors.modalChipBackgroundActive'];

/**
 * Dual-role tokens: legitimately a fill AND an on-glass accent foreground.
 *
 * `glass.primary`'s own `$description` in design/tokens.json calls it "the
 * on-glass action colour" and records it measuring 9.01:1 as a foreground on
 * the sheet. Since #2770 flipped it from the deep sage #3E5A47 (1.66:1, which
 * is what #2764 actually was) to the light sage #9DBFA8, painting it as an
 * accent foreground on glass is correct, not a defect — six call sites do it
 * today at ~6.3:1.
 *
 * So these are NOT banned from direct foreground positions. What remains banned
 * is handing them to a `ColorScheme` slot (below), which is a different defect.
 */
const DUAL_ROLE_TOKENS = ['GlassTokens.primary', 'AppColors.modalPrimaryButtonBackground'];

/** Every token this lint tracks, for the cheap "is it even in this file" test. */
const FILL_TOKENS = [...FILL_ONLY_TOKENS, ...DUAL_ROLE_TOKENS];

/**
 * ColorScheme slots Material resolves as text/icon colours.
 *
 * Banned for ALL tracked tokens, dual-role included, and the reason survives
 * #2770's value change: a token in `ColorScheme.primary` is resolved by Material
 * into many roles at once — TextButton foreground AND selected-day fill — and
 * the author picks none of them. That is the #2764 mechanism. With today's light
 * sage the day cell would be light sage under white text at 2.01:1, so the ban
 * is load-bearing regardless of how the token measures on the sheet.
 */
const FOREGROUND_SCHEME_SLOTS = ['primary', 'secondary', 'tertiary', 'inversePrimary'];

const FOREGROUND_PARAMS = ['foregroundColor', 'iconColor', 'labelColor', 'headerForegroundColor'];

function walk(dir, out = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === 'gen' || entry.name === 'l10n') continue;
      walk(p, out);
    } else if (entry.name.endsWith('.dart') && !/\.(g|freezed|mocks)\.dart$/.test(entry.name)) {
      out.push(p);
    }
  }
  return out;
}

function loadAllowlist() {
  const p = path.join(ROOT, ALLOWLIST_REL);
  if (!fs.existsSync(p)) return new Set();
  return new Set(
    fs
      .readFileSync(p, 'utf8')
      .split('\n')
      .map((l) => l.replace(/#.*$/, '').trim())
      .filter(Boolean),
  );
}

/** Finds TextStyle(...) spans so `color:` inside them can be treated as text. */
function textStyleRanges(src) {
  const ranges = [];
  const re = /TextStyle\s*\(/g;
  let m;
  while ((m = re.exec(src))) {
    let depth = 1;
    let i = re.lastIndex;
    while (i < src.length && depth > 0) {
      if (src[i] === '(') depth++;
      else if (src[i] === ')') depth--;
      i++;
    }
    ranges.push([m.index, i]);
  }
  return ranges;
}

function colorSchemeRanges(src) {
  const ranges = [];
  const re = /ColorScheme\s*\(/g;
  let m;
  while ((m = re.exec(src))) {
    let depth = 1;
    let i = re.lastIndex;
    while (i < src.length && depth > 0) {
      if (src[i] === '(') depth++;
      else if (src[i] === ')') depth--;
      i++;
    }
    ranges.push([m.index, i]);
  }
  return ranges;
}

/** Finds `Icon(...)` spans so `color:` inside them counts as a foreground. */
function iconRanges(src) {
  const ranges = [];
  const re = /\bIcon\s*\(/g;
  let m;
  while ((m = re.exec(src))) {
    let depth = 1;
    let i = re.lastIndex;
    while (i < src.length && depth > 0) {
      if (src[i] === '(') depth++;
      else if (src[i] === ')') depth--;
      i++;
    }
    ranges.push([m.index, i]);
  }
  return ranges;
}

/**
 * One-hop local aliases of a token: `final primary = GlassTokens.primary;`
 *
 * #2798's chip did exactly this and then wrote `color: primary` inside a
 * TextStyle. Matching only the literal token string meant the scanner reported
 * nothing at all — the worst outcome for a lint, because silence reads as
 * "clean" rather than "not looked at".
 */
function aliasesOf(src, token) {
  const names = [];
  const escaped = token.replace(/\./g, '\\.');
  const re = new RegExp(
    `(?:final|const|var|static\\s+const)\\s+(?:Color\\s+)?([A-Za-z_][A-Za-z0-9_]*)\\s*=\\s*${escaped}\\s*;`,
    'g',
  );
  let m;
  while ((m = re.exec(src))) names.push(m[1]);
  return names;
}

const inAny = (ranges, idx) => ranges.some(([a, b]) => idx >= a && idx < b);

function scan(file, allowlist) {
  const rel = path.relative(ROOT, file);
  if (allowlist.has(rel)) return [];
  const src = fs.readFileSync(file, 'utf8');
  if (!FILL_TOKENS.some((t) => src.includes(t))) return [];

  const violations = [];
  const styles = textStyleRanges(src);
  const schemes = colorSchemeRanges(src);
  const icons = iconRanges(src);
  const lineOf = (idx) => src.slice(0, idx).split('\n').length;

  for (const token of FILL_TOKENS) {
    const fillOnly = FILL_ONLY_TOKENS.includes(token);

    // The literal token, plus any local variable it was assigned to.
    const needles = [
      { text: token, via: null },
      ...aliasesOf(src, token).map((name) => ({ text: name, via: token })),
    ];

    for (const needle of needles) {
      let from = 0;
      for (;;) {
        const idx = src.indexOf(needle.text, from);
        if (idx === -1) break;
        from = idx + needle.text.length;

        // An alias is a bare identifier, so require whole-word boundaries —
        // otherwise `primary` matches inside `primaryColor` and `onPrimary`.
        if (needle.via) {
          const prev = src[idx - 1] ?? '';
          const next = src[idx + needle.text.length] ?? '';
          if (/[A-Za-z0-9_.]/.test(prev) || /[A-Za-z0-9_]/.test(next)) continue;
        }

        // What parameter is this value being passed to? Look back to the nearest
        // `name:` on the same or previous line.
        const before = src.slice(Math.max(0, idx - 200), idx);
        const param = /([A-Za-z_][A-Za-z0-9_]*)\s*:\s*$/.exec(before.replace(/\s+$/, (s) => s))?.[1]
          ?? /([A-Za-z_][A-Za-z0-9_]*)\s*:\s*[^,;{}]*$/.exec(before)?.[1];

        let why = null;
        if (inAny(schemes, idx) && param && FOREGROUND_SCHEME_SLOTS.includes(param)) {
          // Banned for every tracked token — Material resolves one slot into
          // both a fill and a foreground and the author chooses neither.
          why = `assigned to ColorScheme.${param}, which Material also resolves as a text/icon colour`;
        } else if (fillOnly && param && FOREGROUND_PARAMS.includes(param)) {
          why = `passed to ${param}:`;
        } else if (fillOnly && param === 'color' && inAny(styles, idx)) {
          why = 'used as TextStyle.color';
        } else if (fillOnly && param === 'color' && inAny(icons, idx)) {
          why = 'used as Icon.color';
        }
        if (why) {
          violations.push({
            rel,
            line: lineOf(idx),
            token,
            why: needle.via ? `${why}, via the local alias \`${needle.text}\`` : why,
          });
        }
      }
    }
  }
  return violations;
}

function main() {
  const allowlist = loadAllowlist();
  const violations = walk(SCAN_DIR).flatMap((f) => scan(f, allowlist));

  if (!violations.length) {
    console.log('check_glass_foreground passed: no fill token used as an on-glass foreground.');
    return;
  }
  console.error('check_glass_foreground: fill tokens used as foregrounds\n');
  for (const v of violations) {
    console.error(`  ${v.rel}:${v.line}`);
    console.error(`    ${v.token} ${v.why}`);
  }
  console.error(
    `\n${violations.length} violation(s). A FILL token is standing in a foreground slot.` +
      '\n' +
      '\nFor a ColorScheme slot: give Material a foreground token instead. One' +
      '\nslot becomes both a fill and a foreground and you choose neither — that' +
      '\nis #2764, and it stays wrong no matter what the token measures.' +
      '\n' +
      '\nOtherwise, pick the fix by asking what this element paints UNDERNEATH:' +
      '\n' +
      '\n  - Nothing — it sits directly on the glass sheet:' +
      '\n      use an on-glass foreground token (GlassTokens.textPrimary /' +
      '\n      textSecondary / textFaint), or GlassTokens.primary for an accent.' +
      '\n' +
      '\n  - Its own fill — a Container/DecoratedBox with a colour:' +
      '\n      pair the foreground against THAT fill, not against the sheet.' +
      '\n      Do NOT reach for the on-glass ramp here. That is #2798: an earlier' +
      '\n      version of this message sent a white label onto a #F2F2EE fill and' +
      '\n      took it from 1.79:1 to 1.12:1. Use the matched pairs —' +
      '\n      fillStrong/onFillStrong for chips, primary/onPrimary for the CTA.' +
      '\n      This lint cannot see your fill; app/test/helpers/contrast_helpers' +
      '\n      .dart measures it.' +
      '\n' +
      `\nOr add the file to ${ALLOWLIST_REL} with a tracking issue.`,
  );
  process.exit(1);
}

if (require.main === module) main();

module.exports = {
  scan,
  loadAllowlist,
  FILL_TOKENS,
  FILL_ONLY_TOKENS,
  DUAL_ROLE_TOKENS,
  FOREGROUND_SCHEME_SLOTS,
};
