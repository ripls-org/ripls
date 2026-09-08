// Tests for the composited-contrast analyzer (#2770).
//
// Fixtures are synthesised with hard edges and analytic ground truth, so a
// failure here is a logic bug rather than an anti-aliasing artifact. The
// anti-aliased case is covered by rendering the real bundle in the sweep.
//
// Ground truth for the glass stack: over a flat backdrop B, the scrim
// (black @ 35%) then the sheet (white @ 15%) composite to
//   0.85 * (0.65 * B) + 38.25
// which for B = 0x1A is 52.6 — the value the #2764 measurement sits on.

import test from 'node:test';
import assert from 'node:assert';

import {
  analyze,
  classify,
  connectedComponents,
  contrastRatio,
  judge,
  parseHex,
  relativeLuminance,
  ringBackground,
} from './contrast_analyzer.mjs';

const W = 60;
const H = 60;

/** Composited glass background over a flat backdrop, per the formula above. */
const composite = (b) => Math.round(0.85 * (0.65 * b) + 38.25);

const GLASS_DARK = composite(0x1a); // 53
const SAGE = [0x3e, 0x5a, 0x47]; // #3E5A47 — the #2764 offender
const WHITE = [255, 255, 255];

function frame(width, height, fill) {
  const buf = Buffer.alloc(width * height * 3);
  for (let i = 0; i < width * height; i++) {
    buf[i * 3] = fill[0];
    buf[i * 3 + 1] = fill[1];
    buf[i * 3 + 2] = fill[2];
  }
  return buf;
}

function rect(buf, width, x0, y0, w, h, color) {
  for (let y = y0; y < y0 + h; y++) {
    for (let x = x0; x < x0 + w; x++) {
      const i = y * width + x;
      buf[i * 3] = color[0];
      buf[i * 3 + 1] = color[1];
      buf[i * 3 + 2] = color[2];
    }
  }
}

test('contrastRatio: known anchors', () => {
  assert.ok(Math.abs(contrastRatio([255, 255, 255], [0, 0, 0]) - 21) < 0.01);
  assert.ok(Math.abs(contrastRatio([120, 120, 120], [120, 120, 120]) - 1) < 1e-9);
  // Symmetric regardless of argument order.
  assert.strictEqual(contrastRatio(SAGE, WHITE), contrastRatio(WHITE, SAGE));
});

test('relativeLuminance: black, white, and the sRGB knee', () => {
  assert.strictEqual(relativeLuminance([0, 0, 0]), 0);
  assert.ok(Math.abs(relativeLuminance([255, 255, 255]) - 1) < 1e-9);
  // Below the 0.04045 knee the transfer function is linear, not a power curve.
  assert.ok(Math.abs(relativeLuminance([10, 10, 10]) - 10 / 255 / 12.92) < 1e-9);
});

test('parseHex: with and without leading hash', () => {
  assert.deepStrictEqual(parseHex('#3E5A47'), SAGE);
  assert.deepStrictEqual(parseHex('3E5A47'), SAGE);
});

test('analyze: recovers the #2764 contrast from pixels alone', () => {
  const bg = [GLASS_DARK, GLASS_DARK, GLASS_DARK];
  const baseline = frame(W, H, bg);
  const sentinel = frame(W, H, bg);
  // A run of "text" painted with the primary token.
  rect(baseline, W, 10, 10, 20, 6, SAGE);
  rect(sentinel, W, 10, 10, 20, 6, parseHex('#FF0000'));

  const findings = analyze({
    baseline,
    sentinel,
    width: W,
    height: H,
    sentinelMap: { primary: '#FF0000' },
  });

  assert.strictEqual(findings.length, 1);
  assert.strictEqual(findings[0].token, 'primary');
  assert.deepStrictEqual(findings[0].foreground, SAGE);
  assert.deepStrictEqual(findings[0].background, bg);
  // Deep sage on the composited dark glass — the value #2764 reports.
  const expected = contrastRatio(SAGE, bg);
  assert.ok(Math.abs(findings[0].ratio - expected) < 1e-9);
  assert.ok(
    findings[0].ratio > 1.5 && findings[0].ratio < 1.8,
    `expected ~1.6:1, got ${findings[0].ratio}`,
  );
});

