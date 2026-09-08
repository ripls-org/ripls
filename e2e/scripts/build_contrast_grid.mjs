#!/usr/bin/env node
// build_contrast_grid.mjs — the side-by-side candidate grid (#2770).
//
//   node scripts/build_contrast_grid.mjs --slug photos --out ../docs/concepts/theme/contrast-grid.html
//
// Replaces the first comparison sheet, which stacked captures vertically. That
// made the only question worth asking — "is this candidate better than that
// one?" — a memory exercise: you scrolled from one to the next and tried to
// hold the difference in your head. Options have to sit side by side.
//
// Layout follows from which axes are being *chosen*:
//   rows    = glass ramp      \  the two candidate axes, always both visible
//   columns = palette         /
//   dropdowns = screen, theme, backdrop — these select WHICH grid you see,
//               they are not additional rows
//
// So one row reads "same palette set, four materials" and one column reads
// "same material, three palettes". Nothing is hidden behind a scroll.
//
// Backdrops here are real photographs, not the synthetic white/black/noise
// fields. Those bound the worst case and are right for GATING, but under a wash
// they are flat grey rectangles — they cannot answer "does the photo still look
// like a photo", which is the judgement this page exists for.

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
const out = arg('out');
const root = join('investigations', slug);

const mapPath = join('..', 'design', 'sentinel_map.json');
const sentinel = existsSync(mapPath)
  ? JSON.parse(readFileSync(mapPath, 'utf8'))
  : null;

const candidatesDir = join('..', 'design', 'candidates');
const candidates = readdirSync(candidatesDir)
  .filter((f) => f.endsWith('.json'))
  .map((f) => JSON.parse(readFileSync(join(candidatesDir, f), 'utf8')));
const palettes = candidates.filter((c) => c.axis === 'palette');
const ramps = candidates.filter((c) => c.axis === 'glass');

// Assets live next to the page. A page that only renders from someone's
// gitignored scratch directory is not a decision artifact.
const assetDir = join(dirname(out), basename(out).replace(/\.html$/, ''));
mkdirSync(assetDir, { recursive: true });

const manifests = readdirSync(root).filter((f) =>
  /^manifest-.+-baseline\.json$/.test(f),
);
if (manifests.length === 0) {
  console.error(`build_contrast_grid: no baseline manifests in ${root}`);
  process.exit(1);
}

// Unequal capture counts mean manifests from more than one run are present, so
// the grid would compare candidates rendered from different code states — a
// wrong answer wearing the costume of a decision artifact.
{
  const counts = manifests.map((f) => {
    const m = JSON.parse(readFileSync(join(root, f), 'utf8'));
    return { variant: m.variant, n: m.captures.length };
  });
  if (new Set(counts.map((c) => c.n)).size > 1) {
    console.error(
      'build_contrast_grid: variants have different capture counts — refusing to build.\n',
    );
    for (const c of counts.sort((a, b) => a.n - b.n)) {
      console.error(`  ${String(c.n).padStart(3)}  ${c.variant}`);
    }
    process.exit(1);
  }
}

const captures = [];
let measured = 0;

for (const file of manifests) {
  const baseManifest = JSON.parse(readFileSync(join(root, file), 'utf8'));
  const variant = baseManifest.variant;
  const [palette, ramp] = variant.split('__');
  const sentPath = join(root, file.replace('-baseline.json', '-sentinel.json'));
  const sentById = existsSync(sentPath)
    ? new Map(
        JSON.parse(readFileSync(sentPath, 'utf8')).captures.map((c) => [c.id, c]),
      )
    : new Map();

  for (const capture of baseManifest.captures) {
    let worst = null;

    // The badge is a floor, not the verdict — this page is for eyes. When the
    // sentinel twin is missing the image still renders; only the number drops.
    const twin = sentById.get(capture.id);
    if (twin && sentinel) {
      const base = decodePng(readFileSync(capture.file));
      const sent = decodePng(readFileSync(twin.file));
      if (base.width === sent.width && base.height === sent.height) {
        const findings = judge(
          analyze({
            baseline: base.buffer,
            sentinel: sent.buffer,
            width: base.width,
            height: base.height,
            sentinelMap: sentinel.hues,
          }),
          { roles: sentinel.roles ?? {}, values: sentinel.values ?? {} },
        );
        measured += findings.length;
        const judged = findings.filter(
          (f) => !f.lowConfidence && f.required !== null,
        );
        if (judged.length) {
          const w = judged.reduce((a, b) => (b.ratio < a.ratio ? b : a));
          worst = { token: w.token, ratio: w.ratio, required: w.required };
        }
      }
    }

    const assetName = `${variant}-${capture.id}.png`;
    copyFileSync(capture.file, join(assetDir, assetName));
    captures.push({
      palette,
      ramp,
      surface: capture.surface,
      theme: capture.theme,
      backdrop: capture.backdrop,
      file: `${basename(assetDir)}/${assetName}`,
      worst,
    });
  }
  console.log(`  ✓ ${variant}: ${baseManifest.captures.length} captures`);
}

