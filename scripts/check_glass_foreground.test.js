'use strict';

// Tests for the on-glass foreground lint (#2770).
//
// The lint exists because the rule it enforces previously lived only in a doc
// comment, and #2764 shipped anyway. A lint that misses the pattern it was
// written for is worse than none — it reads as proof the code is clean.

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');

const {
  scan,
  loadAllowlist,
  FILL_TOKENS,
  FILL_ONLY_TOKENS,
  DUAL_ROLE_TOKENS,
} = require('./check_glass_foreground.js');

function withFile(contents, fn) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'glassfg-'));
  const file = path.join(dir, 'sample.dart');
  fs.writeFileSync(file, contents);
  try {
    return fn(file);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
}

test('flags the #2764 pattern: a fill token as ColorScheme.primary', () => {
  const violations = withFile(
    `
    const scheme = ColorScheme(
      brightness: Brightness.dark,
      primary: AppColors.modalPrimaryButtonBackground,
      onPrimary: AppColors.modalPrimaryButtonText,
    );
    `,
    (f) => scan(f, new Set()),
  );
  assert.strictEqual(violations.length, 1);
  assert.match(violations[0].why, /ColorScheme\.primary/);
});

test('flags a fill-only token used as TextStyle.color', () => {
  const violations = withFile(
    `
    Text('19', style: TextStyle(fontSize: 22, color: GlassTokens.fillStrong));
    `,
    (f) => scan(f, new Set()),
  );
  assert.strictEqual(violations.length, 1);
  assert.match(violations[0].why, /TextStyle\.color/);
});

test('flags a fill-only token passed to foregroundColor', () => {
  const violations = withFile(
    `
    TextButton.styleFrom(foregroundColor: AppColors.modalChipBackgroundActive);
    `,
    (f) => scan(f, new Set()),
  );
  assert.strictEqual(violations.length, 1);
  assert.match(violations[0].why, /foregroundColor/);
});

test('flags a fill-only token used as Icon.color (#2798 gap, closed in #2806)', () => {
  // `Icon(color:)` is neither `iconColor:` nor inside a TextStyle. In #2798
  // that is why the icon beside the broken label kept the fill token while the
  // label next to it got "fixed".
  const violations = withFile(
    `Icon(Icons.add, color: GlassTokens.fillStrong, size: 20);`,
    (f) => scan(f, new Set()),
  );
  assert.strictEqual(violations.length, 1);
  assert.match(violations[0].why, /Icon\.color/);
});

test('resolves a one-hop local alias (#2798 gap, closed in #2806)', () => {
  // Verbatim shape of feedback_sheet.dart's type pill. Matching only the
  // literal token string meant this evaded the scanner entirely — the chip was
  // never reported, not even wrongly.
  const violations = withFile(
    `
    final active = GlassTokens.fillStrong;
    return Text(label, style: TextStyle(fontSize: 13, color: active));
    `,
    (f) => scan(f, new Set()),
  );
  assert.strictEqual(violations.length, 1);
  assert.match(violations[0].why, /local alias/);
  assert.match(violations[0].why, /TextStyle\.color/);
});

test('alias matching respects word boundaries', () => {
  // `active` must not match inside `activeColor` or `isActive`.
  const violations = withFile(
    `
    final active = GlassTokens.fillStrong;
    Text('x', style: TextStyle(color: activeColor));
    Container(decoration: BoxDecoration(color: active));
    `,
    (f) => scan(f, new Set()),
  );
  assert.deepStrictEqual(violations, []);
});

test('the dual-role sage is allowed as a direct on-glass foreground (#2806)', () => {
  // This is the correction. `glass.primary` measures ~6.3:1 on the sheet since
  // #2770 and its own $description calls it "the on-glass action colour", so an
  // accent icon or label painted with it is correct. The blanket ban flagged
  // six such call sites; that was the lint being wrong, not the code.
  const violations = withFile(
    `
    final accent = GlassTokens.primary;
    Icon(Icons.ios_share_outlined, size: 15, color: accent);
    Text('Share', style: TextStyle(color: accent, fontSize: 12));
    Icon(Icons.group_outlined, color: AppColors.modalPrimaryButtonBackground);
    `,
    (f) => scan(f, new Set()),
  );
  assert.deepStrictEqual(violations, []);
});

test('the dual-role sage is still banned in a ColorScheme slot (#2764)', () => {
  // The ColorScheme ban does NOT relax with the value change: one slot becomes
  // both the TextButton foreground and the selected-day fill, and the author
  // picks neither. With today's light sage that day cell is light sage under
  // white text at 2.01:1.
  const violations = withFile(
    `
    const scheme = ColorScheme(brightness: Brightness.dark, primary: GlassTokens.primary);
    `,
    (f) => scan(f, new Set()),
  );
  assert.strictEqual(violations.length, 1);
  assert.match(violations[0].why, /ColorScheme\.primary/);
});

