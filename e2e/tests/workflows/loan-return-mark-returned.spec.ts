// e2e/tests/workflows/loan-return-mark-returned.spec.ts
//
// Maps to: docs/workflows/loan.md → Example 1, step 11 (the borrower returns
// the item and marks the loan complete) — the return slice only; interest,
// selection, and start are RPC preconditions.
//
// What this spec proves:
//   - The borrower's two mark-returned affordances are reachable through the
//     REAL browser accessibility tree. The Home NEEDS-YOU "Mark returned"
//     pill is the canonical regression: nested inside the row Tappable it has
//     no semantics node, so the row's flt-semantics element swallows every
//     click on Flutter Web (and hides the pill from screen readers) — while
//     the widget test's raw gesture arena still passes. Only a real browser
//     catches this class (#2638 follow-up).
//   - The gear who-card CTA flips to "Mark returned" for the current holder
//     and completes the loan (pre-fix the gear screen had NO return
//     affordance for the borrower).
//
// What this spec does NOT prove:
//   - Transfer state-machine correctness, notifications, or impact math —
//     server integration tests cover Example 1 end-to-end via RPC.
//   - The confirm-sheet/undo plumbing beyond "the mutation landed".
//
// Divergences from loan.md Example 1:
//   - Single borrower (no borrower B) — the queue is irrelevant to the
//     return affordances under test.
//   - Steps 1-10 (share, interest, start) are seeded via RPC; only step 11
//     runs through the UI, once per surface (Home pill / gear who-card CTA).

import { test, expect } from '../../lib/fixtures.js';
import { createTestClient, installRequestIdOverride } from '../../lib/connect.js';
import { newRecordingContext } from '../../lib/context.js';
import { injectAuth } from '../../lib/auth.js';
import { dismissDataConsent } from '../../lib/ui/consent.js';
import { registerUser, registerUserViaInvite } from '../../lib/seed/users.js';
import { seedSharedGear } from '../../lib/seed/gear.js';
import { GearService } from '../../gen/ripls/api/gear_service_pb.js';
import { TransferService } from '../../gen/ripls/api/transfer_service_pb.js';

const SPEC_SLUG = 'loan-return-mark-returned';
const DAY_MS = 86_400_000;

interface LoanWorld {
  gearId: string;
  communityId: string;
  borrower: Awaited<ReturnType<typeof registerUser>>;
  owner: Awaited<ReturnType<typeof registerUser>>;
}

/** Seed loan.md Example 1 through step 10: shared gear, a booking by the
 *  borrower, and the loan started by the owner (transfer ACTIVE). */
async function seedActiveLoan(baseUrl: string, slug: string): Promise<LoanWorld> {
  const owner = await registerUser({ baseUrl, specSlug: slug, name: 'Marc Hall' });
  const gear = await seedSharedGear({
    baseUrl,
    specSlug: slug,
    accessToken: owner.accessToken,
    name: 'Coleman 120qt Cooler',
  });
  const borrower = await registerUserViaInvite({
    baseUrl,
    specSlug: slug,
    name: 'Alice Holder',
    inviterAccessToken: owner.accessToken,
    communityId: gear.communityId,
  });

  // Returns a bigint: start_date_unix_sec / end_date_unix_sec are int64, which
  // protobuf-es maps to bigint.
  const day = (n: number): bigint => {
    const d = new Date(Date.now() + n * DAY_MS);
    return BigInt(
      Math.floor(new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime() / 1000),
    );
  };
  const gearClient = createTestClient(GearService, {
    baseUrl,
    specSlug: slug,
    accessToken: borrower.accessToken,
  });
  const claim = await gearClient.claimGearDays({
    gearId: gear.gearId,
    communityId: gear.communityId,
    startDateUnixSec: day(2),
    endDateUnixSec: day(3),
  });
  const ownerTransfers = createTestClient(TransferService, {
    baseUrl,
    specSlug: slug,
    accessToken: owner.accessToken,
  });
  await ownerTransfers.startLoan({ transferId: claim.booking!.id });

  return { gearId: gear.gearId, communityId: gear.communityId, borrower, owner };
}

