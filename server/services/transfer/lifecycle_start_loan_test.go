package transfer

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

func TestService_StartLoan_MarksGearUnavailable(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	// Create available gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ctx := context.Background()
	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	// Express interest
	req := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})
	_, err := service.ExpressInterest(recipientCtx, req)
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Get transfer (already auto-approved for loans)
	transfers, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("Failed to query transfers: %v", err)
	}
	transfer := transfers[0].(*models.Transfer)

	// Verify transfer is auto-approved
	if transfer.State != models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Fatalf("Expected auto-approved RECIPIENT_SELECTED state, got %v", transfer.State)
	}

	// Start the loan
	startReq := connect.NewRequest(&api.StartLoanRequest{
		TransferId: transfer.Id,
	})

	_, err = service.StartLoan(ownerCtx, startReq)
	if err != nil {
		t.Fatalf("StartLoan failed: %v", err)
	}

	// Verify transfer state
	err = testStorage.GetByID(ctx, transfer.Id, transfer)
	if err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}

	if transfer.State != models.TransferState_TRANSFER_STATE_ACTIVE {
		t.Errorf("Expected ACTIVE state, got %v", transfer.State)
	}

	// Verify ImpactEstimate was persisted at StartLoan
	if transfer.ImpactEstimate == nil {
		t.Error("Expected ImpactEstimate to be set after StartLoan")
	} else if transfer.ImpactEstimate.TimeSaved == nil || transfer.ImpactEstimate.TimeSaved.Minutes == nil {
		t.Error("Expected TimeSaved.Minutes to be set after StartLoan")
	}

	// Verify provenance on TimeSaved
	if ts := transfer.ImpactEstimate.GetTimeSaved(); ts != nil {
		if ts.Provenance == nil {
			t.Error("Expected TimeSaved.Provenance to be set after StartLoan")
		} else {
			if ts.Provenance.Source != models.ProvenanceSource_PROVENANCE_SOURCE_CONFIG_DEFAULT {
				t.Errorf("Expected TimeSaved source CONFIG_DEFAULT, got %v", ts.Provenance.Source)
			}
			if ts.Provenance.Name == "" {
				t.Error("Expected TimeSaved provenance name to be non-empty")
			}
			if ts.Provenance.Version <= 0 {
				t.Errorf("Expected TimeSaved provenance version > 0, got %d", ts.Provenance.Version)
			}
		}
	}

	// Verify gear is now unavailable
	gear := &models.Gear{}
	err = testStorage.GetByID(ctx, gearID, gear)
	if err != nil {
		t.Fatalf("Failed to retrieve gear: %v", err)
	}

	if gear.State != models.GearState_GEAR_STATE_UNAVAILABLE {
		t.Errorf("Expected gear state UNAVAILABLE, got %v", gear.State)
	}
}

func TestService_StartLoan_RecipientCanMarkPickedUp(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	// Create available gear
	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)

	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ctx := context.Background()
	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)

	// Express interest
	req := connect.NewRequest(&api.ExpressInterestRequest{
		GearId: gearID,
	})
	_, err := service.ExpressInterest(recipientCtx, req)
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Get transfer (already auto-approved for loans)
	transfers, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("Failed to query transfers: %v", err)
	}
	transfer := transfers[0].(*models.Transfer)

	// Verify transfer is auto-approved
	if transfer.State != models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Fatalf("Expected auto-approved RECIPIENT_SELECTED state, got %v", transfer.State)
	}

	// Recipient marks as picked up (not owner)
	startReq := connect.NewRequest(&api.StartLoanRequest{
		TransferId: transfer.Id,
	})

	_, err = service.StartLoan(recipientCtx, startReq)
	if err != nil {
		t.Fatalf("StartLoan by recipient failed: %v", err)
	}

	// Verify transfer state
	err = testStorage.GetByID(ctx, transfer.Id, transfer)
	if err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}

	if transfer.State != models.TransferState_TRANSFER_STATE_ACTIVE {
		t.Errorf("Expected ACTIVE state, got %v", transfer.State)
	}
}

func TestService_StartLoan_RecordsActualPickupTime(t *testing.T) {
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ctx := context.Background()
	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	// Express interest
	req := connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})
	_, err := service.ExpressInterest(recipientCtx, req)
	if err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	// Get transfer (already auto-approved for loans)
	transfers, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("Failed to query transfers: %v", err)
	}
	transfer := transfers[0].(*models.Transfer)

	// Verify transfer is auto-approved
	if transfer.State != models.TransferState_TRANSFER_STATE_RECIPIENT_SELECTED {
		t.Fatalf("Expected auto-approved RECIPIENT_SELECTED state, got %v", transfer.State)
	}

	// Verify no actual pickup time before StartLoan
	err = testStorage.GetByID(ctx, transfer.Id, transfer)
	if err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}
	if transfer.ActualPickupUnixSec != nil {
		t.Errorf("Expected no actual pickup time before StartLoan, got %d", *transfer.ActualPickupUnixSec)
	}

	// Start loan
	startReq := connect.NewRequest(&api.StartLoanRequest{TransferId: transfer.Id})
	_, err = service.StartLoan(ownerCtx, startReq)
	if err != nil {
		t.Fatalf("StartLoan failed: %v", err)
	}

	// Verify actual pickup time was recorded
	err = testStorage.GetByID(ctx, transfer.Id, transfer)
	if err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}

	if transfer.ActualPickupUnixSec == nil {
		t.Error("Expected actual_pickup_unix_sec to be set after StartLoan")
	}
}

