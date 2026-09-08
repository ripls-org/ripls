// Tests for the candidate comparison sheet (#2770 Phase 2).
//
// The sheet is a decision artifact — if it silently drops a candidate or
// mislabels a failure, the wrong palette gets chosen and nobody finds out from
// looking at it. These cover the parts that would fail quietly.

import test from 'node:test';
import assert from 'node:assert';

import { buildSheet } from './build_contrast_sheet.mjs';

const sweep = {
  palettes: [
    { id: 'ink-sage', label: 'Ink & Sage' },
    { id: 'dual-primary', label: 'Dual-surface primary' },
  ],
  ramps: [
    { id: 'current', label: 'Current glass' },
    { id: 'opaque', label: 'Heavier scrim' },
  ],
  surfaces: [
    { id: 'glass-date-picker', group: 'glassMedia', description: 'The #2764 surface.' },
    { id: 'request-detail', group: 'flat', description: 'Control group.' },
    { id: 'workshop', group: 'bespoke', description: 'Parchment.' },
  ],
  captures: [
    {
      palette: 'ink-sage', ramp: 'current', surface: 'glass-date-picker',
      theme: 'dark', backdrop: 'white', file: 'a.png',
      worst: { token: 'primary', ratio: 1.64, required: 4.5 },
    },
    {
      palette: 'dual-primary', ramp: 'opaque', surface: 'glass-date-picker',
      theme: 'dark', backdrop: 'white', file: 'b.png',
      worst: { token: 'primary', ratio: 5.2, required: 4.5 },
    },
    {
      palette: 'ink-sage', ramp: 'current', surface: 'request-detail',
      theme: 'light', backdrop: 'none', file: 'c.png',
      worst: { token: 'warning', ratio: 2.1, required: 4.5 },
    },
    {
      palette: 'ink-sage', ramp: 'current', surface: 'workshop',
      theme: 'light', backdrop: 'none', file: 'd.png', worst: null,
    },
  ],
};

test('buildSheet: every capture reaches the page', () => {
  const html = buildSheet(sweep);
  for (const c of sweep.captures) {
    assert.ok(html.includes(`src="${c.file}"`), `missing capture ${c.file}`);
  }
});

test('buildSheet: each shot carries all four axes as filter data', () => {
  // The picker filters on these; a missing attribute makes a shot unreachable
  // in every combination, which looks like "we never rendered it".
  const html = buildSheet(sweep);
  assert.ok(
    html.includes(
      'data-palette="ink-sage" data-ramp="current" data-theme="dark" data-backdrop="white"',
    ),
  );
});

test('buildSheet: palette and ramp are separate pickers, not one combined axis', () => {
  const html = buildSheet(sweep);
  assert.ok(html.includes('<select id="palette">'));
  assert.ok(html.includes('<select id="ramp">'));
  assert.ok(html.includes('>Ink &amp; Sage</option>'));
  assert.ok(html.includes('>Heavier scrim</option>'));
});

test('buildSheet: failures are marked, passes are not', () => {
  const html = buildSheet(sweep);
  assert.ok(html.includes('class="fail">primary 1.64:1 / 4.5</figcaption>'));
  assert.ok(html.includes('class="pass">primary 5.20:1 / 4.5</figcaption>'));
});

test('buildSheet: bespoke and creation groups start collapsed', () => {
  // Per the review that rows per colour set should not overwhelm: glass+media
  // and flat expanded, the rest behind disclosure.
  const html = buildSheet(sweep);
  const glass = html.slice(html.indexOf('Glass + media core') - 200, html.indexOf('Glass + media core'));
  const bespoke = html.slice(html.indexOf('Other bespoke materials') - 200, html.indexOf('Other bespoke materials'));
  assert.ok(glass.includes('<details class="group" open>'), 'glass group should be open');
  assert.ok(bespoke.includes('<details class="group">'), 'bespoke group should be collapsed');
});

test('buildSheet: groups render in evaluation-set order', () => {
  const html = buildSheet(sweep);
  const order = ['Glass + media core', 'Flat surfaces + status states', 'Other bespoke materials'];
  const positions = order.map((label) => html.indexOf(label));
  assert.deepStrictEqual(
    positions,
    [...positions].sort((a, b) => a - b),
    'sections are out of order',
  );
});

test('buildSheet: a capture without a measurement still renders', () => {
  const html = buildSheet(sweep);
  assert.ok(html.includes('no measurement'));
  assert.ok(html.includes('src="d.png"'));
});

test('buildSheet: escapes text that would otherwise break the markup', () => {
  const html = buildSheet({
    ...sweep,
    surfaces: [{ id: 'x', group: 'flat', description: '<script>alert(1)</script> & "quotes"' }],
    captures: [],
  });
  assert.ok(!html.includes('<script>alert(1)</script>'));
  assert.ok(html.includes('&lt;script&gt;'));
});

test('buildSheet: an empty sweep produces a page rather than throwing', () => {
  const html = buildSheet({ palettes: [], ramps: [], surfaces: [], captures: [] });
  assert.ok(html.includes('No captures in this sweep'));
});
