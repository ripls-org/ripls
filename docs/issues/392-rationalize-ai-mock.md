# Issue #392: Rationalize provider_mock.go

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/392
**Filed:** 2025-12-01 | **Last activity:** 2026-03-26
**Labels:** none | **Assignees:** unassigned
**Context docs:** none

## Problem Statement

The codebase has proliferated AI provider mocks. There is one "official" mock
(`server/ai/provider_mock.go`) plus at least four private mocks scattered
across service test files. Every time the `ai.Provider` interface changes —
adding a method, changing a signature — all of these mocks break independently.
The official mock is also not customizable enough for many test scenarios,
which is why service authors keep creating their own.

The goal is to converge to a single, flexible mock that lives in the `ai`
package and is powerful enough that no test file needs to roll its own
implementation.

## Context from Comments

- **Triage (2026-03-05, tescobar):** Rated P3 code cleanup. Acknowledged the
  breakage pattern but not urgent.
- **Bryan (2026-03-26):** Clarified the real objective: "take a hard look at
  the various mocks... create a plan to converge to just one, in a way that
  minimizes development burden and code change blast radius outside the ai
  package when the API changes." This reframes the issue from cleanup to
  architectural improvement — the key metric is reducing blast radius when the
  interface evolves.

## Current State

### The Provider Interface (16 methods)

`server/ai/provider.go` defines `Provider` with 16 methods across 5
categories: gear (3), community (1), request (2), experience (3),
conversation (2), utility (5 — time parsing, story, social attributes, health).

### Existing Mock Implementations

| # | Location | Type | Approach | Used By |
|---|----------|------|----------|---------|
| 1 | `server/ai/provider_mock.go` | `MockProvider` | Function-field overrides with defaults | ~10 test files across gear, community, request, experience, transfer, jobs |
| 2 | `server/services/chat/summary_test.go` | `MockAIProvider` | Captures `SummaryStyle`, returns configurable summary | Chat summary tests only |
| 3 | `server/product/lookup_test.go` | `mockAIProvider` | Returns configurable gear generation + error | Product lookup tests only |
| 4 | `server/services/experience/common_test.go` | `mockAIProvider` | Returns preconfigured experience responses | Experience service tests |
| 5 | `server/ai/provider_loadbalanced_test.go` | `mockProvider` | Tracks call counts, supports error injection | Load balancer + fallback routing tests |

### Pain Points

1. **Adding a method touches 6+ files.** The main mock, 4 custom mocks, and
   both decorator tests all need updating.
2. **Custom mocks are inconsistent.** Some return `nil` for unused methods,
   some store configurable results, some inject errors — no shared pattern.
3. **No argument capture.** The official `MockProvider` lets you override
   behavior but doesn't capture call arguments. The chat mock does capture
   `SummaryStyle`, showing this is a real need.
4. **No call counting.** The load-balancer mock tracks calls, but the official
   mock doesn't. Tests that need to verify a method was (or wasn't) called
   must build their own mock.
5. **Verbose setup.** Custom mocks must implement all 16 methods even when
   testing only 1-2.

## Potential Solutions

### Option A: Enhanced Function-Field Mock (Evolve Current Pattern)

- **Approach:** Keep the current `MockProvider` pattern (struct with one
  function field per method) but add call tracking, argument capture, and
  richer defaults. Migrate all custom mocks to use this enhanced version.
- **Pros:** Familiar pattern already used in ~10 test files. No new
  dependencies. Incremental migration — can convert one test file at a time.
  Stays close to the existing code.
- **Cons:** Still requires adding a new function field every time a method is
  added to the interface. The struct definition grows linearly with the
  interface. Call tracking and argument capture add boilerplate to the mock
  itself.
- **Effort:** Medium.

### Option B: Code-Generated Mock (mockgen / moq)

- **Approach:** Use a code generation tool like `moq` to auto-generate the
  mock from the `Provider` interface. The generated mock handles call tracking,
  argument capture, and return value configuration automatically.
