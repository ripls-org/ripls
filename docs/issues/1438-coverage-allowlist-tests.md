# Issue #1438: Add Go test coverage to 4 packages on the coverage allowlist

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/1438
**Filed:** 2026-04-26 | **Last activity:** 2026-04-27
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

`.github/coverage-allowlist.txt` (added in #1437) currently exempts four
packages from the "every Go package must have tests" CI gate:

- `type2fun.org/ripls/server/firebase`
- `type2fun.org/ripls/server/health`
- `type2fun.org/ripls/server/planning`
- `type2fun.org/ripls/server/undo/testing`

The allowlist is meant to be temporary; this issue is the cleanup tracker
referenced by the comment at the top of the file. Each package needs either
real test coverage *or* a structural change that removes its need to be a
"regular package" under the gate. Once tests land, the same PR removes the
allowlist line — CI already emits a warning when an allowlist entry has tests
but is still listed.

The codebase already has good models for each shape of package:

- `server/notifications/fcm/provider_test.go` (a fake `messageSender` —
  no live network) shows how the codebase tests Firebase-adjacent code.
- `server/services/{experience,request}/*_test.go` show how
  `planning.*` is exercised end-to-end against a real PostgreSQL test
  container via `storage.SetupTestStorage`.
- `server/services/experience/undo_roundtrip_test.go` and
  `server/services/request/undo_test.go` already use
  `undotesting.RoundTrip` against real services — i.e., the
  test-harness package is exercised in production-shaped tests, just not
  in its own package.

## Context from Comments

The only comment on the issue is `@claude` (drive-by trigger). No
maintainer clarification beyond the body, so the acceptance criteria as
written are authoritative:

- `server/firebase`: add a `*_test.go` covering the FCM provider's
  exported surface (likely with a fake transport).
- `server/health`: add a `*_test.go` covering the health-check handler.
- `server/planning`: add a `*_test.go` *or* move/delete if no longer used.
- `server/undo/testing`: add tests *or* rename/restructure so it isn't a
  regular package needing tests (e.g. fold into `undo` as test-only
  helpers).
- Remove each landed package from `.github/coverage-allowlist.txt` in
  the same PR.

One small mismatch with the body: the issue body describes
`server/firebase` as "Firebase Cloud Messaging push provider" and
`server/planning` as "dev-only planning utility." Both are slightly off.
`server/firebase` is a thin Firebase-app initializer shared by FCM and
phone auth; the actual FCM push provider lives at
`server/notifications/fcm/`. `server/planning` is the shared
needs/contributions library used by both the experience and request
services (see `server/planning/README.md` and the call sites in
`server/services/{experience,request}/`). It is **not** a dev-only
utility and definitely should not be deleted. The plan below treats both
as production code that needs real tests.

## Potential Solutions

### Option A — One PR, one test file per package

Add a single `*_test.go` to each of the four package directories in one
PR, using whatever pattern the package's surface naturally suggests
(unit tests for the pure/init code, integration-flavored tests via
`storage.SetupTestStorage` for `planning`). Remove all four allowlist
entries in the same PR.

- **Approach:** Land everything together. Mirrors the PR-shape implied
  by the acceptance criteria ("removed … in the same PR").
- **Pros:** Closes the issue in one merge. Easy to review the full
  picture: tests + allowlist removal. Smallest diff to the allowlist
  comment header (which currently points at #1438 — the file isn't
  empty after this PR, but the four entries are gone).
- **Cons:** Larger PR. If one package's tests are flaky or contentious,
  the whole PR is held up. ~600–900 LOC total of new test code.
- **Effort:** Medium.

### Option B — Four sequential PRs, one per package

Split each package into its own PR, each removing one line from the
allowlist.

- **Approach:** Iterative. Start with the easiest (`server/health`),
  finish with `server/planning`.
- **Pros:** Smaller, easier-to-review PRs. Failures in one package's
  tests don't block the others.
- **Cons:** Four times the review/merge overhead. The CI warning about
  "stale allowlist entries" will fire repeatedly until the last PR
  lands. Issue stays open longer.
- **Effort:** Medium — same total work, more process overhead.

### Option C — Restructure `server/undo/testing` into `_test.go` helpers in `server/undo`, write tests for the other three

The issue explicitly suggests this for `server/undo/testing`: "fold into
`undo` as test-only helpers." Concretely, rename `roundtrip.go` to
`roundtrip_test.go` and move it into `server/undo/`, so the file is
test-only and the package directory disappears entirely. Then write
real tests for `firebase`, `health`, `planning`.

- **Approach:** Mix of rewrite and refactor. For the three remaining
  packages: same as Option A.
- **Pros:** Eliminates the package entirely — there's nothing left to
  test in `server/undo/testing` because it's gone. Honest signal to
  CI: this was always test code, now it's labeled as such.
- **Cons:** **Won't compile as written.** Go's `_test.go` files in
  `package undo` aren't importable from sibling packages
  (`server/services/experience/undo_roundtrip_test.go`,
  `server/services/request/undo_test.go`) — those tests live in
  *different* packages and can only consume exported symbols from a
  non-test file. The current `server/undo/testing` package is reachable
  by other packages' tests precisely *because* it's a regular package
  with exported symbols. Folding it into `_test.go` files breaks both
  consumers. The "rename to make it a test-only package" idea would
  need to instead move into a separate package like
  `server/undotest` whose `.go` files are still regular (not
  `_test.go`) — which doesn't actually solve the gate problem; it just
  renames the allowlist entry.

### Option D — Option A plus a small amount of testability surface for `server/firebase`

Same as A, but acknowledge upfront that `server/firebase.NewApp` is
hard to unit-test cleanly because it instantiates a real
`firebase.App` from credentials. The realistic test coverage is:
`os.ReadFile` failure (bogus credentials path), the ADC branch
returning a valid app handle (Firebase's `NewApp` happily constructs
an app object even without real credentials — it only fails on first
RPC), and bytes-level assertions on the credentials path string in
log output. We accept that the happy path isn't truly verified end-
to-end at unit level; the FCM provider tests at
`server/notifications/fcm/` already cover the live integration.

- **Approach:** Same as A, with `firebase` tests being honest about
  their scope: error paths + that `NewApp` returns a non-nil handle
  for the ADC and credentials-bytes branches.
- **Pros:** No code-shape changes needed in `server/firebase`. Tests
  document what's verifiable.
- **Cons:** `server/firebase` test coverage is shallow — but matches
  the package's actual surface (a thin wrapper over
  `firebase.NewApp`).
