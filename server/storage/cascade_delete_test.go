package storage

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestCascadeDeleteMedia(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	deletedMetadata := &models.DeletedMetadata{
		DeletedByUserId:  "test-user",
		DeletedAtUnixSec: time.Now().Unix(),
	}

	t.Run("soft-deletes all provided media IDs", func(t *testing.T) {
		// Create media items
		media1 := &models.Media{
			UserId:      "user1",
			ContentType: "image/jpeg",
			Filename:    proto.String("test1.jpg"),
		}
		media2 := &models.Media{
			UserId:      "user1",
			ContentType: "image/png",
			Filename:    proto.String("test2.png"),
		}

		id1, err := storage.Insert(ctx, media1)
		if err != nil {
			t.Fatalf("Failed to insert media1: %v", err)
		}
		id2, err := storage.Insert(ctx, media2)
		if err != nil {
			t.Fatalf("Failed to insert media2: %v", err)
		}

		// Cascade delete media
		err = CascadeDeleteMedia(ctx, storage, []string{id1, id2}, deletedMetadata)
		if err != nil {
			t.Fatalf("CascadeDeleteMedia failed: %v", err)
		}

		// Verify media are soft-deleted
		retrieved1 := &models.Media{}
		err = storage.GetByID(ctx, id1, retrieved1, QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to get media1: %v", err)
		}
		if retrieved1.Deleted == nil || retrieved1.Deleted.DeletedAtUnixSec == 0 {
			t.Error("Expected media1 to be soft-deleted")
		}
		if retrieved1.Deleted.DeletedByUserId != "test-user" {
			t.Errorf("Expected DeletedByUserId 'test-user', got '%s'", retrieved1.Deleted.DeletedByUserId)
		}

		retrieved2 := &models.Media{}
		err = storage.GetByID(ctx, id2, retrieved2, QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to get media2: %v", err)
		}
		if retrieved2.Deleted == nil || retrieved2.Deleted.DeletedAtUnixSec == 0 {
			t.Error("Expected media2 to be soft-deleted")
		}
	})

	t.Run("handles non-existent media gracefully", func(t *testing.T) {
		// Cascade delete with non-existent ID should not error
		err := CascadeDeleteMedia(ctx, storage, []string{"non-existent-id"}, deletedMetadata)
		if err != nil {
			t.Fatalf("CascadeDeleteMedia should not error on non-existent media: %v", err)
		}
	})

	t.Run("skips already-deleted media", func(t *testing.T) {
		// Create and immediately soft-delete media
		media := &models.Media{
			UserId:      "user1",
			ContentType: "image/jpeg",
			Filename:    proto.String("already-deleted.jpg"),
			Deleted: &models.DeletedMetadata{
				DeletedByUserId:  "other-user",
				DeletedAtUnixSec: time.Now().Unix() - 3600, // 1 hour ago
			},
		}

		id, err := storage.Insert(ctx, media)
		if err != nil {
			t.Fatalf("Failed to insert media: %v", err)
		}

		// Cascade delete should skip this media
		err = CascadeDeleteMedia(ctx, storage, []string{id}, deletedMetadata)
		if err != nil {
			t.Fatalf("CascadeDeleteMedia failed: %v", err)
		}

		// Verify the original deletion metadata is preserved
		retrieved := &models.Media{}
		err = storage.GetByID(ctx, id, retrieved, QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to get media: %v", err)
		}
		if retrieved.Deleted.DeletedByUserId != "other-user" {
			t.Errorf("Expected original DeletedByUserId 'other-user', got '%s'", retrieved.Deleted.DeletedByUserId)
		}
	})

	t.Run("handles empty media IDs slice", func(t *testing.T) {
		err := CascadeDeleteMedia(ctx, storage, []string{}, deletedMetadata)
		if err != nil {
			t.Fatalf("CascadeDeleteMedia should not error on empty slice: %v", err)
		}

		err = CascadeDeleteMedia(ctx, storage, nil, deletedMetadata)
		if err != nil {
			t.Fatalf("CascadeDeleteMedia should not error on nil slice: %v", err)
		}
	})
}

