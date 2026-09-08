// e2e/tests/workflows/community-promote-nudge.spec.ts
//
// DISPLAY-1 "name this group" promote nudge (#2492). A host turns a nameless,
// per-item ad-hoc community into a real, named one through the community
// create/AI modal running in *promote* mode (final submit → UpdateCommunity).
// Two owner-facing entry points are exercised end-to-end through the real
// Flutter Web UI:
//   1. the event roster ("Who's In") "Name this group…" link, and
//   2. the People tab's nameless-group row "Name" chip (#2568 — the dock-era
//      replacement for the retired Workshop switcher's "Name the group" row).
//
// The nudge only appears once the per-item community has a SECOND member.
//
// The ROSTER flow runs entirely IN-APP (no page.goto reload): create → the share
// sheet auto-opens → Close it → the app pushes the event detail → open the
// Who's-In roster from it. The in-app detail IS tappable in the headless harness,
// BUT only on a clean flow: opening the share sheet's "Invite people" / "Invite
// community" sub-modals leaves the app on a stale pushed route that mis-renders
// the detail afterward, so membership + community sharing are set up via RPC
// (registerUserViaInvite + CommunityService.shareItem) and the detail just needs
// a moment to settle before the first tap. The PEOPLE-TAB flow keeps the in-app
// "Invite people" (to exercise the live community-list refresh) and reaches the
// People tab via a single home (/) reload.
//
// NB: this was originally blamed on a platform-view iframe intercepting taps —
// that was WRONG. A DOM probe showed the only iframe is a harmless 1×1 Firebase
// auth iframe and there is no <video>; the detail is genuinely interactable.
//
// Success is asserted via GetCommunity (the per-item community's `name` is now
// non-empty) and the named community appearing in the roster. Community
// creation no longer generates any text — the host types the group's name
// directly into the modal's name field (the only AI is a best-effort background
// image, which is irrelevant to the promote assertion).

import { test, expect, type Page } from '../../lib/fixtures.js';
import { installRequestIdOverride, createTestClient } from '../../lib/connect.js';
import { newRecordingContext } from '../../lib/context.js';
import { injectAuth } from '../../lib/auth.js';
import { registerUser, registerUserViaInvite } from '../../lib/seed/users.js';
import { createCommunity } from '../../lib/seed/communities.js';
import { createEventViaWebUI } from '../../lib/ui/create-event.js';
import { ExperienceService } from '../../gen/ripls/api/experience_service_pb.js';
import { CommunityService } from '../../gen/ripls/api/community_service_pb.js';

const SPEC_SLUG = 'community-promote-nudge';

// The group name the host types to promote the per-item community. There is no
// minimum length and no AI text generation anymore — the name is used verbatim.
const GROUP_NAME = 'Friday Night Trivia';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) {
    throw new Error('E2E_BASE_URL not set; global-setup.ts is supposed to populate it');
  }
  return url;
}

const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

// Brief, deliberate pause so a human watching the recorded video can take in
// each screen. NOT a correctness sync — Playwright auto-waits for elements; this
// only paces the recording.
const pause = (page: Page, ms = 700): Promise<void> => page.waitForTimeout(ms);

// Create an event through the web UI, then resolve its id + its per-item
// (origin) ad-hoc community id by reading ListMyExperiences back. A freshly
// created event is shared only into its own per-item community, so that is the
// sole entry in sharedCommunityIds. Leaves the share sheet open.
async function createEventAndResolve(
  page: Page,
  baseUrl: string,
  hostAccessToken: string,
): Promise<{ experienceId: string; perItemCommunityId: string }> {
  await createEventViaWebUI(page, baseUrl);
  const hostExp = createTestClient(ExperienceService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: hostAccessToken,
  });
  let experienceId = '';
  let perItemCommunityId = '';
  const deadline = Date.now() + 15_000;
  while (Date.now() < deadline) {
    const mine = await hostExp.listMyExperiences({});
    const created = mine.experiences[0];
    if (created && created.sharedCommunityIds.length > 0) {
      experienceId = created.id;
      perItemCommunityId = created.sharedCommunityIds[0];
      break;
    }
    await sleep(250);
  }
  expect(perItemCommunityId, 'event created with a per-item community').not.toBe('');
  return { experienceId, perItemCommunityId };
}

