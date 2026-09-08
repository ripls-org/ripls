package feed

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/authn"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

// createAuthenticatedContext creates a context with authentication info.
func createAuthenticatedContext(userID, email string, role models.Role) context.Context {
	authInfo := &auth.Info{
		UserID: userID,
		Email:  email,
		Role:   role,
	}
	return authn.SetInfo(context.Background(), authInfo)
}

// setupTestStorage creates a PostgreSQL database for testing.
func setupTestStorage(t *testing.T) *storage.ProtoSQLStorage {
	sqlStorage, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	return sqlStorage
}

// setupTestService creates a feed service for testing.
func setupTestService(sqlStorage *storage.ProtoSQLStorage) *Service {
	return New(sqlStorage)
}

// setupTestUser creates a test user in the database and returns the user ID.
func setupTestUser(t *testing.T, storage *storage.ProtoSQLStorage, email, name string) string {
	user := &models.User{
		Email: email,
		Name:  name,
		Role:  models.Role_ROLE_USER,
	}
	userID, err := storage.Insert(context.Background(), user)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	return userID
}

// setupTestCommunity creates a test community and returns the community ID.
func setupTestCommunity(t *testing.T, storage *storage.ProtoSQLStorage, creatorID, name string) string {
	community := &models.Community{
		Name:             name,
		Description:      "Test community",
		CreatorId:        creatorID,
		OwnerUserId:      creatorID,
		CreatedAtUnixSec: time.Now().Unix(),
		UpdatedAtUnixSec: time.Now().Unix(),
	}
	communityID, err := storage.Insert(context.Background(), community)
	if err != nil {
		t.Fatalf("Failed to create test community: %v", err)
	}

	addCommunityMember(t, storage, communityID, creatorID)

	return communityID
}

// addCommunityMember joins a user to a community so they can read its feed.
func addCommunityMember(t *testing.T, s *storage.ProtoSQLStorage, communityID, userID string) {
	t.Helper()
	membership := &models.CommunityUser{
		CommunityId:      communityID,
		UserId:           userID,
		InviterId:        userID,
		CreatedAtUnixSec: time.Now().Unix(),
	}
	if _, err := s.Insert(context.Background(), membership); err != nil {
		t.Fatalf("Failed to add user %s to community: %v", userID, err)
	}
}

// setupTestGear creates test gear and returns the gear ID.
func setupTestGear(t *testing.T, storage *storage.ProtoSQLStorage, userID, name string) string {
	gear := &models.Gear{
		Name:        name,
		Description: "Test gear",
		OwnerId:     userID,
		State:       models.GearState_GEAR_STATE_AVAILABLE,
	}
	gearID, err := storage.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("Failed to create test gear: %v", err)
	}
	return gearID
}

// createGearSharedEvent creates a gear sharing event.
func createGearSharedEvent(t *testing.T, storage *storage.ProtoSQLStorage, communityID, actorID, gearID string) string {
	event := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_GEAR_SHARED,
		ActorId:           actorID,
		GearId:            gearID,
		OccurredAtUnixSec: time.Now().Unix(),
	}
	eventID, err := storage.Insert(context.Background(), event)
	if err != nil {
		t.Fatalf("Failed to create gear shared event: %v", err)
	}
	return eventID
}

// setupCommunityGear creates a CommunityGear record (gear shared with community).
func setupCommunityGear(t *testing.T, storage *storage.ProtoSQLStorage, communityID, gearID string, availability models.Availability) string {
	communityGear := &models.CommunityGear{
		CommunityId:      communityID,
		GearId:           gearID,
		Availability:     availability,
		CreatedAtUnixSec: time.Now().Unix(),
	}
	communityGearID, err := storage.Insert(context.Background(), communityGear)
	if err != nil {
		t.Fatalf("Failed to create community gear: %v", err)
	}
	return communityGearID
}

// setupTestRequest creates a test request and returns the request ID.
func setupTestRequest(t *testing.T, storage *storage.ProtoSQLStorage, requesterID, communityID, description string, state models.RequestState) string {
	// Wanted a week out, so the request is a live opportunity by default —
	// matching what most feed tests are exercising. Tests that need a stale
	// request set NeededByUnixSec themselves.
	neededBy := time.Now().Unix() + 7*24*3600
	request := &models.Request{
		RequesterId:     requesterID,
		Description:     description,
		State:           state,
		NeededByUnixSec: &neededBy,
	}
	requestID, err := storage.Insert(context.Background(), request)
	if err != nil {
		t.Fatalf("Failed to create test request: %v", err)
	}

	// Link request to community
	_, err = storage.Insert(context.Background(), &models.CommunityRequest{
		RequestId:   requestID,
		CommunityId: communityID,
	})
	if err != nil {
		t.Fatalf("Failed to link request to community: %v", err)
	}

	return requestID
}

