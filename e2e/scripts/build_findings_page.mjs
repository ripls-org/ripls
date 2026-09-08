#!/usr/bin/env node
// build_findings_page.mjs — problem identification, one screen at a time (#2770).
//
//   node scripts/build_findings_page.mjs --slug photos --variant ink-sage__current \
//     --out ../docs/concepts/theme/contrast-findings.html
//
// This is deliberately NOT a candidate matrix. The matrix answered "which
// option looks better", which is a TUNING question, and it could not answer the
// two questions that come first:
//
//   1. Where exactly is the contrast bad? A headline number on a screenshot is
//      unfalsifiable — "home-feed light: 1.8:1" reads as wrong when the screen
//      looks fine, and there is no way to check. Every finding here is drawn as
//      a box on the pixels it was measured from, labelled with its token. If
//      the box lands on something that looks fine, the measurement is wrong and
//      that is worth knowing.
//
//   2. Is this element even following the theme? The sentinel pass re-renders
//      with every token replaced by a garish marker hue. Anything that still
//      looks NORMAL in that render is not reading from a token at all — it is a
//      hardcoded colour that no palette or ramp change will ever move. That is
//      why the sentinel render is shown as a full panel rather than used only
//      as measurement scaffolding.
//
// Tuning the scrim and the ramp comes after both are settled.

import {
  readFileSync,
  writeFileSync,
  existsSync,
  readdirSync,
  mkdirSync,
  copyFileSync,
} from 'node:fs';
import { join, dirname, basename } from 'node:path';

import { analyze, judge } from './contrast_analyzer.mjs';
import { decodePng } from './png.mjs';
import { SURFACES } from './contrast_surfaces.mjs';

function arg(name, fallback) {
  const i = process.argv.indexOf(`--${name}`);
  if (i >= 0 && process.argv[i + 1]) return process.argv[i + 1];
  if (fallback !== undefined) return fallback;
  throw new Error(`missing required --${name}`);
}

const slug = arg('slug', 'photos');
const variant = arg('variant', 'ink-sage__current');
const out = arg('out');
const root = join('investigations', slug);

// `--before <slug>` puts the previous render in the first column.
//
// Without it the page shows only what a change achieved, never what it cost.
// The scrim work is the case in point: the failure count is a clean 85 -> 5,
// and the whole question that number cannot answer is whether the photograph
// still looks like a photograph. Side by side, at the same size, is the only
// way to see that.
const beforeSlug = arg('before', '');
const beforeRoot = beforeSlug ? join('investigations', beforeSlug) : null;

const sentinelMapPath = join('..', 'design', 'sentinel_map.json');
const sentinelMap = JSON.parse(readFileSync(sentinelMapPath, 'utf8'));

const assetDir = join(dirname(out), basename(out).replace(/\.html$/, ''));
mkdirSync(assetDir, { recursive: true });

const basePath = join(root, `manifest-${variant}-baseline.json`);
const sentPath = join(root, `manifest-${variant}-sentinel.json`);
if (!existsSync(basePath) || !existsSync(sentPath)) {
  console.error(`build_findings_page: missing manifests for ${variant} in ${root}`);
  console.error('available: ' + readdirSync(root).filter((f) => f.startsWith('manifest-')).join(', '));
  process.exit(1);
}

const baseManifest = JSON.parse(readFileSync(basePath, 'utf8'));
const sentById = new Map(
  JSON.parse(readFileSync(sentPath, 'utf8')).captures.map((c) => [c.id, c]),
);

const descriptions = Object.fromEntries(SURFACES.map((s) => [s.id, s.description]));
const cases = [];

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
      sentinelMap: sentinelMap.hues,
    }),
    { roles: sentinelMap.roles ?? {}, values: sentinelMap.values ?? {} },
  );

  const baseAsset = `${capture.id}-baseline.png`;
  const sentAsset = `${capture.id}-sentinel.png`;
  copyFileSync(capture.file, join(assetDir, baseAsset));
  copyFileSync(twin.file, join(assetDir, sentAsset));

  // The prior render of the SAME capture id, if one was supplied. Matching by
  // id rather than by index — the two runs may cover different surface sets, so
  // a positional pairing would silently compare unrelated screens.
  let beforeRel = null;
  if (beforeRoot) {
    const beforeManifest = join(beforeRoot, `manifest-${variant}-baseline.json`);
    if (existsSync(beforeManifest)) {
      const prior = JSON.parse(readFileSync(beforeManifest, 'utf8')).captures.find(
        (c) => c.id === capture.id,
      );
      if (prior && existsSync(prior.file)) {
        const beforeAsset = `${capture.id}-before.png`;
        copyFileSync(prior.file, join(assetDir, beforeAsset));
        beforeRel = `${basename(assetDir)}/${beforeAsset}`;
      }
    }
  }

  const hex = (c) =>
    '#' + c.map((v) => Math.round(v).toString(16).padStart(2, '0')).join('');

  cases.push({
    id: capture.id,
    surface: capture.surface,
    theme: capture.theme,
    backdrop: capture.backdrop,
    width: base.width,
    height: base.height,
    baseline: `${basename(assetDir)}/${baseAsset}`,
    sentinel: `${basename(assetDir)}/${sentAsset}`,
    before: beforeRel,
    findings: findings
      // lowConfidence findings describe the rasterisation, not the token; they
      // are kept out of the default view but not deleted, because a box that
      // lands somewhere surprising is itself a signal about the analyzer.
      .map((f) => ({
        token: f.token,
        ratio: Number(f.ratio.toFixed(2)),
        required: f.required,
        passes: f.passes,
        role: f.role,
        low: !!f.lowConfidence,
        density: Number(f.density.toFixed(2)),
        pixels: f.pixels,
        fg: hex(f.foreground),
        bg: hex(f.background),
        b: f.bounds,
      }))
      .sort((a, b) => a.ratio - b.ratio),
  });
  const fails = cases.at(-1).findings.filter((f) => !f.passes && !f.low).length;
  console.log(`  ✓ ${capture.id}: ${cases.at(-1).findings.length} findings, ${fails} failing`);
}

