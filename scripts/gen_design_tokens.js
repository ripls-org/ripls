#!/usr/bin/env node
/**
 * gen_design_tokens.js — generate Dart, CSS, and HTML from design/tokens.json.
 *
 * design/tokens.json (W3C Design Tokens format) is the single source of truth
 * for brand/neutral/semantic colors and font families (issue #2441). This
 * script emits the three consumed artifacts, all checked in:
 *
 *   app/lib/core/theme/gen/design_tokens.gen.dart   raw constants for AppColors
 *   server/services/web/static/css/gen/tokens.gen.css          CSS custom properties
 *   design/guidelines.html                          human-viewable guidelines
 *                                                   (prose from design/guidelines.md)
 *
 * Generation FAILS if any declared contrast pair (CONTRAST_PAIRS) falls below
 * its WCAG threshold — the palette cannot regress below AA by construction.
 *
 * Modes:
 *   node scripts/gen_design_tokens.js            Write all outputs.
 *   node scripts/gen_design_tokens.js --check     Verify outputs are current
 *                                                 (regenerate-and-diff); used as
 *                                                 the CI gate (lint:design-tokens).
 *
 * No dependencies — plain Node, like check_versions.js. Output is
 * deterministic (no timestamps) so regenerate-and-diff is byte-exact.
 */

'use strict';

const fs = require('fs');
const path = require('path');

const ROOT = path.resolve(__dirname, '..');
const TOKENS_REL = 'design/tokens.json';
const PROSE_REL = 'design/guidelines.md';
const OUTPUTS = {
  dart: 'app/lib/core/theme/gen/design_tokens.gen.dart',
  // Material outputs come from MATERIALS[*].output — see generate().
  css: 'server/services/web/static/css/gen/tokens.gen.css',
  go: 'server/email/design_tokens_gen.go',
  html: 'design/guidelines.html',
};

// Every theme must define exactly these color keys, in this order.
const COLOR_KEYS = [
  'background', 'surface', 'border',
  'text-primary', 'text-secondary', 'text-faint',
  'primary', 'on-primary', 'primary-hover',
  'accent', 'on-accent',
  'success', 'warning', 'error', 'info',
];
const THEMES = ['light', 'dark'];
const STATUS_KEYS = ['success', 'warning', 'error', 'info'];

// ---------------------------------------------------------------------------
// Materials (#2770)
// ---------------------------------------------------------------------------
//
// A "material" is a set of translucent values painted over content the palette
// cannot predict — a frosted sheet above the page, or a wash applied to a user's
// photo. Two things make them unlike the palette:
//
//   * Alpha is load-bearing, so values are #AARRGGBB.
//   * They are NOT per-theme. The backdrop, not the app theme, decides what is
//     behind them.
//
// They are the surfaces the #2441 contrast gate structurally cannot see, which
// is why both accumulated dozens of hand-tuned constants and why the failures
// concentrate here. Registered in a table rather than hand-coded per material:
// the second one arriving is the signal that this wants a primitive.
const MATERIALS = {
  glass: {
    className: 'GlassTokens',
    output: 'app/lib/core/theme/gen/glass_tokens.gen.dart',
    summary: 'Frosted-glass material — sheets and modals floating above content.',
    detail: [
      'One set, not per-theme: the sheet sits on a dark scrim that handles theme',
      'adaptation, so on-glass content is identical in light and dark.',
    ],
    colorKeys: [
      'scrim', 'scrim-heavy', 'scrim-tint',
      'surface', 'border', 'border-soft', 'border-active', 'divider', 'hairline',
      'fill-faint', 'fill-subtle', 'fill-strong', 'on-fill-strong', 'drag-handle',
      'text-primary', 'text-secondary', 'text-muted', 'text-faint',
      'primary', 'on-primary',
      'secondary-fill', 'secondary-border',
    ],
    numberKeys: ['blur-sigma', 'backdrop-blur-sigma'],
    // Surfaces are what foregrounds are measured against, never subjects.
    sentinelExcluded: [
      'scrim', 'scrim-heavy', 'scrim-tint', 'surface',
      'fill-faint', 'fill-subtle', 'fill-strong', 'secondary-fill',
    ],
    roles: {
      'text-primary': 'text',
      'text-secondary': 'text',
      'text-muted': 'text',
      'text-faint': 'text-faint',
      primary: 'text',
      'on-primary': 'text',
      'on-fill-strong': 'text',
      border: 'decorative',
      'border-soft': 'decorative',
      'border-active': 'ui',
      divider: 'decorative',
      hairline: 'decorative',
      'drag-handle': 'decorative',
      'secondary-border': 'ui',
    },
    // A divider that equals the surface it divides is invisible by construction.
    distinctPairs: [['divider', 'surface']],
  },
  overlay: {
    className: 'OverlayTokens',
    output: 'app/lib/core/theme/gen/overlay_tokens.gen.dart',
    summary: 'Media-overlay material — scrims and controls painted onto user photos.',
    detail: [
      'A different material from glass: glass floats above content, this is a',
      'wash on the media itself. Not per-theme — the photo is the backdrop and',
      'does not follow the app theme.',
    ],
    colorKeys: [
      'scrim-top', 'scrim-bottom', 'scrim-flat', 'scrim-floor',
      'wash-top', 'wash-bottom',
      'outline', 'chip-fill', 'control-fill', 'field-fill',
      'text-primary', 'text-secondary', 'text-faint',
    ],
    numberKeys: [],
    // Backdrops, not foregrounds. The sentinel pass recolours a token to
    // attribute pixels to it; doing that to a wash repaints the photo and the
    // diff stops meaning "this token is here".
    sentinelExcluded: [
      'scrim-top', 'scrim-bottom', 'scrim-flat', 'scrim-floor',
      'wash-top', 'wash-bottom',
      'chip-fill', 'control-fill', 'field-fill',
    ],
    roles: {
      'text-primary': 'text',
      'text-secondary': 'text',
      'text-faint': 'text-faint',
      outline: 'decorative',
    },
    distinctPairs: [],
  },
  completion: {
    className: 'CompletionTokens',
    output: 'app/lib/core/theme/gen/completion_tokens.gen.dart',
    summary: 'Completion/fulfillment sheet brand colours.',
    detail: [
      'Small on purpose: these sheets render on always-dark glass (#2492), so',
      'their surfaces, text and borders come from GlassTokens. Only the sheet',
      'base and the three brand hues are their own.',
    ],
    colorKeys: ['sheet', 'green', 'green-light', 'accent-light'],
    numberKeys: [],
    sentinelExcluded: ['sheet'],
    roles: {
      green: 'ui',
      'green-light': 'ui',
      'accent-light': 'ui',
    },
    distinctPairs: [],
  },
  paper: {
    className: 'PaperTokens',
    output: 'app/lib/core/theme/gen/paper_tokens.gen.dart',
    summary: 'Impact-paper material — the warm printed-report surface of the impact and chart views.',
    detail: [
      'Opaque and light-only, unlike glass and overlay: the impact views have no',
      'dark treatment today. The category-* ramp is a SEMANTIC scale, not decoration',
      '— the same category must read as the same colour on every screen.',
    ],
    colorKeys: [
      'surface', 'surface-raised',
      'border', 'border-subtle', 'border-strong',
      'text-primary', 'text-secondary', 'text-muted', 'text-faint', 'ink',
      'accent-coral', 'accent-sage', 'accent-gold', 'accent-green',
      'accent-forest', 'accent-forest-light', 'accent-teal', 'accent-teal-light',
      'accent-blue', 'accent-bronze', 'accent-green-strong', 'accent-blue-strong',
      'tint-amber', 'tint-teal',
      'highlight', 'ember',
    ],
    numberKeys: [],
    // Surfaces and the accent fills are backdrops or shapes, never the subject
    // of a contrast measurement.
    sentinelExcluded: [
      'surface', 'surface-raised', 'tint-amber', 'tint-teal',
      'accent-coral', 'accent-sage', 'accent-gold', 'accent-green',
      'accent-forest', 'accent-forest-light', 'accent-teal', 'accent-teal-light',
      'accent-blue', 'accent-bronze', 'accent-green-strong', 'accent-blue-strong',
    ],
    roles: {
      'text-primary': 'text',
      'text-secondary': 'text',
      'text-muted': 'text',
      'text-faint': 'text-faint',
      ink: 'text',
      border: 'decorative',
      'border-subtle': 'decorative',
      'border-strong': 'decorative',
      highlight: 'ui',
      ember: 'ui',
    },
    // A subtle border that equals the raised surface it outlines is invisible.
    distinctPairs: [['border-subtle', 'surface-raised']],
  },
};

