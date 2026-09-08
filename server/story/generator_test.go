package story

import (
	"context"
	"testing"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/storage"
)

func TestNewGenerator(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	storyStorage := storage.NewStoryStorage(sqlStorage)
	generator := NewGenerator(storyStorage)

	if generator == nil {
		t.Fatal("Expected generator to be created")
	}

	if generator.storyStorage != storyStorage {
		t.Error("Expected storyStorage to be set correctly")
	}

	if generator.logger == nil {
		t.Error("Expected logger to be initialized")
	}
}

func TestGenerator_CreateStory_LoanCompleted(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	storyStorage := storage.NewStoryStorage(sqlStorage)
	generator := NewGenerator(storyStorage)
	ctx := context.Background()

	req := CreateStoryRequest{
		CommunityID:       "community1",
		StoryType:         "STORY_TYPE_LOAN_COMPLETED",
		ParticipantIDs:    []string{"user1", "user2"},
		ParticipantNames:  []string{"Alice", "Bob"},
		MediaIDs:          []string{"media1"},
		RelatedEntityID:   "loan123",
		RelatedEntityName: "camping tent",
		RelatedLoanID:     "loan123",
	}

	story, err := generator.CreateStory(ctx, req)
	if err != nil {
		t.Fatalf("CreateStory failed: %v", err)
	}

	if story == nil {
		t.Fatal("Expected story to be created")
	}

	if story.Id == "" {
		t.Error("Expected story ID to be generated")
	}

	if story.CommunityId != req.CommunityID {
		t.Errorf("Expected community ID %s, got %s", req.CommunityID, story.CommunityId)
	}

	if story.StoryType != req.StoryType {
		t.Errorf("Expected story type %s, got %s", req.StoryType, story.StoryType)
	}

	if story.Title == "" {
		t.Error("Expected title to be generated")
	}

	if story.Description == "" {
		t.Error("Expected description to be generated")
	}

	if len(story.ParticipantIds) != 2 {
		t.Errorf("Expected 2 participant IDs, got %d", len(story.ParticipantIds))
	}

	if story.LoanId != req.RelatedLoanID {
		t.Errorf("Expected loan ID %s, got %s", req.RelatedLoanID, story.LoanId)
	}

	if story.CreatedAtUnixSec == 0 {
		t.Error("Expected created timestamp to be set")
	}
}

func TestGenerator_CreateStory_GiveawayCompleted(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	storyStorage := storage.NewStoryStorage(sqlStorage)
	generator := NewGenerator(storyStorage)
	ctx := context.Background()

	req := CreateStoryRequest{
		CommunityID:       "community1",
		StoryType:         "STORY_TYPE_GIVEAWAY_COMPLETED",
		ParticipantIDs:    []string{"user1", "user2"},
		ParticipantNames:  []string{"Carol", "Dave"},
		MediaIDs:          []string{"media1", "media2"},
		RelatedEntityID:   "gear456",
		RelatedEntityName: "bicycle",
		RelatedGearID:     "gear456",
	}

	story, err := generator.CreateStory(ctx, req)
	if err != nil {
		t.Fatalf("CreateStory failed: %v", err)
	}

	if story.StoryType != "STORY_TYPE_GIVEAWAY_COMPLETED" {
		t.Errorf("Expected STORY_TYPE_GIVEAWAY_COMPLETED, got %s", story.StoryType)
	}

	if story.GearId != req.RelatedGearID {
		t.Errorf("Expected gear ID %s, got %s", req.RelatedGearID, story.GearId)
	}
}

