# Notification Audit and Testing Plan

## Overview

This plan outlines the work to audit and improve testing of push notifications in the Ripls application. The goal is to ensure that notifications are sent exactly when expected and that integration tests verify notification behavior.

## Current State Analysis

### Push Notification Implementation

**Location**: `server/services/community/notifications.go`

**Events that trigger notifications** (`shouldNotify` function):
- `TRANSFER_INTEREST_EXPRESSED` - notify gear owner
- `TRANSFER_RECIPIENT_SELECTED` - notify selected recipient (giveaways only - see note below)
- `REQUEST_OFFER_MADE` - notify request creator
- `REQUEST_FULFILLED` - notify all offerers (except actor)
- `EXPERIENCE_RSVP_YES` - notify experience owner
- `EXPERIENCE_RSVP_MAYBE` - notify experience owner

**Important Note on Auto-Approved Loans**: For loans, transfers are auto-approved and skip the `INTEREST_EXPRESSED` state - they go directly to `RECIPIENT_SELECTED`. In this case:
- The actor is the borrower (who triggered the auto-approval by expressing interest)
- The notification recipient would also be the borrower
- The safety check "don't notify actor" correctly prevents self-notification
- **Result**: No notifications are sent for auto-approved loans

For giveaways, the owner manually selects the recipient, so:
- The actor is the owner
- The notification recipient is the selected user
- The safety check allows the notification
- **Result**: Notifications work correctly for giveaways

**Events that do NOT trigger notifications** (documented in push_notifications.md):
- `REQUEST_CREATED`, `REQUEST_CANCELLED` - low value, would spam
- `GEAR_SHARED`, `INVITATION_*` - broadcast events
- System chat messages - handled by community events

### Workflow Notification Gaps

After reviewing the workflow documents and notification implementation, the following gaps exist between documented workflow steps and notification triggers:

| Workflow | Event | Currently Notified? | Should Notify? |
|----------|-------|---------------------|----------------|
| Loan (auto-approved) | Interest expressed | No (skipped) | N/A - event not created |
| Loan (auto-approved) | Recipient selected | No (actor==recipient) | No - correct behavior |
| Loan | Loan started (ACTIVE) | No | Consider |
| Loan | Loan completed | No | Consider |
| Loan | Loan cancelled | No | Consider |
| Giveaway | Interest expressed | Yes (owner) | Yes |
| Giveaway | Recipient selected | Yes (recipient) | Yes |
| Giveaway | Giveaway completed | No | Consider |
| Request | Offer made | Yes (creator) | Yes |
| Request | Request fulfilled | Yes (offerers) | Yes |
| Request | Request cancelled | No | Low value |
| Experience | RSVP Yes | Yes (owner) | Yes |
| Experience | RSVP Maybe | Yes (owner) | Yes |
| Experience | In Process | No | Consider |
| Experience | Completed | No | Consider |
| Experience | Cancelled | No | Consider (attendees) |

### No-Op Provider Current State

**Location**: `server/notifications/noop/provider.go`

The noop provider logs notifications with the message "noop notification (not actually sent)". Integration tests can verify notifications by capturing server logs and parsing for these messages.

---

## Work Plan

### Phase 1: Clean Up Documentation

**Status**: [x] Complete

**Task 1.1**: Revise `docs/push_notifications.md`
- [x] Remove "Implementation Status" section (historical tracking of phases)
- [x] Remove phase markers (✅ Phase 1, ✅ Phase 2, etc.)
- [x] Remove historical implementation notes
- [x] Keep architecture, design principles, configuration, and testing sections
- [x] Update any outdated information
- [x] Ensure the document is useful as a current reference

**Task 1.2**: Verify document accuracy
- [x] Cross-reference `shouldNotify()` in `notifications.go` with documented events
- [x] Cross-reference `buildNotification()` with documented notification content
- [x] Verify server configuration flags are current
- [x] Verify client configuration is current

---

### Phase 2: Update Workflow Documents with Notification Expectations

**Status**: [x] Complete

For each workflow document, add a "Notifications" section that specifies:
1. Which steps trigger notifications
2. Who receives each notification
3. What the notification content should be

**Task 2.1**: Update `docs/workflows/loan.md`
- [x] Add "### Notifications" subsection under "Side Effects"
- [x] Document which events trigger notifications and to whom
- [x] Update workflow examples to include notification expectations in postconditions

**Task 2.2**: Update `docs/workflows/giveaway.md`
- [x] Add "### Notifications" subsection under "Side Effects"
- [x] Document which events trigger notifications and to whom
- [x] Update workflow examples to include notification expectations in postconditions

**Task 2.3**: Update `docs/workflows/request.md`
- [x] Add "### Notifications" subsection under "Side Effects"
- [x] Document which events trigger notifications and to whom
- [x] Update workflow examples to include notification expectations in postconditions

