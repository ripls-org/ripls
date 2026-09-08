// e2e/tests/walkthroughs/events.spec.ts
//
// @walkthrough — the EVENTS WALKTHROUGH (#2684): seven paced scenes showcasing
// the core event journeys, cast and content from the real Mother's Day brunch
// (e2e/fixtures/walkthroughs/brunch/ — consented family photos + a scrubbed
// screenplay in content.json):
//
//   01 Leslie creates the brunch (place + time detected from the prompt) and
//      shares it: copy link, invite Marc + Eric individually, add Austin Fam
//   02 Susan accepts by phone — the SSR landing already shows who's coming
//   03 Leslie plans in context: Marc's RSVP lands live, the needs list posts
//   04 Leslie builds momentum in the event's own chat; replies roll in live
//   05 Lisa pitches in — claims "Brunch dishes", then LENDS her folding table
//      (a new listing, Lend the default) → a real loan to the host for the
//      event's duration (#2708 event child transfers)
//   06 Bryan watches photos + comments stream in from the day, joins in
//   07 Leslie wraps up — confirm who came, then the impact total
//
// One scene = one test = one actor on one recording context = one clip
// (lib/walkthrough.ts); everything else happens off camera via RPC seed
// helpers, including MID-SCENE actions the camera watches land live (RSVPs,
// claims, photo messages). Scenes run serially in one worker, sharing the
// hermetic server + DB, so the world accumulates scene over scene. Render:
//
//   e2e/scripts/run_walkthrough.sh events
//
// This is a SHOWCASE, not a correctness gate — off-camera RPC mid-scene is by
// design here (see e2e/README.md's UI-only rule, which binds workflow specs).
// The correctness versions of these journeys live in tests/workflows/.

import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { test, expect } from '../../lib/fixtures.js';
import { ExperienceService, RSVPIntention } from '../../gen/ripls/api/experience_service_pb.js';
import { ExperienceState } from '../../gen/ripls/api/experience_pb.js';
import { TransferState, TransferType } from '../../gen/ripls/api/transfer_pb.js';
import { UserService } from '../../gen/ripls/api/user_service_pb.js';
import { CommunityService } from '../../gen/ripls/api/community_service_pb.js';
import { createTestClient } from '../../lib/connect.js';
import { registerUser, registerUserViaInvite, type SeededUser } from '../../lib/seed/users.js';
import { createCommunity } from '../../lib/seed/communities.js';
import { createProvisionalUser } from '../../lib/seed/provisional.js';
import { createLocation } from '../../lib/seed/locations.js';
import { setUserAvatar } from '../../lib/seed/attendees.js';
import { uploadMedia } from '../../lib/seed/experiences.js';
import { sendExperienceChatMessage } from '../../lib/seed/chat.js';
import { addExperienceNeed, claimExperienceNeed, addExperienceContribution } from '../../lib/seed/planning.js';
import { emulatorVerificationCode } from '../../lib/ui/phone-register.js';
import { openScene, requireBaseUrl, settle, typeInto, stripEmulatorBanner } from '../../lib/walkthrough.js';
import { setFilmedAt } from '../../lib/filmed-at.js';

const SPEC_SLUG = 'events';

// Unique phone fixture (see onboarding.spec.ts for why these never repeat
// across specs).
const SUSAN_PHONE = '+15551234576';

const BRUNCH = resolve(__dirname, '..', '..', 'fixtures', 'walkthroughs', 'brunch');
const brunchFile = (f: string) => resolve(BRUNCH, f);
// Stock item photos (e.g. the folding table Lisa lends) live in the shared
// stock set one level up — Unsplash-sourced, attributed in ../manifest.json —
// not the private, consent-gated brunch pack.
const stockFile = (f: string) => resolve(BRUNCH, '..', f);

// The screenplay extracted from the real event (scrubbed; see the pack's
// manifest.json). Names, message cadence, and the needs list inform the
// beats below — paraphrased where the real text was chat-logistics noise.
interface BrunchContent {
  event: { name: string; description: string; locality: string };
}
const CONTENT = JSON.parse(readFileSync(brunchFile('content.json'), 'utf-8')) as BrunchContent;