test('allows a fill token used as an actual fill', () => {
  // The whole point: these values are correct as backgrounds. A lint that
  // flagged them everywhere would just get disabled.
  const violations = withFile(
    `
    Container(
      decoration: BoxDecoration(color: GlassTokens.primary),
      child: Text('Save', style: TextStyle(color: GlassTokens.onPrimary)),
    );
    DatePickerThemeData(
      dayBackgroundColor: WidgetStateProperty.all(GlassTokens.primary),
      backgroundColor: AppColors.modalPrimaryButtonBackground,
    );
    `,
    (f) => scan(f, new Set()),
  );
  assert.deepStrictEqual(violations, []);
});

test('allows on-glass foreground tokens in foreground positions', () => {
  const violations = withFile(
    `
    Text('x', style: TextStyle(color: GlassTokens.textPrimary));
    TextButton.styleFrom(foregroundColor: GlassTokens.textSecondary);
    `,
    (f) => scan(f, new Set()),
  );
  assert.deepStrictEqual(violations, []);
});

test('a file on the allowlist is skipped entirely', () => {
  withFile(`Text('x', style: TextStyle(color: GlassTokens.primary));`, (f) => {
    const rel = path.relative(path.resolve(__dirname, '..'), f);
    assert.deepStrictEqual(scan(f, new Set([rel])), []);
  });
});

test('nothing is allowlisted — #2764 and its class are fixed, not deferred', () => {
  // This started as a ratchet listing three known-bad files. All three were
  // fixed, so the allowlist is gone and every file is now guarded. Re-adding an
  // entry would be a deliberate regression; entries may be removed, never added.
  const allowlist = loadAllowlist();
  assert.strictEqual(
    allowlist.size,
    0,
    `expected no allowlisted files, got: ${[...allowlist].join(', ')}`,
  );
});

test('allowlist entries, if ever reintroduced, must be exact existing paths', () => {
  // A wildcard or a stale path would silently blind the lint over a wider area
  // than intended — the failure mode that looks like "the code is clean".
  const root = path.resolve(__dirname, '..');
  for (const rel of loadAllowlist()) {
    assert.ok(!rel.includes('*'), `${rel}: allowlist entries must be exact paths`);
    assert.ok(fs.existsSync(path.join(root, rel)), `${rel}: allowlisted file does not exist`);
  }
});

test('glass_pickers.dart is clean — the file the reporter photographed', () => {
  const file = path.resolve(
    __dirname,
    '..',
    'app/lib/presentation/widgets/modal/glass/glass_pickers.dart',
  );
  assert.ok(fs.existsSync(file), 'glass_pickers.dart moved — the lint needs re-pointing');
  assert.deepStrictEqual(scan(file, new Set()), []);
});

test('feedback_sheet.dart is clean — the file #2798 was reported against', () => {
  const file = path.resolve(
    __dirname,
    '..',
    'app/lib/presentation/widgets/feedback/feedback_sheet.dart',
  );
  assert.ok(fs.existsSync(file), 'feedback_sheet.dart moved — the lint needs re-pointing');
  assert.deepStrictEqual(scan(file, new Set()), []);
});

test('the token split is exhaustive and disjoint (#2806)', () => {
  // The two lists drive different rules, so a token landing in both — or in
  // neither — silently changes what is enforced.
  const overlap = FILL_ONLY_TOKENS.filter((t) => DUAL_ROLE_TOKENS.includes(t));
  assert.deepStrictEqual(overlap, [], 'a token cannot be both fill-only and dual-role');
  assert.deepStrictEqual(
    [...FILL_TOKENS].sort(),
    [...FILL_ONLY_TOKENS, ...DUAL_ROLE_TOKENS].sort(),
    'FILL_TOKENS must be exactly the union of the two lists',
  );
});

test('this lint still cannot see a locally-painted fill — by construction', () => {
  // Pinning the structural limit so it stays a documented fact rather than a
  // surprise. #2798's shape is invisible here: the foreground token is a
  // legitimate on-glass one, and what makes it wrong is the fill the element
  // paints for itself, which this scanner never looks at.
  //
  // app/test/helpers/contrast_helpers.dart is what catches it. If you are
  // tempted to teach this lint about fills, measure instead — that is the
  // lesson of #2798 and of #2806.
  const violations = withFile(
    `
    Container(
      decoration: BoxDecoration(color: AppColors.accentButtonBackground),
      child: Text('x', style: TextStyle(color: AppColors.modalTextPrimary)),
    );
    `,
    (f) => scan(f, new Set()),
  );
  assert.deepStrictEqual(violations, [], 'position-based linting cannot reach this case');
});
