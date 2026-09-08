package storage

import (
	"context"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

func TestNewStoryStorage(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	storyStorage := NewStoryStorage(storage)

	if storyStorage == nil {
		t.Fatal("Expected storyStorage to be created")
	}

	if storyStorage.storage != storage {
		t.Error("Expected storage to be set correctly")
	}
}

func TestStoryStorage_InsertAndGetByID(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	storyStorage := NewStoryStorage(storage)
	ctx := context.Background()

	story := &models.Story{
		CommunityId:      "community1",
		StoryType:        "STORY_TYPE_LOAN_COMPLETED",
		Title:            "A successful loan",
		Description:      "John borrowed a tent from Sarah and returned it in great condition.",
		MediaIds:         []string{"media1", "media2"},
		ParticipantIds:   []string{"user1", "user2"},
		LoanId:           "loan123",
		CreatedAtUnixSec: time.Now().Unix(),
	}

	// Insert story
	storyID, err := storyStorage.Insert(ctx, story)
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	if storyID == "" {
		t.Fatal("Expected story ID to be generated")
	}

	// Retrieve story
	retrieved, err := storyStorage.GetByID(ctx, storyID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	// Verify fields
	if retrieved.Id != storyID {
		t.Errorf("Expected ID %s, got %s", storyID, retrieved.Id)
	}

	if retrieved.CommunityId != story.CommunityId {
		t.Errorf("Expected community ID %s, got %s", story.CommunityId, retrieved.CommunityId)
	}

	if retrieved.StoryType != story.StoryType {
		t.Errorf("Expected story type %s, got %s", story.StoryType, retrieved.StoryType)
	}

	if retrieved.Title != story.Title {
		t.Errorf("Expected title %s, got %s", story.Title, retrieved.Title)
	}

	if retrieved.Description != story.Description {
		t.Errorf("Expected description %s, got %s", story.Description, retrieved.Description)
	}

	if len(retrieved.MediaIds) != 2 {
		t.Errorf("Expected 2 media IDs, got %d", len(retrieved.MediaIds))
	}

	if len(retrieved.ParticipantIds) != 2 {
		t.Errorf("Expected 2 participant IDs, got %d", len(retrieved.ParticipantIds))
	}

	if retrieved.LoanId != story.LoanId {
		t.Errorf("Expected related loan ID %s, got %s", story.LoanId, retrieved.LoanId)
	}
}

// TestStoryStorage_TemplateKeyAndParamsRoundTrip confirms Phase 4b's
// new structured-template fields (template_key + template_params)
// persist through storage round-trip on the Story proto. Without this
// the client would never receive the locale-renderable payload.
func TestStoryStorage_TemplateKeyAndParamsRoundTrip(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	storyStorage := NewStoryStorage(storage)
	ctx := context.Background()
	templateKey := "story.loan_completed"
	story := &models.Story{
		CommunityId:      "community1",
		StoryType:        "STORY_TYPE_LOAN_COMPLETED",
		Title:            "tent Lent",
		Description:      "Alice borrowed tent from Bob and returned it.",
		ParticipantIds:   []string{"user1", "user2"},
		LoanId:           "loan123",
		CreatedAtUnixSec: time.Now().Unix(),
		TemplateKey:      &templateKey,
		TemplateParams: map[string]string{
			"borrowerName": "Alice",
			"gearName":     "tent",
			"lenderName":   "Bob",
		},
	}

	storyID, err := storyStorage.Insert(ctx, story)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	retrieved, err := storyStorage.GetByID(ctx, storyID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}

	if retrieved.GetTemplateKey() != "story.loan_completed" {
		t.Errorf("TemplateKey round-trip: got %q; want %q",
			retrieved.GetTemplateKey(), "story.loan_completed")
	}
	if retrieved.TemplateParams["borrowerName"] != "Alice" ||
		retrieved.TemplateParams["gearName"] != "tent" ||
		retrieved.TemplateParams["lenderName"] != "Bob" {
		t.Errorf("TemplateParams round-trip: got %v; want borrower=Alice, gear=tent, lender=Bob",
			retrieved.TemplateParams)
	}
}

// TestStoryStorage_TemplateKeyOmittedForAIPath confirms AI-generated
// stories (no template_key, no params) still persist correctly. The
// client falls back to literal title/description for these rows.
func TestStoryStorage_TemplateKeyOmittedForAIPath(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	storyStorage := NewStoryStorage(storage)
	ctx := context.Background()
	story := &models.Story{
		CommunityId:      "community1",
		StoryType:        "STORY_TYPE_LOAN_COMPLETED",
		Title:            "AI-generated headline",
		Description:      "AI-generated prose.",
		CreatedAtUnixSec: time.Now().Unix(),
		// TemplateKey + TemplateParams intentionally absent.
	}
	storyID, err := storyStorage.Insert(ctx, story)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	retrieved, err := storyStorage.GetByID(ctx, storyID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if retrieved.TemplateKey != nil {
		t.Errorf("TemplateKey should be nil for AI path; got %q",
			retrieved.GetTemplateKey())
	}
	if len(retrieved.TemplateParams) != 0 {
		t.Errorf("TemplateParams should be empty for AI path; got %v",
			retrieved.TemplateParams)
	}
}

func TestStoryStorage_ListByCommunity(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	storyStorage := NewStoryStorage(storage)
	ctx := context.Background()

	// Create multiple stories in different communities
	now := time.Now().Unix()
	stories := []*models.Story{
		{
			CommunityId:      "community1",
			StoryType:        "STORY_TYPE_LOAN_COMPLETED",
			Title:            "Story 1",
			Description:      "First story",
			CreatedAtUnixSec: now - 100,
		},
		{
			CommunityId:      "community1",
			StoryType:        "STORY_TYPE_GIVEAWAY_COMPLETED",
			Title:            "Story 2",
			Description:      "Second story",
			CreatedAtUnixSec: now - 50,
		},
		{
			CommunityId:      "community1",
			StoryType:        "STORY_TYPE_NEW_MEMBER_WELCOME",
			Title:            "Story 3",
			Description:      "Third story (newest)",
			CreatedAtUnixSec: now,
		},
		{
			CommunityId:      "community2",
			StoryType:        "STORY_TYPE_LOAN_COMPLETED",
			Title:            "Story 4",
			Description:      "Different community",
			CreatedAtUnixSec: now,
		},
	}

	for _, story := range stories {
		_, err := storyStorage.Insert(ctx, story)
		if err != nil {
			t.Fatalf("Insert failed: %v", err)
		}
	}

	// List stories for community1
	retrieved, err := storyStorage.ListByCommunity(ctx, "community1", 10)
	if err != nil {
		t.Fatalf("ListByCommunity failed: %v", err)
	}

	if len(retrieved) != 3 {
		t.Fatalf("Expected 3 stories for community1, got %d", len(retrieved))
	}

	// Verify stories are ordered by creation time (newest first)
	if retrieved[0].Title != "Story 3" {
		t.Errorf("Expected newest story first, got %s", retrieved[0].Title)
	}

	if retrieved[1].Title != "Story 2" {
		t.Errorf("Expected second newest story second, got %s", retrieved[1].Title)
	}

	if retrieved[2].Title != "Story 1" {
		t.Errorf("Expected oldest story last, got %s", retrieved[2].Title)
	}

	// List stories for community2
	retrieved2, err := storyStorage.ListByCommunity(ctx, "community2", 10)
	if err != nil {
		t.Fatalf("ListByCommunity failed: %v", err)
	}

	if len(retrieved2) != 1 {
		t.Fatalf("Expected 1 story for community2, got %d", len(retrieved2))
	}

	if retrieved2[0].Title != "Story 4" {
		t.Errorf("Expected Story 4 for community2, got %s", retrieved2[0].Title)
	}
}

func TestStoryStorage_ListByCommunity_EmptyCommunity(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	storyStorage := NewStoryStorage(storage)
	ctx := context.Background()

	// List stories for non-existent community
	retrieved, err := storyStorage.ListByCommunity(ctx, "nonexistent", 10)
	if err != nil {
		t.Fatalf("ListByCommunity failed: %v", err)
	}

	if len(retrieved) != 0 {
		t.Errorf("Expected 0 stories for nonexistent community, got %d", len(retrieved))
	}
}

func TestStoryStorage_ListByCommunity_Limit(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	storyStorage := NewStoryStorage(storage)
	ctx := context.Background()

	// Create 5 stories
	for i := 0; i < 5; i++ {
		story := &models.Story{
			CommunityId:      "community1",
			StoryType:        "STORY_TYPE_LOAN_COMPLETED",
			Title:            "Story",
			Description:      "Description",
			CreatedAtUnixSec: time.Now().Unix(),
		}
		_, err := storyStorage.Insert(ctx, story)
		if err != nil {
			t.Fatalf("Insert failed: %v", err)
		}
	}

	// List with limit of 3
	retrieved, err := storyStorage.ListByCommunity(ctx, "community1", 3)
	if err != nil {
		t.Fatalf("ListByCommunity failed: %v", err)
	}

	if len(retrieved) != 3 {
		t.Errorf("Expected 3 stories (limit), got %d", len(retrieved))
	}
}

func TestStoryStorage_Exists_LoanCompleted(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	storyStorage := NewStoryStorage(storage)
	ctx := context.Background()

	story := &models.Story{
		CommunityId:      "community1",
		StoryType:        "STORY_TYPE_LOAN_COMPLETED",
		Title:            "Loan completed",
		Description:      "Description",
		LoanId:           "loan123",
		CreatedAtUnixSec: time.Now().Unix(),
	}

	// Story doesn't exist yet
	exists, err := storyStorage.Exists(ctx, "community1", "STORY_TYPE_LOAN_COMPLETED", "loan123")
	if err != nil {
		t.Fatalf("Exists check failed: %v", err)
	}

	if exists {
		t.Error("Expected story to not exist yet")
	}

	// Insert story
	_, err = storyStorage.Insert(ctx, story)
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	// Now story should exist
	exists, err = storyStorage.Exists(ctx, "community1", "STORY_TYPE_LOAN_COMPLETED", "loan123")
	if err != nil {
		t.Fatalf("Exists check failed: %v", err)
	}

	if !exists {
		t.Error("Expected story to exist after insert")
	}
}

func TestStoryStorage_Exists_DifferentRelatedEntityTypes(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	storyStorage := NewStoryStorage(storage)
	ctx := context.Background()

	tests := []struct {
		name          string
		story         *models.Story
		checkEntityID string
		shouldExist   bool
	}{
		{
			name: "Gear-related story",
			story: &models.Story{
				CommunityId:      "community1",
				StoryType:        "STORY_TYPE_GIVEAWAY_COMPLETED",
				Title:            "Giveaway",
				Description:      "Description",
				GearId:           "gear123",
				CreatedAtUnixSec: time.Now().Unix(),
			},
			checkEntityID: "gear123",
			shouldExist:   true,
		},
		{
			name: "Experience-related story",
			story: &models.Story{
				CommunityId:      "community1",
				StoryType:        "STORY_TYPE_EXPERIENCE_CONCLUDED",
				Title:            "Experience",
				Description:      "Description",
				ExperienceId:     "exp123",
				CreatedAtUnixSec: time.Now().Unix(),
			},
			checkEntityID: "exp123",
			shouldExist:   true,
		},
		{
			name: "Request-related story",
			story: &models.Story{
				CommunityId:      "community1",
				StoryType:        "STORY_TYPE_REQUEST_FULFILLED",
				Title:            "Request",
				Description:      "Description",
				RequestId:        "req123",
				CreatedAtUnixSec: time.Now().Unix(),
			},
			checkEntityID: "req123",
			shouldExist:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := storyStorage.Insert(ctx, tt.story)
			if err != nil {
				t.Fatalf("Insert failed: %v", err)
			}

			exists, err := storyStorage.Exists(ctx, tt.story.CommunityId, tt.story.StoryType, tt.checkEntityID)
			if err != nil {
				t.Fatalf("Exists check failed: %v", err)
			}

			if exists != tt.shouldExist {
				t.Errorf("Expected exists=%v, got %v", tt.shouldExist, exists)
			}
		})
	}
}

func TestStoryStorage_Exists_DifferentCommunity(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	storyStorage := NewStoryStorage(storage)
	ctx := context.Background()

	story := &models.Story{
		CommunityId:      "community1",
		StoryType:        "STORY_TYPE_LOAN_COMPLETED",
		Title:            "Loan",
		Description:      "Description",
		LoanId:           "loan123",
		CreatedAtUnixSec: time.Now().Unix(),
	}

	_, err := storyStorage.Insert(ctx, story)
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	// Check in different community
	exists, err := storyStorage.Exists(ctx, "community2", "STORY_TYPE_LOAN_COMPLETED", "loan123")
	if err != nil {
		t.Fatalf("Exists check failed: %v", err)
	}

	if exists {
		t.Error("Expected story to not exist in different community")
	}
}

func TestStoryStorage_Exists_DifferentStoryType(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	storyStorage := NewStoryStorage(storage)
	ctx := context.Background()

	story := &models.Story{
		CommunityId:      "community1",
		StoryType:        "STORY_TYPE_LOAN_COMPLETED",
		Title:            "Loan",
		Description:      "Description",
		LoanId:           "loan123",
		CreatedAtUnixSec: time.Now().Unix(),
	}

	_, err := storyStorage.Insert(ctx, story)
	if err != nil {
		t.Fatalf("Insert failed: %v", err)
	}

	// Check with different story type
	exists, err := storyStorage.Exists(ctx, "community1", "STORY_TYPE_GIVEAWAY_COMPLETED", "loan123")
	if err != nil {
		t.Fatalf("Exists check failed: %v", err)
	}

	if exists {
		t.Error("Expected story to not exist with different story type")
	}
}

func TestStoryStorage_Exists_NonExistent(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	storyStorage := NewStoryStorage(storage)
	ctx := context.Background()

	// Check for non-existent story
	exists, err := storyStorage.Exists(ctx, "community1", "STORY_TYPE_LOAN_COMPLETED", "nonexistent")
	if err != nil {
		t.Fatalf("Exists check failed: %v", err)
	}

	if exists {
		t.Error("Expected story to not exist")
	}
}

func TestStoryStorage_ListByUser(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(t *testing.T, ss *StoryStorage)
		userID string
		limit  int
		want   int
	}{
		{
			name: "returns stories where user is participant",
			setup: func(t *testing.T, ss *StoryStorage) {
				ctx := context.Background()
				// Create story with user1 and user2
				_, err := ss.Insert(ctx, &models.Story{
					CommunityId:      "community1",
					StoryType:        "STORY_TYPE_LOAN_COMPLETED",
					Title:            "Story 1",
					Description:      "User1 and User2",
					ParticipantIds:   []string{"user1", "user2"},
					CreatedAtUnixSec: time.Now().Unix(),
				})
				if err != nil {
					t.Fatalf("Failed to insert story 1: %v", err)
				}

				// Create story with user2 and user3
				_, err = ss.Insert(ctx, &models.Story{
					CommunityId:      "community1",
					StoryType:        "STORY_TYPE_LOAN_COMPLETED",
					Title:            "Story 2",
					Description:      "User2 and User3",
					ParticipantIds:   []string{"user2", "user3"},
					CreatedAtUnixSec: time.Now().Unix() - 100,
				})
				if err != nil {
					t.Fatalf("Failed to insert story 2: %v", err)
				}

				// Create story with only user3
				_, err = ss.Insert(ctx, &models.Story{
					CommunityId:      "community1",
					StoryType:        "STORY_TYPE_LOAN_COMPLETED",
					Title:            "Story 3",
					Description:      "Only User3",
					ParticipantIds:   []string{"user3"},
					CreatedAtUnixSec: time.Now().Unix() - 200,
				})
				if err != nil {
					t.Fatalf("Failed to insert story 3: %v", err)
				}
			},
			userID: "user2",
			limit:  10,
			want:   2, // Stories 1 and 2
		},
		{
			name: "respects limit",
			setup: func(t *testing.T, ss *StoryStorage) {
				ctx := context.Background()
				// Create 3 stories with user1
				for i := 0; i < 3; i++ {
					_, err := ss.Insert(ctx, &models.Story{
						CommunityId:      "community1",
						StoryType:        "STORY_TYPE_LOAN_COMPLETED",
						Title:            "Story",
						ParticipantIds:   []string{"user1"},
						CreatedAtUnixSec: time.Now().Unix() - int64(i*100),
					})
					if err != nil {
						t.Fatalf("Failed to insert story %d: %v", i, err)
					}
				}
			},
			userID: "user1",
			limit:  2,
			want:   2, // Only 2 stories despite 3 existing
		},
		{
			name: "orders by created_at desc",
			setup: func(t *testing.T, ss *StoryStorage) {
				ctx := context.Background()
				// Create stories with different timestamps
				_, err := ss.Insert(ctx, &models.Story{
					CommunityId:      "community1",
					StoryType:        "STORY_TYPE_LOAN_COMPLETED",
					Title:            "Oldest",
					ParticipantIds:   []string{"user1"},
					CreatedAtUnixSec: 1000,
				})
				if err != nil {
					t.Fatalf("Failed to insert oldest story: %v", err)
				}

				_, err = ss.Insert(ctx, &models.Story{
					CommunityId:      "community1",
					StoryType:        "STORY_TYPE_LOAN_COMPLETED",
					Title:            "Newest",
					ParticipantIds:   []string{"user1"},
					CreatedAtUnixSec: 3000,
				})
				if err != nil {
					t.Fatalf("Failed to insert newest story: %v", err)
				}

				_, err = ss.Insert(ctx, &models.Story{
					CommunityId:      "community1",
					StoryType:        "STORY_TYPE_LOAN_COMPLETED",
					Title:            "Middle",
					ParticipantIds:   []string{"user1"},
					CreatedAtUnixSec: 2000,
				})
				if err != nil {
					t.Fatalf("Failed to insert middle story: %v", err)
				}
			},
			userID: "user1",
			limit:  10,
			want:   3, // Should get all 3, newest first
		},
		{
			name: "excludes deleted stories",
			setup: func(t *testing.T, ss *StoryStorage) {
				ctx := context.Background()
				// Create active story
				_, err := ss.Insert(ctx, &models.Story{
					CommunityId:      "community1",
					StoryType:        "STORY_TYPE_LOAN_COMPLETED",
					Title:            "Active",
					ParticipantIds:   []string{"user1"},
					CreatedAtUnixSec: time.Now().Unix(),
				})
				if err != nil {
					t.Fatalf("Failed to insert active story: %v", err)
				}

				// Create deleted story
				_, err = ss.Insert(ctx, &models.Story{
					CommunityId:      "community1",
					StoryType:        "STORY_TYPE_LOAN_COMPLETED",
					Title:            "Deleted",
					ParticipantIds:   []string{"user1"},
					CreatedAtUnixSec: time.Now().Unix() - 100,
					Deleted: &models.DeletedMetadata{
						DeletedAtUnixSec: time.Now().Unix(),
						DeletedByUserId:  "admin",
					},
				})
				if err != nil {
					t.Fatalf("Failed to insert deleted story: %v", err)
				}
			},
			userID: "user1",
			limit:  10,
			want:   1, // Only active story
		},
		{
			name: "returns empty list for user with no stories",
			setup: func(t *testing.T, ss *StoryStorage) {
				ctx := context.Background()
				// Create story for different user
				_, err := ss.Insert(ctx, &models.Story{
					CommunityId:      "community1",
					StoryType:        "STORY_TYPE_LOAN_COMPLETED",
					Title:            "Other user",
					ParticipantIds:   []string{"user2"},
					CreatedAtUnixSec: time.Now().Unix(),
				})
				if err != nil {
					t.Fatalf("Failed to insert story: %v", err)
				}
			},
			userID: "user1",
			limit:  10,
			want:   0, // No stories for user1
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage, cleanup := SetupTestStorage(t)
			defer cleanup()

			ss := NewStoryStorage(storage)
			tt.setup(t, ss)

			ctx := context.Background()
			stories, err := ss.ListByUser(ctx, tt.userID, tt.limit)
			if err != nil {
				t.Fatalf("ListByUser failed: %v", err)
			}

			if len(stories) != tt.want {
				t.Errorf("Expected %d stories, got %d", tt.want, len(stories))
			}

			// For ordering test, verify order
			if tt.name == "orders by created_at desc" && len(stories) == 3 {
				if stories[0].Title != "Newest" {
					t.Errorf("Expected first story to be 'Newest', got '%s'", stories[0].Title)
				}
				if stories[1].Title != "Middle" {
					t.Errorf("Expected second story to be 'Middle', got '%s'", stories[1].Title)
				}
				if stories[2].Title != "Oldest" {
					t.Errorf("Expected third story to be 'Oldest', got '%s'", stories[2].Title)
				}
			}
		})
	}
}

