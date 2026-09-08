'use strict';

const { test } = require('node:test');
const assert = require('node:assert/strict');
const { mkdtempSync, writeFileSync, existsSync, readFileSync, rmSync } = require('node:fs');
const { join } = require('node:path');
const { tmpdir } = require('node:os');
const { gunzipSync, brotliDecompressSync } = require('node:zlib');
const { randomBytes } = require('node:crypto');

const {
  shouldCompress,
  precompressDir,
  COMPRESSIBLE_EXTENSIONS,
  MIN_SIZE_BYTES,
} = require('./precompress_web_bundle');

// A payload comfortably above the size floor that also compresses well
// (highly repetitive), so the smaller-than-source guard passes.
const bigText = 'a'.repeat(MIN_SIZE_BYTES * 4);

function scratch() {
  return mkdtempSync(join(tmpdir(), 'precompress-test-'));
}

test('shouldCompress: allowlisted extension above the floor', () => {
  assert.equal(shouldCompress('main.dart.wasm', MIN_SIZE_BYTES + 1), true);
  assert.equal(shouldCompress('main.dart.js', MIN_SIZE_BYTES + 1), true);
  assert.equal(shouldCompress('index.html', MIN_SIZE_BYTES + 1), true);
});

test('shouldCompress: filters non-allowlisted (already-compressed) types', () => {
  assert.equal(shouldCompress('logo.png', MIN_SIZE_BYTES * 10), false);
  assert.equal(shouldCompress('font.woff2', MIN_SIZE_BYTES * 10), false);
  assert.equal(shouldCompress('icon.ico', MIN_SIZE_BYTES * 10), false);
});

test('shouldCompress: enforces the size floor', () => {
  assert.equal(shouldCompress('tiny.js', MIN_SIZE_BYTES - 1), false);
  assert.equal(shouldCompress('tiny.js', MIN_SIZE_BYTES), true);
});

test('shouldCompress: never re-compresses .br/.gz siblings (idempotency)', () => {
  assert.equal(shouldCompress('main.dart.wasm.br', MIN_SIZE_BYTES * 10), false);
  assert.equal(shouldCompress('main.dart.wasm.gz', MIN_SIZE_BYTES * 10), false);
});

test('shouldCompress: extension match is case-insensitive', () => {
  assert.equal(shouldCompress('DATA.JSON', MIN_SIZE_BYTES + 1), true);
});

test('precompressDir: writes valid .br and .gz next to a compressible file', () => {
  const dir = scratch();
  try {
    writeFileSync(join(dir, 'main.dart.js'), bigText);

    const stats = precompressDir(dir);

    assert.equal(stats.files, 1);
    assert.ok(existsSync(join(dir, 'main.dart.js.br')), '.br written');
    assert.ok(existsSync(join(dir, 'main.dart.js.gz')), '.gz written');
    assert.ok(stats.brBytesSaved > 0, 'brotli reported savings');
    assert.ok(stats.gzBytesSaved > 0, 'gzip reported savings');

    // Round-trip: the variants decode back to the original bytes.
    assert.equal(
      brotliDecompressSync(readFileSync(join(dir, 'main.dart.js.br'))).toString(),
      bigText,
    );
    assert.equal(
      gunzipSync(readFileSync(join(dir, 'main.dart.js.gz'))).toString(),
      bigText,
    );
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test('precompressDir: recurses into subdirectories', () => {
  const dir = scratch();
  try {
    const sub = join(dir, 'canvaskit');
    require('node:fs').mkdirSync(sub);
    writeFileSync(join(sub, 'skwasm.wasm'), bigText);

    const stats = precompressDir(dir);

    assert.equal(stats.files, 1);
    assert.ok(existsSync(join(sub, 'skwasm.wasm.br')));
    assert.ok(existsSync(join(sub, 'skwasm.wasm.gz')));
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test('precompressDir: skips non-allowlisted and sub-floor files', () => {
  const dir = scratch();
  try {
    writeFileSync(join(dir, 'photo.png'), bigText); // wrong extension
    writeFileSync(join(dir, 'tiny.js'), 'a'.repeat(MIN_SIZE_BYTES - 1)); // too small

    const stats = precompressDir(dir);

    assert.equal(stats.files, 0);
    assert.ok(!existsSync(join(dir, 'photo.png.br')));
    assert.ok(!existsSync(join(dir, 'tiny.js.br')));
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test('precompressDir: is idempotent (second run is a no-op, no .br.br)', () => {
  const dir = scratch();
  try {
    writeFileSync(join(dir, 'main.dart.js'), bigText);

    const first = precompressDir(dir);
    const second = precompressDir(dir);

    assert.equal(first.files, 1);
    assert.equal(second.files, 1, 'still finds the one source file');
    assert.ok(!existsSync(join(dir, 'main.dart.js.br.br')), 'no double-compression');
    assert.ok(!existsSync(join(dir, 'main.dart.js.gz.gz')));
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test('precompressDir: skips a variant that would be larger than source', () => {
  const dir = scratch();
  try {
    // Genuinely incompressible high-entropy bytes just over the floor:
    // gzip/brotli output is >= source, so neither variant is kept.
    writeFileSync(join(dir, 'random.json'), randomBytes(MIN_SIZE_BYTES + 16));

    precompressDir(dir);

    assert.ok(!existsSync(join(dir, 'random.json.gz')), 'no larger-than-source .gz');
    assert.ok(!existsSync(join(dir, 'random.json.br')), 'no larger-than-source .br');
  } finally {
    rmSync(dir, { recursive: true, force: true });
  }
});

test('COMPRESSIBLE_EXTENSIONS includes the load-bearing WASM bundle types', () => {
  for (const ext of ['.wasm', '.js', '.mjs', '.json', '.html', '.symbols']) {
    assert.ok(COMPRESSIBLE_EXTENSIONS.has(ext), `${ext} allowlisted`);
  }
});
