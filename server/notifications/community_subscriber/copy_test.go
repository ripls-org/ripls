package community_subscriber

import (
	"context"
	"strings"
	"testing"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// strPtr returns a pointer to s. Used to populate optional proto fields.
func strPtr(s string) *string { return &s }

// boolPtr returns a pointer to b. Used to populate optional bool proto fields.
func boolPtr(b bool) *bool { return &b }

// seedBuildNotification creates the canonical fixture set used by the
// buildNotification table-driven tests: an actor, two users, a gear, an
// experience, a request, and a transfer of each type. Returns a struct of
// IDs for the table cases to reference.
type bnFixtures struct {
	actorID, ownerID   string
	communityID        string
	gearID             string
	loanTransferID     string
	giveawayTransferID string
	requestID          string
	experienceID       string
	communityName      string
}

func seedBuildNotification(t *testing.T) (*storage.ProtoSQLStorage, bnFixtures) {
	t.Helper()
	s := setupTestStorage(t)

	actorID := insertTestUser(t, s, "actor@example.com", "Alice Actor")
	ownerID := insertTestUser(t, s, "owner@example.com", "Owen Owner")

	communityName := "Test Community"
	communityID := insertTestCommunity(t, s, communityName, actorID)

	gearID, err := s.Insert(context.Background(), &models.Gear{Name: "Cordless Drill", OwnerId: ownerID})
	if err != nil {
		t.Fatalf("insert gear: %v", err)
	}
	loanTransferID, err := s.Insert(context.Background(), &models.Transfer{
		GearId:       gearID,
		OwnerId:      ownerID,
		RecipientId:  actorID,
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		CommunityId:  communityID,
	})
	if err != nil {
		t.Fatalf("insert loan transfer: %v", err)
	}
	giveawayTransferID, err := s.Insert(context.Background(), &models.Transfer{
		GearId:       gearID,
		OwnerId:      ownerID,
		RecipientId:  actorID,
		TransferType: models.TransferType_TRANSFER_TYPE_GIVEAWAY,
		CommunityId:  communityID,
	})
	if err != nil {
		t.Fatalf("insert giveaway transfer: %v", err)
	}
	requestID, err := s.Insert(context.Background(), &models.Request{
		Title:       "Need a ladder",
		RequesterId: actorID,
	})
	if err != nil {
		t.Fatalf("insert request: %v", err)
	}
	experienceID, err := s.Insert(context.Background(), &models.Experience{Name: "Trail Day", OwnerId: ownerID})
	if err != nil {
		t.Fatalf("insert experience: %v", err)
	}

	return s, bnFixtures{
		actorID:            actorID,
		ownerID:            ownerID,
		communityID:        communityID,
		gearID:             gearID,
		loanTransferID:     loanTransferID,
		giveawayTransferID: giveawayTransferID,
		requestID:          requestID,
		experienceID:       experienceID,
		communityName:      communityName,
	}
}

func TestBuildNotification_AllNotifiedEventTypes(t *testing.T) {
	s, f := seedBuildNotification(t)
	ctx := context.Background()

	tests := []struct {
		name         string
		event        *models.CommunityEvent
		wantTitle    string // exact match; "" means skip exact-match check (use wantTitleContains)
		wantTitleHas string // substring match if wantTitle is ""
		wantBodyHas  []string
		wantPayload  bool // assert CommunityEvent payload is populated with expected fields
	}{
		{
			name: "TRANSFER_INTEREST_EXPRESSED",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_EXPRESSED,
				GearId:      f.gearID,
			},
			wantTitle:   "Interest in Cordless Drill",
			wantBodyHas: []string{"Alice Actor", "Cordless Drill", "borrow"},
		},
		{
			name: "TRANSFER_INTEREST_WITHDRAWN",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_INTEREST_WITHDRAWN,
				GearId:      f.gearID,
			},
			wantTitle:   "Interest withdrawn",
			wantBodyHas: []string{"no longer", "Cordless Drill"},
		},
		{
			name: "TRANSFER_RECIPIENT_SELECTED",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.ownerID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_RECIPIENT_SELECTED,
				GearId:      f.gearID,
			},
			wantTitle:   "Request approved",
			wantBodyHas: []string{"Cordless Drill", "picked you"},
		},
		{
			name: "TRANSFER_ACTIVE_LOAN",
			event: &models.CommunityEvent{
				CommunityId:  f.communityID,
				ActorId:      f.actorID,
				EventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE,
				GearId:       f.gearID,
				TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			},
			wantTitle:   "Loan started",
			wantBodyHas: []string{"Cordless Drill", "active"},
		},
		{
			name: "TRANSFER_ACTIVE_GIVEAWAY",
			event: &models.CommunityEvent{
				CommunityId:  f.communityID,
				ActorId:      f.actorID,
				EventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_ACTIVE,
				GearId:       f.gearID,
				TransferType: models.TransferType_TRANSFER_TYPE_GIVEAWAY,
			},
			wantTitle:   "Giveaway complete",
			wantBodyHas: []string{"Cordless Drill", "new owner"},
		},
		{
			name: "TRANSFER_CANCELLED_LOAN",
			event: &models.CommunityEvent{
				CommunityId:  f.communityID,
				ActorId:      f.actorID,
				EventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED,
				GearId:       f.gearID,
				TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			},
			wantTitle:   "Loan cancelled",
			wantBodyHas: []string{"Cordless Drill", "Alice Actor"},
		},
		{
			name: "TRANSFER_CANCELLED_GIVEAWAY",
			event: &models.CommunityEvent{
				CommunityId:  f.communityID,
				ActorId:      f.actorID,
				EventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_CANCELLED,
				GearId:       f.gearID,
				TransferType: models.TransferType_TRANSFER_TYPE_GIVEAWAY,
			},
			wantTitle:   "Giveaway cancelled",
			wantBodyHas: []string{"Cordless Drill", "Alice Actor"},
		},
		{
			name: "TRANSFER_PICKUP_PROPOSED",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_PICKUP_PROPOSED,
				GearId:      f.gearID,
			},
			wantTitle:   "Pickup proposed",
			wantBodyHas: []string{"Alice Actor", "Cordless Drill"},
		},
		{
			name: "REQUEST_OFFER_MADE",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_MADE,
				Topic:       &models.CommunityEvent_RequestId{RequestId: f.requestID},
			},
			wantTitle:   "Help offered",
			wantBodyHas: []string{"Alice Actor", "Need a ladder"},
		},
		{
			name: "REQUEST_OFFER_SELECTED",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_SELECTED,
				Topic:       &models.CommunityEvent_RequestId{RequestId: f.requestID},
			},
			wantTitle:   "Offer selected",
			wantBodyHas: []string{"Alice Actor", "Need a ladder"},
		},
		{
			name: "REQUEST_OFFER_WITHDRAWN",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_OFFER_WITHDRAWN,
				Topic:       &models.CommunityEvent_RequestId{RequestId: f.requestID},
			},
			wantTitle:   "Offer withdrawn",
			wantBodyHas: []string{"Alice Actor", "Need a ladder"},
		},
		{
			name: "REQUEST_FULFILLED",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED,
				Topic:       &models.CommunityEvent_RequestId{RequestId: f.requestID},
			},
			wantTitle:   "Request fulfilled",
			wantBodyHas: []string{"Need a ladder", "fulfilled"},
		},
		{
			name: "REQUEST_CANCELLED",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CANCELLED,
				Topic:       &models.CommunityEvent_RequestId{RequestId: f.requestID},
			},
			wantTitle:   "Request cancelled",
			wantBodyHas: []string{"Need a ladder", "cancelled"},
		},
		{
			name: "REQUEST_CREATED",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
				Topic:       &models.CommunityEvent_RequestId{RequestId: f.requestID},
			},
			wantTitle:   "New request",
			wantBodyHas: []string{"Alice Actor", "Need a ladder"},
		},
		{
			name: "GEAR_SHARED",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
				GearId:      f.gearID,
			},
			// No CommunityGear availability row → unspecified → loan branch.
			// Article is supplied by the template (name is article-free).
			wantTitle:   "New gear to borrow",
			wantBodyHas: []string{"Alice Actor", "lending out", "a Cordless Drill"},
		},
		{
			name: "EXPERIENCE_RSVP_YES",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_YES,
				Topic:       &models.CommunityEvent_ExperienceId{ExperienceId: f.experienceID},
			},
			wantTitle:   "New RSVP",
			wantBodyHas: []string{"Alice Actor", "attending", "Trail Day"},
		},
		{
			name: "EXPERIENCE_RSVP_MAYBE",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_RSVP_MAYBE,
				Topic:       &models.CommunityEvent_ExperienceId{ExperienceId: f.experienceID},
			},
			wantTitle:   "New RSVP",
			wantBodyHas: []string{"Alice Actor", "might", "Trail Day"},
		},
		{
			name: "EXPERIENCE_CREATED",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
				Topic:       &models.CommunityEvent_ExperienceId{ExperienceId: f.experienceID},
			},
			wantTitle:   "New event",
			wantBodyHas: []string{"Alice Actor", "Trail Day"},
		},
		{
			name: "EXPERIENCE_COMPLETED",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_COMPLETED,
				Topic:       &models.CommunityEvent_ExperienceId{ExperienceId: f.experienceID},
			},
			wantTitle:   "Event completed",
			wantBodyHas: []string{"Trail Day"},
		},
		{
			name: "EXPERIENCE_STARTED",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED,
				Topic:       &models.CommunityEvent_ExperienceId{ExperienceId: f.experienceID},
			},
			wantTitle:   "Event starting now",
			wantBodyHas: []string{"Trail Day", "starting"},
		},
		{
			name: "EXPERIENCE_CANCELLED",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CANCELLED,
				Topic:       &models.CommunityEvent_ExperienceId{ExperienceId: f.experienceID},
			},
			wantTitle:   "Event cancelled",
			wantBodyHas: []string{"Trail Day", "cancelled"},
		},
		{
			name: "EXPERIENCE_UPDATED_time_and_location",
			event: &models.CommunityEvent{
				CommunityId:     f.communityID,
				ActorId:         f.actorID,
				EventType:       models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED,
				Topic:           &models.CommunityEvent_ExperienceId{ExperienceId: f.experienceID},
				TimeChanged:     boolPtr(true),
				LocationChanged: boolPtr(true),
			},
			wantTitle:   "Event updated",
			wantBodyHas: []string{"Trail Day", "new time and place"},
			wantPayload: true,
		},
		{
			name: "EXPERIENCE_UPDATED_time_only",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED,
				Topic:       &models.CommunityEvent_ExperienceId{ExperienceId: f.experienceID},
				TimeChanged: boolPtr(true),
			},
			wantTitle:   "Event updated",
			wantBodyHas: []string{"Trail Day", "new time"},
		},
		{
			name: "EXPERIENCE_UPDATED_location_only",
			event: &models.CommunityEvent{
				CommunityId:     f.communityID,
				ActorId:         f.actorID,
				EventType:       models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED,
				Topic:           &models.CommunityEvent_ExperienceId{ExperienceId: f.experienceID},
				LocationChanged: boolPtr(true),
			},
			wantTitle:   "Event updated",
			wantBodyHas: []string{"Trail Day", "new location"},
		},
		{
			name: "PLANNING_NEED_ADDED",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_ADDED,
				Topic:       &models.CommunityEvent_ExperienceId{ExperienceId: f.experienceID},
			},
			wantTitle:   "New need",
			wantBodyHas: []string{"Alice Actor", "needs", "Trail Day"},
		},
		{
			name: "PLANNING_NEED_CLAIMED",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_CLAIMED,
				Topic:       &models.CommunityEvent_ExperienceId{ExperienceId: f.experienceID},
			},
			wantTitle:   "Need claimed",
			wantBodyHas: []string{"Alice Actor", "offered to bring", "Trail Day"},
		},
		{
			name: "PLANNING_CONTRIBUTION_ADDED",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_CONTRIBUTION_ADDED,
				Topic:       &models.CommunityEvent_ExperienceId{ExperienceId: f.experienceID},
			},
			wantTitle:   "New contribution",
			wantBodyHas: []string{"Alice Actor", "offered to bring", "Trail Day"},
		},
		{
			name: "INVITATION_LINK_USED",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_INVITATION_LINK_USED,
			},
			wantTitle:   "New member",
			wantBodyHas: []string{"Alice", "Test Community"},
		},
		{
			name: "COMMUNITY_RESTORED",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_RESTORED,
			},
			wantTitle:   "Community restored",
			wantBodyHas: []string{"Alice", "Test Community", "restored"},
		},
		{
			name: "OWNERSHIP_TRANSFERRED",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_OWNERSHIP_TRANSFERRED,
			},
			wantTitle:   "You're now the owner",
			wantBodyHas: []string{"Alice", "Test Community", "ownership"},
		},
		{
			name: "Unrecognized event type falls back to default",
			event: &models.CommunityEvent{
				CommunityId: f.communityID,
				ActorId:     f.actorID,
				EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_UNSPECIFIED,
			},
			wantTitle:   "New activity",
			wantBodyHas: []string{"posted an update", "Test Community"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := buildNotification(ctx, s, tt.event, enLoc(t))
			if n == nil {
				t.Fatal("buildNotification returned nil")
			}
			if tt.wantTitle != "" && n.Title != tt.wantTitle {
				t.Errorf("title = %q, want %q", n.Title, tt.wantTitle)
			}
			if tt.wantTitleHas != "" && !strings.Contains(n.Title, tt.wantTitleHas) {
				t.Errorf("title = %q, want it to contain %q", n.Title, tt.wantTitleHas)
			}
			for _, sub := range tt.wantBodyHas {
				if !strings.Contains(strings.ToLower(n.Body), strings.ToLower(sub)) {
					t.Errorf("body = %q, want it to contain %q (case-insensitive)", n.Body, sub)
				}
			}
			// Every notification carries a CommunityEvent payload with at
			// least the community id and event type filled in.
			payload := n.GetCommunityEvent()
			if payload == nil {
				t.Fatal("expected CommunityEvent payload, got nil")
			}
			if payload.CommunityId != tt.event.CommunityId {
				t.Errorf("payload.CommunityId = %q, want %q", payload.CommunityId, tt.event.CommunityId)
			}
			if payload.EventType != tt.event.EventType.String() {
				t.Errorf("payload.EventType = %q, want %q", payload.EventType, tt.event.EventType.String())
			}
		})
	}
}

