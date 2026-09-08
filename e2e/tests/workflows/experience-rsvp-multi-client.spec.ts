// e2e/tests/workflows/experience-rsvp-multi-client.spec.ts — proof of
// the workflow-aligned multi-client model for #2162 Phase 2.
//
// Maps to: docs/workflows/experience.md §"Workflow Examples" → Example 1
//   ("Experience Completed (Happy Path)"), trimmed to host + 1
//   participant. The canonical server integration test
//   (TestExperience_Example1_HappyPath in
//   server/integration_tests/experience_test.go) covers the full
//   owner + 3-participant + state-machine matrix via RPC. This spec
//   covers what that one cannot: that the production UI gesture
//   path drives the workflow end-to-end across two distinct
//   authenticated browsers.
//
// Core principle this spec encodes
// ────────────────────────────────
// **State changes during the test body happen through UI manipulation
//   on the real Flutter Web bundle.** RPCs are used ONLY for:
//     (a) preparing the world (preconditions) before the test body
//         starts — seeding users, communities, experiences, etc.
//     (b) asserting the final state (postconditions) after the UI
//         gestures complete — reading state / counts / lists.
// If the spec calls an RPC mid-test to manipulate state, it stops
// proving anything the existing `server/integration_tests/` doesn't
// already cover and just adds browser-overhead cost.
//
// What this spec proves
// ─────────────────────
//   1. The multi-client primitive: two Playwright BrowserContexts
//      with independently injected auth land as distinct
//      authenticated sessions against the same hermetic server.
//   2. The production tap-to-RSVP flow lands the workflow step:
//      participant taps the "RSVP — are you in?" call-to-action →
//      ExperienceRsvpComposerSheet opens → selects Going → taps
//      "Send" → ExperienceNotifier.updateRSVP fires RSVPToExperience
//      → server transitions state to JOINED and records the RSVP.
//      No part of this is RPC-faked.
//   3. The server-state contract matches the canonical integration
//      test (Example 1 Step 4): state transitions to JOINED, rsvps[]
//      contains the participant with `intention = RSVP_INTENTION_YES`.
//   4. **Multi-client state convergence.** After the participant's
//      RSVP, the host browser reloads and the host's roster shows
//      the participant in the GOING list — proving GetExperience
//      returns the post-RSVP state to a *second* authenticated
//      client and the redesigned roster renders it.
//      The stronger claim — that the host's ALREADY-OPEN page
//      updates live via `StreamUserEvents` with NO reload — is
//      tracked separately in #2531. The prior "stream-driven" form
//      of this assertion was a false positive: before the Who's-In
//      redesign (#2492) the server rendered every no-reply community
//      member as a static individual roster row, so the participant
//      was visible before any RSVP and the real-time path was never
//      actually exercised (see the POSTCONDITION 2 note below).
//
// What this spec deliberately does NOT prove
// ──────────────────────────────────────────
//   - The share-link handoff flow (`/event/{id}?rsvp=yes&code=…`).
//     That path joins the community late and is currently exposed
//     to the #2237 race (server records the RSVP, but the bundle's
//     initial GetExperience 403'd before the join, so the screen
//     pins on a retry banner). PR #2239 fixes it. A separate spec
//     can cover the share-link path once #2239 lands.
//   - Reading the literal "1 attendee" text from the DOM. Flutter
//     Web with CanvasKit renders text to the canvas; the text
//     isn't in the DOM. The accessibility tree exposes the row's
//     `aria-label`, not the visible count. Server-side
//     `rsvp_yes_count` is the count source of truth.

import { test, expect } from '../../lib/fixtures.js';
import { installRequestIdOverride, createTestClient } from '../../lib/connect.js';
import { injectAuth } from '../../lib/auth.js';
import { registerUser, registerUserViaInvite } from '../../lib/seed/users.js';
import { createCommunity } from '../../lib/seed/communities.js';
import { createExperience } from '../../lib/seed/experiences.js';
import {
  ExperienceService,
  RSVPIntention,
} from '../../gen/ripls/api/experience_service_pb.js';
import { ExperienceState } from '../../gen/ripls/api/experience_pb.js';

const SPEC_SLUG = 'experience-rsvp-multi-client';

