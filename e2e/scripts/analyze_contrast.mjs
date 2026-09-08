#!/usr/bin/env node
// analyze_contrast.mjs — turn a baseline/sentinel capture pair into a verdict (#2770).
//
//   node scripts/analyze_contrast.mjs --slug contrast [--json report.json]
//
// Pairs each capture from the baseline pass with its sentinel twin, attributes
// pixels to tokens by sentinel diff, and reports the composited contrast of
// every token on every surface. Exits non-zero if anything fails WCAG, so this
// doubles as the CI gate.
//
// PNG decoding is in-process (./png.mjs, built on Node's zlib) rather than a
// shell-out to ffmpeg. A full sweep decodes ~876 gate images: via ffmpeg that
// would be ~1,750 subprocess spawns and ~4.4GB of intermediate raw files, since
// each 960x1800 frame is ~5MB of rgb24. ffmpeg is also absent from the
// self-hosted runner image, so relying on it would couple this gate to a runner
// rebuild.

import { readFileSync, writeFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';

import { analyze, judge } from './contrast_analyzer.mjs';
import { decodePng } from './png.mjs';

function arg(name, fallback) {
  const i = process.argv.indexOf(`--${name}`);
  if (i >= 0 && process.argv[i + 1]) return process.argv[i + 1];
  if (fallback !== undefined) return fallback;
  throw new Error(`missing required --${name}`);
}

const slug = arg('slug', 'contrast');
const jsonOut = arg('json', '');
const root = join('investigations', slug);

/** Decodes a screenshot to a raw rgb24 buffer with its dimensions. */
function decode(path) {
  return decodePng(readFileSync(path));
}

const sentinelMapPath = join('..', 'design', 'sentinel_map.json');
if (!existsSync(sentinelMapPath)) {
  console.error(
    `analyze_contrast: ${sentinelMapPath} missing — run\n` +
      '  node scripts/gen_design_tokens.js --sentinel\n' +
      'from the repo root before the sentinel capture pass.',
  );
  process.exit(1);
}
const { hues, roles = {}, values = {} } = JSON.parse(readFileSync(sentinelMapPath, 'utf8'));

const baseManifest = JSON.parse(readFileSync(join(root, 'manifest-baseline.json'), 'utf8'));
const sentManifest = JSON.parse(readFileSync(join(root, 'manifest-sentinel.json'), 'utf8'));
const sentById = new Map(sentManifest.captures.map((c) => [c.id, c]));

const results = [];
let missing = 0;

for (const capture of baseManifest.captures) {
  const twin = sentById.get(capture.id);
  if (!twin) {
    console.warn(`  ! ${capture.id}: no sentinel twin, skipped`);
    missing++;
    continue;
  }
  const base = decode(capture.file);
  const sent = decode(twin.file);
  if (base.width !== sent.width || base.height !== sent.height) {
    console.warn(`  ! ${capture.id}: size mismatch between passes, skipped`);
    missing++;
    continue;
  }
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
  results.push({ ...capture, findings });
}

// ---------------------------------------------------------------------------
// Report
// ---------------------------------------------------------------------------

const failures = [];
for (const r of results) {
  for (const f of r.findings) if (!f.passes) failures.push({ capture: r, finding: f });
}

console.log(`\nComposited contrast — ${results.length} surfaces analysed\n`);
for (const r of results) {
  const bad = r.findings.filter((f) => !f.passes);
  const skipped = r.findings.filter((f) => f.lowConfidence).length;
  const mark = bad.length ? '✗' : '✓';
  console.log(
    `${mark} ${r.id}  (${r.findings.length} instances, ${bad.length} failing` +
      (skipped ? `, ${skipped} low-confidence` : '') +
      ')',
  );
  // One line per token: its worst instance. Listing every instance buries the
  // finding — a heading and its twenty glyph runs are one problem, not twenty.
  const worst = new Map();
  for (const f of bad) {
    if (!worst.has(f.token) || f.ratio < worst.get(f.token).ratio) worst.set(f.token, f);
  }
  const counts = new Map();
  for (const f of bad) counts.set(f.token, (counts.get(f.token) ?? 0) + 1);
  for (const [token, f] of [...worst].sort((a, b) => a[1].ratio - b[1].ratio)) {
    console.log(
      `    ${token.padEnd(16)} ${f.ratio.toFixed(2)}:1  (needs ${f.required}:1)` +
        `  fg [${f.foreground}] on bg [${f.background}]` +
        `  ×${counts.get(token)}`,
    );
  }
}

// Which tokens fail most often, across surfaces — this is what points at a token
// rather than at one unlucky screen.
const byToken = new Map();
for (const { finding } of failures) {
  const e = byToken.get(finding.token) ?? { count: 0, worst: Infinity };
  e.count++;
  e.worst = Math.min(e.worst, finding.ratio);
  byToken.set(finding.token, e);
}
if (byToken.size) {
  console.log('\nFailing tokens, by instance count:');
  for (const [token, e] of [...byToken].sort((a, b) => b[1].count - a[1].count)) {
    console.log(`  ${token.padEnd(16)} ${String(e.count).padStart(3)} instances, worst ${e.worst.toFixed(2)}:1`);
  }
}

if (jsonOut) {
  writeFileSync(jsonOut, JSON.stringify({ slug, results }, null, 2));
  console.log(`\nWrote ${jsonOut}`);
}

console.log(
  `\n${failures.length} failing token instances across ${results.length} surfaces` +
    (missing ? ` (${missing} captures skipped)` : ''),
);
process.exit(failures.length ? 1 : 0);
