// Tests for the untokenized-colour inventory (#2770).
//
// The inventory's whole value is that its counts drive a migration plan, so the
// classification has to be right about which literals are mechanical renames
// and which need a decision. A near-miss silently reported as an exact match
// would migrate a colour to a token that does not actually equal it.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');

const {
  normalizeHex,
  distance,
  nearestToken,
  scanFile,
  isScannable,
} = require('./inventory_raw_colors.js');

const TOKENS = [
  { name: 'color.light.primary', argb: 'FF3E5A47' },
  { name: 'glass.surface', argb: '26FFFFFF' },
  { name: 'overlay.scrim-top', argb: '80000000' },
];

test('normalizeHex accepts every spelling and yields 8-digit ARGB', () => {
  assert.equal(normalizeHex('#3E5A47'), 'FF3E5A47');
  assert.equal(normalizeHex('#FF3E5A47'), 'FF3E5A47');
  assert.equal(normalizeHex('0xFF3e5a47'), 'FF3E5A47');
  // A 6-digit value is opaque by convention; alpha must not be left undefined,
  // or every distance against it silently compares garbage.
  assert.equal(normalizeHex('3E5A47').slice(0, 2), 'FF');
});

test('distance separates RGB from alpha', () => {
  // Same colour, different opacity — an ad-hoc opacity step, not a new hue.
  const d = distance('26FFFFFF', 'FFFFFFFF');
  assert.equal(d.rgb, 0);
  assert.equal(d.alpha, 0xff - 0x26);

  // Same opacity, different colour.
  const d2 = distance('FF000000', 'FFFFFFFF');
  assert.equal(d2.alpha, 0);
  assert.ok(d2.rgb > 400, `expected a large RGB distance, got ${d2.rgb}`);
});

test('nearestToken finds an exact match with zero distance', () => {
  const n = nearestToken('FF3E5A47', TOKENS);
  assert.equal(n.name, 'color.light.primary');
  assert.equal(n.rgb, 0);
  assert.equal(n.alpha, 0);
});

test('nearestToken does not report a near-miss as exact', () => {
  // 0xFFA7C59E is the real `_rsvp` literal from gear_who_card.dart.
  const n = nearestToken('FFA7C59E', TOKENS);
  assert.ok(n.rgb > 0, 'a different colour must not measure as distance 0');
});

test('nearestToken weighs alpha so a translucent white prefers the glass token', () => {
  const n = nearestToken('26FFFFFF', TOKENS);
  assert.equal(n.name, 'glass.surface');
});

function withFixture(source, run) {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'inv-'));
  const file = path.join(dir, 'fixture.dart');
  fs.writeFileSync(file, source);
  try {
    return run(file);
  } finally {
    fs.rmSync(dir, { recursive: true, force: true });
  }
}

test('scanFile finds hex, Material shortcuts and alpha modifiers', () => {
  const findings = withFixture(
    [
      "const a = Color(0xFFA7C59E);",
      "const b = Colors.white;",
      "final c = something.withValues(alpha: 0.35);",
      "final d = other.withAlpha(56);",
      "const e = Color.fromARGB(255, 62, 90, 71);",
    ].join('\n'),
    (f) => scanFile(f, TOKENS),
  );

  const kinds = findings.map((f) => f.kind);
  assert.ok(kinds.includes('hex'));
  assert.ok(kinds.includes('material'));
  assert.ok(kinds.includes('alpha-withValues'));
  assert.ok(kinds.includes('alpha-withAlpha'));

  // fromARGB must normalize to the same ARGB spelling as a hex literal, or the
  // two spellings of one colour appear as two separate values in the report.
  const argb = findings.find((f) => f.kind === 'hex-argb');
  assert.equal(argb.argb, 'FF3E5A47');
  assert.equal(argb.nearest.rgb, 0);
});

test('scanFile ignores commented-out colours', () => {
  const findings = withFixture(
    ['// const old = Color(0xFFE07A5F);', '/// See Color(0xFF123456) for why.'].join('\n'),
    (f) => scanFile(f, TOKENS),
  );
  assert.deepEqual(findings, []);
});

test('scanFile skips Colors.transparent, which is structural not palette', () => {
  const findings = withFixture('const a = Colors.transparent;', (f) => scanFile(f, TOKENS));
  assert.deepEqual(findings, []);
});

test('scanFile flags files that paint on glass or media', () => {
  // A file with no glass/media signal is ordinary risk.
  const flat = withFixture(
    'Widget build() => Container(color: Color(0xFF112233));',
    (f) => scanFile(f, TOKENS),
  );
  assert.equal(flat.length, 1);
  assert.equal(flat[0].onSurface, false);

  const real = withFixture(
    ['Widget build() => BackdropFilter(', '  child: Container(color: Color(0xFF112233)),', ');'].join('\n'),
    (f) => scanFile(f, TOKENS),
  );
  assert.equal(real.length, 1);
  assert.equal(real[0].onSurface, true);
});

test('isScannable excludes generated and theme-definition sources', () => {
  assert.equal(isScannable(path.join('app', 'lib', 'core', 'theme', 'app_colors.dart')), false);
  assert.equal(isScannable(path.join('app', 'lib', 'data', 'gen', 'foo.dart')), false);
  assert.equal(isScannable(path.join('app', 'lib', 'widgets', 'x.g.dart')), false);
  assert.equal(isScannable(path.join('app', 'lib', 'widgets', 'x.dart')), true);
});
