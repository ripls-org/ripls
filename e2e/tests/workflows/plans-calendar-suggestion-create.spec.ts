// e2e/tests/workflows/plans-calendar-suggestion-create.spec.ts
//
// Maps to: docs/client/calendar.md → "Weather and open-day suggestions" +
// the suggestion→create flow (blank_create_dispatcher.openSuggestedCreate).
//
// What this spec proves (the UI-integration failure class RPC tests can't):
//   - The Plans tab (bottom-dock calendar destination) renders a seeded event
//     on its own day, AND on a different, event-free day it renders an
//     open-day *suggestion* built from the viewer's past-activity history.
//   - Tapping the suggestion's "Plan it with them" CTA carries THAT calendar
//     day into unified-create as a STRUCTURAL date seed (unified_create_modal
//     _seedEventTime, marked user-edited so the AI stream can't overwrite it),
//     so the created event lands on exactly the day the user started from —
//     not on "today", and not on whatever date the AI prompt text parses to.
//     (Only the suggestion / alternative CTAs seed the date; the generic
//     "Suggest a plan" / "something else" paths do not — see calendar.md.)
//
// What this spec does NOT prove:
//   - Server suggestion math (the ≥2-occurrence eligibility, embedding
//     clustering, weather fit) — that's server/services/portfolio and its unit
//     tests. Here it's only a precondition, seeded by history + mock weather.
//   - The AI generation content itself — the deterministic e2e provider
//     (--mock-ai-provider) stands in for the model.
//
// Divergences from a workflow.md example: none — this is a Plans-calendar
// regression spec, not a 1:1 workflow port. Preconditions are seeded via RPC;
// every state change in the test body is a real UI gesture on the Flutter Web
// bundle; the postcondition is read back over RPC.

import { test, expect } from '../../lib/fixtures.js';
import { createTestClient, installRequestIdOverride } from '../../lib/connect.js';
import { newRecordingContext } from '../../lib/context.js';
import { injectAuth } from '../../lib/auth.js';
import { dismissDataConsent } from '../../lib/ui/consent.js';
import { registerUser } from '../../lib/seed/users.js';
import { createCommunity } from '../../lib/seed/communities.js';
import { createExperience } from '../../lib/seed/experiences.js';
import { createLocation } from '../../lib/seed/locations.js';
import { PortfolioService, HomeUpNextKind } from '../../gen/ripls/api/portfolio_pb.js';

const SPEC_SLUG = 'plans-calendar-suggestion-create';

// A fixed IANA zone pinned on the browser context AND used for every date
// computation + the postcondition GetHomeView, so the app's local-day bucketing
// (chips, cells, suggestion placement) and the test's seeded days can't drift
// apart across timezones — the classic calendar-test flake.
const TZ = 'America/Los_Angeles';

// A distinctly event-shaped, recurring activity name (no gear/request cue words,
// so the deterministic classifier routes it to EVENT — server/ai/provider_e2e.go
// classifyE2EText). Seeded twice in the past to clear the ≥2 activity bar and
// produce an open-day suggestion.
const ACTIVITY_NAME = 'Sunset Rooftop Social';
// The pre-existing future event, on a different day, distinct name.
const EXISTING_EVENT_NAME = 'Neighborhood Craft Circle';

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set; global-setup.ts populates it');
  return url;
}

// ── Local-day helpers (all in TZ) ────────────────────────────────────────────

interface LaDay {
  y: number;
  m: number; // 1-12
  d: number;
  wd: number; // JS weekday: Sun=0 … Sat=6
}

const JS_WEEKDAY: Record<string, number> = {
  Sun: 0, Mon: 1, Tue: 2, Wed: 3, Thu: 4, Fri: 5, Sat: 6,
};

/** The TZ-local calendar parts of an instant. */
function laDayOf(instant: Date): LaDay {
  const parts = Object.fromEntries(
    new Intl.DateTimeFormat('en-US', {
      timeZone: TZ,
      year: 'numeric',
      month: 'numeric',
      day: 'numeric',
      weekday: 'short',
    })
      .formatToParts(instant)
      .map((p) => [p.type, p.value]),
  );
  return {
    y: Number(parts.year),
    m: Number(parts.month),
    d: Number(parts.day),
    wd: JS_WEEKDAY[parts.weekday],
  };
}

