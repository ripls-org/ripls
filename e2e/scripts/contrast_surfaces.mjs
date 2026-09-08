// contrast_surfaces.mjs — the evaluation set for the composited-contrast gate (#2770).
//
// Declarative on purpose: adding a surface should be a data change, not a code
// change, because the matrix is expected to grow through Phase 2 of
// docs/issues/2770-token-contrast-evaluation.md.
//
// The set is defined by one rule: every place a token is composited onto
// something other than `background` or `surface` belongs here. Flat surfaces are
// included as a control group — they prove a candidate did not fix glass by
// wrecking the surfaces that already worked.
//
// `backdrops` names which seeded hero image sits behind a media-bearing surface.
// The synthetic ones bound the worst case a real photo can produce, so passing
// them is a guarantee rather than a sample: nothing a user uploads is brighter
// than white or busier than full-amplitude noise.

/** Groups, in the order the comparison sheet presents them. */
export const GROUPS = {
  glassMedia: 'Glass + media core',
  flat: 'Flat surfaces + status states',
  bespoke: 'Other bespoke materials',
  creation: 'Creation & preview flows',
};

/** Backdrops that gate. Real photos are for visual judgement and are not gated. */
export const SYNTHETIC_BACKDROPS = ['white', 'black', 'noise'];

/**
 * Real photographs, for judging rather than gating.
 *
 * A synthetic white rectangle bounds the worst case, which is what the gate
 * needs, but under a wash it is just a grey rectangle — it cannot say whether a
 * photograph still looks like one. These span what the adaptive scrim keys on:
 * bright needs the most wash, dark needs none.
 */
export const REAL_BACKDROPS = ['photo-bright', 'photo-mid', 'photo-dark'];

/**
 * A surface is:
 *   id        stable slug, used in artifact filenames
 *   group     which section of the comparison sheet it belongs to
 *   themes    which app themes to capture
 *   backdrops synthetic backdrops to capture, or ['none'] for non-media surfaces
 *   open      async (page, world, baseUrl) => void — navigate and settle
 *
 * `open` must leave the page on the surface with animations finished. It may
 * only use role/label locators: CanvasKit paints text to canvas, so DOM text
 * assertions do not work (e2e/README.md § What's reliably reachable).
 */
export const SURFACES = [
  {
    id: 'gear-detail-hero',
    group: 'glassMedia',
    themes: ['light', 'dark'],
    backdrops: SYNTHETIC_BACKDROPS,
    description:
      'Content hero over media — title, chips, status chip and floating actions painted directly on the photo.',
    async open(page, world, baseUrl) {
      await page.goto(`${baseUrl}/gear/${world.gear.gearId}`);
      await settle(page);
    },
  },
  {
    id: 'experience-detail-hero',
    group: 'glassMedia',
    themes: ['light', 'dark'],
    backdrops: SYNTHETIC_BACKDROPS,
    description: 'Event hero with the agenda dock over media.',
    async open(page, world, baseUrl) {
      await page.goto(`${baseUrl}/experience/${world.experience.experienceId}`);
      await settle(page);
    },
  },
  {
    id: 'glass-date-picker',
    group: 'glassMedia',
    themes: ['light', 'dark'],
    backdrops: SYNTHETIC_BACKDROPS,
    description:
      'The glass date/time picker — #2764. glass_pickers.dart hands ' +
      'modalPrimaryButtonBackground (deep sage, validated as a button FILL) to ' +
      "ColorScheme.primary, so Material paints it as the TextButton FOREGROUND " +
      'for Cancel/OK and as todayForegroundColor, directly on the dark sheet.',
    async open(page, world, baseUrl) {
      await page.goto(`${baseUrl}/experience/${world.experience.experienceId}`);
      await settle(page);
      // Labels confirmed against the live aria tree: the WHEN row is
      // `button "Change time WHEN TBD"` wrapping `button "Set the event time"`.
      // Target the inner one — the outer wrapper is the row, not the control.
      await page.getByRole('button', { name: /set the event time/i }).first().tap();
      await settle(page, 2500);
      await page.getByRole('button', { name: /^set the time$/i }).first().tap();
      await settle(page, 2500);
    },
  },
  {
    id: 'material-date-picker',
    group: 'glassMedia',
    themes: ['light', 'dark'],
    backdrops: ['none'],
    description:
      'The Material date dialog from #2764 — the screen the reporter actually ' +
      'photographed. Reached from event CREATION (preview card → Set time → ' +
      'Select a date), not from an existing event, which is why the earlier ' +
      'glass-date-picker surface missed it: that one is the custom ' +
      'DateTimePickerView, whose Cancel/Save were never broken. This one is ' +
      'themed by glass_pickers.dart, where the sage button FILL was handed to ' +
      'ColorScheme.primary and became the Cancel/OK foreground at 1.01:1.',
    async open(page, world, baseUrl) {
      await page.goto(`${baseUrl}/`);
      await settle(page, 1500);
      await page.getByRole('button', { name: /^create$/i }).click();
      await page.waitForTimeout(1200);
      await page.getByRole('button', { name: /^text$/i }).click();
      await page.waitForTimeout(800);
      const prompt = page.getByRole('textbox').first();
      await prompt.click();
      // Deliberately timeless, so the preview row reads "Set time" rather than a
      // formatted time — the locator below depends on it.
      await prompt.pressSequentially('Neighbourhood potluck at the garden', {
        delay: 20,
      });
      await page.getByRole('button', { name: /^draft it$/i }).click();
      // The preview is saveable once "Save event" exists; generation is served
      // by the deterministic mock AI provider, so this is bounded.
      await page
        .getByRole('button', { name: /^save event$/i })
        .waitFor({ timeout: 45_000 });
      await page.getByRole('button', { name: /^set time$/i }).first().click();
      await page.waitForTimeout(1500);
      await page.getByRole('button', { name: /select a date/i }).first().click();
      await page.waitForTimeout(2000);
    },
  },
  {
    id: 'request-detail',
    group: 'flat',
    themes: ['light', 'dark'],
    backdrops: ['none'],
    description:
      'Request detail — largely flat surfaces; the control group for status colours (#2445).',
    async open(page, world, baseUrl) {
      await page.goto(`${baseUrl}/request/${world.request.requestId}`);
      await settle(page);
    },
  },
  {
    id: 'home-feed',
    group: 'glassMedia',
    themes: ['light', 'dark'],
    backdrops: SYNTHETIC_BACKDROPS,
    description: 'Feed cards over media plus the bottom nav dock.',
    async open(page, world, baseUrl) {
      await page.goto(`${baseUrl}/`);
      await settle(page);
    },
  },
  {
    id: 'feedback-sheet',
    // glassMedia, not bespoke: it is a GlassSheet composited over the home
    // feed's media, and that group is what perPrMatrix() runs. A surface that
    // just shipped a user-visible regression belongs in the fast per-PR scope,
    // not only in the nightly full matrix.
    group: 'glassMedia',
    themes: ['light', 'dark'],
    // Not ['none']: the sheet is translucent and opens over the home feed's
    // media, so the backdrop is the axis that decides whether its inset fills
    // hold up. #2798's opaque defects are gated by
    // app/test/helpers/contrast_helpers.dart; what only this layer can measure
    // is the translucent majority of the sheet.
    backdrops: SYNTHETIC_BACKDROPS,
    description:
      'The feedback sheet — #2798. Two controls painted ' +
      'accentButtonBackground (DesignTokens.lightSurface, #F2F2EE, a ' +
      'light-THEME surface) as their own fill inside the always-dark glass ' +
      'sheet and then drew on-glass foregrounds on it: the "Add Screenshots" ' +
      'label at 1.12:1 and the selected type chip at 1.79:1. Reached through ' +
      'the profile menu, which is also where #2797 renamed the row. Expect the ' +
      'sheet body to look near-identical in light and dark — glass is an ' +
      'always-dark material — so a theme difference here is itself a finding.',
    async open(page, world, baseUrl) {
      await page.goto(`${baseUrl}/`);
      await settle(page);
      // Labels come from the widgets themselves: the avatar is
      // Tappable(semanticsLabel: l10n.a11yOpenProfileMenu) and _MeSheetRow
      // passes its title as the Tappable label (the subtitle is not announced).
      await page.getByRole('button', { name: /open account menu/i }).first().tap();
      await settle(page, 1500);
      await page.getByRole('button', { name: /help & feedback/i }).first().tap();
      await settle(page, 2000);
    },
  },
];

