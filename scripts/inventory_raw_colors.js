#!/usr/bin/env node
/**
 * inventory_raw_colors.js — enumerate every colour and alpha in app/lib that
 * does NOT resolve through a design token (#2770).
 *
 * Why this exists: the contrast gate measures by sentinel-diff, which recolours
 * *tokens*. A colour no token owns is never recoloured, so it is absent from
 * both the numerator and the denominator — it cannot fail, because it is never
 * examined. `gear_who_card.dart`'s "See the calendar" CTA measured 5% changed
 * (max delta 17, i.e. noise) and its Share button 0%: both invisible to the
 * gate while looking like the lowest-contrast controls on the screen.
 *
 * `lint:dart:colors` does not cover this. It blocks an enumerated list of ~12
 * coral/sage hex values, so an arbitrary green is legal and unmonitored.
 *
 * The output is a migration worklist, not just a count. Each finding carries
 * its nearest token, so the mechanical cases (a literal that IS a token value,
 * or sits within a few units of one) separate from those needing a judgement
 * call about which canonical token the author meant.
 *
 * Usage:
 *   node scripts/inventory_raw_colors.js              # human summary
 *   node scripts/inventory_raw_colors.js --json       # machine-readable
 *   node scripts/inventory_raw_colors.js --glass-only # only glass/media files
 */

'use strict';

const fs = require('fs');
const path = require('path');

const ROOT = path.join(__dirname, '..');
const SCAN_ROOT = path.join(ROOT, 'app', 'lib');
const TOKENS_JSON = path.join(ROOT, 'design', 'tokens.json');

// core/theme is where literals legitimately live — it is the definition site.
// gen/ and l10n are generated. Tests are not shipped pixels.
const EXCLUDED_SEGMENTS = [
  `${path.sep}gen${path.sep}`,
  `${path.sep}core${path.sep}theme${path.sep}`,
  `${path.sep}l10n${path.sep}`,
];
const EXCLUDED_SUFFIXES = ['.g.dart', '.freezed.dart', '.mocks.dart'];

/**
 * Signals that a file paints onto glass or media rather than a flat opaque
 * surface. Findings there are the high-risk set: the palette's contrast gate
 * validated its tokens against `background`/`surface`, so a literal chosen to
 * look right on a card can be far off over a photo.
 */
const SURFACE_SIGNALS = [
  'modal',
  'Glass',
  'glass',
  'Overlay',
  'onContentImage',
  'BackdropFilter',
  'ContentGradient',
  'MediaBackground',
];

function walk(dir, out = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      walk(full, out);
    } else if (entry.name.endsWith('.dart')) {
      out.push(full);
    }
  }
  return out;
}

function isScannable(file) {
  if (EXCLUDED_SEGMENTS.some((seg) => file.includes(seg))) return false;
  if (EXCLUDED_SUFFIXES.some((suf) => file.endsWith(suf))) return false;
  return true;
}

/** Flattens tokens.json into [{name, argb}] with 8-digit ARGB. */
function loadTokens() {
  const raw = JSON.parse(fs.readFileSync(TOKENS_JSON, 'utf8'));
  const out = [];
  const visit = (node, prefix) => {
    for (const [key, value] of Object.entries(node)) {
      if (key.startsWith('$')) continue;
      if (value && typeof value === 'object' && value.$type === 'color') {
        out.push({ name: `${prefix}${key}`, argb: normalizeHex(value.$value) });
      } else if (value && typeof value === 'object') {
        visit(value, `${prefix}${key}.`);
      }
    }
  };
  visit(raw, '');
  return out;
}

