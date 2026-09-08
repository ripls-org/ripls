// e2e/scripts/fetch_walkthrough_fixtures.mjs
//
// Generator for the committed e2e image fixtures: a hero photo for the event,
// portrait avatars for the seeded host/attendees, and item heroes for the gear
// + request phone-first specs. Idempotent — it SKIPS any fixture already on
// disk (preserving its manifest attribution), so a re-run only fetches missing
// files. Delete a file to refresh just that one.
//
//   UNSPLASH_ACCESS_KEY=$(../scripts/fetch_secret.sh dev unsplash-access-key) \
//     node scripts/fetch_walkthrough_fixtures.mjs
//
// (from the e2e/ directory). Writes JPEGs + a manifest (Unsplash photo id +
// photographer, for attribution) under e2e/fixtures/walkthroughs/. The images are
// committed so the specs are deterministic and need no Unsplash key at test time.
//
// Unsplash API: https://unsplash.com/documentation — random photo endpoint,
// Client-ID auth. Demo apps are rate-limited to 50 req/hour.

import { existsSync, mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const KEY = process.env.UNSPLASH_ACCESS_KEY;
if (!KEY) {
  console.error('Set UNSPLASH_ACCESS_KEY (fetch_secret.sh dev unsplash-access-key).');
  process.exit(1);
}

const OUT_DIR = join(dirname(fileURLToPath(import.meta.url)), '..', 'fixtures', 'walkthroughs');
mkdirSync(OUT_DIR, { recursive: true });

const API = 'https://api.unsplash.com';
const auth = { headers: { Authorization: `Client-ID ${KEY}`, 'Accept-Version': 'v1' } };

/** Fetch `count` random photos for a query, returning the raw photo objects. */
async function randomPhotos(query, orientation, count) {
  const url = `${API}/photos/random?query=${encodeURIComponent(query)}&orientation=${orientation}&count=${count}`;
  const resp = await fetch(url, auth);
  if (!resp.ok) throw new Error(`unsplash ${resp.status}: ${await resp.text()}`);
  const body = await resp.json();
  return Array.isArray(body) ? body : [body];
}

/** Download a photo's raw URL at the given width as a JPEG file. */
async function downloadJpeg(photo, width, destPath) {
  const url = `${photo.urls.raw}&w=${width}&q=80&fm=jpg&fit=crop`;
  const resp = await fetch(url);
  if (!resp.ok) throw new Error(`download ${resp.status} for ${photo.id}`);
  writeFileSync(destPath, Buffer.from(await resp.arrayBuffer()));
}

// Preserve attribution for fixtures we skip on a re-run.
const manifestPath = join(OUT_DIR, 'manifest.json');
const existing = new Map();
if (existsSync(manifestPath)) {
  for (const e of JSON.parse(readFileSync(manifestPath, 'utf-8'))) existing.set(e.file, e);
}

const manifest = [];
function record(file, photo) {
  manifest.push(
    photo
      ? {
          file,
          unsplashId: photo.id,
          photographer: photo.user?.name ?? '',
          profile: photo.user?.links?.html ?? '',
          source: photo.links?.html ?? '',
        }
      : (existing.get(file) ?? { file }),
  );
}

/** Download `file` for `query` unless it already exists (then keep it). */
async function ensureSingle(file, query, orientation, width) {
  const dest = join(OUT_DIR, file);
  if (existsSync(dest)) {
    record(file, null);
    console.log(`${file} ← (kept)`);
    return;
  }
  const [photo] = await randomPhotos(query, orientation, 1);
  await downloadJpeg(photo, width, dest);
  record(file, photo);
  console.log(`${file} ← `, photo.id, photo.user?.name);
}

/**
 * Like ensureSingle but via the search endpoint's top-relevance hit
 * instead of the random endpoint. Use for subjects where random sampling
 * routinely returns look-alikes (e.g. produce: "broccoli" random loves
 * parsley and cabbage); search's relevance ranking is near-deterministic
 * for canonical subjects. `skip` picks a later result when the top hit
 * was reviewed and rejected.
 */
async function ensureSingleSearch(file, query, orientation, width, skip = 0) {
  const dest = join(OUT_DIR, file);
  if (existsSync(dest)) {
    record(file, null);
    console.log(`${file} ← (kept)`);
    return;
  }
  const url = `${API}/search/photos?query=${encodeURIComponent(query)}&orientation=${orientation}&per_page=${skip + 1}`;
  const resp = await fetch(url, auth);
  if (!resp.ok) throw new Error(`unsplash search ${resp.status}: ${await resp.text()}`);
  const body = await resp.json();
  const photo = body.results?.[skip];
  if (!photo) throw new Error(`no search results for ${query}`);
  await downloadJpeg(photo, width, dest);
  record(file, photo);
  console.log(`${file} ← `, photo.id, photo.user?.name);
}

// 1 landscape hero — golden-hour rooftop gathering (event).
await ensureSingle('hero-rooftop.jpg', 'rooftop sunset gathering friends', 'landscape', 1280);

// 8 squarish portraits — host avatar + attendee avatars. Fetched as one batch;
// kept wholesale when already present (delete avatar-1.jpg to refresh the set).
if (existsSync(join(OUT_DIR, 'avatar-1.jpg'))) {
  for (let i = 1; i <= 8; i++) record(`avatar-${i}.jpg`, null);
  console.log('avatars ← (kept)');
} else {
  const portraits = await randomPhotos('portrait headshot smiling person', 'squarish', 8);
  for (let i = 0; i < portraits.length; i++) {
    const file = `avatar-${i + 1}.jpg`;
    await downloadJpeg(portraits[i], 320, join(OUT_DIR, file));
    record(file, portraits[i]);
    console.log(`${file} ← `, portraits[i].id, portraits[i].user?.name);
  }
}

// Item heroes for the gear + request phone-first specs.
await ensureSingle('gear-drill.jpg', 'cordless power drill tool workbench', 'landscape', 1280);
await ensureSingle('request-ladder.jpg', 'aluminium extension ladder', 'landscape', 1280);

// Item heroes for the gear/request authoring + sharing specs and the
// gear-giveaway phone-first loop (#2492 Shared-with + appless engagement).
await ensureSingle('gear-record-player.jpg', 'vintage record player turntable vinyl', 'landscape', 1280);
await ensureSingle('gear-pressure-washer.jpg', 'pressure washer cleaning patio', 'landscape', 1280);
await ensureSingle('request-folding-tables.jpg', 'folding tables event hall setup', 'landscape', 1280);

// A ninth portrait — the original batch skewed male (2 women / 6 men) and
// the kitchen walkthrough (#2687) casts three women.
await ensureSingleSearch('avatar-9.jpg', 'woman portrait headshot smiling', 'squarish', 320);

// Food-item heroes for the shared-food giveaway walkthrough (#2687). The
// filenames are load-bearing: the e2e AI provider keys its deterministic
// listings on them (server/ai/provider_e2e.go e2eGearDetectionForFilename).
await ensureSingleSearch('food-spaghetti.jpg', 'dry spaghetti pasta raw', 'landscape', 1280, 1);
await ensureSingle('food-bread.jpg', 'sourdough bread loaf', 'landscape', 1280);
await ensureSingleSearch('food-broccoli.jpg', 'broccoli', 'landscape', 1280);

// Item hero for the requests walkthrough (#2686) — the request is a lawn mower.
// Search endpoint: random sampling for "lawn mower" loves riding mowers and
// lawns without a mower in frame; search's top relevance is the canonical
// push mower.
await ensureSingleSearch('request-lawn-mower.jpg', 'lawn mower', 'landscape', 1280);

// Theo's own mower for the requests walkthrough (#2724): a DIFFERENT photo
// than the request hero — reusing one file for both roles read on camera as
// June already owning the mower she was asking for. Any filename containing
// "mower" maps to the deterministic Honda listing (provider_e2e.go), so no
// provider change is needed.
await ensureSingleSearch('gear-mower.jpg', 'push lawn mower grass', 'landscape', 1280, 2);

// Fixtures for the classroom-supply-drive requests walkthrough (#2703 — the
// second requests reel: a teacher's back-to-school supply drive, mostly
// giveaways + a whiteboard loan). The hero backs the request's SSR landing (an
// elementary classroom); the item photos are the gear parents give or lend
// (books, storage bins, art supplies, a whiteboard) — each backs a real gear
// listing whose give/lend offer lands on the teacher's request. Search endpoint:
// relevance beats random for these canonical classroom subjects.
await ensureSingleSearch('hero-classroom.jpg', 'empty elementary classroom', 'landscape', 1280);
await ensureSingleSearch('gear-books.jpg', 'stack of childrens picture books', 'landscape', 1280);
await ensureSingleSearch('gear-bins.jpg', 'colorful plastic storage bins', 'landscape', 1280);
await ensureSingleSearch('gear-art-supplies.jpg', 'crayons markers art supplies', 'landscape', 1280);
await ensureSingleSearch('gear-whiteboard.jpg', 'whiteboard', 'landscape', 1280);

// Fixtures for the recurring-group events walkthrough (#2684 — a Wednesday
// morning run club that outlives its first meetup). The hero backs the event
// itself and, once the group is named, the group's own photo; the second shot
// is the day-of chat's "we actually did it" frame. Three more portraits: the
// reel casts a host plus six runners, and the existing nine avatars are
// spoken for by the earlier reels' casts.
await ensureSingleSearch('hero-morning-run.jpg', 'group running morning park path', 'landscape', 1280);
await ensureSingleSearch('run-group-photo.jpg', 'group of runners together outdoors', 'landscape', 1280, 1);
await ensureSingleSearch('run-post-run-coffee.jpg', 'runners coffee after run', 'landscape', 1280);
await ensureSingleSearch('avatar-10.jpg', 'man portrait headshot smiling', 'squarish', 320, 1);
await ensureSingleSearch('avatar-11.jpg', 'woman smiling outdoors portrait', 'squarish', 320, 1);
await ensureSingleSearch('avatar-12.jpg', 'person portrait headshot smiling', 'squarish', 320, 3);

writeFileSync(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);
console.log(`\nwrote manifest.json (${manifest.length} files) to ${OUT_DIR}`);