func TestBuildNotification_CommunityDeletedReadsSoftDeletedRow(t *testing.T) {
	s := setupTestStorage(t)
	actorID := insertTestUser(t, s, "actor@example.com", "Olivia Owner")
	communityID := insertTestCommunity(t, s, "Doomed Community", actorID)

	// Soft-delete.
	c := &models.Community{}
	if err := s.GetByID(context.Background(), communityID, c); err != nil {
		t.Fatalf("get community: %v", err)
	}
	c.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: 1700000000, DeletedByUserId: actorID}
	if err := s.Update(context.Background(), c); err != nil {
		t.Fatalf("delete community: %v", err)
	}

	// The COMMUNITY_DELETED copy explicitly re-reads with
	// IncludeDeleted: true so the message can still name the community —
	// the ordinary read filter hides a soft-deleted row.
	n := buildNotification(context.Background(), s, &models.CommunityEvent{
		CommunityId: communityID,
		ActorId:     actorID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED,
	}, enLoc(t))
	if !strings.Contains(n.Body, "Doomed Community") {
		t.Errorf("body = %q, want it to name the soft-deleted community", n.Body)
	}
	if !strings.Contains(n.Body, "30 days to restore") {
		t.Errorf("body = %q, want it to mention the 30-day restore window", n.Body)
	}
}

