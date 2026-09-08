// e2e/tests/workflows/share-sheet-community-picker.spec.ts
//
// Per-item community visibility (#2492): a host-only nameless community — the
// per-item community provisioned when you create an event/gear/request, before
// anyone else joins — is pure clutter and must NOT appear in the share sheet's
// community picker. Only named communities (and multi-member ad-hoc ones,
// rendered like group chats) belong there. Suppression is server-side in
// ListCommunities; this exercises it end-to-end through the real share-sheet UI.
//
// Per the project's UI-test principle, the create + picker interaction happens
// through the real Flutter Web UI; only the named-community seed is RPC.

import { test, expect } from '../../lib/fixtures.js';
import { installRequestIdOverride } from '../../lib/connect.js';
import { newRecordingContext } from '../../lib/context.js';
import { injectAuth } from '../../lib/auth.js';
import { registerUser } from '../../lib/seed/users.js';
import { createCommunity } from '../../lib/seed/communities.js';
import { createEventViaWebUI } from '../../lib/ui/create-event.js';

const SPEC_SLUG = 'share-sheet-community-picker';

test('share sheet picker shows named communities, hides host-only per-item ones', async ({
  browser,
  browserName,
}) => {
  test.skip(
    browserName === 'webkit',
    'create + share-sheet UI flow is validated on chromium only',
  );
  const baseUrl = requireBaseUrl();

  // ── Seed (RPC): host + one named community ──
  const host = await registerUser({
    baseUrl,
    specSlug: SPEC_SLUG,
    name: 'E2E Host',
  });
  await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Trail Crew',
  });

  const ctx = await newRecordingContext(browser);
  const page = await ctx.newPage();
  try {
    await injectAuth(ctx, {
      accessToken: host.accessToken,
      refreshToken: host.refreshToken,
      user: { id: host.userId, name: host.name },
      serverUrl: baseUrl,
    });
    await installRequestIdOverride(page, SPEC_SLUG);

    // Creating the event provisions a host-only, nameless per-item community —
    // exactly the per-item clutter that must NOT reach the picker.
    await createEventViaWebUI(page, baseUrl);

    // Open the share sheet's community picker.
    await page.getByRole('button', { name: /share to communities/i }).click();

    // The named community is offered…
    await expect(
      page.getByText('Trail Crew', { exact: true }).first(),
    ).toBeVisible({ timeout: 10_000 });

    // …and the event's host-only per-item community is suppressed. Had it
    // leaked through, it would render with the nameless fallback label
    // "1 member" (no name, no other members); assert that label is absent.
    await expect(page.getByText('1 member')).toHaveCount(0);
  } finally {
    await ctx.close();
  }
});

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) {
    throw new Error(
      'E2E_BASE_URL not set; global-setup.ts is supposed to populate it',
    );
  }
  return url;
}
