#!/usr/bin/env node
/**
 * check_active_community_gate.js — guard against forgotten deleted-community
 * filtering in community-scoped read paths.
 *
 * Background: #1621 added a 30-day soft-delete state for communities. Every
 * read path that scopes by community must skip soft-deleted communities,
 * unless the caller has explicitly opted in (Settings → Communities deleted
 * list, admin cleanup paths). The active-community gate lives in
 * server/auth — `RequireMemberOfActiveCommunity`, `RequireActiveCommunity`,
 * `FilterActiveMemberCommunities`. This script catches handlers that query
 * by community_id without going through one of those helpers.
 *
 * Four rules:
 *
 *   Rule A — `RequireUserIsCommunityMember` is banned in server/services/.
 *     The pre-#1621 helper does not check Community.deleted; replacement is
 *     `RequireMemberOfActiveCommunity` (see server/auth/community_active.go).
 *
 *   Rule B — Any file under server/services/ whose RPC handlers explicitly
 *     receive a community ID from the request (`req.Msg.CommunityId` or
 *     `req.Msg.CommunityIds`) must also reference at least one of the
 *     active-community gates. This catches the real failure mode: a new
 *     RPC that takes a community ID but forgets to validate the community
 *     is active. Files that do community-scoped storage queries downstream
 *     of an entity-level authorization (Gear ownership, Transfer
 *     participation, Experience host, etc.) are not flagged because their
 *     authorization model is different.
 *
 *   Rule C — Any file in server/community/ or server/services/ that
 *     dispatches push notifications (calls `.NotifyUser(`) must also
 *     reference `community.IsActive`. The dispatcher gate suppresses
 *     pushes for soft-deleted communities; #1622 introduced it as a
 *     single chokepoint shared by the community-event and chat
 *     dispatchers. #1623 generalized the helper from
 *     MaySendNotification to IsActive so the same chokepoint covers
 *     async write guards too. Catches a forgetful future-author who
 *     adds a third dispatcher path.
 *
 *   Rule D — Any file under server/services/ or server/jobs/ that
 *     contains BOTH (a) a goroutine spawn (`logging.GoSafe(` or
 *     `go func`) AND (b) a community-scoped storage write (heuristic:
 *     a `"community_id"` literal in a field map AND a
 *     `.storage.Update(`/`.storage.Insert(` call) must reference
 *     `community.IsActive`. Catches a forgetful future-author who
 *     adds a new async background-write path scoped to a community
 *     without going through the gate. False positives (existing
 *     async paths that don't actually write community-scoped data)
 *     are silenced via RULE_D_ALLOWLIST below with a one-sentence
 *     rationale.
 *
 * Exempt directories:
 *   server/services/admin/  — admin cleanup paths legitimately read deleted
 *                             communities for purge/restore tooling.
 *   _test.go files          — tests legitimately exercise the unfiltered
 *                             paths and force-flip deleted state directly.
 *   generated code          — server/gen/.
 *
 * Exempt files (Rule C only):
 *   server/community/notifications.go — defines IsActive itself.
 *   server/notifications/             — generic FCM provider layer; doesn't
 *                                        know about communities by design.
 *
 * Usage:
 *   node scripts/check_active_community_gate.js
 */

const fs = require('fs');
const path = require('path');

const ROOT = path.join(__dirname, '..');
const SERVER_DIR = path.join(ROOT, 'server');
const SERVICES_DIR = path.join(SERVER_DIR, 'services');
const COMMUNITY_DIR = path.join(SERVER_DIR, 'community');
const JOBS_DIR = path.join(SERVER_DIR, 'jobs');

// File-path prefixes that the rule does not apply to.
const EXEMPT_PREFIXES = [
  path.join(SERVICES_DIR, 'admin') + path.sep,
];
const EXEMPT_SUFFIXES = ['_test.go'];

// Rule C: the helper definition file itself, and any caller that legitimately
// bypasses the gate (none today). The generic FCM provider layer
// (server/notifications/) is not in scope for Rule C — it doesn't know about
// communities by design and shouldn't gain that knowledge.
const RULE_C_EXEMPT_FILES = new Set([
  path.join(COMMUNITY_DIR, 'notifications.go'),
]);

const BANNED_RULE_A = /\bauth\.RequireUserIsCommunityMember\b/;

// Rule B trigger: an RPC handler in this file consumes a community ID from
// the request message (req.Msg.CommunityId or req.Msg.CommunityIds). That's
// the only context where "did I authorize the community?" is the right
// question. Internal helpers that take a community_id parameter from an
// already-authorized caller are not RPC entry points and are not flagged.
const REQ_MSG_COMMUNITY_ID = /\breq\.Msg\.CommunityIds?\b/;

