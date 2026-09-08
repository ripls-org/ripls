// e2e/tests/workflows/request-authoring-share.spec.ts
//
// The request authoring + sharing loop (#2492 "Shared with"): the host submits a
// request through the REAL unified-create UI, invites two people from the share
// sheet that auto-opens after Save, lands directly in the new request (no
// reload), inspects the "Shared with" card (→ access sheet, which lists the
// invited people by name), then adds a whole community from the access sheet.
// The request twin of gear-authoring-share.spec.ts.
//
// Creation + inviting + sharing + inspection run through the real Flutter Web
// UI; the audience is also confirmed via RPC (GetRequest) so the card's counts
// are anchored to server truth.

import { test, expect } from '../../lib/fixtures.js';
import { installRequestIdOverride, createTestClient } from '../../lib/connect.js';
import { newRecordingContext } from '../../lib/context.js';
import { injectAuth } from '../../lib/auth.js';
import { registerUser, registerUserViaInvite } from '../../lib/seed/users.js';
import { createCommunity } from '../../lib/seed/communities.js';
import { createRequestViaWebUI } from '../../lib/ui/create-item.js';
import { RequestService } from '../../gen/ripls/api/request_service_pb.js';

const SPEC_SLUG = 'request-authoring-share';

test('host submits a request, invites people + a community, and inspects the Shared with panel', async ({
  browser,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'create + share UI flow is validated on chromium only');
  const baseUrl = requireBaseUrl();

  // ── Seed (RPC): host, a named community to add later (PTA, with one member),
  // and two people the host invites individually (in a second shared community
  // so the invite sheet lists them). ──
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Dana Brooks' });
  const pta = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'PTA Volunteers',
  });
  await registerUserViaInvite({
    baseUrl,
    specSlug: SPEC_SLUG,
    name: 'Wes Carter',
    inviterAccessToken: host.accessToken,
    communityId: pta.communityId,
  });
  const blockCrew = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Block Party Crew',
  });
  for (const name of ['Sam Okafor', 'Elena Petrova']) {
    await registerUserViaInvite({
      baseUrl,
      specSlug: SPEC_SLUG,
      name,
      inviterAccessToken: host.accessToken,
      communityId: blockCrew.communityId,
    });
  }

  const hostReq = createTestClient(RequestService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
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

    // ── Host submits the request via the web UI; the share sheet auto-opens. ──
    await createRequestViaWebUI(page, baseUrl, {
      prompt: 'Need folding tables for a weekend bake sale',
    });

    // ── Invite two individuals from the auto-opened share sheet. After the
    // invite, the app drops straight into the new request — no reload. ──
    await page.getByRole('button', { name: 'Invite people', exact: true }).click();
    await invitePerson(page, 'Sam Okafor');
    await invitePerson(page, 'Elena Petrova');
    await page.getByRole('button', { name: /^invite 2$/i }).click();

    // ── The "Shared with" card is now on the request read shell: Sam + Elena =
    // 2 people the host shared with. The count base is the sharee set — the
    // owner is not someone you share with, and the who's-helping panel
    // subtracts from this same base (#2724). ──
    const card = page.getByRole('button', { name: /shared with/i });
    await expect(card).toHaveAccessibleName(/shared with 2 people/i, { timeout: 30_000 });

    // Resolve the request by the invite landing (ListMyRequests omits the
    // audience fields; GetRequest populates them; mock AI titles aren't the prompt).
    const requestId = await pollFor(async () => {
      const mine = await hostReq.listMyRequests({});
      for (const r of mine.requests) {
        const detail = await hostReq.getRequest({ requestId: r.id });
        const names = (detail.request?.invitedIndividuals ?? []).map((u) => u.name);
        if (names.includes('Sam Okafor') && names.includes('Elena Petrova')) return r.id;
      }
      return undefined;
    }, 25_000);
    expect(requestId, 'request created + Sam/Elena invited as individuals').toBeTruthy();

    // ── Tapping the card opens the access sheet, titled "Shared with", listing
    // the invited people by name (the request's own ad-hoc community) — not a
    // faceless "N other communities" aggregate. ──
    await card.click();
    await expect(page.getByText('Shared with')).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText('Sam')).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText('Elena')).toBeVisible();
    await expect(page.getByText(/other communit/i)).toHaveCount(0);

    // ── Add a whole community from the open access sheet: Invite Someone →
    // share sheet → Share to communities → PTA Volunteers → Confirm. ──
    await page.getByRole('button', { name: /invite someone/i }).click();
    await page.getByRole('button', { name: /share to communities/i }).click();
    await page.getByText('PTA Volunteers', { exact: true }).first().click();
    await page.getByRole('button', { name: /^confirm$/i }).click();

    // ── The community share lands (host + Sam + Elena + Wes = 4 distinct)… ──
    const afterShare = await pollFor(async () => {
      const d = await hostReq.getRequest({ requestId: requestId! });
      const names = (d.request?.sharedCommunities ?? []).map((c) => c.communityName);
      return names.includes('PTA Volunteers') ? d : undefined;
    });
    expect(afterShare, 'request shared to PTA Volunteers').toBeTruthy();

    // ── …and the STILL-OPEN panel updates in place to show it (no reopen). ──
    await expect(page.getByText('PTA Volunteers')).toBeVisible({ timeout: 10_000 });
    await page.waitForTimeout(800); // let the final panel settle on camera

    // ── RPC truth: invited individuals + community + the deduped count. ──
    const detail = await hostReq.getRequest({ requestId: requestId! });
    const invited = (detail.request?.invitedIndividuals ?? []).map((u) => u.name).sort();
    expect(invited, 'Sam + Elena are invited individuals on the request').toEqual([
      'Elena Petrova',
      'Sam Okafor',
    ]);
    expect((detail.request?.sharedCommunities ?? []).map((c) => c.communityName)).toContain(
      'PTA Volunteers',
    );
    expect(detail.request?.totalDistinctMemberCount).toBe(4);
  } finally {
    await ctx.close();
  }
});

/** Tap a person in the InviteMembersSheet quick-add grid. The grid labels each
 * face with the person's FIRST name (e.g. "Sam"), so we match that. After the
 * tap, a selected chip carrying the person's FULL name appears — assert it to
 * (a) confirm the RIGHT person was selected (the grid reflows as people are
 * added, and a stale semantic node could otherwise select a neighbour), and
 * (b) let the grid settle before the next tap. */
async function invitePerson(page: import('@playwright/test').Page, fullName: string): Promise<void> {
  const firstName = fullName.split(' ')[0];
  await page
    .getByRole('button', { name: new RegExp(`\\b${firstName}\\b`) })
    .first()
    .click();
  await expect(page.getByText(fullName, { exact: true })).toBeVisible({ timeout: 10_000 });
}

async function pollFor<T>(fn: () => Promise<T | undefined>, timeoutMs = 15_000): Promise<T | undefined> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const v = await fn();
    if (v !== undefined) return v;
    await new Promise((r) => setTimeout(r, 250));
  }
  return undefined;
}

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set; global-setup.ts is supposed to populate it');
  return url;
}
