// e2e/tests/walkthroughs/onboarding.spec.ts
//
// @walkthrough — a PACED, video-generating baseline of the app-less,
// phone-first guest onboarding (epic #2492) that doubles as our first
// walkthrough clip. This is NOT a correctness gate (the
// phone-rsvp-full-loop spec is); it exists so we can WATCH the whole
// new-user signup as one continuous video and iterate on the look + steps.
//
// A single-scene reel (#2684): it runs ONLY under the `walkthrough`
// Playwright project (video:'on', chromium, dark color scheme, tall 2× phone
// frame, pinned Pacific tz), which is the only project whose grep matches
// @walkthrough. The standard mobile-chromium/webkit projects grepInvert it out,
// so the deliberate per-screen pauses never slow the normal suite / CI gate.
//
//   Run it HERMETICALLY (provisions a throwaway Postgres testcontainer, runs,
//   exports deliverables, reaps the container — expects nothing, leaves
//   nothing). Build the server once first, then:
//     npm run build:web:e2e && go build -o tmp/server ./server   # once
//     e2e/scripts/run_walkthrough.sh onboarding
//   Deliverables land under e2e/videos-walkthroughs/onboarding/.
//
// What's ON camera: only the GUEST. The host + a fully-populated event
// (real venue, evening time, hero photo, a roster of attendees with
// avatars) + the share link + the phone-keyed provisional invitee are all
// seeded OFF camera via RPC so the video is purely the new user's
// experience — receive a link, view a real-looking event, sign up with a
// phone number, and land RSVP'd.
//
// Pacing + scene plumbing come from lib/walkthrough.ts (settle/typeInto/
// stripEmulatorBanner; knobs WALKTHROUGH_PAUSE_MS, WALKTHROUGH_TYPE_DELAY_MS).

import { test, expect } from '../../lib/fixtures.js';
import { resolve } from 'node:path';
import { ExperienceService, RSVPIntention } from '../../gen/ripls/api/experience_service_pb.js';
import { createTestClient, installRequestIdOverride } from '../../lib/connect.js';
import { registerUser } from '../../lib/seed/users.js';
import { mintExperienceShareLink } from '../../lib/seed/communities.js';
import { createProvisionalUser } from '../../lib/seed/provisional.js';
import { createLocation } from '../../lib/seed/locations.js';
import { seedAttendee, setUserAvatar, type RSVP } from '../../lib/seed/attendees.js';
import { uploadMedia } from '../../lib/seed/experiences.js';
import { emulatorVerificationCode } from '../../lib/ui/phone-register.js';
import {
  armCameraTrim,
  preseedObservabilityConsent,
  requireBaseUrl,
  settle,
  stripEmulatorBanner,
  typeInto,
} from '../../lib/walkthrough.js';

const SPEC_SLUG = 'walkthrough-onboarding';
// Fixed number — the run is idempotent because the DB is reinitialized each
// time (scripts/run_walkthrough.sh provisions a throwaway DB per run, so
// the server boots a clean schema). Without that reset a persistent local DB
// would carry a prior run's promoted phone account and surface "an account
// already exists" on the create-account screen.
// Distinct from every other spec's phone fixture: with parallel workers all
// sharing one Auth Emulator, two specs registering the same number concurrently
// would collide in the emulator's auth store. (This @walkthrough spec isn't in the
// CI projects, but keeping every fixture unique is the rule.)
const GUEST_PHONE = '+15551234575';
const GUEST_NAME = 'Alex';
const EVENT_TZ = 'America/Los_Angeles'; // must match the project's timezoneId

const FIXTURES = resolve(__dirname, '..', '..', 'fixtures', 'walkthroughs');
const HERO = resolve(FIXTURES, 'hero-rooftop.jpg');
const avatar = (n: number) => resolve(FIXTURES, `avatar-${n}.jpg`);

const EVENT_NAME = 'Sunset Rooftop Gathering';
const EVENT_DESCRIPTION =
  'Drinks and good company as the sun goes down. Come hang out on the roof!';

