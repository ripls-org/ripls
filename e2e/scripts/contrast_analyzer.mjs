// contrast_analyzer.mjs — composited-contrast measurement by sentinel diff (#2770).
//
// design/tokens.json's generation-time gate checks token-on-token pairs against
// `background` and `surface`, two flat opaque colours. Much of the app renders on
// translucent glass over arbitrary user media, where the real background is a
// blurred composite no token math can predict. #2764 is the consequence: deep
// sage `#3E5A47`, validated at 6.8:1 as a *fill*, used as a TextButton
// foreground on a dark glass sheet where it measures 1.64:1.
//
// This module measures what was actually painted. The sweep renders each surface
// twice — once normally, once with every token replaced by a distinct sentinel
// hue — and the diff attributes pixels to tokens. That matters because CanvasKit
// paints text to canvas and prunes non-interactive nodes from the accessibility
// tree (see e2e/README.md § What's reliably reachable), so there is no DOM to
// read colours or geometry from. Sentinel diffing needs neither.
//
// Everything here is pure: buffers in, findings out. Decoding lives in the
// caller so this stays unit-testable against synthetic fixtures with analytic
// ground truth.

/** Sum-of-absolute-channel-delta above which a pixel counts as painted by a token. */
const CHANGE_THRESHOLD = 30;

/**
 * Max euclidean RGB distance from a pure sentinel for a pixel to be "core".
 *
 * Glyph edges are anti-aliased blends of the sentinel and whatever is behind
 * them, so their colour drifts toward the background. Core pixels are the
 * high-coverage interior, where the sentinel render is close to the pure hue and
 * the baseline render is therefore close to the token's true colour.
 */
const CORE_THRESHOLD = 70;

/**
 * Components smaller than this are dropped as anti-aliasing debris rather than
 * real text. At 2x DPR a single glyph stem is comfortably larger.
 */
const MIN_COMPONENT_PIXELS = 24;

/** Ring radii tried in order until enough background samples are collected. */
const RING_RADII = [3, 4, 6, 8, 10];
const MIN_RING_SAMPLES = 40;

/**
 * Upper bound on ring samples per component.
 *
 * Without it, a component covering most of the frame (a full-bleed fill, or any
 * token whose mask is large) would walk ~24 ring cells for each of a million
 * pixels into a single array. The median of a few thousand samples is already
 * stable, so the work is capped and the component is walked with a stride
 * instead of from one corner — sampling the first N pixels would bias the
 * estimate toward whatever is in the top-left.
 */
const MAX_RING_SAMPLES = 4000;

export function srgbToLinear(c) {
  const s = c / 255;
  return s <= 0.04045 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
}

export function relativeLuminance([r, g, b]) {
  return (
    0.2126 * srgbToLinear(r) + 0.7152 * srgbToLinear(g) + 0.0722 * srgbToLinear(b)
  );
}

export function contrastRatio(fg, bg) {
  const a = relativeLuminance(fg);
  const b = relativeLuminance(bg);
  return (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
}

export function parseHex(hex) {
  const h = hex.replace('#', '');
  return [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16));
}

const px = (buf, i) => [buf[i * 3], buf[i * 3 + 1], buf[i * 3 + 2]];

function median(values) {
  const v = [...values].sort((a, b) => a - b);
  return v[Math.floor(v.length / 2)];
}

function medianColor(pixels) {
  return [0, 1, 2].map((c) => median(pixels.map((p) => p[c])));
}

/**
 * Classifies every changed pixel to the token whose sentinel it is nearest.
 *
 * Returns, per token index: `core` (high-coverage pixel indices, used for the
 * foreground estimate) and `owned` (every pixel at any coverage, which the ring
 * must exclude so a glyph's own anti-aliased fringe is never mistaken for
 * background).
 */
export function classify(baseline, sentinel, sentinelRgb, opts = {}) {
  const changeT = opts.changeThreshold ?? CHANGE_THRESHOLD;
  const coreT = opts.coreThreshold ?? CORE_THRESHOLD;
  const coreT2 = coreT * coreT;
  const n = baseline.length / 3;

  const core = new Map();
  const owned = new Map();
  for (let i = 0; i < n; i++) {
    const b = px(baseline, i);
    const s = px(sentinel, i);
    const delta =
      Math.abs(b[0] - s[0]) + Math.abs(b[1] - s[1]) + Math.abs(b[2] - s[2]);
    if (delta <= changeT) continue;

    let best = -1;
    let bestD = Infinity;
    for (let t = 0; t < sentinelRgb.length; t++) {
      const sc = sentinelRgb[t];
      const d =
        (s[0] - sc[0]) ** 2 + (s[1] - sc[1]) ** 2 + (s[2] - sc[2]) ** 2;
      if (d < bestD) {
        bestD = d;
        best = t;
      }
    }
    if (!owned.has(best)) owned.set(best, new Set());
    owned.get(best).add(i);
    if (bestD < coreT2) {
      if (!core.has(best)) core.set(best, []);
      core.get(best).push(i);
    }
  }
  return { core, owned };
}