/**
 * Dismisses the first-load consent modal, then waits for CanvasKit to paint.
 *
 * The dismissal is NOT optional politeness. A freshly-registered user gets the
 * Data Consent sheet on first load, and it covers the whole screen — every
 * `home-feed` capture in this set was silently measuring that modal instead of
 * the feed, which is why the surface "passed" while the bottom tab dock behind
 * it was never measured at all. `lib/ui/consent.ts` documents the same trap from
 * #2531; this is the third time it has bitten.
 *
 * There is no reliable "rendered" signal to await — the wasm bundle boots, then
 * paints, then settles images. A fixed dwell is crude but it is what the
 * walkthrough capture pipeline already relies on, and an under-settled frame
 * shows up as a missing mask rather than a wrong number.
 */
export async function settle(page, ms = 3500) {
  const acceptAll = page.getByRole('button', { name: /accept all/i });
  const present = await acceptAll
    .waitFor({ state: 'visible', timeout: 12_000 })
    .then(() => true)
    .catch(() => false);
  if (present) {
    await acceptAll.click();
    await page.waitForTimeout(600);
  }
  await page.waitForTimeout(ms);
}

/** Every (surface, theme, backdrop) triple the gate captures. */
export function expandMatrix(surfaces = SURFACES, { groups } = {}) {
  const out = [];
  for (const surface of surfaces) {
    if (groups && !groups.includes(surface.group)) continue;
    for (const theme of surface.themes) {
      for (const backdrop of surface.backdrops) {
        out.push({ surface, theme, backdrop, id: `${surface.id}-${theme}-${backdrop}` });
      }
    }
  }
  return out;
}

/** Media-bearing surfaces over real photographs — the judgement set. */
export function photoMatrix(surfaces = SURFACES) {
  const out = [];
  for (const surface of surfaces) {
    if (surface.backdrops[0] === 'none') continue;
    for (const theme of surface.themes) {
      for (const backdrop of REAL_BACKDROPS) {
        out.push({ surface, theme, backdrop, id: `${surface.id}-${theme}-${backdrop}` });
      }
    }
  }
  return out;
}

/**
 * The reduced per-PR set: glass+media only, both themes, and only the harshest
 * backdrop. Bryan's call — minimal per-PR, full matrix nightly — so a PR pays
 * for one extra bundle build and a handful of renders rather than the full
 * sweep.
 */
export function perPrMatrix(surfaces = SURFACES) {
  return expandMatrix(surfaces, { groups: ['glassMedia'] }).filter(
    (c) => c.backdrop === 'white' || c.backdrop === 'none',
  );
}