func TestBuildNotification_PayloadCarriesOptionalEntityIDs(t *testing.T) {
	s, f := seedBuildNotification(t)

	// Event references both a request and an experience via Topic
	// alternatives. buildNotification populates payload.RequestId only
	// when GetRequestId() is set, and payload.ExperienceId only when
	// GetExperienceId() is set. Verify both branches.

	t.Run("request topic populates RequestId", func(t *testing.T) {
		n := buildNotification(context.Background(), s, &models.CommunityEvent{
			CommunityId: f.communityID,
			ActorId:     f.actorID,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
			Topic:       &models.CommunityEvent_RequestId{RequestId: f.requestID},
		}, enLoc(t))
		p := n.GetCommunityEvent()
		if p.RequestId == nil || *p.RequestId != f.requestID {
			t.Errorf("payload.RequestId = %v, want %q", p.RequestId, f.requestID)
		}
		if p.ExperienceId != nil {
			t.Errorf("payload.ExperienceId = %v, want nil for request-only event", p.ExperienceId)
		}
	})

	t.Run("experience topic populates ExperienceId", func(t *testing.T) {
		n := buildNotification(context.Background(), s, &models.CommunityEvent{
			CommunityId: f.communityID,
			ActorId:     f.actorID,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
			Topic:       &models.CommunityEvent_ExperienceId{ExperienceId: f.experienceID},
		}, enLoc(t))
		p := n.GetCommunityEvent()
		if p.ExperienceId == nil || *p.ExperienceId != f.experienceID {
			t.Errorf("payload.ExperienceId = %v, want %q", p.ExperienceId, f.experienceID)
		}
		if p.RequestId != nil {
			t.Errorf("payload.RequestId = %v, want nil for experience-only event", p.RequestId)
		}
	})
}