const MATERIAL_NAMES = Object.keys(MATERIALS);

// Load-bearing pairs checked per theme at generation time. [fg, bg, min].
// 4.5 = WCAG AA normal text; 3.0 = large text / UI components.
const CONTRAST_PAIRS = [
  ['text-primary', 'background', 4.5],
  ['text-primary', 'surface', 4.5],
  ['text-secondary', 'background', 4.5],
  ['text-secondary', 'surface', 4.5],
  ['text-faint', 'background', 3.0],
  ['on-primary', 'primary', 4.5],
  ['on-primary', 'primary-hover', 4.5],
  ['primary', 'background', 4.5],
  ['on-accent', 'accent', 4.5],
  ...STATUS_KEYS.flatMap((s) => [[s, 'background', 4.5], [s, 'surface', 4.5]]),
];

// ---------------------------------------------------------------------------
// Color math
// ---------------------------------------------------------------------------

function parseHex(hex) {
  if (typeof hex !== 'string' || !/^#[0-9A-F]{6}$/.test(hex)) {
    throw new Error(`invalid color "${hex}" — must be uppercase #RRGGBB`);
  }
  return [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16));
}

/** Glass colours carry alpha, so they are #AARRGGBB rather than #RRGGBB. */
function parseArgb(hex) {
  if (typeof hex !== 'string' || !/^#[0-9A-F]{8}$/.test(hex)) {
    throw new Error(`invalid glass color "${hex}" — must be uppercase #AARRGGBB`);
  }
  return [1, 3, 5, 7].map((i) => parseInt(hex.slice(i, i + 2), 16));
}