// Any one of these references is enough to pass Rule B for the file.
const GATE_PATTERNS = [
  /\bauth\.RequireMemberOfActiveCommunity\b/,
  /\bauth\.RequireActiveCommunity\b/,
  /\bauth\.FilterActiveMemberCommunities\b/,
];

// Rule C trigger: any call that fans out a push notification.
// The mock implementations of NotificationService also expose .NotifyUser(,
// but those live in test files (already exempt) or in server/notifications/
// (the generic provider layer, exempt by RULE_C_EXEMPT_FILES below).
const NOTIFY_USER_CALL = /\.NotifyUser\(/;

// Rule C gate: callers outside the helper's home package must use the
// fully-qualified `community.IsActive`. This is strict on purpose: an
// unqualified `\bIsActive\b` would let a typo'd import alias
// (`community_DISABLED.IsActive`) silently pass the gate. The home file
// (server/community/notifications.go) is already exempt via
// RULE_C_EXEMPT_FILES, so the strict qualified pattern covers every
// legitimate caller.
const RULE_C_GATE_PATTERNS = [
  /\bcommunity\.IsActive\b/,
];

// Rule D triggers (both must be present in the file):
//   - GOROUTINE_SPAWN: file launches a background goroutine
//   - COMMUNITY_ID_LITERAL: a "community_id" field-map literal — the
//       conventional shape at any community-scoped storage call site
//       (`map[string]any{"community_id": ...}` for QueryByFields, etc.).
//
// We deliberately don't also require a direct `.Update(`/`.Insert(`
// in the same file: the dominant pattern is "spawn goroutine in
// caller → call helper in another file → helper does the write."
// Requiring the write to be visible in the same file misses most
// real cases (verified: feed/nudges.go delegates to insertNudge in
// nudge_storage.go; request/lifecycle.go delegates to
// fetchAndAttachStockImage in stock_imagery.go). The goroutine +
// community_id-literal conjunction is specific enough on its own;
// false positives (background goroutines that touch community
// queries but don't write) are silenced via RULE_D_ALLOWLIST.
const GOROUTINE_SPAWN = /logging\.GoSafe\(|go func\(/;
const COMMUNITY_ID_LITERAL = /"community_id"/;

// Rule D allowlist: relative paths from repo root. Files here have a
// goroutine + community write but do not need IsActive — typically
// because the write isn't actually community-scoped, or because the
// containing function is exempt for a different documented reason.
// Each entry should have a one-sentence rationale.
const RULE_D_ALLOWLIST = new Map([
  // save.go's goroutines are LLM inference calls that do not write
  // community-scoped data; the synchronous EXPERIENCE_UPDATED fan-out
  // that uses "community_id" is not inside a goroutine.
  ['server/services/experience/save.go', 'LLM goroutines do not write community-scoped data; EXPERIENCE_UPDATED fan-out is synchronous.'],
]);

function isExempt(filePath) {
  if (EXEMPT_PREFIXES.some((p) => filePath.startsWith(p))) return true;
  if (EXEMPT_SUFFIXES.some((s) => filePath.endsWith(s))) return true;
  return false;
}

function walkGo(dir, out = []) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      walkGo(full, out);
    } else if (entry.isFile() && entry.name.endsWith('.go')) {
      out.push(full);
    }
  }
  return out;
}

function ruleAViolations(files) {
  const out = [];
  for (const file of files) {
    const src = fs.readFileSync(file, 'utf-8');
    const lines = src.split('\n');
    for (let i = 0; i < lines.length; i++) {
      if (BANNED_RULE_A.test(lines[i])) {
        out.push({ path: path.relative(ROOT, file), line: i + 1 });
      }
    }
  }
  return out;
}

function ruleBViolations(files) {
  const out = [];
  for (const file of files) {
    const src = fs.readFileSync(file, 'utf-8');
    if (!REQ_MSG_COMMUNITY_ID.test(src)) continue;
    if (GATE_PATTERNS.some((p) => p.test(src))) continue;
    out.push({ path: path.relative(ROOT, file) });
  }
  return out;
}

// ruleCViolations walks server/community/ and server/services/ for files
// that call .NotifyUser( without referencing community.IsActive.
function ruleCViolations() {
  const out = [];
  const dirs = [COMMUNITY_DIR, SERVICES_DIR];
  for (const dir of dirs) {
    for (const file of walkGo(dir)) {
      if (file.endsWith('_test.go')) continue;
      if (RULE_C_EXEMPT_FILES.has(file)) continue;
      // Rule A's admin exemption applies here too.
      if (EXEMPT_PREFIXES.some((p) => file.startsWith(p))) continue;
      const src = fs.readFileSync(file, 'utf-8');
      if (!NOTIFY_USER_CALL.test(src)) continue;
      if (RULE_C_GATE_PATTERNS.some((p) => p.test(src))) continue;
      out.push({ path: path.relative(ROOT, file) });
    }
  }
  return out;
}