// This reel is filmed on the Thursday before Mother's Day 2026, so the brunch
// falls on the real one (Sunday May 10) and is still three days out on camera.
// Both halves of every scene are held to this moment — see lib/filmed-at.ts.
// Without it the reel could only ever film dates a few days from whenever it
// was rendered: a fixed calendar date drifted into the past ("64d ago" while
// the cast planned), and "this Sunday" lands on whatever Sunday follows the
// render (#2724).
const FILMED_AT = new Date('2026-05-07T16:00:00Z'); // 09:00 America/Los_Angeles
setFilmedAt(FILMED_AT);

// The mock AI provider titles the event from the prompt's first clause and
// extracts the date ("this Sunday" → the Sunday after FILMED_AT) + clock
// ("10:30am"), so the create preview pre-fills WHEN on camera; "at our
// place" leaves the location query empty, which the server resolves to
// Leslie's saved home address.
const EVENT_NAME = "Mother's Day Brunch";
const CREATE_PROMPT =
  "Mother's Day Brunch. At our place this Sunday at 10:30am — the kids are cooking, bring the whole crew!";

// ── World state accumulated across scenes (same worker, same server+DB) ──
let baseUrl: string;
let leslie: SeededUser;
let bryan: SeededUser;
let lisa: SeededUser;
let marc: SeededUser;
let eric: SeededUser;
let austinFamCommunityId: string;
let leslieHomeLocationId: string;
let experienceId: string;
let adhocCommunityId: string;
let shareUrl: string;
let shareCode: string;
const needIds: Record<string, string> = {};

const expClient = (user: SeededUser) =>
  createTestClient(ExperienceService, { baseUrl, specSlug: SPEC_SLUG, accessToken: user.accessToken });

/**
 * A family member "taps Leslie's link": accept the event's share link (joins
 * its ad-hoc community) and RSVP YES there. Seeding into the ad-hoc community
 * keeps this reel's world simple; it is no longer required for correctness —
 * attendance is now resolved event-scoped, so completion tolerates RSVPs in any
 * shared community (#2690). The cross-community completion path is regression-
 * guarded by TestExperience_CrossCommunityCompletion, not by this reel.
 */
async function joinAndRsvpYes(user: SeededUser): Promise<void> {
  try {
    await createTestClient(CommunityService, {
      baseUrl, specSlug: SPEC_SLUG, accessToken: user.accessToken,
    }).acceptInvitationLink({ shortCode: shareCode });
  } catch {
    // Already a member (e.g. added by the on-camera individual invite).
  }
  await expClient(user).rSVPToExperience({
    experienceId, communityId: adhocCommunityId, intention: RSVPIntention.RSVP_INTENTION_YES,
  });
}

async function pollFor<T>(fn: () => Promise<T | undefined>, timeoutMs = 30_000): Promise<T | undefined> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const v = await fn();
    if (v !== undefined) return v;
    await new Promise((r) => setTimeout(r, 500));
  }
  return undefined;
}

const asActor = (user: SeededUser) => ({
  accessToken: user.accessToken,
  refreshToken: user.refreshToken,
  user: { id: user.userId, name: user.name },
  serverUrl: baseUrl,
});

/** Open the event view directly (/experience/:id) — deterministic across
 * cast members, and one boot per scene keeps the clip clean (the head is
 * trimmed at export). */
async function openEvent(page: import('@playwright/test').Page): Promise<void> {
  await page.goto(`${baseUrl}/experience/${experienceId}`);
  await page.getByRole('button', { name: 'View attendees' }).waitFor({ timeout: 30_000 });
  await page.waitForTimeout(1_500); // let the pane settle
}

test.describe.configure({ mode: 'serial' });