**Task 2.4**: Update `docs/workflows/experience.md`
- [x] Add "### Notifications" subsection under "Side Effects"
- [x] Document which events trigger notifications and to whom
- [x] Update workflow examples to include notification expectations in postconditions

**Task 2.5**: Update `docs/testing/update_system_tests.md`
- [x] Add section on notification testing expectations
- [x] Document that tests should verify notifications using log capture
- [x] Provide example patterns for notification assertions

---

### Phase 3: Test Infrastructure for Notification Verification

**Status**: [x] Complete (revised approach)

**Decision Change**: After experimentation, we decided to use **log capture** instead of a test provider with capture mode. The noop provider logs "sending notification to user" messages, and integration tests capture these logs for verification. This approach is simpler and works well for external process testing.

**Task 3.1**: ~~Design the test provider interface~~ → Use log capture instead
- [x] Keep the existing noop provider (logs notifications)
- [x] Integration tests capture server stdout/stderr
- [x] Parse JSON logs to find notification-related messages

**Task 3.2**: ~~Rename and upgrade to test provider~~ → Not needed
- [x] Noop provider remains at `server/notifications/noop/`
- [x] No code changes needed to provider

**Task 3.3**: Add test helpers in `server/integration_tests/test_helpers.go`
- [x] Added `LogCapture` struct with `ParseFrom()` and thread-safe storage
- [x] Added `GetNotificationLogs()` to find "sending notification to user" messages
- [x] Added `GetNotificationLogsForUser(userID)` helper
- [x] Added `WaitForNotificationLog(predicate, timeout)` for async handling
- [x] Added `GetNoopNotificationLogs()` for noop provider messages
- [x] Added `startTestServerWithLogCapture()` helper

---

### Phase 4: Expand Integration Tests with Notification Verification

**Status**: [ ] In Progress

Update existing integration tests to verify notifications are sent correctly.

**Task 4.1**: Update `server/integration_tests/loan_test.go`
- [x] Add notification verification to Example 1 (happy path)
  - ~~Verify owner notified when borrower expresses interest~~ N/A - auto-approved loans skip INTEREST_EXPRESSED
  - ~~Verify borrower notified when selected~~ N/A - actor==recipient prevents self-notification
  - [x] Added `Postcondition_NoNotificationsForAutoApprovedLoans` test verifying no notifications sent
- [x] Add notification verification to Example 2 (withdrawal)
  - [x] Added `Postcondition_NoNotificationsForWithdrawal` verifying no notifications for auto-approved loans or withdrawal
- [x] Add notification verification to Example 3 (owner declines)
  - [x] Added `Postcondition_NoNotificationsForCancellation` verifying only interest notification sent (test uses giveaway)
- [x] Add notification verification to Example 4 (cancelled after approval)
  - [x] Added `Postcondition_NoNotificationsForCancellation` verifying no notifications for auto-approved loans

**Task 4.2**: Update `server/integration_tests/giveaway_test.go`
- [x] Add notification verification to Example 1 (happy path)
  - [x] Added `Postcondition_OwnerNotifiedOnInterest` verifying owner receives 3 notifications (one per user)
  - [x] Added `Postcondition_RecipientNotifiedOnSelection` verifying User B receives notification when selected
- [x] Add notification verification to Example 2 (owner cancels)
  - [x] Added `Postcondition_NoNotificationsForCancellation` verifying only interest notifications sent
- [x] Add notification verification to Example 3 (user withdraws)
  - [x] Added `Postcondition_NoNotificationsForWithdrawal` verifying only interest notifications sent

**Task 4.3**: Update `server/integration_tests/request_test.go`
- [x] Add notification verification to Example 1 (fulfilled)
  - [x] Added `Postcondition_RequesterNotifiedOnOffers` verifying requester receives 3 notifications
  - [x] Added `Postcondition_HelpersNotifiedOnFulfillment` verifying each helper receives fulfillment notification
- [x] Add notification verification to Example 2 (cancelled)
  - [x] Added `Postcondition_NoNotificationsForCancellation` verifying only offer notification sent
- [x] Add notification verification to remaining examples (Examples 3-7)
  - [x] Example 3: Verify only offer notifications, not withdrawal
  - [x] Example 4: Verify only offer notification, not withdrawal
  - [x] Example 5: Verify only offer notification, not deletion
  - [x] Example 6: Verify requester receives 2 offer notifications, helpers receive 2 fulfillment notifications (one per community)
  - [x] Example 7: Verify only offer notifications, not unsharing

**Task 4.4**: Update `server/integration_tests/experience_test.go`
- [x] Add notification verification to Example 1 (completed)
  - [x] Added `Postcondition_OwnerNotifiedOnRSVPs` verifying owner receives 3 RSVP notifications
