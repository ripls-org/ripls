// e2e/tests/walkthroughs/runclub.spec.ts
//
// @walkthrough — the RECURRING GROUP walkthrough (#2684): seven paced scenes
// on the journey an event does NOT end with — a Wednesday morning run that
// turns into a standing group and books its own next meetup:
//
//   01 Nadia posts "Wednesday morning run" and copies the link to text out
//   02 Marcus opens that link — no app, no account — and is in by phone
//   03 By Tuesday evening six people have joined off the same link
//   04 Wednesday, 6:30am: the run has happened and its photos are in the
//      thread; Priya reads them and adds a note
//   05 Nadia wraps it up from the card foot — confirm who came, read the total
//   06 The People tab's nameless group becomes "Hump Day Milers", and she
//      copies an invite link for the GROUP rather than for any one run
//   07 Home offers to schedule the run again; the draft opens on next
//      Wednesday, same hour, same crew
//
// TIME MOVES IN THIS REEL. Unlike every earlier one it does not film a single
// instant: each scene re-declares `setFilmedAt` (lib/filmed-at.ts reads it
// lazily, so both the seed RPCs and the scene's page get the new moment), so
// the reel runs Monday → Tuesday → Wednesday. A recurring group is a story
// about elapsed time; compressed into one morning, the host would be filmed
// creating a run that had already happened.
//
// The same knob keeps the chat honest. A scene's page clock is FIXED, so
// everything sent inside one scene shares a timestamp, and chat history breaks
// those ties by message id — a UUID — which is how an on-camera reply once
// rendered *between* two photos sent before it. Anything that must read as
// earlier is therefore seeded at an earlier `setFilmedAt` than the scene it
// appears in.
//
// One scene = one test = one actor on one recording context = one clip
// (lib/walkthrough.ts); everything else happens off camera via RPC seed
// helpers. Scenes run serially in one worker, sharing the hermetic server +
// DB, so the world accumulates scene over scene. Render:
//
//   e2e/scripts/run_walkthrough.sh runclub
//
// This is a SHOWCASE, not a correctness gate — off-camera RPC mid-scene is by
// design here (see e2e/README.md's UI-only rule, which binds workflow specs).
// The correctness versions of these journeys live in tests/workflows/; the
// promote-by-naming beat in scene 06 has its twin in
// tests/workflows/community-promote-nudge.spec.ts.

import { resolve } from 'node:path';
import { test, expect, type Page } from '../../lib/fixtures.js';
import { ExperienceService, RSVPIntention } from '../../gen/ripls/api/experience_service_pb.js';
import { ExperienceState } from '../../gen/ripls/api/experience_pb.js';
import { UserService } from '../../gen/ripls/api/user_service_pb.js';
import { CommunityService } from '../../gen/ripls/api/community_service_pb.js';
import { WorkshopService } from '../../gen/ripls/api/workshop_service_pb.js';
import { PortfolioService } from '../../gen/ripls/api/portfolio_pb.js';
import { createTestClient } from '../../lib/connect.js';
import { registerUser, type SeededUser } from '../../lib/seed/users.js';
import { createLocation } from '../../lib/seed/locations.js';
import { setUserAvatar } from '../../lib/seed/attendees.js';
import { uploadMedia } from '../../lib/seed/experiences.js';
import { sendExperienceChatMessage } from '../../lib/seed/chat.js';
import { emulatorVerificationCode } from '../../lib/ui/phone-register.js';
import { openScene, requireBaseUrl, settle, typeInto } from '../../lib/walkthrough.js';
import { setFilmedAt } from '../../lib/filmed-at.js';

const SPEC_SLUG = 'runclub';

// Unique phone fixture (see onboarding.spec.ts for why these never repeat
// across specs).
const MARCUS_PHONE = '+15551234592';

// This reel's content is fabricated — no prod extraction, no consent-gated
// pack. Copy lives in fixtures/walkthroughs/runclub/walkthrough.md; the
// imagery is the shared Unsplash stock set, attributed in ../manifest.json.
const STOCK = resolve(__dirname, '..', '..', 'fixtures', 'walkthroughs');
const stockFile = (f: string) => resolve(STOCK, f);

const EVENT_NAME = 'Wednesday morning run';
const GROUP_NAME = 'Hump Day Milers';

