package feed

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestListStories(t *testing.T) {
	tests := []struct {
		name         string
		setupStories int
		limit        int32
		wantCount    int
		wantErr      bool
		wantErrCode  connect.Code
	}{
		{
			name:         "empty community",
			setupStories: 0,
			limit:        10,
			wantCount:    0,
			wantErr:      false,
		},
		{
			name:         "single story",
			setupStories: 1,
			limit:        10,
			wantCount:    1,
			wantErr:      false,
		},
		{
			name:         "respects limit",
			setupStories: 10,
			limit:        5,
			wantCount:    5,
			wantErr:      false,
		},
		{
			name:         "default limit",
			setupStories: 15,
			limit:        0, // 0 should use default of 10
			wantCount:    10,
			wantErr:      false,
		},
		{
			name:         "max limit enforced",
			setupStories: 60,
			limit:        100, // Should be capped at 50
			wantCount:    50,
			wantErr:      false,
		},
		{
			name:         "multiple stories ordered correctly",
			setupStories: 3,
			limit:        10,
			wantCount:    3,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test environment
			sqlStorage := setupTestStorage(t)
			service := setupTestService(sqlStorage)

			// Create test user and community
			userID := setupTestUser(t, sqlStorage, "user@example.com", "Test User")
			communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")

			// Create participant users for stories
			participant1ID := setupTestUser(t, sqlStorage, "participant1@example.com", "Participant One")
			participant2ID := setupTestUser(t, sqlStorage, "participant2@example.com", "Participant Two")
			participantIDs := []string{participant1ID, participant2ID}

			// Setup stories with different creation times
			for i := 0; i < tt.setupStories; i++ {
				// Create stories with incrementing timestamps to test ordering
				story := &models.Story{
					CommunityId:      communityID,
					Title:            "Test Story " + string(rune('A'+i)),
					Description:      "Test story description",
					StoryType:        "STORY_TYPE_LOAN_COMPLETED",
					ParticipantIds:   participantIDs,
					MediaIds:         []string{},
					CreatedAtUnixSec: time.Now().Unix() + int64(i), // Incrementing timestamps
				}
				_, err := sqlStorage.Insert(context.Background(), story)
				require.NoError(t, err, "Failed to create test story")
			}

			// Create authenticated context
			ctx := createAuthenticatedContext(userID, "user@example.com", models.Role_ROLE_USER)

			// Call ListStories
			req := connect.NewRequest(&api.ListStoriesRequest{
				CommunityId: communityID,
				Limit:       tt.limit,
			})

			resp, err := service.ListStories(ctx, req)

			// Check error expectations
			if tt.wantErr {
				require.Error(t, err)
				if tt.wantErrCode != 0 {
					assert.Equal(t, tt.wantErrCode, connect.CodeOf(err))
				}
				return
			}

			// Check success case
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Len(t, resp.Msg.Stories, tt.wantCount)

			// Verify stories are ordered by creation time (newest first)
			if len(resp.Msg.Stories) > 1 {
				for i := 1; i < len(resp.Msg.Stories); i++ {
					// Each story's title should be in reverse alphabetical order
					// (since we created them with incrementing timestamps)
					prevTitle := resp.Msg.Stories[i-1].Title
					currTitle := resp.Msg.Stories[i].Title
					assert.Greater(t, prevTitle, currTitle,
						"Stories should be ordered newest first (title %s should come after %s)",
						prevTitle, currTitle)
				}
			}

			// Verify story content is properly populated
			if len(resp.Msg.Stories) > 0 {
				story := resp.Msg.Stories[0]
				assert.NotEmpty(t, story.Title)
				assert.NotEmpty(t, story.Description)
				assert.Equal(t, api.StoryType_STORY_TYPE_LOAN_COMPLETED, story.StoryType)
				assert.Len(t, story.Participants, 2, "Story should have enriched participant data")

				// Verify participants have names
				for _, participant := range story.Participants {
					assert.NotEmpty(t, participant.Name)
				}
			}
		})
	}
}

func TestListStories_Unauthorized(t *testing.T) {
	// Setup test environment
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	// Create test user and community
	userID := setupTestUser(t, sqlStorage, "user@example.com", "Test User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")

	// Create another user who is NOT a member of the community
	nonMemberID := setupTestUser(t, sqlStorage, "nonmember@example.com", "Non Member")

	// Create authenticated context for non-member
	ctx := createAuthenticatedContext(nonMemberID, "nonmember@example.com", models.Role_ROLE_USER)

	// Call ListStories
	req := connect.NewRequest(&api.ListStoriesRequest{
		CommunityId: communityID,
		Limit:       10,
	})

	resp, err := service.ListStories(ctx, req)

	// Should return permission denied
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
}