func TestCascadeDeleteConversationsByTopic(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	deletedMetadata := &models.DeletedMetadata{
		DeletedByUserId:  "test-user",
		DeletedAtUnixSec: time.Now().Unix(),
	}

	t.Run("soft-deletes conversations by gear_id topic", func(t *testing.T) {
		gearID := "test-gear-123"
		communityID := "test-community-123"

		// Create conversations linked to gear
		conv1 := &models.ChatConversation{
			Topic:       &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
			CommunityId: communityID,
		}
		conv2 := &models.ChatConversation{
			Topic:       &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
			CommunityId: communityID,
		}

		id1, err := storage.Insert(ctx, conv1)
		if err != nil {
			t.Fatalf("Failed to insert conv1: %v", err)
		}
		id2, err := storage.Insert(ctx, conv2)
		if err != nil {
			t.Fatalf("Failed to insert conv2: %v", err)
		}

		// Cascade delete conversations
		err = CascadeDeleteConversationsByTopic(ctx, storage, "topic_gear_id", gearID, deletedMetadata)
		if err != nil {
			t.Fatalf("CascadeDeleteConversationsByTopic failed: %v", err)
		}

		// Verify conversations are soft-deleted
		retrieved1 := &models.ChatConversation{}
		err = storage.GetByID(ctx, id1, retrieved1, QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to get conv1: %v", err)
		}
		if retrieved1.Deleted == nil || retrieved1.Deleted.DeletedAtUnixSec == 0 {
			t.Error("Expected conv1 to be soft-deleted")
		}

		retrieved2 := &models.ChatConversation{}
		err = storage.GetByID(ctx, id2, retrieved2, QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to get conv2: %v", err)
		}
		if retrieved2.Deleted == nil || retrieved2.Deleted.DeletedAtUnixSec == 0 {
			t.Error("Expected conv2 to be soft-deleted")
		}
	})

	t.Run("soft-deletes conversations by experience_id topic", func(t *testing.T) {
		experienceID := "test-experience-456"
		communityID := "test-community-456"

		conv := &models.ChatConversation{
			Topic:       &models.ConversationTopic{TopicId: &models.ConversationTopic_ExperienceId{ExperienceId: experienceID}},
			CommunityId: communityID,
		}

		id, err := storage.Insert(ctx, conv)
		if err != nil {
			t.Fatalf("Failed to insert conversation: %v", err)
		}

		err = CascadeDeleteConversationsByTopic(ctx, storage, "topic_experience_id", experienceID, deletedMetadata)
		if err != nil {
			t.Fatalf("CascadeDeleteConversationsByTopic failed: %v", err)
		}

		retrieved := &models.ChatConversation{}
		err = storage.GetByID(ctx, id, retrieved, QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to get conversation: %v", err)
		}
		if retrieved.Deleted == nil || retrieved.Deleted.DeletedAtUnixSec == 0 {
			t.Error("Expected conversation to be soft-deleted")
		}
	})

	t.Run("skips already-deleted conversations", func(t *testing.T) {
		gearID := "test-gear-skip"
		communityID := "test-community-skip"

		conv := &models.ChatConversation{
			Topic:       &models.ConversationTopic{TopicId: &models.ConversationTopic_GearId{GearId: gearID}},
			CommunityId: communityID,
			Deleted: &models.DeletedMetadata{
				DeletedByUserId:  "other-user",
				DeletedAtUnixSec: time.Now().Unix() - 3600,
			},
		}

		id, err := storage.Insert(ctx, conv)
		if err != nil {
			t.Fatalf("Failed to insert conversation: %v", err)
		}

		err = CascadeDeleteConversationsByTopic(ctx, storage, "topic_gear_id", gearID, deletedMetadata)
		if err != nil {
			t.Fatalf("CascadeDeleteConversationsByTopic failed: %v", err)
		}

		// Verify original deletion metadata is preserved
		retrieved := &models.ChatConversation{}
		err = storage.GetByID(ctx, id, retrieved, QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to get conversation: %v", err)
		}
		if retrieved.Deleted.DeletedByUserId != "other-user" {
			t.Errorf("Expected original DeletedByUserId 'other-user', got '%s'", retrieved.Deleted.DeletedByUserId)
		}
	})

	t.Run("handles no matching conversations gracefully", func(t *testing.T) {
		err := CascadeDeleteConversationsByTopic(ctx, storage, "topic_gear_id", "non-existent-gear", deletedMetadata)
		if err != nil {
			t.Fatalf("CascadeDeleteConversationsByTopic should not error when no conversations found: %v", err)
		}
	})
}

