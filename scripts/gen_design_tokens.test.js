'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const path = require('path');

const {
  COLOR_KEYS,
  THEMES,
  contrastRatio,
  parseHex,
  validate,
  checkContrast,
  buildDart,
  buildCss,
  markdownToHtml,
  generate,
  sentinelHues,
  sentinelTokens,
  generateSentinel,
  farthestPointColors,
  MATERIALS,
  applyCandidates,
} = require('./gen_design_tokens.js');

const REAL_TOKENS = JSON.parse(
  fs.readFileSync(path.join(__dirname, '..', 'design', 'tokens.json'), 'utf8'),
);

function clone(o) {
  return JSON.parse(JSON.stringify(o));
}

test('contrastRatio: known values', () => {
  assert.ok(Math.abs(contrastRatio('#FFFFFF', '#000000') - 21) < 0.01);
  assert.ok(Math.abs(contrastRatio('#FFFFFF', '#FFFFFF') - 1) < 0.001);
  // Symmetric regardless of argument order.
  assert.strictEqual(
    contrastRatio('#3E5A47', '#FFFFFF'),
    contrastRatio('#FFFFFF', '#3E5A47'),
  );
});

test('parseHex: rejects malformed colors', () => {
  assert.throws(() => parseHex('#fff'));
  assert.throws(() => parseHex('#3e5a47')); // lowercase rejected — canonical form is uppercase
  assert.throws(() => parseHex('3E5A47'));
  assert.throws(() => parseHex('#3E5A4G'));
  assert.deepStrictEqual(parseHex('#FF0080'), [255, 0, 128]);
});

test('validate: real tokens.json is valid', () => {
  assert.deepStrictEqual(validate(REAL_TOKENS), []);
});

test('validate: catches missing key, bad hex, unknown key, missing description', () => {
  const missing = clone(REAL_TOKENS);
  delete missing.color.light.primary;
  assert.ok(validate(missing).some((e) => e.includes('color.light.primary')));

  const badHex = clone(REAL_TOKENS);
  badHex.color.dark.error.$value = 'red';
  assert.ok(validate(badHex).some((e) => e.includes('color.dark.error')));

  const unknown = clone(REAL_TOKENS);
  unknown.color.light.tertiary = { $type: 'color', $value: '#123456', $description: 'x' };
  assert.ok(validate(unknown).some((e) => e.includes('unknown key')));

  const noDesc = clone(REAL_TOKENS);
  delete noDesc.color.light.surface.$description;
  assert.ok(validate(noDesc).some((e) => e.includes('missing $description')));
});

test('contrast gate: real tokens pass every declared pair', () => {
  const { failures } = checkContrast(REAL_TOKENS);
  assert.deepStrictEqual(failures, []);
});

test('contrast gate: detects a failing pair', () => {
  const bad = clone(REAL_TOKENS);
  // Legacy shared warning on white — the documented sub-3:1 failure.
  bad.color.light.warning.$value = '#E8A661';
  const { failures } = checkContrast(bad);
  assert.ok(failures.some((f) => f.includes('color.light: warning')));
});

test('generate: throws on contrast failure (the gate is load-bearing)', () => {
  const bad = clone(REAL_TOKENS);
  bad.color.light['text-primary'].$value = '#CCCCCC';
  assert.throws(() => generate(bad, ''), /contrast gate failed/);
});

test('buildDart: emits every color constant and the font families', () => {
  const dart = buildDart(REAL_TOKENS);
  for (const theme of ['light', 'dark']) {
    for (const key of COLOR_KEYS) {
      const name = theme + key.split('-').map((w) => w[0].toUpperCase() + w.slice(1)).join('');
      assert.ok(dart.includes(`static const Color ${name} = Color(0xFF`), `missing ${name}`);
    }
  }
  assert.ok(dart.includes(`serifFamily = '${REAL_TOKENS.font.serif.$value[0]}'`));
  assert.ok(dart.includes(`sansFamily = '${REAL_TOKENS.font.sans.$value[0]}'`));
  assert.ok(dart.includes('GENERATED from design/tokens.json'));
});

