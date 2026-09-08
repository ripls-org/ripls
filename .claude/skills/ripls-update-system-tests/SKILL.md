---
name: ripls-update-system-tests
description: Update system tests to match workflow documentation. Use this skill when the user types /ripls-update-system-tests or asks to update, sync, or fix system tests. Also triggers when workflow docs change and tests need updating.
---

# Update System Tests

Synchronize server integration tests with the workflow documentation in `docs/workflows/*.md`, following the instructions in `docs/testing/update_system_tests.md`.

## Workflow

### Phase 1 — Read workflow documents

Read all workflow examples from the source-of-truth documents:

1. `docs/workflows/loan.md`
2. `docs/workflows/giveaway.md`
3. `docs/workflows/request.md`
4. `docs/workflows/experience.md`

For each document, extract every numbered workflow example including:

- **Preconditions**: Setup requirements
- **Steps**: The sequence of user and system actions
- **Postconditions**: Verifiable outcomes (including "Notifications sent")

### Phase 2 — Read existing tests

Read the current integration tests:

1. `server/integration_tests/helpers_test.go`
2. `server/integration_tests/loan_test.go`
3. `server/integration_tests/giveaway_test.go`
4. `server/integration_tests/request_test.go`
5. `server/integration_tests/experience_test.go`

### Phase 3 — Diff and plan

For each workflow category (loans, giveaways, requests, experiences):

1. List every workflow example from the doc
2. List every top-level test function in the corresponding test file
3. Identify:
   - **Missing tests**: workflow examples with no corresponding test
   - **Stale tests**: tests that don't correspond to any workflow example
   - **Divergent tests**: tests whose preconditions, steps, or postconditions don't match the doc
   - **Missing notification checks**: tests that don't verify the "Notifications sent" postconditions

Present the diff summary to the user and get approval before making changes.

### Phase 4 — Implement changes

For each category, apply changes:

1. **Add tests** for missing workflow examples
2. **Update tests** where preconditions, steps, or postconditions have diverged
3. **Add notification verification** where missing (using `LogCapture` infrastructure)
4. **Remove tests** that don't correspond to any workflow example

### Phase 5 — Verify

Run the integration tests to verify they compile and pass:

```bash
cd server && go test ./integration_tests/ -v -count=1
```

Fix any compilation errors or test failures. If a test fails because the implementation doesn't match the documented behavior, follow the strict alignment requirement from the guide:

1. Try to fix the implementation, OR
2. Fix the documentation if the doc was wrong, OR
3. Add a TODO comment in the test explaining the divergence, with a permissive assertion

## Implementation guidelines

### Test naming

```go
func TestLoan_Example1_SharingAndBorrowingGear(t *testing.T)
func TestGiveaway_Example1_GivingAwayItem(t *testing.T)
```

### Test structure

Use subtests (`t.Run`) for steps and postconditions. Separate phases with comments:

```go
// ========== PRECONDITIONS ==========
// ========== STEPS ==========
// ========== POSTCONDITIONS ==========
```

### Notification testing

Use `startTestServerWithLogCapture` and verify notifications in postconditions:

```go
t.Run("Postcondition_NotificationsSent", func(t *testing.T) {
    WaitForNotificationCount(logCapture, expectedCount, 500*time.Millisecond)
    ownerNotifications := logCapture.GetNotificationLogsForUser(ownerID)
    // ... assertions ...
})
```

### Divergence TODOs

When a test can't match the documented postcondition:

```go
// TODO: Per docs/workflows/loan.md, gear should still be shared after return.
// Currently gear state is AVAILABLE instead of SHARED - investigate why.
// For now, using permissive assertion.
if gear.State != models.GearState_GEAR_STATE_SHARED {
    t.Errorf("Expected gear state SHARED, got %v", gear.State)
}
```

### Shared helpers

Shared infrastructure belongs in `helpers_test.go`:

- `startTestServer` / `startTestServerWithLogCapture`
- `registerFirstUser` / `registerUserByInvite`
- `createAuthGearClient` / `createAuthChatClient`
- `authTransport` type
