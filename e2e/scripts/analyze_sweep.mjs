#!/usr/bin/env node
// analyze_sweep.mjs — measure every candidate combination and build the sheet (#2770).
//
//   node scripts/analyze_sweep.mjs --slug sweep --out docs/concepts/theme/contrast-eval.html
//
// Pairs each variant's baseline and sentinel captures, measures composited
// contrast per token, and emits the comparison sheet. The per-capture headline
// is the WORST surviving instance, not a mean: a screen is only as legible as
// its least legible element, and averaging is how a genuine failure gets hidden
// behind twenty passing glyph runs.

import { readFileSync, writeFileSync, existsSync, readdirSync, mkdirSync, copyFileSync } from 'node:fs';
import { join, dirname, basename } from 'node:path';

import { analyze, judge } from './contrast_analyzer.mjs';
import { decodePng } from './png.mjs';
import { buildSheet } from './build_contrast_sheet.mjs';
import { SURFACES } from './contrast_surfaces.mjs';

function arg(name, fallback) {
  const i = process.argv.indexOf(`--${name}`);
  if (i >= 0 && process.argv[i + 1]) return process.argv[i + 1];
  if (fallback !== undefined) return fallback;
  throw new Error(`missing required --${name}`);
}

const slug = arg('slug', 'sweep');
const out = arg('out');
const root = join('investigations', slug);

const mapPath = join('..', 'design', 'sentinel_map.json');
if (!existsSync(mapPath)) {
  console.error(`analyze_sweep: ${mapPath} missing — the sweep writes it during the sentinel pass.`);
  process.exit(1);
}
const { hues, roles = {}, values = {} } = JSON.parse(readFileSync(mapPath, 'utf8'));

const candidatesDir = join('..', 'design', 'candidates');
const candidates = readdirSync(candidatesDir)
  .filter((f) => f.endsWith('.json'))
  .map((f) => JSON.parse(readFileSync(join(candidatesDir, f), 'utf8')));
const palettes = candidates.filter((c) => c.axis === 'palette').map((c) => ({ id: c.id, label: c.label }));
const ramps = candidates.filter((c) => c.axis === 'glass').map((c) => ({ id: c.id, label: c.label }));

// Images are copied next to the sheet so it is portable — a page that renders
// only from someone's gitignored scratch directory is not a decision artifact.
const assetDir = join(dirname(out), 'contrast-eval');
mkdirSync(assetDir, { recursive: true });

const manifests = readdirSync(root).filter((f) => /^manifest-.+-baseline\.json$/.test(f));
const captures = [];
let measured = 0;

// Every variant must have been captured over the same cells. Unequal counts mean
// manifests from different runs got mixed — a sheet built from those compares
// candidates rendered from different code states, which is a wrong answer
// wearing the costume of a decision artifact.
{
  const counts = manifests.map((f) => {
    const m = JSON.parse(readFileSync(join(root, f), 'utf8'));
    return { variant: m.variant, n: m.captures.length };
  });
  const sizes = new Set(counts.map((c) => c.n));
  if (sizes.size > 1) {
    console.error('analyze_sweep: variants have different capture counts — refusing to build a sheet.\n');
    for (const c of counts.sort((a, b) => a.n - b.n)) {
      console.error(`  ${String(c.n).padStart(3)}  ${c.variant}`);
    }
    console.error(
      '\nThis means manifests from more than one run are present. Re-run the sweep' +
        '\n(it now clears prior state first) rather than trusting a mixed sheet.',
    );
    process.exit(1);
  }
}

for (const file of manifests) {
  const baseManifest = JSON.parse(readFileSync(join(root, file), 'utf8'));
  const variant = baseManifest.variant;
  const sentPath = join(root, file.replace('-baseline.json', '-sentinel.json'));
  if (!existsSync(sentPath)) {
    console.warn(`  ! ${variant}: no sentinel manifest, skipped`);
    continue;
  }
  const sentManifest = JSON.parse(readFileSync(sentPath, 'utf8'));
  const sentById = new Map(sentManifest.captures.map((c) => [c.id, c]));
  const [palette, ramp] = variant.split('__');

  for (const capture of baseManifest.captures) {
    const twin = sentById.get(capture.id);
    if (!twin) continue;
    const base = decodePng(readFileSync(capture.file));
    const sent = decodePng(readFileSync(twin.file));
    if (base.width !== sent.width || base.height !== sent.height) continue;

    const findings = judge(
      analyze({
        baseline: base.buffer,
        sentinel: sent.buffer,
        width: base.width,
        height: base.height,
        sentinelMap: hues,
      }),
      { roles, values },
    );
    measured += findings.length;

    // Findings that were suppressed as low-confidence describe the
    // rasterisation, not the token — they must not become a surface's headline.
    const judged = findings.filter((f) => !f.lowConfidence && f.required !== null);
    const worst = judged.length
      ? judged.reduce((a, b) => (b.ratio < a.ratio ? b : a))
      : null;

    const assetName = `${variant}-${capture.id}.png`;
    copyFileSync(capture.file, join(assetDir, assetName));

    captures.push({
      palette,
      ramp,
      surface: capture.surface,
      theme: capture.theme,
      backdrop: capture.backdrop,
      file: `${basename(assetDir)}/${assetName}`,
      worst: worst
        ? { token: worst.token, ratio: worst.ratio, required: worst.required }
        : null,
    });
  }
  console.log(`  ✓ ${variant}: ${baseManifest.captures.length} captures`);
}

const surfaces = SURFACES.map((s) => ({
  id: s.id,
  group: s.group,
  description: s.description,
}));

writeFileSync(out, buildSheet({ palettes, ramps, surfaces, captures }));
console.log(
  `\nWrote ${out}\n  ${captures.length} captures, ${measured} token instances measured` +
    `\n  ${palettes.length} palettes x ${ramps.length} ramps`,
);
