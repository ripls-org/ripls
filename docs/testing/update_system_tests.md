---
# Doc-context metadata for docs/llms.txt — update when this doc changes.
# How it works + schema: docs/context_map.md
context:
  description: How to maintain end-to-end system integration tests that map 1:1 to the workflow-doc examples (preconditions/steps/postconditions), plus the selective Playwright e2e companion specs and their conventions.
  globs: [server/integration_tests/**, e2e/tests/workflows/**]
  triggers: [integration-test, system-test, e2e, playwright, workflow-examples, postconditions, journey]
  lens: [testing]
freshness:
  verified_commit: "43bda6508"
  verified_on: "2026-07-01"
---
# Updating System Integration Tests

This document provides instructions for maintaining system integration tests that exercise complete user journeys end-to-end.

## Test Locations

- `server/integration_tests/` — **primary** server-side tests; map 1:1 to
  workflow examples, drive everything via RPC, run fast, and own the
  state-machine and notification-dispatch coverage.
- `e2e/tests/workflows/` — **selective** UI companion specs that exercise
  the Flutter Web bundle in a real browser via Playwright (#2162). These
  specs are deliberately not a 1:1 port of the server tests; they exist
  only where the UI catches something the RPC tests cannot. See
  [E2E companion specs](#e2e-companion-specs) below.

## Source of Truth

**Workflow examples in `docs/workflows/*.md` are the source of truth for integration tests.**

Each workflow document contains a "Workflow Examples" section with numbered examples. Each example includes:

- **Preconditions**: Setup requirements before the journey begins
- **Steps**: The sequence of user and system actions
- **Postconditions**: Verifiable outcomes after the journey completes

Server integration tests map 1:1 to these workflow examples. E2E
companion specs map selectively — see below.

### Workflow Documents

| Category    | Workflow Document                           | Test File                                     |
| ----------- | ------------------------------------------- | --------------------------------------------- |
| Loans       | [loan.md](../workflows/loan.md)             | `server/integration_tests/loan_test.go`       |
| Giveaways   | [giveaway.md](../workflows/giveaway.md)     | `server/integration_tests/giveaway_test.go`   |
| Requests    | [request.md](../workflows/request.md)       | `server/integration_tests/request_test.go`    |
| Experiences | [experience.md](../workflows/experience.md) | `server/integration_tests/experience_test.go` |

## Implementation Guidelines

### File Organization

Server integration tests live in `server/integration_tests/` with the following structure:

```
server/integration_tests/
├── test_helpers.go      # Shared test infrastructure
├── loan_test.go         # Loan workflow tests
├── giveaway_test.go     # Giveaway workflow tests
├── request_test.go      # Request workflow tests
└── experience_test.go   # Experience workflow tests
```

**test_helpers.go** should contain:

- Server startup/shutdown functions (`startTestServer`, `waitForServerReady`, etc.)
- User registration helpers (`registerFirstUser`, `registerUserByInvite`)
- Authenticated client factories (`createAuthGearClient`, `createAuthChatClient`, etc.)
- Common test utilities and the `authTransport` type

### Test Naming Convention

Test function names should include the workflow example number for easy cross-reference:

```go
func TestLoan_Example1_SharingAndBorrowingGear(t *testing.T)
func TestLoan_Example2_BorrowerWithdrawsInterest(t *testing.T)
func TestGiveaway_Example1_GivingAwayItem(t *testing.T)
```

### Test Structure

Each test should use subtests (`t.Run`) for fine-grained assertions while maintaining one top-level test per workflow example. Structure the test code with comments separating the three phases:

```go
func TestLoan_Example1_SharingAndBorrowingGear(t *testing.T) {
    dbURL, cleanup := storage.SetupTestDatabase(t)
    defer cleanup()

    serverCmd, serverURL := startTestServer(t, dbURL)
    defer func() { _ = serverCmd.Process.Kill() }()

    ctx := context.Background()

    // ========== PRECONDITIONS ==========
    // - A test community exists
    // - Three users exist: an owner, borrower A, and borrower B

    ownerToken, ownerID := registerFirstUser(t, serverURL, "owner@example.com", "Owner")
    // ... setup code ...

    // ========== STEPS ==========

    // Step 1: Owner shares gear with community
    t.Run("Step1_OwnerSharesGear", func(t *testing.T) {
        // ... test code ...
    })

    // Step 2: System creates perpetual conversation
    t.Run("Step2_ConversationCreated", func(t *testing.T) {
        // ... test code ...
    })

    // ... remaining steps ...

    // ========== POSTCONDITIONS ==========

    t.Run("Postcondition_GearStillShared", func(t *testing.T) {
        // ... verification ...
    })

    t.Run("Postcondition_GearAvailable", func(t *testing.T) {
        // ... verification ...
    })

}
```

### Chat Messages in Tests

Workflow steps involving chat messages should be implemented using the Chat service RPCs (`SendMessage`, `GetConversation`, etc.). This tests the full integration of messaging with the loan/giveaway/request workflows.

### Notification Testing

Workflow postconditions include "Notifications sent" assertions that specify which push notifications should be sent and to whom. **The workflow documents (`docs/workflows/*.md`) are the canonical source for notification expectations** - tests should verify these notifications as part of postcondition checking.

**Test Provider:**

The integration-test server runs the no-op notification provider
(`server/notifications/noop/provider.go`, selected by the `"test"`/`"noop"`
notification mode in `server/channels.go`). Instead of actually sending pushes, the
dispatch path emits structured log entries (e.g. `"notification dispatched to
user"`), which the `LogCapture` infrastructure in
`server/integration_tests/test_helpers.go` parses and exposes via helpers like
`GetNotificationLogsForUser`.

**Test Setup:**

```go
// Start server with log capture enabled
serverCmd, serverURL, logCapture := startTestServerWithLogCapture(t, dbURL)
```

**Verification Pattern:**

```go
t.Run("Postcondition_NotificationsSent", func(t *testing.T) {
    // Wait for the async notification pipeline to reach the expected count
    // AND settle (no new push for notificationSettleWindow), so an
    // over-emission fails the exact-count assert loudly instead of racing
    // it (#2657). The 5s budget is free on the happy path — the wait
    // returns one settle window after the last expected push lands.
    WaitForNotificationCount(logCapture, expectedCount, 5*time.Second)

    // Verify owner received notification when borrower expressed interest
    ownerNotifications := logCapture.GetNotificationLogsForUser(ownerID)
    if len(ownerNotifications) == 0 {
        t.Error("Expected owner to receive notification when borrower expressed interest")
    }
})
```

Exact-count postconditions must also dump the captured notifications on
failure (`t.Logf("  Notification: user=%s title=%s", …)`) so a CI-only
mismatch is diagnosable from the run log alone.

**Finding Expected Notifications:**

Each workflow document has a "Notifications" section under "Side Effects" that lists which events trigger notifications and who receives them. The workflow examples include "Notifications sent" in their postconditions specifying exactly which notifications should be verified.

See [push_notifications.md](../push_notifications.md) for notification architecture details.

### Update Order

When product behavior changes:

1. Update the workflow document (`docs/workflows/*.md`)
2. Update the integration tests to match

### Strict Alignment Requirement

**Test assertions must strictly match the postconditions specified in workflow documents.**

When a test cannot verify a postcondition exactly as documented, you must either:

1. **Fix the implementation** to match the documented behavior, or
2. **Fix the documentation** if the documented behavior was incorrect, or
3. **Add a TODO comment** in the test explaining the divergence

TODO comments for divergences must follow this format:

```go
// TODO: Per docs/workflows/loan.md, gear should be GEAR_STATE_AVAILABLE after completion.
// Currently seeing GEAR_STATE_ACTIVE - investigate why completion isn't resetting gear state.
// For now, using permissive assertion that accepts either state.
if gear.State != api.GEAR_STATE_AVAILABLE && gear.State != api.GEAR_STATE_ACTIVE {
    t.Errorf("Expected gear state AVAILABLE or ACTIVE, got %v", gear.State)
}
```

**Key requirements for divergence TODOs:**

- Reference the specific workflow document and postcondition
- State the expected value from the documentation
- State the actual observed value
- Briefly explain the suspected cause (if known)
- Use a permissive assertion that passes with current behavior

This ensures tests remain green while clearly documenting gaps between specification and implementation for future resolution.

## E2E companion specs

The server integration tests above are the **primary** coverage. They run
fast, cover state transitions deterministically, and verify notifications
via `LogCapture` — they catch the bulk of workflow regressions.

In addition, a small set of **UI companion specs** lives in `e2e/tests/workflows/`
under the Playwright harness introduced by #2162. These specs run the
Flutter Web bundle in a real browser (mobile-Chromium + WebKit) and
exercise the integration boundary between the bundle, the Semantics
tree, and the Connect-Web wire format.

### When to add an e2e companion spec

E2E specs are deliberately **not** a 1:1 port of the server integration
tests. We add one only when the UI catches a failure class that the RPC
test cannot. The criterion:

> **Add an e2e companion only when the UI is the thing under test.**

Concretely, that means:

- **Multi-user real-time UI propagation** — server tests verify the
  state change; the e2e spec verifies that user B's open browser
  actually reflects user A's action. (E.g., guest RSVPs → host sees
  attendee count update.)
- **Browser-only code paths** — video playback, deep-link/route
  landings, accessibility tree serialization, anything the bundle does
  that no server test exercises. (E.g., the #2155 hero-video
  regression.)
- **Cross-context UI invariants** — the same server state must render
  consistently across distinct community/role/permission contexts.
  (E.g., loan-out in community A renders as UNAVAILABLE in community B
  without leaking the borrower's identity.)

We do **not** add an e2e spec when:

- The bug is a server state-machine transition (server test owns it).
- The bug is a notification-dispatch issue (server test owns it via
  `LogCapture`).
- The bug is impact-metric math (deterministic from inputs).
- The bug is a UI render that no realistic user action will trigger.

A spec that simply drives every workflow step through `Tappable` taps
is **not** an e2e companion — it's a Patrol re-run, and that path was
removed (#1887) for the cost-vs-value reasons that still apply.

### Current e2e companion coverage

| E2E spec                                                                  | Workflow doc reference                                              | UI-specific failure class |
| ------------------------------------------------------------------------- | ------------------------------------------------------------------- | ------------------------- |
| `e2e/tests/workflows/experience-rsvp-multi-client.spec.ts`                | `experience.md` Example 1 (host + 1 guest slice)                    | Multi-client UI propagation: guest RSVPs in browser A, host's browser B reflects the new attendee count after refresh. |
| `e2e/tests/workflows/phone-rsvp-full-loop.spec.ts`                        | `phone_first_rsvp.md` Workflow Example (guest RSVPs via phone OTP)   | Phone-first auth IS the unit under test: drives the real phone-register UI (Firebase Auth Emulator OTP), then verifies the provisional placeholder is promoted and the RSVP auto-fires after community join. |
| `e2e/tests/workflows/event-whos-in-counts.spec.ts`                        | `experience.md` (host + community RSVP slice, #2492)                 | "Who's In" roster rendering: host creates an event and invites a community through the real UI, then the rendered roster must show the right per-section Going/Maybe/Not-going counts plus a per-community "N haven't responded" count (responders subtracted). |
| `e2e/tests/workflows/share-sheet-community-picker.spec.ts`                | `experience.md` (share-sheet community picker, #2492)               | Share-sheet picker visibility: a host-only nameless per-item community must be suppressed from the community picker while named communities still render — exercised end-to-end through the real share-sheet UI. |
| `e2e/tests/workflows/event-invite-home-multi-client.spec.ts`              | `experience.md` (directly-invited individuals slice, #2492)         | Multi-client home propagation for directly invited individuals: a host invites people (not a community) to an event; each invitee's separate browser must show the event on their Home Up-next even before they reply, and host-set / self-set RSVP status syncs both ways across the distinct clients. |
| `e2e/tests/workflows/event-rsvp-streaming-multi-client.spec.ts`            | `experience.md` (live RSVP streaming slice, #2492 / #2531)          | Real-time multi-client RSVP propagation with no reload via `StreamUserEvents`: an invitee RSVPs through the composer and the host's already-open Who's-In roster updates live; the host sets an invitee's RSVP and that invitee's already-open event page updates live (their RSVP call-to-action disappears). |
| `e2e/tests/workflows/phone-gear-interest-full-loop.spec.ts`               | `phone_first_gear_interest.md` Workflow Example (guest borrows/claims a shared gear via phone OTP, #2492 WEB-4) | Phone-first auth IS the unit under test: drives the real phone-register UI (Firebase Auth Emulator OTP) from a shared-gear link, then verifies the provisional placeholder is promoted and ExpressInterest auto-fires after the gear's ad-hoc community join. |
| `e2e/tests/workflows/phone-request-offer-full-loop.spec.ts`               | `phone_first_request_offer.md` Workflow Example (guest offers to help on a shared request via phone OTP, #2492 WEB-3) | Phone-first auth IS the unit under test: drives the real phone-register UI (Firebase Auth Emulator OTP) from a shared-request link, then verifies the provisional placeholder is promoted and OfferToFulfill auto-fires after the request's ad-hoc community join. |
| `e2e/tests/workflows/phone-gear-giveaway-full-loop.spec.ts`               | `phone_first_gear_interest.md` Workflow Example (guest claims a shared GIVEAWAY via phone OTP, #2492 WEB-4) | Phone-first auth IS the unit under test, giveaway twin of the interest loop: drives the real phone-register UI from a shared-giveaway link, then verifies the SSR landing CTA ("I want this"), the promoted provisional identity, and that ExpressInterest leaves the item at INTEREST_EXPRESSED (no auto-recipient) so the guest surfaces in the transfer's pending requests. |
| `e2e/tests/workflows/phone-existing-member-login.spec.ts`                 | `phone_first_gear_interest.md` Workflow Example (returning member re-enters their phone, #2492)            | Register→login fallback IS the unit under test: a phone that already has an account hits `AlreadyExists` on PhoneRegister, and `PhoneAuthScreen._handlePhoneRegister` must log them in via PhoneLogin and fire the same auto-action rather than rejecting them as a duplicate. |
| `e2e/tests/workflows/phone-guest-email-register.spec.ts`                  | `phone_first_rsvp.md` Workflow Example (guest RSVPs, but chooses EMAIL over phone OTP, #2595)              | Inline email register on the phone-first screen IS the unit under test: drives the shared `EmailRegisterForm` expanded in place on `/verify-phone` (no route-hop to `/register`, no Firebase OTP — `EmailRegister` is a single RPC), then verifies the guest lands on the item view and the RSVP auto-fires after community join (host sees `rsvpYesCount` increment). |
| `e2e/tests/workflows/add-phone-to-account.spec.ts`                        | [registration_and_login.md](../registration_and_login.md) → *Adding a Phone Number to an Existing Account* (#2596) | Attach-mode phone-OTP UI IS the unit under test: an email/password user adds a verified phone from the profile-edit screen through the real UI (Firebase Auth Emulator OTP, attach mode of `PhoneAuthScreen` → `UserService.AddPhoneNumber`), then a self `GetUser` confirms the number landed on the **same** existing account (additive, not a new sign-up). |
| `e2e/tests/workflows/gear-authoring-share.spec.ts`                        | `item_sharing.md` (gear authoring + "Shared with", #2492)                                                  | Gear authoring/share UI IS the unit under test: the host creates a gear through the real unified-create UI, invites people from the auto-opened share sheet, lands directly in the gear (no reload), and inspects the `SharedWithCard` → `AccessSheet`, then adds a community via the card's Invite button (counts anchored to `GetGear`). |
| `e2e/tests/workflows/request-authoring-share.spec.ts`                     | `item_sharing.md` (request authoring + "Shared with", #2492)                                               | Request twin of gear-authoring-share: the host submits a request through the real unified-create UI, invites people from the auto-opened share sheet, lands directly in the request (no reload), and inspects the `SharedWithCard` → `AccessSheet` (which lists invited people by name), then adds a community from the access sheet (counts anchored to `GetRequest`). |
| `e2e/tests/workflows/loan-return-mark-returned.spec.ts`                   | `loan.md` Example 1, step 11 (borrower returns; return slice only, #2638)                                  | Mark-returned reachability through the real browser a11y tree: the Home NEEDS-YOU pill's semantics node used to be merged into its row, so on Flutter Web the row's flt-semantics element swallowed every pill click (the widget test's gesture arena still passed — a browser-only failure class); also proves the holder's gear who-card "Mark returned" CTA completes the loan. |
| `e2e/tests/workflows/plans-calendar-suggestion-create.spec.ts`            | [calendar.md](../client/calendar.md) → *Weather and open-day suggestions* + the suggestion→create flow      | Plans-tab suggestion→create date seeding: the open-day suggestion's "Plan it with them" CTA must carry that calendar day into unified-create as a structural date seed (`_seedEventTime`, marked user-edited so the AI stream can't overwrite it), so the created event lands on exactly the day the user started from — not "today", not whatever the AI prompt text parses to. Also proves a seeded event renders on its own day in the dock's calendar destination. |
| `e2e/tests/workflows/library-shelf-gear.spec.ts`                          | [calendar.md](../client/calendar.md) sibling — the Library dock destination (DiscoverScreen, #2634 v2)      | Library-tab shelf rendering: gear shared into MORE THAN ONE of the viewer's communities aggregates onto the single Library tab as `LibraryShelfTile`s (SearchService empty-query → category shelves), and the borrowable-vs-giveaway distinction is painted only in the tile's semantics (`"<name> · Giveaway"` vs bare name) — invisible to any RPC test. Second test: turning off the "Giving" filter category re-renders the shelves (the giveaway tile disappears, the loan tile stays), proving the filter-sheet → searchProvider → shelf wiring end-to-end. |
| `e2e/tests/workflows/people-directory.spec.ts`                            | The People dock destination (DirectoryScreen, #2634)                                                        | People-tab list merge: the tab combines two independent RPC reads into one rendered list — the viewer's communities (`ListCommunities`) AND connected people (`GetPortfolioInboxView.people`). Asserts both of the viewer's communities render as rows (aggregation) and a co-member who owns shared activity renders as a PERSON row (the directoryPeopleProvider path no community-only spec touches). Complements community-calendar-shared-event (community-row→calendar routing) and community-promote-nudge (the "Name" chip). |
| `e2e/tests/workflows/universal-search.spec.ts`                            | The header universal-search overlay (UniversalSearchScreen, `a11yNavDockSearch`)                            | Universal-search wiring end-to-end in the browser: the header "Search your communities" icon opens the overlay, real keystrokes into the autofocused Flutter-Web text field drive `SearchService.UniversalSearch` (300ms debounce), and a gear shared into the viewer's community surfaces as a result row by name. Also proves membership scoping: after the query changes, the in-scope result disappears (search re-ran) and a stranger's gear in a non-shared community never appears. |

The Phase-1 anchor specs in `e2e/tests/` (not under `workflows/`) cover
browser-only code paths that don't map to a workflow example:

| E2E spec                                  | Failure class                                                                       |
| ----------------------------------------- | ----------------------------------------------------------------------------------- |
| `e2e/tests/video-on-event-hero.spec.ts`   | #2155 regression — Web-only `createCachedVideoController` path through CanvasKit.   |
| `e2e/tests/guest-rsvp-confirm.spec.ts`    | Share-link handoff: `?rsvp=yes&code=…` chains `AcceptInvitationLink → RSVPToExperience`. |
| `e2e/tests/ported-smoke.spec.ts`          | Bundle bootstrap (3 checks ported from the deleted `server/cmd/web-smoke/`).        |

### Spec file conventions

Each e2e workflow spec must:

1. **Drive workflow steps through the UI, not through RPCs.** RPCs
   are restricted to preconditions (seed) and postconditions
   (assert server state). The test body uses real UI gestures —
   taps, form fills, URL navigations — on the production Flutter
   Web bundle. A spec that fires `someClient.someRpc(…)` mid-test
   to flip state is not adding coverage over `server/integration_tests/`
   and isn't worth the browser-overhead cost. See
   [`e2e/README.md` § "State changes during the test body happen
   through the UI"](../../e2e/README.md#state-changes-during-the-test-body-happen-through-the-ui).

2. **Name the workflow doc source in the file header.** Use the form:
   `Maps to: docs/workflows/{workflow}.md §"Workflow Examples" → Example N`.
   If the spec covers a slice of the example (e.g., 1 guest instead of 3),
   say so and explain why the smaller scope is sufficient.

3. **Name the UI-specific failure class it catches.** One short
   paragraph in the header. If you can't write this paragraph, the
   spec is probably a candidate for deletion, not for adding to the
   suite.

4. **Document deliberate scope-outs.** If the spec uses an RPC for
   a step that could be UI-driven once identifiers land (e.g., the
   picker modal is identifier-less today), say so in the header AND
   file a follow-up — don't quietly fake the gesture indefinitely.

5. **Add the new spec to the "Current e2e companion coverage" table
   above** in the same PR.

6. **Add only the semantics identifiers the spec needs.** Per the
   Phase-0 convention doc (`docs/client/testing/semantics_identifiers.md`),
   identifiers are added "only when a test asks for it" — not
   speculatively. Identifiers are kebab-case (`require_kebab_case_for_semantics_identifier`).

## Instructions for Coding Agents

If you are a coding agent asked to update system tests, do the following:

1. **Read the workflow documents**: Read all workflow examples in `docs/workflows/*.md`, including preconditions, steps, and postconditions
2. **Read existing tests**: Examine the current integration tests at `server/integration_tests/*`
3. **For each workflow category** (loans, giveaways, requests, experiences):
   1. Ensure that a corresponding integration test file exists
   2. Ensure that there is a one-to-one correspondence between top-level test cases and workflow examples
   3. Add a test for any workflow examples that are missing
   4. Update any existing tests so that the preconditions, steps, and postconditions match the workflow doc
   5. **Verify notifications**: Each test should verify the "Notifications sent" postconditions from the workflow doc
   6. Remove any tests that don't correspond to a workflow example
4. **Do NOT add an e2e companion spec by default.** E2E coverage is
   selective (see [E2E companion specs](#e2e-companion-specs)). If you
   believe the workflow change introduces a UI-specific failure class
   not covered by existing e2e specs, raise it for human review rather
   than adding a new spec autonomously.
