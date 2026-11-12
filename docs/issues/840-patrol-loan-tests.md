# Issue #840: Create Patrol tests for loan workflows

**Source:** https://github.com/Type2Fun-Foundation/ripls/issues/840
**Filed:** 2026-03-14 | **Labels:** P1 | **Assignees:** maintainer
**Branch:** `lend_patrol_2` | **PR:** #856

## Problem Statement

We need end-to-end Patrol tests that exercise the loan workflow through the Flutter UI, catching regressions that server-only integration tests miss. The loan workflow was migrated to a chat-oriented pattern (issue #838), where all actions happen via conversation screen action buttons and dropdown menus — not modals.

## Current State (as of 2026-03-17)

### What's working

- **Patrol 4.3.0** + patrol_cli 4.2.0 + patrol_finders 3.1.0
- **All tests pass locally AND in CI** (PR #856, patrol-test job: 8m36s)
- **Passing tests:**
  - `minimal_test.dart` — smoke test (app launches, login screen visible)
  - `loan_borrower_flow_test.dart` — request to borrow (login → discover → gear → Request → conversation)
  - `loan_borrower_lifecycle_test.dart` — full borrower lifecycle (27s, 15 steps)
  - `loan_owner_lifecycle_test.dart` — full owner lifecycle (26s, 13 steps)
- **Removed:** `app_smoke_test.dart` and `login_test.dart` (redundant with lifecycle tests; `minimal_test.dart` kept as diagnostic fallback)
- **Test infrastructure:**
  - `patrol_test/helpers/test_setup.dart` — `testSetup($, email:)` handles reset + launch + login + permissions + consent + onboarding; `navigateToGear($, name)` handles discover tab + find + tap
  - `patrol_test/helpers/test_action_client.dart` — Dart client for `TestActionClient` with all loan RPC actions + `resetTestData()` + `getTestData()`
  - `server/cmd/patrol-test-server/` — test server with `/test-actions` (port 8090), `/test-data`, `/test-reset` endpoints
  - `env.patrol.json` — separate config with `CACHE_TTL_MINUTES=0`
  - Media seeding: `community.jpg` and `tent.jpg` uploaded and attached during seed
- **UI instrumentation for testing:**
  - `_ChatActionButton` auto-generates Keys from label (e.g., `conversation_action_borrowing`, `conversation_action_managing`)
  - Gear detail comment bar uses Keys: `comment_bar_comment_area`, `comment_bar_placeholder`, `comment_bar_action_button`
- **CI/CD:**
  - GitHub Actions workflow `.github/workflows/test_patrol.yaml` runs all patrol tests
  - Self-hosted Linux runner with KVM-accelerated Android emulator (API 34, Pixel 6)
  - Test server started with `-actions-port 8090` for multi-user RPC coordination
  - patrol_cli 4.2.0 matches local version

### What needs to be done

- **iOS**: blocked by Xcode 26 incompatibilities, revisit after Android CI works
- **Other workflows**: giveaway, request, experience tests using same infrastructure

## Key Architecture

### Chat-oriented loan workflow (issue #838)

The loan UI uses the conversation screen, not dedicated modals:

- **Borrower**: gear detail → tap "Request" → auto-express interest + auto-approve → navigate to conversation → pickup scheduling sheet auto-opens → after confirming, action button shows "Scheduled" → owner starts loan via RPC → gRPC stream updates button to "Borrowing" → dropdown → "Mark as Returned"
- **Owner**: gear detail → tap chat pill (`comment_bar_comment_area`) → conversation → action button "Managing" → dropdown → "Mark as Picked Up" → button changes to "On Loan" → dropdown → "Mark as Returned"
- **Real-time updates**: Conversation screen uses persistent gRPC streaming (not polling). When the other user's RPC changes state, a system message is broadcast and the action button updates instantly. No navigation/refresh needed.
- **Widget cards**: `LoanActiveCard` (STARTED system message), `LoanCompletionCard` (COMPLETED system message) appear inline in chat, inside collapsible system message sections
- **Pickup scheduling**: `PickupSchedulingSheet` bottom sheet auto-opens after auto-approval with `QuickDateChips`, `QuickTimeChips`, `DurationChips`

### Multi-user testing strategy

Each test drives **one user's UI**. The other user's actions are triggered via HTTP POST to `http://10.0.2.2:8090/test-actions` (Android emulator) with JSON body `{ "user": "owner"|"borrower", "action": "start-loan", "transfer_id": "..." }`. The `TestActionClient` Dart helper wraps this. The conversation screen's gRPC stream delivers the state change in real-time — just poll with `$.pump()` until the action button key appears.

### Test data lifecycle

- `/test-reset` truncates all DB tables and re-seeds fresh data (users, community, gear with media)
- Called at the start of every test via `testSetup()` for idempotent runs
- Android Test Orchestrator's `clearPackageData = "true"` gives each test file a fresh app instance

## Hard-Won Lessons

1. **Port 8081 is reserved by Patrol** for its `MobileAutomatorClient`. Use 8090+ for test-action servers.
2. **Patrol's `tap()` internally calls `pumpAndTrySettle`** after every tap (default `SettlePolicy.trySettle`, 10s timeout). The conversation screen has persistent animations/timers that cause every tap to wait the full 10s timeout. **Fix:** pass `settlePolicy: SettlePolicy.noSettle` to all `tap()` calls on the conversation screen, then follow with explicit `$.pump(Duration(...))`.
3. **Use Discover tab to find gear**, not the feed. The feed is a vertical `PageView` where `scrollTo()`, `drag()`, and `fling()` all fail to reliably advance pages.
4. **Consent dialog appears asynchronously** after login — use a polling loop, not a one-shot check. The `testSetup()` helper handles this.
5. **Onboarding screens** appear for fresh test users — tap "Skip". Also handled by `testSetup()`.
6. **Bottom nav icons**: home = `Icons.home_outlined`, discover = `Icons.place_outlined`, metrics = `RiplsIcons.ripls`, add = `+` button.
7. **`flutter clean` required** after Patrol/dependency upgrades or Mapbox Gradle builds fail with stale cache.
8. **`PatrolIntegrationTester`** (from `package:patrol`) is the type for `$` in shared helpers, not `PatrolTester` (from `package:patrol_finders`). Only `PatrolIntegrationTester` has `$.platform`.
9. **`--invite-link-hostname test.ripls.app`** is required when starting the Ripls server (added in invitation system work).
10. **Use auto-generated widget Keys** for interactive elements that may have duplicate text on screen. The `_ChatActionButton` generates keys like `conversation_action_borrowing` from its label. Text selectors like `$('Borrowing')` can match multiple widgets (e.g., action button AND `LoanActiveCard` subtitle).
11. **Gear detail action bar keys**: `comment_bar_comment_area` (opens conversation), `content_action_bar_helper_section` (shows requesters), `content_action_bar_action_button` ("Review"/"Request"). There is no `gear_chat_button` key.
12. **Pickup sheet auto-opens** after tapping "Request" (auto-approval triggers it). No need to manually tap "Scheduled" → "Set Pickup Details".
13. **Conversation updates via gRPC streaming**, not polling. After an RPC changes state, the action button updates in real-time. Stay on the conversation screen and poll for the new button key — no navigate-back-and-reopen needed.
14. **Completion card is inside collapsed system message**. Verify completion via `$('This transfer has been completed')` text, not `$('Loan Complete')` which requires expanding the system message.
15. **Hung tests block subsequent runs** ("0 tests" result). Kill with `adb shell am force-stop org.ripls.app.dev` + `adb shell pm clear org.ripls.app.dev` before retrying.
16. **`SettlePolicy` is re-exported by `package:patrol/patrol.dart`** — no need to separately import `package:patrol_finders/patrol_finders.dart` (causes `unnecessary_import` lint).
17. **CI uses `10.0.2.2` → host localhost mapping** — the Android emulator maps `10.0.2.2` to the host machine's `localhost`, so the test server on `localhost:8090` is reachable from the app at `10.0.2.2:8090`.

## File Reference

```text
patrol_test/
├── helpers/
│   ├── test_setup.dart           # testSetup(), navigateToGear()
│   └── test_action_client.dart   # TestActionClient, TestData, ActionResult
├── pages/
│   ├── pages.dart                # barrel export
│   ├── login_page.dart
│   ├── home_page.dart            # feedTab, discoverTab (place_outlined)
│   ├── gear_detail_page.dart     # chatPill, actionButton, helperSection
│   ├── conversation_page.dart    # actionButton(label), tapDropdownItem(), noSettle
│   ├── discover_page.dart
│   ├── inbox_page.dart
│   ├── transfer_modal_page.dart  # legacy, for express interest via modal
│   ├── loan_borrower_modal_page.dart  # legacy modal page object
│   └── loan_owner_modal_page.dart     # legacy modal page object
├── loan_borrower_flow_test.dart       # ✅ PASSING (local + CI)
├── loan_borrower_lifecycle_test.dart  # ✅ PASSING (local + CI)
├── loan_owner_lifecycle_test.dart     # ✅ PASSING (local + CI)
└── minimal_test.dart                  # ✅ PASSING (diagnostic fallback)

server/cmd/patrol-test-server/
├── main.go          # server startup, seeding, /test-reset, /test-data
├── actions.go       # /test-actions handler (all loan RPC actions)
├── media.go         # uploadTestMedia() with embedded files
└── test_media/      # community.jpg, tent.jpg (embedded in binary)

.github/workflows/test_patrol.yaml    # CI: self-hosted Linux + KVM emulator

app/env.patrol.json  # CACHE_TTL_MINUTES=0, ENVIRONMENT=local
```

## Implementation Plan

### Completed

- [x] **Phase 0**: Patrol 4.3 upgrade, `$.native` → `$.platform.mobile`, `env.patrol.json`, patrol_cli 4.2
- [x] **Infrastructure**: `/test-actions` (8 actions), `/test-data`, `/test-reset`, `TestActionClient`, media seeding
- [x] **Shared helpers**: `testSetup()`, `navigateToGear()`
- [x] **Borrower flow test**: login → discover → gear → Request → conversation (passing, idempotent)
- [x] **Widget Keys**: `_ChatActionButton` auto-generated keys (`conversation_action_*`), gear detail action bar keys (`content_action_bar_*`)
- [x] **Borrower lifecycle test**: Request → pickup sheet → owner RPC start → Borrowing → Mark as Returned → completed (27s, 15 steps)
- [x] **Owner lifecycle test**: borrower RPC interest → chat pill → Managing → Mark as Picked Up → On Loan → Mark as Returned → completed (26s, 13 steps)
- [x] **ConversationPage page object**: Key-based `actionButton(label)`, `tapDropdownItem()`, noSettle for all taps
- [x] **GearDetailPage page object**: `chatPill`, `actionButton`, `helperSection` with correct Keys
- [x] **CI/CD**: all patrol tests pass in GitHub Actions (8m36s, self-hosted Linux runner with KVM emulator, patrol_cli 4.2.0, actions server on port 8090)
- [x] **Test cleanup**: removed redundant `app_smoke_test.dart` and `login_test.dart`; kept `minimal_test.dart` as diagnostic fallback

### Test flow details

#### Borrower lifecycle (`loan_borrower_lifecycle_test.dart`)

```text
testSetup($, email: 'borrower@test.ripls.org')
navigateToGear($, 'Test Camping Tent')
→ Tap "Request" → conversation + pickup sheet auto-opens
→ Select "Today" / "Morning" / "1 week" → "Confirm Pickup Details"
→ actions.startLoan(user: 'owner', transferId: ...)  // owner RPC
→ gRPC stream delivers state change → poll for Key('conversation_action_borrowing')
→ Tap borrowing button → dropdown → "Mark as Returned"
→ Verify "This transfer has been completed"
```

#### Owner lifecycle (`loan_owner_lifecycle_test.dart`)

```text
testSetup($, email: 'owner@test.ripls.org')
actions.expressInterest(user: 'borrower', gearId: ...)  // borrower RPC
navigateToGear($, 'Test Camping Tent')
→ Tap Key('comment_bar_comment_area') → conversation screen
→ Poll for Key('conversation_action_managing')
→ Tap managing button → dropdown → "Mark as Picked Up"
→ Poll for Key('conversation_action_on_loan')
→ Tap on-loan button → dropdown → "Mark as Returned"
→ Verify "This transfer has been completed"
```

#### Key interaction patterns for all future tests

| Pattern | How | Notes |
| ------- | --- | ----- |
| Find action button | `$(Key('conversation_action_<label>'))` | Label lowercased, spaces → underscores |
| Tap dropdown item | `button.tap(settlePolicy: noSettle)` then `$('Item Text').tap(settlePolicy: noSettle)` | Always use noSettle on conversation screen |
| Wait for state change | Poll with `$.pump(Duration(seconds: 1))` loop | gRPC stream delivers updates in real-time |
| Enter conversation (owner) | `$(Key('comment_bar_comment_area')).tap()` | From gear detail action bar |
| Enter conversation (borrower) | `$('Request').tap()` | Auto-approves and navigates to conversation |
| Verify completion | `$('This transfer has been completed')` | System message text (card is collapsed) |
| Other user's action | `TestActionClient` HTTP POST to test server | gRPC stream updates conversation automatically |

### Future work

- [ ] **iOS testing**: revisit once Xcode 26 / CocoaPods / Patrol iOS compatibility improves
- [ ] **Additional test media**: add `mixer.jpg` and `experience.jpg` to seed data for multi-item and experience tests
- [ ] **Other workflows**: giveaway, request, experience tests using same infrastructure
- [ ] **Go tests for action handler**: verify routing and auth in `/test-actions`

## Running Tests

```bash
# Start patrol test server (from server/ directory)
cd server && go run ./cmd/patrol-test-server -port 8080 -actions-port 8090

# Run a specific test
cd app && patrol test -t patrol_test/loan_borrower_lifecycle_test.dart \
  --dart-define-from-file=env.patrol.json -d emulator-5554

# Run all patrol tests
cd app && patrol test --dart-define-from-file=env.patrol.json -d emulator-5554

# Run via npm script (starts server automatically)
npm run test:patrol
```