// The mock AI provider titles the event from the prompt's first clause, and
// resolves the bare weekday to its NEXT occurrence after the filmed moment
// (e2eExtractWeekday) plus the clock — so filming scene 01 on a Monday puts
// the run on that same week's Wednesday. Keep gear/request cue words
// ("borrow", "lend", "need", "looking for") out of the prompt or
// classifyE2EText misroutes the create flow away from an event.
const CREATE_PROMPT =
  'Wednesday morning run. We meet at 6:30am at the Laurelhurst Park gate ' +
  'for an easy three miles — coffee after for whoever can stay.';

// ── The week this reel is filmed over ────────────────────────────────────
// America/Los_Angeles (what the capture emulation pins), August 2026 = PDT.
// Aug 10 is a Monday, Aug 12 the Wednesday it points at, Aug 19 the Wednesday
// the repeat draft should land on.
const MON_MORNING = new Date('2026-08-10T16:00:00Z'); // Mon 09:00
const MON_EVENING = new Date('2026-08-11T02:30:00Z'); // Mon 19:30
const TUE_MIDDAY = new Date('2026-08-11T19:00:00Z'); // Tue 12:00
const TUE_EVENING = new Date('2026-08-12T01:00:00Z'); // Tue 18:00
/** The run itself, and the photos from it. */
const WED_RUN_PHOTO = new Date('2026-08-12T14:22:00Z'); // Wed 07:22
const WED_COFFEE_PHOTO = new Date('2026-08-12T14:31:00Z'); // Wed 07:31
const WED_AFTER_RUN = new Date('2026-08-12T14:45:00Z'); // Wed 07:45
const WED_MID_MORNING = new Date('2026-08-12T16:30:00Z'); // Wed 09:30
const WED_LATER = new Date('2026-08-12T16:45:00Z'); // Wed 09:45
/** What "Schedule the next one" has to propose: Wed Aug 19, 6:30am PDT. */
const NEXT_WEDNESDAY_UNIX_SEC = Math.floor(
  new Date('2026-08-19T13:30:00Z').getTime() / 1000,
);

// ── World state accumulated across scenes (same worker, same server+DB) ──
let baseUrl: string;
let nadia: SeededUser;
/** One of the six who followed the link — the eyes for the day-of scene. */
let priya: SeededUser;
let parkLocationId: string;
let experienceId: string;
let adhocCommunityId: string;
let shareCode: string;

const expClient = (user: SeededUser) =>
  createTestClient(ExperienceService, {
    baseUrl, specSlug: SPEC_SLUG, accessToken: user.accessToken,
  });

const commClient = (user: SeededUser) =>
  createTestClient(CommunityService, {
    baseUrl, specSlug: SPEC_SLUG, accessToken: user.accessToken,
  });

const asActor = (user: SeededUser) => ({
  accessToken: user.accessToken,
  refreshToken: user.refreshToken,
  user: { id: user.userId, name: user.name },
  serverUrl: baseUrl,
});

/**
 * A runner "taps Nadia's link": register, accept the event's share link (which
 * joins its ad-hoc community — the same membership the SSR landing grants),
 * take a portrait, and RSVP YES. This is the whole of how this group grows —
 * nobody is invited by hand, which is what makes scene 03's roster read as
 * "they all followed the same link".
 */
async function joinsViaLink(name: string, avatar: string): Promise<SeededUser> {
  const user = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name });
  await commClient(user).acceptInvitationLink({ shortCode: shareCode });
  await setUserAvatar({
    baseUrl, specSlug: SPEC_SLUG, accessToken: user.accessToken,
    userId: user.userId, avatarPath: stockFile(avatar),
  });
  await expClient(user).rSVPToExperience({
    experienceId,
    communityId: adhocCommunityId,
    intention: RSVPIntention.RSVP_INTENTION_YES,
  });
  return user;
}

async function pollFor<T>(
  fn: () => Promise<T | undefined>,
  timeoutMs = 30_000,
): Promise<T | undefined> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const v = await fn();
    if (v !== undefined) return v;
    await new Promise((r) => setTimeout(r, 500));
  }
  return undefined;
}

/** Open the event view directly (/experience/:id) — deterministic across
 * cast members, and one boot per scene keeps the clip clean (the head is
 * trimmed at export). */
