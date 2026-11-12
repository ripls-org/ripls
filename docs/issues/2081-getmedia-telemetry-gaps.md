# Issue #2081: GetMedia Phase 1 telemetry: three gaps before flipping to enforce (#1529)

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/2081
**Filed:** 2026-05-20T22:55:01Z | **Last activity:** 2026-05-20T22:55:01Z
**Labels:** none | **Assignees:** unassigned
**Context docs:** [`docs/issues/1529-getmedia-auth.md`](./1529-getmedia-auth.md) — original Phase 1 design + audit

## Audit log (post-`/ripls-audit`)

Audited against `docs/server/observability.md`. Three changes made:

1. **`media_id` is already on the deny log** via the request-scoped `logger.With(..., "media_id", req.Msg.Id, ...)` at `server/services/media/service.go:397-401`. The plan originally said "add `media_id` to the deny log site" — corrected to "verify it appears in actual emitted log output" with a sub-task to re-run the dev log query and confirm. If `media_id` is absent from emitted JSON, that's a separate bug to file, not the work scoped here.
2. **`operation` field is missing** on the `GetMedia` handler logger. The observability doc's canonical service pattern (§ "Logging Usage Pattern", lines 196-213) sets `"operation": "MethodName"` on the request-scoped logger so the existing `rpc_errors_{environment}` log-based metric (§ Part 4, lines 700-706) can group by operation. Added as a small side-fix in Phase 1.
3. **Log level once enforce flips** is now an explicit Phase 4 consideration. The observability doc (line 65) requires user errors at INFO/WARN, not ERROR. Phase 2 enforce returns `connect.CodePermissionDenied` — must propagate through the logging middleware as WARN (matching 4xx), not ERROR. Verifying this is now a deferred-phase task.

No client- or accessibility-side concerns: this issue is server-only and emits no user-visible strings.

## Problem Statement

PR #2071 shipped Phase 1 of #1529: every `GetMedia` call now invokes `canUserAccessMedia` and emits a structured `would deny media read` WARN log when the eight pass rules (owner / avatar / community / story / gear / experience / request / chat) all fail, but the request still succeeds. The intent was to collect 24-48h of telemetry on dev (and eventually prod) so we can flip to enforce with confidence that legitimate reads aren't being mis-classified.

The first two-hour soak on dev showed the telemetry has three gaps that make it inadequate for that goal:

1. **No pass rule for stock images** (`media.user_id = "system"`). 12 of 71 dev denies are reads against stock media that the access rules don't enumerate. Phase 2 would 403 every stock image in the app.
2. **An unexplained self-deny** (1 of 71). Reader `ad931a71-…` denied against `media_owner_id=ad931a71-…`. The owner rule is a single string compare — a self-deny means the stored `media.UserId` doesn't match the authenticated caller's id, which is either a backfill gap, an auth-middleware mismatch, or a stale-id format issue.
3. **Reason taxonomy collapse on deny.** Every deny logs `reason=deny`. Pass paths name the matching rule; fail paths don't surface which rule was the closest miss, so a real production deny is a black box.

The three findings are bundled because they all gate the same decision: flipping `canUserAccessMedia` from log-only to enforce. Issue 3 is the prerequisite for cleanly diagnosing 1 and 2 in production logs, and 1 is the highest-impact correctness fix.

## Context from Comments

No comments — I filed the issue based on dev-log telemetry analysis. The body already includes ranked fix options (A: per-rule deny codes; B: bitmask of probed rules) and the ordering rationale (3 → 1 → 2). Nothing to disambiguate from comment history.

## Potential Solutions

### Option A: Per-rule deny codes + system rule + investigation (recommended)

**Approach:**
- **Issue 3 (telemetry):** Introduce a small set of deny-reason constants (`accessReasonDenyNoAvatar`, `accessReasonDenyNoCommunityLink`, `accessReasonDenyNoStoryCommunity`, …). Each per-surface helper (`communityDirectAllows`, `storyAllows`, etc.) returns `(bool, string, error)` instead of `(bool, error)`, where the string is the deny reason on a clean false. `canUserAccessMedia` accumulates the per-surface deny reasons and returns the most-specific one. Add `media_id` to the log site so a real deny is debuggable to a specific row.
- **Issue 1 (system rule):** Add `accessReasonSystem` and a new short-circuit at step 1.5 (after owner, before avatar): `if media.UserId == systemUserID { return true, accessReasonSystem, nil }`. Move `systemUserID` from `server/media/unsplash_provider.go` to a package-level constant in `server/services/media/authorization.go` (or expose it from a shared spot) so the rule has a stable reference.
- **Issue 2 (self-deny investigation):** Pull the specific `media_id` from the dev `would_deny` log entry, inspect the row, decide whether it points at a backfill / auth / format issue. Fix or file follow-up. No code changes if the row turns out to be benign (e.g. orphan).

