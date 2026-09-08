package impact_metrics

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/authn"
	"github.com/google/uuid"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/storage"
)

// setupTestService creates a Service backed by a real test database and the embedded estimator config.
func setupTestService(t *testing.T) (*Service, *storage.ProtoSQLStorage) {
	t.Helper()
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)

	cfg, err := estimator.LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed: %v", err)
	}

	svc := NewService(db, cfg)
	return svc, db
}

// contextWithAuth returns a context carrying the given user's auth info.
func contextWithAuth(userID, email string) context.Context {
	info := &auth.Info{
		UserID: userID,
		Email:  email,
		Role:   models.Role_ROLE_USER,
	}
	return authn.SetInfo(context.Background(), info)
}

// insertTestUser inserts a minimal User and returns its ID.
func insertTestUser(t *testing.T, db *storage.ProtoSQLStorage, name string) string {
	t.Helper()
	user := &models.User{
		Id:   uuid.New().String(),
		Name: name,
	}
	id, err := db.Insert(context.Background(), user)
	if err != nil {
		t.Fatalf("insertTestUser: %v", err)
	}
	return id
}

// insertTestCommunity inserts a Community and returns its ID.
func insertTestCommunity(t *testing.T, db *storage.ProtoSQLStorage, name string) string {
	t.Helper()
	community := &models.Community{
		Id:          uuid.New().String(),
		Name:        name,
		CreatorId:   "test-user",
		OwnerUserId: "test-user",
	}
	id, err := db.Insert(context.Background(), community)
	if err != nil {
		t.Fatalf("insertTestCommunity: %v", err)
	}
	return id
}

// insertTestMembership links a user to a community.
func insertTestMembership(t *testing.T, db *storage.ProtoSQLStorage, communityID, userID string) {
	t.Helper()
	membership := &models.CommunityUser{
		Id:          uuid.New().String(),
		CommunityId: communityID,
		UserId:      userID,
		InviterId:   userID,
	}
	if _, err := db.Insert(context.Background(), membership); err != nil {
		t.Fatalf("insertTestMembership: %v", err)
	}
}

// insertTestCompletedTransfer inserts a completed loan transfer linked to a community.
func insertTestCompletedTransfer(
	t *testing.T,
	db *storage.ProtoSQLStorage,
	communityID, ownerID, recipientID, gearID string,
	ie *models.ImpactEstimate,
) string {
	t.Helper()
	now := time.Now().Unix()
	pickup := now - 3600
	transfer := &models.Transfer{
		Id:                  uuid.New().String(),
		CommunityId:         communityID,
		OwnerId:             ownerID,
		RecipientId:         recipientID,
		GearId:              gearID,
		TransferType:        models.TransferType_TRANSFER_TYPE_LOAN,
		State:               models.TransferState_TRANSFER_STATE_COMPLETED,
		ActualPickupUnixSec: &pickup,
		ActualReturnUnixSec: &now,
		ImpactEstimate:      ie,
	}
	id, err := db.Insert(context.Background(), transfer)
	if err != nil {
		t.Fatalf("insertTestCompletedTransfer: %v", err)
	}
	return id
}

// insertTestFulfilledRequest inserts a fulfilled request linked to a community via CommunityRequest.
func insertTestFulfilledRequest(
	t *testing.T,
	db *storage.ProtoSQLStorage,
	communityID, requesterID, helperID string,
	ie *models.ImpactEstimate,
) string {
	t.Helper()
	ctx := context.Background()
	sharedAt := time.Now().Unix() - 7200
	fulfilledAt := time.Now().Unix()
	request := &models.Request{
		Id:                 uuid.New().String(),
		Title:              "Need a ladder",
		RequesterId:        requesterID,
		State:              models.RequestState_REQUEST_STATE_FULFILLED,
		FulfilledAtUnixSec: &fulfilledAt,
		ConfirmedHelperIds: []string{helperID},
		ImpactEstimate:     ie,
	}
	requestID, err := db.Insert(ctx, request)
	if err != nil {
		t.Fatalf("insertTestFulfilledRequest (request): %v", err)
	}
	cr := &models.CommunityRequest{
		Id:              uuid.New().String(),
		CommunityId:     communityID,
		RequestId:       requestID,
		SharedAtUnixSec: sharedAt,
	}
	if _, err := db.Insert(ctx, cr); err != nil {
		t.Fatalf("insertTestFulfilledRequest (community_request): %v", err)
	}
	return requestID
}

// testImpactEstimate builds a minimal ImpactEstimate for test data.
// A zero value for any dimension omits that field from the estimate.
func testImpactEstimate(costUSD, carbonGrams, timeMinutes float32) *models.ImpactEstimate {
	ie := &models.ImpactEstimate{}
	if costUSD > 0 {
		ie.MoneySaved = &models.MoneySavings{
			ValueUsd: &models.Estimate{Mean: costUSD, Stddev: costUSD * 0.3},
		}
	}
	if carbonGrams > 0 {
		ie.EmissionsPrevented = &models.PreventedEmissions{
			ManufactureAvoidedCarbon: &models.CarbonEstimate{
				Co2EGrams: &models.Estimate{Mean: carbonGrams, Stddev: carbonGrams * 0.4},
			},
		}
	}
	if timeMinutes > 0 {
		ie.TimeSaved = &models.TimeSavings{
			Minutes: &models.Estimate{Mean: timeMinutes, Stddev: timeMinutes * 0.3},
		}
	}
	return ie
}

