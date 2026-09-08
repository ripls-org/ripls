#!/usr/bin/env node
/**
 * doc_freshness.js — deterministic plumbing for the ripls-doc-freshness skill.
 *
 * The skill itself does the *semantic* work (does the prose still describe the
 * code?) — that's an LLM judgment. This script owns the mechanical parts around
 * it, so the skill and the documentation context map (scripts/gen_context_map.js)
 * agree on "what is a durable doc" and "what code does this doc govern":
 *
 *   - enumerate durable docs and the code each one governs (its `context: globs`
 *     plus inline-cited code paths in the body);
 *   - the deterministic referenced-path-existence check (the #2403 lane);
 *   - the reverse index: changed files -> docs whose governed code moved (the
 *     ratchet's trigger);
 *   - read / advance the top-level `freshness:` frontmatter stamp.
 *
 * The `freshness:` block lives OUTSIDE the `context:` block on purpose:
 * gen_context_map.js reads only `doc.context`, so stamping a doc never touches
 * the routing index (docs/llms.txt) or trips lint:context-map. See
 * docs/context_map.md and the cross-check in doc_freshness.test.js.
 *
 * Subcommands:
 *   list                         List durable docs (+ stamp state).
 *   governs <doc>                Print the code a doc governs and what resolves.
 *   check-paths [<doc>... | --all]   Report inline code paths that don't resolve.
 *   affected (--base <ref> | --changed <f>...)   Docs whose governed code changed.
 *   stamp <doc> --commit <sha> [--date <YYYY-MM-DD>]   Write/advance the stamp.
 */

'use strict';

const fs = require('fs');
const path = require('path');
const { execFileSync } = require('child_process');
const yaml = require('js-yaml');

const {
  ROOT,
  splitFrontmatter,
  parseContext,
  collectDocs,
} = require('./gen_context_map');

// ---------------------------------------------------------------------------
// Code-path recognition
// ---------------------------------------------------------------------------

// Repo top-level dirs that hold code/config a doc can legitimately reference. A
// backtick token must start with one of these to be treated as a code path —
// this is what suppresses prose nouns and partial paths. Generated/output dirs
// are excluded because they are absent on a clean checkout.
const TOP_LEVEL_PREFIXES = [
  'server/', 'app/', 'proto/', 'scripts/', 'e2e/', 'terraform/', 'website/',
  'renovate/', 'runners/', 'cloud_functions/', 'infra/', '.github/', 'docs/',
];

// Subpaths that are generated/output and won't exist on a clean checkout — a
// reference into them is not "drift", so don't flag it.
const GENERATED_DENYLIST = [
  'server/gen/', 'app/lib/data/gen/', 'e2e/gen/', 'node_modules/', 'tmp/',
  'media_storage/', 'benchmark_results/', 'app/build/', 'build/',
];

function hasPrefix(p, prefixes) {
  return prefixes.some((pre) => p === pre.replace(/\/$/, '') || p.startsWith(pre));
}

/**
 * normalizeRef strips the decorations a path picks up in prose so it can be
 * resolved on disk: a trailing glob (`/**`, `/*`), a `:line` or `#anchor`
 * suffix, surrounding backticks, and trailing sentence punctuation.
 * Returns { path, glob } where glob is true when a wildcard tail was stripped.
 */
