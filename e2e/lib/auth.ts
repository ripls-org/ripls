// e2e/lib/auth.ts — pre-navigation auth-state injection for the
// Flutter Web bundle. Hydrates the four localStorage entries the
// bundle's auth_state.dart loadAuthState() reads at startup:
//
//   flutter.access_token   — JSON-quoted JWT.
//   flutter.refresh_token  — JSON-quoted opaque string.
//   flutter.user           — JSON-encoded User object ({id, name, mediaId}).
//   flutter.server_url     — JSON-quoted current server URL.
//
// All four are required: if any is missing the bundle's safety
// checks discard the access token and fall through to the
// unauthenticated state (see auth_state.dart §304 server-URL
// guard + §348 user-missing fall-through).
//
// shared_preferences_web JSON-decodes every value at read time
// (§_decodeValue), so the writes here go through JSON.stringify.

import type { BrowserContext, Page } from '@playwright/test';

// SharedPreferencesAsync (the API the bundle uses for these reads)
// does NOT prefix keys with 'flutter.' — that's a legacy of the
// sync SharedPreferencesPlugin. See shared_preferences_web 2.4.3
// _getAllowedKeys exact-match filter.
const ACCESS_TOKEN_KEY = 'access_token';
const REFRESH_TOKEN_KEY = 'refresh_token';
const USER_KEY = 'user';
const SERVER_URL_KEY = 'server_url';

export interface SessionUser {
  id: string;
  name: string;
  /** Optional avatar/media id; pass '' when the user has no profile media. */
  mediaId?: string;
}

export interface InjectAuthOptions {
  accessToken: string;
  refreshToken: string;
  user: SessionUser;
  /**
   * Server URL the bundle resolves to — must match the origin the
   * Playwright browser dials (Environment.getServer on local env
   * uses Uri.base.origin). Typically the same `baseUrl` used to
   * register the user.
   */
  serverUrl: string;
}

/**
 * Pre-inject the four localStorage entries the bundle needs to
 * hydrate as authenticated on the next navigation. Pass either a
 * Page (scopes to one tab) or a BrowserContext (covers every Page
 * in the context — useful for multi-client scenarios in Phase 2).
 * Must be called BEFORE navigating to the target URL.
 */
export async function injectAuth(
  target: Page | BrowserContext,
  opts: InjectAuthOptions,
): Promise<void> {
  await target.addInitScript(
    (args) => {
      const userJson = JSON.stringify({
        id: args.user.id,
        name: args.user.name,
        mediaId: args.user.mediaId ?? '',
      });
      window.localStorage.setItem(args.keys.access, JSON.stringify(args.accessToken));
      window.localStorage.setItem(args.keys.refresh, JSON.stringify(args.refreshToken));
      window.localStorage.setItem(args.keys.user, JSON.stringify(userJson));
      window.localStorage.setItem(args.keys.serverUrl, JSON.stringify(args.serverUrl));
    },
    {
      keys: {
        access: ACCESS_TOKEN_KEY,
        refresh: REFRESH_TOKEN_KEY,
        user: USER_KEY,
        serverUrl: SERVER_URL_KEY,
      },
      accessToken: opts.accessToken,
      refreshToken: opts.refreshToken,
      user: opts.user,
      serverUrl: opts.serverUrl,
    },
  );
}

/**
 * Clear any previously-injected auth state. Useful for scenarios
 * that start authed and then exercise the logged-out flow. Note
 * this clears via addInitScript so it only takes effect on the
 * next navigation; it does not race a currently-loaded bundle.
 */
export async function clearAuth(target: Page | BrowserContext): Promise<void> {
  await target.addInitScript(
    (keys) => {
      window.localStorage.removeItem(keys.access);
      window.localStorage.removeItem(keys.refresh);
      window.localStorage.removeItem(keys.user);
      window.localStorage.removeItem(keys.serverUrl);
    },
    {
      access: ACCESS_TOKEN_KEY,
      refresh: REFRESH_TOKEN_KEY,
      user: USER_KEY,
      serverUrl: SERVER_URL_KEY,
    },
  );
}
