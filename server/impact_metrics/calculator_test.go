package impact_metrics

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/google/uuid"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/storage"
)

// setupTestDatabase creates a test database using the storage package helper.
func setupTestDatabase(t *testing.T) *storage.ProtoSQLStorage {
	db, cleanup := storage.SetupTestStorage(t)
	t.Cleanup(cleanup)
	return db
}

// createTestUser is a helper to create a test user.
func createTestUser(t *testing.T, db *storage.ProtoSQLStorage, name string) *models.User {
	user := &models.User{
		Id:   uuid.New().String(),
		Name: name,
	}
	if _, err := db.Insert(context.Background(), user); err != nil {
		t.Fatalf("Failed to write user: %v", err)
	}
	return user
}

// loadTestEstimatorConfig loads the embedded estimator config for tests.
func loadTestEstimatorConfig(t *testing.T) *estimator.Config {
	t.Helper()
	cfg, err := estimator.LoadConfigFromEmbed()
	if err != nil {
		t.Fatalf("LoadConfigFromEmbed() error: %v", err)
	}
	return cfg
}

// createTestCommunity creates a test community and returns its ID.
func createTestCommunity(t *testing.T, db *storage.ProtoSQLStorage) string {
	t.Helper()
	communityID := uuid.New().String()
	community := &models.Community{
		Id:          communityID,
		Name:        "Test Community",
		CreatorId:   "test-user",
		OwnerUserId: "test-user",
	}
	if _, err := db.Insert(context.Background(), community); err != nil {
		t.Fatalf("Failed to insert community: %v", err)
	}
	return communityID
}

// testImpactEstimate creates a models.ImpactEstimate for testing.
// costUSD=0 omits MoneySaved, carbonGrams=0 omits EmissionsPrevented, timeMinutes=0 omits TimeSaved.
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

// insertTransfer inserts a transfer with optional ImpactEstimate and soft-delete.
func insertTransfer(t *testing.T, db *storage.ProtoSQLStorage, communityID string,
	transferType models.TransferType, state models.TransferState,
	ie *models.ImpactEstimate, deleted bool,
) {
	t.Helper()
	transfer := &models.Transfer{
		Id:             uuid.New().String(),
		CommunityId:    communityID,
		TransferType:   transferType,
		State:          state,
		ImpactEstimate: ie,
	}
	if deleted {
		transfer.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: time.Now().Unix()}
	}
	if _, err := db.Insert(context.Background(), transfer); err != nil {
		t.Fatalf("Failed to insert transfer: %v", err)
	}
}

// insertFulfilledRequest inserts a fulfilled request linked to a community.
func insertFulfilledRequest(t *testing.T, db *storage.ProtoSQLStorage, communityID string, ie *models.ImpactEstimate) {
	t.Helper()
	ctx := context.Background()
	request := &models.Request{
		Id:             uuid.New().String(),
		State:          models.RequestState_REQUEST_STATE_FULFILLED,
		ImpactEstimate: ie,
	}
	if _, err := db.Insert(ctx, request); err != nil {
		t.Fatalf("Failed to insert request: %v", err)
	}
	cr := &models.CommunityRequest{
		Id:          uuid.New().String(),
		CommunityId: communityID,
		RequestId:   request.Id,
		Archived:    true, // Fulfilled requests are always archived by MarkRequestFulfilled.
	}
	if _, err := db.Insert(ctx, cr); err != nil {
		t.Fatalf("Failed to insert community request: %v", err)
	}
}

// insertCompletedExperience inserts a completed experience linked to a community.
func insertCompletedExperience(t *testing.T, db *storage.ProtoSQLStorage, communityID string, ie *models.ImpactEstimate) {
	t.Helper()
	ctx := context.Background()
	experience := &models.Experience{
		Id:             uuid.New().String(),
		State:          models.ExperienceState_EXPERIENCE_STATE_COMPLETED,
		ImpactEstimate: ie,
	}
	if _, err := db.Insert(ctx, experience); err != nil {
		t.Fatalf("Failed to insert experience: %v", err)
	}
	ce := &models.CommunityExperience{
		Id:           uuid.New().String(),
		CommunityId:  communityID,
		ExperienceId: experience.Id,
	}
	if _, err := db.Insert(ctx, ce); err != nil {
		t.Fatalf("Failed to insert community experience: %v", err)
	}
}

