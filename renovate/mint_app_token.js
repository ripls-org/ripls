// Mint a GitHub App installation access token for the ripls-renovate App.
//
// Runs inside the ripls-renovate image with `node` (no npm packages — uses only
// node built-in `crypto` + global `fetch`), so the host needs no openssl/jq/
// python. Prints the installation token to stdout; run_renovate.sh captures it
// and passes it to Renovate as RENOVATE_TOKEN.
//
// Reads from the environment:
//   GITHUB_APP_ID                 (required) the App's numeric id
//   GITHUB_APP_PRIVATE_KEY_FILE   path to the App private key PEM, or
//   GITHUB_APP_PRIVATE_KEY        the PEM contents directly
//   GITHUB_APP_INSTALLATION_ID    (optional) defaults to the App's first install
'use strict';
const fs = require('fs');
const crypto = require('crypto');

function b64url(input) {
  return Buffer.from(input)
    .toString('base64')
    .replace(/\+/g, '-')
    .replace(/\//g, '_')
    .replace(/=+$/, '');
}

async function main() {
  const appId = process.env.GITHUB_APP_ID;
  if (!appId) throw new Error('GITHUB_APP_ID is not set');

  let pem = process.env.GITHUB_APP_PRIVATE_KEY;
  if (!pem && process.env.GITHUB_APP_PRIVATE_KEY_FILE) {
    pem = fs.readFileSync(process.env.GITHUB_APP_PRIVATE_KEY_FILE, 'utf8');
  }
  if (!pem) {
    throw new Error('GITHUB_APP_PRIVATE_KEY or GITHUB_APP_PRIVATE_KEY_FILE is not set');
  }

  // App JWT (RS256), valid for a short window; iat backdated to absorb clock skew.
  const now = Math.floor(Date.now() / 1000);
  const header = b64url(JSON.stringify({ alg: 'RS256', typ: 'JWT' }));
  const payload = b64url(JSON.stringify({ iat: now - 60, exp: now + 540, iss: appId }));
  const signer = crypto.createSign('RSA-SHA256');
  signer.update(`${header}.${payload}`);
  const jwt = `${header}.${payload}.${b64url(signer.sign(pem))}`;

  const headers = {
    Authorization: `Bearer ${jwt}`,
    Accept: 'application/vnd.github+json',
    'User-Agent': 'ripls-renovate',
    'X-GitHub-Api-Version': '2022-11-28',
  };

  let installationId = process.env.GITHUB_APP_INSTALLATION_ID;
  if (!installationId) {
    const res = await fetch('https://api.github.com/app/installations', { headers });
    if (!res.ok) {
      throw new Error(`listing installations failed: ${res.status} ${await res.text()}`);
    }
    const installs = await res.json();
    if (!Array.isArray(installs) || installs.length === 0) {
      throw new Error('the App has no installations');
    }
    installationId = installs[0].id;
  }

  const res = await fetch(
    `https://api.github.com/app/installations/${installationId}/access_tokens`,
    { method: 'POST', headers },
  );
  if (!res.ok) {
    throw new Error(`minting installation token failed: ${res.status} ${await res.text()}`);
  }
  const body = await res.json();
  process.stdout.write(body.token);
}

main().catch((err) => {
  process.stderr.write(`mint_app_token: ${err.message || err}\n`);
  process.exit(1);
});