async function openEvent(page: Page): Promise<void> {
  await page.goto(`${baseUrl}/experience/${experienceId}`);
  await page.getByRole('button', { name: 'View attendees' }).waitFor({ timeout: 30_000 });
  await page.waitForTimeout(1_500); // let the pane settle
}

test.describe.configure({ mode: 'serial' });

test('@walkthrough 01 post the run', async ({ browser }) => {
  test.setTimeout(300_000);
  baseUrl = requireBaseUrl();
  setFilmedAt(MON_MORNING);

  // ---- OFF CAMERA: Nadia, and the gate she runs from. The e2e provider
  //      returns no location query, so the server falls back to the host's
  //      primary residence — parking the park there is what puts a real
  //      meeting point on the create preview with zero geocoding. ----
  nadia = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Nadia' });
  await setUserAvatar({
    baseUrl, specSlug: SPEC_SLUG, accessToken: nadia.accessToken,
    userId: nadia.userId, avatarPath: stockFile('avatar-9.jpg'),
  });
  parkLocationId = await createLocation({
    baseUrl, specSlug: SPEC_SLUG, accessToken: nadia.accessToken,
    name: 'Laurelhurst Park — SE Oak St gate',
    addressLines: ['SE Oak St & SE 39th Ave'],
    locality: 'Portland', administrativeArea: 'Oregon',
    regionCode: 'US', postalCode: '97214',
    latitudeDeg: 45.5266, longitudeDeg: -122.6255,
  });
  await createTestClient(UserService, {
    baseUrl, specSlug: SPEC_SLUG, accessToken: nadia.accessToken,
  }).saveUser({ userId: nadia.userId, primaryResidenceLocationId: parkLocationId });

  // ---- ON CAMERA: Nadia posts the run and copies its link. ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-01`, actor: asActor(nadia) });
  const { page } = scene;
  try {
    await page.goto(`${baseUrl}/`);
    // Wait for a rendered Home before the first settle — the first settle
    // sets the export's head-trim point, and goto resolves long before the
    // Flutter bundle paints (#2724).
    await page.getByRole('button', { name: /^create$/i }).waitFor({ timeout: 30_000 });
    await page.waitForTimeout(1_200);
    await settle(page, 'Home');

    await page.getByRole('button', { name: /^create$/i }).click();
    await page.getByRole('button', { name: /^text$/i }).click();
    await settle(page, 'Create — text tab');
    await typeInto(page, CREATE_PROMPT);
    await settle(page, 'Create — prompt typed');
    await page.getByRole('button', { name: /^draft it$/i }).click();
    const saveBtn = page.getByRole('button', { name: /^save event$/i });
    await saveBtn.waitFor({ timeout: 30_000 });
    await settle(page, 'Create — smart preview');
    await settle(page, 'Wednesday 6:30am, at the gate');

    // Nadia picks the photo ON CAMERA (item-creation upload is web-enabled,
    // #2157), so the run is born with its hero and the view behind the share
    // sheet is never a black placeholder.
    await page.getByRole('button', { name: /add a photo/i }).click();
    const photosBtn = page.getByRole('button', { name: /^photos$/i });
    await photosBtn.waitFor({ timeout: 20_000 });
    const chooser = page.waitForEvent('filechooser', { timeout: 20_000 });
    await photosBtn.click();
    await (await chooser).setFiles(stockFile('hero-morning-run.jpg'));
    await settle(page, 'Photo picked');

    await saveBtn.click();

    // The share sheet auto-opens (two-phase create → share, #2492).
    await page.getByRole('button', { name: /copy link/i }).waitFor({ timeout: 20_000 });

    // OFF CAMERA, while the sheet covers the screen: discover the created
    // event and mint its share link. Pin ONLY the location — name,
    // description and time keep the values the provider generated on camera
    // (re-asserting them mutates the card between scenes, #2724).
    const mine = await expClient(nadia).listMyExperiences({});
    const created = mine.experiences.find((e) => e.name === EVENT_NAME);
    if (!created?.id) {
      throw new Error(`UI-created event "${EVENT_NAME}" not found via ListMyExperiences`);
    }
    experienceId = created.id;
    const share = await commClient(nadia).shareItem({
      item: { case: 'experienceId', value: experienceId },
    });
    if (!share.adhocCommunityId || !share.shareUrl) {
      throw new Error('ShareItem did not return adhoc community + share url');
    }
    adhocCommunityId = share.adhocCommunityId;
    shareCode = share.shareUrl.split('/').pop() ?? '';
    if (!shareCode) throw new Error(`no short code in share url ${share.shareUrl}`);
    const cur = await expClient(nadia).getExperience({
      id: experienceId, communityId: adhocCommunityId,
    });
    await expClient(nadia).saveExperience({
      id: experienceId,
      // Keep the hero Nadia picked on camera (an empty list would strip it).
      mediaIds: cur.experience?.mediaIds ?? [],
      locationId: parkLocationId,
      maxParticipants: 0,
    });
    await settle(page, 'Share sheet — the link and its code');

    await page.getByRole('button', { name: /copy link/i }).click();
    await settle(page, 'Link copied, ready to text');
  } finally {
    await scene.close();
  }
});