/**
 * How much each axis actually moves on each screen.
 *
 * Without this the page invites a wrong conclusion. `gear-detail-hero` renders
 * four byte-identical rows, because the ramp candidates vary `glass.*` and that
 * material governs sheets — a photo-backed hero is the `overlay` material and
 * does not respond at all. Someone scanning that screen would reasonably decide
 * the four materials are indistinguishable. Measured: ramp moves 0% of pixels
 * there and 90.6% on `glass-date-picker`.
 *
 * So each screen carries a note saying which axis is live on it. Cheap to
 * compute: one image pair per axis per screen.
 */
function pixelDelta(fileA, fileB) {
  const a = decodePng(readFileSync(join(assetDir, fileA)));
  const b = decodePng(readFileSync(join(assetDir, fileB)));
  if (a.width !== b.width || a.height !== b.height) return null;
  let differing = 0;
  const total = a.buffer.length / 3;
  for (let i = 0; i < a.buffer.length; i += 3) {
    const d =
      Math.abs(a.buffer[i] - b.buffer[i]) +
      Math.abs(a.buffer[i + 1] - b.buffer[i + 1]) +
      Math.abs(a.buffer[i + 2] - b.buffer[i + 2]);
    if (d > 10) differing++;
  }
  return (100 * differing) / total;
}

function axisVariance(surfaceId) {
  const rows = captures.filter((c) => c.surface === surfaceId);
  if (rows.length === 0) return null;
  const { theme, backdrop } = rows[0];
  const cell = (p, r) =>
    rows.find(
      (c) =>
        c.palette === p && c.ramp === r && c.theme === theme && c.backdrop === backdrop,
    );
  const p0 = palettes[0]?.id;
  const r0 = ramps[0]?.id;
  const otherPalette = palettes.find((p) => p.id !== p0 && cell(p.id, r0));
  const otherRamp = ramps.find((r) => r.id !== r0 && cell(p0, r.id));
  const base = cell(p0, r0);
  if (!base) return null;
  const out = {};
  if (otherRamp) {
    const d = pixelDelta(basename(base.file), basename(cell(p0, otherRamp.id).file));
    if (d !== null) out.ramp = d;
  }
  if (otherPalette) {
    const d = pixelDelta(basename(base.file), basename(cell(otherPalette.id, r0).file));
    if (d !== null) out.palette = d;
  }
  return out;
}

const surfaces = SURFACES.filter((s) =>
  captures.some((c) => c.surface === s.id),
).map((s) => ({
  id: s.id,
  label: s.id,
  description: s.description,
  variance: axisVariance(s.id),
}));

const themes = [...new Set(captures.map((c) => c.theme))].sort();
const backdrops = [...new Set(captures.map((c) => c.backdrop))].sort();

/** The combination currently shipped, marked so the eye has an anchor. */
const SHIPPED = { palette: 'ink-sage', ramp: 'current' };

const data = {
  palettes: palettes.map((p) => ({ id: p.id, label: p.label })),
  ramps: ramps.map((r) => ({ id: r.id, label: r.label })),
  surfaces,
  themes,
  backdrops,
  captures,
  shipped: SHIPPED,
};