func TestCascadeDeleteConversationByID(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	deletedMetadata := &models.DeletedMetadata{
		DeletedByUserId:  "test-user",
		DeletedAtUnixSec: time.Now().Unix(),
	}

	t.Run("soft-deletes conversation by ID", func(t *testing.T) {
		conv := &models.ChatConversation{
			Topic:       &models.ConversationTopic{TopicId: &models.ConversationTopic_RequestId{RequestId: "test-request-123"}},
			CommunityId: "test-community-789",
		}

		id, err := storage.Insert(ctx, conv)
		if err != nil {
			t.Fatalf("Failed to insert conversation: %v", err)
		}

		err = CascadeDeleteConversationByID(ctx, storage, id, deletedMetadata)
		if err != nil {
			t.Fatalf("CascadeDeleteConversationByID failed: %v", err)
		}

		retrieved := &models.ChatConversation{}
		err = storage.GetByID(ctx, id, retrieved, QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to get conversation: %v", err)
		}
		if retrieved.Deleted == nil || retrieved.Deleted.DeletedAtUnixSec == 0 {
			t.Error("Expected conversation to be soft-deleted")
		}
		if retrieved.Deleted.DeletedByUserId != "test-user" {
			t.Errorf("Expected DeletedByUserId 'test-user', got '%s'", retrieved.Deleted.DeletedByUserId)
		}
	})

	t.Run("handles non-existent conversation gracefully", func(t *testing.T) {
		err := CascadeDeleteConversationByID(ctx, storage, "non-existent-id", deletedMetadata)
		if err != nil {
			t.Fatalf("CascadeDeleteConversationByID should not error on non-existent ID: %v", err)
		}
	})

	t.Run("handles empty conversation ID", func(t *testing.T) {
		err := CascadeDeleteConversationByID(ctx, storage, "", deletedMetadata)
		if err != nil {
			t.Fatalf("CascadeDeleteConversationByID should not error on empty ID: %v", err)
		}
	})

	t.Run("skips already-deleted conversation", func(t *testing.T) {
		conv := &models.ChatConversation{
			Topic:       &models.ConversationTopic{TopicId: &models.ConversationTopic_RequestId{RequestId: "test-request-456"}},
			CommunityId: "test-community-456",
			Deleted: &models.DeletedMetadata{
				DeletedByUserId:  "other-user",
				DeletedAtUnixSec: time.Now().Unix() - 3600,
			},
		}

		id, err := storage.Insert(ctx, conv)
		if err != nil {
			t.Fatalf("Failed to insert conversation: %v", err)
		}

		err = CascadeDeleteConversationByID(ctx, storage, id, deletedMetadata)
		if err != nil {
			t.Fatalf("CascadeDeleteConversationByID failed: %v", err)
		}

		// Verify original deletion metadata is preserved
		retrieved := &models.ChatConversation{}
		err = storage.GetByID(ctx, id, retrieved, QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to get conversation: %v", err)
		}
		if retrieved.Deleted.DeletedByUserId != "other-user" {
			t.Errorf("Expected original DeletedByUserId 'other-user', got '%s'", retrieved.Deleted.DeletedByUserId)
		}
	})
}

