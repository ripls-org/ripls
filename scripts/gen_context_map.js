#!/usr/bin/env node
/**
 * gen_context_map.js — generate docs/llms.txt from per-doc `context:` frontmatter.
 *
 * Every durable guidance doc under docs/ carries a `context:` YAML frontmatter
 * block (schema + rationale in docs/context_map.md). This script reads those
 * blocks and emits docs/llms.txt — an llms.txt-format index that the
 * ripls-issue and ripls-audit skills (and the CLAUDE.md "Finding Documentation
 * Context" protocol) read to load only the docs salient to a given task.
 *
 * Modes:
 *   node scripts/gen_context_map.js            Write docs/llms.txt.
 *   node scripts/gen_context_map.js --check     Verify docs/llms.txt is current
 *                                               and all blocks are valid; used
 *                                               as the CI gate (lint:context-map).
 *
 * The gate fails on: an invalid/malformed `context:` block, an unknown `lens`
 * value, an `alwaysApply` anchor with no `description`, or a stale docs/llms.txt.
 * Docs with no block yet are reported as "untagged" but do NOT fail the build
 * while ENFORCE_FULL_COVERAGE is false (the backfill is incremental); flip that
 * flag once every durable doc is tagged to lock coverage in.
 *
 * Excluded subtrees (outputs / ephemera, never indexed): see EXCLUDED_PREFIXES.
 */

'use strict';

const fs = require('fs');
const path = require('path');
const yaml = require('js-yaml');

const ROOT = path.resolve(__dirname, '..');
const DOCS_DIR = path.join(ROOT, 'docs');
const OUTPUT_REL = 'docs/llms.txt';
const OUTPUT_ABS = path.join(ROOT, OUTPUT_REL);

// Backfill is complete: every durable doc carries a `context:` block, so a doc
// with no block now fails the gate instead of merely being reported.
const ENFORCE_FULL_COVERAGE = true;

// Controlled vocabulary for `lens` (the gate rejects anything else, so it can't
// sprawl). Drives anchor routing + map grouping. Keep in sync with
// docs/context_map.md.
const LENS_VOCAB = [
  // Structural — perspective, side, and topic.
  'architecture', 'conventions', 'product', 'infra', 'client', 'server',
  'domain', 'workflow',
  // Technical review dimensions — each docs/reviews/<x>_review_prompt.md is the
  // anchor for its lens; loaders follow that prompt's "Areas to Review" section.
  'accessibility', 'api_design', 'code_quality', 'dependency', 'error_handling',
  'observability', 'performance', 'security', 'tech_debt', 'testing',
];

// Skills that may consider a doc. `skills` defaults to all of these when unset.
const SKILL_VOCAB = ['issue', 'audit', 'triage', 'reviews'];

// Allowed keys inside the `context:` block (typos like `lenses:` must fail).
const ALLOWED_KEYS = new Set([
  'description', 'globs', 'triggers', 'lens', 'skills', 'alwaysApply', 'domain',
]);

// Subtrees that are outputs/ephemera and are never indexed (relative to repo
// root, posix separators, trailing slash = whole subtree).
const EXCLUDED_PREFIXES = [
  'docs/ai/',
  'docs/issues/',
  'docs/release/notes/',
  'docs/crashlytics/',
  'docs/reviews/reports/',
  'docs/evals/',
  // Instance-specific guidance that stays in this repo when the product code is
  // published (#2953): business strategy, runbooks for our own hosts and
  // accounts, and store-account state. Excluding it here is what lets
  // docs/llms.txt stay byte-identical between this repo and the public one —
  // the export is a pure path filter, so an index that differed would have to
  // be regenerated during export, and a regenerated file is untested
  // divergence. See docs/issues/2953-open-source-release.md → Correction 3.
  'docs/private/',
];

// Individual files that are exempt (the spec doc about this very system).
const EXCLUDED_FILES = new Set([
  'docs/context_map.md',
]);

// Map a doc's first path segment under docs/ to a display domain. Falls through
// to the segment name, then `domain`/`lens` from the block, then 'misc'.
const DIR_DOMAIN = {
  client: 'client',
  server: 'server',
  users: 'product',
  workflows: 'workflows',
  testing: 'testing',
  impact_metrics: 'impact',
  concepts: 'design',
  design: 'design',
  release: 'release',
  reviews: 'reviews',
};

// Preferred display order of domains in the generated index; unknown domains
// sort alphabetically after these.
const DOMAIN_ORDER = [
  'conventions', 'architecture', 'client', 'server', 'infra', 'security',
  'observability', 'testing', 'workflows', 'product', 'impact', 'domain',
  'design', 'release', 'reviews', 'misc',
];

// ---------------------------------------------------------------------------
// File collection
// ---------------------------------------------------------------------------