const data = { variant, cases, descriptions };

const html = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Contrast findings — ${variant} (#2770)</title>
<style>
  :root { --bg:#fff; --fg:#191c19; --muted:#4a524c; --line:#ddddd4; --surface:#f2f2ee;
          --pass:#2e7041; --fail:#b5492b; }
  @media (prefers-color-scheme: dark) { :root {
    --bg:#141714; --fg:#f4f4ef; --muted:#abb3ab; --line:#323832; --surface:#1f2421;
    --pass:#7a9b76; --fail:#db7f63; } }
  * { box-sizing:border-box; }
  body { margin:0; background:var(--bg); color:var(--fg);
    font-family:"Public Sans",-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif; }
  header { position:sticky; top:0; z-index:20; background:var(--bg); border-bottom:1px solid var(--line); padding:14px 20px; }
  h1 { font-family:"Libre Baskerville",Baskerville,Georgia,serif; font-weight:400; font-size:19px; margin:0 0 6px; }
  .sub { color:var(--muted); font-size:12.5px; margin:0 0 12px; max-width:84ch; line-height:1.55; }
  .controls { display:flex; gap:16px; align-items:flex-end; flex-wrap:wrap; }
  .ctl { display:flex; flex-direction:column; gap:4px; }
  label { font-size:10px; letter-spacing:.09em; text-transform:uppercase; color:var(--muted); font-weight:600; }
  select { font:inherit; font-size:13px; padding:6px 8px; background:var(--surface); color:var(--fg);
    border:1px solid var(--line); border-radius:7px; min-width:210px; }
  .toggles { display:flex; gap:14px; align-items:center; font-size:12.5px; color:var(--muted); }
  main { padding:20px; display:flex; gap:26px; align-items:flex-start; flex-wrap:wrap; }
  .panel h2 { font-size:12px; margin:0 0 3px; letter-spacing:.04em; text-transform:uppercase; }
  .panel p { font-size:11.5px; color:var(--muted); margin:0 0 8px; max-width:34ch; line-height:1.5; }
  .stage { position:relative; display:inline-block; line-height:0; }
  .stage img { width:330px; height:auto; border-radius:10px; border:1px solid var(--line); }
  .box { position:absolute; border:1.5px solid var(--fail); border-radius:2px; cursor:pointer; }
  .box.ok { border-color:var(--pass); }
  .box.low { border-style:dotted; opacity:.65; }
  .box:hover { background:rgba(181,73,43,.18); }
  .list { flex:1; min-width:320px; max-width:560px; }
  table { border-collapse:collapse; width:100%; font-size:12px; }
  th,td { text-align:left; padding:5px 8px; border-bottom:1px solid var(--line); vertical-align:top; }
  th { font-size:10px; letter-spacing:.07em; text-transform:uppercase; color:var(--muted); }
  tr.sel td { background:var(--surface); }
  .ratio { font-variant-numeric:tabular-nums; font-weight:700; }
  .ratio.fail { color:var(--fail); } .ratio.pass { color:var(--pass); }
  .sw { display:inline-block; width:10px; height:10px; border-radius:2px; border:1px solid var(--line); vertical-align:-1px; }
  .empty { color:var(--muted); font-size:12.5px; padding:10px 0; }
  footer { padding:0 20px 40px; color:var(--muted); font-size:11.5px; max-width:88ch; line-height:1.6; }
</style></head><body>
<header>
  <h1>Contrast findings — <span id="variantName"></span></h1>
  <p class="sub"><strong>Left</strong> is the screen as it renders. <strong>Middle</strong> draws a box
  around the exact pixels each measurement came from — if a box lands on something that looks fine, the
  measurement is wrong and that is worth knowing. <strong>Right</strong> is the same screen re-rendered
  with every design token replaced by a marker hue: <em>anything that still looks normal there is not
  reading from a token at all</em>, so no palette or ramp change will ever move it.</p>
  <div class="controls">
    <div class="ctl"><label for="case">Screen / theme / photo</label><select id="case"></select></div>
    <div class="toggles">
      <label style="text-transform:none;letter-spacing:0;font-size:12.5px;font-weight:400">
        <input type="checkbox" id="showPass"> also box passing findings</label>
      <label style="text-transform:none;letter-spacing:0;font-size:12.5px;font-weight:400">
        <input type="checkbox" id="showLow"> include low-confidence</label>
    </div>
  </div>
</header>
<main>
  <div class="panel" id="panelBefore" hidden><h2>0 · Before</h2><p>The previous render, same screen and photo. Compare against panel 1 for what the change <em>cost</em>, not just what it fixed.</p>
    <div class="stage"><img id="imgBefore" alt="before"></div></div>
  <div class="panel"><h2>1 · As rendered</h2><p>What a user sees now.</p>
    <div class="stage"><img id="imgA" alt="baseline"></div></div>
  <div class="panel"><h2>2 · Where the numbers come from</h2><p>Red = fails its threshold. Hover a box to highlight its row.</p>
    <div class="stage"><img id="imgB" alt="baseline with findings"><div id="boxes"></div></div></div>
  <div class="panel"><h2>3 · Theme compliance</h2><p>Tokens replaced by marker hues. Anything still looking normal is hardcoded.</p>
    <div class="stage"><img id="imgC" alt="sentinel"></div></div>
  <div class="list"><h2 style="font-size:12px;letter-spacing:.04em;text-transform:uppercase;margin:0 0 8px">Findings</h2>
    <div id="tableWrap"></div></div>
</main>
<footer id="note"></footer>
<script>
const DATA = ${JSON.stringify(data)};
const $ = (id) => document.getElementById(id);
$('variantName').textContent = DATA.variant;
$('case').innerHTML = DATA.cases.map((c,i) =>
  '<option value="'+i+'">'+c.surface+' · '+c.theme+' · '+c.backdrop+'</option>').join('');

function visible(c) {
  return c.findings.filter((f) =>
    (f.low ? $('showLow').checked : true) && (f.passes ? $('showPass').checked : true));
}
function render() {
  const c = DATA.cases[+$('case').value];
  $('imgA').src = c.baseline; $('imgB').src = c.baseline; $('imgC').src = c.sentinel;
  const bp = $('panelBefore');
  if (c.before) { $('imgBefore').src = c.before; bp.hidden = false; } else { bp.hidden = true; }
  const shown = visible(c);
  $('boxes').innerHTML = shown.map((f,i) =>
    '<div class="box '+(f.passes?'ok':'')+(f.low?' low':'')+'" data-i="'+i+'" title="'+f.token+' — '+f.ratio+':1 (needs '+f.required+')" style="left:'+
    (100*f.b.x/c.width)+'%;top:'+(100*f.b.y/c.height)+'%;width:'+(100*f.b.width/c.width)+
    '%;height:'+(100*f.b.height/c.height)+'%"></div>').join('');
  $('tableWrap').innerHTML = shown.length ? '<table><thead><tr><th>Ratio</th><th>Token</th><th>Role</th><th>fg / bg</th><th>Size</th></tr></thead><tbody>'
    + shown.map((f,i) => '<tr data-i="'+i+'"><td class="ratio '+(f.passes?'pass':'fail')+'">'+f.ratio.toFixed(2)+':1'
      + '<div style="font-weight:400;color:var(--muted);font-size:10px">needs '+f.required+'</div></td>'
      + '<td>'+f.token+(f.low?' <span style="color:var(--muted)">(low conf)</span>':'')+'</td><td>'+f.role+'</td>'
      + '<td><span class="sw" style="background:'+f.fg+'"></span> '+f.fg+'<br><span class="sw" style="background:'+f.bg+'"></span> '+f.bg+'</td>'
      + '<td>'+f.pixels+'px<div style="color:var(--muted);font-size:10px">density '+f.density+'</div></td></tr>').join('')
    + '</tbody></table>'
    : '<p class="empty">No findings match the current filters — everything measured on this screen passes.</p>';
  for (const el of document.querySelectorAll('.box')) {
    el.addEventListener('mouseenter', () => {
      const r = document.querySelector('tr[data-i="'+el.dataset.i+'"]'); if (r) r.classList.add('sel');
    });
    el.addEventListener('mouseleave', () => {
      const r = document.querySelector('tr[data-i="'+el.dataset.i+'"]'); if (r) r.classList.remove('sel');
    });
  }
  $('note').textContent = DATA.descriptions[c.surface] ?? '';
}
for (const id of ['case','showPass','showLow']) $(id).addEventListener('change', render);
render();
</script></body></html>
`;

writeFileSync(out, html);
const totalFail = cases.reduce(
  (n, c) => n + c.findings.filter((f) => !f.passes && !f.low).length,
  0,
);
console.log(
  `\nWrote ${out}\n  ${cases.length} screens, ${totalFail} failing findings (excluding low-confidence)`,
);
