// filmed-at.ts — the "now" a run is filmed against (#2724).
//
// A reel's story often implies a date: a Mother's Day brunch has to fall on a
// Mother's Day. Left to the wall clock, only dates a few days out are
// reachable, and they drift with every re-render.
//
// So a reel may declare the moment it is filmed at, and everything that reads
// a clock is told the same thing:
//
//   - the SERVER, via the X-Simulation-Timestamp header on every request —
//     both the seed RPCs here and the app's own calls from the page. It
//     resolves relative dates in prompts ("this Sunday"), and stamps the
//     timestamps chat rows are rendered from.
//   - the BROWSER, via Playwright's clock, which reaches Dart's
//     `DateTime.now()` through the wasm bundle — that is what turns a stored
//     start time into "3d from now".
//
// Both or neither: a page that believes it is May reading rows the server
// stamped in July renders "2 months ago" on a message sent moments ago. This
// module exists so the two cannot be set apart.
//
// The header is honoured only under --dev-mode (server/routes.go), and the
// forecast a filmed date needs comes from --mock-weather-provider rather than
// a live service that only covers the next two weeks.

/** The declared moment, or null when a run just uses the wall clock. */
let filmedAtMs: number | null = null;

/**
 * Declares the moment this run is filmed at. Call before seeding or opening
 * any scene — it governs both the seed RPCs and every page.
 */
export function setFilmedAt(when: Date): void {
  filmedAtMs = when.getTime();
}

/** The declared moment as a Date, or null when unset. */
export function filmedAt(): Date | null {
  return filmedAtMs === null ? null : new Date(filmedAtMs);
}

/** The declared moment as Unix seconds, or null when unset. */
export function filmedAtUnixSec(): number | null {
  return filmedAtMs === null ? null : Math.floor(filmedAtMs / 1000);
}

/** Header the server's dev-mode simulation clock reads. */
export const SIMULATION_TIMESTAMP_HEADER = 'X-Simulation-Timestamp';
