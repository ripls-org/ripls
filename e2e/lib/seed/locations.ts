// e2e/lib/seed/locations.ts — create a place/location row that can be
// attached to an experience via SaveExperienceRequest.location_id.
//
// Goes through the public LocationService.SaveLocation RPC. The SSR event
// landing (server/services/web/event_page.go fetchPlaceName) renders
// Location.GetName() when set, so pass `name` for a venue label rather than
// a bare locality.

import { LocationService } from '../../gen/ripls/api/location_service_pb.js';
import { createTestClient } from '../connect.js';

export interface CreateLocationOptions {
  baseUrl: string;
  specSlug: string;
  accessToken: string;
  /** Venue name shown as the place label, e.g. "Skyline Rooftop & Lounge". */
  name: string;
  /** Street address lines, e.g. ["44 Tehama St"]. */
  addressLines: string[];
  locality: string; // city
  administrativeArea: string; // state/province
  regionCode: string; // CLDR, e.g. "US"
  postalCode: string;
  latitudeDeg: number;
  longitudeDeg: number;
}

export async function createLocation(opts: CreateLocationOptions): Promise<string> {
  const client = createTestClient(LocationService, {
    baseUrl: opts.baseUrl,
    specSlug: opts.specSlug,
    accessToken: opts.accessToken,
  });
  const resp = await client.saveLocation({
    name: opts.name,
    addressLines: opts.addressLines,
    locality: opts.locality,
    administrativeArea: opts.administrativeArea,
    regionCode: opts.regionCode,
    postalCode: opts.postalCode,
    latitudeDeg: opts.latitudeDeg,
    longitudeDeg: opts.longitudeDeg,
  });
  if (!resp.id) {
    throw new Error('SaveLocation returned empty id');
  }
  return resp.id;
}