**Pros:**
- Each phase is shippable and gates the next: Phase 1 (telemetry) unblocks Phase 2 (system rule) diagnosis if it surfaces more gaps; Phase 3 (investigation) only happens once we know what the deny reason actually was.
- The system rule is the highest-impact correctness fix and the smallest code change (≈5 lines + tests).
- Per-rule deny codes match the existing pass-side taxonomy, so log readers don't need a second mental model.

**Cons:**
- Requires the per-surface helpers to grow a third return value, which is a small ergonomic cost in the helper signatures (acceptable — the helpers are internal).
- Phase 2 still depends on more dev soak after Phase 1 lands; not strictly parallel.

**Effort:** medium. Phase 1 is the bulk (≈80 lines + tests); Phase 2 is small (≈10 lines + tests); Phase 3 is investigation only.

### Option B: Bitmask of probed rules (no helper signature changes)

**Approach:** Keep `accessReasonDeny` as the umbrella code, but add a new `probed` log field that lists which of the eight rules were evaluated and returned false. Don't change helper signatures — keep `(bool, error)` — and track the probed-but-failed set in `canUserAccessMedia` itself. Issue 1 and Issue 2 are unchanged.

**Pros:**
- Smaller diff for the telemetry change; no helper signature churn.
- Mechanical and unambiguous: `probed=owner,avatar,community,story,gear,experience,request,chat` tells you exactly what ran.

