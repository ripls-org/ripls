// Unit tests for the walkthrough generator (#2684). Run via
// `npm run test:scripts` (node --test), same harness as scripts/*.test.js.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, readFileSync, existsSync } from 'node:fs';
import { join } from 'node:path';
import { tmpdir } from 'node:os';
import {
  parseWalkthrough,
  renderWalkthroughHtml,
  renderIndexHtml,
  deployToSite,
  teaserClip,
} from './build_walkthrough_page.mjs';

const SAMPLE_MD = `---
reel: events
slug: brunch
title: Group Event Walkthrough: Mother's Day Brunch
blurb: A real family's Mother's Day.
---

## 01 · Start with a sentence
actor: Leslie, the host

Leslie types one sentence — **saying it once makes it real**.

## 02 · Join from a text

No actor line on this one, and *emphasis* survives.
`;

test('parseWalkthrough: frontmatter, sections, optional actor', () => {
  const { frontmatter, sections } = parseWalkthrough(SAMPLE_MD);
  assert.equal(frontmatter.reel, 'events');
  assert.equal(frontmatter.slug, 'brunch');
  // Value keeps everything after the FIRST colon.
  assert.equal(frontmatter.title, "Group Event Walkthrough: Mother's Day Brunch");
  assert.equal(sections.length, 2);
  assert.deepEqual(
    sections.map((s) => [s.num, s.title, s.actor]),
    [
      ['01', 'Start with a sentence', 'Leslie, the host'],
      ['02', 'Join from a text', ''],
    ],
  );
  assert.match(sections[0].copy, /types one sentence/);
});