- **Effort:** Medium.

## Recommendation

**Option D** — bundle everything into one PR, write honest tests for
each package, and don't try to restructure `server/undo/testing`
(Option C is incorrect; see its Cons).

Rationale:

1. The acceptance criteria explicitly couples the test additions to the
   allowlist removals ("Each landed package is removed … in the same
   PR"). One PR satisfies that contract literally.
2. Each of the four packages is small (firebase is ~45 lines, health is
   ~33 lines, planning is ~450 lines across 5 files, undo/testing is
   ~140 lines). The total test addition is moderate — bundling them is
   cheaper to review than four PR cycles.
3. `server/undo/testing` already has *consumers* in two service
   packages. The fastest way to add coverage is to add a
   `roundtrip_test.go` *inside* `server/undo/testing/` that exercises
   `RoundTrip`'s success/failure paths with toy proto messages — no
   storage container needed. This is genuine test coverage of the
   harness, not a paperwork shuffle, and the issue allows it ("either
   has tests or is renamed/restructured").
4. `server/planning` deserves real integration tests against a
   PostgreSQL container — `ListNeedsAndContributions`, `ClaimSlot`, and
   `ReleaseSlot` all touch storage and have non-trivial behavior
   (slot atomicity, fully-claimed-need exclusion, best-effort blob
   resync). Pure-unit tests using `MockScopeLoader` cover the scope
   plumbing, but the storage-coupled functions need a real DB. Tests
   in `server/services/{experience,request}` already do this — we
   re-use the same `storage.SetupTestStorage(t)` pattern.

## Open Questions

1. **Should the `server/firebase` tests skip when no working network /
   credentials are available?**
   Recommendation: No — there's no live network call. `firebase.NewApp`
   constructs a Go object that defers all network I/O until the first
   service call (`Messaging`, `Auth`, etc.). The ADC branch will succeed
   in CI without credentials configured because `firebase.NewApp(ctx,
   nil)` doesn't actually validate ADC at construction time; it just
   stores the resolution strategy. We assert on a non-nil return and
   the logged credentials_path field. If `firebase.NewApp` ever
   *does* fail at construction time in CI, we can guard with `t.Skip`
   then. Don't pre-empt.

2. **Should `server/planning` tests use the live `chat.SystemMessageWriter`
   or a fake?**
   Recommendation: The chat helpers (`PostNeedAdded`, etc.) are
   thin shims over `chat.SystemMessageWriter`. Use a fake/stub
   `SystemMessageWriter` (or a lightweight mock that records calls)
   for the chat-helper tests. For `ListNeedsAndContributions` /
   `ClaimSlot` / `ReleaseSlot` use real storage. Mirrors the
   "test what each function actually does" principle.

3. **Should `server/undo/testing.RoundTrip` tests use a real proto
   message type or a `proto.Message` stand-in?**
   Recommendation: Use a real generated proto from
   `server/gen/ripls/models` (e.g., `models.Experience` or
   `models.User`) so the `proto.Equal` and `prototext.Format` paths
   exercise generated code, not a hand-rolled stub. Build snapshots
   in-memory; no storage needed. The harness itself is pure logic
   over `proto.Message` interfaces.

4. **Does `server/undo/testing` need a `doc.go` rename to follow the
   `// Package undotesting …` convention?**
   Recommendation: It already has a package comment at the top of
   `roundtrip.go`. Leave as-is. CLAUDE.md's `doc.go` convention is for
   multi-file packages; this is single-file. No change.

5. **Should we leave behind a comment in
   `.github/coverage-allowlist.txt` about why the file is empty (or
   delete the file entirely)?**
   Recommendation: Keep the file with just the comment header pointing
   to a future tracking issue. The CI step at
   `.github/workflows/test_go.yaml:84` reads the file unconditionally;
   deleting it is a separate, larger change. After this PR the file
   will contain only its three comment lines and that's fine.

## Implementation Plan

### Phase 1: `server/health` and `server/firebase` tests

**Goal:** Land the two simplest packages first as a smoke test of the
overall approach.

**Tasks:**

- [ ] `server/health/status_test.go`
  - `TestStatus_IsHealthy_Empty` — empty `Error` returns true.
  - `TestStatus_IsHealthy_NonEmpty` — non-empty `Error` returns false.
  - `TestStatus_AllFieldsRoundTrip` — sanity-check that all five fields
    (Name, Backend, LatencyMs, Metadata, Error) hold their values
    through a struct-literal round trip. Cheap protection against
    accidental field renames in the future.
- [ ] `server/firebase/app_test.go`
  - `TestNewApp_ReadsCredentialsFile` — write a minimal valid Firebase
    service-account JSON to `t.TempDir()`, call `NewApp(ctx, path)`,
    assert non-nil app and no error. The JSON must parse as a
    service-account credential blob (Firebase's
    `option.WithAuthCredentialsJSON` validates the JSON shape;
    real credentials aren't required because no RPC is made). Use a
    minimally-valid service-account JSON template
    (`{"type":"service_account",
    "project_id":"test-project", ...}`) — exact required fields are
    documented at https://firebase.google.com/docs/admin/setup; if
    Firebase's loader is too strict for a synthetic blob, fall back
    to writing fake bytes and asserting on the *successful path*
    only via the no-credentials branch (test below).
  - `TestNewApp_NoCredentials_UsesADC` — call `NewApp(ctx, "")`,
    assert non-nil app and no error. (This works in CI without ADC
    because `firebase.NewApp` does not fetch credentials at
    construction time; first-RPC is what fails on missing ADC.)
  - `TestNewApp_CredentialsFileMissing` — call `NewApp(ctx,
    "/nonexistent/path.json")`, assert error wraps the path and
    mentions "failed to read credentials file".
- [ ] Remove `type2fun.org/ripls/server/firebase` and
  `type2fun.org/ripls/server/health` from
  `.github/coverage-allowlist.txt`.
- [ ] Run `npm run build` and `go test ./server/health/...
  ./server/firebase/...`.

**Validates:** That the test infrastructure works for the trivial
packages and that the allowlist gate flips clean (CI emits no warning
about stale entries for these two).

### Phase 2: `server/undo/testing` self-tests

**Goal:** Cover the harness in its own package. Pure logic; no storage.

**Tasks:**

- [ ] `server/undo/testing/roundtrip_test.go`
  - `TestRoundTrip_Success` — supply `SnapshotBefore` / `Mutate` /
    `SnapshotAfter` / `Undo` callbacks where Undo restores the
    pre-action state. Use a small in-memory map of `proto.Message`
    instances (e.g., `models.Experience` with mutable fields) so
    snapshots vary between calls. Assert no `t.Fail` is recorded
    via a sub-test wrapper.
  - `TestRoundTrip_VacuousMutate_Fails` — Mutate is a no-op so
    `SnapshotAfter == SnapshotBefore`. Confirm the harness reports
    the "vacuous test" failure (use `testing.T` capture pattern via
    a nested `t.Run` and `t.Failed()` assertion, similar to how
    `testing/testing` self-tests work; or use `t.Cleanup` +
    re-running the assertion).
  - `TestRoundTrip_EmptySnapshot_Fails` — `SnapshotBefore` returns
    an empty `Snapshot{}`. Confirm the harness fatals with the
    "harness requires at least one entry" message.
  - `TestRoundTrip_EmptyCommunityEventID_Fails` — `Mutate` returns
    `""`. Confirm the harness fatals with the "Mutate returned
    empty community_event_id" message.
  - `TestRoundTrip_DriftAfterUndo_Fails` — Undo doesn't restore the
    original state. Confirm the harness reports the per-entry diff
    with the entry label and the "drifted after undo" message.
  - `TestRoundTrip_LengthMismatchAfterUndo_Fails` — Undo restores
    fewer entries. Confirm the "snapshot length: want N, got M"
    message fires.
  - Use a generated proto type (`models.User` or a simple message)
    for snapshot entries — exercises `proto.Equal` and
    `prototext.Format` on real generated code.
  - Pattern for capturing harness failures inside a test: invoke
    `RoundTrip` with a `*testing.T` allocated via a wrapper that
    captures `Errorf`/`Fatalf` calls without aborting the parent
    test. The cleanest approach is a custom `mockT` struct that
    embeds `*testing.T` and overrides `Errorf` / `Fatalf` to record
    rather than abort. (See similar pattern in any of Go's
    `testing/testing_test.go` self-tests if needed.)
- [ ] Remove `type2fun.org/ripls/server/undo/testing` from
  `.github/coverage-allowlist.txt`.
- [ ] Run `npm run build` and `go test
  ./server/undo/testing/...`.

**Validates:** The harness itself is exercised, and the cross-package
consumers in `server/services/{experience,request}` continue to pass
(no signature changes).

### Phase 3: `server/planning` tests

**Goal:** Cover the largest of the four packages, with a mix of pure
unit tests and storage-backed integration tests.

**Tasks:**

- [ ] `server/planning/scope_test.go` (pure unit tests, no storage)
  - `TestScope_ScopeID` — table-driven: ExperienceID set, RequestID
    set, both empty.
  - `TestScope_IsExperience` / `TestScope_IsRequest` — predicate
    correctness across the same matrix.
  - `TestScope_scopeFieldName` — `experience_id` for experience scope,
    `request_id` for request scope. (`scopeFieldName` is unexported,
    so the test must live in the `planning` package, not
    `planning_test`.)
- [ ] `server/planning/mock_test.go` (pure unit, no storage)
  - `TestMockScopeLoader_RecordsCallsAndReturnsConfiguredResult` —
    populate `Result` and assert it's returned; populate `Err`
    and assert it's returned; assert `Calls` accumulates every
    invocation in order.
- [ ] `server/planning/scope_loader_test.go` (uses
  `storage.SetupTestStorage`; **no** `//go:build integration` tag —
  per CLAUDE.md, that tag is reserved for live external-API calls,
  not testcontainer-backed storage tests, which run by default with
  `go test ./...`)
  - `TestExperienceScopeLoader_LoadActiveScope_ActiveExperience` —
    happy path returns ConversationID + OwnerID.
  - `TestExperienceScopeLoader_LoadActiveScope_TerminalState` —
    completed/cancelled experience returns
    `CodeFailedPrecondition`.
  - `TestExperienceScopeLoader_LoadActiveScope_NotFound` —
    nonexistent ID returns `CodeNotFound`.
  - Same three tests for `RequestScopeLoader`. (Six tests total.)
- [ ] `server/planning/list_test.go` (uses `storage.SetupTestStorage`)
  - `TestListNeedsAndContributions_Empty` — no needs/contribs returns
    empty result.
  - `TestListNeedsAndContributions_FiltersFullyClaimedNeeds` —
    needs with `slots_remaining == 0` are excluded.
  - `TestListNeedsAndContributions_EnrichesUsers` — both proposer
    and contributor are populated correctly.
  - `TestListNeedsAndContributions_DropsOrphanedReferences` — a need
    whose proposer doesn't exist is silently dropped (current
    behavior at `list.go:81`).
  - Use direct `storage.Insert` to seed `models.PlanningNeed` /
    `models.PlanningContribution` / `models.User` rows.
- [ ] `server/planning/slots_test.go` (uses `storage.SetupTestStorage`)
  - `TestClaimSlot_DecrementsAndSyncsBlob` — decrements both column
    and blob.
  - `TestClaimSlot_NoSlots_ReturnsResourceExhausted` — slots_remaining
    is already 0; assert `CodeResourceExhausted`.
  - `TestReleaseSlot_IncrementsAndSyncsBlob` — happy path.
  - `TestReleaseSlot_MissingNeed_NoOp` — call against a nonexistent
    needID; assert no panic and no error surfaces (best-effort
    semantics).
  - `TestRestoreSlotOnFailure_Increments` — increments column.
  - `TestRestoreSlotOnFailure_MissingNeed_NoOp` — same best-effort
    guarantee.
- [ ] `server/planning/chat_test.go` (uses
  `storage.SetupTestStorage`; calls real
  `chat.SystemMessageWriter`)
  - `TestPostNeedAdded_WithNote_EmitsUserMessage` — note set →
    `InsertUserMessageOnBehalfOf` is called with the right
    `NeedID`/`NeedName`.
  - `TestPostNeedAdded_NoNote_EmitsSystemMessage` — note empty →
    `InsertSystemMessage` is called with
    `CHAT_SYSTEM_ACTION_PLANNING_NEED_ADDED` and the coalesce key
    matches the need ID.
  - `TestPostNeedAdded_NilWriter_NoOp` — writer == nil returns nil
    error and writes nothing.
  - `TestPostNeedAdded_EmptyConversationID_NoOp` — same.
  - Equivalent pairs for `PostNeedRemoved`, `PostNeedClaimed`,
    `PostContributionAdded`, `PostContributionRemoved`.
  - Pattern: instantiate a real `chat.SystemMessageWriter` against
    test storage, call the helper, then query the storage for the
    inserted `ChatMessage` rows and assert on their content. Mirrors
    `server/services/experience/needs_system_messages_test.go`. The
    helpers' soft-failure log paths (lines 46, 69, 106, 143, 166)
    don't need direct coverage — they fire only when chat insertion
    fails, which we can simulate by closing storage between calls
    if needed but isn't worth the test complexity.
- [ ] Remove `type2fun.org/ripls/server/planning` from
  `.github/coverage-allowlist.txt`.
- [ ] Run `npm run build` and `go test ./server/planning/...`. (No
  `-tags=integration` needed — none of these tests call live
  external APIs.)

**Validates:** The largest package is fully covered; the existing
service-level tests in `server/services/{experience,request}` continue
to pass unchanged.

### Phase 4: Final allowlist cleanup and CI verification

**Goal:** Confirm the file is empty of package entries and CI is clean.

**Tasks:**

- [ ] After phases 1–3, `.github/coverage-allowlist.txt` should
  contain only the three comment header lines.
- [ ] Run the full Go suite: `go test -tags=integration -timeout 20m
  ./server/...`.
- [ ] Run `npm run build` (golangci-lint + `go build ./...`) and
  `go build -tags=integration ./...` to catch integration-build
  signature drift.
- [ ] Verify that the CI step "Require tests on every Go package"
  emits neither error (new untested package) nor warning (stale
  allowlist entry). Easiest way to check locally:
  ```bash
  allowlist=$(grep -v '^#' .github/coverage-allowlist.txt | grep . | sort -u)
  untested=$(go list -f '{{if and (ne .Name "main") (eq (len .TestGoFiles) 0) (eq (len .XTestGoFiles) 0)}}{{.ImportPath}}{{end}}' ./server/... \
    | grep -v '/gen/' | sort -u)
  comm -23 <(echo "$untested") <(echo "$allowlist")  # must be empty (no new untested)
  comm -13 <(echo "$untested") <(echo "$allowlist")  # must be empty (no stale entries)
  ```

**Validates:** Issue acceptance criteria met end-to-end. Allowlist
header still references #1438 — that's fine; the issue closes when
this PR merges.

## Acceptance Criteria Checklist

- [ ] `server/firebase` has `*_test.go` covering the FCM provider's
  exported surface.
- [ ] `server/health` has `*_test.go` covering the health-check handler.
- [ ] `server/planning` has `*_test.go` covering scope, list, slots,
  chat helpers, and the mock loader.
- [ ] `server/undo/testing` has `*_test.go` exercising `RoundTrip` (no
  rename/restructure needed; the package has real reusable logic).
- [ ] `.github/coverage-allowlist.txt` no longer contains any of the
  four package import paths.
- [ ] `npm run build` passes (golangci-lint + `go build ./...`).
- [ ] `go test -timeout 20m ./server/...` passes (unit + storage
  tests; no `integration` tag needed for any of the new tests).
- [ ] `go test -tags=integration -timeout 20m ./server/...` still
  passes (re-runs the existing live-API tests untouched, plus the
  new unit/storage tests).

## Related Issues

- #1437 — introduced the coverage allowlist file. This issue is the
  cleanup tracker that #1437's file header points to.
- #1433 — broader test-coverage epic this issue rolls up under.
- #1288, #1289, #1361 — sibling per-package "add tests" issues that
  use the same overall pattern (real `storage.SetupTestStorage` for
  storage-coupled code, pure unit tests for everything else).