func TestGenerator_CreateStory_NewMemberWelcome(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	storyStorage := storage.NewStoryStorage(sqlStorage)
	generator := NewGenerator(storyStorage)
	ctx := context.Background()

	req := CreateStoryRequest{
		CommunityID:      "community1",
		StoryType:        "STORY_TYPE_NEW_MEMBER_WELCOME",
		ParticipantIDs:   []string{"user1"},
		ParticipantNames: []string{"Eve"},
		CommunityName:    "Downtown Sharing",
		RelatedEntityID:  "user1",
	}

	story, err := generator.CreateStory(ctx, req)
	if err != nil {
		t.Fatalf("CreateStory failed: %v", err)
	}

	if story.StoryType != "STORY_TYPE_NEW_MEMBER_WELCOME" {
		t.Errorf("Expected STORY_TYPE_NEW_MEMBER_WELCOME, got %s", story.StoryType)
	}

	if len(story.ParticipantIds) != 1 {
		t.Errorf("Expected 1 participant, got %d", len(story.ParticipantIds))
	}
}

func TestGenerator_CreateStory_Deduplication(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	storyStorage := storage.NewStoryStorage(sqlStorage)
	generator := NewGenerator(storyStorage)
	ctx := context.Background()

	req := CreateStoryRequest{
		CommunityID:       "community1",
		StoryType:         "STORY_TYPE_LOAN_COMPLETED",
		ParticipantIDs:    []string{"user1", "user2"},
		ParticipantNames:  []string{"Alice", "Bob"},
		RelatedEntityID:   "loan123",
		RelatedEntityName: "tent",
		RelatedLoanID:     "loan123",
	}

	// Create first story
	story1, err := generator.CreateStory(ctx, req)
	if err != nil {
		t.Fatalf("First CreateStory failed: %v", err)
	}

	if story1 == nil {
		t.Fatal("Expected first story to be created")
	}

	// Try to create duplicate story
	story2, err := generator.CreateStory(ctx, req)
	if err == nil {
		t.Fatal("Expected error when creating duplicate story")
	}

	if story2 != nil {
		t.Error("Expected nil story when duplicate is prevented")
	}

	// Verify error message mentions duplicate
	expectedErrorMsg := "story already exists for this event"
	if err.Error() != expectedErrorMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedErrorMsg, err.Error())
	}
}

func TestGenerator_CreateStory_DifferentCommunities(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	storyStorage := storage.NewStoryStorage(sqlStorage)
	generator := NewGenerator(storyStorage)
	ctx := context.Background()

	// Create story in community1
	req1 := CreateStoryRequest{
		CommunityID:       "community1",
		StoryType:         "STORY_TYPE_LOAN_COMPLETED",
		ParticipantIDs:    []string{"user1", "user2"},
		ParticipantNames:  []string{"Alice", "Bob"},
		RelatedEntityID:   "loan123",
		RelatedEntityName: "tent",
		RelatedLoanID:     "loan123",
	}

	story1, err := generator.CreateStory(ctx, req1)
	if err != nil {
		t.Fatalf("CreateStory for community1 failed: %v", err)
	}

	// Create story for same loan in community2 (should succeed - different community)
	req2 := req1
	req2.CommunityID = "community2"

	story2, err := generator.CreateStory(ctx, req2)
	if err != nil {
		t.Fatalf("CreateStory for community2 failed: %v", err)
	}

	if story1.Id == story2.Id {
		t.Error("Expected different story IDs for different communities")
	}
}

func TestGenerator_CreateStory_NoRelatedEntity(t *testing.T) {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	defer cleanup()

	storyStorage := storage.NewStoryStorage(sqlStorage)
	generator := NewGenerator(storyStorage)
	ctx := context.Background()

	// Request without RelatedEntityID (no deduplication)
	req := CreateStoryRequest{
		CommunityID:      "community1",
		StoryType:        "STORY_TYPE_NEW_MEMBER_WELCOME",
		ParticipantIDs:   []string{"user1"},
		ParticipantNames: []string{"Eve"},
		CommunityName:    "Test Community",
		// No RelatedEntityID
	}

	story, err := generator.CreateStory(ctx, req)
	if err != nil {
		t.Fatalf("CreateStory failed: %v", err)
	}

	if story == nil {
		t.Fatal("Expected story to be created")
	}
}