func TestCascadeDeleteCommunityGearByGearID(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	deletedMetadata := &models.DeletedMetadata{
		DeletedByUserId:  "test-user",
		DeletedAtUnixSec: time.Now().Unix(),
	}

	t.Run("soft-deletes live and archived rows for the gear", func(t *testing.T) {
		gearID := "gear-cascade-1"
		live := &models.CommunityGear{GearId: gearID, CommunityId: "comm-a"}
		archived := &models.CommunityGear{GearId: gearID, CommunityId: "comm-b", Archived: true}
		other := &models.CommunityGear{GearId: "gear-cascade-2", CommunityId: "comm-c"}

		liveID, err := store.Insert(ctx, live)
		if err != nil {
			t.Fatalf("insert live: %v", err)
		}
		archivedID, err := store.Insert(ctx, archived)
		if err != nil {
			t.Fatalf("insert archived: %v", err)
		}
		otherID, err := store.Insert(ctx, other)
		if err != nil {
			t.Fatalf("insert other: %v", err)
		}

		if err := CascadeDeleteCommunityGearByGearID(ctx, store, gearID, deletedMetadata); err != nil {
			t.Fatalf("cascade: %v", err)
		}

		got := &models.CommunityGear{}
		if err := store.GetByID(ctx, liveID, got, QueryOptions{IncludeDeleted: true}); err != nil {
			t.Fatalf("get live: %v", err)
		}
		if got.Deleted == nil || got.Deleted.DeletedAtUnixSec == 0 {
			t.Error("expected live row to be soft-deleted")
		}

		got = &models.CommunityGear{}
		if err := store.GetByID(ctx, archivedID, got, QueryOptions{IncludeDeleted: true}); err != nil {
			t.Fatalf("get archived: %v", err)
		}
		if got.Deleted == nil {
			t.Error("expected archived row to also be soft-deleted")
		}
		if !got.Archived {
			t.Error("expected archived=true to be preserved alongside deleted")
		}

		got = &models.CommunityGear{}
		if err := store.GetByID(ctx, otherID, got); err != nil {
			t.Fatalf("get other: %v", err)
		}
		if got.Deleted != nil {
			t.Error("unrelated gear's row should not be touched")
		}
	})

	t.Run("noop for gear with no join rows", func(t *testing.T) {
		if err := CascadeDeleteCommunityGearByGearID(ctx, store, "no-such-gear", deletedMetadata); err != nil {
			t.Fatalf("cascade on empty set should be a noop: %v", err)
		}
	})

	t.Run("skips already-deleted rows idempotently", func(t *testing.T) {
		gearID := "gear-cascade-3"
		prior := &models.DeletedMetadata{DeletedByUserId: "earlier-user", DeletedAtUnixSec: time.Now().Unix() - 3600}
		row := &models.CommunityGear{GearId: gearID, CommunityId: "comm-d", Deleted: prior}
		id, err := store.Insert(ctx, row)
		if err != nil {
			t.Fatalf("insert: %v", err)
		}

		if err := CascadeDeleteCommunityGearByGearID(ctx, store, gearID, deletedMetadata); err != nil {
			t.Fatalf("cascade: %v", err)
		}

		got := &models.CommunityGear{}
		if err := store.GetByID(ctx, id, got, QueryOptions{IncludeDeleted: true}); err != nil {
			t.Fatalf("get: %v", err)
		}
		if got.Deleted.DeletedByUserId != "earlier-user" {
			t.Errorf("expected prior deleted metadata to be preserved, got %q", got.Deleted.DeletedByUserId)
		}
	})
}

func TestCascadeDeleteCommunityRequestByRequestID(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	deletedMetadata := &models.DeletedMetadata{
		DeletedByUserId:  "test-user",
		DeletedAtUnixSec: time.Now().Unix(),
	}

	requestID := "req-cascade-1"
	row := &models.CommunityRequest{RequestId: requestID, CommunityId: "comm-a"}
	other := &models.CommunityRequest{RequestId: "req-cascade-2", CommunityId: "comm-b"}

	rowID, err := store.Insert(ctx, row)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	otherID, err := store.Insert(ctx, other)
	if err != nil {
		t.Fatalf("insert other: %v", err)
	}

	if err := CascadeDeleteCommunityRequestByRequestID(ctx, store, requestID, deletedMetadata); err != nil {
		t.Fatalf("cascade: %v", err)
	}

	got := &models.CommunityRequest{}
	if err := store.GetByID(ctx, rowID, got, QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("get row: %v", err)
	}
	if got.Deleted == nil || got.Deleted.DeletedAtUnixSec == 0 {
		t.Error("expected row to be soft-deleted")
	}

	got = &models.CommunityRequest{}
	if err := store.GetByID(ctx, otherID, got); err != nil {
		t.Fatalf("get other: %v", err)
	}
	if got.Deleted != nil {
		t.Error("unrelated request's row should not be touched")
	}
}

func TestCascadeDeleteCommunityExperienceByExperienceID(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	deletedMetadata := &models.DeletedMetadata{
		DeletedByUserId:  "test-user",
		DeletedAtUnixSec: time.Now().Unix(),
	}

	experienceID := "exp-cascade-1"
	row := &models.CommunityExperience{ExperienceId: experienceID, CommunityId: "comm-a"}
	other := &models.CommunityExperience{ExperienceId: "exp-cascade-2", CommunityId: "comm-b"}

	rowID, err := store.Insert(ctx, row)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	otherID, err := store.Insert(ctx, other)
	if err != nil {
		t.Fatalf("insert other: %v", err)
	}

	if err := CascadeDeleteCommunityExperienceByExperienceID(ctx, store, experienceID, deletedMetadata); err != nil {
		t.Fatalf("cascade: %v", err)
	}

	got := &models.CommunityExperience{}
	if err := store.GetByID(ctx, rowID, got, QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("get row: %v", err)
	}
	if got.Deleted == nil || got.Deleted.DeletedAtUnixSec == 0 {
		t.Error("expected row to be soft-deleted")
	}

	got = &models.CommunityExperience{}
	if err := store.GetByID(ctx, otherID, got); err != nil {
		t.Fatalf("get other: %v", err)
	}
	if got.Deleted != nil {
		t.Error("unrelated experience's row should not be touched")
	}
}

