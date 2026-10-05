// e2e/scripts/build_walkthrough_page.mjs — render a reel's scene clips
// into a scene-by-scene walkthrough page and DEPLOY it onto the walkthrough
// site (#2684).
//
//   node scripts/build_walkthrough_page.mjs <reel> <walkthrough.md> <outDir>
//
// The copy lives in the fixture pack (hand-editable markdown; see
// docs/walkthroughs.md → "Walkthrough copy"). Frontmatter carries
// `reel:` / `slug:` / `title:` / `blurb:`; each `## NN · Heading` section
// maps to the scene clip `scene-NN-*.mp4` in outDir (the NN is mapping-only
// and never rendered), with an optional `actor:` line naming who's on
// camera.
//
// Outputs:
//   <outDir>/walkthrough.html                      standalone artifact
//   website/content/walkthroughs/<slug>/           deployed page + clips +
//                                                  meta.json
//   website/content/walkthroughs/index.html        regenerated index of all
//                                                  deployed walkthroughs
//
// Colors/fonts come from the site's generated design tokens — the fixed
// --light-* set so the page reads light regardless of OS theme —
// because lint:web:colors bans raw colors under website/content.
//
// Invoked by export_walkthrough.sh when a pack's walkthrough.md names
// the reel; safe to run standalone after an export. Unit tests:
// build_walkthrough_page.test.mjs (npm run test:scripts).