func TestGenerator_CreateStory_AllStoryTypes(t *testing.T) {
	tests := []struct {
		name      string
		storyType string
	}{
		{"Loan Completed", "STORY_TYPE_LOAN_COMPLETED"},
		{"Giveaway Completed", "STORY_TYPE_GIVEAWAY_COMPLETED"},
		{"Event Concluded", "STORY_TYPE_EXPERIENCE_CONCLUDED"},
		{"Request Fulfilled", "STORY_TYPE_REQUEST_FULFILLED"},
		{"New Member Welcome", "STORY_TYPE_NEW_MEMBER_WELCOME"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqlStorage, cleanup := storage.SetupTestStorage(t)
			defer cleanup()

			storyStorage := storage.NewStoryStorage(sqlStorage)
			generator := NewGenerator(storyStorage)
			ctx := context.Background()

			req := CreateStoryRequest{
				CommunityID:       "community1",
				StoryType:         tt.storyType,
				ParticipantIDs:    []string{"user1", "user2"},
				ParticipantNames:  []string{"Alice", "Bob"},
				RelatedEntityID:   "entity123",
				RelatedEntityName: "test item",
				CommunityName:     "Test Community",
			}

			story, err := generator.CreateStory(ctx, req)
			if err != nil {
				t.Fatalf("CreateStory failed: %v", err)
			}

			if story.StoryType != tt.storyType {
				t.Errorf("Expected story type %s, got %s", tt.storyType, story.StoryType)
			}

			if story.Title == "" {
				t.Error("Expected title to be generated")
			}

			if story.Description == "" {
				t.Error("Expected description to be generated")
			}
		})
	}
}

func TestGenerateFromTemplate_LoanCompleted(t *testing.T) {
	req := CreateStoryRequest{
		StoryType:         "STORY_TYPE_LOAN_COMPLETED",
		ParticipantNames:  []string{"Alice", "Bob"},
		RelatedEntityName: "tent",
	}

	got := generateFromTemplate(req)
	title, description := got.Title, got.Description

	if title == "" {
		t.Error("Expected title to be generated")
	}

	if description == "" {
		t.Error("Expected description to be generated")
	}

	// Check that participant names and entity name appear in description
	if !contains(description, "Alice") {
		t.Error("Expected description to contain borrower name")
	}

	if !contains(description, "Bob") {
		t.Error("Expected description to contain lender name")
	}

	if !contains(description, "tent") {
		t.Error("Expected description to contain gear name")
	}

	// Phase 4b: structured template payload must also be populated.
	if got.TemplateKey != "story.loan_completed" {
		t.Errorf("TemplateKey = %q; want %q", got.TemplateKey, "story.loan_completed")
	}
	if got.Params["borrowerName"] != "Alice" || got.Params["gearName"] != "tent" || got.Params["lenderName"] != "Bob" {
		t.Errorf("Params = %v; want borrower=Alice, gear=tent, lender=Bob", got.Params)
	}
}

func TestGenerateFromTemplate_NewMemberWelcome(t *testing.T) {
	req := CreateStoryRequest{
		StoryType:        "STORY_TYPE_NEW_MEMBER_WELCOME",
		ParticipantNames: []string{"Charlie"},
		CommunityName:    "Downtown Sharing",
	}

	got := generateFromTemplate(req)
	title, description := got.Title, got.Description

	if title == "" {
		t.Error("Expected title to be generated")
	}

	if !contains(description, "Charlie") {
		t.Error("Expected description to contain new member name")
	}

	if !contains(description, "Downtown Sharing") {
		t.Error("Expected description to contain community name")
	}

	if got.TemplateKey != "story.new_member_welcome" {
		t.Errorf("TemplateKey = %q; want %q", got.TemplateKey, "story.new_member_welcome")
	}
	if got.Params["memberName"] != "Charlie" || got.Params["communityName"] != "Downtown Sharing" {
		t.Errorf("Params = %v; want member=Charlie, community=Downtown Sharing", got.Params)
	}
}