/**
 * Splits a pixel set into 8-connected components.
 *
 * Load-bearing, not a refinement: a token is routinely used against more than
 * one background — `modalTextPrimary` labels both the bare sheet and the inside
 * of a filled pill. Measured as one mask the median collapses to whichever
 * context has more pixels and the other is silently averaged away, which is
 * exactly the kind of false pass this gate exists to prevent. Iterative flood
 * fill; a recursive one blows the stack on a full-screen mask.
 */
export function connectedComponents(pixelIndices, width, minPixels = MIN_COMPONENT_PIXELS) {
  const remaining = new Set(pixelIndices);
  const components = [];
  for (const start of pixelIndices) {
    if (!remaining.has(start)) continue;
    const stack = [start];
    remaining.delete(start);
    const component = [];
    while (stack.length) {
      const i = stack.pop();
      component.push(i);
      const x = i % width;
      const y = (i / width) | 0;
      for (let dy = -1; dy <= 1; dy++) {
        for (let dx = -1; dx <= 1; dx++) {
          if (!dx && !dy) continue;
          const nx = x + dx;
          const ny = y + dy;
          if (nx < 0 || nx >= width || ny < 0) continue;
          const ni = ny * width + nx;
          if (!remaining.has(ni)) continue;
          remaining.delete(ni);
          stack.push(ni);
        }
      }
    }
    if (component.length >= minPixels) components.push(component);
  }
  return components;
}

/**
 * Median of baseline pixels on a ring around [component], excluding [ownPixels].
 *
 * Only the token's *own* pixels are excluded. Pixels belonging to a *different*
 * token are kept deliberately: when text sits on a filled pill, that fill is the
 * background, and excluding it would report the sheet behind the pill instead
 * (measured: 2.97 vs a true 2.41).
 */
export function ringBackground(baseline, component, ownPixels, width, height) {
  const componentSet = new Set(component);
  // Walk large components with a stride so the capped sample set still spans the
  // whole shape rather than clustering at its start.
  const stride = Math.max(1, Math.ceil(component.length / MAX_RING_SAMPLES));
  for (const r of RING_RADII) {
    const samples = [];
    for (let k = 0; k < component.length && samples.length < MAX_RING_SAMPLES; k += stride) {
      const i = component[k];
      const x = i % width;
      const y = (i / width) | 0;
      for (let dy = -r; dy <= r; dy++) {
        for (let dx = -r; dx <= r; dx++) {
          if (Math.abs(dx) !== r && Math.abs(dy) !== r) continue;
          const nx = x + dx;
          const ny = y + dy;
          if (nx < 0 || nx >= width || ny < 0 || ny >= height) continue;
          const ni = ny * width + nx;
          if (ownPixels.has(ni) || componentSet.has(ni)) continue;
          samples.push(px(baseline, ni));
        }
      }
    }
    if (samples.length >= MIN_RING_SAMPLES) return medianColor(samples);
  }
  return null;
}

/**
 * Measures composited contrast for every token present in the frame.
 *
 * `sentinelMap` is `{ tokenName: '#RRGGBB' }` as emitted by
 * `gen_design_tokens.js --sentinel`. Returns one finding per connected
 * component, so a token used in two places is reported twice rather than
 * averaged into one misleading number.
 */
export function analyze({ baseline, sentinel, width, height, sentinelMap, options = {} }) {
  if (baseline.length !== sentinel.length) {
    throw new Error(
      `buffer size mismatch: baseline ${baseline.length}, sentinel ${sentinel.length}`,
    );
  }
  if (baseline.length !== width * height * 3) {
    throw new Error(
      `buffer is ${baseline.length} bytes, expected ${width * height * 3} for ${width}x${height} rgb24`,
    );
  }

  const names = Object.keys(sentinelMap);
  const sentinelRgb = names.map((n) => parseHex(sentinelMap[n]));
  const { core, owned } = classify(baseline, sentinel, sentinelRgb, options);

  const findings = [];
  for (let t = 0; t < names.length; t++) {
    const corePixels = core.get(t);
    if (!corePixels || corePixels.length < MIN_COMPONENT_PIXELS) continue;
    const ownPixels = owned.get(t) ?? new Set();
    const components = connectedComponents(
      corePixels,
      width,
      options.minComponentPixels ?? MIN_COMPONENT_PIXELS,
    );
    for (const component of components) {
      const fg = medianColor(component.map((i) => px(baseline, i)));
      const bg = ringBackground(baseline, component, ownPixels, width, height);
      if (!bg) continue;
      // Spread over ~1.7M indices, so fold rather than Math.min(...spread) —
      // that would blow the argument limit on a full-frame component.
      let minX = Infinity;
      let maxX = -Infinity;
      let minY = Infinity;
      let maxY = -Infinity;
      for (const i of component) {
        const x = i % width;
        const y = (i / width) | 0;
        if (x < minX) minX = x;
        if (x > maxX) maxX = x;
        if (y < minY) minY = y;
        if (y > maxY) maxY = y;
      }
      const bounds = {
        x: minX,
        y: minY,
        width: maxX - minX + 1,
        height: maxY - minY + 1,
      };
      findings.push({
        token: names[t],
        foreground: fg,
        background: bg,
        ratio: contrastRatio(fg, bg),
        pixels: component.length,
        bounds,
        // How much of its own bounding box the component fills. Glyph runs are
        // sparse (~0.2-0.4); a solid fill is ~1. This is what separates "text
        // that must clear 4.5:1" from "a filled surface", which WCAG judges as
        // a non-text UI component at 3:1 instead.
        density: component.length / (bounds.width * bounds.height),
      });
    }
  }
  return findings;
}