test('@walkthrough 02 join by phone', async ({ browser }) => {
  test.setTimeout(300_000);
  setFilmedAt(MON_EVENING);

  // ---- ON CAMERA: Marcus taps the link from the neighbourhood thread — no
  //      app, no account — and is in by phone (epic #2492). ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-02` });
  const { page } = scene;
  try {
    // Navigate by short code against the LOCAL server — shareUrl's display
    // host is ripls.app (E2E_INVITE_LINK_HOSTNAME) for camera realism.
    await page.goto(`${baseUrl}/go/${shareCode}`);
    await page.getByRole('link', { name: /rsvp yes/i }).waitFor({ timeout: 20_000 });
    await settle(page, 'The link opens to the run');
    await page.getByRole('link', { name: /rsvp yes/i }).click();

    await page.getByRole('button', { name: /send code/i }).waitFor({ timeout: 30_000 });
    await settle(page, 'Confirm your phone');
    await typeInto(page, MARCUS_PHONE);
    await settle(page, 'Phone filled');
    await page.getByRole('button', { name: /send code/i }).click();

    await page.getByRole('button', { name: /^verify$/i }).waitFor({ timeout: 20_000 });
    const code = await emulatorVerificationCode(MARCUS_PHONE);
    await typeInto(page, code);
    await settle(page, 'OTP filled');
    await page.getByRole('button', { name: /^verify$/i }).click();

    await page.getByRole('button', { name: /finish rsvp/i }).waitFor({ timeout: 20_000 });
    await typeInto(page, 'Marcus');
    await settle(page, 'Name filled');
    await page.getByRole('button', { name: /finish rsvp/i }).click();

    // Marcus's promoted-account RSVP lands server-side.
    const landed = await pollFor(async () => {
      const resp = await expClient(nadia).getExperience({
        id: experienceId, communityId: adhocCommunityId,
      });
      return resp.rsvps.some(
        (r) => r.user?.name === 'Marcus' && r.intention === RSVPIntention.RSVP_INTENTION_YES,
      ) || undefined;
    });
    expect(landed, "Marcus RSVP'd YES after phone registration").toBe(true);

    // The server having the RSVP is not the scene: Marcus's OWN row is. Wait
    // for his view to stop offering the RSVP he just made rather than filming
    // whatever frame the poll landed on (#2727). The chip's accessible name
    // is its a11y label, NOT its visible "RSVP — are you in?" text.
    await expect(
      page.getByRole('button', { name: /rsvp to this event/i }),
    ).toHaveCount(0, { timeout: 20_000 });
    await settle(page, "Marcus is in — no app, no account");
  } finally {
    await scene.close();
  }

  // Marcus keeps his initials for the rest of the reel, deliberately: he
  // registered IN THE BROWSER, so his tokens never passed through this
  // process (on web the bundle keeps them in flutter_secure_storage, not the
  // plain localStorage keys lib/auth.ts injects), and SaveUser is self-only
  // so not even the host can set a portrait for him. It reads true anyway —
  // someone who joined by phone sixty seconds ago hasn't picked a photo.
});