func TestFormatParticipantNames(t *testing.T) {
	tests := []struct {
		name     string
		names    []string
		expected string
	}{
		{"No names", []string{}, ""},
		{"One name", []string{"Alice"}, "Alice"},
		{"Two names", []string{"Alice", "Bob"}, "Alice and Bob"},
		{"Three names", []string{"Alice", "Bob", "Carol"}, "Alice, Bob, and Carol"},
		{"Four names", []string{"Alice", "Bob", "Carol", "Dave"}, "Alice, Bob, Carol, and Dave"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatParticipantNames(tt.names)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

// TestGenerateFromTemplate_EveryStoryTypeCarriesATemplateKey is the guard
// that keeps untranslated literal prose out of stories (#2936).
//
// A story renders client-side from template_key + params. A result with an
// empty key falls back to the literal English title/description, which is
// exactly the failure this issue removed. generateFromTemplate's default arm
// still emits no key, so any story type that reaches it ships English to
// every viewer regardless of locale — this test is what makes that visible
// when a new type is added.
//
// Both the full and the degraded ("simple") shape of each type are covered,
// because the participant-count branches pick different keys.
func TestGenerateFromTemplate_EveryStoryTypeCarriesATemplateKey(t *testing.T) {
	tests := []struct {
		name string
		req  CreateStoryRequest
	}{
		{"loan completed", CreateStoryRequest{
			StoryType:         ai.StoryTypeLoanCompleted,
			ParticipantNames:  []string{"Alice", "Bob"},
			RelatedEntityName: "tent",
		}},
		{"loan completed, one participant", CreateStoryRequest{
			StoryType:         ai.StoryTypeLoanCompleted,
			ParticipantNames:  []string{"Alice"},
			RelatedEntityName: "tent",
		}},
		{"giveaway completed", CreateStoryRequest{
			StoryType:         ai.StoryTypeGiveawayCompleted,
			ParticipantNames:  []string{"Alice", "Bob"},
			RelatedEntityName: "kayak",
		}},
		{"giveaway completed, one participant", CreateStoryRequest{
			StoryType:         ai.StoryTypeGiveawayCompleted,
			ParticipantNames:  []string{"Alice"},
			RelatedEntityName: "kayak",
		}},
		{"experience concluded", CreateStoryRequest{
			StoryType:         ai.StoryTypeExperienceConcluded,
			ParticipantNames:  []string{"Alice"},
			ParticipantIDs:    []string{"u1", "u2"},
			RelatedEntityName: "Trail day",
		}},
		{"experience concluded, no participants", CreateStoryRequest{
			StoryType:         ai.StoryTypeExperienceConcluded,
			RelatedEntityName: "Trail day",
		}},
		{"request fulfilled", CreateStoryRequest{
			StoryType:         ai.StoryTypeRequestFulfilled,
			ParticipantNames:  []string{"Alice", "Bob"},
			RelatedEntityName: "a ladder",
		}},
		{"request fulfilled, one participant", CreateStoryRequest{
			StoryType:         ai.StoryTypeRequestFulfilled,
			ParticipantNames:  []string{"Alice"},
			RelatedEntityName: "a ladder",
		}},
		{"new member welcome", CreateStoryRequest{
			StoryType:        ai.StoryTypeNewMemberWelcome,
			ParticipantNames: []string{"Charlie"},
			CommunityName:    "Downtown Sharing",
		}},
		{"new member welcome, no name", CreateStoryRequest{
			StoryType:     ai.StoryTypeNewMemberWelcome,
			CommunityName: "Downtown Sharing",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateFromTemplate(tt.req)
			if got.TemplateKey == "" {
				t.Errorf("TemplateKey is empty for %s — the client would render "+
					"the literal English description instead of resolving it in "+
					"the viewer's locale", tt.req.StoryType)
			}
			if got.Description == "" {
				t.Errorf("Description is empty for %s", tt.req.StoryType)
			}
			if got.Title == "" {
				t.Errorf("Title is empty for %s", tt.req.StoryType)
			}
		})
	}
}

// Helper function to check if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
