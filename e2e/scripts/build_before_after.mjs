#!/usr/bin/env node
// build_before_after.mjs — side-by-side review of what the fixes changed (#2770).
//
//   node scripts/build_before_after.mjs --before <dir> --after <dir> --out <page.html>
//
// The gate reports ratios; this is for looking. Numbers say the failure rate
// fell 50x, but only eyes can say whether the app still looks like itself —
// and the scrim change in particular trades photo brightness for legibility,
// which is a judgement no measurement makes.
//
// Pairs files by surface id: `<variant>-<surface>-<theme>-<backdrop>.png` on the
// before side, `<surface>-<theme>-<backdrop>.png` on the after side.

import { readdirSync, readFileSync, writeFileSync, mkdirSync, copyFileSync, existsSync } from 'node:fs';
import { join, dirname, basename } from 'node:path';

function arg(name, fallback) {
  const i = process.argv.indexOf(`--${name}`);
  if (i >= 0 && process.argv[i + 1]) return process.argv[i + 1];
  if (fallback !== undefined) return fallback;
  throw new Error(`missing required --${name}`);
}

const beforeDir = arg('before');
const afterDir = arg('after');
const out = arg('out');
const notesPath = arg('notes', '');

const esc = (s) =>
  String(s).replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]);

/**
 * Strips a leading `<palette>__<ramp>-` variant prefix, if present.
 *
 * `[^-]+` after the separator, not `[a-z-]+`: the greedy version swallows most
 * of the surface name too (ramp ids and surface ids both contain hyphens), and
 * silently yields zero matched pairs rather than an error.
 */
const surfaceKey = (f) => basename(f, '.png').replace(/^[^_]+__[^-]+-/, '');

const notes = notesPath && existsSync(notesPath)
  ? JSON.parse(readFileSync(notesPath, 'utf8'))
  : {};

const beforeFiles = new Map(
  readdirSync(beforeDir).filter((f) => f.endsWith('.png')).map((f) => [surfaceKey(f), f]),
);
const afterFiles = new Map(
  readdirSync(afterDir).filter((f) => f.endsWith('.png')).map((f) => [surfaceKey(f), f]),
);

const assetDir = join(dirname(out), basename(out, '.html'));
mkdirSync(assetDir, { recursive: true });

const pairs = [];
for (const [key, beforeFile] of [...beforeFiles].sort()) {
  const afterFile = afterFiles.get(key);
  if (!afterFile) continue;
  copyFileSync(join(beforeDir, beforeFile), join(assetDir, `before-${key}.png`));
  copyFileSync(join(afterDir, afterFile), join(assetDir, `after-${key}.png`));
  pairs.push({ key, dir: basename(assetDir) });
}

const rows = pairs
  .map(
    (p) => `<section class="pair">
  <h3>${esc(p.key)}</h3>
  ${notes[p.key] ? `<p class="note">${esc(notes[p.key])}</p>` : ''}
  <div class="shots">
    <figure><img loading="lazy" src="${p.dir}/before-${esc(p.key)}.png" alt="${esc(p.key)} before"><figcaption>before</figcaption></figure>
    <figure><img loading="lazy" src="${p.dir}/after-${esc(p.key)}.png" alt="${esc(p.key)} after"><figcaption class="after">after</figcaption></figure>
  </div>
</section>`,
  )
  .join('\n');

writeFileSync(
  out,
  `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Contrast fixes — before and after (#2770)</title>
<style>
  :root { color-scheme: dark; --bg:#141714; --fg:#F4F4EF; --dim:#ABB3AB; --line:#323832; }
  * { box-sizing: border-box; }
  body { margin:0; padding:28px; background:var(--bg); color:var(--fg);
         font-family:-apple-system,"Public Sans",system-ui,sans-serif; }
  h1 { font-size:22px; margin:0 0 8px; font-family:"Libre Baskerville",Georgia,serif; }
  .lede { color:var(--dim); max-width:66ch; line-height:1.65; font-size:14px; margin:0 0 26px; }
  .lede strong { color:var(--fg); }
  .pair { border-top:1px solid var(--line); padding:18px 0 6px; }
  .pair h3 { font-size:15px; margin:0 0 4px; font-family:ui-monospace,Menlo,monospace; }
  .note { color:var(--dim); font-size:13px; line-height:1.55; margin:0 0 12px; max-width:70ch; }
  .shots { display:flex; gap:18px; flex-wrap:wrap; }
  figure { margin:0; width:300px; }
  figure img { width:100%; border-radius:12px; border:1px solid var(--line); display:block; }
  figcaption { font-size:12px; letter-spacing:.09em; text-transform:uppercase;
               color:var(--dim); margin-top:7px; }
  figcaption.after { color:#7A9B76; }
</style>
</head>
<body>
<h1>Contrast fixes — before and after</h1>
<p class="lede">
  Same surfaces, same seeded content, same white backdrop photo — the harshest
  case for a translucent material. <strong>Before</strong> is the shipped app;
  <strong>after</strong> is with the media scrim floor, the #2764 fill/foreground
  fix, and media text moved onto the overlay ramp. The measured failure rate went
  from <strong>28.5%</strong> to <strong>0.55%</strong> of token instances, but
  the scrim trades photo brightness for legibility and that is a judgement call
  no number makes. This page is for making it.
</p>
${rows || '<p class="note">No matching pairs found.</p>'}
</body>
</html>
`,
);
console.log(`Wrote ${out} (${pairs.length} pairs)`);
