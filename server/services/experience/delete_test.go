package experience

import (
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestService_DeleteExperience(t *testing.T) {
	service, testStorage, _ := setupTestService(t)

	t.Run("successful deletion with authentication", func(t *testing.T) {
		createTestUser(t, testStorage, "user-del-1", "del1@example.com", "Del User 1")
		ctx := createAuthenticatedContext("user-del-1", "del1@example.com", models.Role_ROLE_USER)

		// Create experience
		createReq := connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "Test Experience",
			Description: "A test experience",
		})

		createResp, err := service.SaveExperience(ctx, createReq)
		if err != nil {
			t.Fatalf("Failed to create experience: %v", err)
		}

		expID := createResp.Msg.Experience.Id

		// Delete experience
		deleteReq := connect.NewRequest(&api.DeleteExperienceRequest{
			ExperienceId: expID,
		})

		_, err = service.DeleteExperience(ctx, deleteReq)
		if err != nil {
			t.Fatalf("DeleteExperience failed: %v", err)
		}

		// Verify experience is deleted
		stored := &models.Experience{}
		err = testStorage.GetByID(ctx, expID, stored)
		if err == nil {
			t.Fatal("Expected error when getting deleted experience")
		}
	})

	t.Run("delete experience by non-owner fails", func(t *testing.T) {
		createTestUser(t, testStorage, "user-del-2", "del2@example.com", "Del User 2")
		createTestUser(t, testStorage, "user-del-3", "del3@example.com", "Del User 3")
		ctx123 := createAuthenticatedContext("user-del-2", "del2@example.com", models.Role_ROLE_USER)

		// Create experience as user-del-2
		createReq := connect.NewRequest(&api.SaveExperienceRequest{
			Name: "User Del 2's Experience",
		})

		createResp, err := service.SaveExperience(ctx123, createReq)
		if err != nil {
			t.Fatalf("Failed to create experience: %v", err)
		}

		// Try to delete as user-del-3
		ctx456 := createAuthenticatedContext("user-del-3", "del3@example.com", models.Role_ROLE_USER)
		deleteReq := connect.NewRequest(&api.DeleteExperienceRequest{
			ExperienceId: createResp.Msg.Experience.Id,
		})

		_, err = service.DeleteExperience(ctx456, deleteReq)
		if err == nil {
			t.Fatal("Expected error when non-owner tries to delete experience")
		}

		connectErr, ok := err.(*connect.Error)
		if !ok {
			t.Fatalf("Expected connect.Error, got %T", err)
		}

		if connectErr.Code() != connect.CodePermissionDenied {
			t.Errorf("Expected PermissionDenied error, got %v", connectErr.Code())
		}
	})

	t.Run("soft deletion sets deleted metadata", func(t *testing.T) {
		createTestUser(t, testStorage, "user-del-4", "del4@example.com", "Del User 4")
		ctx := createAuthenticatedContext("user-del-4", "del4@example.com", models.Role_ROLE_USER)

		// Create experience
		createReq := connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "Soft Delete Test Experience",
			Description: "Testing soft deletion",
		})
		createResp, err := service.SaveExperience(ctx, createReq)
		if err != nil {
			t.Fatalf("Failed to create experience: %v", err)
		}
		expID := createResp.Msg.Experience.Id

		// Delete experience
		deleteReq := connect.NewRequest(&api.DeleteExperienceRequest{ExperienceId: expID})
		_, err = service.DeleteExperience(ctx, deleteReq)
		if err != nil {
			t.Fatalf("DeleteExperience failed: %v", err)
		}

		// Verify experience is not accessible via normal GetByID
		exp := &models.Experience{}
		err = testStorage.GetByID(ctx, expID, exp)
		if err == nil {
			t.Error("Expected error when getting deleted experience via normal GetByID")
		}

		// Verify experience still exists with IncludeDeleted option and has deletion metadata
		err = testStorage.GetByID(ctx, expID, exp, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to get deleted experience with IncludeDeleted: %v", err)
		}

		if exp.Deleted == nil {
			t.Fatal("Expected experience to have deleted metadata")
		}
		if exp.Deleted.DeletedByUserId != "user-del-4" {
			t.Errorf("Expected deleted_by_user_id 'user-del-4', got '%s'", exp.Deleted.DeletedByUserId)
		}
		if exp.Deleted.DeletedAtUnixSec == 0 {
			t.Error("Expected deleted_at_unix_sec to be set")
		}
	})

	t.Run("deletion cascades to conversation", func(t *testing.T) {
		createTestUser(t, testStorage, "user-del-5", "del5@example.com", "Del User 5")
		ctx := createAuthenticatedContext("user-del-5", "del5@example.com", models.Role_ROLE_USER)

		// Create experience
		createReq := connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "Experience with Conversation",
			Description: "Testing cascade deletion",
		})
		createResp, err := service.SaveExperience(ctx, createReq)
		if err != nil {
			t.Fatalf("Failed to create experience: %v", err)
		}
		expID := createResp.Msg.Experience.Id

		// Create a conversation linked to this experience via experience_id topic
		// NOTE: We intentionally do NOT set Experience.ConversationId - the cascade
		// deletion should find and delete conversations based on the topic field alone.
		// This matches production behavior where conversations are linked via Topic.
		conversation := &models.ChatConversation{
			Topic: &models.ConversationTopic{TopicId: &models.ConversationTopic_ExperienceId{ExperienceId: expID}},
		}
		convID, err := testStorage.Insert(ctx, conversation)
		if err != nil {
			t.Fatalf("Failed to create conversation: %v", err)
		}

		// Verify conversation exists and is not deleted initially
		storedConv := &models.ChatConversation{}
		err = testStorage.GetByID(ctx, convID, storedConv)
		if err != nil {
			t.Fatalf("Failed to get conversation: %v", err)
		}
		if storedConv.Deleted != nil {
			t.Fatalf("Expected conversation to not be deleted initially")
		}

		// Delete the experience
		deleteReq := connect.NewRequest(&api.DeleteExperienceRequest{ExperienceId: expID})
		_, err = service.DeleteExperience(ctx, deleteReq)
		if err != nil {
			t.Fatalf("DeleteExperience failed: %v", err)
		}

		// Verify conversation is not accessible via normal GetByID
		conv := &models.ChatConversation{}
		err = testStorage.GetByID(ctx, convID, conv)
		if err == nil {
			t.Error("Expected error when getting cascade-deleted conversation via normal GetByID")
		}

		// Verify conversation still exists with IncludeDeleted option and has deletion metadata
		err = testStorage.GetByID(ctx, convID, conv, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to get deleted conversation with IncludeDeleted: %v", err)
		}

		if conv.Deleted == nil {
			t.Fatal("Expected conversation to have deleted metadata")
		}
		if conv.Deleted.DeletedByUserId != "user-del-5" {
			t.Errorf("Expected deleted_by_user_id 'user-del-5', got '%s'", conv.Deleted.DeletedByUserId)
		}
		if conv.Deleted.DeletedAtUnixSec == 0 {
			t.Error("Expected deleted_at_unix_sec to be set")
		}
	})

	t.Run("deletion cascades to media", func(t *testing.T) {
		userID := "user-del-6"
		createTestUser(t, testStorage, userID, "del6@example.com", "Del User 6")
		ctx := createAuthenticatedContext(userID, "del6@example.com", models.Role_ROLE_USER)

		// Create media first
		media1 := &models.Media{
			UserId:      userID,
			ContentType: "image/jpeg",
			Filename:    proto.String("experience-image-1.jpg"),
		}
		media2 := &models.Media{
			UserId:      userID,
			ContentType: "image/png",
			Filename:    proto.String("experience-image-2.png"),
		}
		mediaID1, err := testStorage.Insert(ctx, media1)
		if err != nil {
			t.Fatalf("Failed to create media1: %v", err)
		}
		mediaID2, err := testStorage.Insert(ctx, media2)
		if err != nil {
			t.Fatalf("Failed to create media2: %v", err)
		}

		// Create experience with media via insert (not SaveExperience which may alter media)
		exp := &models.Experience{
			OwnerId:     userID,
			Name:        "Experience with Media",
			Description: "Testing media cascade deletion",
			MediaIds:    []string{mediaID1, mediaID2},
		}
		expID, err := testStorage.Insert(ctx, exp)
		if err != nil {
			t.Fatalf("Failed to insert experience: %v", err)
		}

		// Verify media exists before deletion
		media := &models.Media{}
		err = testStorage.GetByID(ctx, mediaID1, media)
		if err != nil {
			t.Fatalf("Failed to get media1 before deletion: %v", err)
		}

		// Delete the experience
		deleteReq := connect.NewRequest(&api.DeleteExperienceRequest{ExperienceId: expID})
		_, err = service.DeleteExperience(ctx, deleteReq)
		if err != nil {
			t.Fatalf("DeleteExperience failed: %v", err)
		}

		// Verify media is no longer retrievable via normal GetByID
		err = testStorage.GetByID(ctx, mediaID1, media)
		if err == nil {
			t.Error("Expected error when getting deleted media1, but got none")
		}
		err = testStorage.GetByID(ctx, mediaID2, media)
		if err == nil {
			t.Error("Expected error when getting deleted media2, but got none")
		}

		// Verify media still exists with IncludeDeleted option and has deletion metadata
		err = testStorage.GetByID(ctx, mediaID1, media, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to retrieve deleted media1 with IncludeDeleted: %v", err)
		}
		if media.Deleted == nil {
			t.Error("Expected media1 to have deleted metadata")
		}
		if media.Deleted.DeletedByUserId != userID {
			t.Errorf("Expected media1 deleted_by_user_id %s, got %s", userID, media.Deleted.DeletedByUserId)
		}

		err = testStorage.GetByID(ctx, mediaID2, media, storage.QueryOptions{IncludeDeleted: true})
		if err != nil {
			t.Fatalf("Failed to retrieve deleted media2 with IncludeDeleted: %v", err)
		}
		if media.Deleted == nil {
			t.Error("Expected media2 to have deleted metadata")
		}
	})

	t.Run("deletion cascades to community_experience", func(t *testing.T) {
		userID := "user-del-ce"
		createTestUser(t, testStorage, userID, "delce@example.com", "Del User CE")
		ctx := createAuthenticatedContext(userID, "delce@example.com", models.Role_ROLE_USER)

		createResp, err := service.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "Cascade CE Experience",
			Description: "experience with cascade coverage",
		}))
		if err != nil {
			t.Fatalf("SaveExperience: %v", err)
		}
		expID := createResp.Msg.Experience.Id

		liveCE := &models.CommunityExperience{
			ExperienceId: expID,
			CommunityId:  "community-a",
		}
		archivedCE := &models.CommunityExperience{
			ExperienceId: expID,
			CommunityId:  "community-b",
			Archived:     true,
		}
		liveCEID, err := testStorage.Insert(ctx, liveCE)
		if err != nil {
			t.Fatalf("insert liveCE: %v", err)
		}
		archivedCEID, err := testStorage.Insert(ctx, archivedCE)
		if err != nil {
			t.Fatalf("insert archivedCE: %v", err)
		}

		if _, err := service.DeleteExperience(ctx, connect.NewRequest(&api.DeleteExperienceRequest{ExperienceId: expID})); err != nil {
			t.Fatalf("DeleteExperience: %v", err)
		}

		live := &models.CommunityExperience{}
		if err := testStorage.GetByID(ctx, liveCEID, live, storage.QueryOptions{IncludeDeleted: true}); err != nil {
			t.Fatalf("get liveCE: %v", err)
		}
		if live.Deleted == nil || live.Deleted.DeletedAtUnixSec == 0 {
			t.Error("expected live community_experience row to be soft-deleted")
		}
		if live.Deleted != nil && live.Deleted.DeletedByUserId != userID {
			t.Errorf("expected DeletedByUserId=%q, got %q", userID, live.Deleted.DeletedByUserId)
		}

		archived := &models.CommunityExperience{}
		if err := testStorage.GetByID(ctx, archivedCEID, archived, storage.QueryOptions{IncludeDeleted: true}); err != nil {
			t.Fatalf("get archivedCE: %v", err)
		}
		if archived.Deleted == nil {
			t.Error("expected archived community_experience row to also be soft-deleted")
		}
		if !archived.Archived {
			t.Error("expected archived=true to be preserved alongside deleted")
		}

		// Normal query (filter-applied) should now return no rows.
		remaining, err := testStorage.QueryByField(ctx, "experience_id", expID, &models.CommunityExperience{})
		if err != nil {
			t.Fatalf("QueryByField: %v", err)
		}
		if len(remaining) != 0 {
			t.Errorf("expected 0 live community_experience rows after delete, got %d", len(remaining))
		}
	})

	// Regression test for #1698: DeleteExperience should cascade to all
	// current-state participant tables (RSVP, time proposal, planning
	// need, planning contribution) AND should NOT touch activity-log
	// rows (CommunityEvent, Story).
	t.Run("deletion cascades to participant rows but not activity log", func(t *testing.T) {
		userID := "user-del-1698"
		createTestUser(t, testStorage, userID, "del1698@example.com", "Del User 1698")
		ctx := createAuthenticatedContext(userID, "del1698@example.com", models.Role_ROLE_USER)

		createResp, err := service.SaveExperience(ctx, connect.NewRequest(&api.SaveExperienceRequest{
			Name:        "Cascade #1698 Experience",
			Description: "regression for participant-state cascade",
		}))
		if err != nil {
			t.Fatalf("SaveExperience: %v", err)
		}
		expID := createResp.Msg.Experience.Id

		// Seed participant-state rows scoped to this experience.
		rsvp := &models.ExperienceRSVP{
			ExperienceId: expID, UserId: "rsvp-user", CommunityId: "comm-x",
			Intention: models.RSVPIntention_RSVP_INTENTION_YES, Attended: models.AttendedStatus_ATTENDED_STATUS_UNKNOWN,
		}
		rsvpID, err := testStorage.Insert(ctx, rsvp)
		if err != nil {
			t.Fatalf("insert rsvp: %v", err)
		}

		proposal := &models.ExperienceTimeProposal{
			ExperienceId: expID, ProposedByUserId: userID,
		}
		proposalID, err := testStorage.Insert(ctx, proposal)
		if err != nil {
			t.Fatalf("insert proposal: %v", err)
		}

		need := &models.PlanningNeed{
			ProposerId: userID, Name: "snacks", Slots: 1, SlotsRemaining: 1,
			Scope: &models.PlanningNeed_ExperienceId{ExperienceId: expID},
		}
		needID, err := testStorage.Insert(ctx, need)
		if err != nil {
			t.Fatalf("insert need: %v", err)
		}

		contrib := &models.PlanningContribution{
			ContributorId: "contrib-user", Title: "bring chairs",
			Scope: &models.PlanningContribution_ExperienceId{ExperienceId: expID},
		}
		contribID, err := testStorage.Insert(ctx, contrib)
		if err != nil {
			t.Fatalf("insert contrib: %v", err)
		}

		// Seed activity-log rows that reference this experience. These
		// MUST survive the delete intact — history isn't tombstoned.
		event := &models.CommunityEvent{
			CommunityId:       "comm-x",
			EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
			ActorId:           userID,
			Topic:             &models.CommunityEvent_ExperienceId{ExperienceId: expID},
			OccurredAtUnixSec: 1000,
		}
		eventID, err := testStorage.Insert(ctx, event)
		if err != nil {
			t.Fatalf("insert event: %v", err)
		}

		story := &models.Story{
			ExperienceId: expID,
			CommunityId:  "comm-x",
		}
		storyID, err := testStorage.Insert(ctx, story)
		if err != nil {
			t.Fatalf("insert story: %v", err)
		}

		// Also seed a planning-need scoped to a *different* experience to
		// verify scope isolation.
		otherNeed := &models.PlanningNeed{
			ProposerId: userID, Name: "tools", Slots: 1, SlotsRemaining: 1,
			Scope: &models.PlanningNeed_ExperienceId{ExperienceId: "exp-elsewhere"},
		}
		otherNeedID, err := testStorage.Insert(ctx, otherNeed)
		if err != nil {
			t.Fatalf("insert otherNeed: %v", err)
		}

		// Delete the experience.
		if _, err := service.DeleteExperience(ctx, connect.NewRequest(&api.DeleteExperienceRequest{ExperienceId: expID})); err != nil {
			t.Fatalf("DeleteExperience: %v", err)
		}

		// Helper: verify a row is soft-deleted, attributed to userID.
		assertSoftDeleted := func(name, id string, msg proto.Message, getDeleted func() *models.DeletedMetadata) {
			t.Helper()
			if err := testStorage.GetByID(ctx, id, msg, storage.QueryOptions{IncludeDeleted: true}); err != nil {
				t.Fatalf("%s GetByID(IncludeDeleted): %v", name, err)
			}
			d := getDeleted()
			if d == nil || d.DeletedAtUnixSec == 0 {
				t.Errorf("expected %s to be soft-deleted", name)
			}
			if d != nil && d.DeletedByUserId != userID {
				t.Errorf("expected %s DeletedByUserId=%q, got %q", name, userID, d.DeletedByUserId)
			}
		}

		gotRSVP := &models.ExperienceRSVP{}
		assertSoftDeleted("rsvp", rsvpID, gotRSVP, func() *models.DeletedMetadata { return gotRSVP.Deleted })

		gotProp := &models.ExperienceTimeProposal{}
		assertSoftDeleted("time_proposal", proposalID, gotProp, func() *models.DeletedMetadata { return gotProp.Deleted })

		gotNeed := &models.PlanningNeed{}
		assertSoftDeleted("planning_need", needID, gotNeed, func() *models.DeletedMetadata { return gotNeed.Deleted })

		gotContrib := &models.PlanningContribution{}
		assertSoftDeleted("planning_contribution", contribID, gotContrib, func() *models.DeletedMetadata { return gotContrib.Deleted })

		// Activity-log rows must be untouched (history preserved).
		gotEvent := &models.CommunityEvent{}
		if err := testStorage.GetByID(ctx, eventID, gotEvent); err != nil {
			t.Fatalf("get CommunityEvent after experience delete: %v", err)
		}

		gotStory := &models.Story{}
		if err := testStorage.GetByID(ctx, storyID, gotStory); err != nil {
			t.Fatalf("get Story after experience delete: %v", err)
		}
		if gotStory.Deleted != nil {
			t.Error("Story should not be soft-deleted on experience delete (activity log)")
		}

		// Scope isolation: the unrelated planning_need should be intact.
		gotOtherNeed := &models.PlanningNeed{}
		if err := testStorage.GetByID(ctx, otherNeedID, gotOtherNeed); err != nil {
			t.Fatalf("get otherNeed: %v", err)
		}
		if gotOtherNeed.Deleted != nil {
			t.Error("planning_need scoped to a different experience should not be touched")
		}

		// And default-filtered queries should now return nothing for the
		// cascaded participant tables.
		liveRSVPs, err := testStorage.QueryByField(ctx, "experience_id", expID, &models.ExperienceRSVP{})
		if err != nil {
			t.Fatalf("query rsvps: %v", err)
		}
		if len(liveRSVPs) != 0 {
			t.Errorf("expected 0 live RSVPs, got %d", len(liveRSVPs))
		}
	})
}
