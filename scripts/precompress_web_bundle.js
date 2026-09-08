'use strict';

// Precompress the Flutter Web bundle so the Go embed handler can serve
// brotli/gzip variants instead of the raw ~9.6 MB main.dart.wasm.
//
// Flutter emits nothing precompressed and the server (app_handler.go)
// serves the //go:embed bundle uncompressed, so every cold web visit
// pulls the full raw bytes. This walks the build output and writes a
// `<file>.br` (brotli, quality 11) and `<file>.gz` (gzip, level 9)
// next to each compressible file; ServeApp() then negotiates
// br -> gzip -> identity from the request's Accept-Encoding.
//
// We ship BOTH brotli and gzip on purpose (issue #2119): brotli is the
// primary win (every browser that runs the app supports brotli over
// HTTPS), and gzip is cheap insurance for proxies that strip `br` from
// Accept-Encoding and for the older-browser JS-fallback tier. Dropping
// to brotli-only later is a clean trim if the extra embed weight ever
// matters.
//
// Runs at build time from the `build:web:*` npm scripts, on
// `app/build/web` before that output is copied into
// `server/services/web/app_assets/` — so the variants embed with no
// other change. No system `brotli` binary needed: Node's built-in
// `zlib` provides both codecs.
//
// Usage: node scripts/precompress_web_bundle.js [dir]
//   dir defaults to app/build/web (relative to cwd).

const {
  brotliCompressSync,
  gzipSync,
  constants: zlibConstants,
} = require('node:zlib');
const { readdirSync, readFileSync, writeFileSync, statSync } = require('node:fs');
const { join, extname } = require('node:path');

// Extensions worth compressing. Text and wasm compress well; already-
// compressed binary formats (png/jpg/webp/woff2/ico) do not and are
// skipped so we don't spend build time producing larger-than-source
// variants. `.symbols` are Flutter's deobfuscation maps (large, plain
// text) and `.map` are source maps — both compress well.
const COMPRESSIBLE_EXTENSIONS = new Set([
  '.js',
  '.mjs',
  '.wasm',
  '.json',
  '.html',
  '.css',
  '.otf',
  '.ttf',
  '.map',
  '.symbols',
  '.txt',
  '.xml',
  '.svg',
]);

// Below this size the compression overhead (and the extra HTTP
// negotiation + embed entries) isn't worth it.
const MIN_SIZE_BYTES = 1024;

/** True when a file at `name` should get precompressed variants. */
function shouldCompress(name, sizeBytes) {
  // Never compress an already-compressed sibling (idempotency: a second
  // run must not produce foo.js.br.br).
  if (name.endsWith('.br') || name.endsWith('.gz')) return false;
  if (sizeBytes < MIN_SIZE_BYTES) return false;
  return COMPRESSIBLE_EXTENSIONS.has(extname(name).toLowerCase());
}

const brotli = (buf) =>
  brotliCompressSync(buf, {
    params: {
      [zlibConstants.BROTLI_PARAM_QUALITY]: 11,
      [zlibConstants.BROTLI_PARAM_SIZE_HINT]: buf.length,
    },
  });

const gzip = (buf) => gzipSync(buf, { level: 9 });

/**
 * Recursively precompress every compressible file under `dir`.
 * Writes `<file>.br` and `<file>.gz` only when the compressed output
 * is actually smaller than the source. Idempotent.
 *
 * @returns {{files:number, brBytesSaved:number, gzBytesSaved:number}}
 */
function precompressDir(dir) {
  const stats = { files: 0, brBytesSaved: 0, gzBytesSaved: 0 };

  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) {
      const sub = precompressDir(full);
      stats.files += sub.files;
      stats.brBytesSaved += sub.brBytesSaved;
      stats.gzBytesSaved += sub.gzBytesSaved;
      continue;
    }
    if (!entry.isFile()) continue;

    const size = statSync(full).size;
    if (!shouldCompress(entry.name, size)) continue;

    const source = readFileSync(full);

    const br = brotli(source);
    if (br.length < source.length) {
      writeFileSync(`${full}.br`, br);
      stats.brBytesSaved += source.length - br.length;
    }

    const gz = gzip(source);
    if (gz.length < source.length) {
      writeFileSync(`${full}.gz`, gz);
      stats.gzBytesSaved += source.length - gz.length;
    }

    stats.files += 1;
  }

  return stats;
}

const mib = (bytes) => (bytes / 1024 / 1024).toFixed(2);

function main() {
  const dir = process.argv[2] || 'app/build/web';
  try {
    statSync(dir);
  } catch {
    console.error(`precompress: directory not found: ${dir}`);
    process.exit(1);
  }

  const { files, brBytesSaved, gzBytesSaved } = precompressDir(dir);
  console.log(
    `precompress: compressed ${files} file(s) in ${dir} ` +
      `(brotli saved ${mib(brBytesSaved)} MiB, gzip saved ${mib(gzBytesSaved)} MiB)`,
  );
}

module.exports = { shouldCompress, precompressDir, COMPRESSIBLE_EXTENSIONS, MIN_SIZE_BYTES };

// Run main() only when invoked directly, not when imported by the test.
if (require.main === module) {
  main();
}
