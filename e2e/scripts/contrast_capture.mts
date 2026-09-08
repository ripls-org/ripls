// contrast_capture.ts — one capture pass of the composited-contrast gate (#2770).
//
//   npx tsx scripts/contrast_capture.ts --slug contrast --pass baseline
//   npx tsx scripts/contrast_capture.ts --slug contrast --pass sentinel
//
// Run once against the normally-built bundle and once against a bundle built
// from `gen_design_tokens.js --sentinel`. Diffing the two passes attributes
// every painted pixel to the token that painted it — the only way to read
// colour off a CanvasKit surface, which has no DOM to inspect.
//
// The two passes run against separate hermetic environments, so seeding must be
// DETERMINISTIC: identical names, identical hero images, no generated content
// that could shift text by a pixel. Ids differ between passes but never render.

import { mkdirSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { execFileSync } from 'node:child_process';

import { openProbe, launchProbeBrowser } from '../lib/probe.js';
import { readEnvState } from '../lib/investigation-state.js';
import { registerUser } from '../lib/seed/users.js';
import { seedSharedGear } from '../lib/seed/gear.js';
import { seedRequest } from '../lib/seed/requests.js';
import { createExperience } from '../lib/seed/experiences.js';
import { createCommunity } from '../lib/seed/communities.js';
import { SURFACES, expandMatrix, perPrMatrix, photoMatrix } from './contrast_surfaces.mjs';

function arg(name: string, fallback?: string): string {
  const i = process.argv.indexOf(`--${name}`);
  if (i >= 0 && process.argv[i + 1]) return process.argv[i + 1];
  if (fallback !== undefined) return fallback;
  throw new Error(`missing required --${name}`);
}

const slug = arg('slug', 'contrast');
const pass = arg('pass');
const scope = arg('scope', 'full');
if (pass !== 'baseline' && pass !== 'sentinel') {
  throw new Error(`--pass must be baseline or sentinel, got "${pass}"`);
}

const env = readEnvState(slug);
if (!env?.baseUrl) {
  throw new Error(`no env for slug "${slug}" — run: npm run env:start -- --slug ${slug}`);
}
const baseUrl = env.baseUrl;

// A sweep renders many candidates, so captures are nested per variant. The
// default keeps single-token-set gate runs at captures/<pass>/ unchanged.
const variant = arg('variant', '');
const outDir = variant
  ? join('investigations', slug, 'captures', variant, pass)
  : join('investigations', slug, 'captures', pass);
mkdirSync(outDir, { recursive: true });

// ---------------------------------------------------------------------------
// Synthetic backdrops
// ---------------------------------------------------------------------------
//
// Generated rather than committed: they are exactly reproducible from these
// ffmpeg invocations, and committing three PNGs to make a point about bounding
// the worst case would be three binaries nobody can diff.

const backdropDir = join('investigations', slug, 'backdrops');
mkdirSync(backdropDir, { recursive: true });

/// Real photographs, for judging rather than gating.
///
/// The synthetic backdrops bound the worst case a photo could produce, which is
/// what the gate needs. They cannot answer "does the app still look good",
/// because a white rectangle under a wash is just a grey rectangle. These are
/// the shipped walkthrough fixtures, chosen to span what the adaptive scrim
/// actually keys on: `photo-bright` needs the most wash (39%), `photo-dark`
/// needs none.
// Paths are relative to e2e/, which is this script's working directory.
const REAL_PHOTOS: Record<string, string> = {
  'photo-bright': 'fixtures/walkthroughs/gear-whiteboard.jpg',
  'photo-mid': 'fixtures/walkthroughs/gear-drill.jpg',
  'photo-dark': 'fixtures/walkthroughs/hero-rooftop.jpg',
};

function makeBackdrop(kind: string): string {
  const real = REAL_PHOTOS[kind];
  if (real) return real;

  const path = join(backdropDir, `${kind}.jpg`);
  const size = '1200x1200';
  const filters: Record<string, string[]> = {
    // Brightest possible media: white-on-glass has almost nothing to sit against.
    white: ['-f', 'lavfi', '-i', `color=c=white:s=${size}`],
    // Darkest: mid-tone brand colours vanish into the scrim.
    black: ['-f', 'lavfi', '-i', `color=c=black:s=${size}`],
    // Busiest: blur averages high-frequency detail to a mid grey that nothing
    // reads against. Seeded so the two passes are byte-identical.
    noise: [
      '-f', 'lavfi',
      '-i', `color=c=gray:s=${size}`,
      '-vf', 'noise=alls=100:allf=t+u:all_seed=12345',
    ],
  };
  execFileSync('ffmpeg', [
    '-y', '-loglevel', 'error',
    ...filters[kind],
    '-frames:v', '1',
    path,
  ]);
  return path;
}

// ---------------------------------------------------------------------------
// Deterministic world
// ---------------------------------------------------------------------------

async function seedWorld() {
  const specSlug = `contrast-${pass}`;
  const owner = await registerUser({ baseUrl, specSlug, name: 'Ada Contrast' });

  const backdrops: Record<string, string> = {};
  for (const kind of ['white', 'black', 'noise', ...Object.keys(REAL_PHOTOS)]) {
    backdrops[kind] = makeBackdrop(kind);
  }

  // One gear per backdrop so the media axis is a seeding parameter rather than
  // something the probe has to manipulate at render time.
  const gearByBackdrop: Record<string, { gearId: string }> = {};
  for (const [kind, path] of Object.entries(backdrops)) {
    gearByBackdrop[kind] = await seedSharedGear({
      baseUrl,
      specSlug,
      accessToken: owner.accessToken,
      name: 'Cordless Drill',
      description: 'A drill for the contrast evaluation set.',
      heroImagePath: path,
    });
  }

  const community = await createCommunity({
    baseUrl,
    specSlug,
    accessToken: owner.accessToken,
    name: 'Contrast Neighbours',
    description: 'Host community for the contrast evaluation set.',
  });

  const experienceByBackdrop: Record<string, { experienceId: string }> = {};
  for (const [kind, path] of Object.entries(backdrops)) {
    experienceByBackdrop[kind] = await createExperience({
      baseUrl,
      specSlug,
      accessToken: owner.accessToken,
      communityId: community.communityId,
      name: 'Neighbourhood Potluck',
      description: 'An event for the contrast evaluation set.',
      heroMedia: { path, contentType: 'image/jpeg', filename: `${kind}.jpg` },
    });
  }

  const request = await seedRequest({
    baseUrl,
    specSlug,
    accessToken: owner.accessToken,
    title: 'Need a ladder',
    description: 'A request for the contrast evaluation set.',
  });

  return { owner, community, gearByBackdrop, experienceByBackdrop, request };
}

// ---------------------------------------------------------------------------
// Capture
// ---------------------------------------------------------------------------

const world = await seedWorld();

// `--surfaces a,b` narrows the run to specific screens. Tuning one material is
// an iterative loop — change a value, look, change it again — and paying for 24
// captures when two screens are in question makes the loop slow enough that you
// stop running it, which is how a value ends up chosen by argument instead of
// by looking.
const only = arg('surfaces', '')
  .split(',')
  .map((s) => s.trim())
  .filter(Boolean);
const pool = only.length
  ? SURFACES.filter((s) => only.includes(s.id))
  : SURFACES;
if (only.length && pool.length !== only.length) {
  const missing = only.filter((id) => !SURFACES.some((s) => s.id === id));
  console.error(`contrast_capture: unknown surface(s): ${missing.join(', ')}`);
  console.error(`available: ${SURFACES.map((s) => s.id).join(', ')}`);
  process.exit(1);
}

const matrix = scope === 'per-pr'
  ? perPrMatrix(pool)
  : scope === 'photos'
    ? photoMatrix(pool)
    : expandMatrix(pool);

console.log(
  `contrast_capture: ${pass} pass, ${matrix.length} captures (${scope} scope` +
    `${only.length ? `, surfaces: ${only.join(', ')}` : ''})`,
);

const manifest: Array<Record<string, unknown>> = [];

// One browser for the whole pass; each capture gets a fresh context so themes
// and storage stay isolated. Launching per capture costs seconds each, which at
// the full matrix's scale is the difference between minutes and hours.
const browser = await launchProbeBrowser();

for (const cell of matrix) {
  const probe = await openProbe({
    slug,
    role: `${pass}-${cell.id}`,
    user: world.owner,
    colorScheme: cell.theme as 'light' | 'dark',
    browser,
  });
  try {
    // Bind the surface to the gear/experience seeded with this backdrop.
    const bound = {
      ...world,
      gear: world.gearByBackdrop[cell.backdrop] ?? world.gearByBackdrop.white,
      experience:
        world.experienceByBackdrop[cell.backdrop] ?? world.experienceByBackdrop.white,
    };
    await cell.surface.open(probe.page, bound, baseUrl);
    const file = join(outDir, `${cell.id}.png`);
    await probe.page.screenshot({ path: file });
    manifest.push({
      id: cell.id,
      surface: cell.surface.id,
      group: cell.surface.group,
      theme: cell.theme,
      backdrop: cell.backdrop,
      file,
    });
    console.log(`  ✓ ${cell.id}`);
  } catch (e) {
    console.error(`  ✗ ${cell.id}: ${(e as Error).message}`);
  } finally {
    await probe.close();
  }
}

await browser.close();

writeFileSync(
  join(
    'investigations',
    slug,
    variant ? `manifest-${variant}-${pass}.json` : `manifest-${pass}.json`,
  ),
  JSON.stringify({ pass, variant, captures: manifest }, null, 2),
);
console.log(`contrast_capture: wrote ${manifest.length} captures to ${outDir}`);