function normalizeRef(raw) {
  let s = String(raw).replace(/`/g, '').trim(); // backticks never belong in a path
  let glob = false;
  // Strip a trailing glob tail: server/x/** or server/x/*
  const g = s.match(/^(.*?)\/\*\*?$/);
  if (g) { s = g[1]; glob = true; }
  if (s.includes('*')) glob = true;
  // Strip :line and #anchor suffixes.
  s = s.replace(/[:#].*$/, '');
  // Strip trailing punctuation that rode along from the sentence.
  s = s.replace(/[.,;:)\]]+$/, '');
  return { path: s, glob };
}

/**
 * extractInlineCodePaths returns the de-duplicated set of code-path-looking
 * tokens inside backtick spans in a doc body (frontmatter already removed).
 * Only tokens whose first segment is a recognized top-level dir qualify.
 */
function extractInlineCodePaths(body) {
  const out = new Set();
  // Inline code spans: `...` (single backtick; good enough for path mentions).
  const spanRe = /`([^`\n]+)`/g;
  let m;
  while ((m = spanRe.exec(body)) !== null) {
    const token = m[1].trim();
    // A span may contain more than a path (e.g. "see `server/x.go` for ...") —
    // but our spans are path mentions; split on whitespace and test each word.
    for (const word of token.split(/\s+/)) {
      const { path: p } = normalizeRef(word);
      if (!p || !p.includes('/')) continue;
      if (!hasPrefix(p, TOP_LEVEL_PREFIXES)) continue;
      out.add(p);
    }
  }
  return [...out];
}

/** pathIsGenerated reports whether a resolved path lives under an output dir. */
function pathIsGenerated(p) {
  return hasPrefix(p, GENERATED_DENYLIST);
}

/**
 * resolveRef tests whether a normalized reference exists on disk. For a glob
 * reference (a `/**` tail was present) the base directory existing is enough;
 * for a concrete path the file or dir must exist.
 */
function resolveRef(repoRoot, ref) {
  const { path: p, glob } = normalizeRef(ref);
  if (!p) return true;
  if (pathIsGenerated(p)) return true; // not checked-in; not drift
  const abs = path.join(repoRoot, p);
  if (fs.existsSync(abs)) return true;
  // A glob like `server/foo/**` resolves if `server/foo` exists as a dir.
  if (glob && fs.existsSync(path.dirname(abs))) return true;
  return false;
}

// ---------------------------------------------------------------------------
// Glob matching (minimal — no dependency). Supports `**`, `*`, and literals.
// ---------------------------------------------------------------------------

function globToRegExp(glob) {
  let re = '';
  for (let i = 0; i < glob.length; i++) {
    const c = glob[i];
    if (c === '*') {
      if (glob[i + 1] === '*') {
        // `**` — any chars including `/`. Swallow an optional trailing slash so
        // `server/**` matches `server/x` and `server/`.
        re += '.*';
        i++;
        if (glob[i + 1] === '/') i++;
      } else {
        re += '[^/]*'; // `*` — any chars except `/`
      }
    } else if ('\\^$+?.()|[]{}'.includes(c)) {
      re += `\\${c}`;
    } else {
      re += c;
    }
  }
  return new RegExp(`^${re}$`);
}

/** matchGlob reports whether a repo-relative file path matches a glob. */
function matchGlob(glob, file) {
  return globToRegExp(glob).test(file);
}

// ---------------------------------------------------------------------------
// Durable-doc model
// ---------------------------------------------------------------------------

/** docGlobs returns the `context: globs` list for a doc (empty if none). */
function docGlobs(src) {
  const { context } = parseContext(src);
  if (!context || !Array.isArray(context.globs)) return [];
  return context.globs.filter((g) => typeof g === 'string');
}

/**
 * affectedDocs returns the durable docs whose governed code (their `context:
 * globs`) matches any of the changed files — the ratchet's reverse index. Docs
 * that were themselves changed are always included.
 */
function affectedDocs(changedFiles, repoRoot = ROOT) {
  const docs = collectDocs();
  const changedSet = new Set(changedFiles);
  const affected = [];
  for (const rel of docs) {
    if (changedSet.has(rel)) { affected.push(rel); continue; }
    const src = fs.readFileSync(path.join(repoRoot, rel), 'utf8');
    const globs = docGlobs(src);
    if (changedFiles.some((f) => globs.some((g) => matchGlob(g, f)))) {
      affected.push(rel);
    }
  }
  return affected;
}

// ---------------------------------------------------------------------------
// Freshness stamp — top-level `freshness:` frontmatter block.
// ---------------------------------------------------------------------------

const FRESHNESS_KEY = 'freshness';

/** parseFreshness returns the `freshness` object from frontmatter, or null. */
function parseFreshness(src) {
  const { fm } = splitFrontmatter(src);
  if (fm === null) return null;
  let doc;
  try {
    doc = yaml.load(fm);
  } catch {
    return null;
  }
  if (!doc || typeof doc !== 'object' || !(FRESHNESS_KEY in doc)) return null;
  return doc[FRESHNESS_KEY];
}

function renderFreshnessBlock({ verifiedCommit, verifiedOn }) {
  // Quote both values so YAML keeps them as strings — an unquoted date parses as
  // a YAML timestamp (Date), and an all-numeric sha could parse as a number.
  return [
    'freshness:',
    `  verified_commit: "${verifiedCommit}"`,
    `  verified_on: "${verifiedOn}"`,
  ].join('\n');
}

/**
 * upsertFreshness returns `src` with the `freshness:` block inserted or replaced,
 * preserving the breadcrumb comments and the `context:` block exactly (a
 * surgical text edit, NOT a YAML round-trip which would drop comments and
 * reorder keys). If the doc has no frontmatter, one is created.
 */
function upsertFreshness(src, { verifiedCommit, verifiedOn }) {
  const block = renderFreshnessBlock({ verifiedCommit, verifiedOn });
  const lines = src.split('\n');

  if (lines[0].trim() !== '---') {
    // No frontmatter — create a minimal one carrying just the stamp.
    return `---\n${block}\n---\n\n${src}`;
  }

  // Find the closing fence of the frontmatter.
  let close = -1;
  for (let i = 1; i < lines.length; i++) {
    if (lines[i].trim() === '---') { close = i; break; }
  }
  if (close === -1) return src; // unterminated frontmatter — leave untouched

  // Remove an existing top-level `freshness:` block (the key line + its indented
  // children) from within the frontmatter, if present.
  const fmStart = 1;
  const kept = [];
  for (let i = fmStart; i < close; i++) {
    const line = lines[i];
    if (/^freshness:\s*$/.test(line) || /^freshness:\s/.test(line)) {
      // Skip this line and any following indented (child) lines.
      let j = i + 1;
      while (j < close && /^\s+\S/.test(lines[j])) j++;
      i = j - 1;
      continue;
    }
    kept.push(line);
  }

  const newFm = [...kept, ...block.split('\n')];
  return [lines[0], ...newFm, ...lines.slice(close)].join('\n');
}

// ---------------------------------------------------------------------------
// CLI
// ---------------------------------------------------------------------------

function readDoc(rel) {
  return fs.readFileSync(path.join(ROOT, rel), 'utf8');
}

function gitChangedFiles(base) {
  const out = execFileSync('git', ['diff', '--name-only', `${base}...HEAD`], {
    cwd: ROOT, encoding: 'utf8',
  });
  return out.split('\n').map((s) => s.trim()).filter(Boolean);
}

function todayISO() {
  return new Date().toISOString().slice(0, 10);
}

function cmdList() {
  for (const rel of collectDocs()) {
    const f = parseFreshness(readDoc(rel));
    const stamp = f && f.verified_commit ? f.verified_commit : '(unstamped)';
    console.log(`${stamp}\t${rel}`);
  }
}

function cmdGoverns(rel) {
  const src = readDoc(rel);
  const { body } = splitFrontmatter(src);
  const globs = docGlobs(src);
  const inline = extractInlineCodePaths(body);
  console.log(`# ${rel}`);
  console.log('## context globs');
  for (const g of globs) console.log(`  ${g}`);
  console.log('## inline code paths');
  for (const p of inline) {
    console.log(`  ${resolveRef(ROOT, p) ? 'ok ' : 'MISSING'} ${p}`);
  }
}

function cmdCheckPaths(targets, strict) {
  const docs = targets.length ? targets : collectDocs();
  let unresolved = 0;
  for (const rel of docs) {
    const { body } = splitFrontmatter(readDoc(rel));
    for (const p of extractInlineCodePaths(body)) {
      if (!resolveRef(ROOT, p)) {
        unresolved++;
        console.log(`${rel}: unresolved code path \`${p}\``);
      }
    }
  }
  if (unresolved === 0) console.log('check-paths: all referenced code paths resolve.');
  if (strict && unresolved > 0) process.exit(1);
}

function cmdAffected(args) {
  let changed;
  const baseIdx = args.indexOf('--base');
  if (baseIdx !== -1) {
    changed = gitChangedFiles(args[baseIdx + 1]);
  } else {
    const ci = args.indexOf('--changed');
    changed = ci === -1 ? [] : args.slice(ci + 1).filter((a) => !a.startsWith('--'));
  }
  for (const rel of affectedDocs(changed)) console.log(rel);
}

function cmdStamp(args) {
  const rel = args[0];
  if (!rel || rel.startsWith('--')) {
    console.error('stamp: requires a doc path. Usage: stamp <doc> --commit <sha> [--date <YYYY-MM-DD>]');
    process.exit(1);
  }
  const ci = args.indexOf('--commit');
  if (ci === -1 || !args[ci + 1]) {
    console.error('stamp: --commit <sha> is required');
    process.exit(1);
  }
  const di = args.indexOf('--date');
  const verifiedOn = di !== -1 && args[di + 1] ? args[di + 1] : todayISO();
  const abs = path.join(ROOT, rel);
  const next = upsertFreshness(fs.readFileSync(abs, 'utf8'), {
    verifiedCommit: args[ci + 1],
    verifiedOn,
  });
  fs.writeFileSync(abs, next);
  console.log(`stamped ${rel} @ ${args[ci + 1]} (${verifiedOn})`);
}

function main() {
  const [cmd, ...rest] = process.argv.slice(2);
  switch (cmd) {
    case 'list': return cmdList();
    case 'governs': return cmdGoverns(rest[0]);
    case 'check-paths': {
      const strict = rest.includes('--strict');
      const all = rest.includes('--all');
      const targets = all ? [] : rest.filter((a) => !a.startsWith('--'));
      return cmdCheckPaths(targets, strict);
    }
    case 'affected': return cmdAffected(rest);
    case 'stamp': return cmdStamp(rest);
    default:
      console.error('Usage: doc_freshness.js <list|governs|check-paths|affected|stamp> ...');
      process.exit(1);
  }
}

if (require.main === module) {
  main();
}

module.exports = {
  TOP_LEVEL_PREFIXES,
  GENERATED_DENYLIST,
  normalizeRef,
  extractInlineCodePaths,
  resolveRef,
  globToRegExp,
  matchGlob,
  docGlobs,
  affectedDocs,
  parseFreshness,
  upsertFreshness,
  renderFreshnessBlock,
};
