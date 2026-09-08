#!/usr/bin/env node
// build_pr_contrast_page.mjs — the reviewer-facing before/after for #2770.
//
//   node scripts/build_pr_contrast_page.mjs --before <slug> --after <slug> \
//     --out ../website/content/contrast-review/index.html
//
// Lives under website/content/ so the staging deploy publishes it with the PR
// and a reviewer can look at the change instead of reading a diff of hex
// values. Every other page this branch produced was a working artefact for one
// question; this one is the summary.
//
// "Before" is a capture of the PR's BASE (origin/main), not an early state of
// the branch — the branch was already several fixes deep by the time the first
// captures existed, so an in-branch snapshot would understate the change.
//
// Baseline renders only. The sentinel pass and the finding boxes answer "is
// this measurement real", which was the working question; the reviewer's
// question is "does the app look better", and boxes get in the way of it.

import {
  readdirSync,
  readFileSync,
  writeFileSync,
  mkdirSync,
  copyFileSync,
  existsSync,
} from 'node:fs';
import { join, dirname, basename } from 'node:path';

function arg(name, fallback) {
  const i = process.argv.indexOf(`--${name}`);
  if (i >= 0 && process.argv[i + 1]) return process.argv[i + 1];
  if (fallback !== undefined) return fallback;
  throw new Error(`missing required --${name}`);
}

// Directories of baseline PNGs, one per side.
const beforeSlug = arg('before');
const afterSlug = arg('after');
const out = arg('out');