test('@walkthrough 01 create and share', async ({ browser }) => {
  test.setTimeout(300_000);
  baseUrl = requireBaseUrl();

  // ---- OFF CAMERA: the cast. Leslie hosts and belongs to THREE
  //      communities (so the share sheet's community picker shows a real
  //      list): "Austin Fam" (Bryan, Lisa — the invited community),
  //      "Alameda Craft Circle" (Marc), "HIIT Moms" (Eric). Marc and Eric
  //      get invited as INDIVIDUALS in scene 1; Susan joins by phone in
  //      scene 2. Avatars are the real (consented) profile photos. ----
  leslie = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Leslie' });
  await setUserAvatar({
    baseUrl, specSlug: SPEC_SLUG, accessToken: leslie.accessToken,
    userId: leslie.userId, avatarPath: brunchFile('avatar-leslie.jpg'),
  });

  // Leslie's home — "at our place" in the create prompt resolves to the
  // host's primary residence (the server's no-location fallback), so the
  // create preview shows a real address with zero geocoding.
  leslieHomeLocationId = await createLocation({
    baseUrl, specSlug: SPEC_SLUG, accessToken: leslie.accessToken,
    name: '1200 Monroe St', addressLines: ['1200 Monroe St'],
    locality: CONTENT.event.locality || 'Austin',
    administrativeArea: 'Texas', regionCode: 'US', postalCode: '78704',
    latitudeDeg: 30.245, longitudeDeg: -97.755,
  });
  await createTestClient(UserService, {
    baseUrl, specSlug: SPEC_SLUG, accessToken: leslie.accessToken,
  }).saveUser({ userId: leslie.userId, primaryResidenceLocationId: leslieHomeLocationId });

  const community = (name: string) =>
    createCommunity({ baseUrl, specSlug: SPEC_SLUG, accessToken: leslie.accessToken, name });
  const austinFam = await community('Austin Fam');
  austinFamCommunityId = austinFam.communityId;
  const craftCircle = await community('Alameda Craft Circle');
  const hiitMoms = await community('HIIT Moms');

  const invite = (communityId: string) =>
    ({ baseUrl, specSlug: SPEC_SLUG, inviterAccessToken: leslie.accessToken, communityId }) as const;
  bryan = await registerUserViaInvite({ ...invite(austinFamCommunityId), name: 'Bryan' });
  lisa = await registerUserViaInvite({ ...invite(austinFamCommunityId), name: 'Lisa' });
  marc = await registerUserViaInvite({ ...invite(craftCircle.communityId), name: 'Marc' });
  eric = await registerUserViaInvite({ ...invite(hiitMoms.communityId), name: 'Eric' });
  await setUserAvatar({
    baseUrl, specSlug: SPEC_SLUG, accessToken: bryan.accessToken,
    userId: bryan.userId, avatarPath: brunchFile('avatar-bryan.jpg'),
  });
  await setUserAvatar({
    baseUrl, specSlug: SPEC_SLUG, accessToken: lisa.accessToken,
    userId: lisa.userId, avatarPath: brunchFile('avatar-lisa.jpg'),
  });
  await setUserAvatar({
    baseUrl, specSlug: SPEC_SLUG, accessToken: marc.accessToken,
    userId: marc.userId, avatarPath: brunchFile('avatar-marc.png'),
  });

  // ---- ON CAMERA: Leslie creates the brunch and shares it. ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-01`, actor: asActor(leslie) });
  const { page } = scene;
  try {
    await page.goto(`${baseUrl}/`);
    // Wait for a rendered Home before the first settle — the first settle
    // sets the export's head-trim point, and goto resolves long before the
    // Flutter bundle paints. The semantics tree lands before the canvas
    // paints on skwasm, so give pixels a beat too (#2724).
    await page.getByRole('button', { name: /^create$/i }).waitFor({ timeout: 30_000 });
    await page.waitForTimeout(1_200);
    await settle(page, 'Home');

    // Unified create: prompt → generate → save (mock AI provider). The
    // preview pre-fills WHEN (next Sunday, 10:30 AM — extracted from the
    // prompt) and WHERE ("at our place" → Leslie's saved home address).
    await page.getByRole('button', { name: /^create$/i }).click();
    await page.getByRole('button', { name: /^text$/i }).click();
    await settle(page, 'Create — text tab');
    await typeInto(page, CREATE_PROMPT);
    await settle(page, 'Create — prompt typed');
    await page.getByRole('button', { name: /^draft it$/i }).click();
    const saveBtn = page.getByRole('button', { name: /^save event$/i });
    await saveBtn.waitFor({ timeout: 30_000 });
    await settle(page, 'Create — smart preview');
    await settle(page, 'Create — place and time detected');

    // Leslie picks the brunch photo ON CAMERA (item-creation upload is
    // web-enabled, #2157) — the event is born with its hero, so the view
    // behind the share sheet is never a black placeholder. The pencil reads
    // "Add a photo" until one exists (#2724); it opens an Add Media
    // sheet, and "Photos" there opens the OS picker.
    await page.getByRole('button', { name: /add a photo/i }).click();
    const photosBtn = page.getByRole('button', { name: /^photos$/i });
    await photosBtn.waitFor({ timeout: 20_000 });
    const chooser = page.waitForEvent('filechooser', { timeout: 20_000 });
    await photosBtn.click();
    await (await chooser).setFiles(brunchFile('event.jpg'));
    await settle(page, 'Brunch photo picked');

    await saveBtn.click();

    // The share sheet auto-opens (two-phase create → share, #2492).
    await page.getByRole('button', { name: /copy link/i }).waitFor({ timeout: 20_000 });

    // OFF CAMERA, while the sheet covers the screen: discover the created
    // event and re-assert the on-camera values (time, place, the picked
    // photo) with a tidied description, so the reel stays deterministic
    // even if a detection beat drifts.
    const mine = await expClient(leslie).listMyExperiences({});
    const created = mine.experiences.find((e) => e.name === EVENT_NAME);
    if (!created?.id) throw new Error(`UI-created event "${EVENT_NAME}" not found via ListMyExperiences`);
    experienceId = created.id;
    const share = await createTestClient(CommunityService, {
      baseUrl, specSlug: SPEC_SLUG, accessToken: leslie.accessToken,
    }).shareItem({ item: { case: 'experienceId', value: experienceId } });
    if (!share.adhocCommunityId || !share.shareUrl) {
      throw new Error('ShareItem did not return adhoc community + share url');
    }
    adhocCommunityId = share.adhocCommunityId;
    shareUrl = share.shareUrl;
    shareCode = shareUrl.split('/').pop() ?? '';
    if (!shareCode) throw new Error(`no short code in share url ${shareUrl}`);
    const cur = await expClient(leslie).getExperience({ id: experienceId, communityId: adhocCommunityId });
    // Pin ONLY the location ("our place" → Leslie's home). Name, description,
    // and time keep the values the provider generated on camera — re-asserting
    // them used to mutate the quote card between scenes and log a no-op
    // "updated time →" line into the fresh chat (#2724).
    await expClient(leslie).saveExperience({
      id: experienceId,
      // Keep the hero Leslie picked on camera (an empty list would strip it).
      mediaIds: cur.experience?.mediaIds ?? [],
      locationId: leslieHomeLocationId,
      maxParticipants: 0,
    });
    await settle(page, 'Share sheet');

    // Copy the link for the text thread FIRST — confirming a community share
    // dismisses the whole sheet ("Shared to communities" toast).
    await page.getByRole('button', { name: /copy link/i }).click();
    await settle(page, 'Link copied');

    // Invite Marc and Eric individually (the member picker searches all of
    // Leslie's communities). Confirming DISMISSES the whole share sheet.
    await page.getByRole('button', { name: /invite people/i }).click();
    await settle(page, 'Invite people');
    await page.getByRole('button', { name: 'Marc' }).first().click();
    await page.getByRole('button', { name: 'Eric' }).first().click();
    await settle(page, 'Marc and Eric picked');
    await page.getByRole('button', { name: /^invite \d+$/i }).click();
    await settle(page, 'People invited');

    // Reopen the share surface from the roster ("Who's In" → Invite) and add
    // the Austin Fam community (picker lists all three of Leslie's
    // communities). Confirming dismisses the sheet with the share toast.
    await page.getByRole('button', { name: 'View attendees' }).click();
    await settle(page, 'Roster — invited');
    await page.getByRole('button', { name: /invite people to the event/i }).click();
    await page.getByRole('button', { name: /share to communities/i }).click();
    await settle(page, 'Pick a community');
    await page.getByText('Austin Fam').first().click();
    await settle(page, 'Austin Fam selected');
    await page.getByRole('button', { name: /^confirm$/i }).click();
    await settle(page, 'Shared with the fam');
  } finally {
    await scene.close();
  }

  // ---- OFF CAMERA: pre-invite Susan by phone, and let Bryan + Lisa RSVP
  //      before Susan opens the link — the SSR landing then shows who's
  //      already coming, not just the host. ----
  await createProvisionalUser({
    baseUrl, specSlug: SPEC_SLUG, accessToken: leslie.accessToken,
    communityId: adhocCommunityId, name: 'Susan', phoneNumber: SUSAN_PHONE,
  });
  await joinAndRsvpYes(bryan);
  await joinAndRsvpYes(lisa);
});