test('analyze: a token used twice is reported per component, not averaged', () => {
  // The failure this prevents: modalTextPrimary labels both the bare sheet and
  // the inside of a filled pill. Averaged into one mask, the minority context
  // disappears and a genuine failure reads as a pass.
  const sheet = [GLASS_DARK, GLASS_DARK, GLASS_DARK];
  const baseline = frame(W, H, sheet);
  const sentinel = frame(W, H, sheet);

  // Component 1: white text directly on the sheet.
  rect(baseline, W, 5, 5, 20, 6, WHITE);
  rect(sentinel, W, 5, 5, 20, 6, parseHex('#00FF00'));

  // Component 2: white text on a sage fill, far enough away not to 8-connect.
  rect(baseline, W, 5, 30, 30, 20, SAGE);
  rect(sentinel, W, 5, 30, 30, 20, parseHex('#FF0000'));
  rect(baseline, W, 12, 37, 16, 6, WHITE);
  rect(sentinel, W, 12, 37, 16, 6, parseHex('#00FF00'));

  const findings = analyze({
    baseline,
    sentinel,
    width: W,
    height: H,
    sentinelMap: { primary: '#FF0000', textPrimary: '#00FF00' },
  });

  const white = findings.filter((f) => f.token === 'textPrimary');
  assert.strictEqual(white.length, 2, 'expected two separate components');

  const onSheet = white.find((f) => f.bounds.y < 20);
  const onFill = white.find((f) => f.bounds.y >= 20);

  // Each component resolves the background it actually sits on.
  assert.deepStrictEqual(onSheet.background, sheet);
  assert.deepStrictEqual(onFill.background, SAGE);

  // And they genuinely differ — white-on-sheet is legible, white-on-sage less so.
  assert.ok(Math.abs(onSheet.ratio - contrastRatio(WHITE, sheet)) < 1e-9);
  assert.ok(Math.abs(onFill.ratio - contrastRatio(WHITE, SAGE)) < 1e-9);
  assert.ok(onSheet.ratio > onFill.ratio);
});

test('ringBackground: reports the fill under the text, not the surface behind it', () => {
  // A whole-bbox background estimate returns the sheet here (measured 2.97
  // against a true 2.41); the ring must return the fill.
  const sheet = [GLASS_DARK, GLASS_DARK, GLASS_DARK];
  const baseline = frame(W, H, sheet);
  const sentinel = frame(W, H, sheet);
  rect(baseline, W, 5, 5, 40, 30, SAGE);
  rect(sentinel, W, 5, 5, 40, 30, parseHex('#FF0000'));
  rect(baseline, W, 15, 15, 18, 8, WHITE);
  rect(sentinel, W, 15, 15, 18, 8, parseHex('#00FF00'));

  const sentinelRgb = [parseHex('#FF0000'), parseHex('#00FF00')];
  const { core, owned } = classify(baseline, sentinel, sentinelRgb);
  const textPixels = core.get(1);
  const bg = ringBackground(baseline, textPixels, owned.get(1), W, H);

  assert.deepStrictEqual(bg, SAGE, 'ring should sample the fill, not the sheet');
});

test('connectedComponents: separates runs and drops debris', () => {
  const pixels = [];
  // Two 6x6 blocks, well separated.
  for (let y = 0; y < 6; y++) {
    for (let x = 0; x < 6; x++) {
      pixels.push(y * W + x);
      pixels.push((y + 20) * W + (x + 20));
    }
  }
  // A stray 2-pixel speck that must not survive.
  pixels.push(40 * W + 40, 40 * W + 41);

  const components = connectedComponents(pixels, W, 24);
  assert.strictEqual(components.length, 2);
  assert.ok(components.every((c) => c.length === 36));
});

