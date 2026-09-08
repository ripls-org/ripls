// e2e/lib/seed/users.ts — register users via the public
// LoginService.EmailRegister RPC. Dev-mode accepts an empty
// short_code (the first user bootstrap path), so tests can mint
// fresh authenticated users without going through an invite flow
// first.
//
// Registration is passwordless since #2571: each user first proves
// ownership of its address through RequestEmailCode/VerifyEmailCode.
// The code comes back on `devCode`, which the server populates only
// in dev mode — the same seam the Go integration helpers use, and the
// reason this needs no mailbox (and still no Firebase Auth Emulator).
//
// Per the #2162 plan, the e2e harness re-uses the existing public
// RPCs the Go simulation library already drives — no new dev-mode
// service. Persona data lives in TypeScript here (tiny set; only
// names) rather than reaching back into the Go catalog.

import { spawnSync } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { resolve } from 'node:path';
import { LoginService } from '../../gen/ripls/api/login_service_pb.js';
import { createTestClient } from '../connect.js';
import { mintCommunityInviteLink } from './communities.js';

export interface SeededUser {
  userId: string;
  email: string;
  name: string;
  accessToken: string;
  refreshToken: string;
}

export interface RegisterUserOptions {
  baseUrl: string;
  specSlug: string;
  /** Override the name. Default: 'E2E Test User'. */
  name?: string;
  /**
   * Override the email. Default: e2e-{uuid}@ripls.test (per-run
   * unique so the testcontainer DB doesn't collide across runs).
   */
  email?: string;
}

const DEFAULT_NAME = 'E2E Test User';

/**
 * Run the mailed-code loop for an address and return the short-lived
 * proof token EmailRegister accepts in place of a password.
 *
 * Throws a pointed error when `devCode` is absent: that means the
 * server is not in dev mode, which is the same precondition the empty
 * short_code bootstrap path already relies on.
 */
async function proveEmailOwnership(
  client: ReturnType<typeof createTestClient<typeof LoginService>>,
  email: string,
): Promise<string> {
  const codeResp = await client.requestEmailCode({ email });
  if (!codeResp.devCode) {
    throw new Error(
      `RequestEmailCode returned no devCode for ${email}; check server dev-mode flag`,
    );
  }
  const verifyResp = await client.verifyEmailCode({
    email,
    code: codeResp.devCode,
  });
  if (!verifyResp.emailProofToken) {
    throw new Error(`VerifyEmailCode returned no proof token for ${email}`);
  }
  return verifyResp.emailProofToken;
}

/**
 * Mint a fresh authenticated user via LoginService.EmailRegister.
 * Returns the seeded user plus its access/refresh tokens, ready to
 * pass into auth.injectTokens(page, …).
 */
export async function registerUser(opts: RegisterUserOptions): Promise<SeededUser> {
  const email = opts.email ?? `e2e-${randomUUID()}@ripls.test`;
  const name = opts.name ?? DEFAULT_NAME;

  const client = createTestClient(LoginService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
  });
  const resp = await client.emailRegister({
    email,
    name,
    emailProofToken: await proveEmailOwnership(client, email),
    shortCode: '',
  });
  if (!resp.user || !resp.tokens) {
    throw new Error(
      `EmailRegister returned no user/tokens for ${email}; check server dev-mode flag`,
    );
  }
  return {
    userId: resp.user.id,
    email,
    name,
    accessToken: resp.tokens.accessToken,
    refreshToken: resp.tokens.refreshToken,
  };
}

export interface RegisterUserViaInviteOptions extends RegisterUserOptions {
  /** Bearer token of an existing member who can mint the invite link. */
  inviterAccessToken: string;
  /** Community to invite the new user into. */
  communityId: string;
}

/**
 * Register a fresh user *as a member of the given community* via the
 * canonical invite flow: the inviter mints a community-invite short
 * code, the new user calls EmailRegister with that code, and the
 * server joins them to the community as part of registration.
 *
 * This is the analog of `registerUserByInvite` in
 * `server/integration_tests/test_helpers.go`. Prefer this over
 * `registerUser` + share-link-handoff-during-test when a workflow
 * spec's preconditions require pre-existing community membership —
 * the share-link path joins late and exposes the test to the
 * #2237-class race (server records the action but the bundle's
 * initial RPCs already 403'd against `RequireAccessToCommunity-
 * ScopedEntity`).
 */
export async function registerUserViaInvite(
  opts: RegisterUserViaInviteOptions,
): Promise<SeededUser> {
  const inviteLink = await mintCommunityInviteLink({
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: opts.inviterAccessToken,
    communityId: opts.communityId,
  });

  const email = opts.email ?? `e2e-${randomUUID()}@ripls.test`;
  const name = opts.name ?? DEFAULT_NAME;

  const client = createTestClient(LoginService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
  });
  const resp = await client.emailRegister({
    email,
    name,
    emailProofToken: await proveEmailOwnership(client, email),
    shortCode: inviteLink.shortCode,
  });
  if (!resp.user || !resp.tokens) {
    throw new Error(
      `EmailRegister(shortCode=${inviteLink.shortCode}) returned no user/tokens for ${email}`,
    );
  }
  return {
    userId: resp.user.id,
    email,
    name,
    accessToken: resp.tokens.accessToken,
    refreshToken: resp.tokens.refreshToken,
  };
}

/**
 * Seed a LEGACY password account — one holding a password hash and carrying no
 * verified-email stamp — for the specs that exercise the login screen's
 * password fallback.
 *
 * This does NOT go through EmailRegister. Registration refuses a password in
 * every environment now (#2864), deliberately: a dev-only exception would mean
 * e2e exercised a branch production does not have, and would make "you cannot
 * create a password account" untestable in the only place Playwright runs.
 * The row is written by a Go helper instead, through the same storage layer the
 * server uses — protosql keeps the authoritative copy in binary_proto and
 * mirrors the flat columns, so a hand-written INSERT here would have to
 * reproduce both correctly.
 *
 * Returns the created user's id. Leaves with the password path.
 */
export function seedLegacyPasswordUser(opts: {
  email: string;
  name: string;
  password: string;
}): string {
  const dbUrl = process.env.E2E_WORKER_DB_URL;
  if (!dbUrl) {
    throw new Error(
      'seedLegacyPasswordUser: E2E_WORKER_DB_URL is unset; the workerServer ' +
        'fixture is supposed to publish it',
    );
  }

  const res = spawnSync(
    'go',
    [
      'run',
      './server/cmd/seed-legacy-user',
      `-db=${dbUrl}`,
      `-email=${opts.email}`,
      `-name=${opts.name}`,
      `-password=${opts.password}`,
    ],
    { cwd: resolve(__dirname, '..', '..', '..'), encoding: 'utf8' },
  );

  if (res.status !== 0) {
    throw new Error(
      `seed-legacy-user failed (status ${res.status}): ${res.stderr || res.stdout}`,
    );
  }
  return res.stdout.trim();
}