test('@walkthrough 02 join by phone', async ({ browser }) => {
  test.setTimeout(300_000);

  // ---- ON CAMERA: Susan taps the link from her text thread — no app, no
  //      account — and lands RSVP'd via phone OTP (epic #2492). ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-02` });
  const { page } = scene;
  try {
    // Navigate by short code against the LOCAL server — shareUrl's display
    // host is ripls.app (E2E_INVITE_LINK_HOSTNAME) for camera realism.
    await page.goto(`${baseUrl}/go/${shareCode}`);
    await page.getByRole('link', { name: /rsvp yes/i }).waitFor({ timeout: 20_000 });
    await settle(page, 'SSR invite landing');
    await settle(page, "Who's already coming");
    await page.getByRole('link', { name: /rsvp yes/i }).click();

    await page.getByRole('button', { name: /send code/i }).waitFor({ timeout: 30_000 });
    await settle(page, 'Confirm your phone');
    await typeInto(page, SUSAN_PHONE);
    await settle(page, 'Phone filled');
    await page.getByRole('button', { name: /send code/i }).click();

    await page.getByRole('button', { name: /^verify$/i }).waitFor({ timeout: 20_000 });
    const code = await emulatorVerificationCode(SUSAN_PHONE);
    await typeInto(page, code);
    await settle(page, 'OTP filled');
    await page.getByRole('button', { name: /^verify$/i }).click();

    await page.getByRole('button', { name: /finish rsvp/i }).waitFor({ timeout: 20_000 });
    await typeInto(page, 'Susan');
    await settle(page, 'Name filled');
    await page.getByRole('button', { name: /finish rsvp/i }).click();

    // Susan's promoted-account RSVP lands server-side.
    let susanIn = false;
    const deadline = Date.now() + 30_000;
    while (Date.now() < deadline) {
      const resp = await expClient(leslie).getExperience({ id: experienceId, communityId: adhocCommunityId });
      susanIn = resp.rsvps.some(
        (r) => r.user?.name === 'Susan' && r.intention === RSVPIntention.RSVP_INTENTION_YES,
      );
      if (susanIn) break;
      await new Promise((r) => setTimeout(r, 500));
    }
    expect(susanIn, "Susan RSVP'd YES after phone registration").toBe(true);

    // The server having the RSVP is not the scene: Susan's OWN row is. Wait
    // for the guest's view to stop offering her the RSVP she just made,
    // rather than filming whatever frame the poll happened to land on
    // (#2727). The chip's accessible name is its a11y label, NOT its visible
    // "RSVP — are you in?" text.
    await expect(
      page.getByRole('button', { name: /rsvp to this event/i }),
    ).toHaveCount(0, { timeout: 20_000 });
    await settle(page, "Susan's in");
  } finally {
    await scene.close();
  }
});