- **Pros:** Zero maintenance when the interface changes — just re-run the
  generator. Argument capture and call counting come free.
- **Cons:** Adds a dev dependency. Generated code can be verbose. Default
  return values would need to be layered on top via a helper.
- **Effort:** Medium.

### Option C: Test Double with Builder Pattern (Hand-Written)

- **Approach:** Create a `TestProvider` with a builder API:
  `ai.NewTestProvider().WithGearDetection(result).WithError(...)`.
- **Pros:** Expressive test setup. Self-documenting.
- **Cons:** Unfamiliar pattern. Builder API is another abstraction. Still
  requires manual updates when the interface changes.
- **Effort:** Medium-large.

## Recommendation

**Option A (Enhanced Function-Field Mock)** — evolve the current pattern to
add call tracking and argument capture, then consolidate all private mocks
into it.

This is the right choice because:

1. **No new dependencies.** The project values minimal dependencies, and the
   current function-field pattern already works well for ~10 test files.
2. **Minimal migration friction.** Existing tests that set
   `mock.GenerateGearFromTextFunc = ...` continue to work identically. The
   enhancement is additive.
3. **Solves the real problem.** The fragmentation exists because the mock
   lacked call tracking and argument capture. Adding these features to the
   single mock eliminates the reason private mocks were created.
4. **Reduces blast radius.** After consolidation, adding a new method to
   `Provider` requires updating only `provider_mock.go` — not 6+ files.

The tradeoff is that adding a new interface method still requires a manual
update to the mock (unlike code generation). But with all mocks consolidated
into one file, this is a single edit in a predictable location.

## Open Questions

- **What call-tracking pattern to use?** — Recommendation: Add a `Calls`
  struct field to `MockProvider` with a sub-field per method (e.g.,
  `Calls.DetectGearInImage` as a slice of argument structs). This is the
  pattern used by `moq`-generated code and is straightforward to use in
  assertions. Each method appends to its slice before delegating to the
  function field (or default). This also provides call counting for free
  (`len(mock.Calls.DetectGearInImage)`).

- **Should argument capture be opt-in or always-on?** — Recommendation:
  Always-on. The overhead of appending to a slice is negligible in tests, and
  having it always available eliminates the need for special "capturing" mock
  variants. Any test can assert on arguments without extra setup.

- **Should the load-balancer/fallback tests use the same mock?** —
  Recommendation: Yes. The enhanced mock's call tracking replaces the custom
  `mockProvider`'s call-counting fields. The `shouldError` / `errorMsg` fields
  map directly to setting the function field to return an error.

- **Should this be done all at once or incrementally?** — Recommendation:
  Incrementally in two phases. Phase 1 enhances the mock and verifies existing
  tests still pass. Phase 2 migrates each private mock one test file at a
  time. This keeps PRs reviewable and limits blast radius.

## Implementation Plan

### Phase 1: Enhance MockProvider with Call Tracking

**Goal:** Add call tracking and argument capture to the existing
`MockProvider` without breaking any existing tests.

**Tasks:**
- [ ] Define a `MockProviderCalls` struct with a slice field per method,
  where each slice element is a struct capturing that method's arguments.
  If the combined call-tracking types and mock implementation would push
  `provider_mock.go` beyond ~1000 lines, split into two files:
  `provider_mock.go` (struct + methods) and `provider_mock_calls.go`
  (argument structs and `MockProviderCalls` type). Per the server architecture
  guide, files should stay under 1000 lines.
- [ ] Add a `Calls MockProviderCalls` field to `MockProvider`
- [ ] Update each method implementation to append its arguments to the
  corresponding `Calls` slice before delegating to the function field (or
  default)
- [ ] Add a `Reset()` method to zero out all call slices (useful for
  multi-phase tests)
- [ ] Add unit tests for the new call-tracking behavior in
  `provider_mock_test.go`: verify that calling a method increments the
  corresponding call slice, verify argument structs capture all parameters
  correctly, and verify `Reset()` clears all slices. Per the server
  architecture guide: "No new functionality should be added without test
  coverage."