// A believable evening-event roster (off-camera). Avatars 1–6 are the
// attendees; avatar 7 is the host. A realistic spread of intentions: mostly
// yes, one maybe, two no.
const ATTENDEES: { name: string; avatar: number; rsvp: RSVP }[] = [
  { name: 'Maya', avatar: 1, rsvp: 'yes' },
  { name: 'Jordan', avatar: 2, rsvp: 'yes' },
  { name: 'Priya', avatar: 3, rsvp: 'yes' },
  { name: 'Diego', avatar: 4, rsvp: 'maybe' },
  { name: 'Sam', avatar: 5, rsvp: 'no' },
  { name: 'Naomi', avatar: 6, rsvp: 'no' },
];

/**
 * Next Friday at 6:30 PM Pacific, as Unix seconds. The SSR landing renders
 * in the event's stored timezone and the browser context is pinned to the
 * same tz, so both show "6:30 PM". (June is always PDT/UTC-7; the +7 offset
 * holds for the near-future dates this fixture ever targets.)
 */
function nextFridayEveningPacificUnixSec(): number {
  const laParts = (d: Date) =>
    Object.fromEntries(
      new Intl.DateTimeFormat('en-CA', {
        timeZone: EVENT_TZ,
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        weekday: 'short',
      })
        .formatToParts(d)
        .map((p) => [p.type, p.value]),
    ) as { year: string; month: string; day: string; weekday: string };
  const DAY_MS = 86_400_000;
  for (let i = 1; i <= 7; i++) {
    const p = laParts(new Date(Date.now() + i * DAY_MS));
    if (p.weekday === 'Fri') {
      // 18:30 PDT → +7h = UTC. Date.UTC overflows the hour cleanly into the
      // next calendar day.
      return Math.floor(Date.UTC(+p.year, +p.month - 1, +p.day, 18 + 7, 30, 0) / 1000);
    }
  }
  throw new Error('no upcoming Friday found');
}