func TestGetUserName_MissingIDReturnsEmpty(t *testing.T) {
	s := setupTestStorage(t)
	if got := getUserName(context.Background(), s, "nonexistent-user"); got != "" {
		t.Errorf("getUserName for missing user = %q, want empty", got)
	}
}

func TestGetGearName_EmptyIDShortCircuits(t *testing.T) {
	s := setupTestStorage(t)
	if got := getGearName(context.Background(), s, ""); got != "" {
		t.Errorf("getGearName with empty id = %q, want empty", got)
	}
}

func TestGetExperienceName_EmptyIDShortCircuits(t *testing.T) {
	s := setupTestStorage(t)
	if got := getExperienceName(context.Background(), s, ""); got != "" {
		t.Errorf("getExperienceName with empty id = %q, want empty", got)
	}
}

func TestGetCommunityName_EmptyIDShortCircuits(t *testing.T) {
	s := setupTestStorage(t)
	if got := getCommunityName(context.Background(), s, ""); got != "" {
		t.Errorf("getCommunityName with empty id = %q, want empty", got)
	}
}

func TestGetRequestTitle_EmptyIDShortCircuits(t *testing.T) {
	s := setupTestStorage(t)
	if got := getRequestTitle(context.Background(), s, ""); got != "" {
		t.Errorf("getRequestTitle with empty id = %q, want empty", got)
	}
}