// insertGearWithValue inserts a Gear with the given estimated value and returns its ID.
func insertGearWithValue(t *testing.T, db *storage.ProtoSQLStorage, ownerID, name string, valueUSD float32) string {
	t.Helper()
	gear := &models.Gear{
		Id:      uuid.New().String(),
		OwnerId: ownerID,
		Name:    name,
		ValueEstimate: &models.ValueEstimate{
			EstimatedValueUsd: valueUSD,
		},
	}
	id, err := db.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("insertGearWithValue: %v", err)
	}
	return id
}

// insertGearWithCategory inserts a Gear with a category set (for redundancy group tests).
func insertGearWithCategory(t *testing.T, db *storage.ProtoSQLStorage, ownerID, name, category string, valueUSD float32) string {
	t.Helper()
	gear := &models.Gear{
		Id:      uuid.New().String(),
		OwnerId: ownerID,
		Name:    name,
		Category: &models.TrackedString{
			Value: category,
		},
		ValueEstimate: &models.ValueEstimate{
			EstimatedValueUsd: valueUSD,
		},
	}
	id, err := db.Insert(context.Background(), gear)
	if err != nil {
		t.Fatalf("insertGearWithCategory: %v", err)
	}
	return id
}

// insertCommunityGearLink creates a CommunityGear junction record.
func insertCommunityGearLink(t *testing.T, db *storage.ProtoSQLStorage, communityID, gearID string) {
	t.Helper()
	cg := &models.CommunityGear{
		Id:          uuid.New().String(),
		CommunityId: communityID,
		GearId:      gearID,
	}
	if _, err := db.Insert(context.Background(), cg); err != nil {
		t.Fatalf("insertCommunityGearLink: %v", err)
	}
}

// insertExperienceForOwner inserts a completed Experience owned by ownerID with the given impact estimate.
func insertExperienceForOwner(t *testing.T, db *storage.ProtoSQLStorage, ownerID string, ie *models.ImpactEstimate) string {
	t.Helper()
	exp := &models.Experience{
		Id:             uuid.New().String(),
		OwnerId:        ownerID,
		Name:           "Test Experience",
		State:          models.ExperienceState_EXPERIENCE_STATE_COMPLETED,
		ImpactEstimate: ie,
	}
	id, err := db.Insert(context.Background(), exp)
	if err != nil {
		t.Fatalf("insertTestExperience: %v", err)
	}
	return id
}

// insertGiveawayTransfer inserts a completed giveaway transfer linked to a community.
func insertGiveawayTransfer(
	t *testing.T,
	db *storage.ProtoSQLStorage,
	communityID, ownerID, recipientID, gearID string,
	ie *models.ImpactEstimate,
) string {
	t.Helper()
	now := time.Now().Unix()
	pickup := now - 3600
	transfer := &models.Transfer{
		Id:                  uuid.New().String(),
		CommunityId:         communityID,
		OwnerId:             ownerID,
		RecipientId:         recipientID,
		GearId:              gearID,
		TransferType:        models.TransferType_TRANSFER_TYPE_GIVEAWAY,
		State:               models.TransferState_TRANSFER_STATE_COMPLETED,
		ActualPickupUnixSec: &pickup,
		ActualReturnUnixSec: &now,
		ImpactEstimate:      ie,
	}
	id, err := db.Insert(context.Background(), transfer)
	if err != nil {
		t.Fatalf("insertGiveawayTransfer: %v", err)
	}
	return id
}

// insertCompletedTransferAt inserts a completed loan transfer with an explicit return timestamp.
// Use this to place transfers in specific calendar months for trend tests.
func insertCompletedTransferAt(
	t *testing.T,
	db *storage.ProtoSQLStorage,
	communityID, ownerID, recipientID, gearID string,
	ie *models.ImpactEstimate,
	completedAtUnixSec int64,
) string {
	t.Helper()
	pickup := completedAtUnixSec - 3600
	transfer := &models.Transfer{
		Id:                  uuid.New().String(),
		CommunityId:         communityID,
		OwnerId:             ownerID,
		RecipientId:         recipientID,
		GearId:              gearID,
		TransferType:        models.TransferType_TRANSFER_TYPE_LOAN,
		State:               models.TransferState_TRANSFER_STATE_COMPLETED,
		ActualPickupUnixSec: &pickup,
		ActualReturnUnixSec: &completedAtUnixSec,
		ImpactEstimate:      ie,
	}
	id, err := db.Insert(context.Background(), transfer)
	if err != nil {
		t.Fatalf("insertCompletedTransferAt: %v", err)
	}
	return id
}

// testImpactEstimateWithQT builds an ImpactEstimate like testImpactEstimate but also populates
// QualityTimeEstimate.QualityTimeMinutes for tests that exercise the quality-time dimension.
func testImpactEstimateWithQT(costUSD, carbonGrams, timeMinutes, qtMinutes float32) *models.ImpactEstimate {
	ie := testImpactEstimate(costUSD, carbonGrams, timeMinutes)
	if qtMinutes > 0 {
		ie.QualityTime = &models.QualityTimeEstimate{
			QualityTimeMinutes: &models.Estimate{Mean: qtMinutes, Stddev: qtMinutes * 0.2},
		}
	}
	return ie
}
