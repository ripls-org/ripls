// e2e/lib/connect.ts — Connect-Node client factory for seed RPCs,
// plus a Playwright helper that overrides X-Request-ID on browser-
// side RPC traffic.
//
// Why both sides:
//
// 1. Seed primitives (createCommunity, registerUser, …) run in the
//    Node test process and need a real Connect-Node client. The
//    factory below attaches:
//      - X-Request-ID: e2e-{specSlug}-{seq} so seed-side RPC failures
//        correlate to the server slog line that handled them.
//      - Authorization: Bearer {accessToken} when provided.
//
// 2. Browser-side RPC traffic (the Flutter bundle dialing the server
//    from inside Playwright's Chromium / WebKit) goes through the
//    bundle's own connectrpc/dart client, which already attaches a
//    request ID via `RequestIdGenerator`. To keep the e2e-{slug}-…
//    prefix consistent across both sides, the test installs a
//    `page.route` handler that rewrites any outgoing X-Request-ID
//    that doesn't already start with the test's slug. See
//    `installRequestIdOverride` below.
//
// Per the #2162 plan and observability conventions
// (docs/server/observability.md), every request gets a correlation
// ID; tests use the predictable prefix so triagers can grep server
// logs by spec.

import type { Interceptor, Transport } from '@connectrpc/connect';
import { createClient } from '@connectrpc/connect';
import { createConnectTransport } from '@connectrpc/connect-node';
import type { DescService } from '@bufbuild/protobuf';
import { SIMULATION_TIMESTAMP_HEADER, filmedAtUnixSec } from './filmed-at.js';
import type { Page } from '@playwright/test';

export interface TestClientOptions {
  /** Server base URL (e.g. http://localhost:8080). */
  baseUrl: string;
  /**
   * Short kebab slug identifying the spec (e.g. "video-on-event-hero").
   * Used as the X-Request-ID prefix so server logs can be sliced by spec.
   */
  specSlug: string;
  /** Optional bearer token to attach as Authorization header. */
  accessToken?: string;
}

/**
 * Build a typed Connect-Node client for the given service, with
 * X-Request-ID and Authorization interceptors wired up.
 *
 * Each returned client carries its own monotonic seq counter so two
 * concurrent specs can't collide on the same request ID.
 */
export function createTestClient<S extends DescService>(
  service: S,
  opts: TestClientOptions,
) {
  let seq = 0;
  const requestIdInterceptor: Interceptor = (next) => async (req) => {
    req.header.set('X-Request-ID', `e2e-${opts.specSlug}-${++seq}`);
    return await next(req);
  };
  const authInterceptor: Interceptor = (next) => async (req) => {
    if (opts.accessToken) {
      req.header.set('Authorization', `Bearer ${opts.accessToken}`);
    }
    return await next(req);
  };
  // Seeded content must be stamped on the same clock the page believes in —
  // see lib/filmed-at.ts. Read per-request, not per-client: a spec declares
  // the moment once and every client built before or after honours it.
  const filmedAtInterceptor: Interceptor = (next) => async (req) => {
    const sec = filmedAtUnixSec();
    if (sec !== null) req.header.set(SIMULATION_TIMESTAMP_HEADER, String(sec));
    return await next(req);
  };

  const transport: Transport = createConnectTransport({
    baseUrl: opts.baseUrl,
    httpVersion: '1.1',
    interceptors: [requestIdInterceptor, authInterceptor, filmedAtInterceptor],
  });
  return createClient(service, transport);
}

/**
 * Rewrite outgoing X-Request-ID on browser-side Connect calls so the
 * bundle's own RequestIdGenerator output is overridden with the
 * e2e-{specSlug}-{seq} prefix. The handler matches any path under
 * `/ripls.api.*Service/*` (the Connect protocol's URL shape).
 *
 * Call once per spec, after the page is created but before any
 * navigation. The seq counter is independent of the seed-side
 * client's counter — browser and node sides have separate streams,
 * and the slug prefix is what ties them together when grepping logs.
 */
export async function installRequestIdOverride(
  page: Page,
  specSlug: string,
): Promise<void> {
  let seq = 0;
  await page.route(
    /\/ripls\.api\.[A-Za-z]+Service\//,
    async (route) => {
      const headers = {
        ...route.request().headers(),
        'x-request-id': `e2e-${specSlug}-browser-${++seq}`,
      };
      await route.continue({ headers });
    },
  );
}