// Invite an existing Ripls member to the item via the share sheet's "Invite
// people" → DarkPersonSearch. The person must be a co-member of one of the
// host's communities to appear in the search. Adds them to the item's per-item
// community in-app, which refreshes the community list + roster live (no
// reload). Leaves the share sheet open.
async function invitePersonViaShareSheet(page: Page, name: string): Promise<void> {
  await page.getByRole('button', { name: /invite people/i }).click();
  // The sheet pre-lists the host's community members under "ADD FROM YOUR
  // COMMUNITY"; each row's accessible name reads like "Ada A Ada" (name +
  // avatar initials), so match the name as a word rather than exactly.
  const result = page
    .getByRole('button', { name: new RegExp(`\\b${name}\\b`, 'i') })
    .first();
  await result.waitFor({ timeout: 10_000 });
  await result.click();
  await pause(page, 400);
  await page.getByRole('button', { name: /^invite \d+$/i }).click();
}

// Drive the community modal (already opened in promote mode). It opens straight
// to a two-field form (name + optional description) — no Text/Image toggle and
// no AI "Generate" step — so type the group's name and confirm. The name field
// is targeted by its accessible name ("Community name"); the screen behind the
// modal has its own search box, so a bare textbox locator would be ambiguous.
// The confirm button is targeted by its stable semantics id because its
// promote-mode label ("Name this group") collides with the roster link that can
// open it.
async function promoteViaModal(page: Page, name: string): Promise<void> {
  // The name field's accessible name is its hint, "Community name" (the
  // description field's is "Community description"), so an exact match is
  // unambiguous and won't collide with the search box on the screen behind.
  const field = page.getByRole('textbox', { name: 'Community name', exact: true });
  await field.click();
  // Flutter-web text fields ignore .fill(); use real keystrokes so Flutter's
  // input model registers each character.
  await field.pressSequentially(name, { delay: 30 });
  await pause(page, 500);
  const confirm = page.locator(
    '[flt-semantics-identifier="community-preview-confirm"]',
  );
  await confirm.waitFor({ timeout: 30_000 });
  // Let a human watching the recording see the typed name + photo before confirm.
  await pause(page, 1300);
  await confirm.click();
}

// Poll GetCommunity until the per-item community has gained a name (the promote
// landed). Returns the new name.
async function expectCommunityNamed(
  baseUrl: string,
  accessToken: string,
  communityId: string,
): Promise<string> {
  const client = createTestClient(CommunityService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken,
  });
  let name = '';
  const deadline = Date.now() + 15_000;
  while (Date.now() < deadline) {
    const got = await client.getCommunity({ id: communityId });
    if (got.name.trim() !== '') {
      name = got.name;
      break;
    }
    await sleep(250);
  }
  expect(name, 'per-item community gained a name after promote').not.toBe('');
  return name;
}