test('two members view an event; participant taps RSVP Yes; both browsers reflect the new state', async ({ browser }) => {
  const baseUrl = requireBaseUrl();

  // ── PRECONDITIONS (RPC seed; not part of the test body) ──────────
  // Mirrors workflow.md Example 1 preconditions: community exists,
  // host (owner) and one participant are both members. The
  // participant joins via the canonical invite-link flow
  // (registerUserViaInvite is the analog of registerUserByInvite
  // in server/integration_tests/test_helpers.go).
  const host = await registerUser({
    baseUrl,
    specSlug: SPEC_SLUG,
    name: 'E2E Host',
  });
  const community = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  const participant = await registerUserViaInvite({
    baseUrl,
    specSlug: SPEC_SLUG,
    name: 'E2E Participant',
    inviterAccessToken: host.accessToken,
    communityId: community.communityId,
  });
  const experience = await createExperience({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId: community.communityId,
  });

  // ── TWO BROWSER CONTEXTS ─────────────────────────────────────────
  // The proof's centerpiece. Each context gets its own injected
  // auth state and records video to its own per-user subdirectory
  // (see e2e/README.md § Per-context video recording).
  const hostContext = await browser.newContext({
    recordVideo: { dir: `test-results/videos/${SPEC_SLUG}/host` },
  });
  await injectAuth(hostContext, {
    accessToken: host.accessToken,
    refreshToken: host.refreshToken,
    user: { id: host.userId, name: host.name },
    serverUrl: baseUrl,
  });
  const hostPage = await hostContext.newPage();
  await installRequestIdOverride(hostPage, `${SPEC_SLUG}-host`);

  const participantContext = await browser.newContext({
    recordVideo: { dir: `test-results/videos/${SPEC_SLUG}/participant` },
  });
  await injectAuth(participantContext, {
    accessToken: participant.accessToken,
    refreshToken: participant.refreshToken,
    user: { id: participant.userId, name: participant.name },
    serverUrl: baseUrl,
  });
  const participantPage = await participantContext.newPage();
  await installRequestIdOverride(participantPage, `${SPEC_SLUG}-participant`);

  const consoleLog: string[] = [];
  for (const [label, p] of [
    ['host', hostPage],
    ['participant', participantPage],
  ] as const) {
    p.on('console', (m) => consoleLog.push(`[${label}] ${m.type()}: ${m.text()}`));
    p.on('pageerror', (e) => consoleLog.push(`[${label}] pageerror: ${e.message}`));
  }

  let testFailed = false;
  try {
    // ── BOTH BROWSERS OPEN THE EVENT PAGE ───────────────────────────
    // Both users are pre-existing members, so GetExperience returns
    // 200 in both contexts and the event renders. Anchor identifier
    // (`web-event-screen`) is the reliable "did the screen render"
    // signal; the "View attendees" button is the redesigned read
    // shell's Who's-in roster card, present for both members.
    await Promise.all([
      hostPage.goto(`${baseUrl}/event/${experience.experienceId}`),
      participantPage.goto(`${baseUrl}/event/${experience.experienceId}`),
    ]);
    for (const page of [hostPage, participantPage]) {
      await expect(
        page.locator('[flt-semantics-identifier="web-event-screen"]'),
      ).toBeAttached({ timeout: 20_000 });
      await expect(
        page.getByRole('button', { name: 'View attendees' }),
      ).toBeVisible({ timeout: 15_000 });
    }

    // ── PARTICIPANT RSVPS YES THROUGH THE PRODUCTION UI PATH ────────
    // Workflow Step 4 ("Participant A RSVPs Yes"). In the redesigned
    // read shell (#2278), a participant who has not replied yet sees
    // the "RSVP — are you in?" call-to-action at the foot of the
    // Who's-in card (a11y label "RSVP to this event"). Tapping it
    // opens ExperienceRsvpComposerSheet — the unified composer with
    // Going / Maybe / No chips and a "Send" action that commits the
    // intention. (The old AttendeeListSheet "Can you make it?" toggle
    // flow now lives only in edit mode.)
    await participantPage
      .getByRole('button', { name: 'RSVP to this event' })
      .tap();
    // The composer opens defaulted to Going — its title reads
    // "You're going" — which is the cleanest marker that the modal is
    // open.
    await expect(
      participantPage.getByText("You're going"),
    ).toBeVisible({ timeout: 10_000 });
    // Select Going explicitly (a GlassChip wrapping the Toggle
    // primitive, surfaced as a button via its Semantics wrapper),
    // then Send to commit the YES intention through updateRSVP.
    await participantPage
      .getByRole('button', { name: 'RSVP: Going' })
      .tap();
    await participantPage
      .getByRole('button', { name: 'Send your RSVP' })
      .tap();

    // ── POSTCONDITION 1: Server-side state landed (RPC, not RPC-driven) ─
    // We poll GetExperience until the participant's RSVP is in
    // rsvps[] with intention=YES — that's the server's
    // confirmation that the UI tap reached RSVPToExperience.
    // Same shape as the canonical integration test's Step 4
    // ("Expected JOINED state after first RSVP").
    const hostExpClient = createTestClient(ExperienceService, {
      baseUrl,
      specSlug: SPEC_SLUG,
      accessToken: host.accessToken,
    });
    // Poll until BOTH conditions hold: the participant's RSVP is in
    // rsvps[] AND the experience.state has transitioned to JOINED.
    // The server's RSVPToExperience handler is not transactional —
    // the RSVP Insert (line 158) commits before the state-transition
    // Update (line 187), with conversation/event/system-message
    // work interleaved. A polling loop that breaks on the first
    // condition alone can catch the state mid-transaction (rsvps
    // visible, state still ACTIVE). Waiting on the state — which is
    // the canonical "RSVP fully landed" signal — eliminates that
    // race.
    let resp: Awaited<ReturnType<typeof hostExpClient.getExperience>> | undefined;
    const deadline = Date.now() + 15_000;
    while (Date.now() < deadline) {
      resp = await hostExpClient.getExperience({
        id: experience.experienceId,
        communityId: community.communityId,
      });
      const hasParticipantRsvp = resp.rsvps.some(
        (r) =>
          r.user?.id === participant.userId &&
          r.intention === RSVPIntention.RSVP_INTENTION_YES,
      );
      const stateJoined = resp.experience?.state === ExperienceState.JOINED;
      if (hasParticipantRsvp && stateJoined) break;
      await new Promise((r) => setTimeout(r, 250));
    }
    expect(
      resp?.experience?.state,
      'experience transitions to JOINED on first YES RSVP',
    ).toBe(ExperienceState.JOINED);
    const participantRsvp = resp?.rsvps?.find(
      (r) => r.user?.id === participant.userId,
    );
    expect(
      participantRsvp?.intention,
      'participant appears in rsvps[] with intention=YES',
    ).toBe(RSVPIntention.RSVP_INTENTION_YES);

    // ── POSTCONDITION 2: Host's UI reflects the RSVP after refresh ──
    // The participant is a member of the *named* community the event
    // is shared to — not the origin per-item community. Under the
    // Who's-In redesign (#2492) shared-community members who have not
    // responded are represented by that community's no-reply COUNT,
    // not by an individual roster row; only the origin per-item
    // community's invitees and actual responders get their own rows.
    // So before the RSVP the participant has NO individual row, and
    // after RSVP Yes they become an individual "Going" row.
    //
    // Here we verify the host picks up that post-RSVP server state on
    // a deterministic reload: a fresh GetExperience now returns the
    // participant's YES, and the redesigned roster renders them in
    // GOING. This is on-load correctness — provably guaranteed.
    //
    // Proving the host's ALREADY-OPEN page updates live via
    // `StreamUserEvents` with no reload is tracked in #2531. The
    // client refresh chain is intact (EventRouter → `content` target,
    // `app/lib/services/event_router.dart §154-158` →
    // `contentCacheInvalidationProvider.notify()` →
    // `ExperienceNotifier.scheduleRefresh()`); what #2531 must confirm
    // is the cross-browser stream delivery in the headless web e2e.
    await hostPage.reload();
    await expect(
      hostPage.locator('[flt-semantics-identifier="web-event-screen"]'),
    ).toBeAttached({ timeout: 20_000 });
    await expect(
      hostPage.getByRole('button', { name: 'View attendees' }),
    ).toBeVisible({ timeout: 15_000 });
    await hostPage.getByRole('button', { name: 'View attendees' }).tap();
    // Each roster row exposes two "View <name>'s profile" tap targets — the
    // avatar (accessible name ends in the initial) and the name row (ends in
    // the full name). Target the name row specifically so the assertion is not
    // a strict-mode-ambiguous substring match on both.
    await expect(
      hostPage.getByRole('button', {
        name: "View E2E Participant's profile E2E Participant",
      }),
      'host sees participant in the Going list after reload',
    ).toBeVisible({ timeout: 10_000 });

    // ── POSTCONDITION 3: Participant's own UI reflects their RSVP ────
    // After Send commits the YES intention, the composer closes and
    // the read shell rebuilds from experienceProvider. The viewer now
    // has an intention, so the "RSVP — are you in?" call-to-action is
    // replaced by their status row — i.e. the CTA disappears. Its
    // absence is a direct, unambiguous signal that the bound widget
    // tree picked up the post-send state.
    await expect(
      participantPage.getByRole('button', { name: 'RSVP to this event' }),
      'participant\'s RSVP call-to-action is gone once they are going',
    ).toHaveCount(0, { timeout: 5_000 });
  } catch (err) {
    testFailed = true;
    test.info().attach('console-log', {
      contentType: 'text/plain',
      body: consoleLog.join('\n'),
    });
    throw err;
  } finally {
    // Capture video references before closing — closing the context
    // finalizes the video file, after which Page.video() may return
    // a stale reference depending on Playwright version.
    const hostVideo = hostPage.video();
    const participantVideo = participantPage.video();
    await hostContext.close();
    await participantContext.close();
    // Keep videos on failure for the CI artifact upload (the
    // `test_web_e2e.yaml` workflow uploads e2e/test-results/ on
    // every run, including failures, with 7-day retention). Delete
    // them on pass so the artifact bundle stays small.
    // E2E_KEEP_VIDEOS=1 retains videos even on a passing run, for
    // grabbing demo material.
    if (!testFailed && !process.env.E2E_KEEP_VIDEOS) {
      await hostVideo?.delete().catch(() => {});
      await participantVideo?.delete().catch(() => {});
    }
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