const html = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Contrast candidates — ramp × palette (#2770)</title>
<style>
  :root {
    --bg: #ffffff; --fg: #191c19; --muted: #4a524c; --line: #ddddd4;
    --surface: #f2f2ee; --pass: #2e7041; --fail: #b5492b;
  }
  @media (prefers-color-scheme: dark) {
    :root { --bg:#141714; --fg:#f4f4ef; --muted:#abb3ab; --line:#323832; --surface:#1f2421; --pass:#7a9b76; --fail:#db7f63; }
  }
  * { box-sizing: border-box; }
  body { margin:0; background:var(--bg); color:var(--fg);
    font-family:"Public Sans",-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif; }
  header { position:sticky; top:0; z-index:10; background:var(--bg);
    border-bottom:1px solid var(--line); padding:14px 20px; }
  h1 { font-family:"Libre Baskerville",Baskerville,Georgia,serif; font-size:19px; margin:0 0 4px; font-weight:400; }
  .sub { color:var(--muted); font-size:12.5px; margin:0 0 12px; max-width:78ch; line-height:1.5; }
  .controls { display:flex; gap:16px; flex-wrap:wrap; align-items:flex-end; }
  .ctl { display:flex; flex-direction:column; gap:4px; }
  .ctl label { font-size:10px; letter-spacing:.09em; text-transform:uppercase; color:var(--muted); font-weight:600; }
  select { font:inherit; font-size:13px; padding:6px 8px; background:var(--surface);
    color:var(--fg); border:1px solid var(--line); border-radius:7px; min-width:190px; }
  .scroll { overflow-x:auto; padding:20px; }
  table { border-collapse:separate; border-spacing:0; }
  th, td { padding:0; }
  thead th { position:sticky; top:0; background:var(--bg); padding:0 8px 10px; text-align:center;
    font-size:12.5px; font-weight:600; vertical-align:bottom; }
  thead th .hint { display:block; font-weight:400; font-size:10.5px; color:var(--muted); margin-top:2px; }
  tbody th { position:sticky; left:0; background:var(--bg); text-align:right; padding-right:12px;
    font-size:12.5px; font-weight:600; white-space:nowrap; vertical-align:middle; }
  tbody th .hint { display:block; font-weight:400; font-size:10.5px; color:var(--muted); }
  td { padding:0 8px 18px; vertical-align:top; }
  figure { margin:0; width:230px; }
  img { width:230px; height:auto; display:block; border-radius:9px; border:1px solid var(--line); background:var(--surface); }
  .shipped img { outline:2px solid var(--fg); outline-offset:2px; }
  .cap { display:flex; align-items:center; gap:6px; margin-top:5px; font-size:11px; color:var(--muted); }
  .badge { font-variant-numeric:tabular-nums; font-weight:700; }
  .badge.pass { color:var(--pass); } .badge.fail { color:var(--fail); }
  .tag { font-size:9.5px; letter-spacing:.07em; text-transform:uppercase; border:1px solid var(--line);
    border-radius:4px; padding:1px 4px; color:var(--muted); }
  .missing { width:230px; height:150px; display:grid; place-items:center; border:1px dashed var(--line);
    border-radius:9px; color:var(--muted); font-size:11.5px; }
  footer { padding:16px 20px 40px; color:var(--muted); font-size:11.5px; max-width:82ch; line-height:1.6; }
  code { background:var(--surface); padding:1px 4px; border-radius:4px; font-size:11px; }
  footer p { margin:0 0 8px; }
  .variance { border-left:2px solid var(--line); padding-left:10px; }
</style>
</head>
<body>
<header>
  <h1>Contrast candidates — glass ramp × palette</h1>
  <p class="sub">Real in-app renderings. Rows are the glass material, columns are the palette, so a row
  compares four materials under one palette and a column compares three palettes under one material.
  The dropdowns pick which screen you are looking at. The outlined cell is what ships today. Numbers are
  the <em>worst</em> measured token on that screen — a floor to catch regressions, not the verdict; the
  point of this page is your eyes.</p>
  <div class="controls">
    <div class="ctl"><label for="surface">Screen</label><select id="surface"></select></div>
    <div class="ctl"><label for="theme">Theme</label><select id="theme"></select></div>
    <div class="ctl"><label for="backdrop">Backdrop photo</label><select id="backdrop"></select></div>
  </div>
</header>
<div class="scroll"><table id="grid"></table></div>
<footer id="note"></footer>
<script>
const DATA = ${JSON.stringify(data)};
const $ = (id) => document.getElementById(id);
const key = (c) => [c.palette,c.ramp,c.surface,c.theme,c.backdrop].join('|');
const INDEX = new Map(DATA.captures.map((c) => [key(c), c]));

function fill(sel, items, fmt) {
  sel.innerHTML = items.map((i) => '<option value="'+i.id+'">'+fmt(i)+'</option>').join('');
}
fill($('surface'), DATA.surfaces, (s) => s.label);
fill($('theme'), DATA.themes.map((t) => ({id:t})), (t) => t.id);
fill($('backdrop'), DATA.backdrops.map((b) => ({id:b})), (b) => b.id);

function render() {
  const surface = $('surface').value, theme = $('theme').value, backdrop = $('backdrop').value;
  let head = '<thead><tr><th></th>' + DATA.palettes.map((p) =>
    '<th>'+p.label+'</th>').join('') + '</tr></thead>';
  let body = '<tbody>' + DATA.ramps.map((r) => {
    const cells = DATA.palettes.map((p) => {
      const c = INDEX.get([p.id,r.id,surface,theme,backdrop].join('|'));
      if (!c) return '<td><div class="missing">not captured</div></td>';
      const isShipped = p.id===DATA.shipped.palette && r.id===DATA.shipped.ramp;
      let badge = '<span class="tag">not measured</span>';
      if (c.worst) {
        const ok = c.worst.ratio >= c.worst.required;
        badge = '<span class="badge '+(ok?'pass':'fail')+'">'+c.worst.ratio.toFixed(2)+':1</span>'
              + '<span class="tag">'+c.worst.token+'</span>';
      }
      return '<td><figure class="'+(isShipped?'shipped':'')+'">'
           + '<img loading="lazy" src="'+c.file+'" alt="'+p.id+' / '+r.id+'">'
           + '<figcaption class="cap">'+badge+(isShipped?'<span class="tag">shipped</span>':'')+'</figcaption>'
           + '</figure></td>';
    }).join('');
    return '<tr><th>'+r.label.replace(/ \\(.*/,'')+'<span class="hint">'+(r.label.match(/\\((.*)\\)/)?.[1]??'')+'</span></th>'+cells+'</tr>';
  }).join('') + '</tbody>';
  $('grid').innerHTML = head + body;
  const s = DATA.surfaces.find((x) => x.id === surface);
  let note = s && s.description ? '<p>' + s.description + '</p>' : '';
  const v = s && s.variance;
  if (v) {
    const bits = [];
    const fmt = (n) => n.toFixed(1) + '% of pixels';
    if (typeof v.ramp === 'number')
      bits.push(v.ramp < 0.5
        ? '<strong>The ramp does not vary on this screen</strong> — the rows are identical. Glass tokens govern sheets and modals; a photo-backed hero is the overlay material and does not respond to them.'
        : 'Changing the ramp moves ' + fmt(v.ramp) + '.');
    if (typeof v.palette === 'number')
      bits.push(v.palette < 0.5
        ? '<strong>The palette does not vary on this screen.</strong>'
        : 'Changing the palette moves ' + fmt(v.palette) + '.');
    if (bits.length) note += '<p class="variance">' + bits.join(' ') + '</p>';
  }
  $('note').innerHTML = note;
}
for (const id of ['surface','theme','backdrop']) $(id).addEventListener('change', render);
render();
</script>
</body>
</html>
`;

writeFileSync(out, html);
console.log(
  `\nWrote ${out}\n  ${captures.length} captures, ${measured} token instances measured` +
    `\n  ${ramps.length} ramps x ${palettes.length} palettes, ` +
    `${surfaces.length} screens x ${themes.length} themes x ${backdrops.length} backdrops`,
);
