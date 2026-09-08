package user

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/storage"
)

func TestService_GetUserStats(t *testing.T) {
	tests := []struct {
		name           string
		setupData      func(ctx context.Context, str interface{}, userID string) error
		userID         string
		wantErr        bool
		wantCode       connect.Code
		validateResult func(t *testing.T, resp *api.GetUserStatsResponse)
	}{
		{
			name:    "new user with no activity",
			userID:  "", // Will be set by test
			wantErr: false,
			setupData: func(ctx context.Context, str interface{}, userID string) error {
				// User already exists (created by test setup), no additional data needed
				return nil
			},
			validateResult: func(t *testing.T, resp *api.GetUserStatsResponse) {
				if resp.CommunityCount != 0 {
					t.Errorf("Expected 0 communities, got %d", resp.CommunityCount)
				}
				if resp.ItemCount != 0 {
					t.Errorf("Expected 0 items, got %d", resp.ItemCount)
				}
				if resp.LoansCount != 0 {
					t.Errorf("Expected 0 loans, got %d", resp.LoansCount)
				}
				if resp.BorrowsCount != 0 {
					t.Errorf("Expected 0 borrows, got %d", resp.BorrowsCount)
				}
				if resp.Savings == nil {
					t.Fatal("Expected savings object, got nil")
				}
				if resp.Savings.CostSavedUsd != 0 {
					t.Errorf("Expected 0 cost saved, got %f", resp.Savings.CostSavedUsd)
				}
			},
		},
		{
			name:   "user with communities",
			userID: "",
			setupData: func(ctx context.Context, str interface{}, userID string) error {
				sqlStorage := str.(*storage.ProtoSQLStorage)
				// Create community memberships
				memberships := []models.CommunityUser{
					{UserId: userID, CommunityId: "comm-1"},
					{UserId: userID, CommunityId: "comm-2"},
					{UserId: userID, CommunityId: "comm-3"},
				}
				for i := range memberships {
					if _, err := sqlStorage.Insert(ctx, &memberships[i]); err != nil {
						return err
					}
				}
				return nil
			},
			validateResult: func(t *testing.T, resp *api.GetUserStatsResponse) {
				if resp.CommunityCount != 3 {
					t.Errorf("Expected 3 communities, got %d", resp.CommunityCount)
				}
			},
		},
		{
			name:   "user with gear items",
			userID: "",
			setupData: func(ctx context.Context, str interface{}, userID string) error {
				sqlStorage := str.(*storage.ProtoSQLStorage)
				// Create gear items
				gear := []models.Gear{
					{OwnerId: userID, Name: "Tent"},
					{OwnerId: userID, Name: "Bike"},
				}
				for i := range gear {
					if _, err := sqlStorage.Insert(ctx, &gear[i]); err != nil {
						return err
					}
				}
				return nil
			},
			validateResult: func(t *testing.T, resp *api.GetUserStatsResponse) {
				if resp.ItemCount != 2 {
					t.Errorf("Expected 2 items, got %d", resp.ItemCount)
				}
			},
		},
		{
			name:   "user with transfers as owner",
			userID: "",
			setupData: func(ctx context.Context, str interface{}, userID string) error {
				sqlStorage := str.(*storage.ProtoSQLStorage)
				// Create another user as recipient
				recipientUser := &models.User{
					Email: "recipient@example.com",
					Name:  "Recipient User",
					Role:  models.Role_ROLE_USER,
				}
				recipientID, err := sqlStorage.Insert(ctx, recipientUser)
				if err != nil {
					return err
				}

				// Create transfers where user is owner (lender)
				transfers := []models.Transfer{
					{
						OwnerId:      userID,
						RecipientId:  recipientID,
						GearId:       "gear-1",
						TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
					},
					{
						OwnerId:      userID,
						RecipientId:  recipientID,
						GearId:       "gear-2",
						TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
					},
					{
						OwnerId:      userID,
						RecipientId:  recipientID,
						GearId:       "gear-3",
						TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
					},
				}
				for i := range transfers {
					if _, err := sqlStorage.Insert(ctx, &transfers[i]); err != nil {
						return err
					}
				}
				return nil
			},
			validateResult: func(t *testing.T, resp *api.GetUserStatsResponse) {
				if resp.LoansCount != 3 {
					t.Errorf("Expected 3 loans, got %d", resp.LoansCount)
				}
				// Transfers are not completed and have no IE, so savings should be zero
				if resp.Savings.TimeSavedHours != 0 {
					t.Errorf("Expected 0 hours saved (no completed transfers), got %d", resp.Savings.TimeSavedHours)
				}
			},
		},
		{
			name:   "user with borrows",
			userID: "",
			setupData: func(ctx context.Context, str interface{}, userID string) error {
				sqlStorage := str.(*storage.ProtoSQLStorage)
				// Create another user as owner
				ownerUser := &models.User{
					Email: "owner@example.com",
					Name:  "Owner User",
					Role:  models.Role_ROLE_USER,
				}
				ownerID, err := sqlStorage.Insert(ctx, ownerUser)
				if err != nil {
					return err
				}

				// Create transfers where user is recipient (borrower)
				transfers := []models.Transfer{
					{
						OwnerId:      ownerID,
						RecipientId:  userID,
						GearId:       "gear-1",
						TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
					},
					{
						OwnerId:      ownerID,
						RecipientId:  userID,
						GearId:       "gear-2",
						TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
					},
				}
				for i := range transfers {
					if _, err := sqlStorage.Insert(ctx, &transfers[i]); err != nil {
						return err
					}
				}
				return nil
			},
			validateResult: func(t *testing.T, resp *api.GetUserStatsResponse) {
				if resp.BorrowsCount != 2 {
					t.Errorf("Expected 2 borrows, got %d", resp.BorrowsCount)
				}
			},
		},
		{
			name:   "user with giveaways",
			userID: "",
			setupData: func(ctx context.Context, str interface{}, userID string) error {
				sqlStorage := str.(*storage.ProtoSQLStorage)
				// Create another user as recipient
				recipientUser := &models.User{
					Email: "giveaway-recipient@example.com",
					Name:  "Giveaway Recipient",
					Role:  models.Role_ROLE_USER,
				}
				recipientID, err := sqlStorage.Insert(ctx, recipientUser)
				if err != nil {
					return err
				}

				// Create giveaway transfers
				giveaways := []models.Transfer{
					{
						OwnerId:      userID,
						RecipientId:  recipientID,
						GearId:       "gear-1",
						TransferType: models.TransferType_TRANSFER_TYPE_GIVEAWAY,
					},
					{
						OwnerId:      userID,
						RecipientId:  recipientID,
						GearId:       "gear-2",
						TransferType: models.TransferType_TRANSFER_TYPE_GIVEAWAY,
					},
				}
				for i := range giveaways {
					if _, err := sqlStorage.Insert(ctx, &giveaways[i]); err != nil {
						return err
					}
				}

				// Create a loan (should not count as giveaway)
				loan := &models.Transfer{
					OwnerId:      userID,
					RecipientId:  recipientID,
					GearId:       "gear-3",
					TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
				}
				if _, err := sqlStorage.Insert(ctx, loan); err != nil {
					return err
				}

				return nil
			},
			validateResult: func(t *testing.T, resp *api.GetUserStatsResponse) {
				if resp.GiveawaysCount != 2 {
					t.Errorf("Expected 2 giveaways, got %d", resp.GiveawaysCount)
				}
				if resp.LoansCount != 1 {
					t.Errorf("Expected 1 loan, got %d", resp.LoansCount)
				}
			},
		},
		{
			name:   "comprehensive user with all activity types",
			userID: "",
			setupData: func(ctx context.Context, str interface{}, userID string) error {
				sqlStorage := str.(*storage.ProtoSQLStorage)

				// Communities
				membership := &models.CommunityUser{
					UserId:      userID,
					CommunityId: "comm-1",
				}
				if _, err := sqlStorage.Insert(ctx, membership); err != nil {
					return err
				}

				// Gear
				gear := []models.Gear{
					{OwnerId: userID, Name: "Drill"},
					{OwnerId: userID, Name: "Ladder"},
				}
				for i := range gear {
					if _, err := sqlStorage.Insert(ctx, &gear[i]); err != nil {
						return err
					}
				}

				// Create recipient user
				recipientUser := &models.User{
					Email: "comprehensive-recipient@example.com",
					Name:  "Comprehensive Recipient",
					Role:  models.Role_ROLE_USER,
				}
				recipientID, err := sqlStorage.Insert(ctx, recipientUser)
				if err != nil {
					return err
				}

				// Loans as owner
				loans := []models.Transfer{
					{
						OwnerId:      userID,
						RecipientId:  recipientID,
						GearId:       "gear-1",
						TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
					},
					{
						OwnerId:      userID,
						RecipientId:  recipientID,
						GearId:       "gear-2",
						TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
					},
				}
				for i := range loans {
					if _, err := sqlStorage.Insert(ctx, &loans[i]); err != nil {
						return err
					}
				}

				// Borrow as recipient
				borrow := &models.Transfer{
					OwnerId:      recipientID,
					RecipientId:  userID,
					GearId:       "other-gear-1",
					TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
				}
				if _, err := sqlStorage.Insert(ctx, borrow); err != nil {
					return err
				}

				return nil
			},
			validateResult: func(t *testing.T, resp *api.GetUserStatsResponse) {
				if resp.CommunityCount != 1 {
					t.Errorf("Expected 1 community, got %d", resp.CommunityCount)
				}
				if resp.ItemCount != 2 {
					t.Errorf("Expected 2 items (gear), got %d", resp.ItemCount)
				}
				if resp.LoansCount != 2 {
					t.Errorf("Expected 2 loans, got %d", resp.LoansCount)
				}
				if resp.BorrowsCount != 1 {
					t.Errorf("Expected 1 borrow, got %d", resp.BorrowsCount)
				}
				// No completed transfers, so savings should be zero
				if resp.Savings.TimeSavedHours != 0 {
					t.Errorf("Expected 0 hours saved (no completed transfers), got %d", resp.Savings.TimeSavedHours)
				}
			},
		},
		{
			name:   "user with help offered (request offers)",
			userID: "",
			setupData: func(ctx context.Context, str interface{}, userID string) error {
				sqlStorage := str.(*storage.ProtoSQLStorage)
				// Create active (non-withdrawn) offers
				offers := []models.RequestOffer{
					{UserId: userID, RequestId: "req-1", CommunityId: "comm-1", Withdrawn: false},
					{UserId: userID, RequestId: "req-2", CommunityId: "comm-1", Withdrawn: false},
				}
				for i := range offers {
					if _, err := sqlStorage.Insert(ctx, &offers[i]); err != nil {
						return err
					}
				}
				// Withdrawn offer should not count
				withdrawn := &models.RequestOffer{
					UserId: userID, RequestId: "req-3", CommunityId: "comm-1", Withdrawn: true,
				}
				if _, err := sqlStorage.Insert(ctx, withdrawn); err != nil {
					return err
				}
				return nil
			},
			validateResult: func(t *testing.T, resp *api.GetUserStatsResponse) {
				if resp.HelpOfferedCount != 2 {
					t.Errorf("Expected 2 help offered, got %d", resp.HelpOfferedCount)
				}
			},
		},
		{
			name:   "user with experiences hosted",
			userID: "",
			setupData: func(ctx context.Context, str interface{}, userID string) error {
				sqlStorage := str.(*storage.ProtoSQLStorage)
				experiences := []models.Experience{
					{OwnerId: userID, Name: "Hike"},
					{OwnerId: userID, Name: "Pottery class"},
					{OwnerId: userID, Name: "Board game night"},
				}
				for i := range experiences {
					if _, err := sqlStorage.Insert(ctx, &experiences[i]); err != nil {
						return err
					}
				}
				return nil
			},
			validateResult: func(t *testing.T, resp *api.GetUserStatsResponse) {
				if resp.EventsHostedCount != 3 {
					t.Errorf("Expected 3 events hosted, got %d", resp.EventsHostedCount)
				}
			},
		},
		{
			name:     "missing user_id",
			userID:   "",
			wantErr:  true,
			wantCode: connect.CodeInvalidArgument,
			setupData: func(ctx context.Context, str interface{}, userID string) error {
				return nil
			},
			validateResult: nil,
		},
		{
			name:     "user not found",
			userID:   "nonexistent-user-999",
			wantErr:  true,
			wantCode: connect.CodeNotFound,
			setupData: func(ctx context.Context, str interface{}, userID string) error {
				// Don't create the user
				return nil
			},
			validateResult: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, userManager, sqlStorage := setupTestService(t)
			ctx := context.Background()

			var actualUserID string

			// Create user if needed (unless testing error cases)
			if tt.wantErr && tt.wantCode == connect.CodeInvalidArgument {
				// Don't create user for missing user_id test
				actualUserID = ""
			} else if tt.wantErr && tt.wantCode == connect.CodeNotFound {
				// Don't create user for user not found test
				actualUserID = tt.userID
			} else {
				// Create test user
				testUser, err := userManager.CreateUser(ctx, tt.name+"@example.com", "Test User "+tt.name, models.Role_ROLE_USER)
				if err != nil {
					t.Fatalf("Failed to create test user: %v", err)
				}
				actualUserID = testUser.Id
			}

			// Setup test data
			if tt.setupData != nil {
				if err := tt.setupData(ctx, sqlStorage, actualUserID); err != nil {
					t.Fatalf("Failed to setup test data: %v", err)
				}
			}

			// Make the request
			req := connect.NewRequest(&api.GetUserStatsRequest{
				UserId: actualUserID,
			})

			resp, err := service.GetUserStats(ctx, req)

			// Check error expectations
			if tt.wantErr {
				if err == nil {
					t.Fatal("Expected error, got nil")
				}
				connectErr, ok := err.(*connect.Error)
				if !ok {
					t.Fatalf("Expected connect.Error, got %T", err)
				}
				if connectErr.Code() != tt.wantCode {
					t.Errorf("Expected error code %v, got %v", tt.wantCode, connectErr.Code())
				}
				return
			}

			// Check success case
			if err != nil {
				t.Fatalf("GetUserStats failed: %v", err)
			}

			if resp == nil || resp.Msg == nil {
				t.Fatal("Expected response, got nil")
			}

			// Validate results
			if tt.validateResult != nil {
				tt.validateResult(t, resp.Msg)
			}
		})
	}
}

