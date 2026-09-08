// e2e/lib/seed/gear.ts — create a gear item and return its per-item ad-hoc
// community (#2492 WEB-4). SaveGear provisions that community and shares the
// gear in, carrying the creation-time Lend/Give choice (#2687) so the
// loan-vs-giveaway state is deterministic (ExpressInterest requires it set). All
// via public RPCs; `accessToken` must be an authenticated host.

import { resolve } from 'node:path';
import { GearService } from '../../gen/ripls/api/gear_service_pb.js';
import { Availability } from '../../gen/ripls/api/gear_pb.js';
import { MaterialCategory } from '../../gen/ripls/api/common_pb.js';
import { createTestClient } from '../connect.js';
import { uploadMedia } from './experiences.js';

/** Optional impact metadata for a seeded gear item so a loan/giveaway of it
 * produces non-zero money saved (value) and emissions avoided (weight +
 * material). Without it the estimator has nothing to work from and the impact
 * lands at $0 / 0kg — see the classroom walkthrough (#2703). */
export interface SeedGearImpact {
  valueUsd?: number;
  weightGrams?: number;
  material?: MaterialCategory;
}

// Realistic committed hero (Unsplash, see fixtures/walkthroughs/manifest.json) so
// the SSR gear landing + the phone-first backdrop show a real photo.
const DEFAULT_GEAR_HERO = resolve(
  __dirname,
  '..',
  '..',
  'fixtures',
  'walkthroughs',
  'gear-drill.jpg',
);

export interface SeededGear {
  gearId: string;
  /** The gear's per-item ad-hoc community (host-only at seed time). */
  communityId: string;
  name: string;
}

export interface SeedSharedGearOptions {
  baseUrl: string;
  specSlug: string;
  accessToken: string;
  /** Default: 'E2E Gear {short uuid}'. */
  name?: string;
  description?: string;
  /** Default: FOR_LOAN. */
  availability?: Availability;
  /** Hero image path; defaults to the committed gear fixture. Pass '' to skip. */
  heroImagePath?: string;
  /** Optional value/weight/material so a give/lend of this gear has real impact. */
  impact?: SeedGearImpact;
}

export async function seedSharedGear(
  opts: SeedSharedGearOptions,
): Promise<SeededGear> {
  const gearClient = createTestClient(GearService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: opts.accessToken,
  });
  const name = opts.name ?? `E2E Gear ${shortId()}`;

  const heroPath = opts.heroImagePath ?? DEFAULT_GEAR_HERO;
  const mediaIds = heroPath
    ? [
        await uploadMedia({
          baseUrl: opts.baseUrl,
          specSlug: opts.specSlug,
          accessToken: opts.accessToken,
          path: heroPath,
          contentType: 'image/jpeg',
          filename: 'gear-drill.jpg',
        }),
      ]
    : [];

  const impact = opts.impact;
  const saveResp = await gearClient.saveGear({
    name,
    description: opts.description ?? 'Auto-created by the e2e harness.',
    mediaIds,
    // Honored on insert only: SaveGear carries this into the per-item
    // community's first share, so the gear is born lend-or-give (#2687).
    availability: opts.availability ?? Availability.FOR_LOAN,
    // "text" mode so the server assigns provenance to the metadata fields.
    // The field is generation_mode; this said `mode`, which SaveGearRequest has
    // no such field for, so protobuf-es dropped it silently and seeded gear
    // never actually got provenance assigned. Caught by the tsc gate.
    generationMode: impact ? 'text' : undefined,
    metadata: impact
      ? {
          valueEstimate:
            impact.valueUsd != null ? { estimatedValueUsd: impact.valueUsd } : undefined,
          weightGrams:
            impact.weightGrams != null
              ? { value: { mean: impact.weightGrams, stddev: 0 } }
              : undefined,
          materialCategory: impact.material != null ? { value: impact.material } : undefined,
        }
      : undefined,
  });
  if (!saveResp.id || !saveResp.itemCommunityId) {
    throw new Error(
      'SaveGear returned no id / item_community_id (per-item community not provisioned)',
    );
  }

  return {
    gearId: saveResp.id,
    communityId: saveResp.itemCommunityId,
    name,
  };
}

function shortId(): string {
  return Math.random().toString(36).slice(2, 8);
}