import {
  copyFileSync,
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  writeFileSync,
} from 'node:fs';
import { join, basename, dirname, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

// ── Parsing ─────────────────────────────────────────────────────────────

/**
 * Drop one matching pair of surrounding quotes from a frontmatter value.
 *
 * A title containing a colon ("Event: The Wednesday Morning Run") has to be
 * quoted to stay valid YAML, and without this the quotes reached the page as
 * part of the title.
 */
function unquote(value) {
  const m = value.match(/^(["'])([\s\S]*)\1$/);
  return m ? m[2] : value;
}

/**
 * Parse a walkthrough.md into { frontmatter, sections }. Sections carry the
 * two-digit `num` used only to pair them with scene clips.
 */
export function parseWalkthrough(raw) {
  const fmMatch = raw.match(/^---\n([\s\S]*?)\n---\n?/);
  const frontmatter = {};
  if (fmMatch) {
    for (const line of fmMatch[1].split('\n')) {
      const m = line.match(/^(\w+):\s*(.*)$/);
      if (m) frontmatter[m[1]] = unquote(m[2].trim());
    }
  }
  const body = fmMatch ? raw.slice(fmMatch[0].length) : raw;

  const chunks = body.split(/^## /m);
  chunks.shift(); // anything before the first section is ignored
  const sections = chunks.map((chunk) => {
    const lines = chunk.split('\n');
    const heading = lines.shift()?.trim() ?? '';
    const num = heading.match(/^(\d{2})/)?.[1] ?? '';
    const title = heading.replace(/^\d{2}\s*[·—-]?\s*/, '').trim();
    let actor = '';
    while (lines.length && !lines[0].trim()) lines.shift();
    const actorMatch = lines[0]?.match(/^actor:\s*(.+)$/i);
    if (actorMatch) {
      actor = actorMatch[1].trim();
      lines.shift();
    }
    return { num, title, actor, copy: lines.join('\n').trim() };
  });
  return { frontmatter, sections };
}

// ── Markdown-lite → HTML ────────────────────────────────────────────────

const escapeHtml = (s) =>
  s.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;');
// This renderer knows `**bold**`, `*em*` and `` `code` `` — nothing else.
// Anything it doesn't know reaches the page as the literal characters the
// author typed, which is how `_emphasis_` shipped as visible underscores.
// Silence is the bug: refuse to build rather than publish the markup.
const UNSUPPORTED_MARKUP = [
  [/_[^_\n]+_/, 'underscore emphasis — use **bold** or "quotes"'],
  [/(^|\s)#{1,6}\s/, 'inline heading — scenes get their heading from the fixture'],
  [/\[[^\]]+\]\([^)]+\)/, 'link — this page has no link styling'],
];
const inline = (s) => {
  for (const [pattern, why] of UNSUPPORTED_MARKUP) {
    const hit = s.match(pattern);
    if (hit) {
      throw new Error(
        `walkthrough copy uses markup this page can't render: ${hit[0].trim()}\n` +
          `  ${why}\n` +
          `  in: ${s.trim().slice(0, 120)}`,
      );
    }
  }
  return escapeHtml(s)
    .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
    .replace(/\*([^*]+)\*/g, '<em>$1</em>')
    .replace(/`([^`]+)`/g, '<code>$1</code>');
};
const paragraphs = (md) =>
  md
    .split(/\n{2,}/)
    .filter((p) => p.trim())
    .map((p) => `<p>${inline(p.trim().replace(/\n/g, ' '))}</p>`)
    .join('\n      ');

// ── Shared page chrome ──────────────────────────────────────────────────

// The fixed light token set: the semantic --color-* variables flip with the
// viewer's OS theme, and these pages should always read light.
const PAGE_STYLE = `
  * { margin: 0; padding: 0; box-sizing: border-box; }
  body {
    background: var(--light-background);
    color: var(--light-text-primary);
    font-family: var(--font-sans);
  }
  h1, h2, .brand { font-family: var(--font-serif); }
  .hero { max-width: 720px; margin: 0 auto; padding: 4rem 1.5rem 1.5rem; text-align: center; }
  .hero .brand { font-size: 1.2rem; color: var(--light-accent); letter-spacing: 0.04em; }
  .hero .brand a { color: inherit; text-decoration: none; }
  .hero h1 { font-weight: 600; font-size: clamp(1.8rem, 4vw, 2.8rem); line-height: 1.2; margin-top: 1rem; }
  .scene { padding: 3.5rem 1.5rem; }
  .scene.alt { background: var(--light-surface); }
  .scene-inner { max-width: 980px; margin: 0 auto; display: flex; gap: 3rem; align-items: center; flex-wrap: wrap; justify-content: center; }
  .scene.alt .scene-inner { flex-direction: row-reverse; }
  .scene-copy { flex: 1 1 340px; max-width: 480px; }
  .scene-copy h2 { font-weight: 600; font-size: clamp(1.5rem, 3vw, 2.1rem); line-height: 1.2; }
  .scene-copy .actor { font-size: 0.9rem; color: var(--light-text-faint); margin: 0.4rem 0 1rem; }
  .scene-copy p { font-weight: 300; line-height: 1.7; color: var(--light-text-secondary); margin-bottom: 0.9rem; }
  .scene-copy strong { color: var(--light-text-primary); font-weight: 600; }
  .scene-phone { flex: 0 0 auto; }
  /* The bezel is a wrapper so the border never eats into the video's box —
     border-on-the-video shrinks its content box and letterboxes the clip.
     #00000048 = pure black + alpha, the one raw form lint:web:colors allows. */
  .phone-frame {
    width: min(300px, 80vw);
    border-radius: 28px; border: 6px solid var(--dark-background); overflow: hidden;
    box-shadow: 0 24px 60px #00000048; background: var(--dark-background);
  }
  .phone-frame video { width: 100%; height: auto; display: block; }
  .missing { color: var(--light-text-faint); font-style: italic; }
  .index-list { max-width: 760px; margin: 2.5rem auto 0; padding: 0 1.5rem; list-style: none; display: grid; gap: 2rem; text-align: left; }
  .index-row { display: grid; grid-template-columns: 92px 1fr; gap: 1.25rem; align-items: start; }
  .index-text a { font-family: var(--font-serif); font-size: clamp(1.2rem, 2vw, 1.5rem); color: var(--light-text-primary); text-decoration: none; border-bottom: 1px solid var(--light-border); }
  .index-text a:hover { color: var(--light-accent); border-bottom-color: var(--light-accent); }
  .index-list .desc { display: block; font-weight: 300; font-size: 0.95rem; line-height: 1.55; color: var(--light-text-secondary); margin-top: 0.35rem; }
  /* Phone-shaped teaser, matching the scene frames on the reel pages. */
  .index-teaser {
    display: block; width: 92px; aspect-ratio: 780 / 1688; border: 0;
    border-radius: 12px; overflow: hidden; background: var(--dark-background);
    box-shadow: 0 8px 22px #00000026;
  }
  .index-teaser video { width: 100%; height: 100%; object-fit: cover; display: block; }
  @media (max-width: 520px) {
    .index-row { grid-template-columns: 68px 1fr; gap: 1rem; }
    .index-teaser { width: 68px; }
  }
  footer { padding: 3rem 1.5rem 4rem; text-align: center; color: var(--light-text-faint); font-size: 0.85rem; }
`;

// tokenHref: the deployed pages live one or two levels under content/, so
// absolute site paths always work; the extra relative link styles the
// standalone artifact next to its tokens.gen.css copy (404s harmlessly
// on the site).
// canonicalPath: the page's own site path (e.g. "/walkthroughs/brunch/"). These
// pages are public and indexable — the home page links the gallery — so each
// self-canonicals to the non-www host, matching every other marketing page.
function pageHead(title, canonicalPath) {
  return `<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>${escapeHtml(title)}</title>
<link rel="canonical" href="https://ripls.org${canonicalPath}">
<link rel="stylesheet" href="/css/fonts.css">
<link rel="stylesheet" href="/css/gen/tokens.gen.css">
<link rel="stylesheet" href="tokens.gen.css">
<style>${PAGE_STYLE}</style>`;
}

// ── Walkthrough page ────────────────────────────────────────────────────

/**
 * Render the walkthrough page HTML from parsed copy + the scene clip
 * filenames present in the output directory.
 */
export function renderWalkthroughHtml({ frontmatter, sections }, clips, slug = frontmatter.slug) {
  const clipByNum = new Map(clips.map((f) => [f.match(/^scene-(\d{2})/)?.[1], f]));
  const title = frontmatter.title ?? 'Ripls walkthrough';

  const sectionHtml = sections
    .map((s, idx) => {
      const clip = clipByNum.get(s.num) ?? '';
      if (!clip) console.warn(`walkthrough: no clip for section ${s.num} "${s.title}"`);
      return `
  <section class="scene${idx % 2 ? ' alt' : ''}">
    <div class="scene-inner">
      <div class="scene-copy">
        <h2>${inline(s.title)}</h2>
        ${s.actor ? `<p class="actor">${inline(s.actor)}</p>` : ''}
        ${paragraphs(s.copy)}
      </div>
      <div class="scene-phone">
        ${
          clip
            ? `<div class="phone-frame"><video src="${clip}" muted playsinline loop autoplay controls preload="metadata"></video></div>`
            : '<p class="missing">clip missing</p>'
        }
      </div>
    </div>
  </section>`;
    })
    .join('\n');

  return `<!DOCTYPE html>
<html lang="en">
<head>
${pageHead(title.replace(/<[^>]+>/g, ''), `/walkthroughs/${slug}/`)}
</head>
<body>
<header class="hero">
  <p class="brand"><a href="/walkthroughs/">ripls</a></p>
  <h1>${title.includes('<em>') ? title : inline(title)}</h1>
</header>
${sectionHtml}
<footer>Filmed in the live product by the Ripls walkthrough pipeline (#2684) — every clip is the real app.</footer>
</body>
</html>
`;
}

// ── Index page ──────────────────────────────────────────────────────────

/**
 * Render the /walkthroughs/ index from deployed entries
 * ({ slug, title, blurb, teaser? }), sorted by title.
 *
 * `teaser` is a clip filename inside the entry's own directory (the reel's
 * opening scene); when present it renders as a muted, looping phone-shaped
 * thumbnail beside the entry. It is `preload="metadata"` and only starts on
 * hover/tap, so an index of a dozen reels doesn't pull a dozen videos over
 * the wire before the reader has chosen one.
 */
export function renderIndexHtml(entries) {
  const items = [...entries]
    .sort((a, b) => a.title.localeCompare(b.title))
    .map(
      (e) =>
        `    <li class="index-row">
      <a class="index-teaser" href="./${e.slug}/" aria-hidden="true" tabindex="-1">${
          e.teaser
            ? `<video src="./${e.slug}/${e.teaser}" muted loop playsinline preload="metadata"></video>`
            : ''
        }</a>
      <span class="index-text"><a href="./${e.slug}/">${inline(e.title)}</a>${
          e.blurb ? `<span class="desc">${inline(e.blurb)}</span>` : ''
        }</span>
    </li>`,
    )
    .join('\n');

  return `<!DOCTYPE html>
<html lang="en">
<head>
${pageHead('Ripls — Walkthroughs', '/walkthroughs/')}
</head>
<body>
<header class="hero">
  <p class="brand"><a href="/">ripls</a></p>
  <h1>Walkthroughs</h1>
  <ul class="index-list">
${items}
  </ul>
</header>
<footer>Filmed in the live product by the Ripls walkthrough pipeline (#2684) — every clip is the real app.</footer>
<script>
// Teasers hold at their first frame until the reader shows interest, then
// loop. Autoplaying every one would pull the whole index's video down before
// anyone has picked a walkthrough.
for (const row of document.querySelectorAll('.index-row')) {
  const video = row.querySelector('video');
  if (!video) continue;
  const play = () => { video.play().catch(() => {}); };
  const stop = () => { video.pause(); video.currentTime = 0; };
  row.addEventListener('pointerenter', play);
  row.addEventListener('pointerleave', stop);
  row.addEventListener('focusin', play);
  row.addEventListener('focusout', stop);
}
</script>
</body>
</html>
`;
}

// ── Site deployment ─────────────────────────────────────────────────────

/**
 * Which scene a reel teases with on the index when its pack doesn't say.
 *
 * NOT scene 1: every reel opens on the same empty Home ("Good morning, {name}"
 * over the same two prompts), so a row of first-scene stills is five identical
 * thumbnails. Scene 2 is where each story's own subject is on screen — the
 * brunch table, the runners, the supply list, the mower. A pack whose scene 2
 * is still setup overrides this with `teaser:` in its frontmatter.
 */
const DEFAULT_TEASER_SCENE = '02';

/**
 * The clip a deployed reel directory teases with: the scene numbered
 * [scene] (zero-padded, from the pack's `teaser:`), falling back to the
 * lowest-numbered clip present. Undefined when the directory holds no clips.
 */
export function teaserClip(dir, scene = DEFAULT_TEASER_SCENE) {
  const clips = readdirSync(dir)
    .filter((f) => /^scene-\d{2}.*\.mp4$/.test(f))
    .sort();
  const wanted = String(scene).padStart(2, '0');
  return clips.find((f) => f.startsWith(`scene-${wanted}-`)) ?? clips[0];
}

/**
 * Deploy a rendered walkthrough into website/content/walkthroughs/<slug>/
 * (page as index.html, clips, meta.json for the index) and regenerate the
 * walkthroughs index from every deployed meta.json.
 */
export function deployToSite({
  siteWalkthroughsDir,
  slug,
  title,
  blurb,
  html,
  outDir,
  clips,
  teaserScene,
}) {
  const dir = join(siteWalkthroughsDir, slug);
  mkdirSync(dir, { recursive: true });
  writeFileSync(join(dir, 'index.html'), html);
  for (const clip of clips) copyFileSync(join(outDir, clip), join(dir, clip));
  writeFileSync(
    join(dir, 'meta.json'),
    `${JSON.stringify({ slug, title, blurb, teaserScene }, null, 2)}\n`,
  );

  // The teaser FILE is resolved from what is actually on disk rather than
  // recorded, so a renamed scene can't strand the index on a clip that no
  // longer exists; only which scene to use is carried in meta.json.
  const entries = readdirSync(siteWalkthroughsDir, { withFileTypes: true })
    .filter((d) => d.isDirectory())
    .filter((d) => existsSync(join(siteWalkthroughsDir, d.name, 'meta.json')))
    .map((d) => {
      const meta = JSON.parse(
        readFileSync(join(siteWalkthroughsDir, d.name, 'meta.json'), 'utf-8'),
      );
      return {
        ...meta,
        teaser: teaserClip(
          join(siteWalkthroughsDir, d.name),
          meta.teaserScene ?? DEFAULT_TEASER_SCENE,
        ),
      };
    });
  writeFileSync(join(siteWalkthroughsDir, 'index.html'), renderIndexHtml(entries));
  return entries;
}

// ── CLI ─────────────────────────────────────────────────────────────────

function main() {
  const [reel, mdPath, outDir] = process.argv.slice(2);
  if (!reel || !mdPath || !outDir) {
    console.error('usage: build_walkthrough_page.mjs <reel> <walkthrough.md> <outDir>');
    process.exit(1);
  }

  const parsed = parseWalkthrough(readFileSync(mdPath, 'utf-8'));
  const { frontmatter } = parsed;
  const clips = readdirSync(outDir).filter((f) => /^scene-\d{2}.*\.mp4$/.test(f));
  // Slug first: the page self-canonicals to /walkthroughs/<slug>/, so the
  // render needs it.
  const slug = frontmatter.slug ?? reel;
  const html = renderWalkthroughHtml(parsed, clips, slug);
  writeFileSync(join(outDir, 'walkthrough.html'), html);

  // Deploy onto the marketing site so staging/review is part of the render.
  const title = (frontmatter.title ?? `Ripls — ${reel}`).replace(/<[^>]+>/g, '');
  const siteWalkthroughsDir = resolve(
    dirname(fileURLToPath(import.meta.url)),
    '..', '..', 'website', 'content', 'walkthroughs',
  );
  const entries = deployToSite({
    siteWalkthroughsDir,
    slug,
    title,
    blurb: frontmatter.blurb ?? '',
    html,
    outDir,
    clips,
    teaserScene: frontmatter.teaser ?? DEFAULT_TEASER_SCENE,
  });

  console.log(
    `walkthrough: ${join(outDir, 'walkthrough.html')} (${parsed.sections.length} scenes, copy: ${basename(mdPath)})`,
  );
  console.log(
    `walkthrough: deployed to ${join(siteWalkthroughsDir, slug)} (index lists ${entries.length} walkthrough(s))`,
  );
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main();
}
