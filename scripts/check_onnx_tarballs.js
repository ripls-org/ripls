#!/usr/bin/env node
/**
 * check_onnx_tarballs.js — assert the pinned ONNXRUNTIME_VERSION publishes the
 * GitHub release tarballs our images download.
 *
 * Both `Dockerfile` and `runners/Dockerfile` install ONNX Runtime by fetching
 * `onnxruntime-linux-<arch>-<version>.tgz` from the GitHub release. Some ONNX
 * Runtime patch releases exist only in package managers and publish no release
 * tarball at all — 1.22.1 and 1.22.2 were two — so pinning one 404s the
 * download and fails the image build. That is a slow, confusing failure: the PR
 * sticks red on `Validate Runner Candidate` with a wget error (#2370), which is
 * why `renovate.json` carried a hand-maintained `<1.22.1` cap and a comment
 * telling humans to "check the target version publishes one".
 *
 * This is that check, so the cap can be raised from evidence instead of prose.
 * It is deliberately NOT wired into `npm run lint` — it needs network and the
 * GitHub API, and a lint gate that fails when you are offline or rate-limited
 * is worse than the problem. Run it when changing ONNXRUNTIME_VERSION:
 *
 *     npm run check:onnx-tarballs            # verify the pinned version
 *     npm run check:onnx-tarballs -- 1.30.0  # verify a candidate before pinning
 *
 * Exit 0 when both tarballs exist, 1 when either is missing, 2 if the release
 * cannot be read (network/rate limit) — a distinct code so a caller can tell
 * "no tarball" from "could not check".
 */

'use strict';

const fs = require('node:fs');
const path = require('node:path');

const ARCHES = ['x64', 'aarch64'];

/** The ONNXRUNTIME_VERSION pinned in versions.env. */
function pinnedVersion(repoRoot) {
  const env = fs.readFileSync(path.join(repoRoot, 'versions.env'), 'utf8');
  const m = /^ONNXRUNTIME_VERSION=(.+)$/m.exec(env);
  if (!m) throw new Error('ONNXRUNTIME_VERSION not found in versions.env');
  return m[1].trim();
}

/** Asset names published on the microsoft/onnxruntime release for `version`. */
async function releaseAssets(version, fetchImpl = fetch) {
  const url = `https://api.github.com/repos/microsoft/onnxruntime/releases/tags/v${version}`;
  const headers = { accept: 'application/vnd.github+json' };
  if (process.env.GH_TOKEN) headers.authorization = `Bearer ${process.env.GH_TOKEN}`;
  const res = await fetchImpl(url, { headers });
  if (res.status === 404) return null; // no such release
  if (!res.ok) throw new Error(`GitHub API ${res.status} for v${version}`);
  const body = await res.json();
  return (body.assets ?? []).map((a) => a.name);
}

/** Which required tarballs are missing from `assets`. */
function missingTarballs(version, assets) {
  return ARCHES.filter(
    (arch) => !assets.includes(`onnxruntime-linux-${arch}-${version}.tgz`),
  );
}

async function main(argv) {
  const repoRoot = path.resolve(__dirname, '..');
  const version = argv[0] || pinnedVersion(repoRoot);

  let assets;
  try {
    assets = await releaseAssets(version);
  } catch (err) {
    console.error(`check_onnx_tarballs: could not read the release — ${err.message}`);
    process.exit(2);
  }
  if (assets === null) {
    console.error(`check_onnx_tarballs: no microsoft/onnxruntime release tagged v${version}`);
    process.exit(1);
  }

  const missing = missingTarballs(version, assets);
  if (missing.length > 0) {
    console.error(
      `check_onnx_tarballs: v${version} publishes no ${missing
        .map((a) => `linux-${a}`)
        .join(' or ')} tarball.\n` +
        'Dockerfile and runners/Dockerfile download these directly, so this\n' +
        'version would 404 the image build. Pick a release that publishes both.',
    );
    process.exit(1);
  }
  console.log(
    `check_onnx_tarballs passed: v${version} publishes ${ARCHES.map((a) => `linux-${a}`).join(' + ')} tarballs.`,
  );
}

if (require.main === module) {
  main(process.argv.slice(2));
}

module.exports = { pinnedVersion, releaseAssets, missingTarballs };