// Suppress unused-import / unused-var warnings when individual cases
// don't reference the broader fixture set.
var _ = strPtr

func TestShouldNotify_RemainingBranches(t *testing.T) {
	// The recipients_test.go::TestShouldNotify table covers most event
	// types. This fills the remaining branches in the switch so the
	// notify decision for every shipped enum value is exercised.
	cases := []struct {
		et   models.CommunityEventType
		want bool
	}{
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED, true},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CANCELLED, true},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_ADDED, true},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_CLAIMED, true},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_CONTRIBUTION_ADDED, true},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED, true},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_RESTORED, true},
		{models.CommunityEventType_COMMUNITY_EVENT_TYPE_OWNERSHIP_TRANSFERRED, true},
	}
	for _, tt := range cases {
		t.Run(tt.et.String(), func(t *testing.T) {
			if got := ShouldNotify(tt.et); got != tt.want {
				t.Errorf("ShouldNotify(%v) = %v, want %v", tt.et, got, tt.want)
			}
		})
	}
}

func TestGetNotificationRecipients_OwnershipTransferred(t *testing.T) {
	s := setupTestStorage(t)
	actorID := insertTestUser(t, s, "actor@example.com", "Actor")
	newOwnerID := insertTestUser(t, s, "newowner@example.com", "New Owner")
	communityID := insertTestCommunity(t, s, "Community", actorID)

	// ObjectUserId carries the new owner.
	got := getNotificationRecipients(context.Background(), s, &models.CommunityEvent{
		CommunityId:  communityID,
		ActorId:      actorID,
		ObjectUserId: newOwnerID,
		EventType:    models.CommunityEventType_COMMUNITY_EVENT_TYPE_OWNERSHIP_TRANSFERRED,
	})
	if len(got) != 1 || got[0] != newOwnerID {
		t.Errorf("ownership-transferred recipients = %v, want [%q]", got, newOwnerID)
	}
}

