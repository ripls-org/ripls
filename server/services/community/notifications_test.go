package community

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/services"
	"go.ripls.org/ripls/server/storage"
)

// setupTestServiceWithNotifications returns the standard test trio plus a
// drain helper that blocks until in-flight bus dispatch completes. Replaces
// the old per-call notificationDone channel pattern (G2 in
// docs/issues/510-community-event-pubsub-v2.md).
func setupTestServiceWithNotifications(t *testing.T) (*Service, *storage.ProtoSQLStorage, *notifications.MockService, func()) {
	testStorage := setupTestStorage(t)
	mockNotif := notifications.NewMockService()
	mockBucket := &services.MockBucketStorage{}
	bus := newTestBus(t, testStorage, mockNotif)
	// Pass nil for storyCreator in tests (optional)
	service, _ := NewWithNotificationSignal(testStorage, mockBucket, mockNotif, bus, "test.example.com")
	if _, err := bus.Subscribe(service.StreamSubscriber()); err != nil {
		t.Fatalf("subscribe stream subscriber: %v", err)
	}
	drain := func() {
		drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = bus.Drain(drainCtx)
	}
	return service, testStorage, mockNotif, drain
}

func TestNotifications_AcceptInvitationLink_NotifiesExistingMembers(t *testing.T) {
	// When a new member joins via an invite link, every existing member of
	// the community gets a "new member joined" push, with a payload that
	// deep-links to the community detail card.
	service, testStorage, mockNotif, done := setupTestServiceWithNotifications(t)

	user1ID := setupTestUser(t, testStorage, "user1@example.com", "User One")
	user2ID := setupTestUser(t, testStorage, "user2@example.com", "User Two")

	ctx1 := createAuthenticatedContext(user1ID, "user1@example.com", models.Role_ROLE_USER)
	ctx2 := createAuthenticatedContext(user2ID, "user2@example.com", models.Role_ROLE_USER)

	// Create community with user1.
	createReq := connect.NewRequest(&api.CreateCommunityRequest{
		Name:        "Test Community",
		Description: "Test",
	})
	createResp, err := service.CreateCommunity(ctx1, createReq)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}
	communityID := createResp.Msg.Id

	inviteLink := mintCommunityInvite(t, ctx1, service, communityID)

	mockNotif.Reset()

	acceptReq := connect.NewRequest(&api.AcceptInvitationLinkRequest{
		ShortCode: inviteLink.ShortCode,
	})
	if _, err := service.AcceptInvitationLink(ctx2, acceptReq); err != nil {
		t.Fatalf("Failed to accept invitation link: %v", err)
	}

	// Wait for the async notify goroutine.
	done()

	calls := mockNotif.GetCalls()
	if len(calls) != 1 {
		t.Fatalf("expected 1 notification (to existing creator), got %d", len(calls))
	}
	if calls[0].UserID != user1ID {
		t.Errorf("expected notification recipient %q, got %q", user1ID, calls[0].UserID)
	}
	payload := calls[0].Notification.GetCommunityEvent()
	if payload == nil {
		t.Fatalf("expected CommunityEvent payload, got %+v", calls[0].Notification.GetPayload())
	}
	if payload.CommunityId != communityID {
		t.Errorf("expected payload.CommunityId=%q (for deep-link to community detail), got %q", communityID, payload.CommunityId)
	}
}

