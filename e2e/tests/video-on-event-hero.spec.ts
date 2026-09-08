// e2e/tests/video-on-event-hero.spec.ts — Phase-1 anchor scenario
// for #2162. Reproduces the bug shape of #2155: a Web-only video
// playback regression that no ViewModel unit test catches because
// the failure path is in createCachedVideoController's platform-
// branched code.
//
// What the spec proves:
//   1. The seeded experience renders on /event/{id} (route + bundle
//      bootstrap + Semantics tree).
//   2. The hero <video> element reaches HAVE_CURRENT_DATA
//      (readyState >= 2) within 10s — i.e., the video bytes actually
//      decoded.
//   3. video.error is null after the wait — no MediaError raised.
//
// A regression that breaks createCachedVideoController on Web will
// fail either step (2) or (3). The Phase-0 anchor identifier
// 'web-event-screen' on WebExperienceScreen confirms step (1).

import { test, expect } from '../lib/fixtures.js';
import { resolve } from 'node:path';
import { installRequestIdOverride } from '../lib/connect.js';
import { injectAuth } from '../lib/auth.js';
import { registerUser } from '../lib/seed/users.js';
import { createCommunity } from '../lib/seed/communities.js';
import { createExperience } from '../lib/seed/experiences.js';

const SPEC_SLUG = 'video-on-event-hero';
const VIDEO_FIXTURE = resolve(__dirname, '..', 'fixtures', 'event-hero.mp4');

test('hero video plays on the seeded event page', async ({ page }) => {
  const baseUrl = requireBaseUrl();

  // Phase 1: seed a user, a community, and an experience whose hero
  // is the LFS-tracked event-hero.mp4 fixture.
  const host = await registerUser({ baseUrl, specSlug: SPEC_SLUG });
  const community = await createCommunity({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
  });
  const experience = await createExperience({
    baseUrl,
    specSlug: SPEC_SLUG,
    accessToken: host.accessToken,
    communityId: community.communityId,
    heroMedia: {
      path: VIDEO_FIXTURE,
      contentType: 'video/mp4',
      filename: 'event-hero.mp4',
    },
  });

  // Phase 2: pre-inject the auth-state localStorage entries so the
  // bundle hydrates authenticated. injectAuth populates tokens + user
  // + server_url; the bundle's safety checks discard tokens when any
  // of those are missing (auth_state.dart §304, §348).
  await injectAuth(page.context(), {
    accessToken: host.accessToken,
    refreshToken: host.refreshToken,
    user: { id: host.userId, name: host.name },
    serverUrl: baseUrl,
  });

  // Phase 3: rewrite browser-side X-Request-ID so server log lines
  // for the bundle's RPCs carry the e2e-{spec}-browser-{seq} prefix.
  await installRequestIdOverride(page, SPEC_SLUG);

  // Phase 4: navigate and assert the WebExperienceScreen rendered.
  // 'web-event-screen' is the anchor identifier added in Phase 0.
  const captureConsole: string[] = [];
  page.on('console', (msg) => {
    const txt = `${msg.type()}: ${msg.text()}`;
    captureConsole.push(txt);
    if (msg.type() === 'error' || /auth|token|user|🔑|🚪|⚠️|Load|server/i.test(msg.text())) {
      console.log(`[browser] ${txt}`);
    }
  });
  page.on('pageerror', (err) => {
    captureConsole.push(`pageerror: ${err.message}`);
    console.log(`[browser:pageerror] ${err.message}`);
  });

  await page.goto(`${baseUrl}/event/${experience.experienceId}`);
  await expect(
    page.locator('[flt-semantics-identifier="web-event-screen"]'),
  ).toBeAttached({ timeout: 20_000 });

  // DEBUG: dump localStorage to confirm token injection landed
  const lsDump = await page.evaluate(() => {
    const out: Record<string, string> = {};
    for (let i = 0; i < window.localStorage.length; i++) {
      const k = window.localStorage.key(i)!;
      const v = window.localStorage.getItem(k) ?? '';
      out[k] = v.length > 60 ? `${v.slice(0, 30)}…(len=${v.length})` : v;
    }
    return out;
  });
  console.log('[debug] localStorage:', JSON.stringify(lsDump, null, 2));

  const video = page.locator('video').first();
  await expect(video).toBeAttached({ timeout: 20_000 });
  const result = await video.evaluate(
    (el, { timeoutMs }) => {
      return new Promise<{ readyState: number; error: MediaError | null }>(
        (resolve) => {
          const v = el as HTMLVideoElement;
          const deadline = performance.now() + timeoutMs;
          const tick = () => {
            if (v.readyState >= 2 || performance.now() > deadline) {
              resolve({ readyState: v.readyState, error: v.error });
              return;
            }
            setTimeout(tick, 100);
          };
          tick();
        },
      );
    },
    { timeoutMs: 10_000 },
  );

  // Attach captured console output to the test report when the
  // assertion fails — makes triage easier without re-running.
  try {
    expect(result.readyState, 'hero video reached HAVE_CURRENT_DATA').toBeGreaterThanOrEqual(2);
    expect(result.error, 'hero video has no MediaError').toBeNull();
  } catch (err) {
    test.info().attach('console-log', {
      contentType: 'text/plain',
      body: captureConsole.join('\n'),
    });
    throw err;
  }

  // Per-element screenshot guards against the "video element exists
  // and readyState >= 2 but the rendered frame is black" failure
  // mode that simple readyState polling misses.
  await video.screenshot({
    path: test.info().outputPath('hero-video-frame.png'),
  });
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