const esc = (s) =>
  String(s).replace(
    /[&<>"]/g,
    (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c],
  );

/**
 * Every baseline PNG under a capture directory, keyed by capture id.
 *
 * Reads the directory rather than the manifest deliberately. A capture run
 * writes `manifest-<variant>-baseline.json`, so running two SCOPES under one
 * slug — the photo surfaces then the flat ones — makes the second overwrite the
 * first. The PNGs all survive; the manifest ends up describing only the last
 * scope. Pairing by filename is immune to that, and the filenames already carry
 * the full identity (`<surface>-<theme>-<backdrop>.png`).
 */
function collect(dir) {
  if (!existsSync(dir)) {
    console.error(`build_pr_contrast_page: no captures at ${dir}`);
    process.exit(1);
  }
  const out = new Map();
  for (const f of readdirSync(dir)) {
    if (!f.endsWith('.png')) continue;
    out.set(f.replace(/\.png$/, ''), { file: join(dir, f) });
  }
  return out;
}

const before = collect(beforeSlug);
const after = collect(afterSlug);

const assetDir = join(dirname(out), 'shots');
mkdirSync(assetDir, { recursive: true });

// Only ids present on BOTH sides. A one-sided row is not a comparison, and
// silently rendering it as "improved" would be the most flattering possible
// reading of a capture that simply did not exist before.
const ids = [...after.keys()].filter((id) => before.has(id)).sort();
const onlyAfter = [...after.keys()].filter((id) => !before.has(id));
const onlyBefore = [...before.keys()].filter((id) => !after.has(id));

const rows = [];
for (const id of ids) {
  const b = `${id}-before.png`;
  const a = `${id}-after.png`;
  copyFileSync(before.get(id).file, join(assetDir, b));
  copyFileSync(after.get(id).file, join(assetDir, a));
  const [, surface, theme, backdrop] =
    id.match(/^(.*)-(light|dark)-(.*)$/) ?? [null, id, '', ''];
  rows.push({ id, surface, theme, backdrop, before: `shots/${b}`, after: `shots/${a}` });
}

const surfaces = [...new Set(rows.map((r) => r.surface))];

const LABELS = {
  'gear-detail-hero': 'Item detail — content over a photo',
  'experience-detail-hero': 'Event detail — content over a photo',
  'glass-date-picker': 'Glass date picker — a frosted sheet',
  'material-date-picker': 'Date dialog from event creation (#2764)',
  'home-feed': 'Home feed — cards, and the bottom tab dock',
  'request-detail': 'Request detail — flat surfaces (#2445 control)',
};

const html = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Ripls — Contrast review (#2770)</title>
<meta name="robots" content="noindex">
<link rel="stylesheet" href="/css/fonts.css">
<link rel="stylesheet" href="/css/gen/tokens.gen.css">
<style>
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body { background: var(--light-background); color: var(--light-text-primary);
    font-family: var(--font-sans); }
  h1, h2 { font-family: var(--font-serif); font-weight: 600; }
  .wrap { max-width: 1100px; margin: 0 auto; padding: 3rem 1.5rem 5rem; }
  h1 { font-size: clamp(1.6rem, 3.5vw, 2.4rem); line-height: 1.2; }
  .lede { color: var(--light-text-secondary); margin-top: .75rem; max-width: 68ch;
    line-height: 1.6; font-size: 1rem; }
  .controls { display: flex; gap: 1.25rem; flex-wrap: wrap; align-items: flex-end;
    margin: 2rem 0 1rem; padding-bottom: 1rem;
    border-bottom: 1px solid var(--light-border); }
  .ctl { display: flex; flex-direction: column; gap: .3rem; }
  label { font-size: .68rem; letter-spacing: .09em; text-transform: uppercase;
    color: var(--light-text-faint); font-weight: 600; }
  select { font: inherit; font-size: .9rem; padding: .45rem .6rem;
    background: var(--light-surface); color: var(--light-text-primary);
    border: 1px solid var(--light-border); border-radius: .45rem; min-width: 15rem; }
  h2 { font-size: 1.15rem; margin: 2.5rem 0 .25rem; }
  .sub { color: var(--light-text-faint); font-size: .82rem; margin-bottom: 1rem; }
  .pair { display: flex; gap: 1.5rem; flex-wrap: wrap; }
  figure { margin: 0; }
  figcaption { font-size: .7rem; letter-spacing: .09em; text-transform: uppercase;
    color: var(--light-text-faint); font-weight: 600; margin-bottom: .4rem; }
  img { width: 300px; height: auto; display: block; border-radius: .6rem;
    border: 1px solid var(--light-border); background: var(--light-surface); }
  .note { background: var(--light-surface); border: 1px solid var(--light-border);
    border-radius: .6rem; padding: 1rem 1.15rem; margin-top: 2.5rem;
    font-size: .88rem; line-height: 1.65; color: var(--light-text-secondary); }
  .note strong { color: var(--light-text-primary); }
  .note ul { margin: .5rem 0 0 1.1rem; }
  .note li { margin: .3rem 0; }
  .temp { border: 1px solid var(--light-warning); border-left-width: 4px;
    border-radius: .4rem; padding: .8rem 1rem; margin-bottom: 2rem;
    font-size: .85rem; line-height: 1.6; color: var(--light-text-secondary); }
  .temp strong { color: var(--light-text-primary); }
  code { font-size: .82em; background: var(--light-surface); padding: .1em .35em;
    border-radius: .25rem; }
</style>
</head>
<body>
<div class="wrap">
  <div class="temp"><strong>Temporary review artifact — delete before merge.</strong>
  This directory carries ~40&nbsp;MB of PNG that exists only so #2789 can be reviewed on
  staging. Remove <code>website/content/contrast-review/</code> once the PR is approved;
  it regenerates from <code>e2e/scripts/build_pr_contrast_page.mjs</code>.</div>
  <h1>Contrast &amp; theme review</h1>
  <p class="lede">Real in-app renders, same seeded content and same backdrop photo on both
  sides. <strong>Before</strong> is this PR's base branch; <strong>after</strong> is the branch.
  Use the pickers to change screen, theme and backdrop.</p>

  <div class="controls">
    <div class="ctl"><label for="surface">Screen</label><select id="surface"></select></div>
    <div class="ctl"><label for="theme">Theme</label><select id="theme"></select></div>
    <div class="ctl"><label for="backdrop">Backdrop</label><select id="backdrop"></select></div>
  </div>

  <h2 id="title"></h2>
  <p class="sub" id="sub"></p>
  <div class="pair">
    <figure><figcaption>Before — base branch</figcaption><img id="imgBefore" alt="before"></figure>
    <figure><figcaption>After — this PR</figcaption><img id="imgAfter" alt="after"></figure>
  </div>

  <div class="note">
    <strong>What changed, and what did not.</strong>
    <ul>
      <li>Status colours resolve per theme (#2445). Light theme was rendering the DARK
          theme's values at 1.86–2.76:1; the correct light values measure 4.73–5.73:1.</li>
      <li>The glass action colour was the LIGHT theme's green on a material that is always
          dark (#2764). It measured 2.38:1 as a foreground on the sheet; the light sage
          measures 9.01:1.</li>
      <li>The media scrim sampled only the bottom 40% of a photo, so a sunset's bright sky
          sat unwashed behind text at 39% down — 1.56:1. It now samples the band the text
          actually occupies.</li>
      <li>The bottom tab dock called itself a "committed light material" but was 42% white,
          which is light only over a light page. Over the dark theme it composited to grey
          119 and its dark glyphs measured 1.82:1.</li>
      <li><strong>Photos are darker on some screens.</strong> That is the cost of the scrim
          change, not a side effect — legibility over a bright photo is bought with wash.
          Judge it here rather than in the numbers.</li>
    </ul>
  </div>
</div>
<script>
const ROWS = ${JSON.stringify(rows)};
const LABELS = ${JSON.stringify(LABELS)};
const $ = (id) => document.getElementById(id);
const uniq = (a) => [...new Set(a)];

function fill(sel, values, labeller) {
  sel.innerHTML = values.map((v) =>
    '<option value="' + v + '">' + (labeller ? labeller(v) : v) + '</option>').join('');
}
fill($('surface'), uniq(ROWS.map((r) => r.surface)), (s) => LABELS[s] || s);

function refine() {
  const s = $('surface').value;
  const forS = ROWS.filter((r) => r.surface === s);
  const t = $('theme').value;
  fill($('theme'), uniq(forS.map((r) => r.theme)));
  if (uniq(forS.map((r) => r.theme)).includes(t)) $('theme').value = t;
  const forT = forS.filter((r) => r.theme === $('theme').value);
  const b = $('backdrop').value;
  fill($('backdrop'), uniq(forT.map((r) => r.backdrop)));
  if (uniq(forT.map((r) => r.backdrop)).includes(b)) $('backdrop').value = b;
  render();
}
function render() {
  const r = ROWS.find((x) => x.surface === $('surface').value
    && x.theme === $('theme').value && x.backdrop === $('backdrop').value);
  if (!r) return;
  $('imgBefore').src = r.before;
  $('imgAfter').src = r.after;
  $('title').textContent = LABELS[r.surface] || r.surface;
  $('sub').textContent = r.theme + ' theme · backdrop: ' + r.backdrop;
}
$('surface').addEventListener('change', refine);
$('theme').addEventListener('change', refine);
$('backdrop').addEventListener('change', render);
refine();
</script>
</body>
</html>
`;

writeFileSync(out, html);
console.log(`Wrote ${out}`);
console.log(`  ${rows.length} paired screens across ${surfaces.length} surfaces`);
if (onlyAfter.length) console.log(`  skipped (after only): ${onlyAfter.join(', ')}`);
if (onlyBefore.length) console.log(`  skipped (before only): ${onlyBefore.join(', ')}`);