- [ ] Verify all existing tests pass: `go test ./...`
- [ ] Run `npm run build` to verify lint + compile

**Validates:** The enhancement is backward-compatible. No existing test
behavior changes. Call tracking works end-to-end and is itself tested.

### Phase 2: Migrate Private Mocks

**Goal:** Eliminate all custom mock implementations in service test files so
there is exactly one mock of `ai.Provider` in the codebase.

**Tasks:**
- [ ] Migrate `server/services/chat/summary_test.go`: Replace
  `MockAIProvider` with `ai.NewMockProvider()` + function override. Use
  `mock.Calls.GenerateConversationSummary` to capture and assert on
  `SummaryStyle` (replaces the custom `ReceivedStyle` field)
- [ ] Migrate `server/product/lookup_test.go`: Replace `mockAIProvider` with
  `ai.NewMockProvider()` + `GenerateGearFromWebpageFunc` override for error
  injection
- [ ] Migrate `server/services/experience/common_test.go`: Replace
  `mockAIProvider` with `ai.NewMockProvider()` + preconfigured experience
  response overrides
- [ ] Migrate `server/ai/provider_loadbalanced_test.go` and
  `provider_fallback_test.go`: Replace custom `mockProvider` with
  `ai.NewMockProvider()` using `len(mock.Calls.MethodName)` for call counting
  and function fields for error injection. Note: load-balancer tests need
  multiple independent mock instances with different `Name()` returns — verify
  this works with the enhanced mock
- [ ] Delete all now-unused custom mock structs
- [ ] Verify all tests pass: `go test ./...`
- [ ] Run `npm run build` to verify lint + compile

**Validates:** A single mock serves all test scenarios. Adding a new method to
`Provider` now requires updating only `provider_mock.go`, not 6+ files.

### Phase 3: Documentation

**Goal:** Ensure the pattern is self-sustaining and future contributors don't
re-introduce private mocks.

**Tasks:**
- [ ] Add a doc comment block at the top of `provider_mock.go` explaining:
  how to use function overrides, how call tracking works, how to add support
  for new interface methods
- [ ] Consider adding a brief mention in `docs/server/testing.md` pointing to
  the mock and its usage patterns

**Validates:** New contributors can use the mock without reading the history.

## Related Issues

- None identified in the issue or comments.

---

## Audit Notes

**Audited:** 2026-03-26 against `docs/server/architecture.md`,
`docs/client/architecture.md`, `docs/client/caching.md`, `docs/client/i18n.md`.

**Compliance:** This plan is server-only and does not touch client code,
caching, or i18n. No client-side compliance issues.

**Issues found and fixed:**

1. **Missing unit tests for new functionality (Phase 1).** The server
   architecture guide requires test coverage for all new functionality
   ("No new functionality should be added without test coverage" — Testing
   Strategy, section 4). Phase 1 adds call tracking, argument capture, and
   `Reset()` but originally only verified existing tests still pass. **Fixed:**
   Added explicit task to write unit tests for call-tracking behavior in
   `provider_mock_test.go`.

2. **File size risk (Phase 1).** The server architecture guide requires files
   under 1000 lines (File Organization section). With 16 methods each needing
   a function field, an argument struct, and call-tracking logic, the mock file
   could exceed this limit. **Fixed:** Added guidance to split into
   `provider_mock.go` + `provider_mock_calls.go` if the file exceeds ~1000
   lines.

**Confirmed compliant:**

- **Mock strategy alignment.** The architecture guide explicitly states: "Use a
  single, shared mock implementation where possible. Define mocks in the
  library package. Reuse mocks across services rather than creating
  service-specific mocks." This plan directly implements that guidance.
- **Interface-based dependency pattern.** The mock stays in the `ai` package
  and implements the `Provider` interface, matching Pattern 2 (Interface-Based
  Dependencies) in the architecture guide.
- **Build verification.** Both phases include `npm run build` verification,
  matching project conventions.