func TestGetNotificationRecipients_OwnershipTransferred_MissingObjectUser(t *testing.T) {
	s := setupTestStorage(t)
	actorID := insertTestUser(t, s, "actor@example.com", "Actor")
	communityID := insertTestCommunity(t, s, "Community", actorID)

	// ObjectUserId unset — recipient set is nil, logger emits a warning.
	got := getNotificationRecipients(context.Background(), s, &models.CommunityEvent{
		CommunityId: communityID,
		ActorId:     actorID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_OWNERSHIP_TRANSFERRED,
	})
	if got != nil {
		t.Errorf("ownership-transferred with no ObjectUserId = %v, want nil", got)
	}
}

func TestGetNotificationRecipients_CommunityRestored_BroadcastsToMembers(t *testing.T) {
	s := setupTestStorage(t)
	actorID := insertTestUser(t, s, "actor@example.com", "Actor")
	memberA := insertTestUser(t, s, "a@example.com", "A")
	memberB := insertTestUser(t, s, "b@example.com", "B")
	communityID := insertTestCommunity(t, s, "Restored", actorID)
	insertMembership(t, s, communityID, actorID)
	insertMembership(t, s, communityID, memberA)
	insertMembership(t, s, communityID, memberB)

	got := getNotificationRecipients(context.Background(), s, &models.CommunityEvent{
		CommunityId: communityID,
		ActorId:     actorID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_RESTORED,
	})
	// Actor is excluded.
	if len(got) != 2 {
		t.Errorf("got %d recipients, want 2 members excluding actor", len(got))
	}
	for _, r := range got {
		if r == actorID {
			t.Errorf("actor was included in recipients: %v", got)
		}
	}
}

func TestGetNotificationRecipients_CommunityDeleted_UsesDeletedSnapshot(t *testing.T) {
	s := setupTestStorage(t)
	actorID := insertTestUser(t, s, "actor@example.com", "Actor")
	memberA := insertTestUser(t, s, "a@example.com", "A")
	memberB := insertTestUser(t, s, "b@example.com", "B")
	communityID := insertTestCommunity(t, s, "Doomed", actorID)
	// Soft-delete with the snapshot — the recipient resolver reads
	// the snapshot exactly, even though community_user rows still
	// exist (deletion path may run before / parallel to cascade).
	c := &models.Community{}
	if err := s.GetByID(context.Background(), communityID, c); err != nil {
		t.Fatalf("get community: %v", err)
	}
	c.Deleted = &models.DeletedMetadata{DeletedByUserId: actorID, DeletedAtUnixSec: 1700000000}
	c.DeletedSnapshot = &models.CommunityDeletedSnapshot{
		MemberUserIds: []string{actorID, memberA, memberB},
	}
	if err := s.Update(context.Background(), c); err != nil {
		t.Fatalf("force-delete: %v", err)
	}

	got := getNotificationRecipients(context.Background(), s, &models.CommunityEvent{
		CommunityId: communityID,
		ActorId:     actorID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_COMMUNITY_DELETED,
	})
	if len(got) != 2 {
		t.Errorf("got %d recipients, want 2 snapshot members excluding deleter", len(got))
	}
}