func TestCascadeDeleteExperienceRSVPsByExperienceID(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	deletedMetadata := &models.DeletedMetadata{
		DeletedByUserId:  "owner",
		DeletedAtUnixSec: time.Now().Unix(),
	}

	const expID = "exp-rsvp-cascade"
	target := &models.ExperienceRSVP{ExperienceId: expID, UserId: "user-a", CommunityId: "comm-1", Intention: models.RSVPIntention_RSVP_INTENTION_YES}
	other := &models.ExperienceRSVP{ExperienceId: "exp-other", UserId: "user-b", CommunityId: "comm-1", Intention: models.RSVPIntention_RSVP_INTENTION_YES}
	preDeleted := &models.ExperienceRSVP{
		ExperienceId: expID, UserId: "user-c", CommunityId: "comm-1", Intention: models.RSVPIntention_RSVP_INTENTION_MAYBE,
		Deleted: &models.DeletedMetadata{DeletedByUserId: "user-c", DeletedAtUnixSec: 1},
	}

	targetID, err := store.Insert(ctx, target)
	if err != nil {
		t.Fatalf("insert target: %v", err)
	}
	otherID, err := store.Insert(ctx, other)
	if err != nil {
		t.Fatalf("insert other: %v", err)
	}
	preDeletedID, err := store.Insert(ctx, preDeleted)
	if err != nil {
		t.Fatalf("insert preDeleted: %v", err)
	}

	if err := CascadeDeleteExperienceRSVPsByExperienceID(ctx, store, expID, deletedMetadata); err != nil {
		t.Fatalf("cascade: %v", err)
	}

	// Idempotency: running twice is safe.
	if err := CascadeDeleteExperienceRSVPsByExperienceID(ctx, store, expID, deletedMetadata); err != nil {
		t.Fatalf("cascade rerun: %v", err)
	}

	got := &models.ExperienceRSVP{}
	if err := store.GetByID(ctx, targetID, got, QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("get target: %v", err)
	}
	if got.Deleted == nil || got.Deleted.DeletedAtUnixSec != deletedMetadata.DeletedAtUnixSec {
		t.Errorf("expected target RSVP soft-deleted with cascade metadata, got %+v", got.Deleted)
	}

	got = &models.ExperienceRSVP{}
	if err := store.GetByID(ctx, otherID, got); err != nil {
		t.Fatalf("get other: %v", err)
	}
	if got.Deleted != nil {
		t.Error("unrelated RSVP should not be touched")
	}

	// Pre-deleted row must keep its original DeletedMetadata.
	got = &models.ExperienceRSVP{}
	if err := store.GetByID(ctx, preDeletedID, got, QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("get preDeleted: %v", err)
	}
	if got.Deleted == nil || got.Deleted.DeletedAtUnixSec != 1 {
		t.Errorf("pre-deleted RSVP should retain original metadata, got %+v", got.Deleted)
	}
}

func TestCascadeDeleteExperienceTimeProposalsByExperienceID(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	deletedMetadata := &models.DeletedMetadata{
		DeletedByUserId:  "owner",
		DeletedAtUnixSec: time.Now().Unix(),
	}

	const expID = "exp-prop-cascade"
	target := &models.ExperienceTimeProposal{ExperienceId: expID, ProposedByUserId: "user-a"}
	other := &models.ExperienceTimeProposal{ExperienceId: "exp-other", ProposedByUserId: "user-b"}

	targetID, err := store.Insert(ctx, target)
	if err != nil {
		t.Fatalf("insert target: %v", err)
	}
	otherID, err := store.Insert(ctx, other)
	if err != nil {
		t.Fatalf("insert other: %v", err)
	}

	if err := CascadeDeleteExperienceTimeProposalsByExperienceID(ctx, store, expID, deletedMetadata); err != nil {
		t.Fatalf("cascade: %v", err)
	}

	got := &models.ExperienceTimeProposal{}
	if err := store.GetByID(ctx, targetID, got, QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("get target: %v", err)
	}
	if got.Deleted == nil {
		t.Error("expected target proposal soft-deleted")
	}

	got = &models.ExperienceTimeProposal{}
	if err := store.GetByID(ctx, otherID, got); err != nil {
		t.Fatalf("get other: %v", err)
	}
	if got.Deleted != nil {
		t.Error("unrelated proposal should not be touched")
	}
}