// Scene titles: "@walkthrough NN <short-ascii-slug>" — the title becomes the
// output dir + scene filename, so keep it short (long titles get middle-hashed
// by Playwright) and ASCII-only.
test('@walkthrough 01 phone onboarding', async ({
  page,
  browserName,
}) => {
  test.skip(browserName !== 'chromium', 'onboarding video is chromium-only');
  const baseUrl = requireBaseUrl();

  // Camera prep (see lib/walkthrough.ts): no emulator banner or consent modal
  // on film, and mark recording start so the export trims the boot footage.
  armCameraTrim(page);
  await stripEmulatorBanner(page);
  await preseedObservabilityConsent(page);

  // ---- OFF CAMERA: seed the host (with avatar), a real venue, the event
  //      (evening time + hero photo), the share link, a roster of attendees
  //      with avatars + RSVPs, and the phone-keyed provisional invitee. ----
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Riley Chen' });
  await setUserAvatar({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    userId: host.userId,
    avatarPath: avatar(7),
  });

  const locationId = await createLocation({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Skyline Rooftop & Lounge',
    addressLines: ['44 Tehama St'],
    locality: 'San Francisco',
    administrativeArea: 'California',
    regionCode: 'US',
    postalCode: '94105',
    latitudeDeg: 37.7881,
    longitudeDeg: -122.3972,
  });

  const heroMediaId = await uploadMedia({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    path: HERO,
    contentType: 'image/jpeg',
    filename: 'hero-rooftop.jpg',
  });

  const hostExp = createTestClient(ExperienceService, {
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  const saved = await hostExp.saveExperience({
    name: EVENT_NAME,
    description: EVENT_DESCRIPTION,
    mediaIds: [heroMediaId],
    locationId,
    maxParticipants: 0,
    time: {
      timeType: {
        case: 'specific' as const,
        value: {
          unixTimestampSec: BigInt(nextFridayEveningPacificUnixSec()),
          timezone: EVENT_TZ,
        },
      },
    },
  });
  const experienceId = saved.experience?.id;
  const communityId = saved.itemCommunityId; // the event's per-item ad-hoc community (#2492)
  if (!experienceId || !communityId) {
    throw new Error('SaveExperience did not return an experience id + per-item community id');
  }

  const shareLink = await mintExperienceShareLink({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId,
    experienceId,
  });

  // Roster — registered members with avatars + RSVPs (off camera).
  for (const a of ATTENDEES) {
    await seedAttendee({
      baseUrl,
      specSlug: SPEC_SLUG,
      inviterAccessToken: host.accessToken,
      communityId,
      experienceId,
      name: a.name,
      avatarPath: avatar(a.avatar),
      rsvp: a.rsvp,
    });
  }

  // Pre-invite the guest by phone → a provisional placeholder that
  // promote-on-verify claims when they verify the same number.
  await createProvisionalUser({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId,
    name: GUEST_NAME,
    phoneNumber: GUEST_PHONE,
  });

  await installRequestIdOverride(page, SPEC_SLUG);

  // ---- ON CAMERA: the guest's app-less onboarding ----

  // Screen 1: the SSR invite landing (real HTML the Go server serves) — what
  // you see when you tap the link in a text message. No app, no account.
  await page.goto(`${baseUrl}/go/${shareLink.shortCode}`);
  await page.getByRole('link', { name: /rsvp yes/i }).waitFor({ timeout: 20_000 });
  await settle(page, 'SSR invite landing (/go/{code})');

  // Tap "I'm in" → hands off into the Flutter web app, which routes the guest
  // STRAIGHT to the phone-first "Confirm your phone to RSVP" screen (the old
  // "Sign in to RSVP" gate + "You're Invited" register screen are gone, #2492).
  await page.getByRole('link', { name: /rsvp yes/i }).click();

  // Screen 2: phone-first entry ("Confirm your phone to RSVP") → Send Code.
  await page.getByRole('button', { name: /send code/i }).waitFor({ timeout: 30_000 });
  await settle(page, 'Confirm your phone to RSVP — empty');
  await typeInto(page, GUEST_PHONE);
  await settle(page, 'Phone entry — number filled');
  await page.getByRole('button', { name: /send code/i }).click();

  // Screen 3: enter the OTP → Verify.
  await page.getByRole('button', { name: /^verify$/i }).waitFor({ timeout: 20_000 });
  await settle(page, 'OTP entry — empty');
  const code = await emulatorVerificationCode(GUEST_PHONE);
  await typeInto(page, code);
  await settle(page, 'OTP entry — code filled');
  await page.getByRole('button', { name: /^verify$/i }).click();

  // Screen 4: "Phone Verified" → enter name → Finish RSVP (fires PhoneRegister,
  // which promotes the provisional invitee into a real account + auto-RSVPs).
  await page.getByRole('button', { name: /finish rsvp/i }).waitFor({ timeout: 20_000 });
  await settle(page, 'Phone Verified — name entry');
  await typeInto(page, GUEST_NAME);
  await settle(page, 'Name entry — filled');
  await page.getByRole('button', { name: /finish rsvp/i }).click();

  // ---- Assert the RSVP landed server-side (the auto-fire handoff) ----
  // Verify the GUEST's auto-fired YES specifically landed — matched by name,
  // not by total count (the host's own auto-RSVP would satisfy a count check on
  // its own, masking a guest RSVP that never landed).
  let guestRsvped = false;
  const deadline = Date.now() + 30_000;
  while (Date.now() < deadline) {
    const resp = await hostExp.getExperience({ id: experienceId, communityId });
    guestRsvped = resp.rsvps.some(
      (r) => r.user?.name === GUEST_NAME && r.intention === RSVPIntention.RSVP_INTENTION_YES,
    );
    if (guestRsvped) break;
    await new Promise((r) => setTimeout(r, 500));
  }
  expect(guestRsvped, `guest "${GUEST_NAME}" RSVP'd YES after phone-OTP registration`).toBe(true);

  // Screen 5: hold on the landed, signed-in event view so the video ends on
  // the "you're in" state.
  await settle(page, 'Signed in — RSVP confirmed');

  const videoPath = await page.video()?.path();
  if (videoPath) {
    // eslint-disable-next-line no-console
    console.log(`[walkthrough] onboarding video → ${videoPath}`);
  }
});
