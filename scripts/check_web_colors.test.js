'use strict';

const test = require('node:test');
const assert = require('node:assert');

const {
  isPureBlackOrWhiteHex,
  isPureBlackOrWhiteFn,
  scanContent,
} = require('./check_web_colors.js');

function rules(violations) {
  return violations.map((v) => v.rule).sort();
}

test('pure black/white hex exempt, palette hex not', () => {
  assert.ok(isPureBlackOrWhiteHex('fff'));
  assert.ok(isPureBlackOrWhiteHex('FFFFFF'));
  assert.ok(isPureBlackOrWhiteHex('000000'));
  assert.ok(isPureBlackOrWhiteHex('00000080')); // black at alpha
  assert.ok(!isPureBlackOrWhiteHex('3E5A47'));
  assert.ok(!isPureBlackOrWhiteHex('F5ECE1'));
});

test('pure black/white rgb()/rgba() exempt, others not', () => {
  assert.ok(isPureBlackOrWhiteFn('0, 0, 0, 0.62'));
  assert.ok(isPureBlackOrWhiteFn('255, 255, 255, 0.7'));
  assert.ok(!isPureBlackOrWhiteFn('91, 130, 104, 0.18'));
  assert.ok(!isPureBlackOrWhiteFn('253, 250, 247, 0.92'));
});

test('css: raw palette hex flagged, black/white overlays and var() not', () => {
  const css = `
    .a { color: #5B8268; }
    .b { background: rgba(0, 0, 0, 0.5); }
    .c { border-color: var(--color-border); }
    .d { box-shadow: 0 1px 2px rgba(61, 53, 49, 0.06); }
  `;
  const v = scanContent(css, 'website/content/css/x.css', false);
  assert.deepStrictEqual(rules(v), ['no_raw_web_color', 'no_raw_web_color']);
  assert.ok(v.some((x) => x.text === '#5B8268'));
});

test('css: comments are ignored', () => {
  const css = '/* legacy cream was #F5ECE1 */ .a { color: var(--ink); }';
  assert.deepStrictEqual(scanContent(css, 'x.css', false), []);
});

test('html: only style regions are scanned — prose hex is fine', () => {
  const html = `
    <p>Our old cream was #F5ECE1 and we left it behind.</p>
    <div style="color: #E07A5F">bad inline</div>
    <svg><rect fill="#9DBFA8"/></svg>
    <style>.x { color: #44644F; }</style>
  `;
  const v = scanContent(html, 'x.html', true);
  assert.strictEqual(v.length, 3);
  assert.ok(v.every((x) => x.rule === 'no_raw_web_color'));
});

test('font rule: off-token family flagged, token stack and var() allowed', () => {
  const css = `
    .a { font-family: 'Newsreader', Georgia, serif; }
    .b { font-family: var(--font-serif); }
    .c { font-family: 'Libre Baskerville', Baskerville, 'Times New Roman', serif; }
    .d { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; }
  `;
  const v = scanContent(css, 'x.css', false);
  assert.deepStrictEqual(rules(v), ['no_off_token_font']);
  assert.ok(v[0].text.includes('newsreader'));
  assert.ok(v[0].text.includes('georgia'));
});

test('line numbers point into the original file', () => {
  const css = 'a { color: var(--x); }\nb { color: #123456; }\n';
  const v = scanContent(css, 'x.css', false);
  assert.strictEqual(v[0].line, 2);
});

test('repo is currently clean against the allowlist (mirrors the CI gate)', () => {
  const { execFileSync } = require('node:child_process');
  const out = execFileSync('node', [require.resolve('./check_web_colors.js')], {
    encoding: 'utf-8',
  });
  assert.ok(out.includes('check_web_colors: OK'));
});
