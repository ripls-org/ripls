#!/usr/bin/env node
// build_contrast_sheet.mjs — the candidate comparison sheet (#2770 Phase 2).
//
//   node scripts/build_contrast_sheet.mjs --sweep <sweep.json> --out <page.html>
//
// Palette and glass ramp are INDEPENDENT axes with their own pickers, rather
// than a flattened grid of every combination: 3 palettes x 3 ramps x 2 themes x
// 4 backdrops is 72 columns per surface, which is not reviewable. The picker
// keeps one variable moving at a time, which is how the question actually gets
// answered — does the material or the palette drive legibility.
//
// Groups render in evaluation-set order, with glass+media and flat expanded and
// the rest behind disclosure, so the rows for any one candidate stay scannable.
//
// Input shape (produced by the sweep):
//   {
//     "palettes":  [{ "id": "ink-sage", "label": "Ink & Sage" }, ...],
//     "ramps":     [{ "id": "current",  "label": "Current glass" }, ...],
//     "surfaces":  [{ "id": "...", "group": "glassMedia", "description": "..." }],
//     "captures":  [{ "palette": "...", "ramp": "...", "surface": "...",
//                     "theme": "light", "backdrop": "white",
//                     "file": "relative/path.png",
//                     "worst": { "token": "...", "ratio": 1.64, "required": 4.5 } }]
//   }

import { readFileSync, writeFileSync } from 'node:fs';

function arg(name, fallback) {
  const i = process.argv.indexOf(`--${name}`);
  if (i >= 0 && process.argv[i + 1]) return process.argv[i + 1];
  if (fallback !== undefined) return fallback;
  throw new Error(`missing required --${name}`);
}

const GROUP_LABELS = {
  glassMedia: 'Glass + media core',
  flat: 'Flat surfaces + status states',
  bespoke: 'Other bespoke materials',
  creation: 'Creation & preview flows',
};
/** Collapsed by default — reviewing every group at once is overwhelming. */
const COLLAPSED_GROUPS = new Set(['bespoke', 'creation']);

const esc = (s) =>
  String(s).replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]);

