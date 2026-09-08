package storage

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestConnectionPoolLimits verifies that connection pool settings prevent connection exhaustion
// when many concurrent database operations are executed.
func TestConnectionPoolLimits(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()
	numConcurrent := 50 // More than the pool size to verify queuing works
	done := make(chan error, numConcurrent)

	// Spawn many concurrent database operations
	for i := 0; i < numConcurrent; i++ {
		go func(idx int) {
			gear := &models.Gear{
				Name:        fmt.Sprintf("Test Gear %d", idx),
				Description: "Connection pool test item",
			}
			_, err := storage.Insert(ctx, gear)
			done <- err
		}(i)
	}

	// Collect all results
	var errors []error
	for i := 0; i < numConcurrent; i++ {
		if err := <-done; err != nil {
			errors = append(errors, err)
		}
	}

	if len(errors) > 0 {
		t.Errorf("Expected all %d concurrent inserts to succeed, but %d failed. First error: %v",
			numConcurrent, len(errors), errors[0])
	}

	// Verify health check reports reasonable connection stats
	statuses, err := storage.CheckHealth(ctx)
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}
	if len(statuses) == 0 {
		t.Fatal("Expected health status")
	}
	t.Logf("Connection pool stats after concurrent operations: open=%s, in_use=%s, idle=%s",
		statuses[0].Metadata["open_connections"],
		statuses[0].Metadata["in_use"],
		statuses[0].Metadata["idle"])
}

// TestSanitizeColumnName verifies that column names exceeding PostgreSQL's
// 63-byte identifier limit are truncated with a hash suffix for uniqueness.
func TestSanitizeColumnName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLen  int  // 0 means same as input
		wantSame bool // true if output should equal input
	}{
		{
			name:     "short name passes through unchanged",
			input:    "id",
			wantSame: true,
		},
		{
			name:     "exactly 63 bytes passes through unchanged",
			input:    strings.Repeat("a", 63),
			wantSame: true,
		},
		{
			name:    "64 bytes gets truncated with hash",
			input:   strings.Repeat("a", 64),
			wantLen: 63,
		},
		{
			name:    "deeply nested impact estimate column name",
			input:   "impact_estimate_emissions_prevented_manufacture_avoided_carbon_co2e_grams_mean",
			wantLen: 63,
		},
		{
			name:    "deeply nested impact estimate stddev column name",
			input:   "impact_estimate_emissions_prevented_manufacture_avoided_carbon_co2e_grams_stddev",
			wantLen: 63,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeColumnName(tt.input)
			if tt.wantSame {
				if result != tt.input {
					t.Errorf("sanitizeColumnName(%q) = %q, want unchanged", tt.input, result)
				}
				return
			}
			if len(result) != tt.wantLen {
				t.Errorf("sanitizeColumnName(%q) length = %d, want %d", tt.input, len(result), tt.wantLen)
			}
		})
	}

	// Verify uniqueness: two different long names produce different results.
	name1 := "impact_estimate_emissions_prevented_manufacture_avoided_carbon_co2e_grams_mean"
	name2 := "impact_estimate_emissions_prevented_manufacture_avoided_carbon_co2e_grams_stddev"
	result1 := sanitizeColumnName(name1)
	result2 := sanitizeColumnName(name2)
	if result1 == result2 {
		t.Errorf("sanitizeColumnName produced identical results for different inputs:\n  %q -> %q\n  %q -> %q",
			name1, result1, name2, result2)
	}
}

// TestInsertAndRetrieveTransferWithImpactEstimate verifies that deeply nested
// ImpactEstimate fields survive Insert/GetByID roundtrip with sanitized column names.
func TestInsertAndRetrieveTransferWithImpactEstimate(t *testing.T) {
	storage, cleanup := SetupTestStorage(t)
	defer cleanup()

	ctx := context.Background()

	transfer := &models.Transfer{
		CommunityId:  "test-community-id",
		GearId:       "test-gear-id",
		OwnerId:      "test-owner-id",
		TransferType: models.TransferType_TRANSFER_TYPE_LOAN,
		State:        models.TransferState_TRANSFER_STATE_COMPLETED,
		ImpactEstimate: &models.ImpactEstimate{
			MoneySaved: &models.MoneySavings{
				ValueUsd: &models.Estimate{Mean: 75.0, Stddev: 22.5},
				Provenance: &models.Provenance{
					Reasoning: proto.String("test reasoning"),
				},
			},
			EmissionsPrevented: &models.PreventedEmissions{
				ManufactureAvoidedCarbon: &models.CarbonEstimate{
					Co2EGrams: &models.Estimate{Mean: 5000, Stddev: 2000},
					Provenance: &models.Provenance{
						Source: models.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
						Name:   "category_average_carbon",
					},
				},
				WasteReducedCarbon: &models.CarbonEstimate{
					Co2EGrams: &models.Estimate{Mean: 1000, Stddev: 400},
					Provenance: &models.Provenance{
						Source: models.ProvenanceSource_PROVENANCE_SOURCE_FORMULA,
						Name:   "weight_material_carbon",
					},
				},
			},
			TimeSaved: &models.TimeSavings{
				Minutes: &models.Estimate{Mean: 120, Stddev: 36},
			},
		},
	}

	id, err := storage.Insert(ctx, transfer)
	if err != nil {
		t.Fatalf("Failed to insert transfer with ImpactEstimate: %v", err)
	}

	// Retrieve and verify via binary_proto roundtrip.
	retrieved := &models.Transfer{}
	if err := storage.GetByID(ctx, id, retrieved); err != nil {
		t.Fatalf("Failed to get transfer: %v", err)
	}

	if retrieved.ImpactEstimate == nil {
		t.Fatal("ImpactEstimate is nil after roundtrip")
	}
	ie := retrieved.ImpactEstimate

	if ie.MoneySaved == nil || ie.MoneySaved.ValueUsd == nil {
		t.Fatal("MoneySaved.ValueUsd is nil after roundtrip")
	}
	if ie.MoneySaved.ValueUsd.Mean != 75.0 {
		t.Errorf("MoneySaved.ValueUsd.Mean = %v, want 75.0", ie.MoneySaved.ValueUsd.Mean)
	}
	if ie.MoneySaved.ValueUsd.Stddev != 22.5 {
		t.Errorf("MoneySaved.ValueUsd.Stddev = %v, want 22.5", ie.MoneySaved.ValueUsd.Stddev)
	}

	if ie.EmissionsPrevented == nil || ie.EmissionsPrevented.ManufactureAvoidedCarbon == nil {
		t.Fatal("EmissionsPrevented.ManufactureAvoidedCarbon is nil after roundtrip")
	}
	if ie.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams.Mean != 5000 {
		t.Errorf("ManufactureAvoidedCarbon.Mean = %v, want 5000",
			ie.EmissionsPrevented.ManufactureAvoidedCarbon.Co2EGrams.Mean)
	}

	if ie.TimeSaved == nil || ie.TimeSaved.Minutes == nil {
		t.Fatal("TimeSaved.Minutes is nil after roundtrip")
	}
	if ie.TimeSaved.Minutes.Mean != 120 {
		t.Errorf("TimeSaved.Minutes.Mean = %v, want 120", ie.TimeSaved.Minutes.Mean)
	}
}