/** #RGB / #RRGGBB / #AARRGGBB / 0xAARRGGBB → AARRGGBB uppercase. */
function normalizeHex(value) {
  let hex = String(value).trim().replace(/^#/, '').replace(/^0x/i, '');
  if (hex.length === 6) hex = `FF${hex}`;
  return hex.toUpperCase();
}

function toRgba(argb) {
  return {
    a: parseInt(argb.slice(0, 2), 16),
    r: parseInt(argb.slice(2, 4), 16),
    g: parseInt(argb.slice(4, 6), 16),
    b: parseInt(argb.slice(6, 8), 16),
  };
}

/**
 * Distance in RGB plus alpha, kept separate because they mean different things:
 * a literal matching a token's RGB but not its alpha is a different kind of
 * finding (an ad-hoc opacity step) from one that matches nothing at all.
 */
function distance(a, b) {
  const x = toRgba(a);
  const y = toRgba(b);
  return {
    rgb: Math.round(
      Math.sqrt((x.r - y.r) ** 2 + (x.g - y.g) ** 2 + (x.b - y.b) ** 2),
    ),
    alpha: Math.abs(x.a - y.a),
  };
}

function nearestToken(argb, tokens) {
  let best = null;
  for (const token of tokens) {
    const d = distance(argb, token.argb);
    const score = d.rgb + d.alpha / 4;
    if (best === null || score < best.score) {
      best = { ...token, ...d, score };
    }
  }
  return best;
}

// Dart colour spellings that bypass the token layer.
const PATTERNS = [
  { kind: 'hex', re: /Color\(\s*(0x[0-9a-fA-F]{8})\s*\)/g },
  { kind: 'hex-argb', re: /Color\.fromARGB\(\s*([0-9]+)\s*,\s*([0-9]+)\s*,\s*([0-9]+)\s*,\s*([0-9]+)\s*\)/g },
  // The trailing \d* is load-bearing: Flutter's opacity shortcuts are spelled
  // `Colors.white70` / `Colors.black45`, and `[a-zA-Z]+\b` silently skips every
  // one of them — 65 sites across 9 distinct opacity steps, which is exactly
  // the "random alphas" this inventory exists to find.
  { kind: 'material', re: /\bColors\.([a-zA-Z]+\d*)(?:\.shade\d+)?\b/g },
  { kind: 'alpha-withValues', re: /\.withValues\(\s*alpha:\s*([0-9.]+)\s*\)/g },
  { kind: 'alpha-withAlpha', re: /\.withAlpha\(\s*([0-9]+)\s*\)/g },
  { kind: 'alpha-withOpacity', re: /\.withOpacity\(\s*([0-9.]+)\s*\)/g },
  // Blur is part of the material, not decoration: `glass.blur-sigma` (24) and
  // `overlay`'s backdrop blur (4) are tokens, so a hand-written sigma is a
  // second, unmanaged material definition.
  { kind: 'blur-sigma', re: /sigma[XY]:\s*([0-9.]+)/g },
];

function scanFile(file, tokens) {
  const text = fs.readFileSync(file, 'utf8');
  const rel = path.relative(ROOT, file);
  const onSurface = SURFACE_SIGNALS.some((sig) => text.includes(sig));
  const lines = text.split('\n');
  const findings = [];

  for (const { kind, re } of PATTERNS) {
    re.lastIndex = 0;
    let match;
    while ((match = re.exec(text)) !== null) {
      const line = text.slice(0, match.index).split('\n').length;
      const source = lines[line - 1].trim();
      // Skip the comment-only mentions that document a migration.
      if (source.startsWith('//') || source.startsWith('///')) continue;

      const finding = { file: rel, line, kind, raw: match[0], source, onSurface };

      if (kind === 'hex') {
        finding.argb = normalizeHex(match[1]);
      } else if (kind === 'hex-argb') {
        const [, a, r, g, b] = match;
        finding.argb = [a, r, g, b]
          .map((n) => Number(n).toString(16).padStart(2, '0'))
          .join('')
          .toUpperCase();
      } else if (kind === 'material') {
        finding.material = match[1];
        // `Colors.transparent` is structural, not a palette choice.
        if (match[1] === 'transparent') continue;
      } else if (kind === 'blur-sigma') {
        finding.sigma = Number(match[1]);
      } else {
        finding.alpha = Number(match[1]);
      }

      if (finding.argb) {
        finding.nearest = nearestToken(finding.argb, tokens);
      }
      findings.push(finding);
    }
  }
  return findings;
}

function main() {
  const args = process.argv.slice(2);
  const asJson = args.includes('--json');
  const glassOnly = args.includes('--glass-only');

  const tokens = loadTokens();
  const files = walk(SCAN_ROOT).filter(isScannable);
  let findings = files.flatMap((f) => scanFile(f, tokens));
  if (glassOnly) findings = findings.filter((f) => f.onSurface);

  if (asJson) {
    process.stdout.write(JSON.stringify({ tokens: tokens.length, findings }, null, 2));
    return;
  }

  const byKind = {};
  for (const f of findings) byKind[f.kind] = (byKind[f.kind] ?? 0) + 1;

  console.log(`Scanned ${files.length} files against ${tokens.length} tokens.`);
  console.log(`${findings.length} untokenized colour/alpha sites.\n`);

  console.log('By kind:');
  for (const [kind, n] of Object.entries(byKind).sort((a, b) => b[1] - a[1])) {
    console.log(`  ${String(n).padStart(4)}  ${kind}`);
  }

  const onSurface = findings.filter((f) => f.onSurface);
  console.log(
    `\nOn glass/media-rendering files: ${onSurface.length} sites across ` +
      `${new Set(onSurface.map((f) => f.file)).size} files (the high-risk set).`,
  );

  // Exact token matches are pure mechanical migration — a literal spelling of a
  // value the token layer already owns.
  const colours = findings.filter((f) => f.argb);
  const exact = colours.filter((f) => f.nearest.rgb === 0 && f.nearest.alpha === 0);
  const near = colours.filter(
    (f) => !(f.nearest.rgb === 0 && f.nearest.alpha === 0) && f.nearest.rgb <= 12,
  );
  const far = colours.filter((f) => f.nearest.rgb > 12);

  console.log('\nColour literals by distance to the nearest existing token:');
  console.log(`  ${String(exact.length).padStart(4)}  EXACT match — mechanical rename`);
  console.log(`  ${String(near.length).padStart(4)}  within 12 — near-duplicate, collapse into the token`);
  console.log(`  ${String(far.length).padStart(4)}  no close token — needs a decision (or a new token)`);

  const valueCounts = new Map();
  for (const f of colours) {
    const entry = valueCounts.get(f.argb) ?? { n: 0, files: new Set(), nearest: f.nearest };
    entry.n += 1;
    entry.files.add(f.file);
    valueCounts.set(f.argb, entry);
  }
  const repeated = [...valueCounts.entries()]
    .filter(([, v]) => v.n > 1)
    .sort((a, b) => b[1].n - a[1].n)
    .slice(0, 15);

  console.log('\nMost-repeated raw values (each is one token waiting to be named):');
  for (const [argb, v] of repeated) {
    const near = v.nearest;
    const hint =
      near.rgb === 0 && near.alpha === 0
        ? `= ${near.name}`
        : `~ ${near.name} (rgb ${near.rgb}, alpha ${near.alpha})`;
    console.log(
      `  ${String(v.n).padStart(3)}x  0x${argb}  in ${String(v.files.size).padStart(2)} files   ${hint}`,
    );
  }

  const alphas = findings.filter((f) => f.alpha !== undefined);
  const alphaCounts = new Map();
  for (const f of alphas) {
    alphaCounts.set(f.alpha, (alphaCounts.get(f.alpha) ?? 0) + 1);
  }
  console.log(
    `\nAd-hoc alpha modifiers: ${alphas.length} sites, ` +
      `${alphaCounts.size} distinct values.`,
  );
  const topAlphas = [...alphaCounts.entries()].sort((a, b) => b[1] - a[1]).slice(0, 12);
  console.log('  ' + topAlphas.map(([v, n]) => `${v}(${n}x)`).join('  '));

  const sigmas = findings.filter((f) => f.sigma !== undefined);
  if (sigmas.length) {
    const sigCounts = new Map();
    for (const f of sigmas) sigCounts.set(f.sigma, (sigCounts.get(f.sigma) ?? 0) + 1);
    console.log(
      `\nHand-written blur sigmas: ${sigmas.length} sites, ` +
        `${sigCounts.size} distinct values (tokens define 24 and 4).`,
    );
    console.log(
      '  ' +
        [...sigCounts.entries()]
          .sort((a, b) => b[1] - a[1])
          .map(([v, n]) => `${v}(${n}x)`)
          .join('  '),
    );
  }

  const materials = findings.filter((f) => f.material);
  const matCounts = new Map();
  for (const f of materials) matCounts.set(f.material, (matCounts.get(f.material) ?? 0) + 1);
  console.log(`\nMaterial palette shortcuts: ${materials.length} sites.`);
  console.log(
    '  ' +
      [...matCounts.entries()]
        .sort((a, b) => b[1] - a[1])
        .slice(0, 12)
        .map(([v, n]) => `Colors.${v}(${n}x)`)
        .join('  '),
  );

  console.log('\nWorst files:');
  const fileCounts = new Map();
  for (const f of findings) fileCounts.set(f.file, (fileCounts.get(f.file) ?? 0) + 1);
  for (const [file, n] of [...fileCounts.entries()].sort((a, b) => b[1] - a[1]).slice(0, 12)) {
    console.log(`  ${String(n).padStart(3)}  ${file}`);
  }
}

if (require.main === module) main();

module.exports = { walk, isScannable, normalizeHex, distance, nearestToken, scanFile, loadTokens };