test('@walkthrough 03 plan the day', async ({ browser }) => {
  test.setTimeout(300_000);

  // ---- ON CAMERA: Leslie posts what's needed and rallies the group. ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-03`, actor: asActor(leslie) });
  const { page } = scene;
  try {
    await openEvent(page);
    await settle(page, 'Event view');

    await page.getByRole('button', { name: 'View attendees' }).click();
    await settle(page, "Who's In");

    // OFF CAMERA, mid-scene: Marc's RSVP lands while Leslie is looking at
    // the roster (Bryan + Lisa said yes before Susan's scene; Marc joining
    // now also makes his scene-4 chat reply a participant's).
    await joinAndRsvpYes(marc);
    await settle(page, 'RSVPs landing');

    // Post the needs list ("We need" → the "What would help?" compose
    // sheet). The dashed custom row commits on Enter (onSubmitted); the
    // publish bar reads "Share list · N pieces" once the list is non-empty.
    await page.getByRole('button', { name: /add something the event needs/i }).click();
    await settle(page, 'What would help');
    for (const need of ['Brunch dishes', 'Dessert', 'Craft supplies for mom crowns', 'Folding table']) {
      await typeInto(page, need);
      await page.keyboard.press('Enter');
      await page.waitForTimeout(400); // commit + field reset between pieces
    }
    await settle(page, 'Needs listed');
    await page.getByRole('button', { name: /share list/i }).click();
    await settle(page, 'Needs posted');
    await settle(page, "Who's In — with needs");
  } finally {
    await scene.close();
  }

  // OFF CAMERA: record the need ids for the claim scene.
  const needs = await expClient(leslie).listExperienceNeedsAndContributions({ experienceId });
  for (const n of needs.needs ?? []) needIds[n.name] = n.id;
});