func TestGetNotificationRecipients_PlanningEvents_IncludeYesAndMaybeRSVPsMinusActor(t *testing.T) {
	s := setupTestStorage(t)
	actorID := insertTestUser(t, s, "actor@example.com", "Actor")
	yesUser := insertTestUser(t, s, "yes@example.com", "Yes")
	maybeUser := insertTestUser(t, s, "maybe@example.com", "Maybe")
	noUser := insertTestUser(t, s, "no@example.com", "No")
	communityID := insertTestCommunity(t, s, "Community", actorID)

	expID, _ := s.Insert(context.Background(), &models.Experience{Name: "Trail Day", OwnerId: actorID})

	rsvps := []models.ExperienceRSVP{
		{ExperienceId: expID, UserId: actorID, CommunityId: communityID, Intention: models.RSVPIntention_RSVP_INTENTION_YES},
		{ExperienceId: expID, UserId: yesUser, CommunityId: communityID, Intention: models.RSVPIntention_RSVP_INTENTION_YES},
		{ExperienceId: expID, UserId: maybeUser, CommunityId: communityID, Intention: models.RSVPIntention_RSVP_INTENTION_MAYBE},
		{ExperienceId: expID, UserId: noUser, CommunityId: communityID, Intention: models.RSVPIntention_RSVP_INTENTION_NO},
	}
	for i := range rsvps {
		_, _ = s.Insert(context.Background(), &rsvps[i])
	}

	got := getNotificationRecipients(context.Background(), s, &models.CommunityEvent{
		CommunityId: communityID,
		ActorId:     actorID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_ADDED,
		Topic:       &models.CommunityEvent_ExperienceId{ExperienceId: expID},
	})
	gotSet := map[string]bool{}
	for _, id := range got {
		gotSet[id] = true
	}
	if !gotSet[yesUser] {
		t.Errorf("YES RSVP user missing from recipients: %v", got)
	}
	if !gotSet[maybeUser] {
		t.Errorf("MAYBE RSVP user missing from recipients: %v", got)
	}
	if gotSet[noUser] {
		t.Errorf("NO RSVP user should not be in recipients: %v", got)
	}
	if gotSet[actorID] {
		t.Errorf("actor should be excluded: %v", got)
	}
}

func TestGetNotificationRecipients_LifecycleEvents_IncludeYesAndMaybeRSVPsMinusActor(t *testing.T) {
	s := setupTestStorage(t)
	actorID := insertTestUser(t, s, "actor@example.com", "Actor")
	yesUser := insertTestUser(t, s, "yes@example.com", "Yes")
	communityID := insertTestCommunity(t, s, "Community", actorID)

	expID, _ := s.Insert(context.Background(), &models.Experience{Name: "Trail Day", OwnerId: actorID})

	for _, r := range []*models.ExperienceRSVP{
		{ExperienceId: expID, UserId: actorID, CommunityId: communityID, Intention: models.RSVPIntention_RSVP_INTENTION_YES},
		{ExperienceId: expID, UserId: yesUser, CommunityId: communityID, Intention: models.RSVPIntention_RSVP_INTENTION_YES},
	} {
		_, _ = s.Insert(context.Background(), r)
	}

	// EXPERIENCE_STARTED / EXPERIENCE_CANCELLED share the lifecycle
	// branch in getNotificationRecipients.
	for _, et := range []models.CommunityEventType{
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_STARTED,
		models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CANCELLED,
	} {
		t.Run(et.String(), func(t *testing.T) {
			got := getNotificationRecipients(context.Background(), s, &models.CommunityEvent{
				CommunityId: communityID,
				ActorId:     actorID,
				EventType:   et,
				Topic:       &models.CommunityEvent_ExperienceId{ExperienceId: expID},
			})
			if len(got) != 1 || got[0] != yesUser {
				t.Errorf("recipients = %v, want [%q]", got, yesUser)
			}
		})
	}
}