- [x] Add notification verification to Example 2 (cancelled)
  - [x] Added `Postcondition_NoNotificationsForCancellation` verifying only RSVP notifications sent
- [x] Add notification verification to remaining examples (Examples 3-6)
  - [x] Example 3: Verify only initial RSVP notifications, not RSVP No
  - [x] Example 4: Verify only initial RSVP notification, not RSVP No
  - [x] Example 5: Verify owner receives 2 RSVP notifications (cross-community)
  - [x] Example 6: Verify only RSVP notifications, not unsharing

---

### Phase 5: Review and Decide on Additional Notifications

**Status**: [x] Complete

Based on the audit, the following additional notifications were approved and implemented:

**Task 5.1**: Review candidates for additional notifications
- [ ] Loan started (ACTIVE) - decided not to add
- [ ] Loan completed - decided not to add
- [x] Loan/giveaway cancelled - notify other party
- [ ] Giveaway completed - decided not to add
- [x] Experience created - notify all community members
- [ ] Experience starting soon (scheduled reminder) - decided not to add
- [x] Experience cancelled - notify all RSVPed attendees (Yes/Maybe)

**Task 5.2**: Implemented additions:
- [x] Updated `shouldNotify()` function (added TRANSFER_CANCELLED, EXPERIENCE_CREATED)
- [x] Updated `getNotificationRecipients()` function (added recipient logic)
- [x] Updated `buildNotification()` function (loan/giveaway-specific wording)
- [x] Added `getCommunityMemberIDs()` helper for community-wide notifications
- [x] Fixed `notifyRSVPedUsers()` to only notify YES/MAYBE RSVPs
- [x] Updated all workflow documentation
- [x] Added unit tests for all new notification functionality
- [x] Updated integration tests to reflect new notification counts

---

## Progress Tracking

### Overall Progress

| Phase | Description | Status | Completion |
|-------|-------------|--------|------------|
| 1 | Clean up documentation | Complete | 100% |
| 2 | Update workflow docs | Complete | 100% |
| 3 | Test infrastructure | Complete | 100% |
| 4 | Expand integration tests | Complete | 100% |
| 5 | Review additional notifications | Complete | 100% |

### Detailed Task Tracking

```
Phase 1: Documentation Cleanup
  [x] 1.1 Revise push_notifications.md
  [x] 1.2 Verify document accuracy

Phase 2: Workflow Documents
  [x] 2.1 Update loan.md (notifications section added, postconditions updated)
  [x] 2.2 Update giveaway.md
  [x] 2.3 Update request.md
  [x] 2.4 Update experience.md
  [x] 2.5 Update update_system_tests.md

Phase 3: Test Infrastructure (revised approach - use log capture)
  [x] 3.1 Decision: use log capture instead of test provider
  [x] 3.2 Keep noop provider (logs notifications)
  [x] 3.3 Add log capture helpers to test_helpers.go

Phase 4: Integration Tests
  [x] 4.1 Update loan_test.go (all 4 examples have notification verification)
  [x] 4.2 Update giveaway_test.go (all 3 examples have notification verification)
  [x] 4.3 Update request_test.go (all 7 examples have notification verification)
  [x] 4.4 Update experience_test.go (all 6 examples have notification verification)

Phase 5: Additional Notifications
  [x] 5.1 Review candidates (loan cancelled, experience created, experience cancelled approved)
  [x] 5.2 Implement approved additions (with unit tests and integration test updates)
```

---

## Technical Notes

### Mock Provider Design

```go
// CapturedNotification holds a captured notification for test verification
type CapturedNotification struct {
    UserID       string
    DeviceToken  string
    Notification *models.Notification
    Timestamp    time.Time
}

// MockService captures all notifications for testing
type MockService struct {
    mu            sync.RWMutex
    notifications []CapturedNotification
    waiters       []chan struct{}
}

// NotifyUser captures the notification instead of sending
func (m *MockService) NotifyUser(ctx context.Context, userID string, notification *models.Notification) error {
    m.mu.Lock()
    m.notifications = append(m.notifications, CapturedNotification{
        UserID:       userID,
        Notification: notification,
        Timestamp:    time.Now(),
    })
    // Wake up any waiters
    for _, ch := range m.waiters {
        select {
        case ch <- struct{}{}:
        default:
        }
    }
    m.mu.Unlock()
    return nil
}
```

### Test Pattern