test('roster "Name this group" link promotes the event\'s per-item community', async ({
  browser,
  browserName,
}) => {
  test.skip(
    browserName === 'webkit',
    'create + roster + modal UI flow is validated on chromium only',
  );
  const baseUrl = requireBaseUrl();

  // Preconditions: a host who already belongs to a named community ("Trail
  // Crew") with two members — both give the host invitable people AND a
  // populated Communities section in the roster, so the still-nameless per-item
  // community reads as the odd one out.
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'E2E Host' });
  const trailCrew = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Trail Crew',
  });
  for (const name of ['Ada', 'Ben']) {
    await registerUserViaInvite({
      baseUrl,
      specSlug: SPEC_SLUG,
      name,
      inviterAccessToken: host.accessToken,
      communityId: trailCrew.communityId,
    });
  }

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

    // Create the event (→ its host-only per-item community); the share sheet
    // auto-opens. Resolve the per-item community before sharing anything else.
    const { experienceId, perItemCommunityId } = await createEventAndResolve(
      page,
      baseUrl,
      host.accessToken,
    );
    await pause(page);

    // Seed a second member into the per-item (origin) community so it has ≥2
    // members → the "Name this group" nudge unlocks, AND share the event with
    // Trail Crew so its members roll up under a named community in the roster's
    // Communities section. Both via RPC: the in-app "Invite people" / "Invite
    // community" sheets leave the app on a pushed detail screen that mis-renders
    // afterward, so RPC keeps the create→detail flow clean (the detail itself is
    // tappable — see the in-app navigation below).
    await registerUserViaInvite({
      baseUrl,
      specSlug: SPEC_SLUG,
      name: 'Cy',
      inviterAccessToken: host.accessToken,
      communityId: perItemCommunityId,
    });
    const hostComm = createTestClient(CommunityService, {
      baseUrl,
      specSlug: SPEC_SLUG,
      accessToken: host.accessToken,
    });
    await hostComm.shareItem({
      item: { case: 'experienceId', value: experienceId },
      shareToCommunityIds: [trailCrew.communityId],
    });
    await pause(page);

    // Close the share sheet → the app navigates IN-APP to the event detail (no
    // reload). Let it settle, then open the Who's-In roster from the detail.
    await page.getByRole('button', { name: /^close$/i }).click();
    const viewAttendees = page.getByRole('button', { name: /view attendees/i });
    await viewAttendees.waitFor({ timeout: 20_000 });
    await pause(page, 2500);
    await viewAttendees.click();
    await pause(page);

    // The Communities section lists Trail Crew, and the per-item community (now
    // ≥2 members, still nameless) offers the "Name this group" link.
    await expect(page.getByText('Trail Crew').first()).toBeVisible();
    const link = page.locator(
      '[flt-semantics-identifier="roster-name-group-link"]',
    );
    await link.waitFor({ timeout: 10_000 });
    await link.click();

    // Promote the per-item community through the create/AI modal → UpdateCommunity.
    await promoteViaModal(page, GROUP_NAME);

    // The per-item community is now a real, named community.
    const newName = await expectCommunityNamed(
      baseUrl,
      host.accessToken,
      perItemCommunityId,
    );
    await pause(page, 1200);
    // Once named, it's no longer hidden — the now-named per-item community shows
    // in the roster's Communities section (alongside Trail Crew), and its
    // "Name this group" link is gone.
    await expect(page.getByText(newName).first()).toBeVisible();
    await expect(
      page.locator('[flt-semantics-identifier="roster-name-group-link"]'),
    ).toHaveCount(0);
    await pause(page, 1000);
  } finally {
    await ctx.close();
  }
});

test('People tab "Name" chip promotes a nameless community the host owns', async ({
  browser,
  browserName,
}) => {
  test.skip(
    browserName === 'webkit',
    'create + People tab + modal UI flow is validated on chromium only',
  );
  const baseUrl = requireBaseUrl();

  // Preconditions: a host with a named community ("Friends") whose member (Ada)
  // is therefore invitable via "Invite people".
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'E2E Host' });
  const friends = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Friends',
  });
  await registerUserViaInvite({
    baseUrl,
    specSlug: SPEC_SLUG,
    name: 'Ada',
    inviterAccessToken: host.accessToken,
    communityId: friends.communityId,
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

    const { perItemCommunityId } = await createEventAndResolve(
      page,
      baseUrl,
      host.accessToken,
    );
    await pause(page);

    // A host-only nameless community is suppressed from the switcher. Invite Ada
    // (in-app) so it gains a second member; inviting refreshes the community list
    // live (#2492).
    await invitePersonViaShareSheet(page, 'Ada');
    await pause(page);

    // Open the People tab. This flow returns home via the / route (a single
    // reload); the community list refetches on load and the directory shows
    // the now-multi-member per-item community as a nameless group row.
    await page.goto(`${baseUrl}/`);
    const consent = page.getByRole('button', { name: /accept all/i });
    if (await consent.isVisible().catch(() => false)) await consent.click();
    await page.getByRole('button', { name: /^people$/i }).click();
    await pause(page);

    // The nameless group row I own carries the "Name" chip (#2568
    // Decision 5) — its own tap target, sibling of the row's — which opens
    // the create/AI modal in promote mode → UpdateCommunity.
    const nameChip = page.locator(
      '[flt-semantics-identifier="directory-name-group-chip"]',
    );
    await nameChip.waitFor({ timeout: 15_000 });
    await nameChip.click();
    await promoteViaModal(page, GROUP_NAME);

    const newName = await expectCommunityNamed(
      baseUrl,
      host.accessToken,
      perItemCommunityId,
    );
    // Promoting refreshes the membership list, so the directory row now
    // wears the real name and its "Name" chip is gone.
    await expect(page.getByText(newName).first()).toBeVisible({
      timeout: 15_000,
    });
    await expect(
      page.locator('[flt-semantics-identifier="directory-name-group-chip"]'),
    ).toHaveCount(0);
    await pause(page, 1200);
  } finally {
    await ctx.close();
  }
});