test('@walkthrough 04 rally the crew', async ({ browser }) => {
  test.setTimeout(300_000);

  // ---- ON CAMERA: Leslie tells the group what's up in the event chat. ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-04`, actor: asActor(leslie) });
  const { page } = scene;
  try {
    await openEvent(page);
    await page.getByRole('button', { name: /open conversation/i }).click();
    await settle(page, 'Event chat');
    await typeInto(
      page,
      'Silly idea: sibling pairs craft a crown for their mom, and she has to wear it 👑',
    );
    await settle(page, 'Message typed');
    await page.getByRole('button', { name: /send message/i }).click();
    await settle(page, 'Message sent');

    // OFF CAMERA, mid-scene: the idea lands — replies stream into Leslie's
    // open chat (real reactions from the actual thread, lightly trimmed).
    await sendExperienceChatMessage({
      baseUrl, specSlug: SPEC_SLUG, accessToken: lisa.accessToken,
      experienceId, text: 'That’s a super cute idea 🥰',
    });
    await settle(page, 'Lisa replies');
    // "…excitement to host" read as a guest claiming to host (#2724); one
    // settle here — the doubled dwell filmed ~8s of identical frames.
    await sendExperienceChatMessage({
      baseUrl, specSlug: SPEC_SLUG, accessToken: marc.accessToken,
      experienceId, text: 'M&S are just “Ripling” with excitement — count us in!!',
    });
    await settle(page, 'The idea catches on');
  } finally {
    await scene.close();
  }
});