test('buildCss: emits light values in :root and dark overrides in the media query', () => {
  const css = buildCss(REAL_TOKENS);
  assert.ok(css.includes(`--color-primary: ${REAL_TOKENS.color.light.primary.$value};`));
  // The file-header comment also mentions the media query — take the last
  // occurrence, which is the real block.
  const darkBlock = css.slice(css.lastIndexOf('@media (prefers-color-scheme: dark)'));
  assert.ok(darkBlock.startsWith('@media'), 'missing dark media query');
  assert.ok(darkBlock.includes(`--color-primary: ${REAL_TOKENS.color.dark.primary.$value};`));
  assert.ok(css.includes('--font-serif:'));
  // Fixed-theme variables for surfaces that don't follow the OS preference.
  assert.ok(css.includes(`--dark-primary: ${REAL_TOKENS.color.dark.primary.$value};`));
  assert.ok(css.includes(`--light-background: ${REAL_TOKENS.color.light.background.$value};`));
});

test('markdownToHtml: headings, lists, bold, code', () => {
  const html = markdownToHtml('# T\n\nPara with **bold** and `code`.\n\n- one\n- two\n');
  assert.ok(html.includes('<h2>T</h2>'));
  assert.ok(html.includes('<b>bold</b>'));
  assert.ok(html.includes('<code>code</code>'));
  assert.ok(html.includes('<li>one</li>'));
});

test('generate: deterministic — same input, byte-identical output', () => {
  const a = generate(clone(REAL_TOKENS), '# P\n\ntext\n');
  const b = generate(clone(REAL_TOKENS), '# P\n\ntext\n');
  assert.deepStrictEqual(a, b);
});

test('generated outputs on disk are current (mirrors the CI gate)', () => {
  const prose = fs.readFileSync(path.join(__dirname, '..', 'design', 'guidelines.md'), 'utf8');
  const outputs = generate(REAL_TOKENS, prose);
  for (const [rel, content] of Object.entries(outputs)) {
    const onDisk = fs.readFileSync(path.join(__dirname, '..', rel), 'utf8');
    assert.strictEqual(onDisk, content, `${rel} is stale — run npm run generate:design-tokens`);
  }
});

// --- Evaluation candidates (#2770) -----------------------------------------

test('applyCandidates: overrides only what the candidate states', () => {
  const merged = applyCandidates(REAL_TOKENS, [
    { id: 'c', color: { light: { primary: { $value: '#2F4A56' } } } },
  ]);
  assert.strictEqual(merged.color.light.primary.$value, '#2F4A56');
  // Untouched values, and the metadata the candidate did not restate, survive.
  assert.strictEqual(merged.color.dark.primary.$value, REAL_TOKENS.color.dark.primary.$value);
  assert.strictEqual(merged.color.light.primary.$type, 'color');
  assert.ok(merged.color.light.primary.$description);
  // Base tree is not mutated — candidates are swept in sequence off one base.
  assert.strictEqual(REAL_TOKENS.color.light.primary.$value, '#3E5A47');
});

test('applyCandidates: merges the glass axis independently of the palette', () => {
  const merged = applyCandidates(REAL_TOKENS, [
    { id: 'p', color: { light: { primary: { $value: '#2F4A56' } } } },
    { id: 'g', glass: { scrim: { $value: '#B3000000' } } },
  ]);
  assert.strictEqual(merged.color.light.primary.$value, '#2F4A56');
  assert.strictEqual(merged.glass.scrim.$value, '#B3000000');
  assert.strictEqual(merged.glass.surface.$value, REAL_TOKENS.glass.surface.$value);
});

test('applyCandidates: rejects a key that does not exist', () => {
  // A typo would otherwise silently render the base value and be read as
  // "this candidate changed nothing", which is the wrong conclusion entirely.
  assert.throws(
    () => applyCandidates(REAL_TOKENS, [{ id: 'c', color: { light: { primry: { $value: '#000000' } } } }]),
    /unknown color\.light\.primry/,
  );
  assert.throws(
    () => applyCandidates(REAL_TOKENS, [{ id: 'c', glass: { scrimm: { $value: '#B3000000' } } }]),
    /unknown glass\.scrimm/,
  );
});

test('applyCandidates: a candidate gets no exemption from the contrast gate', () => {
  const bad = applyCandidates(REAL_TOKENS, [
    { id: 'bad', color: { light: { 'text-primary': { $value: '#CCCCCC' } } } },
  ]);
  const { failures } = checkContrast(bad);
  assert.ok(failures.length > 0);
  assert.throws(() => generate(bad, ''), /contrast gate failed/);
});