test('connectedComponents: 8-connectivity joins diagonal neighbours', () => {
  // A diagonal stroke is one glyph, not N separate ones.
  const pixels = [];
  for (let k = 0; k < 30; k++) pixels.push(k * W + k);
  const components = connectedComponents(pixels, W, 24);
  assert.strictEqual(components.length, 1);
});

test('analyze: rejects mismatched or wrongly-sized buffers', () => {
  const a = frame(W, H, [0, 0, 0]);
  assert.throws(
    () => analyze({ baseline: a, sentinel: Buffer.alloc(9), width: W, height: H, sentinelMap: {} }),
    /size mismatch/,
  );
  assert.throws(
    () => analyze({ baseline: a, sentinel: a, width: 7, height: 7, sentinelMap: {} }),
    /expected/,
  );
});

test('analyze: ignores a token that never rendered', () => {
  const bg = [GLASS_DARK, GLASS_DARK, GLASS_DARK];
  const baseline = frame(W, H, bg);
  const sentinel = frame(W, H, bg);
  rect(baseline, W, 10, 10, 20, 6, SAGE);
  rect(sentinel, W, 10, 10, 20, 6, parseHex('#FF0000'));

  const findings = analyze({
    baseline,
    sentinel,
    width: W,
    height: H,
    sentinelMap: { primary: '#FF0000', warning: '#00FF00', error: '#0000FF' },
  });
  assert.deepStrictEqual(findings.map((f) => f.token), ['primary']);
});

test('judge: applies the large-text threshold by component height', () => {
  const base = { token: 't', foreground: WHITE, background: [0, 0, 0], pixels: 100 };
  const [small, large] = judge(
    [
      { ...base, ratio: 3.5, bounds: { x: 0, y: 0, width: 40, height: 20 } },
      { ...base, ratio: 3.5, bounds: { x: 0, y: 300, width: 40, height: 60 } },
    ],
    { dpr: 2 },
  );
  assert.strictEqual(small.required, 4.5);
  assert.strictEqual(small.passes, false);
  assert.strictEqual(large.required, 3.0);
  assert.strictEqual(large.passes, true);
});

test('analyze: density separates glyph runs from filled surfaces', () => {
  const sheet = [GLASS_DARK, GLASS_DARK, GLASS_DARK];
  const baseline = frame(W, H, sheet);
  const sentinel = frame(W, H, sheet);
  // A solid block — a fill, not text.
  rect(baseline, W, 5, 5, 30, 20, SAGE);
  rect(sentinel, W, 5, 5, 30, 20, parseHex('#FF0000'));

  const [finding] = analyze({
    baseline,
    sentinel,
    width: W,
    height: H,
    sentinelMap: { primary: '#FF0000' },
  });
  assert.ok(finding.density > 0.99, `solid fill should be dense, got ${finding.density}`);
  assert.strictEqual(finding.pixels, 30 * 20);
});

test('judge: a dense fill is held to the non-text 3:1 rule, not 4.5:1', () => {
  // WCAG 1.4.11 governs non-text UI components. Judging a filled surface at the
  // text threshold would manufacture failures the spec does not require.
  const [surface, glyphs] = judge([
    {
      token: 'primary',
      ratio: 3.2,
      pixels: 10000,
      bounds: { x: 0, y: 0, width: 100, height: 100 },
      density: 1.0,
      foreground: WHITE,
      background: [0, 0, 0],
    },
    // Same ratio, but sparse and only ~15 CSS px tall, so the normal-text rule
    // applies rather than the large-text one.
    {
      token: 'primary',
      ratio: 3.2,
      pixels: 3000,
      bounds: { x: 0, y: 300, width: 200, height: 30 },
      density: 0.3,
      foreground: WHITE,
      background: [0, 0, 0],
    },
  ]);
  assert.strictEqual(surface.isSurface, true);
  assert.strictEqual(surface.required, 3.0);
  assert.strictEqual(surface.passes, true);
  assert.strictEqual(glyphs.isSurface, false);
  assert.strictEqual(glyphs.passes, false, 'sparse text at 3.2:1 must still fail 4.5:1');
});