test('@walkthrough 03 who is coming', async ({ browser }) => {
  test.setTimeout(300_000);

  // ---- OFF CAMERA: five more taps on the link, trickling in across Monday
  //      night and Tuesday the way a forwarded link actually spreads. Each
  //      join is stamped at the moment it happened (setFilmedAt), so the
  //      roster Nadia opens on Tuesday evening is a record of two days, not
  //      of five things that happened while she watched. ----
  setFilmedAt(MON_EVENING);
  priya = await joinsViaLink('Priya', 'avatar-11.jpg');
  await joinsViaLink('Theo', 'avatar-12.jpg');
  setFilmedAt(TUE_MIDDAY);
  await joinsViaLink('Jules', 'avatar-10.jpg');
  await joinsViaLink('Dev', 'avatar-1.jpg');
  await joinsViaLink('Alma', 'avatar-2.jpg');

  // ---- ON CAMERA: Tuesday evening, Nadia checks who's coming. ----
  setFilmedAt(TUE_EVENING);
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-03`, actor: asActor(nadia) });
  const { page } = scene;
  try {
    await openEvent(page);
    await settle(page, 'Tuesday evening');
    await page.getByRole('button', { name: 'View attendees' }).click();
    await settle(page, "Who's In");
    await settle(page, 'Six runners, off one link');
  } finally {
    await scene.close();
  }

  // Server-side truth: six people said yes on top of the host. A silent join
  // failure would render a half-empty roster instead of failing the reel.
  const resp = await expClient(nadia).getExperience({
    id: experienceId, communityId: adhocCommunityId,
  });
  const yes = resp.rsvps.filter(
    (r) => r.intention === RSVPIntention.RSVP_INTENTION_YES && r.user?.id !== nadia.userId,
  );
  expect(yes.length, 'six runners RSVPd yes via the link').toBe(6);
});

test('@walkthrough 04 the run happens', async ({ browser }) => {
  test.setTimeout(300_000);

  // ---- OFF CAMERA: the run, and Nadia's two photos from it. Uploaded and
  //      sent under her own account so the chat attribution is right, and
  //      stamped at the minutes they were taken so Priya's on-camera reply
  //      lands strictly after both (see the header note on tie-breaking).
  //      Chat photo authoring is blocked on Flutter Web
  //      (web_unsupported.dart), so photo beats are always filmed from the
  //      RECEIVING side with the sends seeded. ----
  setFilmedAt(WED_RUN_PHOTO);
  const groupShot = await uploadMedia({
    baseUrl, specSlug: SPEC_SLUG, accessToken: nadia.accessToken,
    path: stockFile('run-group-photo.jpg'), contentType: 'image/jpeg',
    filename: 'run-group-photo.jpg',
  });
  await sendExperienceChatMessage({
    baseUrl, specSlug: SPEC_SLUG, accessToken: nadia.accessToken,
    experienceId, text: 'Out before the sun cleared the trees 🌅',
    mediaIds: [groupShot],
  });

  setFilmedAt(WED_COFFEE_PHOTO);
  const coffee = await uploadMedia({
    baseUrl, specSlug: SPEC_SLUG, accessToken: nadia.accessToken,
    path: stockFile('run-post-run-coffee.jpg'), contentType: 'image/jpeg',
    filename: 'run-post-run-coffee.jpg',
  });
  await sendExperienceChatMessage({
    baseUrl, specSlug: SPEC_SLUG, accessToken: nadia.accessToken,
    experienceId, text: 'Coffee earned ☕️', mediaIds: [coffee],
  });

  // ---- ON CAMERA: Priya, back at her desk after the run, reads the thread
  //      and adds to it. Her reply is the newest message, so it lands at the
  //      bottom on her own side of the conversation. ----
  setFilmedAt(WED_AFTER_RUN);
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-04`, actor: asActor(priya) });
  const { page } = scene;
  try {
    await openEvent(page);
    await page.getByRole('button', { name: /open conversation/i }).click();
    await settle(page, 'The morning, in the run’s own thread');
    await settle(page, 'Coffee after, as promised');

    await typeInto(page, 'Best Wednesday I have had in a while');
    await settle(page, 'Priya replies');
    await page.getByRole('button', { name: /send message/i }).click();
    await settle(page, 'Everyone who was there is already in it');
  } finally {
    await scene.close();
  }
});