// createRequestCreatedEvent creates a request creation event.
func createRequestCreatedEvent(t *testing.T, storage *storage.ProtoSQLStorage, communityID, actorID, requestID string) string {
	event := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_REQUEST_CREATED,
		ActorId:           actorID,
		Topic:             &models.CommunityEvent_RequestId{RequestId: requestID},
		OccurredAtUnixSec: time.Now().Unix(),
	}
	eventID, err := storage.Insert(context.Background(), event)
	if err != nil {
		t.Fatalf("Failed to create request created event: %v", err)
	}
	return eventID
}

// setupTestExperience creates test experience and returns the experience ID.
func setupTestExperience(t *testing.T, storage *storage.ProtoSQLStorage, userID, name string) string {
	return setupTestExperienceAt(t, storage, userID, name, time.Now().Unix()+7*24*3600)
}

// setupTestExperienceAt creates a test experience scheduled at startUnixSec.
// Experiences default to a week out, so they read as live opportunities;
// pass a past timestamp to build the stale case the horizon must drop.
func setupTestExperienceAt(t *testing.T, storage *storage.ProtoSQLStorage, userID, name string, startUnixSec int64) string {
	experience := &models.Experience{
		Name:        name,
		Description: "Test experience",
		OwnerId:     userID,
		State:       models.ExperienceState_EXPERIENCE_STATE_ACTIVE,
		Time: &models.ExperienceTime{
			TimeType: &models.ExperienceTime_Specific{
				Specific: &models.SpecificTime{UnixTimestampSec: startUnixSec},
			},
		},
	}
	experienceID, err := storage.Insert(context.Background(), experience)
	if err != nil {
		t.Fatalf("Failed to create test experience: %v", err)
	}
	return experienceID
}

// setupCommunityExperience creates a CommunityExperience record (experience shared with community).
func setupCommunityExperience(t *testing.T, storage *storage.ProtoSQLStorage, communityID, experienceID string, archived bool) string {
	communityExperience := &models.CommunityExperience{
		CommunityId:     communityID,
		ExperienceId:    experienceID,
		SharedAtUnixSec: time.Now().Unix(),
		Archived:        archived,
	}
	communityExperienceID, err := storage.Insert(context.Background(), communityExperience)
	if err != nil {
		t.Fatalf("Failed to create community experience: %v", err)
	}
	return communityExperienceID
}

// createExperienceCreatedEvent creates an experience creation event.
func createExperienceCreatedEvent(t *testing.T, storage *storage.ProtoSQLStorage, communityID, actorID, experienceID string) string {
	event := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         models.CommunityEventType_COMMUNITY_EVENT_TYPE_EXPERIENCE_CREATED,
		ActorId:           actorID,
		Topic:             &models.CommunityEvent_ExperienceId{ExperienceId: experienceID},
		OccurredAtUnixSec: time.Now().Unix(),
	}
	eventID, err := storage.Insert(context.Background(), event)
	if err != nil {
		t.Fatalf("Failed to create experience created event: %v", err)
	}
	return eventID
}

// --- Phase 2: lifecycle event helpers ---.

func createTransferEvent(t *testing.T, s *storage.ProtoSQLStorage, communityID, actorID, gearID string, eventType models.CommunityEventType) string {
	t.Helper()
	event := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         eventType,
		ActorId:           actorID,
		GearId:            gearID,
		OccurredAtUnixSec: time.Now().Unix(),
	}
	id, err := s.Insert(context.Background(), event)
	if err != nil {
		t.Fatalf("createTransferEvent failed: %v", err)
	}
	return id
}

func createRequestLifecycleEvent(t *testing.T, s *storage.ProtoSQLStorage, communityID, actorID, requestID string, eventType models.CommunityEventType) string {
	t.Helper()
	event := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         eventType,
		ActorId:           actorID,
		Topic:             &models.CommunityEvent_RequestId{RequestId: requestID},
		OccurredAtUnixSec: time.Now().Unix(),
	}
	id, err := s.Insert(context.Background(), event)
	if err != nil {
		t.Fatalf("createRequestLifecycleEvent failed: %v", err)
	}
	return id
}

func createExperienceRSVPEvent(t *testing.T, s *storage.ProtoSQLStorage, communityID, actorID, experienceID string, eventType models.CommunityEventType) string {
	t.Helper()
	event := &models.CommunityEvent{
		CommunityId:       communityID,
		EventType:         eventType,
		ActorId:           actorID,
		Topic:             &models.CommunityEvent_ExperienceId{ExperienceId: experienceID},
		OccurredAtUnixSec: time.Now().Unix(),
	}
	id, err := s.Insert(context.Background(), event)
	if err != nil {
		t.Fatalf("createExperienceRSVPEvent failed: %v", err)
	}
	return id
}