test('ringBackground: stays bounded on a component covering most of the frame', () => {
  // A full-frame mask would otherwise walk ~24 ring cells per pixel into one
  // array. This must complete quickly and still return the right colour.
  const bg = [10, 20, 30];
  const baseline = frame(W, H, bg);
  const sentinel = frame(W, H, bg);
  rect(baseline, W, 0, 0, W, H - 6, WHITE);
  rect(sentinel, W, 0, 0, W, H - 6, parseHex('#FF0000'));

  const sentinelRgb = [parseHex('#FF0000')];
  const { core, owned } = classify(baseline, sentinel, sentinelRgb);
  const started = process.hrtime.bigint();
  const result = ringBackground(baseline, core.get(0), owned.get(0), W, H);
  const elapsedMs = Number(process.hrtime.bigint() - started) / 1e6;

  assert.deepStrictEqual(result, bg);
  assert.ok(elapsedMs < 500, `ring sampling took ${elapsedMs.toFixed(0)}ms`);
});

test('judge: honours per-token roles instead of holding everything to 4.5:1', () => {
  const bounds = { x: 0, y: 0, width: 200, height: 30 };
  const base = { pixels: 3000, bounds, density: 0.3, background: [0, 0, 0] };
  const roles = {
    'text-primary': 'text',
    'text-faint': 'text-faint',
    border: 'decorative',
    'primary-hover': 'ui',
  };
  const [text, faint, border, ui] = judge(
    [
      { ...base, token: 'text-primary', ratio: 3.5, foreground: [1, 1, 1] },
      // tokens.json documents text-faint as clearing 3:1, never body copy —
      // failing it at 4.5:1 contradicts its own contract.
      { ...base, token: 'text-faint', ratio: 3.5, foreground: [2, 2, 2] },
      // A hairline divider carries no information; WCAG sets no floor for it.
      { ...base, token: 'border', ratio: 1.1, foreground: [3, 3, 3] },
      { ...base, token: 'primary-hover', ratio: 3.5, foreground: [4, 4, 4] },
    ],
    { roles },
  );

  assert.strictEqual(text.required, 4.5);
  assert.strictEqual(text.passes, false);
  assert.strictEqual(faint.required, 3.0);
  assert.strictEqual(faint.passes, true);
  assert.strictEqual(border.required, null);
  assert.strictEqual(border.passes, true, 'decorative tokens are reported, never failed');
  assert.strictEqual(ui.required, 3.0);
  assert.strictEqual(ui.passes, true);
});

test('judge: does not fail on a foreground that does not match the token', () => {
  // A thin glyph run is mostly anti-aliased edge, so its median blends toward
  // the background and reads far darker than the token really is. That is a
  // measurement artifact, not a colour defect — reported, but not a failure.
  const bounds = { x: 0, y: 0, width: 200, height: 30 };
  //
  // Note the check only bites when the token and its background are far apart —
  // which is exactly when a blended reading is most misleading. Where they are
  // close (sage on dark glass) blending barely moves the value, and the
  // measurement stands on its own.
  const values = {
    light: { 'text-primary': '#191C19' },
    dark: { 'text-primary': '#F4F4EF' },
  };
  const [faithful, blended] = judge(
    [
      {
        token: 'text-primary',
        ratio: 2.0,
        pixels: 3000,
        bounds,
        density: 0.3,
        foreground: [0xf4, 0xf4, 0xef], // exactly the dark token
        background: [120, 120, 120],
      },
      {
        token: 'text-primary',
        ratio: 2.0,
        pixels: 80,
        bounds,
        density: 0.3,
        foreground: [150, 150, 148], // half-covered: nowhere near either value
        background: [120, 120, 120],
      },
    ],
    { roles: { 'text-primary': 'text' }, values },
  );

  assert.strictEqual(faithful.lowConfidence, false);
  assert.strictEqual(faithful.passes, false, 'a real 2.0:1 must still fail');
  assert.strictEqual(blended.lowConfidence, true);
  assert.strictEqual(blended.passes, true, 'artifact measurements must not fail the gate');
});