test('@walkthrough 05 wrap it up', async ({ browser }) => {
  test.setTimeout(300_000);
  setFilmedAt(WED_AFTER_RUN);

  // ---- ON CAMERA: Nadia closes the run out. Confirming who came is what
  //      makes the crew a set the next draft can re-invite. ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-05`, actor: asActor(nadia) });
  const { page } = scene;
  try {
    await openEvent(page);
    await settle(page, 'The run, after');

    // "Wrap up" sits at the foot of the Who's-in card from the moment the
    // event's start time passes — it used to be a row inside the ⋯ manage
    // sheet, which is a lot of hiding for the host's only remaining move.
    const wrapUp = page
      .getByRole('button', { name: /wrap up this event/i })
      .first();
    await wrapUp.waitFor({ timeout: 20_000 });
    await settle(page, 'Wrap up, right on the card');
    await wrapUp.click();

    // The completion sheet: every YES RSVP arrives pre-confirmed (tapping a
    // face REMOVES them), and the impact preview fills live.
    await page.getByRole('button', { name: /^complete$/i }).waitFor({ timeout: 20_000 });
    await settle(page, 'Confirm who came');
    await settle(page, 'The time the group spent together');
    await page.getByRole('button', { name: /^complete$/i }).click();

    // "Crafting your story…" → completed.
    await page.waitForTimeout(2_000);
    await settle(page, 'Wrapped');
  } finally {
    await scene.close();
  }

  // Server-side truth: COMPLETED, and the crew is recorded as having attended
  // — the draft in scene 07 re-invites exactly that set, so a silent failure
  // here would empty the closing beat instead of failing the reel.
  const resp = await expClient(nadia).getExperience({
    id: experienceId, communityId: adhocCommunityId,
  });
  expect(resp.experience?.state, 'experience completed').toBe(ExperienceState.COMPLETED);
});

test('@walkthrough 06 name the group', async ({ browser }) => {
  test.setTimeout(300_000);
  setFilmedAt(WED_MID_MORNING);

  // ---- ON CAMERA: the People tab. The seven of them have been a group all
  //      week without a name; naming it (#2492 DISPLAY-1) is what turns this
  //      run's audience into something the next run can be posted to. ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-06`, actor: asActor(nadia) });
  const { page } = scene;
  try {
    await page.goto(`${baseUrl}/`);
    await page.getByRole('button', { name: /^people$/i }).waitFor({ timeout: 30_000 });
    await page.waitForTimeout(1_200);
    await settle(page, 'Home');
    await page.getByRole('button', { name: /^people$/i }).click();
    await settle(page, 'People — a group with no name');

    // The nameless group row the viewer OWNS carries a sibling "Name" chip
    // (#2568 Decision 5); the row itself still just opens the group.
    const nameChip = page.locator('[flt-semantics-identifier="directory-name-group-chip"]');
    await nameChip.waitFor({ timeout: 15_000 });
    await nameChip.click();

    // Promote mode opens straight to name + description — no AI generate step.
    const field = page.getByRole('textbox', { name: 'Community name', exact: true });
    await field.waitFor({ timeout: 20_000 });
    await settle(page, 'Give them a name');
    await typeInto(page, GROUP_NAME, { field });
    await settle(page, 'Hump Day Milers');
    // The confirm button's label ("Name this group") collides with the roster
    // link that can open this modal, so target its stable semantics id.
    const confirm = page.locator('[flt-semantics-identifier="community-preview-confirm"]');
    await confirm.waitFor({ timeout: 30_000 });
    await confirm.click();

    // OFF CAMERA, while the modal closes: give the new group the run's own
    // photo, so the group page Nadia is about to open is not a blank slate.
    const named = await pollFor(async () => {
      const got = await commClient(nadia).getCommunity({ id: adhocCommunityId });
      return got.name.trim() !== '' ? got : undefined;
    });
    expect(named?.name, 'the group gained a name').toBe(GROUP_NAME);
    const heroId = await uploadMedia({
      baseUrl, specSlug: SPEC_SLUG, accessToken: nadia.accessToken,
      path: stockFile('hero-morning-run.jpg'), contentType: 'image/jpeg',
      filename: 'hump-day-milers.jpg',
    });
    await commClient(nadia).updateCommunity({
      id: adhocCommunityId,
      name: named!.name,
      description: named!.description,
      mediaIds: [heroId],
    });
    await settle(page, 'A real group now');

    // Into the group itself, and the beat this reel exists for: inviting the
    // next runner to the GROUP, not to any one run.
    await page.getByText(GROUP_NAME).first().click();
    await page.getByRole('button', { name: /^invite$/i }).waitFor({ timeout: 30_000 });
    await settle(page, 'Hump Day Milers');
    await page.getByRole('button', { name: /^invite$/i }).click();
    await page.getByRole('button', { name: /copy link/i }).waitFor({ timeout: 20_000 });
    await settle(page, 'Invite to the group, not to one run');
    await page.getByRole('button', { name: /copy link/i }).click();
    await settle(page, 'Copied');
  } finally {
    await scene.close();
  }
});