function relativeLuminance(hex) {
  const [r, g, b] = parseHex(hex).map((v) => {
    const c = v / 255;
    return c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function contrastRatio(a, b) {
  const [hi, lo] = [relativeLuminance(a), relativeLuminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

// ---------------------------------------------------------------------------
// Validation + contrast gate
// ---------------------------------------------------------------------------

function validate(tokens) {
  const errors = [];
  for (const f of ['serif', 'sans']) {
    const t = tokens.font && tokens.font[f];
    if (!t || t.$type !== 'fontFamily' || !Array.isArray(t.$value) || t.$value.length < 2) {
      errors.push(`font.${f}: must be a fontFamily token with a fallback stack`);
    }
  }
  for (const theme of THEMES) {
    const group = tokens.color && tokens.color[theme];
    if (!group) { errors.push(`color.${theme}: missing`); continue; }
    for (const key of COLOR_KEYS) {
      const t = group[key];
      if (!t || t.$type !== 'color') { errors.push(`color.${theme}.${key}: missing or not a color token`); continue; }
      try { parseHex(t.$value); } catch (e) { errors.push(`color.${theme}.${key}: ${e.message}`); }
      if (!t.$description) errors.push(`color.${theme}.${key}: missing $description (the guidelines page needs it)`);
    }
    for (const key of Object.keys(group)) {
      if (!COLOR_KEYS.includes(key)) errors.push(`color.${theme}.${key}: unknown key — add it to COLOR_KEYS in scripts/gen_design_tokens.js or remove it`);
    }
  }
  for (const name of MATERIAL_NAMES) errors.push(...validateMaterial(tokens, name));
  return errors;
}

function validateMaterial(tokens, name) {
  const spec = MATERIALS[name];
  const errors = [];
  const mat = tokens[name];
  if (!mat) return [`${name}: missing`];
  for (const key of spec.colorKeys) {
    const t = mat[key];
    if (!t || t.$type !== 'color') { errors.push(`${name}.${key}: missing or not a color token`); continue; }
    try { parseArgb(t.$value); } catch (e) { errors.push(`${name}.${key}: ${e.message}`); }
    if (!t.$description) errors.push(`${name}.${key}: missing $description`);
  }
  for (const key of spec.numberKeys) {
    const t = mat[key];
    if (!t || t.$type !== 'number' || typeof t.$value !== 'number') {
      errors.push(`${name}.${key}: missing or not a number token`);
    }
  }
  const known = new Set([...spec.colorKeys, ...spec.numberKeys]);
  for (const key of Object.keys(mat)) {
    if (!known.has(key)) {
      errors.push(`${name}.${key}: unknown key — add it to MATERIALS.${name} in scripts/gen_design_tokens.js or remove it`);
    }
  }
  // Values that must stay distinguishable. The shipped glass had its divider
  // byte-identical to the surface it divided; encoding that in the token set
  // would make the mistake permanent.
  for (const [a, b] of spec.distinctPairs) {
    if (mat[a]?.$value && mat[b]?.$value && mat[a].$value === mat[b].$value) {
      errors.push(`${name}.${a}: identical to ${name}.${b} — these must stay distinguishable`);
    }
  }
  return errors;
}

function checkContrast(tokens) {
  const failures = [];
  const results = { light: [], dark: [] };
  for (const theme of THEMES) {
    const group = tokens.color[theme];
    for (const [fg, bg, min] of CONTRAST_PAIRS) {
      const ratio = contrastRatio(group[fg].$value, group[bg].$value);
      results[theme].push({ fg, bg, min, ratio });
      if (ratio < min) {
        failures.push(`color.${theme}: ${fg} (${group[fg].$value}) on ${bg} (${group[bg].$value}) = ${ratio.toFixed(2)}:1, needs >= ${min}:1`);
      }
    }
  }
  return { failures, results };
}

// ---------------------------------------------------------------------------
// Emitters
// ---------------------------------------------------------------------------

const GENERATED_BANNER = `GENERATED from ${TOKENS_REL} by scripts/gen_design_tokens.js — DO NOT EDIT.
Edit ${TOKENS_REL} and run \`npm run generate:design-tokens\`. CI
(\`npm run lint:design-tokens\`) fails if this file drifts from the source.`;

function pascal(key) {
  return key.split('-').map((w) => w[0].toUpperCase() + w.slice(1)).join('');
}

function dartName(theme, key) {
  return theme + pascal(key);
}

function dartColor(hex) {
  return `Color(0xFF${hex.slice(1)})`;
}

function buildDart(tokens) {
  const lines = [];
  lines.push('// ' + GENERATED_BANNER.split('\n').join('\n// '));
  lines.push('');
  lines.push("import 'dart:ui';");
  lines.push('');
  lines.push('/// Raw design-token constants (issue #2441). Consumed by `AppColors` /');
  lines.push('/// `AppTheme`, which own the semantic, theme-aware layer on top — widgets');
  lines.push('/// should keep using those, not reach for these directly.');
  lines.push('class DesignTokens {');
  lines.push('  DesignTokens._();');
  lines.push('');
  lines.push('  /// Display/heading font family.');
  lines.push(`  static const String serifFamily = '${tokens.font.serif.$value[0]}';`);
  lines.push('');
  lines.push('  /// Body/UI font family.');
  lines.push(`  static const String sansFamily = '${tokens.font.sans.$value[0]}';`);
  lines.push('');
  lines.push('  /// Fallback stack for [serifFamily].');
  lines.push('  static const List<String> serifFallbacks = [');
  for (const f of tokens.font.serif.$value.slice(1)) lines.push(`    '${f}',`);
  lines.push('  ];');
  lines.push('');
  lines.push('  /// Fallback stack for [sansFamily].');
  lines.push('  static const List<String> sansFallbacks = [');
  for (const f of tokens.font.sans.$value.slice(1)) lines.push(`    '${f}',`);
  lines.push('  ];');
  for (const theme of THEMES) {
    lines.push('');
    lines.push(`  // ${theme[0].toUpperCase()}${theme.slice(1)} theme.`);
    for (const key of COLOR_KEYS) {
      const t = tokens.color[theme][key];
      lines.push('');
      lines.push(`  /// ${t.$description}`);
      lines.push(`  static const Color ${dartName(theme, key)} = ${dartColor(t.$value)};`);
    }
  }
  lines.push('}');
  lines.push('');
  return lines.join('\n');
}

// buildGo emits the palette as Go for the email package. Email clients can't
// use CSS custom properties, so templates inline the hex at render time from
// these generated constants — editing tokens.json + regenerating updates every
// email. Output is gofmt-aligned so the --check gate matches byte-for-byte.
function camel(key) {
  const parts = key.split('-');
  return parts[0] + parts.slice(1).map((w) => w[0].toUpperCase() + w.slice(1)).join('');
}

function buildMaterialDart(tokens, name) {
  const spec = MATERIALS[name];
  const mat = tokens[name];
  const lines = [
    '// ' + GENERATED_BANNER.split('\n').join('\n// '),
    '',
    "import 'dart:ui';",
    '',
    `/// ${spec.summary} (issue #2770).`,
    '///',
    ...spec.detail.map((l) => `/// ${l}`),
    '///',
    '/// Consumed by `AppColors`, which keeps the semantic names widgets already',
    '/// call pointing here — widgets should keep using those rather than reaching',
    '/// for these directly.',
    `class ${spec.className} {`,
    `  ${spec.className}._();`,
    '',
  ];
  for (const key of spec.colorKeys) {
    const t = mat[key];
    lines.push(`  /// ${t.$description}`);
    lines.push(`  static const Color ${camel(key)} = Color(0x${t.$value.slice(1)});`);
    lines.push('');
  }
  for (const key of spec.numberKeys) {
    const t = mat[key];
    lines.push(`  /// ${t.$description}`);
    lines.push(`  static const double ${camel(key)} = ${t.$value.toFixed(1)};`);
    lines.push('');
  }
  lines.pop();
  lines.push('}');
  return lines.join('\n') + '\n';
}

function buildGo(tokens) {
  const names = COLOR_KEYS.map(pascal);
  const maxName = Math.max(...names.map((n) => n.length));
  const maxKey = maxName + 1; // "Name:"
  const lines = [];
  lines.push('// ' + GENERATED_BANNER.split('\n').join('\n// '));
  lines.push('');
  lines.push('package email');
  lines.push('');
  lines.push('// BrandColors is the design-token color palette (issue #2441) for one theme,');
  lines.push('// as #RRGGBB strings inlined into email templates at render time (email');
  lines.push("// clients can't use CSS custom properties). Edit design/tokens.json and run");
  lines.push('// `npm run generate:design-tokens` to update every email we send.');
  lines.push('type BrandColors struct {');
  for (const name of names) {
    lines.push(`\t${name}${' '.repeat(maxName - name.length + 1)}string`);
  }
  lines.push('}');
  for (const theme of THEMES) {
    lines.push('');
    lines.push(`// ${pascal(theme)}Colors is the ${theme}-theme palette.`);
    lines.push(`var ${pascal(theme)}Colors = BrandColors{`);
    for (const key of COLOR_KEYS) {
      const name = pascal(key);
      const k = `${name}:`;
      lines.push(`\t${k}${' '.repeat(maxKey - k.length + 1)}"${tokens.color[theme][key].$value}",`);
    }
    lines.push('}');
  }
  lines.push('');
  return lines.join('\n');
}

function cssVar(key) {
  return `--color-${key}`;
}

function buildCss(tokens) {
  const lines = [];
  lines.push('/*');
  lines.push(' * ' + GENERATED_BANNER.split('\n').join('\n * '));
  lines.push(' *');
  lines.push(' * Dark mode follows the visitor\'s OS preference via');
  lines.push(' * @media (prefers-color-scheme: dark), matching the app\'s auto-theming.');
  lines.push(' */');
  lines.push('');
  lines.push(':root {');
  lines.push(`  --font-serif: ${tokens.font.serif.$value.map((f) => (f.includes(' ') ? `'${f}'` : f)).join(', ')};`);
  lines.push(`  --font-sans: ${tokens.font.sans.$value.map((f) => (f.includes(' ') ? `'${f}'` : f)).join(', ')};`);
  lines.push('');
  for (const key of COLOR_KEYS) {
    const t = tokens.color.light[key];
    const firstSentence = t.$description.split(/\.(?:\s|$)/)[0];
    lines.push(`  ${cssVar(key)}: ${t.$value};${' '.repeat(Math.max(1, 10 - t.$value.length))}/* ${firstSentence} */`);
  }
  lines.push('}');
  lines.push('');
  lines.push('@media (prefers-color-scheme: dark) {');
  lines.push('  :root {');
  for (const key of COLOR_KEYS) {
    const t = tokens.color.dark[key];
    lines.push(`    ${cssVar(key)}: ${t.$value};`);
  }
  lines.push('  }');
  lines.push('}');
  lines.push('');
  lines.push('/* Fixed-theme variables — for surfaces whose theme does NOT follow the OS');
  lines.push(' * preference (e.g. marketing photo-overlay sections are always dark, their');
  lines.push(' * light sections always light). These never switch. */');
  lines.push(':root {');
  for (const theme of THEMES) {
    for (const key of COLOR_KEYS) {
      lines.push(`  --${theme}-${key}: ${tokens.color[theme][key].$value};`);
    }
  }
  lines.push('}');
  lines.push('');
  return lines.join('\n');
}

// Minimal Markdown subset for design/guidelines.md: #/##/### headings,
// paragraphs, - lists, **bold**, `code`. Enough for a one-page prose fragment.
function markdownToHtml(md) {
  const inline = (s) => s
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/\*\*([^*]+)\*\*/g, '<b>$1</b>')
    .replace(/`([^`]+)`/g, '<code>$1</code>');
  const out = [];
  let items = null; // open list: array of item-text buffers (wrapped lines append)
  let para = [];
  const flushPara = () => { if (para.length) { out.push(`<p>${inline(para.join(' '))}</p>`); para = []; } };
  const flushList = () => {
    if (items) {
      out.push('<ul>');
      for (const it of items) out.push(`<li>${inline(it.join(' '))}</li>`);
      out.push('</ul>');
      items = null;
    }
  };
  for (const raw of md.split('\n')) {
    const line = raw.trimEnd();
    const h = line.match(/^(#{1,3}) +(.*)/);
    if (h) { flushPara(); flushList(); out.push(`<h${h[1].length + 1}>${inline(h[2])}</h${h[1].length + 1}>`); continue; }
    if (/^- /.test(line)) { flushPara(); if (!items) items = []; items.push([line.slice(2)]); continue; }
    if (line === '') { flushPara(); flushList(); continue; }
    if (items) { items[items.length - 1].push(line.trim()); continue; } // wrapped list line
    para.push(line.trim());
  }
  flushPara(); flushList();
  return out.join('\n');
}

function buildHtml(tokens, prose, contrastResults) {
  const themePanel = (theme) => {
    const g = tokens.color[theme];
    const sw = COLOR_KEYS.map((key) => `
      <div class="tok">
        <div class="chip" style="background:${g[key].$value}"></div>
        <div class="ti">
          <div class="tn">${key} <code>${g[key].$value}</code></div>
          <div class="td">${g[key].$description}</div>
        </div>
      </div>`).join('');
    const rows = contrastResults[theme].map(({ fg, bg, min, ratio }) => `
      <tr class="${ratio >= min ? 'ok' : 'bad'}"><td>${fg} / ${bg}</td><td>${ratio.toFixed(2)}:1</td><td>&ge; ${min}:1</td></tr>`).join('');
    return `
    <section class="theme" data-theme="${theme}" style="--bg:${g.background.$value}; --surface:${g.surface.$value}; --border:${g.border.$value}; --text:${g['text-primary'].$value}; --text2:${g['text-secondary'].$value}; --faint:${g['text-faint'].$value}; --primary:${g.primary.$value}; --on-primary:${g['on-primary'].$value}; --accent:${g.accent.$value}; --on-accent:${g['on-accent'].$value};">
      <h3 class="theme-name">${theme}</h3>
      <div class="specimen">
        <div class="spec-display">The things we <span class="acc">share</span> hold us together.</div>
        <div class="spec-body">Lend the ladder. Host the potluck. Ask for the ride. Body text is ${tokens.font.sans.$value[0]} at 1.7 line-height.</div>
        <div class="spec-ui"><span class="btn">Get the app</span><span class="lbl">LABEL · ${tokens.font.sans.$value[0].toUpperCase()}</span></div>
      </div>
      <div class="toks">${sw}</div>
      <table class="ratios"><thead><tr><th>pair</th><th>ratio</th><th>required</th></tr></thead><tbody>${rows}</tbody></table>
    </section>`;
  };

  return `<!DOCTYPE html>
<!--
  ${GENERATED_BANNER.split('\n').join('\n  ')}
-->
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Ripls design tokens — guidelines</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Libre+Baskerville:wght@400;700&family=Public+Sans:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
  * { box-sizing: border-box; margin: 0; }
  body { background: #141714; color: #F4F4EF; font-family: 'Public Sans', sans-serif; padding: 48px 28px 96px; }
  .wrap { max-width: 1160px; margin: 0 auto; }
  h1 { font-family: 'Libre Baskerville', serif; font-weight: 400; font-size: 30px; margin-bottom: 6px; }
  .src { font-size: 12px; color: #ABB3AB; margin-bottom: 30px; }
  .src code { background: #1F2421; padding: 1px 6px; border-radius: 5px; }
  .prose { max-width: 860px; line-height: 1.7; font-size: 14.5px; color: #D6D9D2; margin-bottom: 40px; }
  .prose h2 { font-family: 'Libre Baskerville', serif; font-weight: 400; font-size: 21px; margin: 26px 0 10px; color: #F4F4EF; }
  .prose h3 { font-size: 15px; margin: 18px 0 8px; color: #F4F4EF; }
  .prose p { margin-bottom: 10px; }
  .prose ul { margin: 0 0 12px 20px; }
  .prose li { margin-bottom: 6px; }
  .prose code { background: #1F2421; padding: 1px 6px; border-radius: 5px; font-size: 13px; }
  .prose b { color: #F4F4EF; }
  .themes { display: grid; grid-template-columns: 1fr 1fr; gap: 18px; align-items: start; }
  @media (max-width: 1000px) { .themes { grid-template-columns: 1fr; } }
  .theme { background: var(--bg); color: var(--text); border-radius: 16px; padding: 22px; border: 1px solid #323832; }
  .theme-name { font-size: 11px; font-weight: 700; letter-spacing: 0.16em; text-transform: uppercase; color: var(--text2); margin-bottom: 16px; }
  .specimen { background: var(--surface); border: 1px solid var(--border); border-radius: 12px; padding: 18px; margin-bottom: 18px; }
  .spec-display { font-family: 'Libre Baskerville', serif; font-size: 24px; line-height: 1.25; margin-bottom: 8px; }
  .spec-display .acc { color: var(--primary); box-shadow: inset 0 -0.14em 0 0 color-mix(in srgb, var(--primary) 38%, transparent); }
  .spec-body { font-size: 13.5px; line-height: 1.7; color: var(--text2); margin-bottom: 14px; }
  .spec-ui { display: flex; gap: 12px; align-items: center; }
  .btn { background: var(--primary); color: var(--on-primary); font-size: 12px; font-weight: 700; padding: 10px 20px; border-radius: 999px; }
  .lbl { font-size: 10px; font-weight: 600; letter-spacing: 0.12em; color: var(--faint); }
  .toks { display: grid; gap: 8px; margin-bottom: 18px; }
  .tok { display: flex; gap: 12px; align-items: flex-start; }
  .chip { width: 42px; height: 42px; border-radius: 9px; flex: none; border: 1px solid var(--border); }
  .tn { font-size: 12.5px; font-weight: 600; }
  .tn code { font-weight: 400; color: var(--text2); font-size: 11.5px; margin-left: 5px; }
  .td { font-size: 11.5px; line-height: 1.5; color: var(--text2); }
  .ratios { width: 100%; border-collapse: collapse; font-size: 11px; }
  .ratios th { text-align: left; font-weight: 600; color: var(--text2); padding: 4px 6px; border-bottom: 1px solid var(--border); }
  .ratios td { padding: 3px 6px; border-bottom: 1px solid var(--border); color: var(--text2); font-variant-numeric: tabular-nums; }
  .ratios tr.ok td:nth-child(2) { color: var(--text); }
  .ratios tr.bad td { color: #B5492B; font-weight: 700; }
</style>
</head>
<body>
<div class="wrap">
  <h1>Ripls design tokens</h1>
  <p class="src">Generated from <code>${TOKENS_REL}</code> — every swatch, hex, and ratio on this page comes from the token source. Fonts: <b>${tokens.font.serif.$value[0]}</b> (display) + <b>${tokens.font.sans.$value[0]}</b> (body/UI).</p>
  <div class="prose">
${markdownToHtml(prose)}
  </div>
  <div class="themes">
${themePanel('light')}
${themePanel('dark')}
  </div>
</div>
</body>
</html>
`;
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Sentinel mode (#2770)
// ---------------------------------------------------------------------------
//
// The composited-contrast gate renders each surface twice: once normally, and
// once with every token replaced by a distinct sentinel hue. Diffing the two
// tells the analyzer which pixels each token painted — the only way to attribute
// colour on a CanvasKit surface, where text is canvas pixels rather than DOM.
//
// Hues are assigned per COLOR KEY, not per (theme, key): only one theme renders
// in a given capture, so `light.primary` and `dark.primary` never appear in the
// same frame and can share a hue. That halves the count (15, not 30) and so
// doubles the angular separation between neighbours, which is what keeps
// classification robust against anti-aliased glyph edges.

const SENTINEL_REL = 'design/sentinel_map.json';

/**
 * Tokens that are never sentineled.
 *
 * `background` and `surface` are the *reference* the gate measures against, not
 * subjects of measurement. Sentineling them means nearly every pixel in the
 * frame changes, which (a) produces frame-sized masks the analyzer then has to
 * segment and ring-sample, and (b) yields meaningless findings like "surface on
 * surface". The ring background reads the real composited colour from the
 * baseline render regardless of which token painted it, so nothing is lost.
 */
const SENTINEL_EXCLUDED = ['background', 'surface'];

/** Material keys that are measurable marks (everything not a surface). */
function materialSentinelKeys(name) {
  const spec = MATERIALS[name];
  return spec.colorKeys.filter((k) => !spec.sentinelExcluded.includes(k));
}

/**
 * What each token is, for the purpose of a contrast threshold.
 *
 * Without this the gate holds every token to the 4.5:1 normal-text rule, which
 * is wrong in both directions: `text-faint` is documented in tokens.json as
 * clearing 3:1 and never carrying body copy, while a hairline `border` is a
 * non-text UI component (WCAG 1.4.11) and a decorative divider is exempt
 * entirely. Judging those as body text manufactures failures and buries the
 * real ones.
 *
 *   text        normal body/UI text — 4.5:1 (3:1 when large)
 *   text-faint  deliberately de-emphasised — 3:1, never body copy
 *   ui          non-text UI component or boundary — 3:1
 *   decorative  carries no information on its own — reported, never fails
 */
const TOKEN_ROLES = {
  'text-primary': 'text',
  'text-secondary': 'text',
  'text-faint': 'text-faint',
  primary: 'text',
  'on-primary': 'text',
  'primary-hover': 'ui',
  accent: 'text',
  'on-accent': 'text',
  success: 'text',
  warning: 'text',
  error: 'text',
  info: 'text',
  border: 'decorative',
  background: 'ui',
  surface: 'ui',
};

/**
 * Sentinel colours, chosen by farthest-point selection over the RGB cube.
 *
 * Two properties matter, and they pull in opposite directions:
 *
 *   1. Sentinels must be far from EACH OTHER, because classification assigns
 *      every changed pixel to its nearest sentinel and anti-aliased glyph edges
 *      drift toward the background. If one token's pure sentinel falls inside
 *      another's core window (radius 70 in contrast_analyzer.mjs), pixels get
 *      attributed to the wrong token.
 *   2. Sentinels must be far from the REAL values they replace, or the diff
 *      that builds the mask falls under the change threshold and the mask
 *      silently loses pixels — reporting nothing rather than a failure.
 *
 * Spreading hues round a circle satisfies (1) only while the set is small. At 27
 * sentinels (palette + glass + overlay) the best circle-plus-lightness-cycle
 * arrangement leaves the closest pair ~57 apart, inside the core window. Picking
 * greedily from the whole cube instead of one slice of it clears that
 * comfortably.
 *
 * Deterministic by construction — fixed grid, fixed seed, fixed tie-breaks —
 * because the map must match the bundle that was built from it.
 */
const SENTINEL_GRID_STEP = 51; // 6 levels per channel: 216 candidates

function farthestPointColors(count) {
  const levels = [];
  for (let v = 0; v <= 255; v += SENTINEL_GRID_STEP) levels.push(v);
  const pool = [];
  for (const r of levels) for (const g of levels) for (const b of levels) pool.push([r, g, b]);

  const dist2 = (a, b) => (a[0] - b[0]) ** 2 + (a[1] - b[1]) ** 2 + (a[2] - b[2]) ** 2;
  // Seed with pure red so the sequence is stable no matter how the pool is built.
  const chosen = [[255, 0, 0]];
  while (chosen.length < count) {
    let best = null;
    let bestScore = -1;
    for (const c of pool) {
      const score = Math.min(...chosen.map((s) => dist2(c, s)));
      // Strict > keeps the first candidate in pool order on ties.
      if (score > bestScore) { bestScore = score; best = c; }
    }
    if (!best) break;
    chosen.push(best);
  }
  return chosen;
}

/**
 * Sentinel hues for every measurable token — palette and every material.
 *
 * Material keys are namespaced (`glass.primary`, `overlay.text-faint`) so they
 * never collide with a palette key of the same name. `primary`, `text-primary`
 * and `text-faint` exist in more than one set, and conflating them would
 * attribute on-glass or on-media pixels to the palette token — which is the
 * exact confusion this whole effort exists to undo.
 */
function sentinelHues(tokens) {
  const keys = [
    ...COLOR_KEYS.filter((k) => !SENTINEL_EXCLUDED.includes(k)),
    ...MATERIAL_NAMES.flatMap((m) => materialSentinelKeys(m).map((k) => `${m}.${k}`)),
  ];
  const colors = farthestPointColors(keys.length);
  const out = {};
  for (const [key, rgb] of assignSentinels(keys, colors, tokens)) {
    out[key] = '#' + rgb.map((v) => v.toString(16).toUpperCase().padStart(2, '0')).join('');
  }
  return out;
}

/**
 * Assigns selected colours to keys so each token is FAR FROM ITS OWN sentinel.
 *
 * Selection maximises separation between sentinels; it has no idea which token
 * each will stand in for. Assigning in list order therefore happily gives the
 * white text token a near-white sentinel, at which point the diff that builds
 * its mask falls toward the change threshold and the mask quietly loses pixels.
 * Greedy, worst-key-first: the token with the least room gets to choose.
 */
function assignSentinels(keys, colors, tokens) {
  const realValues = (key) => {
    const dot = key.indexOf('.');
    if (dot === -1) return THEMES.map((t) => parseHex(tokens.color[t][key].$value));
    return [parseHex('#' + tokens[key.slice(0, dot)][key.slice(dot + 1)].$value.slice(3))];
  };
  const delta = (a, b) => a.reduce((s, v, i) => s + Math.abs(v - b[i]), 0);
  const scoreFor = (key, colour) => Math.min(...realValues(key).map((r) => delta(r, colour)));

  const remaining = new Set(colors.map((_, i) => i));
  const pending = [...keys];
  const assigned = [];
  while (pending.length) {
    // Which key currently has the worst best-available option? Serve it first —
    // otherwise the last key left gets whatever nobody wanted.
    let worstKey = null;
    let worstBest = Infinity;
    for (const key of pending) {
      let best = -1;
      for (const i of remaining) best = Math.max(best, scoreFor(key, colors[i]));
      if (best < worstBest) { worstBest = best; worstKey = key; }
    }
    let pickIdx = null;
    let pickScore = -1;
    for (const i of remaining) {
      const s = scoreFor(worstKey, colors[i]);
      if (s > pickScore) { pickScore = s; pickIdx = i; }
    }
    assigned.push([worstKey, colors[pickIdx]]);
    remaining.delete(pickIdx);
    pending.splice(pending.indexOf(worstKey), 1);
  }
  // Restore declaration order so the emitted map reads predictably.
  const byKey = new Map(assigned);
  return keys.map((k) => [k, byKey.get(k)]);
}

/** Deep-clones [tokens] with every colour value replaced by its sentinel hue. */
function sentinelTokens(tokens, hues = sentinelHues(tokens)) {
  const out = JSON.parse(JSON.stringify(tokens));
  for (const theme of THEMES) {
    for (const key of COLOR_KEYS) {
      if (!(key in hues)) continue; // SENTINEL_EXCLUDED keeps its real value
      out.color[theme][key].$value = hues[key];
    }
  }
  for (const name of MATERIAL_NAMES) {
    for (const key of MATERIALS[name].colorKeys) {
      const hue = hues[`${name}.${key}`];
      if (!hue) continue;
      // Keep the ORIGINAL alpha. A 50%-alpha token repainted opaque would cover
      // different pixels than the real one, so the mask would describe a shape
      // the app never draws — and the measured background would be wrong too.
      const alpha = out[name][key].$value.slice(1, 3);
      out[name][key].$value = `#${alpha}${hue.slice(1)}`;
    }
  }
  return out;
}

/**
 * Dart output for the sentinel pass, plus the map the analyzer classifies with.
 *
 * Deliberately skips [checkContrast] — sentinel hues are chosen for mutual
 * separation, not legibility, and would fail the gate by construction. It also
 * emits ONLY the Dart file: the sentinel bundle is a throwaway build artifact,
 * so rewriting the CSS/Go/HTML consumers would be churn for no reader.
 */
function generateSentinel(tokens) {
  const errors = validate(tokens);
  if (errors.length) {
    throw new Error(`invalid ${TOKENS_REL}:\n  ` + errors.join('\n  '));
  }
  const hues = sentinelHues(tokens);
  const roleFor = (key) => {
    const dot = key.indexOf('.');
    if (dot === -1) return TOKEN_ROLES[key] ?? 'text';
    const [name, k] = [key.slice(0, dot), key.slice(dot + 1)];
    return MATERIALS[name].roles[k] ?? 'text';
  };
  const roles = Object.fromEntries(Object.keys(hues).map((key) => [key, roleFor(key)]));
  // The real values, so the analyzer can tell a genuine low-contrast colour from
  // a low-coverage glyph fragment: a fragment's median foreground is a blend of
  // the token and its background, so it reads darker/lighter than the token
  // actually is and would otherwise be reported as a false failure.
  // Glass values are reported without alpha: the analyzer compares against a
  // COMPOSITED foreground, which is the token blended over whatever is behind
  // it, so the pre-multiplied RGB is what a full-coverage pixel actually shows.
  const valueFor = (key, theme) => {
    const dot = key.indexOf('.');
    if (dot === -1) return tokens.color[theme][key].$value;
    return '#' + tokens[key.slice(0, dot)][key.slice(dot + 1)].$value.slice(3);
  };
  const values = Object.fromEntries(
    THEMES.map((theme) => [
      theme,
      Object.fromEntries(Object.keys(hues).map((key) => [key, valueFor(key, theme)])),
    ]),
  );
  const sentinel = sentinelTokens(tokens, hues);
  return {
    [OUTPUTS.dart]: buildDart(sentinel),
    ...Object.fromEntries(
      MATERIAL_NAMES.map((m) => [MATERIALS[m].output, buildMaterialDart(sentinel, m)]),
    ),
    [SENTINEL_REL]: JSON.stringify({ hues, roles, values }, null, 2) + '\n',
  };
}

/**
 * Deep-merges candidate overrides onto the base token tree (#2770).
 *
 * A candidate states only what it changes, so `$type` and `$description` are
 * inherited from the base and a candidate file stays readable as a diff. Merging
 * happens before validation, so a candidate that produces an invalid or
 * contrast-failing set is rejected exactly like a bad edit to tokens.json —
 * candidates get no exemption from the gate they exist to be judged by.
 */
function applyCandidates(tokens, candidates) {
  const out = JSON.parse(JSON.stringify(tokens));
  for (const candidate of candidates) {
    for (const section of ['color', 'glass']) {
      const overrides = candidate[section];
      if (!overrides) continue;
      if (section === 'glass') {
        for (const [key, val] of Object.entries(overrides)) {
          if (!out.glass[key]) throw new Error(`candidate ${candidate.id}: unknown glass.${key}`);
          Object.assign(out.glass[key], val);
        }
      } else {
        for (const [theme, keys] of Object.entries(overrides)) {
          if (!out.color[theme]) throw new Error(`candidate ${candidate.id}: unknown theme ${theme}`);
          for (const [key, val] of Object.entries(keys)) {
            if (!out.color[theme][key]) {
              throw new Error(`candidate ${candidate.id}: unknown color.${theme}.${key}`);
            }
            Object.assign(out.color[theme][key], val);
          }
        }
      }
    }
  }
  return out;
}

function generate(tokens, prose) {
  const errors = validate(tokens);
  if (errors.length) {
    throw new Error(`invalid ${TOKENS_REL}:\n  ` + errors.join('\n  '));
  }
  const { failures, results } = checkContrast(tokens);
  if (failures.length) {
    throw new Error('contrast gate failed:\n  ' + failures.join('\n  '));
  }
  return {
    [OUTPUTS.dart]: buildDart(tokens),
    ...Object.fromEntries(
      MATERIAL_NAMES.map((m) => [MATERIALS[m].output, buildMaterialDart(tokens, m)]),
    ),
    [OUTPUTS.css]: buildCss(tokens),
    [OUTPUTS.go]: buildGo(tokens),
    [OUTPUTS.html]: buildHtml(tokens, prose, results),
  };
}

function main() {
  const checkMode = process.argv.includes('--check');
  const sentinelMode = process.argv.includes('--sentinel');
  let tokens = JSON.parse(fs.readFileSync(path.join(ROOT, TOKENS_REL), 'utf8'));
  const prose = fs.readFileSync(path.join(ROOT, PROSE_REL), 'utf8');

  // --candidate <file> (repeatable): overlay evaluation candidates (#2770).
  const candidates = [];
  process.argv.forEach((a, i) => {
    if (a === '--candidate' && process.argv[i + 1]) {
      candidates.push(JSON.parse(fs.readFileSync(process.argv[i + 1], 'utf8')));
    }
  });
  if (candidates.length) {
    try {
      tokens = applyCandidates(tokens, candidates);
    } catch (e) {
      console.error(`gen_design_tokens: ${e.message}`);
      process.exit(1);
    }
    console.log(`Applied candidates: ${candidates.map((c) => c.id).join(', ')}`);
  }

  let outputs;
  try {
    outputs = sentinelMode ? generateSentinel(tokens) : generate(tokens, prose);
  } catch (e) {
    console.error(`gen_design_tokens: ${e.message}`);
    process.exit(1);
  }

  if (sentinelMode && checkMode) {
    console.error('gen_design_tokens: --sentinel and --check are mutually exclusive');
    process.exit(1);
  }

  if (checkMode) {
    const stale = [];
    for (const [rel, content] of Object.entries(outputs)) {
      const abs = path.join(ROOT, rel);
      if (!fs.existsSync(abs) || fs.readFileSync(abs, 'utf8') !== content) stale.push(rel);
    }
    if (stale.length) {
      console.error('gen_design_tokens: generated outputs are stale:');
      for (const s of stale) console.error(`  ${s}`);
      console.error('\nRun `npm run generate:design-tokens` and commit the results.');
      process.exit(1);
    }
    console.log(`check_design_tokens passed: ${Object.keys(outputs).length} outputs current, all contrast pairs pass.`);
    return;
  }

  for (const [rel, content] of Object.entries(outputs)) {
    const abs = path.join(ROOT, rel);
    fs.mkdirSync(path.dirname(abs), { recursive: true });
    fs.writeFileSync(abs, content);
    console.log(`Wrote ${rel}`);
  }
  if (sentinelMode) {
    console.log(
      '\nSENTINEL build artifacts written. These are throwaway — run\n' +
        '`npm run generate:design-tokens` to restore the real token values.',
    );
  }
}

if (require.main === module) {
  main();
}

module.exports = {
  COLOR_KEYS,
  THEMES,
  CONTRAST_PAIRS,
  parseHex,
  relativeLuminance,
  contrastRatio,
  validate,
  checkContrast,
  applyCandidates,
  sentinelHues,
  sentinelTokens,
  generateSentinel,
  farthestPointColors,
  MATERIALS,
  buildDart,
  buildCss,
  buildGo,
  markdownToHtml,
  generate,
};
