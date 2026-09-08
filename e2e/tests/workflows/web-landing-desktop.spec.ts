// e2e/tests/workflows/web-landing-desktop.spec.ts
//
// #2912 — the signed-out /go/ SSR landings (event, gear, request, community)
// are the product's most desktop-visited pages: guests open invite links on
// laptops. The pages already center their content at a readable measure via
// the site CSS; this spec locks that in — the primary CTA must be roughly
// centered, bounded well under the window width, and fully on screen at
// 1440×810.
//
// #2918 added the second half: the hero title must hold the SAME measure as
// the body. It used to anchor to the window's left edge while everything
// below it centered, so the two halves of the page disagreed about the
// column. The assertion is the h1's left edge against `.event-main`'s content
// box — not against the CTA, which sits 16px further left by design (the
// button spans the column; the text is inset by `.event-main`'s padding).
//
// Tagged @desktop for the desktop-chromium project; under the phone projects
// the width bound relaxes to the viewport and the assertions reduce to
// "on screen". These are plain HTML pages, so role locators are stable links.

import { test, expect } from '../../lib/fixtures.js';
import { newRecordingContext } from '../../lib/context.js';
import { registerUser } from '../../lib/seed/users.js';
import {
  createCommunity,
  mintCommunityInviteLink,
  mintExperienceShareLink,
  mintGearShareLink,
  mintRequestShareLink,
} from '../../lib/seed/communities.js';
import { seedSharedGear } from '../../lib/seed/gear.js';
import { seedRequest } from '../../lib/seed/requests.js';
import { createExperience } from '../../lib/seed/experiences.js';

const SPEC_SLUG = 'web-landing-desktop';

/// The generous SSR content band: the landings' CSS measure is wider than the
/// app's 640 reading column (hero cards + CTA rows), but must stay far from
/// window-wide. 900 is comfortably above today's measured ~800 content width.
const SSR_MAX_WIDTH = 900;

/// Below this window width `.event-hero-inner`'s `max()` picks its own
/// `--space-5` inset and the hero keeps its historical 8px step over the body
/// — phones render exactly as they did before #2918. Above it the measure
/// binds and the two share a left edge. Mirrors event.css's
/// `--ssr-measure` (560) plus the 2×(space-5 − space-4) crossover.
const MEASURE_BINDS_ABOVE = 576;

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set; global-setup.ts populates it');
  return url;
}

test('@desktop the /go/ landings keep their CTAs centered and bounded', async ({
  browser,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'layout geometry validated on chromium only');
  const baseUrl = requireBaseUrl();

  const host = await registerUser({
    baseUrl,
    specSlug: SPEC_SLUG,
    name: 'Lana Landing',
  });
  const community = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Landing Lane',
  });
  const eventTime = new Date();
  eventTime.setHours(12, 0, 0, 0);
  const experience = await createExperience({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId: community.communityId,
    name: 'Porch Concert',
    description: 'Seeded for the SSR event landing.',
    timeUnixSec: Math.floor(eventTime.getTime() / 1000),
  });
  const gear = await seedSharedGear({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    name: 'Extension Ladder',
  });
  const request = await seedRequest({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    title: 'Need a canopy for Saturday',
  });

  const [eventLink, gearLink, requestLink, communityLink] = await Promise.all([
    mintExperienceShareLink({
      baseUrl,
      specSlug: SPEC_SLUG,
      accessToken: host.accessToken,
      communityId: community.communityId,
      experienceId: experience.experienceId,
    }),
    mintGearShareLink({
      baseUrl,
      specSlug: SPEC_SLUG,
      accessToken: host.accessToken,
      communityId: gear.communityId,
      gearId: gear.gearId,
    }),
    mintRequestShareLink({
      baseUrl,
      specSlug: SPEC_SLUG,
      accessToken: host.accessToken,
      communityId: request.communityId,
      requestId: request.requestId,
    }),
    mintCommunityInviteLink({
      baseUrl,
      specSlug: SPEC_SLUG,
      accessToken: host.accessToken,
      communityId: community.communityId,
    }),
  ]);

  // A signed-out context at the project viewport (#2908 forwarding trap).
  const projectViewport = test.info().project.use.viewport;
  const ctx = await newRecordingContext(browser, {
    viewport: projectViewport ?? { width: 1440, height: 810 },
  });
  const page = await ctx.newPage();
  try {
    const viewport = page.viewportSize();
    if (!viewport) throw new Error('no viewport size; every project sets one');
    const where = `${viewport.width}x${viewport.height}`;
    const cap = Math.min(viewport.width, SSR_MAX_WIDTH);

    const expectCenteredAndBounded = async (
      locator: ReturnType<typeof page.getByRole>,
      what: string,
    ): Promise<void> => {
      await expect(locator).toBeVisible({ timeout: 20_000 });
      const box = await locator.boundingBox();
      expect(box, `${what} has no box at ${where}`).not.toBeNull();
      expect(box!.width, `${what} is nearly window-wide at ${where}`)
        .toBeLessThanOrEqual(cap);
      const center = box!.x + box!.width / 2;
      expect(
        Math.abs(center - viewport.width / 2),
        `${what} is off-center by ${Math.round(center - viewport.width / 2)}px at ${where}`,
      ).toBeLessThanOrEqual(cap / 2);
      expect(box!.y + box!.height, `${what} is below the fold at ${where}`)
        .toBeLessThanOrEqual(viewport.height);
    };

    /// #2918 — the hero title holds the body's measure. Compares the h1's
    /// left edge to `.event-main`'s content-box left edge (its border box
    /// plus its computed padding-left), which is the one landmark present on
    /// all four landings regardless of which optional blocks rendered.
    const expectHeroHoldsTheMeasure = async (what: string): Promise<void> => {
      const heroBox = await page.locator('.event-hero-inner h1').boundingBox();
      expect(heroBox, `${what} hero title has no box at ${where}`).not.toBeNull();
      const bodyLeft = await page.locator('.event-main').evaluate((el) => {
        const rect = el.getBoundingClientRect();
        return rect.left + parseFloat(getComputedStyle(el).paddingLeft);
      });
      const seam = heroBox!.x - bodyLeft;
      const expected = viewport.width >= MEASURE_BINDS_ABOVE ? 0 : 8;
      expect(
        Math.abs(seam - expected),
        `${what} hero title is ${Math.round(seam)}px from the body's left edge ` +
          `at ${where} (expected ${expected}px)`,
      ).toBeLessThanOrEqual(1);
    };

    await page.goto(`${baseUrl}/go/${eventLink.shortCode}`);
    await expectCenteredAndBounded(
      page.getByRole('link', { name: /rsvp yes/i }),
      "the event landing's RSVP CTA",
    );
    await expectHeroHoldsTheMeasure('the event landing');

    await page.goto(`${baseUrl}/go/${gearLink.shortCode}`);
    await expectCenteredAndBounded(
      page.getByRole('link', { name: /to borrow/i }),
      "the gear landing's borrow CTA",
    );
    await expectHeroHoldsTheMeasure('the gear landing');

    await page.goto(`${baseUrl}/go/${requestLink.shortCode}`);
    await expectCenteredAndBounded(
      page.getByRole('link', { name: /offer to help/i }),
      "the request landing's offer CTA",
    );
    await expectHeroHoldsTheMeasure('the request landing');

    await page.goto(`${baseUrl}/go/${communityLink.shortCode}`);
    await expectCenteredAndBounded(
      page.getByRole('link', { name: /join landing lane/i }),
      "the community landing's join CTA",
    );
    await expectHeroHoldsTheMeasure('the community landing');
  } finally {
    await ctx.close();
  }
});