test('every candidate on disk clears validation and the contrast gate', () => {
  // The candidates exist to be judged by the gate; one that cannot pass it is a
  // bug in the candidate, and should fail here rather than mid-sweep.
  const dir = path.join(__dirname, '..', 'design', 'candidates');
  const files = fs.readdirSync(dir).filter((f) => f.endsWith('.json'));
  assert.ok(files.length >= 2, 'expected candidate files on disk');
  for (const file of files) {
    const candidate = JSON.parse(fs.readFileSync(path.join(dir, file), 'utf8'));
    assert.ok(candidate.id, `${file}: missing id`);
    assert.ok(['palette', 'glass'].includes(candidate.axis), `${file}: axis must be palette or glass`);
    assert.ok(candidate.label, `${file}: missing label`);
    const merged = applyCandidates(REAL_TOKENS, [candidate]);
    assert.deepStrictEqual(validate(merged), [], `${file}: invalid`);
    assert.deepStrictEqual(checkContrast(merged).failures, [], `${file}: fails the contrast gate`);
  }
});

// --- Sentinel mode (#2770) -------------------------------------------------

test('sentinelHues: covers palette and glass, excluding reference surfaces', () => {
  const hues = sentinelHues(REAL_TOKENS);
  // Reference surfaces are what foregrounds are measured *against* — sentineling
  // them would repaint most of the frame and yield "surface on surface".
  assert.ok(!('background' in hues));
  assert.ok(!('surface' in hues));
  assert.ok(!('glass.surface' in hues));
  assert.ok(!('glass.scrim' in hues));
  assert.ok(!('glass.fill-strong' in hues));
  // Glass keys are namespaced: `primary`, `text-primary` and `border` exist in
  // both sets, and conflating them would attribute on-glass pixels to the
  // palette token.
  assert.ok('glass.primary' in hues);
  assert.ok('primary' in hues);
  assert.notStrictEqual(hues['glass.primary'], hues.primary);
  for (const [key, hex] of Object.entries(hues)) {
    assert.match(hex, /^#[0-9A-F]{6}$/, `${key} -> ${hex}`);
  }
});

test('sentinelHues: neighbours stay far apart in RGB', () => {
  // Classification assigns each changed pixel to its nearest sentinel, so hues
  // that crowd together would make anti-aliased glyph edges ambiguous.
  const values = Object.values(sentinelHues(REAL_TOKENS)).map(parseHex);
  let min = Infinity;
  for (let i = 0; i < values.length; i++) {
    for (let j = i + 1; j < values.length; j++) {
      const d = Math.sqrt(
        values[i].reduce((acc, v, k) => acc + (v - values[j][k]) ** 2, 0),
      );
      min = Math.min(min, d);
    }
  }
  // Classification takes the *nearest* sentinel, so the invariant that matters
  // is that no pure sentinel falls inside another's core window (radius 70 in
  // contrast_analyzer.mjs). Robustness against partially-covered edge pixels
  // comes from taking a median over many core pixels, not from separation
  // alone. The lightness cycle buys ~102 here; even hues alone give only 72.
  assert.ok(min > 70, `closest sentinel pair is ${min.toFixed(1)} apart`);
});

test('sentinelHues: every real token differs enough from its sentinel to register', () => {
  // A mask is built from pixels where |baseline - sentinel| exceeds the change
  // threshold (30, summed across channels). If any token happened to sit near
  // its own sentinel, its glyphs would silently drop out of the mask and the
  // gate would report nothing rather than a failure — the worst way to be
  // wrong. Checked against the real palette, not in the abstract.
  // The bar is the analyzer's change threshold (30, summed across channels).
  // Requiring 2x that leaves headroom for partially-covered pixels: a glyph at
  // 50% coverage still moves ~30 and stays inside the mask, while only the
  // faintest anti-aliased edges drop out — which is what the core filter
  // discards anyway.
  const MIN_DELTA = 60;
  const hues = sentinelHues(REAL_TOKENS);
  let tightest = Infinity;
  let tightestName = '';
  const check = (name, realHex, sentinelHex) => {
    const real = parseHex(realHex);
    const sentinel = parseHex(sentinelHex);
    const delta = real.reduce((acc, v, i) => acc + Math.abs(v - sentinel[i]), 0);
    if (delta < tightest) { tightest = delta; tightestName = name; }
    assert.ok(delta > MIN_DELTA, `${name} (${realHex}) vs sentinel ${sentinelHex}: delta ${delta}`);
  };
  for (const key of Object.keys(hues)) {
    const dot = key.indexOf('.');
    if (dot === -1) {
      for (const theme of THEMES) {
        check(`${theme}.${key}`, REAL_TOKENS.color[theme][key].$value, hues[key]);
      }
    } else {
      // Material values carry alpha; compare the RGB the sentinel replaces.
      const [mat, k] = [key.slice(0, dot), key.slice(dot + 1)];
      check(key, '#' + REAL_TOKENS[mat][k].$value.slice(3), hues[key]);
    }
  }
  assert.ok(tightest > MIN_DELTA, `tightest pair was ${tightestName} at ${tightest}`);
});

test('sentinelTokens: a colour key shares one hue across both themes', () => {
  // Only one theme renders per capture, so light.primary and dark.primary never
  // co-occur — sharing a hue halves the count and doubles the separation.
  const sentinel = sentinelTokens(REAL_TOKENS);
  for (const key of Object.keys(sentinelHues(REAL_TOKENS))) {
    if (key.includes('.')) continue; // materials are one set, not per-theme
    assert.strictEqual(
      sentinel.color.light[key].$value,
      sentinel.color.dark[key].$value,
      `${key} should share a hue across themes`,
    );
  }
});

test('sentinelTokens: sentineled colours are replaced, excluded ones are not', () => {
  const sentinel = sentinelTokens(REAL_TOKENS);
  const hues = sentinelHues(REAL_TOKENS);
  for (const theme of THEMES) {
    for (const key of COLOR_KEYS) {
      if (key in hues) {
        assert.notStrictEqual(
          sentinel.color[theme][key].$value,
          REAL_TOKENS.color[theme][key].$value,
          `${theme}.${key} should have been replaced`,
        );
      } else {
        assert.strictEqual(
          sentinel.color[theme][key].$value,
          REAL_TOKENS.color[theme][key].$value,
          `${theme}.${key} is the measurement reference and must keep its value`,
        );
      }
    }
  }
  assert.deepStrictEqual(sentinel.font, REAL_TOKENS.font);
  // The source tree must not be mutated in place.
  assert.strictEqual(REAL_TOKENS.color.light.primary.$value, '#3E5A47');
});

test('generateSentinel: emits the Dart bundle and the classification map', () => {
  const outputs = generateSentinel(clone(REAL_TOKENS));
  const paths = Object.keys(outputs);
  assert.ok(paths.some((p) => p.endsWith('design_tokens.gen.dart')));
  assert.ok(paths.some((p) => p.endsWith('glass_tokens.gen.dart')));
  assert.ok(paths.some((p) => p.endsWith('overlay_tokens.gen.dart')));
  assert.ok(paths.some((p) => p.endsWith('sentinel_map.json')));
  // The app bundle (palette + every material) plus the map. Rewriting CSS/Go/
  // HTML for a throwaway sentinel build would be churn nobody reads.
  assert.strictEqual(paths.length, 2 + Object.keys(MATERIALS).length);

  const map = JSON.parse(outputs['design/sentinel_map.json']);
  for (const key of COLOR_KEYS) {
    if (key === 'background' || key === 'surface') continue;
    assert.ok(key in map.hues, `missing palette key ${key}`);
  }
  assert.ok('glass.primary' in map.hues);
  assert.ok('overlay.text-faint' in map.hues);
  assert.strictEqual(map.roles['glass.primary'], 'text');
  assert.strictEqual(map.roles['glass.border'], 'decorative');
  assert.strictEqual(map.roles['overlay.text-faint'], 'text-faint');
  // Glass values are reported WITHOUT alpha — the analyzer compares against a
  // composited foreground, which is what a full-coverage pixel actually shows.
  //
  // Derived from tokens.json rather than pinned to a literal. The pinned form
  // asserted '#3E5A47' and went red the moment glass.primary changed, which it
  // did (the light-theme green was wrong on an always-dark material, #2764) —
  // failing for a token edit that was correct, while testing nothing the
  // generator actually does. What matters is that the map reports the token's
  // REAL rgb, whatever that is.
  const glassPrimary = REAL_TOKENS.glass.primary.$value; // #AARRGGBB
  const glassPrimaryRgb = `#${glassPrimary.slice(3).toUpperCase()}`;
  assert.strictEqual(map.values.light['glass.primary'], glassPrimaryRgb);

  const glassDart = outputs['app/lib/core/theme/gen/glass_tokens.gen.dart'];
  assert.ok(
    !glassDart.includes(glassPrimary.toUpperCase()),
    'the real glass.primary must not survive the sentinel swap',
  );
  // Alpha must be preserved: a 50%-alpha token repainted opaque would cover
  // different pixels than the app actually draws.
  assert.ok(/static const Color textFaint = Color\(0x80/.test(glassDart));
  // Roles drive the threshold; values let the analyzer tell a genuinely bad
  // colour from a low-coverage glyph fragment.
  assert.strictEqual(map.roles['text-faint'], 'text-faint');
  assert.strictEqual(map.roles.border, 'decorative');
  assert.strictEqual(map.roles['text-primary'], 'text');
  assert.strictEqual(map.values.light.primary, '#3E5A47');
  assert.strictEqual(map.values.dark.primary, '#9DBFA8');

  const dart = outputs['app/lib/core/theme/gen/design_tokens.gen.dart'];
  assert.ok(!dart.includes('0xFF3E5A47'), 'real sage must not survive the swap');
  assert.ok(dart.includes('lightPrimary'));
});

test('generateSentinel: skips the contrast gate that sentinel hues cannot pass', () => {
  // Hues are chosen for mutual separation, not legibility, so the normal gate
  // would reject them by construction — the sentinel bundle is throwaway.
  const sentinel = sentinelTokens(REAL_TOKENS);
  const { failures } = checkContrast(sentinel);
  assert.ok(failures.length > 0, 'sentinel palette is expected to fail the gate');
  assert.doesNotThrow(() => generateSentinel(clone(REAL_TOKENS)));
});

test('generateSentinel: still validates the source token file', () => {
  const broken = clone(REAL_TOKENS);
  delete broken.color.light.primary;
  assert.throws(() => generateSentinel(broken), /invalid/);
});

test('farthestPointColors: deterministic and well separated', () => {
  // The map must match the bundle built from it, so selection cannot depend on
  // iteration order or randomness.
  const a = farthestPointColors(27);
  const b = farthestPointColors(27);
  assert.deepStrictEqual(a, b);
  assert.strictEqual(a.length, 27);

  let min = Infinity;
  for (let i = 0; i < a.length; i++) {
    for (let j = i + 1; j < a.length; j++) {
      min = Math.min(min, Math.sqrt(a[i].reduce((s, v, k) => s + (v - a[j][k]) ** 2, 0)));
    }
  }
  // Spreading hues round a circle tops out near 57 at this count, inside the
  // analyzer's core window (70). Picking from the whole cube clears it.
  assert.ok(min > 70, `closest pair ${min.toFixed(1)} — inside the core window`);
});

test('every material is registered coherently', () => {
  for (const [name, spec] of Object.entries(MATERIALS)) {
    assert.ok(spec.className && spec.output, `${name}: missing className/output`);
    for (const key of spec.sentinelExcluded) {
      assert.ok(spec.colorKeys.includes(key), `${name}: excluded key ${key} is not a colour key`);
    }
    for (const key of Object.keys(spec.roles)) {
      assert.ok(spec.colorKeys.includes(key), `${name}: role for unknown key ${key}`);
    }
    for (const [a, b] of spec.distinctPairs) {
      assert.ok(spec.colorKeys.includes(a) && spec.colorKeys.includes(b), `${name}: bad distinct pair`);
    }
    // A material whose every key is excluded would be invisible to the gate.
    assert.ok(
      spec.colorKeys.some((k) => !spec.sentinelExcluded.includes(k)),
      `${name}: no measurable keys — the gate would never see this material`,
    );
  }
});

test('validate: a divider identical to its surface is rejected', () => {
  const bad = clone(REAL_TOKENS);
  bad.glass.divider.$value = bad.glass.surface.$value;
  assert.ok(validate(bad).some((e) => /divider: identical to glass\.surface/.test(e)));
});

test('validate: material colours must carry alpha', () => {
  const bad = clone(REAL_TOKENS);
  bad.overlay['scrim-top'].$value = '#000000';
  assert.ok(validate(bad).some((e) => /overlay\.scrim-top.*AARRGGBB/.test(e)));
});

test('sentinelTokens: material sentinels preserve the original alpha', () => {
  // A 50%-alpha token repainted opaque covers different pixels than the app
  // draws, so the mask would describe a shape that never rendered.
  const sentinel = sentinelTokens(REAL_TOKENS);
  for (const [name, spec] of Object.entries(MATERIALS)) {
    for (const key of spec.colorKeys) {
      assert.strictEqual(
        sentinel[name][key].$value.slice(1, 3),
        REAL_TOKENS[name][key].$value.slice(1, 3),
        `${name}.${key}: alpha changed`,
      );
    }
  }
});