/** The borrower's live transfer on the gear, or undefined once the loan is
 *  terminal (GetGearTransferContext only surfaces non-terminal transfers for
 *  non-owners). */
async function liveTransferState(baseUrl: string, slug: string, world: LoanWorld) {
  const client = createTestClient(TransferService, {
    baseUrl,
    specSlug: slug,
    accessToken: world.borrower.accessToken,
  });
  const resp = await client.getGearTransferContext({
    gearId: world.gearId,
    communityId: world.communityId,
  });
  return resp.context?.userTransfer?.state;
}

test('borrower marks the loan returned from the Home NEEDS-YOU pill', async ({
  browser,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'return-flow UI is validated on chromium only');
  const baseUrl = requireBaseUrl();
  const slug = `${SPEC_SLUG}-home`;
  const world = await seedActiveLoan(baseUrl, slug);

  const ctx = await newRecordingContext(browser);
  await injectAuth(ctx, {
    accessToken: world.borrower.accessToken,
    refreshToken: world.borrower.refreshToken,
    user: { id: world.borrower.userId, name: world.borrower.name },
    serverUrl: baseUrl,
  });
  const page = await ctx.newPage();
  await installRequestIdOverride(page, `${slug}-borrower`);

  try {
    await page.goto(`${baseUrl}/`);
    await dismissDataConsent(page);

    // THE regression assert: the pill must exist as its own accessibility
    // node. Pre-fix it was merged into the row's semantics, unreachable by
    // role — and every DOM click ran the row action instead.
    const pill = page.getByRole('button', { name: 'Mark returned' });
    await expect(pill).toBeVisible({ timeout: 20_000 });
    await pill.click();

    // The in-place confirm sheet re-uses the accept label on its confirm
    // button; after it opens there are two "Mark returned" nodes — the
    // dialog's is the later one.
    await page.getByRole('button', { name: 'Mark returned' }).last().click();

    // UI postcondition: the decision resolves and the pill leaves Home.
    await expect(page.getByRole('button', { name: 'Mark returned' })).toHaveCount(0, {
      timeout: 20_000,
    });
  } finally {
    await ctx.close();
  }

  // Server postcondition: no live transfer remains for the borrower.
  expect(await liveTransferState(baseUrl, slug, world)).toBeUndefined();
});

test('current holder marks the loan returned from the gear who-card CTA', async ({
  browser,
  browserName,
}) => {
  test.skip(browserName === 'webkit', 'return-flow UI is validated on chromium only');
  const baseUrl = requireBaseUrl();
  const slug = `${SPEC_SLUG}-card`;
  const world = await seedActiveLoan(baseUrl, slug);

  const ctx = await newRecordingContext(browser);
  await injectAuth(ctx, {
    accessToken: world.borrower.accessToken,
    refreshToken: world.borrower.refreshToken,
    user: { id: world.borrower.userId, name: world.borrower.name },
    serverUrl: baseUrl,
  });
  const page = await ctx.newPage();
  await installRequestIdOverride(page, `${slug}-borrower`);

  try {
    await page.goto(`${baseUrl}/gear/${world.gearId}`);

    // The holder's who-card CTA is "Mark returned" — pre-fix the gear screen
    // offered the holder nothing but "Book for borrowing".
    const cta = page.getByRole('button', { name: 'Mark this loan returned' });
    await expect(cta).toBeVisible({ timeout: 30_000 });
    await expect(
      page.getByRole('button', { name: 'Book this gear for borrowing' }),
    ).toHaveCount(0);

    await cta.click();

    // The CTA completes the loan directly (same handler as the workflow
    // checklist); the impact modal that follows is out of scope — assert the
    // mutation landed server-side.
    await expect
      .poll(async () => await liveTransferState(baseUrl, slug, world), {
        timeout: 20_000,
      })
      .toBeUndefined();
  } finally {
    await ctx.close();
  }
});

function requireBaseUrl(): string {
  const url = process.env.E2E_BASE_URL;
  if (!url) throw new Error('E2E_BASE_URL not set; global-setup.ts is supposed to populate it');
  return url;
}