**Cons:**
- Log readers learn nothing new — knowing all eight ran and all eight failed is the existing state for any clean deny. Useful only on partial probes (which we currently don't have).
- Doesn't capture the *near-miss* semantic (e.g. "membership lookup ran but caller wasn't a member" vs. "no community held this media at all"), which is the information a debugger actually wants.
- Encourages the antipattern of digging into the log fields rather than reading a single specific reason string.

**Effort:** small.

### Option C: Add a metric, not a log

**Approach:** Instead of (or in addition to) richer log fields, emit a per-deny counter labeled by the closest-miss rule, surfaced on the existing observability stack.

**Pros:** Aggregable. Surfaces patterns ("90% of denies are no-chat-share") without log spelunking.
**Cons:** Doesn't help diagnose an individual user's broken image. Logs and metrics serve different jobs — we want both, but the *user-reports-broken-image* path needs the log. Best deferred to a follow-up once the log shape is finalized.

**Effort:** medium, deferred.

## Recommendation

**Option A.** The per-rule deny codes are worth the helper signature churn because they make Phase 2 enforce actually safe to land — without them, a user reporting "I can't see this image" is unfixable from logs alone. The system rule (Issue 1) is the smallest possible safety net for the highest-impact regression risk on Phase 2 flip. The self-deny investigation (Issue 2) is too small to plan for in detail — handle it as a single inspection task after Phase 1 lands and the reason taxonomy makes it readable.

Add Option C (per-deny counter) to the follow-up backlog once Phase 1 lands and we know the deny-reason vocabulary is stable.

## Open Questions

- **Are there other sentinel `user_id` values besides `"system"`?** — Recommendation: only `"system"` is used today (confirmed via `grep -rn 'UserId.*=.*"system"' server/`). All four stock-image providers (Unsplash, Pixabay, Pexels, the fake provider used in dev) write through the same `systemUserID` constant. No other sentinel is referenced from `server/media/`. The plan should still add a defensive assertion or test that fails build if a new sentinel is introduced without an access rule.

- **Should `accessReasonSystem` be the rule name, or should we collapse it into `accessReasonOwner` since both are zero-SQL short-circuits?** — Recommendation: separate code. The telemetry signal that "this read is a stock image" is materially different from "this read is your own upload," and we'll want to count stock-image reads separately when tuning Pexels/Pixabay quotas.

- **Phase 2 enforce: do we flip all eight rules at once or stage per-surface?** — Recommendation: stage. After Phase 1 + Phase 2 of *this* issue land and dev soaks for another 24-48h, flip enforce one surface at a time in this order: stock (lowest blast radius) → community-direct → story → gear → experience → request → chat. The avatar rule needs special care because it's used cross-community; flip it last. Each flip is a separate small PR gated on the prior surface having zero unexpected denies.

- **Do we need to log `media_kind`?** — Recommendation: defer. Deriving kind (avatar / item-photo / chat-attachment / story-frame / community-hero / stock) requires either a denormalized column or a follow-up lookup, and the per-rule pass code already tells us the matching surface for non-deny cases. For denies, the deny-reason taxonomy gives the relevant signal. Re-evaluate after the first prod deny report shows what context is missing.

- **Is there a metric pipeline already in place we'd reuse for Option C?** — Recommendation: defer. I haven't confirmed the metrics shape on the current server. Add a follow-up issue after Phase 1 ships.

## Implementation Plan

### Phase 1: Per-rule deny taxonomy + media_id logging

**Goal:** Make any `would_deny` log entry self-sufficient — a reader can identify the failing surface and the specific media row without leaving the log.

**Tasks:**
- [ ] Add deny-reason constants in `server/services/media/authorization.go` (keep `accessReasonDeny` as the umbrella code; add one specific deny per rule):
  - `accessReasonDenyNotOwnerOrAvatar` — owner + avatar both checked and missed (these are the universal short-circuits, so a single rolled-up label is clearer than two).
  - `accessReasonDenyNoCommunityLink` — no Community holds this media.
  - `accessReasonDenyNoCommunityMembership` — a Community does hold it but caller isn't a member.
  - `accessReasonDenyNoStoryCommunity` / `accessReasonDenyNoStoryMembership` — Story equivalents.
  - `accessReasonDenyNoGearLink` / `accessReasonDenyNoGearCommunityMembership` — Gear via CommunityGear join.
  - `accessReasonDenyNoExperienceLink` / `accessReasonDenyNoExperienceCommunityMembership`.
  - `accessReasonDenyNoRequestLink` / `accessReasonDenyNoRequestCommunityMembership`.
  - `accessReasonDenyNoChatMessage` / `accessReasonDenyNoChatParticipation`.
- [ ] Change the per-surface helper signature from `(bool, error)` to `(bool, string, error)`:
  - `communityDirectAllows`, `storyAllows`, `gearAllows`, `experienceAllows`, `requestAllows`, `chatAllows`.
  - On `false` with no error, return the specific deny reason. The "no link" code fires when the array-contains probe yields zero hits; the "no membership" code fires when hits exist but no membership matched.
- [ ] Update `canUserAccessMedia` to track the *deepest* deny reason it observes (i.e. the most specific surface it actually exercised). When all eight rules miss, return that as the reason. Keep `accessReasonDeny` only as a defensive fallback if no rule ran (shouldn't happen).
- [ ] **Verify `media_id` is in emitted deny logs.** The request-scoped logger at `server/services/media/service.go:397-401` already calls `.With("media_id", req.Msg.Id, …)`, so every `logger.WarnContext` inherits it. Sub-task: re-run the dev log query and confirm `jsonPayload.media_id` is present on `would deny media read` entries. If absent, that's a separate logger-inheritance bug — file it and unblock by passing `media_id` explicitly to the deny call.
- [ ] **Add `operation: "GetMedia"` to the request-scoped logger** at `service.go:397-401`. The observability doc's canonical pattern (§ "Logging Usage Pattern") sets `"operation"` on every service-method logger so the existing `rpc_errors_{environment}` log-based metric (§ Part 4) can group by RPC. Small side-fix; pairs naturally with the deny-log work since both touch the same `.With(...)` block.
- [ ] Extend `server/services/media/authorization_test.go`:
  - `TestCanUserAccessMedia_UnrelatedDenied` (line 221) — assert the returned reason is the specific deny code, not just `accessReasonDeny`.
  - Add `TestCanUserAccessMedia_DenyReasonForEachSurface` — table-driven, one row per (surface-hits-but-no-membership) case, asserting the right `accessReasonDenyNo…Membership` fires.

**Validates:** A `would_deny` log entry on dev names the specific surface that almost-passed. Run a few manual reads against media you shouldn't have access to and confirm the log distinguishes "no community link at all" from "wrong community member."

### Phase 2: System (stock-image) pass rule

**Goal:** Stop denying stock-image reads. Eliminates the largest known false-positive deny class before Phase 2 enforce.

**Tasks:**
- [ ] Add `accessReasonSystem` to the constants block at `authorization.go:26-37`.
- [ ] Promote `systemUserID = "system"` from `server/media/unsplash_provider.go:21` to a shared location. Two viable spots: (a) `server/media/system_owner.go` (new file) re-exported from `server/services/media/authorization.go`; (b) duplicate the constant inline in `authorization.go` with a comment pointing at the source of truth. Pick (a) — single source of truth.
- [ ] Insert the rule as step 1.5 in `canUserAccessMedia`, between the owner short-circuit (line 58-60) and the avatar probe (line 65):
  ```go
  // 1.5. System-owned (stock images). Zero SQL.
  if media.UserId == media.SystemUserID {
      return true, accessReasonSystem, nil
  }
  ```
- [ ] Tests in `authorization_test.go`:
  - `TestCanUserAccessMedia_System` — media with `UserId="system"` returns `(true, accessReasonSystem, nil)` for any caller, including an authenticated user with no relation to the media.
  - Negative test: media with `UserId="systemX"` or `UserId=""` is **not** treated as system.
- [ ] Add a one-line note in `docs/issues/1529-getmedia-auth.md` recording the rule expansion (the design doc currently lists 8 rules; this makes 9).

**Validates:** Deploy to dev, exercise a flow that surfaces stock images (community hero, default avatar fallback if any). Confirm zero `would_deny` against `media_owner_id=system` in the next two-hour log window.

### Phase 3: Self-deny investigation

**Goal:** Determine whether the 1-of-71 self-deny in the dev sample indicates a real bug or a benign edge case.

**Outcome (2026-05-20):** Did not reproduce. Re-queried the dev `would_deny` log over a 72h window after Phase 1 landed — 74 total deny events, **zero** self-denies (`user_id == media_owner_id` rows where owner is not the `system` sentinel). The single event from the original 2h sample was a one-off — most likely a transient race during a media write or a brief mid-update read.

No code change. Investigation closed. If this recurs as a real `PermissionDenied` after Phase 4 enforce-flip, Phase 1's per-rule deny taxonomy will surface it again with `media_id` in the log line, and we can re-open from there.

**Tasks:**
- [x] Re-query dev `would_deny` for entries where `user_id == media_owner_id` and owner is not `system`. Result: 0 in 72h.
- [x] Document outcome inline. No follow-up issue filed.

### Phase 4 (deferred — out of scope of #2081): Per-surface enforce flip

**Goal:** Convert log-only to enforce. **Not part of this issue** — included here only so the reader has visibility on what Phase 1-3 unblock.

After Phase 1-3 land and dev soaks for 24-48h with no unexpected denies, flip enforce one surface at a time in PR-per-surface increments:

1. system (no risk — purely additive)
2. community-direct
3. story
4. gear
5. experience
6. request
7. chat
8. avatar (last, because it's cross-community)

**Log-level invariant for the enforce flip:** the deny path returns `connect.NewError(connect.CodePermissionDenied, …)`. Per `docs/server/observability.md` line 65, user errors must log at INFO or WARN, not ERROR. Before flipping enforce, verify the logging middleware (`server/middleware/logging.go`) classifies `CodePermissionDenied` responses as WARN (matching 4xx). If it classifies them as ERROR, every legitimate deny will fire the `rpc_errors_{environment}` log-based metric and the "RPC Errors Elevated" alert policy (§ Part 4), creating alert-fatigue noise. Fix the middleware classification *before* flipping.

File as #1529 Phase 2.

## Related Issues

- #1529 — Parent: per-resource read authorization for GetMedia. This issue blocks #1529 Phase 2.
- #2071 — Phase 1 implementation that surfaced this telemetry.
- #2080 — Unrelated client-side issue (chat media flicker) filed in the same session; mentioned only so cross-references don't surprise a reviewer.
- #2072 / #2075 — Underlying TEXT[] auto-sync infrastructure the access rules depend on. Already shipped.