test('judge: matches against either theme, since one capture renders one theme', () => {
  // Content screens over media use dark chrome even in light mode, so the
  // rendered value is not always the app's nominal theme.
  const bounds = { x: 0, y: 0, width: 200, height: 30 };
  const [finding] = judge(
    [
      {
        token: 'primary',
        ratio: 6.2,
        pixels: 3000,
        bounds,
        density: 0.3,
        foreground: [0x9d, 0xbf, 0xa8], // the DARK token, captured in light mode
        background: [52, 52, 52],
      },
    ],
    {
      roles: { primary: 'text' },
      values: { light: { primary: '#3E5A47' }, dark: { primary: '#9DBFA8' } },
    },
  );
  assert.strictEqual(finding.lowConfidence, false);
});

test('judge: a glyph fragment inherits the height of the line it sits on', () => {
  // Segmentation splits text into connected glyph runs, so the dot of an "i" is
  // its own ~9px component inside a 48px title. Judged alone it would be held
  // to the normal-text threshold while the letters beside it qualify as large
  // text — a failure invented by the measurement, not present in the design.
  const base = {
    token: 'title', pixels: 300, density: 0.3,
    foreground: WHITE, background: [147, 147, 147], ratio: 3.07,
  };
  const [letter, dot] = judge(
    [
      { ...base, bounds: { x: 0, y: 100, width: 30, height: 50 } }, // a tall letter
      { ...base, bounds: { x: 40, y: 104, width: 8, height: 9 } },  // the i-dot
    ],
    { roles: { title: 'text' }, dpr: 2 },
  );
  assert.strictEqual(letter.isLarge, true);
  assert.strictEqual(dot.isLarge, true, 'the dot belongs to the same line');
  assert.strictEqual(dot.required, 3.0);
  assert.strictEqual(dot.passes, true);
});

test('judge: a genuinely small run on its own line stays normal text', () => {
  // The grouping must not promote unrelated small text just because something
  // large exists elsewhere in the frame.
  const base = {
    token: 'caption', pixels: 300, density: 0.3,
    foreground: WHITE, background: [147, 147, 147], ratio: 3.07,
  };
  const [title, caption] = judge(
    [
      { ...base, bounds: { x: 0, y: 100, width: 30, height: 50 } },
      { ...base, bounds: { x: 0, y: 400, width: 30, height: 14 } }, // far below
    ],
    { roles: { caption: 'text' }, dpr: 2 },
  );
  assert.strictEqual(title.isLarge, true);
  assert.strictEqual(caption.isLarge, false);
  assert.strictEqual(caption.required, 4.5);
  assert.strictEqual(caption.passes, false);
});

test('judge: lines are grouped per token, not across tokens', () => {
  // Two tokens can share a baseline (a label and its value). Sizing one from
  // the other would let a large heading silently exempt small text beside it.
  const base = {
    pixels: 300, density: 0.3, foreground: WHITE,
    background: [147, 147, 147], ratio: 3.07,
  };
  const [, small] = judge(
    [
      { ...base, token: 'heading', bounds: { x: 0, y: 100, width: 30, height: 50 } },
      { ...base, token: 'meta', bounds: { x: 60, y: 120, width: 30, height: 12 } },
    ],
    { roles: { heading: 'text', meta: 'text' }, dpr: 2 },
  );
  assert.strictEqual(small.isLarge, false);
});
