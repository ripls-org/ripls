// Shared GitHub-App authentication for every trigger handler in the
// ripls-alerts codebase. Uses a *dedicated* GitHub App
// (`ripls-alerts-bot`) that is distinct from the App the Go server's
// feedback service uses (`ripls-feedback-bot`).
//
// Why two Apps:
// The original plan had one App for both surfaces (one rotation surface,
// one bot identity). After PR #1762's smoke test we discovered that
// allowlisting `ripls-feedback-bot` in `claude_plan_issue.yaml` would
// transitively allow user-supplied feedback content to trigger Claude
// planning if the feedback service ever started attaching `claude-fix`
// to its issues — a prompt-injection risk. Splitting identities mirrors
// the principle that what's allowlisted to trigger Claude must never
// carry user content. See the plan doc and the discussion under PR
// #1762 for the trust-boundary analysis.
//
// The `ripls-alerts-bot` App is granted Issues: read & write only, on
// this repo only. Its private key lives in GCP Secret Manager as
// github-app-alerts-private-key-base64 (separate secret from the
// feedback App's github-app-private-key-base64). Value is a
// base64-encoded PEM — `decodeBase64Pem` mirrors the Go server's
// `decodeBase64PEM` in server/main.go so the encoding is consistent.

import { createAppAuth } from '@octokit/auth-app';
import { Octokit } from '@octokit/rest';
import { defineSecret } from 'firebase-functions/params';

export const githubAppPrivateKeyBase64 = defineSecret('github-app-alerts-private-key-base64');

// Coordinates for the ripls-alerts-bot GitHub App, stable across
// environments. Hardcoded as plain integers — App ID and Installation
// ID are not secrets (they're identity pointers, not credentials).
export const GITHUB_APP_ID = 3637024;
export const GITHUB_INSTALLATION_ID = 130393653;

// Repo coordinates are constants because the function deploys to one
// repo. If we ever need per-environment routing (e.g. dev →
// ripls-dev/test-issues), promote these to defineString params.
export const REPO_OWNER = 'Type2Fun-Foundation';
export const REPO_NAME = 'ripls';

/**
 * Decodes the base64-encoded PEM held in Secret Manager. Mirrors the Go
 * server's `decodeBase64PEM` in `server/main.go` so both runtimes can
 * read the same encoding unchanged. Throws if the secret value isn't
 * valid base64 or doesn't contain a PEM-shaped string after decode.
 */
export function decodeBase64Pem(base64: string): string {
  const decoded = Buffer.from(base64, 'base64').toString('utf-8');
  if (!decoded.includes('-----BEGIN') || !decoded.includes('-----END')) {
    throw new Error('decoded github-app-alerts-private-key-base64 is not a valid PEM');
  }
  return decoded;
}

/**
 * Constructs an Octokit authenticated as the GitHub App installation.
 * Trigger handlers call this once per invocation and pass the result
 * into the upsertIssue helper.
 */
export function octokitFromAppSecret(): Octokit {
  const privateKey = decodeBase64Pem(githubAppPrivateKeyBase64.value());
  return new Octokit({
    authStrategy: createAppAuth,
    auth: {
      appId: GITHUB_APP_ID,
      installationId: GITHUB_INSTALLATION_ID,
      privateKey,
    },
  });
}