test('@walkthrough 05 friends pitch in', async ({ browser }) => {
  test.setTimeout(300_000);

  // ---- ON CAMERA: Lisa claims "Brunch dishes". ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-05`, actor: asActor(lisa) });
  const { page } = scene;
  try {
    await openEvent(page);
    await page.getByRole('button', { name: 'View attendees' }).click();
    await settle(page, "Who's In — still needed");

    // Tapping the need chip opens the claim sheet directly.
    await page.getByText('Brunch dishes').first().click();
    await settle(page, 'Claim sheet');
    await typeInto(page, 'Pasta salad and a fruit plate, from the kids');
    await settle(page, 'Claim note');
    await page.getByRole('button', { name: /i.?ll bring this/i }).click();
    await settle(page, 'Claimed');

    // ---- The #2708 beat: Lisa also LENDS her folding table. Claiming the
    //      "Folding table" need, she snaps a new listing and lends it FOR THE
    //      BRUNCH — the claim escalates into a real loan to the host for the
    //      event's duration (it completes when the event does; no return
    //      reminder — she takes it home after). Same claim sheet + inline
    //      create + Lend/Give choice the request flow uses (#2702). ----
    await page.getByText('Folding table').first().click();
    await page.getByRole('button', { name: /link an item/i }).waitFor({ timeout: 20_000 });
    await settle(page, 'Bring the folding table — with what, exactly?');
    await page.getByRole('button', { name: /link an item/i }).click();
    // The dashed "Add a new item" row (full unified create); its merged
    // semantics label is the stable locator on Flutter Web.
    const addItemRow = page
      .getByRole('button', { name: /add a new item to your library/i })
      .first();
    await addItemRow.waitFor({ timeout: 20_000 });
    await settle(page, 'Her folding table, one photo away');
    await addItemRow.click();
    const galleryBtn = page.getByRole('button', { name: /open gallery/i });
    await galleryBtn.waitFor({ timeout: 20_000 });
    // Arm the chooser immediately before the click that opens it — armed
    // any earlier, the intervening waits burn its timeout.
    const gearChooser = page.waitForEvent('filechooser', { timeout: 20_000 });
    await galleryBtn.click();
    await (await gearChooser).setFiles(stockFile('folding-table.jpg'));
    // The photo becomes a listing (deterministic e2e detection by filename);
    // saving auto-links it back onto the claim.
    const saveGearBtn = page.getByRole('button', { name: /^save$/i });
    await saveGearBtn.waitFor({ timeout: 30_000 });
    await settle(page, 'Photo became a listing');
    await saveGearBtn.click();
    // Back on the claim sheet: gear linked, Lend/Give visible with lend the
    // default — it comes home after the brunch.
    const confirmTable = page.getByRole('button', { name: /i.?ll bring this/i });
    await confirmTable.waitFor({ timeout: 30_000 });
    await settle(page, 'Lend it for the day');
    await confirmTable.click();
    await settle(page, 'A real loan, sent to the host');

    // OFF CAMERA, mid-scene: the rest of the family piles on while Lisa
    // watches the list fill in. Eric joins+RSVPs first — the claim RPC
    // requires a participant (the in-app flow auto-RSVPs; the raw RPC
    // doesn't). Marc already joined in scene 3.
    await joinAndRsvpYes(eric);
    if (needIds['Dessert']) {
      await claimExperienceNeed({
        baseUrl, specSlug: SPEC_SLUG, accessToken: eric.accessToken,
        experienceId, needId: needIds['Dessert'], note: 'Annie and Paul are both making cake',
      });
    }
    await addExperienceContribution({
      baseUrl, specSlug: SPEC_SLUG, accessToken: marc.accessToken,
      experienceId, title: 'Mimosas!',
    });
    await settle(page, 'Others pitch in');
    await settle(page, 'Plan coming together');
  } finally {
    await scene.close();
  }

  // Server-side truth (#2708): the folding-table claim escalated into a live
  // loan to the host for the event's duration (a silent escalation failure
  // would render the beat wrong instead of failing the reel).
  const lentTable = await pollFor(async () => {
    const resp = await expClient(leslie).listExperienceNeedsAndContributions({ experienceId });
    const c = resp.contributions.find(
      (contrib) => contrib.transferId !== '' && contrib.transferState === TransferState.RECIPIENT_SELECTED,
    );
    return c ?? undefined;
  });
  expect(lentTable, 'the folding-table claim escalated into a live loan').toBeTruthy();
  expect(lentTable!.transferType, 'brought as a loan (comes home after)').toBe(
    TransferType.LOAN,
  );
});