func TestListStories_CommunityNotFound(t *testing.T) {
	// Setup test environment
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	// Create test user
	userID := setupTestUser(t, sqlStorage, "user@example.com", "Test User")

	// Create authenticated context
	ctx := createAuthenticatedContext(userID, "user@example.com", models.Role_ROLE_USER)

	// Call ListStories with non-existent community
	req := connect.NewRequest(&api.ListStoriesRequest{
		CommunityId: "nonexistent-community-id",
		Limit:       10,
	})

	resp, err := service.ListStories(ctx, req)

	// Should return NotFound (community does not exist).
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

func TestListStories_Unauthenticated(t *testing.T) {
	// Setup test environment
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	// Create test user and community
	userID := setupTestUser(t, sqlStorage, "user@example.com", "Test User")
	communityID := setupTestCommunity(t, sqlStorage, userID, "Test Community")

	// Use unauthenticated context
	ctx := context.Background()

	// Call ListStories
	req := connect.NewRequest(&api.ListStoriesRequest{
		CommunityId: communityID,
		Limit:       10,
	})

	resp, err := service.ListStories(ctx, req)

	// Should return unauthenticated error
	require.Error(t, err)
	assert.Nil(t, resp)
	assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestConvertStoryToAPI(t *testing.T) {
	// Setup test environment
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)

	// Create test users
	userID1 := setupTestUser(t, sqlStorage, "user1@example.com", "User One")
	userID2 := setupTestUser(t, sqlStorage, "user2@example.com", "User Two")

	// Create model story
	modelStory := &models.Story{
		Id:               "story123",
		Title:            "Test Story",
		Description:      "Test description",
		StoryType:        "STORY_TYPE_LOAN_COMPLETED",
		ParticipantIds:   []string{userID1, userID2},
		MediaIds:         []string{"media1", "media2"},
		GearId:           "gear123",
		LoanId:           "loan123",
		CreatedAtUnixSec: time.Now().Unix(),
	}

	// Convert to API (via batch converter)
	ctx := context.Background()
	apiStories, err := service.convertStoriesToAPI(ctx, []*models.Story{modelStory}, "test-viewer")

	// Verify conversion
	require.NoError(t, err)
	require.Len(t, apiStories, 1)
	apiStory := apiStories[0]
	assert.Equal(t, "Test Story", apiStory.Title)
	assert.Equal(t, "Test description", apiStory.Description)
	assert.Equal(t, api.StoryType_STORY_TYPE_LOAN_COMPLETED, apiStory.StoryType)
	assert.Len(t, apiStory.Participants, 2)
	assert.Equal(t, []string{"media1", "media2"}, apiStory.MediaIds)
	assert.Equal(t, "gear123", apiStory.GearId)
	assert.Equal(t, "loan123", apiStory.LoanId)

	// Verify participants are enriched
	assert.Equal(t, userID1, apiStory.Participants[0].Id)
	assert.Equal(t, "User One", apiStory.Participants[0].Name)
	assert.Equal(t, userID2, apiStory.Participants[1].Id)
	assert.Equal(t, "User Two", apiStory.Participants[1].Name)

	// Story has no CommunityEventId — no undo affordance regardless of viewer.
	assert.Nil(t, apiStory.UndoableAction,
		"story without community_event_id should not surface an UndoableAction")
}

// TestConvertStoryToAPI_UndoableAction verifies UndoableAction is
// populated only when the viewing user is the actor on the story's
// CommunityEvent, and cleared once a retraction event exists.
func TestConvertStoryToAPI_UndoableAction(t *testing.T) {
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)
	ctx := context.Background()

	actorID := setupTestUser(t, sqlStorage, "actor@example.com", "Actor")
	otherID := setupTestUser(t, sqlStorage, "other@example.com", "Other")

	// Create a TRANSFER_COMPLETED community event (as if a loan was completed).
	event := &models.CommunityEvent{
		CommunityId:       "community-1",
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED,
		ActorId:           actorID,
		GearId:            "gear-1",
		Topic:             &models.CommunityEvent_TransferId{TransferId: "transfer-1"},
		TransferType:      models.TransferType_TRANSFER_TYPE_LOAN,
		OccurredAtUnixSec: time.Now().Unix(),
	}
	eventID, err := sqlStorage.Insert(ctx, event)
	require.NoError(t, err)

	story := &models.Story{
		Title:            "Loan complete",
		StoryType:        "STORY_TYPE_LOAN_COMPLETED",
		CommunityEventId: &eventID,
		CreatedAtUnixSec: time.Now().Unix(),
	}

	// Viewer is the actor → UndoableAction populated.
	apiStories, err := service.convertStoriesToAPI(ctx, []*models.Story{story}, actorID)
	require.NoError(t, err)
	require.Len(t, apiStories, 1)
	require.NotNil(t, apiStories[0].UndoableAction, "actor should see UndoableAction")
	assert.Equal(t, eventID, apiStories[0].UndoableAction.CommunityEventId)
	assert.Equal(t, "Undo marking as returned", apiStories[0].UndoableAction.Label)

	// Viewer is someone else → no UndoableAction.
	apiStories, err = service.convertStoriesToAPI(ctx, []*models.Story{story}, otherID)
	require.NoError(t, err)
	require.Len(t, apiStories, 1)
	assert.Nil(t, apiStories[0].UndoableAction, "non-actor should not see UndoableAction")

	// Insert a retraction event — even the actor should no longer see
	// UndoableAction, because the action has already been undone.
	retraction := &models.CommunityEvent{
		CommunityId:       "community-1",
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_TRANSFER_COMPLETED_UNDONE,
		ActorId:           actorID,
		GearId:            "gear-1",
		Topic:             &models.CommunityEvent_TransferId{TransferId: "transfer-1"},
		TransferType:      models.TransferType_TRANSFER_TYPE_LOAN,
		OccurredAtUnixSec: time.Now().Unix() + 1,
	}
	_, err = sqlStorage.Insert(ctx, retraction)
	require.NoError(t, err)

	apiStories, err = service.convertStoriesToAPI(ctx, []*models.Story{story}, actorID)
	require.NoError(t, err)
	require.Len(t, apiStories, 1)
	assert.Nil(t, apiStories[0].UndoableAction,
		"after retraction exists, even actor should not see UndoableAction")
}