func TestGetNotificationRecipients_PlanningEvent_MissingExperienceIDReturnsNil(t *testing.T) {
	s := setupTestStorage(t)
	communityID := insertTestCommunity(t, s, "Community", insertTestUser(t, s, "actor@example.com", "Actor"))
	got := getNotificationRecipients(context.Background(), s, &models.CommunityEvent{
		CommunityId: communityID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_PLANNING_NEED_ADDED,
		// Topic intentionally omitted.
	})
	if got != nil {
		t.Errorf("planning event missing experience_id should return nil; got %v", got)
	}
}

func TestGetNotificationRecipients_RequestFulfilled_FiltersActor(t *testing.T) {
	s := setupTestStorage(t)
	actorID := insertTestUser(t, s, "actor@example.com", "Actor")
	otherOfferer := insertTestUser(t, s, "o@example.com", "O")
	communityID := insertTestCommunity(t, s, "Community", actorID)
	requestID, _ := s.Insert(context.Background(), &models.Request{
		Title:       "Need a ladder",
		RequesterId: actorID,
	})
	// Actor is an offerer; another offerer too. The actor must be excluded.
	if _, err := s.Insert(context.Background(), &models.RequestOffer{
		RequestId: requestID,
		UserId:    actorID,
		Withdrawn: false,
	}); err != nil {
		t.Fatalf("insert offer (actor): %v", err)
	}
	if _, err := s.Insert(context.Background(), &models.RequestOffer{
		RequestId: requestID,
		UserId:    otherOfferer,
		Withdrawn: false,
	}); err != nil {
		t.Fatalf("insert offer (other): %v", err)
	}

	got := getNotificationRecipients(context.Background(), s, &models.CommunityEvent{
		CommunityId: communityID,
		ActorId:     actorID,
		EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_FULFILLED,
		Topic:       &models.CommunityEvent_RequestId{RequestId: requestID},
	})
	if len(got) != 1 || got[0] != otherOfferer {
		t.Errorf("recipients = %v, want only %q (actor excluded)", got, otherOfferer)
	}
}

// TestBuildNotification_ExperienceUpdatedPayloadFlags verifies that
// TimeChanged and LocationChanged are propagated into CommunityEventPayload
// for EXPERIENCE_UPDATED events.
func TestBuildNotification_ExperienceUpdatedPayloadFlags(t *testing.T) {
	s, f := seedBuildNotification(t)
	ctx := context.Background()

	tc := true
	lc := true
	n := buildNotification(ctx, s, &models.CommunityEvent{
		CommunityId:     f.communityID,
		ActorId:         f.actorID,
		EventType:       models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_UPDATED,
		Topic:           &models.CommunityEvent_ExperienceId{ExperienceId: f.experienceID},
		TimeChanged:     &tc,
		LocationChanged: &lc,
	}, enLoc(t))
	if n == nil {
		t.Fatal("buildNotification returned nil")
	}
	payload := n.GetCommunityEvent()
	if payload == nil {
		t.Fatal("expected CommunityEvent payload")
	}
	if !payload.GetTimeChanged() {
		t.Error("payload.TimeChanged should be true")
	}
	if !payload.GetLocationChanged() {
		t.Error("payload.LocationChanged should be true")
	}
	if payload.ExperienceId == nil || *payload.ExperienceId != f.experienceID {
		t.Errorf("payload.ExperienceId = %v, want %q", payload.ExperienceId, f.experienceID)
	}
}
