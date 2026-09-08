'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');

const {
  normalizeRef,
  extractInlineCodePaths,
  globToRegExp,
  matchGlob,
  docGlobs,
  parseFreshness,
  upsertFreshness,
} = require('./doc_freshness');

const { parseContext, splitFrontmatter } = require('./gen_context_map');

// --- normalizeRef -----------------------------------------------------------

test('normalizeRef strips glob tails, line/anchor suffixes, and punctuation', () => {
  assert.deepEqual(normalizeRef('server/impact/**'), { path: 'server/impact', glob: true });
  assert.deepEqual(normalizeRef('server/x/*'), { path: 'server/x', glob: true });
  assert.deepEqual(normalizeRef('server/storage/protosql.go:42'), { path: 'server/storage/protosql.go', glob: false });
  assert.deepEqual(normalizeRef('docs/context_map.md#schema'), { path: 'docs/context_map.md', glob: false });
  assert.deepEqual(normalizeRef('`server/foo.go`.'), { path: 'server/foo.go', glob: false });
  assert.equal(normalizeRef('app/lib/*sharing*').glob, true);
});

// --- extractInlineCodePaths -------------------------------------------------

test('extractInlineCodePaths picks up backtick code paths, not prose', () => {
  const body = [
    'The check lives in `server/impact/provenance.go` and config at',
    '`server/impact/estimator/config.textproto`. See `proto/ripls/api/x.proto`.',
    'It is not the `Provenance` type, nor a `bare-word`, nor `justtext`.',
    'Globs like `app/lib/**` are paths too.',
  ].join('\n');
  const got = extractInlineCodePaths(body).sort();
  assert.deepEqual(got, [
    'app/lib',
    'proto/ripls/api/x.proto',
    'server/impact/estimator/config.textproto',
    'server/impact/provenance.go',
  ]);
});

test('extractInlineCodePaths ignores tokens without a recognized top-level prefix', () => {
  const body = 'See `foo/bar.go`, `lib/x.dart`, and `Provenance` — none qualify.';
  assert.deepEqual(extractInlineCodePaths(body), []);
});

// --- glob matching ----------------------------------------------------------

test('globToRegExp / matchGlob handle ** , * and literals', () => {
  assert.ok(matchGlob('server/**', 'server/services/gear/service.go'));
  assert.ok(matchGlob('server/**', 'server/x.go'));
  assert.ok(!matchGlob('server/**', 'app/lib/main.dart'));
  assert.ok(matchGlob('app/lib/**', 'app/lib/presentation/widgets/foo.dart'));
  assert.ok(matchGlob('server/storage/instrumented_db.go', 'server/storage/instrumented_db.go'));
  assert.ok(!matchGlob('server/storage/instrumented_db.go', 'server/storage/other.go'));
  // `*` does not cross a path separator.
  assert.ok(matchGlob('app/lib/*.dart', 'app/lib/main.dart'));
  assert.ok(!matchGlob('app/lib/*.dart', 'app/lib/sub/main.dart'));
  // substring glob
  assert.ok(matchGlob('app/lib/**/*sharing*', 'app/lib/presentation/viewmodels/gear_sharing_view_model.dart'));
});

// --- docGlobs ---------------------------------------------------------------

test('docGlobs reads context.globs', () => {
  const src = [
    '---',
    'context:',
    '  globs: [server/impact_metrics/**, server/services/gear/service.go]',
    '  lens: [domain, server]',
    '---',
    '# Body',
  ].join('\n');
  assert.deepEqual(docGlobs(src), ['server/impact_metrics/**', 'server/services/gear/service.go']);
});

// --- freshness stamp --------------------------------------------------------

const DOC_WITH_CONTEXT = [
  '---',
  '# Doc-context metadata for docs/llms.txt — update when this doc changes.',
  '# How it works + schema: docs/context_map.md',
  'context:',
  '  description: Example doc.',
  '  globs: [server/impact_metrics/**]',
  '  lens: [domain, server]',
  '  domain: impact',
  '---',
  '# Heading',
  '',
  'Body text referencing `server/impact_metrics/provenance.go`.',
  '',
].join('\n');

test('parseFreshness returns null when no freshness block is present', () => {
  assert.equal(parseFreshness(DOC_WITH_CONTEXT), null);
});

test('upsertFreshness inserts a block, preserving comments and context', () => {
  const next = upsertFreshness(DOC_WITH_CONTEXT, { verifiedCommit: 'abc123', verifiedOn: '2026-06-08' });
  const f = parseFreshness(next);
  assert.deepEqual(f, { verified_commit: 'abc123', verified_on: '2026-06-08' });
  // Breadcrumb comment and the context block are untouched.
  assert.ok(next.includes('# How it works + schema: docs/context_map.md'));
  assert.ok(next.includes('  globs: [server/impact_metrics/**]'));
  // Body untouched.
  assert.ok(next.includes('Body text referencing `server/impact_metrics/provenance.go`.'));
});

test('upsertFreshness replaces an existing block (advances the stamp) and is idempotent on commit', () => {
  const once = upsertFreshness(DOC_WITH_CONTEXT, { verifiedCommit: 'old', verifiedOn: '2026-01-01' });
  const twice = upsertFreshness(once, { verifiedCommit: 'new', verifiedOn: '2026-06-08' });
  assert.deepEqual(parseFreshness(twice), { verified_commit: 'new', verified_on: '2026-06-08' });
  // Exactly one freshness block remains (no duplication).
  assert.equal((twice.match(/^freshness:/gm) || []).length, 1);
  // Stamping the same commit twice is a no-op.
  const thrice = upsertFreshness(twice, { verifiedCommit: 'new', verifiedOn: '2026-06-08' });
  assert.equal(thrice, twice);
});

test('upsertFreshness creates frontmatter when a doc has none', () => {
  const plain = '# Just a doc\n\nText.\n';
  const next = upsertFreshness(plain, { verifiedCommit: 'sha', verifiedOn: '2026-06-08' });
  assert.deepEqual(parseFreshness(next), { verified_commit: 'sha', verified_on: '2026-06-08' });
  assert.ok(next.includes('# Just a doc'));
});

// --- the load-bearing cross-check ------------------------------------------
// A freshness stamp must be INVISIBLE to the context-map gate: gen_context_map
// reads only `doc.context`, so stamping a doc must not change its parsed context
// (and therefore cannot make docs/llms.txt stale). This is why the stamp lives
// outside the `context:` block.

test('adding a freshness block does not change the parsed context', () => {
  const before = parseContext(DOC_WITH_CONTEXT);
  const stamped = upsertFreshness(DOC_WITH_CONTEXT, { verifiedCommit: 'abc', verifiedOn: '2026-06-08' });
  const after = parseContext(stamped);
  assert.equal(after.parseError, null);
  assert.deepEqual(after.context, before.context);
});

test('a freshness block does not perturb the body the generator sees', () => {
  const stamped = upsertFreshness(DOC_WITH_CONTEXT, { verifiedCommit: 'abc', verifiedOn: '2026-06-08' });
  assert.equal(splitFrontmatter(stamped).body, splitFrontmatter(DOC_WITH_CONTEXT).body);
});