```go
func TestLoan_Example1_SharingAndBorrowingGear(t *testing.T) {
    // ... existing setup ...
    mockNotifications := startTestServerWithMockNotifications(t, dbURL)

    // ... existing test steps ...

    // Notification verification
    t.Run("Notification_OwnerNotifiedOnInterest", func(t *testing.T) {
        notifications := mockNotifications.GetNotificationsForUser(ownerID)
        require.Len(t, notifications, 1)
        assert.Equal(t, "Interest in Mountain Bike", notifications[0].Notification.Title)
        assert.Contains(t, notifications[0].Notification.Body, "Borrower A wants to borrow")
    })
}
```

### Async Notification Handling

The current implementation sends notifications asynchronously via goroutines:
```go
go func() {
    notifyTargetedUsers(context.Background(), s, notificationService, event)
    if notificationDone != nil {
        notificationDone <- struct{}{}
    }
}()
```

The `notificationDone` channel is already available for synchronization in tests. The mock service should leverage this or provide a similar mechanism.

---

## Decisions Made

1. **Additional notifications**: Will consider in Phase 5, including experience created (notify community). Deferred to end of work.

2. **Test granularity**: Notification verification will be part of existing workflow tests as postconditions/side effects, not separate test functions.

3. **Provider approach** (REVISED): Keep the existing noop provider and use **log capture** for integration tests. The noop provider logs notification attempts, and tests parse the JSON logs to verify notifications. This is simpler than a test provider with capture mode and works better for external process testing.

4. **Execution order**: Follow phase order 1 → 2 → 3 → 4 → 5.

5. **Auto-approved loan notifications**: No notifications should be sent for auto-approved loans. This is correct behavior because:
   - Auto-approved loans skip the INTEREST_EXPRESSED event (go directly to RECIPIENT_SELECTED)
   - For RECIPIENT_SELECTED, actor == recipient (the borrower), so the safety check correctly prevents self-notification
   - The loan.md documentation has been updated to reflect this

## Open Questions

1. **Notification content**: Should we standardize notification titles/bodies more strictly, or allow flexibility?

2. **Multi-community experiences**: When an experience is cancelled, should we notify attendees from all communities, or per-community?

---

## Files to Modify

### Documentation
- `docs/push_notifications.md` - cleanup and update
- `docs/workflows/loan.md` - add notification section
- `docs/workflows/giveaway.md` - add notification section
- `docs/workflows/request.md` - add notification section
- `docs/workflows/experience.md` - add notification section
- `docs/testing/update_system_tests.md` - add notification testing guidance

### Server Code (Rename/Upgrade)
- `server/notifications/noop/` → `server/notifications/test/` - upgrade to test provider with capture mode

### Server Code (Modify)
- `server/main.go` - update imports for renamed provider
- `server/services/community/notifications.go` - if adding new notification types (Phase 5)

### Tests (Modify)
- `server/integration_tests/test_helpers.go` - add mock notification setup
- `server/integration_tests/loan_test.go` - add notification verification
- `server/integration_tests/giveaway_test.go` - add notification verification
- `server/integration_tests/request_test.go` - add notification verification
- `server/integration_tests/experience_test.go` - add notification verification

---

## Estimated Effort

| Phase | Estimated Time | Priority |
|-------|---------------|----------|
| 1 | 1-2 hours | High |
| 2 | 2-3 hours | High |
| 3 | 2-3 hours | High |
| 4 | 4-6 hours | High |
| 5 | TBD (depends on decisions) | Medium |

Total: 9-14 hours + Phase 5 (decision-dependent)

---

## Last Updated

2026-01-22 - All phases complete:
- Phase 5: Complete - implemented loan/giveaway cancelled, experience created,
  and experience cancelled notifications with unit and integration tests
- All workflow documentation updated to reflect new notifications

2026-01-22 - Phase 4 complete:
- Phase 4.1: Complete (loan_test.go all 4 examples have notification verification)
- Phase 4.2: Complete (giveaway_test.go all 3 examples have notification verification)
- Phase 4.3: Complete (request_test.go all 7 examples have notification verification)
- Phase 4.4: Complete (experience_test.go all 6 examples have notification verification)
- Remaining: Phase 5 (review additional notifications)

2026-01-22 - Phase 4 progress (loan and giveaway tests complete):
- Phase 4.1: Complete (loan_test.go all 4 examples have notification verification)
- Phase 4.2: Complete (giveaway_test.go all 3 examples have notification verification)
- Remaining: Phase 4.3-4.4 (request, experience tests), Phase 5

2026-01-22 - Phases 1, 2, 3 complete; Phase 4 in progress:
- Phase 1: Complete (push_notifications.md cleaned up, accuracy verified)
- Phase 2: Complete (all workflow docs updated with notification sections)
- Phase 3: Complete (using log capture approach)
- Phase 4.1: Complete (loan_test.go verifies no notifications for auto-approved loans)
- Remaining: Phase 4.2-4.4 (giveaway, request, experience tests), Phase 5

2026-01-22 - Initial plan created