function relPosix(absPath) {
  return path.relative(ROOT, absPath).split(path.sep).join('/');
}

function isExcluded(rel) {
  if (EXCLUDED_FILES.has(rel)) return true;
  return EXCLUDED_PREFIXES.some((p) => rel.startsWith(p));
}

function walk(dir, out = []) {
  if (!fs.existsSync(dir)) return out;
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      walk(full, out);
    } else if (entry.isFile() && entry.name.endsWith('.md')) {
      out.push(full);
    }
  }
  return out;
}

function collectDocs() {
  return walk(DOCS_DIR)
    .map((abs) => relPosix(abs))
    .filter((rel) => !isExcluded(rel))
    .sort();
}

// ---------------------------------------------------------------------------
// Frontmatter parsing
// ---------------------------------------------------------------------------

/**
 * splitFrontmatter returns { fm, body } where fm is the raw YAML text of a
 * leading `--- ... ---` block (or null if absent) and body is everything after.
 */
function splitFrontmatter(src) {
  const lines = src.split('\n');
  if (lines[0].trim() !== '---') return { fm: null, body: src };
  for (let i = 1; i < lines.length; i++) {
    if (lines[i].trim() === '---') {
      return { fm: lines.slice(1, i).join('\n'), body: lines.slice(i + 1).join('\n') };
    }
  }
  return { fm: null, body: src }; // unterminated frontmatter → treat as none
}

/**
 * parseContext extracts the `context` object from a doc's frontmatter. Returns
 * { context, parseError }: context is null when there is no block; parseError is
 * a string when the YAML itself is malformed.
 */
function parseContext(src) {
  const { fm } = splitFrontmatter(src);
  if (fm === null) return { context: null, parseError: null };
  let doc;
  try {
    doc = yaml.load(fm);
  } catch (e) {
    return { context: null, parseError: e.message.split('\n')[0] };
  }
  if (!doc || typeof doc !== 'object' || !('context' in doc)) {
    return { context: null, parseError: null };
  }
  return { context: doc.context, parseError: null };
}

/** firstParagraph returns the first non-empty, non-heading body line (for the
 *  description fallback), collapsed to one line and truncated. */
function firstParagraph(body) {
  for (const raw of body.split('\n')) {
    const line = raw.trim();
    if (!line || line.startsWith('#') || line.startsWith('---')) continue;
    return clampDescription(line);
  }
  return '';
}