// TestCalculateSavings tests ImpactEstimate aggregation from completed transfers.
func TestCalculateSavings(t *testing.T) {
	service, userManager, sqlStorage := setupTestService(t)
	ctx := context.Background()

	// Helper to create a user.
	createUser := func(email, name string) string {
		u, err := userManager.CreateUser(ctx, email, name, models.Role_ROLE_USER)
		if err != nil {
			t.Fatalf("Failed to create user %s: %v", email, err)
		}
		return u.Id
	}

	// Helper to build an ImpactEstimate for testing.
	makeIE := func(usd, minutes, mfgCo2, wasteCo2 float32) *models.ImpactEstimate {
		return &models.ImpactEstimate{
			MoneySaved: &models.MoneySavings{
				ValueUsd: &models.Estimate{Mean: usd, Stddev: 1},
			},
			TimeSaved: &models.TimeSavings{
				Minutes: &models.Estimate{Mean: minutes, Stddev: 1},
			},
			EmissionsPrevented: &models.PreventedEmissions{
				ManufactureAvoidedCarbon: &models.CarbonEstimate{
					Co2EGrams: &models.Estimate{Mean: mfgCo2, Stddev: 1},
				},
				WasteReducedCarbon: &models.CarbonEstimate{
					Co2EGrams: &models.Estimate{Mean: wasteCo2, Stddev: 1},
				},
			},
		}
	}

	t.Run("no transfers yields zero savings", func(t *testing.T) {
		ownerID := createUser("savings-none@example.com", "No Transfers")

		req := connect.NewRequest(&api.GetUserStatsRequest{UserId: ownerID})
		resp, err := service.GetUserStats(ctx, req)
		if err != nil {
			t.Fatalf("GetUserStats failed: %v", err)
		}
		s := resp.Msg.Savings
		if s.CostSavedUsd != 0 || s.TimeSavedHours != 0 || s.Co2SavedKg != 0 {
			t.Errorf("Expected all zero savings, got cost=%f time=%d co2=%f",
				s.CostSavedUsd, s.TimeSavedHours, s.Co2SavedKg)
		}
	})

	t.Run("completed transfers with IE are aggregated", func(t *testing.T) {
		ownerID := createUser("savings-agg@example.com", "Aggregation Owner")
		recipientID := createUser("savings-agg-r@example.com", "Aggregation Recipient")

		// Two completed transfers as owner with ImpactEstimate
		transfers := []models.Transfer{
			{
				OwnerId:        ownerID,
				RecipientId:    recipientID,
				GearId:         "gear-s1",
				TransferType:   models.TransferType_TRANSFER_TYPE_LOAN,
				State:          models.TransferState_TRANSFER_STATE_COMPLETED,
				ImpactEstimate: makeIE(23.0, 120, 2000, 300), // $23, 2h, 2.3kg
			},
			{
				OwnerId:        ownerID,
				RecipientId:    recipientID,
				GearId:         "gear-s2",
				TransferType:   models.TransferType_TRANSFER_TYPE_GIVEAWAY,
				State:          models.TransferState_TRANSFER_STATE_COMPLETED,
				ImpactEstimate: makeIE(10.0, 60, 1000, 200), // $10, 1h, 1.2kg
			},
		}
		for i := range transfers {
			if _, err := sqlStorage.Insert(ctx, &transfers[i]); err != nil {
				t.Fatalf("Failed to insert transfer: %v", err)
			}
		}

		req := connect.NewRequest(&api.GetUserStatsRequest{UserId: ownerID})
		resp, err := service.GetUserStats(ctx, req)
		if err != nil {
			t.Fatalf("GetUserStats failed: %v", err)
		}

		s := resp.Msg.Savings
		// $23 + $10 = $33
		if s.CostSavedUsd < 32.9 || s.CostSavedUsd > 33.1 {
			t.Errorf("Expected ~33 USD saved, got %f", s.CostSavedUsd)
		}
		// 120 + 60 = 180 min = 3 hours
		if s.TimeSavedHours != 3 {
			t.Errorf("Expected 3 hours saved, got %d", s.TimeSavedHours)
		}
		// (2000+300) + (1000+200) = 3500g = 3.5kg
		if s.Co2SavedKg < 3.4 || s.Co2SavedKg > 3.6 {
			t.Errorf("Expected ~3.5 kg CO2 saved, got %f", s.Co2SavedKg)
		}
	})

	t.Run("active transfers are excluded from savings", func(t *testing.T) {
		ownerID := createUser("savings-active@example.com", "Active Owner")
		recipientID := createUser("savings-active-r@example.com", "Active Recipient")

		// One active transfer with IE — should not count
		transfer := models.Transfer{
			OwnerId:        ownerID,
			RecipientId:    recipientID,
			GearId:         "gear-active",
			TransferType:   models.TransferType_TRANSFER_TYPE_LOAN,
			State:          models.TransferState_TRANSFER_STATE_ACTIVE,
			ImpactEstimate: makeIE(50.0, 90, 5000, 500),
		}
		if _, err := sqlStorage.Insert(ctx, &transfer); err != nil {
			t.Fatalf("Failed to insert transfer: %v", err)
		}

		req := connect.NewRequest(&api.GetUserStatsRequest{UserId: ownerID})
		resp, err := service.GetUserStats(ctx, req)
		if err != nil {
			t.Fatalf("GetUserStats failed: %v", err)
		}

		s := resp.Msg.Savings
		if s.CostSavedUsd != 0 || s.TimeSavedHours != 0 || s.Co2SavedKg != 0 {
			t.Errorf("Expected zero savings for active transfers, got cost=%f time=%d co2=%f",
				s.CostSavedUsd, s.TimeSavedHours, s.Co2SavedKg)
		}
	})

	t.Run("completed transfers without IE are skipped gracefully", func(t *testing.T) {
		ownerID := createUser("savings-noie@example.com", "No IE Owner")
		recipientID := createUser("savings-noie-r@example.com", "No IE Recipient")

		// Completed transfer with no ImpactEstimate
		transfer := models.Transfer{
			OwnerId:      ownerID,
			RecipientId:  recipientID,
			GearId:       "gear-noie",
			TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
			State:        models.TransferState_TRANSFER_STATE_COMPLETED,
		}
		if _, err := sqlStorage.Insert(ctx, &transfer); err != nil {
			t.Fatalf("Failed to insert transfer: %v", err)
		}

		req := connect.NewRequest(&api.GetUserStatsRequest{UserId: ownerID})
		resp, err := service.GetUserStats(ctx, req)
		if err != nil {
			t.Fatalf("GetUserStats failed: %v", err)
		}

		s := resp.Msg.Savings
		if s.CostSavedUsd != 0 || s.TimeSavedHours != 0 || s.Co2SavedKg != 0 {
			t.Errorf("Expected zero savings for transfers without IE, got cost=%f time=%d co2=%f",
				s.CostSavedUsd, s.TimeSavedHours, s.Co2SavedKg)
		}
	})

	t.Run("borrower transfers are included in savings", func(t *testing.T) {
		borrowerID := createUser("savings-borrow@example.com", "Borrower")
		lenderID := createUser("savings-borrow-l@example.com", "Lender")

		// Completed transfer where the tested user is the recipient
		transfer := models.Transfer{
			OwnerId:        lenderID,
			RecipientId:    borrowerID,
			GearId:         "gear-borrowed",
			TransferType:   models.TransferType_TRANSFER_TYPE_LOAN,
			State:          models.TransferState_TRANSFER_STATE_COMPLETED,
			ImpactEstimate: makeIE(15.0, 90, 1500, 0), // $15, 1.5h, 1.5kg
		}
		if _, err := sqlStorage.Insert(ctx, &transfer); err != nil {
			t.Fatalf("Failed to insert transfer: %v", err)
		}

		req := connect.NewRequest(&api.GetUserStatsRequest{UserId: borrowerID})
		resp, err := service.GetUserStats(ctx, req)
		if err != nil {
			t.Fatalf("GetUserStats failed: %v", err)
		}

		s := resp.Msg.Savings
		if s.CostSavedUsd < 14.9 || s.CostSavedUsd > 15.1 {
			t.Errorf("Expected ~15 USD saved from borrow, got %f", s.CostSavedUsd)
		}
		// 90 min → rounds to 2 hours
		if s.TimeSavedHours != 2 {
			t.Errorf("Expected 2 hours saved, got %d", s.TimeSavedHours)
		}
		if s.Co2SavedKg < 1.4 || s.Co2SavedKg > 1.6 {
			t.Errorf("Expected ~1.5 kg CO2 saved, got %f", s.Co2SavedKg)
		}
	})
}
