// e2e/lib/seed/requests.ts — submit a request and return its per-item ad-hoc
// community (#2492). SubmitRequest with no community_id creates the request in
// its own per-item community and shares it in there; the guest joins that
// community via the share link and OfferToFulfill lands against it. All via
// public RPCs; `accessToken` must be an authenticated requester.

import { resolve } from 'node:path';
import { RequestService } from '../../gen/ripls/api/request_service_pb.js';
import { createTestClient } from '../connect.js';
import { uploadMedia } from './experiences.js';

// Realistic committed hero (Unsplash, see fixtures/walkthroughs/manifest.json) so
// the SSR request landing + the phone-first backdrop show a real photo.
const DEFAULT_REQUEST_HERO = resolve(
  __dirname,
  '..',
  '..',
  'fixtures',
  'walkthroughs',
  'request-ladder.jpg',
);

export interface SeededRequest {
  requestId: string;
  /** The request's per-item ad-hoc community (host-only at seed time). */
  communityId: string;
  title: string;
}

export interface SeedRequestOptions {
  baseUrl: string;
  specSlug: string;
  accessToken: string;
  /** Default: 'E2E Request {short uuid}'. */
  title?: string;
  description?: string;
  /** Hero image path; defaults to the committed request fixture. Pass '' to skip. */
  heroImagePath?: string;
}

export async function seedRequest(
  opts: SeedRequestOptions,
): Promise<SeededRequest> {
  const client = createTestClient(RequestService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: opts.accessToken,
  });
  const title = opts.title ?? `E2E Request ${shortId()}`;

  const heroPath = opts.heroImagePath ?? DEFAULT_REQUEST_HERO;
  const mediaIds = heroPath
    ? [
        await uploadMedia({
          baseUrl: opts.baseUrl,
          specSlug: opts.specSlug,
          accessToken: opts.accessToken,
          path: heroPath,
          contentType: 'image/jpeg',
          filename: 'request-ladder.jpg',
        }),
      ]
    : [];

  const resp = await client.submitRequest({
    // A submitted request always lands in its own per-item community (#2492).
    // SubmitRequestRequest carried a community_id at one point; it no longer
    // does, so the `communityId: ''` that used to sit here was inert.
    title,
    description: opts.description ?? 'Auto-created by the e2e harness.',
    mediaIds,
  });
  if (!resp.requestId || !resp.itemCommunityId) {
    throw new Error(
      'SubmitRequest returned no request_id / item_community_id (per-item community not provisioned)',
    );
  }
  return {
    requestId: resp.requestId,
    communityId: resp.itemCommunityId,
    title,
  };
}

export interface UpdateRequestContentOptions {
  baseUrl: string;
  specSlug: string;
  /** Must be the requester — UpdateRequest is requester-only for non-media fields. */
  accessToken: string;
  requestId: string;
  /** Empty/omitted fields are left unchanged (UpdateRequest is partial). */
  title?: string;
  description?: string;
  /** Location to pin on the request (a createLocation id). */
  locationId?: string;
  /** Hero image to upload + attach (replaces the request's media list). */
  heroImagePath?: string;
}

/**
 * Tidy a UI-created request off camera: re-assert title/description and attach
 * a committed fixture hero. The walkthrough reels use this right after the
 * on-camera unified create — the hermetic server has no stock-imagery
 * provider, so without an explicit hero the SSR landing falls back to the
 * default logo (#2686).
 */
export async function updateRequestContent(
  opts: UpdateRequestContentOptions,
): Promise<void> {
  const clientOpts = {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: opts.accessToken,
  };
  const mediaIds = opts.heroImagePath
    ? [
        await uploadMedia({
          ...clientOpts,
          path: opts.heroImagePath,
          contentType: 'image/jpeg',
          filename: opts.heroImagePath.split('/').pop() ?? 'hero.jpg',
        }),
      ]
    : [];
  await createTestClient(RequestService, clientOpts).updateRequest({
    requestId: opts.requestId,
    title: opts.title ?? '',
    description: opts.description ?? '',
    locationId: opts.locationId ?? '',
    mediaIds,
  });
}

function shortId(): string {
  return Math.random().toString(36).slice(2, 8);
}