function clampDescription(s) {
  const oneLine = String(s).replace(/\s+/g, ' ').trim();
  return oneLine.length > 260 ? `${oneLine.slice(0, 257)}...` : oneLine;
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

function isStringArray(v) {
  return Array.isArray(v) && v.every((x) => typeof x === 'string');
}

/** validate returns an array of human-readable error strings for one block. */
function validate(rel, ctx) {
  const errors = [];
  const at = (msg) => errors.push(`${rel}: ${msg}`);

  if (!ctx || typeof ctx !== 'object' || Array.isArray(ctx)) {
    at('`context:` must be a mapping of fields');
    return errors;
  }

  for (const key of Object.keys(ctx)) {
    if (!ALLOWED_KEYS.has(key)) {
      at(`unknown field \`${key}\` (allowed: ${[...ALLOWED_KEYS].join(', ')})`);
    }
  }

  // lens — required, non-empty, controlled vocabulary.
  if (!('lens' in ctx)) {
    at('missing required field `lens`');
  } else if (!isStringArray(ctx.lens) || ctx.lens.length === 0) {
    at('`lens` must be a non-empty list of strings');
  } else {
    for (const l of ctx.lens) {
      if (!LENS_VOCAB.includes(l)) {
        at(`invalid lens \`${l}\` (allowed: ${LENS_VOCAB.join(', ')})`);
      }
    }
  }

  // skills — optional; if present, controlled vocabulary.
  if ('skills' in ctx) {
    if (!isStringArray(ctx.skills) || ctx.skills.length === 0) {
      at('`skills` must be a non-empty list of strings when present');
    } else {
      for (const s of ctx.skills) {
        if (!SKILL_VOCAB.includes(s)) {
          at(`invalid skill \`${s}\` (allowed: ${SKILL_VOCAB.join(', ')})`);
        }
      }
    }
  }

  if ('globs' in ctx && !isStringArray(ctx.globs)) {
    at('`globs` must be a list of strings');
  }
  if ('triggers' in ctx && !isStringArray(ctx.triggers)) {
    at('`triggers` must be a list of strings');
  }
  if ('alwaysApply' in ctx && typeof ctx.alwaysApply !== 'boolean') {
    at('`alwaysApply` must be a boolean');
  }
  if ('domain' in ctx && typeof ctx.domain !== 'string') {
    at('`domain` must be a string');
  }

  // Anchors are load-bearing — they must carry an explicit description.
  if (ctx.alwaysApply === true) {
    const d = ctx.description;
    if (typeof d !== 'string' || !d.trim()) {
      at('`alwaysApply: true` requires an explicit `description`');
    }
  }

  return errors;
}

// ---------------------------------------------------------------------------
// Entry building + rendering
// ---------------------------------------------------------------------------

function deriveDomain(rel, ctx) {
  if (typeof ctx.domain === 'string' && ctx.domain.trim()) return ctx.domain.trim();
  const parts = rel.split('/'); // e.g. docs/client/foo.md or docs/foo.md
  if (parts.length >= 3) {
    const seg = parts[1];
    return DIR_DOMAIN[seg] || seg;
  }
  return (ctx.lens && ctx.lens[0]) || 'misc';
}

function buildEntry(rel, ctx, body) {
  const docRel = rel.replace(/^docs\//, ''); // path relative to docs/ for links
  return {
    docRel,
    domain: deriveDomain(rel, ctx),
    description: (ctx.description && clampDescription(ctx.description)) || firstParagraph(body),
    lens: ctx.lens || [],
    globs: ctx.globs || [],
    triggers: ctx.triggers || [],
    // Undefined skills => applies to all skills (rendered as no skills tag).
    skills: ctx.skills || null,
    anchor: ctx.alwaysApply === true,
  };
}

function domainRank(domain) {
  const i = DOMAIN_ORDER.indexOf(domain);
  return i === -1 ? DOMAIN_ORDER.length : i;
}

function renderEntry(e) {
  const marker = e.anchor ? '⚓ ' : '';
  const link = `[${e.docRel}](${encodeURI(e.docRel)})`;
  const facets = [`lens: ${e.lens.join(', ')}`];
  if (e.globs.length) facets.push(`globs: ${e.globs.join(', ')}`);
  if (e.triggers.length) facets.push(`triggers: ${e.triggers.join(', ')}`);
  if (e.skills) facets.push(`skills: ${e.skills.join(', ')}`);
  const desc = e.description ? ` ${e.description}` : '';
  return `- ${marker}${link}:${desc} — ${facets.join(' · ')}`;
}

function renderLlmsTxt(entries) {
  const header = [
    '# Ripls documentation context map',
    '',
    '> Auto-generated by scripts/gen_context_map.js from per-doc `context:` frontmatter.',
    '> Do not edit by hand — run `npm run generate:context-map`.',
    '> How the system works, the schema, and your obligations when changing a doc:',
    '> docs/context_map.md   ·   CI gate: `npm run lint:context-map`',
    '>',
    '> ⚓ marks an always-loaded anchor for its lens. A doc with no `skills:` facet',
    '> applies to all skills; otherwise only the listed ones.',
    '',
  ];

  const byDomain = new Map();
  for (const e of entries) {
    if (!byDomain.has(e.domain)) byDomain.set(e.domain, []);
    byDomain.get(e.domain).push(e);
  }

  const domains = [...byDomain.keys()].sort(
    (a, b) => domainRank(a) - domainRank(b) || a.localeCompare(b),
  );

  const sections = [];
  for (const domain of domains) {
    const rows = byDomain.get(domain).sort((a, b) => a.docRel.localeCompare(b.docRel));
    sections.push(`## ${domain}`, '', ...rows.map(renderEntry), '');
  }

  return `${header.join('\n')}\n${sections.join('\n')}\n`;
}

// ---------------------------------------------------------------------------
// Self-tests — run before the file walk; exit non-zero on failure.
// ---------------------------------------------------------------------------

function assert(cond, msg) {
  if (!cond) {
    console.error(`SELF-TEST FAILED: ${msg}`);
    process.exitCode = 1;
    return false;
  }
  return true;
}

function runSelfTests() {
  let ok = true;

  // Breadcrumb comments are ignored; a valid block parses.
  const valid = [
    '---',
    '# Doc-context metadata for docs/llms.txt — update when this doc changes.',
    '# How it works + schema: docs/context_map.md',
    'context:',
    '  description: Example doc.',
    '  globs: [server/ai/**, server/x/**]',
    '  lens: [architecture, server]',
    '  skills: [issue, audit]',
    '  alwaysApply: false',
    '---',
    '',
    '# Heading',
    'Body text.',
  ].join('\n');
  const v = parseContext(valid);
  ok = assert(v.parseError === null && v.context, 'valid block parses') && ok;
  ok = assert(v.context && v.context.description === 'Example doc.', 'description read') && ok;
  ok = assert(v.context && v.context.globs.length === 2, 'flow list read') && ok;
  ok = assert(validate('x.md', v.context).length === 0, 'valid block has no errors') && ok;

  // No frontmatter → untagged, not an error.
  const none = parseContext('# Just a doc\n\nText.');
  ok = assert(none.context === null && none.parseError === null, 'no frontmatter => untagged') && ok;

  // Frontmatter without a context key → untagged.
  const other = parseContext('---\ntitle: Foo\n---\nBody');
  ok = assert(other.context === null, 'frontmatter without context => untagged') && ok;

  // Invalid lens fails.
  ok = assert(
    validate('x.md', { lens: ['bogus'] }).some((e) => e.includes('invalid lens')),
    'invalid lens rejected',
  ) && ok;

  // Missing lens fails.
  ok = assert(
    validate('x.md', { description: 'd' }).some((e) => e.includes('missing required field `lens`')),
    'missing lens rejected',
  ) && ok;

  // Anchor without description fails.
  ok = assert(
    validate('x.md', { lens: ['architecture'], alwaysApply: true })
      .some((e) => e.includes('requires an explicit `description`')),
    'anchor without description rejected',
  ) && ok;

  // Unknown key fails (typo guard).
  ok = assert(
    validate('x.md', { lens: ['architecture'], lenses: ['x'] }).some((e) => e.includes('unknown field')),
    'unknown field rejected',
  ) && ok;

  // Malformed YAML reports a parse error, not a crash.
  const bad = parseContext('---\ncontext:\n  lens: [a, b\n---\nbody');
  ok = assert(bad.parseError !== null, 'malformed YAML reported') && ok;

  if (!ok) {
    console.error('Self-tests failed — fix gen_context_map.js before running against docs/.');
    process.exit(1);
  }
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

function main() {
  runSelfTests();

  const check = process.argv.includes('--check');
  const docs = collectDocs();

  const entries = [];
  const errors = [];
  const untagged = [];

  for (const rel of docs) {
    const src = fs.readFileSync(path.join(ROOT, rel), 'utf8');
    const { context, parseError } = parseContext(src);
    if (parseError) {
      errors.push(`${rel}: malformed frontmatter YAML — ${parseError}`);
      continue;
    }
    if (context === null) {
      untagged.push(rel);
      continue;
    }
    const errs = validate(rel, context);
    if (errs.length) {
      errors.push(...errs);
      continue;
    }
    const { body } = splitFrontmatter(src);
    entries.push(buildEntry(rel, context, body));
  }

  // Invalid blocks always fail, in both modes.
  if (errors.length) {
    console.error(`gen_context_map: ${errors.length} frontmatter error(s):`);
    for (const e of errors) console.error(`  ${e}`);
    console.error('\nFix the `context:` block(s). Schema: docs/context_map.md');
    process.exit(1);
  }

  const content = renderLlmsTxt(entries);
  const coverage = `${entries.length} tagged, ${untagged.length} untagged ` +
    `(${entries.length + untagged.length} durable docs)`;

  if (check) {
    const current = fs.existsSync(OUTPUT_ABS) ? fs.readFileSync(OUTPUT_ABS, 'utf8') : null;
    if (current !== content) {
      console.error(`gen_context_map: ${OUTPUT_REL} is stale.`);
      console.error('Run `npm run generate:context-map` and commit the result.');
      process.exit(1);
    }
    if (ENFORCE_FULL_COVERAGE && untagged.length) {
      console.error(`gen_context_map: ${untagged.length} durable doc(s) missing a \`context:\` block:`);
      for (const u of untagged) console.error(`  ${u}`);
      console.error('\nEvery durable doc must be tagged. Schema: docs/context_map.md');
      process.exit(1);
    }
    console.log(`check_context_map passed: ${OUTPUT_REL} current — ${coverage}.`);
    if (untagged.length) {
      console.log(`  (${untagged.length} doc(s) not yet tagged — backfill in progress)`);
    }
    return;
  }

  fs.writeFileSync(OUTPUT_ABS, content);
  console.log(`Wrote ${OUTPUT_REL} — ${coverage}.`);
  if (untagged.length) {
    console.log('Untagged durable docs (add a `context:` block — see docs/context_map.md):');
    for (const u of untagged) console.log(`  ${u}`);
  }
}

// Run as a CLI only when invoked directly; when required as a module (e.g. by
// scripts/doc_freshness.js) expose the pure helpers so the freshness tooling and
// this generator share one definition of "durable doc" and frontmatter parsing.
if (require.main === module) {
  main();
}

module.exports = {
  ROOT,
  DOCS_DIR,
  EXCLUDED_PREFIXES,
  EXCLUDED_FILES,
  isExcluded,
  relPosix,
  splitFrontmatter,
  parseContext,
  collectDocs,
};