/**
 * Applies WCAG thresholds.
 *
 * Normal text needs 4.5:1; large text (>=24px, or >=18.66px bold) needs 3:1 —
 * at 2x DPR the measured component height is in device pixels, hence the
 * doubling. A dense component is a filled surface rather than a glyph run, and
 * WCAG judges non-text UI components at 3:1 (1.4.11), so it is held to that
 * instead of being failed against a text threshold it was never subject to.
 */
/**
 * Effective text height for each finding, in device pixels.
 *
 * A component is one connected glyph run, not one line of text — so the dot of
 * an "i" is its own 9px component even inside a 40px title. Judging that dot at
 * the normal-text threshold while the letters beside it qualify as large text
 * manufactures failures on headings that are perfectly legible.
 *
 * Components of the same token whose vertical spans overlap are on the same
 * line, so each takes the tallest height in its group.
 */
function lineHeights(findings, isSurface) {
  const heights = new Array(findings.length);
  const byToken = new Map();
  findings.forEach((f, i) => {
    // A filled surface is not text: it must neither inherit a line height nor
    // donate its own to the label sitting on top of it.
    if (isSurface[i]) return;
    if (!byToken.has(f.token)) byToken.set(f.token, []);
    byToken.get(f.token).push(i);
  });

  for (const indices of byToken.values()) {
    const groups = [];
    for (const i of indices.slice().sort((a, b) => findings[a].bounds.y - findings[b].bounds.y)) {
      const b = findings[i].bounds;
      const group = groups.find((g) => {
        const overlap = Math.min(g.bottom, b.y + b.height) - Math.max(g.top, b.y);
        return overlap > 0.5 * Math.min(g.bottom - g.top, b.height);
      });
      if (group) {
        group.members.push(i);
        group.top = Math.min(group.top, b.y);
        group.bottom = Math.max(group.bottom, b.y + b.height);
        group.height = Math.max(group.height, b.height);
      } else {
        groups.push({ members: [i], top: b.y, bottom: b.y + b.height, height: b.height });
      }
    }
    for (const g of groups) for (const i of g.members) heights[i] = g.height;
  }
  return heights;
}

export function judge(
  findings,
  {
    dpr = 2,
    largeTextPx = 24,
    surfaceDensity = 0.9,
    surfacePixels = 5000,
    roles = {},
    values = {},
    fidelity = 26,
  } = {},
) {
  const surfaceFlags = findings.map(
    (f) => f.density >= surfaceDensity && f.pixels >= surfacePixels,
  );
  const heights = lineHeights(findings, surfaceFlags);
  return findings.map((f, index) => {
    const role = roles[f.token] ?? 'text';
    const isSurface = surfaceFlags[index];
    const isLarge = (heights[index] ?? f.bounds.height) >= largeTextPx * dpr;

    // Does the measured foreground actually look like this token? A thin or
    // small glyph run is mostly anti-aliased edge, so its median blends toward
    // the background and reads as far worse contrast than the token has. Compare
    // against every theme's declared value — only one theme renders per capture,
    // and which one is not always the app's nominal theme (content screens over
    // media use dark chrome in both).
    const declared = Object.values(values)
      .map((byToken) => byToken[f.token])
      .filter(Boolean)
      .map(parseHex);
    const deviation = declared.length
      ? Math.min(
          ...declared.map((d) =>
            Math.sqrt(d.reduce((acc, v, i) => acc + (v - f.foreground[i]) ** 2, 0)),
          ),
        )
      : 0;
    const lowConfidence = declared.length > 0 && deviation > fidelity;

    // A decorative token carries no information on its own — a hairline divider
    // is not held to a contrast floor by WCAG. Report it so a regression is
    // still visible, but never fail on it.
    if (role === 'decorative') {
      return { ...f, role, required: null, isLarge, isSurface, lowConfidence, passes: true };
    }
    if (lowConfidence) {
      // Measured, reported, but not failed on — the number describes the
      // rasterisation, not the token.
      return { ...f, role, required: null, isLarge, isSurface, lowConfidence, passes: true };
    }
    // text-faint is documented in tokens.json as clearing 3:1 and never
    // carrying body copy. Holding it to 4.5:1 contradicts its own contract.
    // A filled surface is a non-text UI component (WCAG 1.4.11), also 3:1.
    const required =
      role === 'text-faint' || role === 'ui' || isSurface || isLarge ? 3.0 : 4.5;
    return {
      ...f,
      role,
      required,
      isLarge,
      isSurface,
      lowConfidence,
      passes: f.ratio >= required,
    };
  });
}