func TestService_StartLoan_DerivesExpectedReturnFromDuration(t *testing.T) {
	// On the ACTIVE transition, a loan with loan_duration_days set must
	// have expected_return_unix_sec computed as actual_pickup +
	// duration*86400. This is the indexed column the
	// scheduled-notification reconciler scans for return reminders
	// (#625).
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ctx := context.Background()
	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})
	if _, err := service.ExpressInterest(recipientCtx, req); err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	transfers, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("query transfers: %v", err)
	}
	transfer := transfers[0].(*models.Transfer)

	// Set loan_duration_days before starting the loan.
	loanDays := int32(7)
	updateReq := connect.NewRequest(&api.UpdateTransferRequest{
		TransferId:       transfer.Id,
		LoanDurationDays: &loanDays,
	})
	if _, err := service.UpdateTransfer(ownerCtx, updateReq); err != nil {
		t.Fatalf("UpdateTransfer: %v", err)
	}

	startReq := connect.NewRequest(&api.StartLoanRequest{TransferId: transfer.Id})
	if _, err := service.StartLoan(ownerCtx, startReq); err != nil {
		t.Fatalf("StartLoan: %v", err)
	}

	if err := testStorage.GetByID(ctx, transfer.Id, transfer); err != nil {
		t.Fatalf("reload transfer: %v", err)
	}
	if transfer.ActualPickupUnixSec == nil {
		t.Fatal("expected actual_pickup_unix_sec to be set after StartLoan")
	}
	if transfer.ExpectedReturnUnixSec == nil {
		t.Fatal("expected expected_return_unix_sec to be set after StartLoan with a loan duration")
	}
	want := *transfer.ActualPickupUnixSec + int64(loanDays)*86400
	if *transfer.ExpectedReturnUnixSec != want {
		t.Errorf("expected_return_unix_sec = %d, want %d (pickup + %dd)",
			*transfer.ExpectedReturnUnixSec, want, int(loanDays))
	}
}

func TestService_StartLoan_NoExpectedReturnWithoutDuration(t *testing.T) {
	// A loan that starts without loan_duration_days set leaves
	// expected_return_unix_sec unset. The reconciler's partial index
	// excludes these rows; they simply don't get return reminders.
	service, testStorage, _, done := setupTestServiceWithNotifications(t)

	ownerID := setupTestUser(t, testStorage, "Owner", "owner@example.com")
	recipientID := setupTestUser(t, testStorage, "Recipient", "recipient@example.com")

	gearID := setupTestGear(t, testStorage, ownerID, models.GearState_GEAR_STATE_AVAILABLE)
	setupCommunityAndShareGear(t, testStorage, ownerID, recipientID, gearID, models.Availability_AVAILABILITY_FOR_LOAN)

	ctx := context.Background()
	recipientCtx := createAuthenticatedContext(recipientID, "recipient@example.com", models.Role_ROLE_USER)
	ownerCtx := createAuthenticatedContext(ownerID, "owner@example.com", models.Role_ROLE_USER)

	req := connect.NewRequest(&api.ExpressInterestRequest{GearId: gearID})
	if _, err := service.ExpressInterest(recipientCtx, req); err != nil {
		t.Fatalf("ExpressInterest failed: %v", err)
	}
	services.WaitForNotification(t, done)

	transfers, err := testStorage.QueryByField(ctx, "gear_id", gearID, &models.Transfer{})
	if err != nil {
		t.Fatalf("query transfers: %v", err)
	}
	transfer := transfers[0].(*models.Transfer)

	startReq := connect.NewRequest(&api.StartLoanRequest{TransferId: transfer.Id})
	if _, err := service.StartLoan(ownerCtx, startReq); err != nil {
		t.Fatalf("StartLoan: %v", err)
	}

	if err := testStorage.GetByID(ctx, transfer.Id, transfer); err != nil {
		t.Fatalf("reload transfer: %v", err)
	}
	if transfer.ExpectedReturnUnixSec != nil {
		t.Errorf("expected expected_return_unix_sec to be unset without a duration, got %d", *transfer.ExpectedReturnUnixSec)
	}
}
