// e2e/tests/workflows/universal-search.spec.ts
//
// Maps to: the universal search surface (UniversalSearchScreen), opened from
// the destination-header search icon ("Search your communities",
// a11yNavDockSearch). No 1:1 workflow doc; this is a search regression.
//
// What this spec proves (UI-integration failure class RPC tests can't):
//   - The header search icon opens universal search, typing a query into the
//     real Flutter Web text field drives SearchService, and a matching item
//     shared into the viewer's community surfaces as a result row under the
//     correct section — the header → text-field → SearchService → sectioned
//     results wiring, end to end in the browser.
//   - Scoping: an item that shares NO community with the viewer must NOT
//     surface (search is membership-scoped).
//
// What this spec does NOT prove:
//   - Search ranking/scoring or fuzzy matching — server/services/search owns
//     that (unit-tested). Here a distinctive token is asserted to surface its
//     item, and an out-of-scope item is asserted absent.
//
// Preconditions seeded via RPC; the query + navigation are real UI gestures.

import { test, expect } from '../../lib/fixtures.js';
import { createTestClient, installRequestIdOverride } from '../../lib/connect.js';
import { newRecordingContext } from '../../lib/context.js';
import { injectAuth } from '../../lib/auth.js';
import { dismissDataConsent } from '../../lib/ui/consent.js';
import { registerUser } from '../../lib/seed/users.js';
import { createCommunity } from '../../lib/seed/communities.js';
import { seedSharedGear } from '../../lib/seed/gear.js';
import { Availability } from '../../gen/ripls/api/gear_pb.js';
import { CommunityService } from '../../gen/ripls/api/community_service_pb.js';

const SPEC_SLUG = 'universal-search';

// Distinctive names so the query token can't collide with any other seed.
const IN_SCOPE_GEAR = 'Zephyr Nebula Telescope';
const OUT_OF_SCOPE_GEAR = 'Quokka Driftwood Canoe';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set; global-setup.ts populates it');
  return url;
}

test('Universal search surfaces a community item and excludes out-of-scope items', async ({
  browser,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'Universal search validated on chromium only');
  const baseUrl = requireBaseUrl();

  // ── PRECONDITIONS (RPC seed) ─────────────────────────────────────────────
  // Viewer belongs to community C; the in-scope gear is shared into C. The
  // out-of-scope gear belongs to a STRANGER's own community the viewer is not
  // in, so search (membership-scoped) must not return it.
  const viewer = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Nadia Viewer' });
  const community = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: viewer.accessToken,
    name: 'Larkspur Lending Circle',
  });
  const inScope = await seedSharedGear({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: viewer.accessToken,
    name: IN_SCOPE_GEAR,
    availability: Availability.FOR_LOAN,
  });
  const community_ = createTestClient(CommunityService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: viewer.accessToken,
  });
  await community_.shareItem({
    item: { case: 'gearId', value: inScope.gearId },
    shareToCommunityIds: [community.communityId],
  });

  // A stranger with their own gear in their own community — no shared scope.
  const stranger = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Otto Stranger' });
  await seedSharedGear({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: stranger.accessToken,
    name: OUT_OF_SCOPE_GEAR,
    availability: Availability.FOR_LOAN,
  });

  // ── BROWSER CONTEXT + AUTH ───────────────────────────────────────────────
  const ctx = await newRecordingContext(browser, { locale: 'en-US' });
  const page = await ctx.newPage();
  try {
    await injectAuth(ctx, {
      accessToken: viewer.accessToken,
      refreshToken: viewer.refreshToken,
      user: { id: viewer.userId, name: viewer.name },
      serverUrl: baseUrl,
    });
    await installRequestIdOverride(page, SPEC_SLUG);
    await page.goto(`${baseUrl}/`);
    await dismissDataConsent(page);

    // ── STEP: open universal search from the header icon ────────────────────
    await page
      .getByRole('button', { name: 'Search your communities' })
      .first()
      .click({ timeout: 15_000 });

    // The field autofocuses on open. Flutter-web text fields ignore .fill();
    // type real keystrokes so the TextEditingController updates and the search
    // fires (300ms debounce, absorbed by the locator auto-wait below).
    const queryField = page.getByRole('textbox').first();
    await queryField.click();
    await queryField.pressSequentially('Zephyr', { delay: 30 });

    // ── ASSERT: the in-scope item surfaces (as the Top Hit and/or a row) ────
    const inScopeResult = page.getByRole('button', { name: IN_SCOPE_GEAR, exact: true });
    await expect(inScopeResult.first()).toBeVisible({ timeout: 15_000 });

    // ── ASSERT: scoping — an item in no shared community is never returned ──
    // Re-query with the stranger's distinctive token. Asserting the in-scope
    // result DISAPPEARS first proves the search actually re-ran on the new
    // query (not a trivial pass); then the out-of-scope item must be absent.
    await queryField.click();
    await page.keyboard.press('ControlOrMeta+A');
    await page.keyboard.press('Backspace');
    await queryField.pressSequentially('Quokka', { delay: 30 });
    await expect(inScopeResult).toHaveCount(0, { timeout: 15_000 });
    await expect(
      page.getByRole('button', { name: OUT_OF_SCOPE_GEAR, exact: true }),
    ).toHaveCount(0);
  } finally {
    await ctx.close();
  }
});
