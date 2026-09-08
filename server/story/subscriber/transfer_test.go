package story_subscriber

import (
	"context"
	"testing"

	cebus "go.ripls.org/ripls/server/community_event_bus"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// activeCommunity inserts a non-deleted community so the IsActive gate
// in Subscriber.Handle passes. Returns the community ID. Inserts a
// throwaway owner first to satisfy the community_owner_required CHECK
// constraint.
func activeCommunity(t *testing.T, s *storage.ProtoSQLStorage) string {
	t.Helper()
	ownerID, err := s.Insert(context.Background(), &models.User{Name: "Community Owner", Email: "co@example.com"})
	if err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	id, err := s.Insert(context.Background(), &models.Community{
		Name:        "Test Community",
		CreatorId:   ownerID,
		OwnerUserId: ownerID,
	})
	if err != nil {
		t.Fatalf("insert community: %v", err)
	}
	return id
}

func TestHandleTransferCompleted_Loan(t *testing.T) {
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	communityID := activeCommunity(t, s)
	ownerID, err := s.Insert(context.Background(), &models.User{Name: "Olivia Owner", Email: "olivia@example.com"})
	if err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	borrowerID, err := s.Insert(context.Background(), &models.User{Name: "Bob Borrower", Email: "bob@example.com"})
	if err != nil {
		t.Fatalf("insert borrower: %v", err)
	}

	creator := &fakeCreator{}
	sub := New(s, creator)

	transfer := &models.Transfer{
		OwnerId:      ownerID,
		RecipientId:  borrowerID,
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		CommunityId:  communityID,
	}
	transfer.Id = "transfer-1"
	gear := &models.Gear{Id: "gear-1", Name: "Cordless Drill", MediaIds: []string{"media-1"}, OwnerId: ownerID}

	evt := &cebus.PublishedEvent{
		Event: &models.CommunityEvent{
			Id:          "evt-1",
			CommunityId: communityID,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED,
		},
		Transfer: transfer,
		Gear:     gear,
	}

	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	got := creator.Captured()
	if len(got) != 1 {
		t.Fatalf("CreateStory called %d times, want 1", len(got))
	}
	req := got[0]
	if req.StoryType != "STORY_TYPE_LOAN_COMPLETED" {
		t.Errorf("StoryType = %q, want STORY_TYPE_LOAN_COMPLETED", req.StoryType)
	}
	// For loans the borrower comes first.
	if len(req.ParticipantIDs) != 2 {
		t.Fatalf("ParticipantIDs = %v, want 2 entries", req.ParticipantIDs)
	}
	if req.ParticipantIDs[0] != borrowerID || req.ParticipantIDs[1] != ownerID {
		t.Errorf("ParticipantIDs = %v, want [borrower, owner]", req.ParticipantIDs)
	}
	if req.ParticipantNames[0] != "Bob Borrower" || req.ParticipantNames[1] != "Olivia Owner" {
		t.Errorf("ParticipantNames = %v, want [Bob, Olivia]", req.ParticipantNames)
	}
	if req.RelatedEntityID != "transfer-1" || req.RelatedEntityName != "Cordless Drill" {
		t.Errorf("Related entity = %q/%q, want transfer-1/Cordless Drill", req.RelatedEntityID, req.RelatedEntityName)
	}
	if req.CommunityEventID != "evt-1" {
		t.Errorf("CommunityEventID = %q, want evt-1", req.CommunityEventID)
	}
	if len(req.MediaIDs) != 1 || req.MediaIDs[0] != "media-1" {
		t.Errorf("MediaIDs = %v, want [media-1]", req.MediaIDs)
	}
}

func TestHandleTransferCompleted_Giveaway(t *testing.T) {
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	communityID := activeCommunity(t, s)
	ownerID, _ := s.Insert(context.Background(), &models.User{Name: "Giver", Email: "g@example.com"})
	receiverID, _ := s.Insert(context.Background(), &models.User{Name: "Receiver", Email: "r@example.com"})

	creator := &fakeCreator{}
	sub := New(s, creator)

	evt := &cebus.PublishedEvent{
		Event: &models.CommunityEvent{
			Id:          "evt-2",
			CommunityId: communityID,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED,
		},
		Transfer: &models.Transfer{
			Id:           "xfer-2",
			OwnerId:      ownerID,
			RecipientId:  receiverID,
			TransferType: models.TransferType_TRANSFER_TYPE_GIVEAWAY,
			CommunityId:  communityID,
		},
		Gear: &models.Gear{Id: "gear-2", Name: "Old Bike", OwnerId: ownerID},
	}

	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	got := creator.Captured()
	if len(got) != 1 || got[0].StoryType != "STORY_TYPE_GIVEAWAY_COMPLETED" {
		t.Fatalf("expected one giveaway story, got %+v", got)
	}
	// Giveaways: giver first, then receiver.
	if got[0].ParticipantIDs[0] != ownerID || got[0].ParticipantIDs[1] != receiverID {
		t.Errorf("Giveaway participant order = %v, want [giver, receiver]", got[0].ParticipantIDs)
	}
}

func TestHandleTransferCompleted_ProvisionalRecipient(t *testing.T) {
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	communityID := activeCommunity(t, s)
	ownerID, _ := s.Insert(context.Background(), &models.User{Name: "Owner", Email: "o@example.com"})
	provID, err := s.Insert(context.Background(), &models.ProvisionalUser{Name: "Provisional Friend"})
	if err != nil {
		t.Fatalf("insert provisional: %v", err)
	}

	creator := &fakeCreator{}
	sub := New(s, creator)

	provisionalRef := provID
	evt := &cebus.PublishedEvent{
		Event: &models.CommunityEvent{
			Id:          "evt-prov",
			CommunityId: communityID,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED,
		},
		Transfer: &models.Transfer{
			Id:                     "xfer-prov",
			OwnerId:                ownerID,
			ProvisionalRecipientId: &provisionalRef,
			TransferType:           models.TransferType_TRANSFER_TYPE_GIVEAWAY,
			CommunityId:            communityID,
		},
		Gear: &models.Gear{Id: "gear-prov", Name: "Old Lamp", OwnerId: ownerID},
	}

	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	got := creator.Captured()
	if len(got) != 1 {
		t.Fatalf("CreateStory not called")
	}
	// Provisional recipient has no registered ID, so the participant list
	// retains only the owner; the name still shows up in
	// ParticipantNames so the story copy can render.
	if len(got[0].ParticipantIDs) != 1 || got[0].ParticipantIDs[0] != ownerID {
		t.Errorf("ParticipantIDs = %v, want only the owner", got[0].ParticipantIDs)
	}
}

func TestHandleTransferCompleted_SoftDeletedCommunityIsSkipped(t *testing.T) {
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	// Insert a community then mark it soft-deleted.
	ownerID, _ := s.Insert(context.Background(), &models.User{Name: "Owner", Email: "o@example.com"})
	communityID, _ := s.Insert(context.Background(), &models.Community{
		Name:        "Deleted",
		CreatorId:   ownerID,
		OwnerUserId: ownerID,
	})
	c := &models.Community{}
	if err := s.GetByID(context.Background(), communityID, c); err != nil {
		t.Fatalf("get community: %v", err)
	}
	c.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: 1700000000}
	if err := s.Update(context.Background(), c); err != nil {
		t.Fatalf("force-delete community: %v", err)
	}

	creator := &fakeCreator{}
	sub := New(s, creator)
	evt := &cebus.PublishedEvent{
		Event: &models.CommunityEvent{
			Id:          "evt-x",
			CommunityId: communityID,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED,
		},
		Transfer: &models.Transfer{Id: "xfer", CommunityId: communityID},
		Gear:     &models.Gear{Id: "gear"},
	}
	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if calls := creator.Captured(); len(calls) != 0 {
		t.Errorf("expected no CreateStory call for soft-deleted community, got %d", len(calls))
	}
}

func TestHandleTransferCompleted_MissingOwnerLogsAndAborts(t *testing.T) {
	s, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	communityID := activeCommunity(t, s)
	creator := &fakeCreator{}
	sub := New(s, creator)

	evt := &cebus.PublishedEvent{
		Event: &models.CommunityEvent{
			Id:          "evt-no-owner",
			CommunityId: communityID,
			EventType:   models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED,
		},
		Transfer: &models.Transfer{
			Id:           "xfer",
			OwnerId:      "nonexistent-owner",
			RecipientId:  "nonexistent-recipient",
			TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			CommunityId:  communityID,
		},
		Gear: &models.Gear{Id: "gear"},
	}
	if err := sub.Handle(context.Background(), evt); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if calls := creator.Captured(); len(calls) != 0 {
		t.Errorf("expected no CreateStory call when owner missing, got %d", len(calls))
	}
}