test('@walkthrough 06 photos from the day', async ({ browser }) => {
  test.setTimeout(300_000);

  // OFF CAMERA: the senders' photos (real brunch shots) are uploaded under
  // their own accounts so chat attribution is right.
  const upload = (user: SeededUser, file: string) =>
    uploadMedia({
      baseUrl, specSlug: SPEC_SLUG, accessToken: user.accessToken,
      path: brunchFile(file), contentType: file.endsWith('.png') ? 'image/png' : 'image/jpeg',
      filename: file,
    });
  const kidsCooking = await upload(leslie, 'event-4.jpg');
  const table = await upload(eric, 'event-2.jpg');
  const moms = await upload(leslie, 'event-5.jpg');
  const crowns = await upload(leslie, 'event-6.jpg');

  // ---- ON CAMERA: Bryan opens the event chat as the photos roll in. ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-06`, actor: asActor(bryan) });
  const { page } = scene;
  try {
    await openEvent(page);
    await page.getByRole('button', { name: /open conversation/i }).click();
    await settle(page, 'Chat — day of');

    const send = (user: SeededUser, text: string, mediaIds?: string[]) =>
      sendExperienceChatMessage({
        baseUrl, specSlug: SPEC_SLUG, accessToken: user.accessToken,
        experienceId, text, mediaIds,
      });

    await send(leslie, 'Kids are ON IT 🧑‍🍳', [kidsCooking]);
    await settle(page, 'First photo lands');
    await send(eric, 'From Lyla and Miles: pasta salad, fruit plate, veggies and dip!', [table]);
    await settle(page, 'Second photo lands');

    await typeInto(page, 'Best Mother’s Day yet 🥰');
    await page.getByRole('button', { name: /send message/i }).click();
    await settle(page, 'Bryan joins in');

    await send(leslie, 'So much love in one kitchen 💛', [moms, crowns]);
    await settle(page, 'More photos land');
    await settle(page, 'The day in photos');
  } finally {
    await scene.close();
  }
});

// NOTE: keep this title short — a longer one middle-hashes the output dir
// name and breaks the export script's lexicographic scene ordering.
test('@walkthrough 07 wrap up', async ({ browser }) => {
  test.setTimeout(300_000);

  // ---- ON CAMERA: Leslie completes the brunch; the completion sheet
  //      live-previews the impact (quality time with the whole crew). ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-07`, actor: asActor(leslie) });
  const { page } = scene;
  try {
    await openEvent(page);
    await settle(page, 'Event view — after');

    // The manage sheet opens from the event pane's edit affordance
    // (a11yExpOpenManageSheet = 'Edit event').
    await page.getByRole('button', { name: /^edit event/i }).first().click();
    await settle(page, 'Manage menu');
    // "Wrap up" — the manage row's verb since #2724 (was "Mark Completed";
    // the sheet's own eyebrow now matches it).
    await page.getByRole('button', { name: /^wrap up$/i }).first().click();

    // The completion sheet: attendee confirm chips + the live impact
    // preview (PreviewExperienceImpact fills the stat tiles).
    await page.getByRole('button', { name: /^complete$/i }).waitFor({ timeout: 20_000 });
    await settle(page, 'Completion — impact preview');
    await settle(page, 'Completion — reading the numbers');
    await page.getByRole('button', { name: /^complete$/i }).click();

    // "Crafting your story…" → completed.
    await page.waitForTimeout(2_000);
    await settle(page, 'Completed');

    // Closing beat: the roster, with everyone who came confirmed. Turning
    // this circle into a standing group is a real affordance here
    // ("Name this group…", #2492) but it belongs to a recurring rhythm, not
    // to a once-a-year Mother's Day — the runclub reel is where that lands.
    await page.getByRole('button', { name: 'View attendees' }).click();
    await settle(page, 'Everyone who came');
  } finally {
    await scene.close();
  }

  // Server-side truth: the experience is COMPLETED (a silent completion
  // failure would render the closing beats wrong instead of failing the
  // reel).
  const resp = await expClient(leslie).getExperience({ id: experienceId, communityId: adhocCommunityId });
  expect(resp.experience?.state, 'experience completed').toBe(ExperienceState.COMPLETED);
});