// NOTE: keep this title short — a longer one middle-hashes the output dir
// name and breaks the export script's lexicographic scene ordering.
test('@walkthrough 07 next wednesday', async ({ browser }) => {
  test.setTimeout(300_000);
  setFilmedAt(WED_LATER);

  // ---- OFF CAMERA: the momentum engine writes the "schedule it again"
  //      prompt for the run Nadia just wrapped. Materialization runs in a
  //      detached goroutine off GetWorkshopBrief, so ask for it and then wait
  //      for the prompt to actually reach the home view rather than filming
  //      whatever the first load happened to have. ----
  await createTestClient(WorkshopService, {
    baseUrl, specSlug: SPEC_SLUG, accessToken: nadia.accessToken,
  }).getWorkshopBrief({ communityIds: [adhocCommunityId] });
  const homeNudge = await pollFor(async () => {
    const home = await createTestClient(PortfolioService, {
      baseUrl, specSlug: SPEC_SLUG, accessToken: nadia.accessToken,
    }).getHomeView({ timezone: 'America/Los_Angeles' });
    return home.nudge?.ctaAction === 'schedule_repeat' ? home.nudge : undefined;
  }, 60_000);
  expect(homeNudge, 'the home view offers to schedule the run again').toBeTruthy();
  expect(homeNudge!.contextId, 'the prompt names the run it would repeat')
    .toBe(experienceId);

  // ---- ON CAMERA: Nadia is back on Home, having left the run behind to name
  //      the group. Home offers her the next one. ----
  const scene = await openScene(browser, { slug: `${SPEC_SLUG}-07`, actor: asActor(nadia) });
  const { page } = scene;
  try {
    await page.goto(`${baseUrl}/`);
    // The card's headline names the run ("Schedule Wednesday morning run
    // again"); its button is the short lever. That the prompt on screen is
    // THIS run's is already pinned above, against the home view itself —
    // a stronger check than reading canvas-rendered text back.
    const scheduleNext = page
      .getByRole('button', { name: /^schedule it$/i })
      .first();
    await scheduleNext.waitFor({ timeout: 30_000 });
    await page.waitForTimeout(1_200);
    await settle(page, 'Home');
    await settle(page, 'Ripls offers to run it again');
    await scheduleNext.click();

    // GenerateWorkshopDraft hands back the whole event: name, description,
    // the same photo and gate, next Wednesday at the same hour, and the crew
    // who were confirmed as having come.
    const createBtn = page.getByRole('button', { name: /^create event$/i });
    await createBtn.waitFor({ timeout: 30_000 });
    await settle(page, 'Same route, same hour, same crew');
    await settle(page, 'Nadia checks it over');
    await createBtn.click();
    await page.waitForTimeout(2_500);
    await settle(page, 'Next Wednesday is posted');
  } finally {
    await scene.close();
  }

  // Server-side truth: a SECOND instance exists, a week on from the first at
  // the same hour. Filming a preview that quietly proposed the wrong week —
  // or nothing at all — is exactly the failure this reel is meant to catch.
  const next = await pollFor(async () => {
    const mine = await expClient(nadia).listMyExperiences({});
    return mine.experiences.find((e) => e.id !== experienceId) ?? undefined;
  });
  expect(next, 'the repeat draft was published as a second event').toBeTruthy();
  expect(next!.name, 'the next one carries the same name').toBe(EVENT_NAME);
  const startUnixSec = Number(
    next!.time?.timeType.case === 'specific'
      ? next!.time.timeType.value.unixTimestampSec
      : 0,
  );
  expect(startUnixSec, 'next instance lands on the following Wednesday, same hour')
    .toBe(NEXT_WEDNESDAY_UNIX_SEC);
});
