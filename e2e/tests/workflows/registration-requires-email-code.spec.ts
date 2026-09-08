// e2e/tests/workflows/registration-requires-email-code.spec.ts
//
// The #2864 ratchet, asserted against a running server: a new email account can
// only be created by someone who proved they receive mail at the address.
//
// This is what fixes the population of password-holding accounts at its current
// size. It can shrink as people migrate to the mailed code, but nothing can add
// to it — which is the precondition for deleting the password path entirely.
//
// It runs against the same server every other spec uses, deliberately. The rule
// holds in dev too: a dev-only exception would mean e2e exercised a branch
// production does not have, and would make exactly this assertion untestable.
//
// The Go unit tests cover the branch in isolation. This covers the wiring —
// that the deployed RPC surface actually refuses, rather than a service method
// refusing while some other path still writes a password.

import { ConnectError, Code } from '@connectrpc/connect';
import { test, expect } from '../../lib/fixtures.js';
import { LoginService } from '../../gen/ripls/api/login_service_pb.js';
import { createTestClient } from '../../lib/connect.js';

const SPEC_SLUG = 'registration-requires-email-code';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set (globalSetup should have set it)');
  return url;
}

test('a password alone cannot create an account', async () => {
  const baseUrl = requireBaseUrl();
  const client = createTestClient(LoginService, { baseUrl, specSlug: SPEC_SLUG });
  const email = `pw-only-${Date.now()}@example.com`;

  const err = await client
    .emailRegister({ email, name: 'Password Only', password: 'password123', shortCode: '' })
    .then(
      () => null,
      (e) => e,
    );

  expect(err, 'a password-only registration was accepted').not.toBeNull();
  expect(ConnectError.from(err).code).toBe(Code.InvalidArgument);
  // Assert the reason, not just the code. A short_code or duplicate-email
  // rejection is also InvalidArgument, so the code alone would keep passing if
  // the credential requirement regressed and something else happened to fail.
  expect(ConnectError.from(err).message).toMatch(/verification code is required/i);

  // And no account was created as a side effect: the address is still free, so
  // a proper code-verified registration for it succeeds.
  const probe = await client
    .requestEmailCode({ email, requireExistingAccount: true })
    .then(
      () => null,
      (e) => e,
    );
  expect(probe, 'the rejected registration still created an account').not.toBeNull();
  expect(ConnectError.from(probe).code).toBe(Code.NotFound);
});

test('an empty credential cannot create an account either', async () => {
  const baseUrl = requireBaseUrl();
  const client = createTestClient(LoginService, { baseUrl, specSlug: SPEC_SLUG });

  const err = await client
    .emailRegister({
      email: `no-cred-${Date.now()}@example.com`,
      name: 'No Credential',
      shortCode: '',
    })
    .then(
      () => null,
      (e) => e,
    );

  expect(err, 'a registration with no credential at all was accepted').not.toBeNull();
  expect(ConnectError.from(err).code).toBe(Code.InvalidArgument);
  expect(ConnectError.from(err).message).toMatch(/verification code is required/i);
});