func TestStoryStorage_ListByItem(t *testing.T) {
	tests := []struct {
		name   string
		setup  func(t *testing.T, ss *StoryStorage)
		itemID string
		limit  int
		want   int
	}{
		{
			name: "returns stories for gear_id",
			setup: func(t *testing.T, ss *StoryStorage) {
				ctx := context.Background()
				// Create stories for gear123
				_, err := ss.Insert(ctx, &models.Story{
					CommunityId:      "community1",
					StoryType:        "STORY_TYPE_LOAN_COMPLETED",
					Title:            "Gear Story 1",
					GearId:           "gear123",
					CreatedAtUnixSec: time.Now().Unix(),
				})
				if err != nil {
					t.Fatalf("Failed to insert gear story 1: %v", err)
				}

				_, err = ss.Insert(ctx, &models.Story{
					CommunityId:      "community1",
					StoryType:        "STORY_TYPE_GIVEAWAY_COMPLETED",
					Title:            "Gear Story 2",
					GearId:           "gear123",
					CreatedAtUnixSec: time.Now().Unix() - 100,
				})
				if err != nil {
					t.Fatalf("Failed to insert gear story 2: %v", err)
				}

				// Create story for different gear
				_, err = ss.Insert(ctx, &models.Story{
					CommunityId:      "community1",
					StoryType:        "STORY_TYPE_LOAN_COMPLETED",
					Title:            "Different Gear",
					GearId:           "gear456",
					CreatedAtUnixSec: time.Now().Unix(),
				})
				if err != nil {
					t.Fatalf("Failed to insert different gear story: %v", err)
				}
			},
			itemID: "gear123",
			limit:  10,
			want:   2,
		},
		{
			name: "returns stories for experience_id",
			setup: func(t *testing.T, ss *StoryStorage) {
				ctx := context.Background()
				_, err := ss.Insert(ctx, &models.Story{
					CommunityId:      "community1",
					StoryType:        "STORY_TYPE_EXPERIENCE_CONCLUDED",
					Title:            "Experience Story",
					ExperienceId:     "exp123",
					CreatedAtUnixSec: time.Now().Unix(),
				})
				if err != nil {
					t.Fatalf("Failed to insert experience story: %v", err)
				}
			},
			itemID: "exp123",
			limit:  10,
			want:   1,
		},
		{
			name: "returns stories for request_id",
			setup: func(t *testing.T, ss *StoryStorage) {
				ctx := context.Background()
				_, err := ss.Insert(ctx, &models.Story{
					CommunityId:      "community1",
					StoryType:        "STORY_TYPE_REQUEST_FULFILLED",
					Title:            "Request Story",
					RequestId:        "req123",
					CreatedAtUnixSec: time.Now().Unix(),
				})
				if err != nil {
					t.Fatalf("Failed to insert request story: %v", err)
				}
			},
			itemID: "req123",
			limit:  10,
			want:   1,
		},
		{
			name: "respects limit",
			setup: func(t *testing.T, ss *StoryStorage) {
				ctx := context.Background()
				// Create 3 stories for gear123
				for i := 0; i < 3; i++ {
					_, err := ss.Insert(ctx, &models.Story{
						CommunityId:      "community1",
						StoryType:        "STORY_TYPE_LOAN_COMPLETED",
						Title:            "Story",
						GearId:           "gear123",
						CreatedAtUnixSec: time.Now().Unix() - int64(i*100),
					})
					if err != nil {
						t.Fatalf("Failed to insert story %d: %v", i, err)
					}
				}
			},
			itemID: "gear123",
			limit:  2,
			want:   2,
		},
		{
			name: "orders by created_at desc",
			setup: func(t *testing.T, ss *StoryStorage) {
				ctx := context.Background()
				_, err := ss.Insert(ctx, &models.Story{
					CommunityId:      "community1",
					StoryType:        "STORY_TYPE_LOAN_COMPLETED",
					Title:            "Oldest",
					GearId:           "gear123",
					CreatedAtUnixSec: 1000,
				})
				if err != nil {
					t.Fatalf("Failed to insert oldest story: %v", err)
				}

				_, err = ss.Insert(ctx, &models.Story{
					CommunityId:      "community1",
					StoryType:        "STORY_TYPE_LOAN_COMPLETED",
					Title:            "Newest",
					GearId:           "gear123",
					CreatedAtUnixSec: 3000,
				})
				if err != nil {
					t.Fatalf("Failed to insert newest story: %v", err)
				}

				_, err = ss.Insert(ctx, &models.Story{
					CommunityId:      "community1",
					StoryType:        "STORY_TYPE_LOAN_COMPLETED",
					Title:            "Middle",
					GearId:           "gear123",
					CreatedAtUnixSec: 2000,
				})
				if err != nil {
					t.Fatalf("Failed to insert middle story: %v", err)
				}
			},
			itemID: "gear123",
			limit:  10,
			want:   3,
		},
		{
			name: "excludes deleted stories",
			setup: func(t *testing.T, ss *StoryStorage) {
				ctx := context.Background()
				// Create active story
				_, err := ss.Insert(ctx, &models.Story{
					CommunityId:      "community1",
					StoryType:        "STORY_TYPE_LOAN_COMPLETED",
					Title:            "Active",
					GearId:           "gear123",
					CreatedAtUnixSec: time.Now().Unix(),
				})
				if err != nil {
					t.Fatalf("Failed to insert active story: %v", err)
				}

				// Create deleted story
				_, err = ss.Insert(ctx, &models.Story{
					CommunityId:      "community1",
					StoryType:        "STORY_TYPE_LOAN_COMPLETED",
					Title:            "Deleted",
					GearId:           "gear123",
					CreatedAtUnixSec: time.Now().Unix() - 100,
					Deleted: &models.DeletedMetadata{
						DeletedAtUnixSec: time.Now().Unix(),
						DeletedByUserId:  "admin",
					},
				})
				if err != nil {
					t.Fatalf("Failed to insert deleted story: %v", err)
				}
			},
			itemID: "gear123",
			limit:  10,
			want:   1,
		},
		{
			name: "returns empty list for item with no stories",
			setup: func(t *testing.T, ss *StoryStorage) {
				ctx := context.Background()
				// Create story for different item
				_, err := ss.Insert(ctx, &models.Story{
					CommunityId:      "community1",
					StoryType:        "STORY_TYPE_LOAN_COMPLETED",
					Title:            "Other item",
					GearId:           "gear456",
					CreatedAtUnixSec: time.Now().Unix(),
				})
				if err != nil {
					t.Fatalf("Failed to insert story: %v", err)
				}
			},
			itemID: "gear123",
			limit:  10,
			want:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			storage, cleanup := SetupTestStorage(t)
			defer cleanup()

			ss := NewStoryStorage(storage)
			tt.setup(t, ss)

			ctx := context.Background()
			stories, err := ss.ListByItem(ctx, tt.itemID, tt.limit)
			if err != nil {
				t.Fatalf("ListByItem failed: %v", err)
			}

			if len(stories) != tt.want {
				t.Errorf("Expected %d stories, got %d", tt.want, len(stories))
			}

			// For ordering test, verify order
			if tt.name == "orders by created_at desc" && len(stories) == 3 {
				if stories[0].Title != "Newest" {
					t.Errorf("Expected first story to be 'Newest', got '%s'", stories[0].Title)
				}
				if stories[1].Title != "Middle" {
					t.Errorf("Expected second story to be 'Middle', got '%s'", stories[1].Title)
				}
				if stories[2].Title != "Oldest" {
					t.Errorf("Expected third story to be 'Oldest', got '%s'", stories[2].Title)
				}
			}
		})
	}
}