/**
 * Unix seconds for ~mid-day (20:00 UTC = 12:00 PST / 13:00 PDT) on a TZ
 * calendar date — safely inside the day for either offset, so the TZ-local day
 * is exactly y/m/d regardless of DST.
 */
function laMiddayUnix(y: number, m: number, d: number): number {
  return Math.floor(Date.UTC(y, m - 1, d, 20, 0, 0) / 1000);
}

/** The TZ day `deltaDays` away from `base` (advancing a mid-day instant). */
function laDayShifted(base: LaDay, deltaDays: number): LaDay {
  const anchor = laMiddayUnix(base.y, base.m, base.d) + deltaDays * 86_400;
  return laDayOf(new Date(anchor * 1000));
}

/** The cell's accessibility-name date prefix: DateFormat('EEEE, MMMM d'). */
function cellDateLabel(day: LaDay): string {
  return new Intl.DateTimeFormat('en-US', {
    timeZone: TZ,
    weekday: 'long',
    month: 'long',
    day: 'numeric',
  }).format(new Date(laMiddayUnix(day.y, day.m, day.d) * 1000));
}

function sameLaDay(a: LaDay, b: LaDay): boolean {
  return a.y === b.y && a.m === b.m && a.d === b.d;
}

/** Absolute month distance of `target` from `from` (…, -1, 0, 1, …). */
function monthOffset(from: LaDay, target: LaDay): number {
  return (target.y - from.y) * 12 + (target.m - from.m);
}