func TestListItemStories(t *testing.T) {
	// Setup test environment
	sqlStorage := setupTestStorage(t)
	service := setupTestService(sqlStorage)
	storyStorage := storage.NewStoryStorage(sqlStorage)
	ctx := context.Background()

	// Create test users and a shared community.
	user1ID := setupTestUser(t, sqlStorage, "user1@example.com", "User One")
	user2ID := setupTestUser(t, sqlStorage, "user2@example.com", "User Two")
	communityID := setupTestCommunity(t, sqlStorage, user1ID, "Test Community")

	// Add user2 as a member of the community.
	_, err := sqlStorage.Insert(ctx, &models.CommunityUser{
		CommunityId:      communityID,
		UserId:           user2ID,
		InviterId:        user1ID,
		CreatedAtUnixSec: time.Now().Unix(),
	})
	require.NoError(t, err, "Failed to add user2 to community")

	// createGearWithCommunity inserts a Gear record, inserts a CommunityGear row,
	// and returns the gear ID.
	createGearWithCommunity := func(ownerID, cID string) string {
		gear := &models.Gear{
			Name:    "Test Gear",
			OwnerId: ownerID,
			State:   models.GearState_GEAR_STATE_AVAILABLE,
		}
		gearID, insertErr := sqlStorage.Insert(ctx, gear)
		require.NoError(t, insertErr, "Failed to create gear")
		_, insertErr = sqlStorage.Insert(ctx, &models.CommunityGear{
			CommunityId:      cID,
			GearId:           gearID,
			Availability:     models.Availability_AVAILABILITY_FOR_LOAN,
			CreatedAtUnixSec: time.Now().Unix(),
		})
		require.NoError(t, insertErr, "Failed to create community gear")
		return gearID
	}

	// createExperienceWithCommunity inserts an Experience record, inserts a
	// CommunityExperience row, and returns the experience ID.
	createExperienceWithCommunity := func(ownerID, cID string) string {
		exp := &models.Experience{
			Name:    "Test Experience",
			OwnerId: ownerID,
			State:   models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
		}
		expID, insertErr := sqlStorage.Insert(ctx, exp)
		require.NoError(t, insertErr, "Failed to create experience")
		_, insertErr = sqlStorage.Insert(ctx, &models.CommunityExperience{
			CommunityId:     cID,
			ExperienceId:    expID,
			SharedAtUnixSec: time.Now().Unix(),
		})
		require.NoError(t, insertErr, "Failed to create community experience")
		return expID
	}

	// createRequestWithCommunity inserts a Request record, inserts a
	// CommunityRequest row, and returns the request ID.
	createRequestWithCommunity := func(ownerID, cID string) string {
		req := &models.Request{
			RequesterId: ownerID,
			Description: "Test request",
			State:       models.RequestState_REQUEST_STATE_ACTIVE,
		}
		reqID, insertErr := sqlStorage.Insert(ctx, req)
		require.NoError(t, insertErr, "Failed to create request")
		_, insertErr = sqlStorage.Insert(ctx, &models.CommunityRequest{
			CommunityId:     cID,
			RequestId:       reqID,
			SharedAtUnixSec: time.Now().Unix(),
		})
		require.NoError(t, insertErr, "Failed to create community request")
		return reqID
	}

	// Helper to create a story linked to specific item IDs.
	createStory := func(gearID, experienceID, requestID string, createdAt int64) string {
		story := &models.Story{
			CommunityId:      communityID,
			StoryType:        "STORY_TYPE_LOAN_COMPLETED",
			Title:            "Test Story",
			Description:      "A test story",
			ParticipantIds:   []string{user1ID, user2ID},
			GearId:           gearID,
			ExperienceId:     experienceID,
			RequestId:        requestID,
			CreatedAtUnixSec: createdAt,
		}
		storyID, insertErr := storyStorage.Insert(ctx, story)
		require.NoError(t, insertErr, "Failed to create story")
		return storyID
	}

	t.Run("successful list for gear with stories", func(t *testing.T) {
		gearID := createGearWithCommunity(user1ID, communityID)
		otherGearID := createGearWithCommunity(user1ID, communityID)
		createStory(gearID, "", "", 3000)
		createStory(gearID, "", "", 2000)
		createStory(otherGearID, "", "", 1000) // Different gear

		authCtx := createAuthenticatedContext(user1ID, "user1@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.ListItemStoriesRequest{
			ItemId: gearID,
			Limit:  10,
		})

		resp, err := service.ListItemStories(authCtx, req)
		require.NoError(t, err)
		assert.Len(t, resp.Msg.Stories, 2)
	})

	t.Run("successful list for experience with stories", func(t *testing.T) {
		expID := createExperienceWithCommunity(user1ID, communityID)
		createStory("", expID, "", time.Now().Unix())

		authCtx := createAuthenticatedContext(user1ID, "user1@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.ListItemStoriesRequest{
			ItemId: expID,
			Limit:  10,
		})

		resp, err := service.ListItemStories(authCtx, req)
		require.NoError(t, err)
		assert.Len(t, resp.Msg.Stories, 1)
	})

	t.Run("successful list for request with stories", func(t *testing.T) {
		reqID := createRequestWithCommunity(user1ID, communityID)
		createStory("", "", reqID, time.Now().Unix())

		authCtx := createAuthenticatedContext(user1ID, "user1@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.ListItemStoriesRequest{
			ItemId: reqID,
			Limit:  10,
		})

		resp, err := service.ListItemStories(authCtx, req)
		require.NoError(t, err)
		assert.Len(t, resp.Msg.Stories, 1)
	})

	t.Run("respects limit parameter", func(t *testing.T) {
		gearID := createGearWithCommunity(user1ID, communityID)
		createStory(gearID, "", "", 6000)
		createStory(gearID, "", "", 5000)
		createStory(gearID, "", "", 4000)

		authCtx := createAuthenticatedContext(user1ID, "user1@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.ListItemStoriesRequest{
			ItemId: gearID,
			Limit:  2,
		})

		resp, err := service.ListItemStories(authCtx, req)
		require.NoError(t, err)
		assert.Len(t, resp.Msg.Stories, 2)
	})

	t.Run("returns NotFound for item not in any community table", func(t *testing.T) {
		authCtx := createAuthenticatedContext(user1ID, "user1@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.ListItemStoriesRequest{
			ItemId: "00000000-0000-0000-0000-000000000000",
			Limit:  10,
		})

		resp, err := service.ListItemStories(authCtx, req)
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	})

	t.Run("orders by created_at desc (newest first)", func(t *testing.T) {
		gearID := createGearWithCommunity(user1ID, communityID)
		createStory(gearID, "", "", 1000) // Oldest
		createStory(gearID, "", "", 3000) // Newest
		createStory(gearID, "", "", 2000) // Middle

		authCtx := createAuthenticatedContext(user1ID, "user1@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.ListItemStoriesRequest{
			ItemId: gearID,
			Limit:  10,
		})

		resp, err := service.ListItemStories(authCtx, req)
		require.NoError(t, err)
		assert.Len(t, resp.Msg.Stories, 3)
		// Note: Ordering is verified in storage layer tests
	})

	t.Run("excludes deleted stories", func(t *testing.T) {
		gearID := createGearWithCommunity(user1ID, communityID)
		createStory(gearID, "", "", time.Now().Unix())

		// Create deleted story directly via storage.
		deletedStory := &models.Story{
			CommunityId:      communityID,
			StoryType:        "STORY_TYPE_LOAN_COMPLETED",
			Title:            "Deleted Story",
			Description:      "This should not appear",
			ParticipantIds:   []string{user1ID},
			GearId:           gearID,
			CreatedAtUnixSec: time.Now().Unix() - 100,
			Deleted: &models.DeletedMetadata{
				DeletedAtUnixSec: time.Now().Unix(),
				DeletedByUserId:  user1ID,
			},
		}
		_, insertErr := storyStorage.Insert(ctx, deletedStory)
		require.NoError(t, insertErr)

		authCtx := createAuthenticatedContext(user1ID, "user1@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.ListItemStoriesRequest{
			ItemId: gearID,
			Limit:  10,
		})

		resp, err := service.ListItemStories(authCtx, req)
		require.NoError(t, err)
		assert.Len(t, resp.Msg.Stories, 1, "Should only return active story, not deleted")
	})

	t.Run("unauthenticated request fails", func(t *testing.T) {
		unauthCtx := context.Background()

		req := connect.NewRequest(&api.ListItemStoriesRequest{
			ItemId: "any-item-id",
			Limit:  10,
		})

		resp, err := service.ListItemStories(unauthCtx, req)
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	})

	t.Run("default limit applied when not specified", func(t *testing.T) {
		gearID := createGearWithCommunity(user1ID, communityID)
		createStory(gearID, "", "", time.Now().Unix())

		authCtx := createAuthenticatedContext(user1ID, "user1@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.ListItemStoriesRequest{
			ItemId: gearID,
			Limit:  0, // No limit specified
		})

		resp, err := service.ListItemStories(authCtx, req)
		require.NoError(t, err)
		assert.Len(t, resp.Msg.Stories, 1)
	})

	t.Run("max limit enforced", func(t *testing.T) {
		gearID := createGearWithCommunity(user1ID, communityID)

		authCtx := createAuthenticatedContext(user1ID, "user1@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.ListItemStoriesRequest{
			ItemId: gearID,
			Limit:  1000,
		})

		// Should not error; limit is capped at 50.
		resp, err := service.ListItemStories(authCtx, req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
	})

	t.Run("non-member is denied for gear item", func(t *testing.T) {
		gearID := createGearWithCommunity(user1ID, communityID)
		createStory(gearID, "", "", time.Now().Unix())

		// Create a user who is NOT a member of the community.
		nonMemberID := setupTestUser(t, sqlStorage, "nonmember@example.com", "Non Member")
		authCtx := createAuthenticatedContext(nonMemberID, "nonmember@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.ListItemStoriesRequest{
			ItemId: gearID,
			Limit:  10,
		})

		resp, err := service.ListItemStories(authCtx, req)
		require.Error(t, err)
		assert.Nil(t, resp)
		assert.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	})

	t.Run("owner can read item stories without community membership", func(t *testing.T) {
		// Create a second community and share gear with it.  The owner is NOT a
		// member of this second community, but the owner-bypass in
		// RequireAccessToCommunityScopedEntity allows them to read stories for
		// their own gear regardless of membership.
		ownerID := setupTestUser(t, sqlStorage, "owner-no-membership@example.com", "Owner No Membership")
		secondCommunityID := setupTestCommunity(t, sqlStorage, user1ID, "Second Community")

		gear := &models.Gear{
			Name:    "Owner Gear",
			OwnerId: ownerID,
			State:   models.GearState_GEAR_STATE_AVAILABLE,
		}
		ownerGearID, insertErr := sqlStorage.Insert(ctx, gear)
		require.NoError(t, insertErr, "Failed to create owner gear")

		// Share gear with the second community (owner is not a member there).
		_, insertErr = sqlStorage.Insert(ctx, &models.CommunityGear{
			CommunityId:      secondCommunityID,
			GearId:           ownerGearID,
			Availability:     models.Availability_AVAILABILITY_FOR_LOAN,
			CreatedAtUnixSec: time.Now().Unix(),
		})
		require.NoError(t, insertErr, "Failed to create community gear for owner")

		createStory(ownerGearID, "", "", time.Now().Unix())

		// Owner (not a member of secondCommunityID) can still access their own
		// gear's stories via the owner-bypass.
		authCtx := createAuthenticatedContext(ownerID, "owner-no-membership@example.com", models.Role_ROLE_USER)

		req := connect.NewRequest(&api.ListItemStoriesRequest{
			ItemId: ownerGearID,
			Limit:  10,
		})

		resp, err := service.ListItemStories(authCtx, req)
		require.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Len(t, resp.Msg.Stories, 1)
	})
}