// ruleDViolations walks server/services/ and server/jobs/ for files that
// spawn goroutines AND do community-scoped storage writes without going
// through the community.IsActive gate. Heuristic — false positives are
// silenced via RULE_D_ALLOWLIST.
function ruleDViolations() {
  const out = [];
  const dirs = [SERVICES_DIR, JOBS_DIR];
  for (const dir of dirs) {
    if (!fs.existsSync(dir)) continue;
    for (const file of walkGo(dir)) {
      if (file.endsWith('_test.go')) continue;
      // Admin exemption applies here too.
      if (EXEMPT_PREFIXES.some((p) => file.startsWith(p))) continue;
      const rel = path.relative(ROOT, file);
      if (RULE_D_ALLOWLIST.has(rel)) continue;
      const src = fs.readFileSync(file, 'utf-8');
      if (!GOROUTINE_SPAWN.test(src)) continue;
      if (!COMMUNITY_ID_LITERAL.test(src)) continue;
      // Pass only on the qualified form. Rule D walks services/ and
      // jobs/ — neither is the helper's home package, so a typo'd
      // import alias (community_DISABLED.IsActive) must not silently
      // pass. Mirrors Rule C's strict-qualified pattern.
      if (/\bcommunity\.IsActive\b/.test(src)) continue;
      out.push({ path: rel });
    }
  }
  return out;
}

function main() {
  const allFiles = walkGo(SERVICES_DIR).filter((f) => !isExempt(f));

  const aViolations = ruleAViolations(allFiles);
  const bViolations = ruleBViolations(allFiles);
  const cViolations = ruleCViolations();
  const dViolations = ruleDViolations();

  if (
    aViolations.length === 0 &&
    bViolations.length === 0 &&
    cViolations.length === 0 &&
    dViolations.length === 0
  ) {
    console.log('check_active_community_gate passed: 0 violations.');
    return;
  }

  if (aViolations.length > 0) {
    console.error(
      `check_active_community_gate: ${aViolations.length} call(s) to the deprecated auth.RequireUserIsCommunityMember:`,
    );
    for (const v of aViolations) console.error(`  ${v.path}:${v.line}`);
    console.error('');
    console.error(
      'Use auth.RequireMemberOfActiveCommunity instead. It checks community.deleted',
    );
    console.error('in the same JOIN query, returns the Community + CommunityUser, and rejects');
    console.error('soft-deleted communities. See server/auth/community_active.go.');
    console.error('');
  }

  if (bViolations.length > 0) {
    console.error(
      `check_active_community_gate: ${bViolations.length} RPC handler file(s) consume req.Msg.CommunityId(s) without an active-community gate:`,
    );
    for (const v of bViolations) console.error(`  ${v.path}`);
    console.error('');
    console.error(
      'Each file must reference one of:',
    );
    console.error('  - auth.RequireMemberOfActiveCommunity (single community + member required)');
    console.error('  - auth.RequireActiveCommunity (single community, no membership)');
    console.error('  - auth.FilterActiveMemberCommunities (multi-community, batched)');
    console.error('');
    console.error(
      'See docs/community_delete_and_leave.md §6.5 and docs/issues/1621-deleted-community-read-filter.md.',
    );
  }

  if (cViolations.length > 0) {
    console.error(
      `check_active_community_gate: ${cViolations.length} file(s) call .NotifyUser( without an active-community gate:`,
    );
    for (const v of cViolations) console.error(`  ${v.path}`);
    console.error('');
    console.error(
      'Each file must reference community.IsActive before calling .NotifyUser(.',
    );
    console.error('The helper suppresses pushes for soft-deleted communities while preserving');
    console.error('the audit trail (CommunityEvent / ChatMessage rows are still recorded).');
    console.error('See docs/community_delete_and_leave.md §12 and docs/issues/1622-notification-dispatcher-deleted-guard.md.');
  }

  if (dViolations.length > 0) {
    console.error(
      `check_active_community_gate: ${dViolations.length} file(s) spawn a goroutine + write community-scoped data without an active-community gate:`,
    );
    for (const v of dViolations) console.error(`  ${v.path}`);
    console.error('');
    console.error(
      'Each file must reference community.IsActive before the async write,',
    );
    console.error('or be added to RULE_D_ALLOWLIST in this script with a one-sentence rationale');
    console.error('(typical reason: the goroutine does not actually write community-scoped data).');
    console.error('See docs/community_delete_and_leave.md §12 and docs/issues/1623-background-job-write-guard.md.');
  }

  process.exit(1);
}

main();