test('Plans tab: existing event shows, suggestion-day create lands on that day', async ({
  browser,
  browserName,
}) => {
  test.skip(
    browserName === 'webkit',
    'Plans calendar + unified-create modal validated on chromium only (sibling: community-calendar-shared-event)',
  );
  const baseUrl = requireBaseUrl();

  // ── DATES (all TZ-local) ─────────────────────────────────────────────────
  const today = laDayOf(new Date());
  // Coming Saturday — mirrors PlansDateSheet._thisWeekend and stays within the
  // 8-nearest-open-days suggestion cap and the 46-day forecast window. When
  // today is already the weekend, it's today (still an open day).
  const dartWeekday = today.wd === 0 ? 7 : today.wd; // Mon=1…Sun=7
  const daysToWeekend = dartWeekday >= 6 ? 0 : 6 - dartWeekday;
  const suggestionDay = laDayShifted(today, daysToWeekend);
  // The 15th of next month — held by the pre-existing event so it is "busy" (no
  // suggestion there). Mid-month + a month out, so it can never coincide with
  // the coming-Saturday suggestion day (which is always ≤6 days away).
  const existingDay: LaDay =
    today.m === 12
      ? laDayOf(new Date(laMiddayUnix(today.y + 1, 1, 15) * 1000))
      : laDayOf(new Date(laMiddayUnix(today.y, today.m + 1, 15) * 1000));

  // ── PRECONDITIONS (RPC seed) ─────────────────────────────────────────────
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG, name: 'Priya Host' });
  const community = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Rosewood Neighbors',
  });

  // A place with coordinates on the events, so the home view can resolve a
  // viewer location → fetch (mock) weather → emit open-day suggestions. Without
  // it resolveViewerPlace fails and every suggestion is suppressed.
  const locationId = await createLocation({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Skyline Rooftop & Lounge',
    addressLines: ['44 Tehama St'],
    locality: 'San Francisco',
    administrativeArea: 'CA',
    regionCode: 'US',
    postalCode: '94105',
    latitudeDeg: 37.789,
    longitudeDeg: -122.396,
  });

  // Two past occurrences of the SAME activity → clears minActivityOccurrences
  // (=2) so the calendar offers an open-day suggestion for it (identical names
  // also survive the exact-name fallback if embeddings aren't ready).
  for (const daysAgo of [21, 35]) {
    const past = laDayShifted(today, -daysAgo);
    await createExperience({
      baseUrl,
      specSlug: SPEC_SLUG,
      accessToken: host.accessToken,
      communityId: community.communityId,
      name: ACTIVITY_NAME,
      description: 'A recurring get-together on the roof at golden hour.',
      timeUnixSec: laMiddayUnix(past.y, past.m, past.d),
      locationId,
    });
  }

  // The pre-existing future event the test opens first.
  await createExperience({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId: community.communityId,
    name: EXISTING_EVENT_NAME,
    description: 'Come join our friendly craft circle! All ages and skill levels.',
    timeUnixSec: laMiddayUnix(existingDay.y, existingDay.m, existingDay.d),
    locationId,
  });

  // ── BROWSER CONTEXT + AUTH ───────────────────────────────────────────────
  const ctx = await newRecordingContext(browser, { timezoneId: TZ, locale: 'en-US' });
  const page = await ctx.newPage();
  try {
    await injectAuth(ctx, {
      accessToken: host.accessToken,
      refreshToken: host.refreshToken,
      user: { id: host.userId, name: host.name },
      serverUrl: baseUrl,
    });
    await installRequestIdOverride(page, SPEC_SLUG);

    await page.goto(`${baseUrl}/`);
    await dismissDataConsent(page);

    // ── STEP 1: open the Plans tab ─────────────────────────────────────────
    await page.getByRole('button', { name: 'Plans', exact: true }).click({ timeout: 15_000 });

    // The month grid opens on the current month with today selected. Track the
    // displayed month as an offset from today's month; the month arrows shift it
    // (and select the 1st of the shifted month).
    let displayedOffset = 0;
    const nextMonth = page.getByRole('button', { name: 'Next month' });
    const prevMonth = page.getByRole('button', { name: 'Previous month' });
    async function showMonth(target: number): Promise<void> {
      while (displayedOffset < target) {
        await nextMonth.click();
        displayedOffset++;
        await page.waitForTimeout(400); // month-slide settle
      }
      while (displayedOffset > target) {
        await prevMonth.click();
        displayedOffset--;
        await page.waitForTimeout(400);
      }
    }
    async function selectDay(day: LaDay): Promise<void> {
      await showMonth(monthOffset(today, day));
      // The cell's accessible name is "EEEE, MMMM d, <summary>…" — anchor on the
      // trailing comma so e.g. "July 1," never matches "July 19,".
      await page
        .getByRole('button', { name: new RegExp(`${cellDateLabel(day)},`) })
        .first()
        .click({ timeout: 15_000 });
      await page.waitForTimeout(400); // detail rebuild
    }

    // ── STEP 2: open the existing event's day → it shows ───────────────────
    await selectDay(existingDay);
    await expect(
      page.getByRole('button', { name: EXISTING_EVENT_NAME }).first(),
    ).toBeVisible({ timeout: 15_000 });

    // ── STEP 3: open the suggestion-only day → suggestion → create ─────────
    await selectDay(suggestionDay);
    const planCta = page.getByRole('button', { name: 'Plan it with them' });
    await expect(planCta).toBeVisible({ timeout: 15_000 });
    await planCta.click();

    // Unified create opens pre-filled and auto-generates (mock AI). The date is
    // seeded structurally to the suggestion day. Save once the stream yields
    // valid content, then wait for the auto-opened share sheet to confirm save.
    const saveBtn = page.getByRole('button', { name: 'Save Event' });
    await saveBtn.waitFor({ timeout: 30_000 });
    await saveBtn.click();
    await page
      .getByRole('button', { name: /copy link/i })
      .waitFor({ timeout: 20_000 });

    // ── POSTCONDITION: the new event is on the suggestion day ───────────────
    const portfolio = createTestClient(PortfolioService, {
      baseUrl,
      specSlug: SPEC_SLUG,
      accessToken: host.accessToken,
    });
    const home = await portfolio.getHomeView({ timezone: TZ });
    const onSuggestionDay = home.calendar.filter(
      (e) =>
        e.kind === HomeUpNextKind.EVENT &&
        e.title !== EXISTING_EVENT_NAME &&
        sameLaDay(laDayOf(new Date(Number(e.timeUnixSec) * 1000)), suggestionDay),
    );
    expect(
      onSuggestionDay.length,
      `expected the newly created event on ${cellDateLabel(suggestionDay)}; ` +
        `calendar EVENT days were ${home.calendar
          .filter((e) => e.kind === HomeUpNextKind.EVENT)
          .map((e) => cellDateLabel(laDayOf(new Date(Number(e.timeUnixSec) * 1000))))
          .join(' | ')}`,
    ).toBeGreaterThan(0);
  } finally {
    await ctx.close();
  }
});