// TestCalculateUserImpactFromPreloadedData tests the in-memory per-user impact calculation.
func TestCalculateUserImpactFromPreloadedData(t *testing.T) {
	const userA = "user-a"
	const userB = "user-b"

	makeTransfer := func(ownerID, recipientID string, state models.TransferState, costUSD float32, deleted bool) *models.Transfer {
		t := &models.Transfer{
			OwnerId:        ownerID,
			RecipientId:    recipientID,
			State:          state,
			ImpactEstimate: testImpactEstimate(costUSD, 0, 0),
		}
		if deleted {
			t.Deleted = &models.DeletedMetadata{DeletedAtUnixSec: 1}
		}
		return t
	}

	makeRequest := func(requesterID string, helpers []string, state models.RequestState, costUSD float32) *models.Request {
		return &models.Request{
			Id:                 "req-" + requesterID,
			RequesterId:        requesterID,
			ConfirmedHelperIds: helpers,
			State:              state,
			ImpactEstimate:     testImpactEstimate(costUSD, 0, 0),
		}
	}

	dim := api.ImpactMetricDimension_IMPACT_METRIC_DIMENSION_MONEY

	t.Run("no data returns zero", func(t *testing.T) {
		got := CalculateUserImpactFromPreloadedData(userA, nil, nil, dim)
		if got != 0 {
			t.Errorf("got %v, want 0", got)
		}
	})

	t.Run("completed transfer as owner counted", func(t *testing.T) {
		transfers := []*models.Transfer{
			makeTransfer(userA, userB, models.TransferState_TRANSFER_STATE_COMPLETED, 100, false),
		}
		got := CalculateUserImpactFromPreloadedData(userA, transfers, nil, dim)
		if math.Abs(got-100) > 0.01 {
			t.Errorf("got %v, want 100", got)
		}
	})

	t.Run("completed transfer as recipient counted", func(t *testing.T) {
		transfers := []*models.Transfer{
			makeTransfer(userB, userA, models.TransferState_TRANSFER_STATE_COMPLETED, 75, false),
		}
		got := CalculateUserImpactFromPreloadedData(userA, transfers, nil, dim)
		if math.Abs(got-75) > 0.01 {
			t.Errorf("got %v, want 75", got)
		}
	})

	t.Run("non-participant transfer excluded", func(t *testing.T) {
		transfers := []*models.Transfer{
			makeTransfer("other-owner", "other-recipient", models.TransferState_TRANSFER_STATE_COMPLETED, 200, false),
		}
		got := CalculateUserImpactFromPreloadedData(userA, transfers, nil, dim)
		if got != 0 {
			t.Errorf("got %v, want 0 (not a participant)", got)
		}
	})

	t.Run("active transfer excluded", func(t *testing.T) {
		transfers := []*models.Transfer{
			makeTransfer(userA, userB, models.TransferState_TRANSFER_STATE_ACTIVE, 100, false),
		}
		got := CalculateUserImpactFromPreloadedData(userA, transfers, nil, dim)
		if got != 0 {
			t.Errorf("got %v, want 0 (not completed)", got)
		}
	})

	t.Run("soft-deleted completed transfer excluded", func(t *testing.T) {
		transfers := []*models.Transfer{
			makeTransfer(userA, userB, models.TransferState_TRANSFER_STATE_COMPLETED, 100, true),
		}
		got := CalculateUserImpactFromPreloadedData(userA, transfers, nil, dim)
		if got != 0 {
			t.Errorf("got %v, want 0 (deleted)", got)
		}
	})

	t.Run("fulfilled request as requester counted", func(t *testing.T) {
		requestMap := map[string]*models.Request{
			"req-a": makeRequest(userA, nil, models.RequestState_REQUEST_STATE_FULFILLED, 50),
		}
		got := CalculateUserImpactFromPreloadedData(userA, nil, requestMap, dim)
		if math.Abs(got-50) > 0.01 {
			t.Errorf("got %v, want 50", got)
		}
	})

	t.Run("fulfilled request as confirmed helper counted", func(t *testing.T) {
		requestMap := map[string]*models.Request{
			"req-b": makeRequest(userB, []string{userA}, models.RequestState_REQUEST_STATE_FULFILLED, 60),
		}
		got := CalculateUserImpactFromPreloadedData(userA, nil, requestMap, dim)
		if math.Abs(got-60) > 0.01 {
			t.Errorf("got %v, want 60", got)
		}
	})

	t.Run("unfulfilled request excluded", func(t *testing.T) {
		requestMap := map[string]*models.Request{
			"req-a": makeRequest(userA, nil, models.RequestState_REQUEST_STATE_ACTIVE, 80),
		}
		got := CalculateUserImpactFromPreloadedData(userA, nil, requestMap, dim)
		if got != 0 {
			t.Errorf("got %v, want 0 (not fulfilled)", got)
		}
	})

	t.Run("transfers and requests summed", func(t *testing.T) {
		transfers := []*models.Transfer{
			makeTransfer(userA, userB, models.TransferState_TRANSFER_STATE_COMPLETED, 100, false),
		}
		requestMap := map[string]*models.Request{
			"req-a": makeRequest(userA, nil, models.RequestState_REQUEST_STATE_FULFILLED, 50),
		}
		got := CalculateUserImpactFromPreloadedData(userA, transfers, requestMap, dim)
		if math.Abs(got-150) > 0.01 {
			t.Errorf("got %v, want 150", got)
		}
	})
}

// assertEstimateZero checks that an Estimate has zero mean and stddev.
func assertEstimateZero(t *testing.T, name string, e *api.Estimate) {
	t.Helper()
	if e == nil {
		return // nil is treated as zero
	}
	if e.Mean != 0 {
		t.Errorf("%s.Mean = %v, want 0", name, e.Mean)
	}
}

// assertEstimateMean checks that an Estimate has the expected mean.
func assertEstimateMean(t *testing.T, name string, e *api.Estimate, wantMean float32) {
	t.Helper()
	if e == nil {
		t.Fatalf("%s is nil, want mean=%v", name, wantMean)
	}
	if math.Abs(float64(e.Mean-wantMean)) > 0.01 {
		t.Errorf("%s.Mean = %v, want %v", name, e.Mean, wantMean)
	}
}