export function buildSheet(sweep) {
  const { palettes, ramps, surfaces, captures } = sweep;
  const themes = [...new Set(captures.map((c) => c.theme))];
  const backdrops = [...new Set(captures.map((c) => c.backdrop))];

  const byGroup = new Map();
  for (const s of surfaces) {
    if (!byGroup.has(s.group)) byGroup.set(s.group, []);
    byGroup.get(s.group).push(s);
  }

  const sections = [...byGroup]
    .sort((a, b) => Object.keys(GROUP_LABELS).indexOf(a[0]) - Object.keys(GROUP_LABELS).indexOf(b[0]))
    .map(([group, groupSurfaces]) => {
      const rows = groupSurfaces
        .map((s) => {
          const cells = captures
            .filter((c) => c.surface === s.id)
            .map((c) => {
              const worst = c.worst;
              const fails = worst && worst.required && worst.ratio < worst.required;
              return `<figure class="shot" data-palette="${esc(c.palette)}" data-ramp="${esc(c.ramp)}" data-theme="${esc(c.theme)}" data-backdrop="${esc(c.backdrop)}">
  <img loading="lazy" src="${esc(c.file)}" alt="${esc(s.id)} — ${esc(c.theme)} on ${esc(c.backdrop)}">
  <figcaption class="${fails ? 'fail' : 'pass'}">${
    worst
      ? `${esc(worst.token)} ${worst.ratio.toFixed(2)}:1${worst.required ? ` / ${worst.required}` : ''}`
      : 'no measurement'
  }</figcaption>
</figure>`;
            })
            .join('\n');
          return `<section class="surface">
  <h3>${esc(s.id)}</h3>
  <p class="desc">${esc(s.description ?? '')}</p>
  <div class="shots">${cells}</div>
</section>`;
        })
        .join('\n');
      const open = COLLAPSED_GROUPS.has(group) ? '' : ' open';
      return `<details class="group"${open}>
  <summary>${esc(GROUP_LABELS[group] ?? group)}</summary>
  ${rows}
</details>`;
    })
    .join('\n');

  const options = (items) =>
    items.map((i) => `<option value="${esc(i.id ?? i)}">${esc(i.label ?? i)}</option>`).join('');

  return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Composited contrast — candidate comparison (#2770)</title>
<style>
  :root { color-scheme: dark; --bg:#141714; --fg:#F4F4EF; --dim:#ABB3AB; --line:#323832; }
  * { box-sizing: border-box; }
  body { margin:0; padding:24px; background:var(--bg); color:var(--fg);
         font-family:-apple-system,"Public Sans",system-ui,sans-serif; }
  h1 { font-size:22px; margin:0 0 6px; }
  .lede { color:var(--dim); max-width:62ch; line-height:1.6; margin:0 0 20px; font-size:14px; }
  .controls { position:sticky; top:0; z-index:5; display:flex; flex-wrap:wrap; gap:14px;
              padding:14px; margin-bottom:20px; background:#1F2421;
              border:1px solid var(--line); border-radius:12px; }
  .controls label { display:flex; flex-direction:column; gap:4px; font-size:11px;
                    letter-spacing:.08em; text-transform:uppercase; color:var(--dim); }
  select { background:#141714; color:var(--fg); border:1px solid var(--line);
           border-radius:8px; padding:7px 10px; font-size:14px; }
  .group { border-top:1px solid var(--line); padding:8px 0 16px; }
  .group > summary { cursor:pointer; font-size:13px; letter-spacing:.1em;
                     text-transform:uppercase; color:var(--dim); padding:8px 0; }
  .surface { margin:14px 0 22px; }
  .surface h3 { font-size:15px; margin:0 0 2px; font-family:"Libre Baskerville",Georgia,serif; }
  .desc { color:var(--dim); font-size:12.5px; line-height:1.5; margin:0 0 10px; max-width:70ch; }
  .shots { display:flex; gap:14px; overflow-x:auto; padding-bottom:6px; }
  .shot { margin:0; flex:0 0 auto; width:190px; }
  .shot img { width:100%; border-radius:10px; border:1px solid var(--line); display:block; }
  figcaption { font-family:ui-monospace,Menlo,monospace; font-size:11px; margin-top:6px; }
  figcaption.fail { color:#DB7F63; }
  figcaption.pass { color:#7A9B76; }
  .empty { color:var(--dim); font-size:13px; font-style:italic; }
</style>
</head>
<body>
<h1>Composited contrast — candidate comparison</h1>
<p class="lede">
  Every frame is the real Flutter Web bundle, measured on the pixels it actually
  painted — glass, blur, scrim and backing media included. Captions show the
  <strong>worst</strong> token instance on that surface, not an average: a screen
  is only as legible as its least legible element. Palette and glass ramp are
  independent axes; move one at a time.
</p>

<div class="controls">
  <label>Palette<select id="palette">${options(palettes)}</select></label>
  <label>Glass ramp<select id="ramp">${options(ramps)}</select></label>
  <label>Theme<select id="theme">${options(themes)}</select></label>
  <label>Backdrop<select id="backdrop">${options(backdrops)}</select></label>
</div>

${sections || '<p class="empty">No captures in this sweep.</p>'}

<script>
  const controls = ['palette', 'ramp', 'theme', 'backdrop'].map((id) => document.getElementById(id));
  function apply() {
    const want = Object.fromEntries(controls.map((c) => [c.id, c.value]));
    for (const shot of document.querySelectorAll('.shot')) {
      const show = Object.entries(want).every(([k, v]) => shot.dataset[k] === v);
      shot.hidden = !show;
    }
    // A surface with nothing to show for this combination is noise, not signal.
    for (const surface of document.querySelectorAll('.surface')) {
      const any = [...surface.querySelectorAll('.shot')].some((s) => !s.hidden);
      surface.hidden = !any;
    }
  }
  controls.forEach((c) => c.addEventListener('change', apply));
  apply();
</script>
</body>
</html>
`;
}

if (import.meta.url === `file://${process.argv[1]}`) {
  const sweep = JSON.parse(readFileSync(arg('sweep'), 'utf8'));
  const out = arg('out');
  writeFileSync(out, buildSheet(sweep));
  console.log(`Wrote ${out} (${sweep.captures.length} captures)`);
}
