'use strict';

const test = require('node:test');
const assert = require('node:assert');

const { releaseAssets, missingTarballs } = require('./check_onnx_tarballs.js');

const assetsFor = (version, arches) =>
  arches.map((a) => `onnxruntime-linux-${a}-${version}.tgz`);

test('a release publishing both tarballs is complete', () => {
  assert.deepStrictEqual(
    missingTarballs('1.29.0', assetsFor('1.29.0', ['x64', 'aarch64'])),
    [],
  );
});

test('regression: a package-manager-only patch is reported missing', () => {
  // 1.22.1/1.22.2 shipped with no release tarball at all, which 404s the image
  // build. That is the failure the <1.22.1 cap existed to prevent (#2370).
  assert.deepStrictEqual(missingTarballs('1.22.1', []), ['x64', 'aarch64']);
});

test('a release missing only aarch64 is still rejected', () => {
  // The prod Dockerfile builds both arches, so x64-only is not good enough.
  assert.deepStrictEqual(
    missingTarballs('1.30.0', assetsFor('1.30.0', ['x64'])),
    ['aarch64'],
  );
});

test('unrelated assets do not satisfy the requirement', () => {
  const assets = [
    'onnxruntime-linux-x64-gpu_cuda12-1.29.0.tgz',
    'onnxruntime-osx-arm64-1.29.0.tgz',
  ];
  assert.deepStrictEqual(missingTarballs('1.29.0', assets), ['x64', 'aarch64']);
});

test('releaseAssets returns null for a nonexistent release', async () => {
  const fake = async () => ({ status: 404, ok: false });
  assert.strictEqual(await releaseAssets('9.9.9', fake), null);
});

test('releaseAssets surfaces asset names', async () => {
  const fake = async () => ({
    status: 200,
    ok: true,
    json: async () => ({ assets: [{ name: 'onnxruntime-linux-x64-1.29.0.tgz' }] }),
  });
  assert.deepStrictEqual(await releaseAssets('1.29.0', fake), [
    'onnxruntime-linux-x64-1.29.0.tgz',
  ]);
});

test('releaseAssets throws on a non-404 API error so it is not read as "no tarball"', async () => {
  const fake = async () => ({ status: 403, ok: false });
  await assert.rejects(() => releaseAssets('1.29.0', fake), /403/);
});