func TestCascadeDeletePlanningNeedsByExperienceID(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	deletedMetadata := &models.DeletedMetadata{
		DeletedByUserId:  "owner",
		DeletedAtUnixSec: time.Now().Unix(),
	}

	const expID = "exp-need-cascade"
	target := &models.PlanningNeed{
		ProposerId: "user-a", Name: "snacks", Slots: 1, SlotsRemaining: 1,
		Scope: &models.PlanningNeed_ExperienceId{ExperienceId: expID},
	}
	otherExp := &models.PlanningNeed{
		ProposerId: "user-b", Name: "drinks", Slots: 1, SlotsRemaining: 1,
		Scope: &models.PlanningNeed_ExperienceId{ExperienceId: "exp-other"},
	}
	requestScoped := &models.PlanningNeed{
		ProposerId: "user-c", Name: "tools", Slots: 1, SlotsRemaining: 1,
		Scope: &models.PlanningNeed_RequestId{RequestId: "req-1"},
	}

	targetID, err := store.Insert(ctx, target)
	if err != nil {
		t.Fatalf("insert target: %v", err)
	}
	otherID, err := store.Insert(ctx, otherExp)
	if err != nil {
		t.Fatalf("insert otherExp: %v", err)
	}
	reqID, err := store.Insert(ctx, requestScoped)
	if err != nil {
		t.Fatalf("insert requestScoped: %v", err)
	}

	if err := CascadeDeletePlanningNeedsByExperienceID(ctx, store, expID, deletedMetadata); err != nil {
		t.Fatalf("cascade: %v", err)
	}

	got := &models.PlanningNeed{}
	if err := store.GetByID(ctx, targetID, got, QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("get target: %v", err)
	}
	if got.Deleted == nil {
		t.Error("expected target need soft-deleted")
	}

	got = &models.PlanningNeed{}
	if err := store.GetByID(ctx, otherID, got); err != nil {
		t.Fatalf("get otherExp: %v", err)
	}
	if got.Deleted != nil {
		t.Error("need scoped to a different experience should not be touched")
	}

	got = &models.PlanningNeed{}
	if err := store.GetByID(ctx, reqID, got); err != nil {
		t.Fatalf("get requestScoped: %v", err)
	}
	if got.Deleted != nil {
		t.Error("need scoped to a request should not be touched")
	}
}

func TestCascadeDeletePlanningContributionsByExperienceID(t *testing.T) {
	store, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	deletedMetadata := &models.DeletedMetadata{
		DeletedByUserId:  "owner",
		DeletedAtUnixSec: time.Now().Unix(),
	}

	const expID = "exp-contrib-cascade"
	target := &models.PlanningContribution{
		ContributorId: "user-a", Title: "bring chairs",
		Scope: &models.PlanningContribution_ExperienceId{ExperienceId: expID},
	}
	requestScoped := &models.PlanningContribution{
		ContributorId: "user-b", Title: "lend drill",
		Scope: &models.PlanningContribution_RequestId{RequestId: "req-1"},
	}

	targetID, err := store.Insert(ctx, target)
	if err != nil {
		t.Fatalf("insert target: %v", err)
	}
	reqID, err := store.Insert(ctx, requestScoped)
	if err != nil {
		t.Fatalf("insert requestScoped: %v", err)
	}

	if err := CascadeDeletePlanningContributionsByExperienceID(ctx, store, expID, deletedMetadata); err != nil {
		t.Fatalf("cascade: %v", err)
	}

	got := &models.PlanningContribution{}
	if err := store.GetByID(ctx, targetID, got, QueryOptions{IncludeDeleted: true}); err != nil {
		t.Fatalf("get target: %v", err)
	}
	if got.Deleted == nil {
		t.Error("expected target contribution soft-deleted")
	}

	got = &models.PlanningContribution{}
	if err := store.GetByID(ctx, reqID, got); err != nil {
		t.Fatalf("get requestScoped: %v", err)
	}
	if got.Deleted != nil {
		t.Error("contribution scoped to a request should not be touched")
	}
}