test('renderWalkthroughHtml: clips mapped by NN, no step numbers rendered', () => {
  const parsed = parseWalkthrough(SAMPLE_MD);
  const html = renderWalkthroughHtml(parsed, [
    'scene-01-create.mp4',
    'scene-02-join.mp4',
  ]);
  assert.match(html, /<h2>Start with a sentence<\/h2>/);
  assert.match(html, /<p class="actor">Leslie, the host<\/p>/);
  assert.match(html, /src="scene-01-create\.mp4"/);
  assert.match(html, /src="scene-02-join\.mp4"/);
  assert.match(html, /<strong>saying it once makes it real<\/strong>/);
  // The section numbers are mapping-only — "Step N" must not render.
  assert.doesNotMatch(html, /Step \d/);
  // Hero is just the brand + title (the subtitle/intro era is over).
  assert.match(html, /<h1>Group Event Walkthrough: Mother's Day Brunch<\/h1>/);
  assert.doesNotMatch(html, /class="subtitle"|class="intro"/);
});

test('renderWalkthroughHtml: only design tokens, light palette', () => {
  const html = renderWalkthroughHtml(parseWalkthrough(SAMPLE_MD), []);
  // The fixed light set keeps the page light regardless of OS theme.
  assert.match(html, /var\(--light-background\)/);
  assert.doesNotMatch(html, /var\(--color-background\)/);
  // No raw colors in the stylesheet: the only literal allowed is pure
  // black+alpha (the phone-frame shadow), which lint:web:colors exempts.
  // (Scan the <style> block only — "#2684" in body text is an issue number,
  // not a color.)
  const style = html.match(/<style>([\s\S]*?)<\/style>/)?.[1] ?? '';
  const rawColors = (style.match(/#[0-9a-fA-F]{3,8}\b|rgba?\(/g) ?? []).filter(
    (c) => !/^#0{6}[0-9a-fA-F]{2}$/.test(c),
  );
  assert.deepEqual(rawColors, []);
});

test('parseWalkthrough: a quoted value loses its quotes', () => {
  // A title containing a colon has to be quoted to stay valid YAML; without
  // unquoting, the quotes shipped as part of the rendered title.
  const { frontmatter } = parseWalkthrough(
    ['---', 'title: "Event: The Regular Wednesday Morning Run"', "blurb: 'Single quoted.'", '---', ''].join('\n'),
  );
  assert.equal(frontmatter.title, 'Event: The Regular Wednesday Morning Run');
  assert.equal(frontmatter.blurb, 'Single quoted.');
});

test('parseWalkthrough: an unbalanced quote is left alone', () => {
  const { frontmatter } = parseWalkthrough(
    ['---', 'blurb: She said "yes"', '---', ''].join('\n'),
  );
  assert.equal(frontmatter.blurb, 'She said "yes"');
});

/*
  These pages are public and linked from the home page's "walkthroughs gallery"
  (#2895). They carried `robots: noindex` from when the gallery was internal;
  losing the canonical or regaining the noindex would quietly de-list them.
*/
test('renderWalkthroughHtml: indexable and self-canonical to its slug', () => {
  const html = renderWalkthroughHtml(parseWalkthrough(SAMPLE_MD), []);
  assert.doesNotMatch(html, /noindex/);
  assert.match(html, /<link rel="canonical" href="https:\/\/ripls\.org\/walkthroughs\/brunch\/">/);
});

test('renderWalkthroughHtml: an explicit slug overrides the frontmatter', () => {
  const html = renderWalkthroughHtml(parseWalkthrough(SAMPLE_MD), [], 'other-slug');
  assert.match(html, /href="https:\/\/ripls\.org\/walkthroughs\/other-slug\/"/);
});

test('renderIndexHtml: indexable and self-canonical to the gallery', () => {
  const html = renderIndexHtml([{ slug: 'brunch', title: 'Brunch', blurb: '' }]);
  assert.doesNotMatch(html, /noindex/);
  assert.match(html, /<link rel="canonical" href="https:\/\/ripls\.org\/walkthroughs\/">/);
});

test('renderIndexHtml: sorted entries with slug links', () => {
  const html = renderIndexHtml([
    { slug: 'zebra', title: 'Zebra reel', blurb: '' },
    { slug: 'brunch', title: 'Brunch', blurb: 'A blurb.' },
  ]);
  assert.match(html, /<h1>Walkthroughs<\/h1>/);
  const brunchAt = html.indexOf('./brunch/');
  const zebraAt = html.indexOf('./zebra/');
  assert.ok(brunchAt !== -1 && zebraAt !== -1 && brunchAt < zebraAt, 'sorted by title');
  assert.match(html, /<span class="desc">A blurb\.<\/span>/);
});

test('renderIndexHtml: a teaser renders as a hold-at-first-frame video', () => {
  const html = renderIndexHtml([
    { slug: 'brunch', title: 'Brunch', blurb: '', teaser: 'scene-01-create.mp4' },
  ]);
  assert.match(html, /<video src="\.\/brunch\/scene-01-create\.mp4"/);
  assert.match(html, /preload="metadata"/, 'must not pull the clip on load');
  assert.doesNotMatch(html, /autoplay/, 'teasers start on interest, not on load');
});

test('renderIndexHtml: an entry with no teaser still renders', () => {
  const html = renderIndexHtml([{ slug: 'brunch', title: 'Brunch', blurb: '' }]);
  assert.match(html, /\.\/brunch\//);
  assert.doesNotMatch(html, /<video/);
});

test('deployToSite: copies page + clips, writes meta, rebuilds index', () => {
  const tmp = mkdtempSync(join(tmpdir(), 'walkthrough-test-'));
  const outDir = join(tmp, 'out');
  const siteDir = join(tmp, 'site', 'walkthroughs');
  mkdirSync(outDir, { recursive: true });
  writeFileSync(join(outDir, 'scene-01-create.mp4'), 'fake-video');

  // A pre-existing deployed walkthrough must survive and stay indexed.
  mkdirSync(join(siteDir, 'older'), { recursive: true });
  writeFileSync(
    join(siteDir, 'older', 'meta.json'),
    JSON.stringify({ slug: 'older', title: 'Older one', blurb: '' }),
  );

  const entries = deployToSite({
    siteWalkthroughsDir: siteDir,
    slug: 'brunch',
    title: 'Brunch',
    blurb: 'A blurb.',
    html: '<html>page</html>',
    outDir,
    clips: ['scene-01-create.mp4'],
  });

  assert.equal(readFileSync(join(siteDir, 'brunch', 'index.html'), 'utf-8'), '<html>page</html>');
  assert.ok(existsSync(join(siteDir, 'brunch', 'scene-01-create.mp4')));
  assert.equal(entries.length, 2);
  const index = readFileSync(join(siteDir, 'index.html'), 'utf-8');
  assert.match(index, /\.\/brunch\//);
  assert.match(index, /\.\/older\//);
  // The teaser comes from what is on disk, so the just-deployed reel gets one
  // and the pre-existing entry (no clips) is simply left without.
  assert.equal(entries.find((e) => e.slug === 'brunch').teaser, 'scene-01-create.mp4');
  assert.equal(entries.find((e) => e.slug === 'older').teaser, undefined);
  assert.match(index, /<video src="\.\/brunch\/scene-01-create\.mp4"/);
});

test('deployToSite: the pack\'s teaser scene is recorded and honoured', () => {
  const tmp = mkdtempSync(join(tmpdir(), 'walkthrough-teaser-'));
  const outDir = join(tmp, 'out');
  const siteDir = join(tmp, 'site', 'walkthroughs');
  mkdirSync(outDir, { recursive: true });
  const clips = ['scene-01-create.mp4', 'scene-02-share.mp4', 'scene-03-claim.mp4'];
  for (const c of clips) writeFileSync(join(outDir, c), 'fake-video');

  deployToSite({
    siteWalkthroughsDir: siteDir,
    slug: 'kitchen',
    title: 'Kitchen',
    blurb: '',
    html: '<html>page</html>',
    outDir,
    clips,
    teaserScene: '03',
  });

  const meta = JSON.parse(readFileSync(join(siteDir, 'kitchen', 'meta.json'), 'utf-8'));
  assert.equal(meta.teaserScene, '03', 'recorded so a later deploy of a SIBLING reel keeps it');
  const index = readFileSync(join(siteDir, 'index.html'), 'utf-8');
  assert.match(index, /<video src="\.\/kitchen\/scene-03-claim\.mp4"/);
});

test('teaserClip: scene 2 by default, not the identical-looking scene 1', () => {
  const tmp = mkdtempSync(join(tmpdir(), 'walkthrough-clip-'));
  for (const f of [
    'scene-03-wrap.mp4',
    'scene-01-create.mp4',
    'scene-02-join.mp4',
    'scene-01-create.webm', // masters live alongside; not a teaser
    'index.html',
  ]) {
    writeFileSync(join(tmp, f), 'x');
  }
  assert.equal(teaserClip(tmp), 'scene-02-join.mp4');
  assert.equal(teaserClip(tmp, '03'), 'scene-03-wrap.mp4');
  assert.equal(teaserClip(tmp, 3), 'scene-03-wrap.mp4', 'unpadded number works');
});

test('teaserClip: falls back to the lowest scene when the ask is absent', () => {
  const tmp = mkdtempSync(join(tmpdir(), 'walkthrough-clip-one-'));
  writeFileSync(join(tmp, 'scene-01-only.mp4'), 'x');
  // A single-scene reel has no scene 2 to prefer.
  assert.equal(teaserClip(tmp), 'scene-01-only.mp4');
  assert.equal(teaserClip(tmp, '07'), 'scene-01-only.mp4');
});

test('teaserClip: no clips at all', () => {
  const tmp = mkdtempSync(join(tmpdir(), 'walkthrough-clip-none-'));
  writeFileSync(join(tmp, 'index.html'), 'x');
  assert.equal(teaserClip(tmp), undefined);
});