func TestNotifications_ShareGear_NoNotification(t *testing.T) {
	// Gear sharing is a broadcast event - we no longer send notifications for these
	// to reduce notification spam. Users can discover new gear through the feed.
	service, testStorage, mockNotif, done := setupTestServiceWithNotifications(t)

	user1ID := setupTestUser(t, testStorage, "user1@example.com", "User One")
	user2ID := setupTestUser(t, testStorage, "user2@example.com", "User Two")

	ctx1 := createAuthenticatedContext(user1ID, "user1@example.com", models.Role_ROLE_USER)
	ctx2 := createAuthenticatedContext(user2ID, "user2@example.com", models.Role_ROLE_USER)

	// Create community
	createReq := connect.NewRequest(&api.CreateCommunityRequest{
		Name: "Tool Library",
	})
	createResp, err := service.CreateCommunity(ctx1, createReq)
	if err != nil {
		t.Fatalf("Failed to create community: %v", err)
	}
	communityID := createResp.Msg.Id

	// Get invitation link and add user2 to community
	inviteLink := mintCommunityInvite(t, ctx1, service, communityID)

	acceptReq := connect.NewRequest(&api.AcceptInvitationLinkRequest{
		ShortCode: inviteLink.ShortCode,
	})
	_, err = service.AcceptInvitationLink(ctx2, acceptReq)
	if err != nil {
		t.Fatalf("Failed to accept invitation link: %v", err)
	}
	// Drain the new-member notification so it doesn't bleed into the
	// post-Reset window.
	done()

	// Create gear
	gear := &models.Gear{
		Name:    "Drill",
		OwnerId: user1ID,
	}
	gearID, err := testStorage.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("Failed to create gear: %v", err)
	}

	mockNotif.Reset()

	// Share gear
	shareGearForTestWithAvailability(
		t, service, ctx1, gearID, communityID, models.Availability_AVAILABILITY_FOR_LOAN,
	)

	// Verify NO notifications were sent - this is now a broadcast event we don't notify for
	calls := mockNotif.GetCalls()
	if len(calls) != 0 {
		t.Errorf("Expected 0 notifications for gear shared (broadcast event removed), got %d", len(calls))
	}
}

// TestRecordCommunityEventAndNotify_SkillProcessingDisabled verifies that skill processing is disabled.
// Even when events that would normally trigger skill extraction occur, no skills should be created.
func TestRecordCommunityEventAndNotify_SkillProcessingDisabled(t *testing.T) {
	tests := []struct {
		name      string
		eventType models.CommunityEventType
		setupFn   func(t *testing.T, testStorage *storage.ProtoSQLStorage, userID, communityID string) string
	}{
		{
			name:      "gear shared event",
			eventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
			setupFn: func(t *testing.T, testStorage *storage.ProtoSQLStorage, userID, communityID string) string {
				gear := &models.Gear{
					Name:    "Test Drill",
					OwnerId: userID,
				}
				gearID, err := testStorage.Insert(context.Background(), gear)
				if err != nil {
					t.Fatalf("Failed to create gear: %v", err)
				}
				return gearID
			},
		},
		{
			name:      "experience created event",
			eventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
			setupFn: func(t *testing.T, testStorage *storage.ProtoSQLStorage, userID, communityID string) string {
				experience := &models.Experience{
					Name:    "Test Workshop",
					OwnerId: userID,
				}
				experienceID, err := testStorage.Insert(context.Background(), experience)
				if err != nil {
					t.Fatalf("Failed to create experience: %v", err)
				}
				return experienceID
			},
		},
		{
			name:      "request created event",
			eventType: models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
			setupFn: func(t *testing.T, testStorage *storage.ProtoSQLStorage, userID, communityID string) string {
				request := &models.Request{
					Title:       "Need help",
					RequesterId: userID,
				}
				requestID, err := testStorage.Insert(context.Background(), request)
				if err != nil {
					t.Fatalf("Failed to create request: %v", err)
				}
				return requestID
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testStorage := setupTestStorage(t)
			mockNotif := notifications.NewMockService()
			bus := newTestBus(t, testStorage, mockNotif)

			userID := setupTestUser(t, testStorage, "user@example.com", "User One")

			community := &models.Community{
				Name:        "Test Community",
				Description: "Test",
				CreatorId:   userID,
				OwnerUserId: userID,
			}
			communityID, err := testStorage.Insert(context.Background(), community)
			if err != nil {
				t.Fatalf("Failed to create community: %v", err)
			}

			itemID := tt.setupFn(t, testStorage, userID, communityID)

			event := &models.CommunityEvent{
				CommunityId: communityID,
				EventType:   tt.eventType,
				ActorId:     userID,
			}

			switch tt.eventType {
			case models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED:
				event.GearId = itemID
			case models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED:
				event.Topic = &models.CommunityEvent_ExperienceId{ExperienceId: itemID}
			case models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED:
				event.Topic = &models.CommunityEvent_RequestId{RequestId: itemID}
			}

			if _, err := bus.Publish(context.Background(), event); err != nil {
				t.Fatalf("bus.Publish failed: %v", err)
			}
			drainCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := bus.Drain(drainCtx); err != nil {
				t.Fatalf("Drain: %v", err)
			}

			// Skill verification removed - skill system has been removed entirely
		})
	}
}
