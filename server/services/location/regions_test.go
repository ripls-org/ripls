package location

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
)

// TestCreateRegionFromAddress_StateCode is the regression test for #2802:
// verifies that CreateRegionFromAddress persists the correct USPS state code
// and scopes the city name with it.
func TestCreateRegionFromAddress_StateCode(t *testing.T) {
	service, sqlStorage, _ := setupTestService(t)
	ctx := createAuthContext("user-test", "test@example.com")

	req := connect.NewRequest(&api.CreateRegionFromAddressRequest{
		Locality:            "Boulder",
		AdministrativeArea:  "Colorado",
		PreferredRegionType: "city",
	})

	resp, err := service.CreateRegionFromAddress(ctx, req)
	if err != nil {
		t.Fatalf("CreateRegionFromAddress failed: %v", err)
	}

	// City region_name must be scoped with the state code.
	if resp.Msg.RegionName != "Boulder, CO" {
		t.Errorf("city region_name = %q, want %q", resp.Msg.RegionName, "Boulder, CO")
	}
	if resp.Msg.RegionType != "city" {
		t.Errorf("region_type = %q, want %q", resp.Msg.RegionType, "city")
	}

	// Verify the parent state region was persisted with region_code = "CO".
	stateResults, err := sqlStorage.QueryByFields(ctx, map[string]interface{}{
		"region_type": "state",
		"region_name": "Colorado",
	}, &models.Region{})
	if err != nil {
		t.Fatalf("failed to query state region: %v", err)
	}
	if len(stateResults) == 0 {
		t.Fatal("expected a state region for Colorado, got none")
	}
	stateRegion := stateResults[0].(*models.Region)
	if stateRegion.GetRegionCode() != "CO" {
		t.Errorf("state region_code = %q, want %q", stateRegion.GetRegionCode(), "CO")
	}
}

// TestCreateRegionFromAddress_StateLevel verifies that a state-level request
// also sets region_code correctly.
func TestCreateRegionFromAddress_StateLevel(t *testing.T) {
	service, sqlStorage, _ := setupTestService(t)
	ctx := createAuthContext("user-test", "test@example.com")

	req := connect.NewRequest(&api.CreateRegionFromAddressRequest{
		AdministrativeArea:  "Colorado",
		PreferredRegionType: "state",
	})

	resp, err := service.CreateRegionFromAddress(ctx, req)
	if err != nil {
		t.Fatalf("CreateRegionFromAddress failed: %v", err)
	}

	if resp.Msg.RegionType != "state" {
		t.Errorf("region_type = %q, want %q", resp.Msg.RegionType, "state")
	}

	// Verify region_code was stored correctly.
	stateResults, err := sqlStorage.QueryByFields(context.Background(), map[string]interface{}{
		"region_type": "state",
		"region_name": "Colorado",
	}, &models.Region{})
	if err != nil {
		t.Fatalf("failed to query state region: %v", err)
	}
	if len(stateResults) == 0 {
		t.Fatal("expected a state region for Colorado, got none")
	}
	stateRegion := stateResults[0].(*models.Region)
	if stateRegion.GetRegionCode() != "CO" {
		t.Errorf("state region_code = %q, want %q", stateRegion.GetRegionCode(), "CO")
	}
}

// TestCreateRegionFromAddress_NonUSState verifies that non-US inputs still
// create a region (with empty code) without error — preserving backward
// compatibility for non-US callers.
func TestCreateRegionFromAddress_NonUSState(t *testing.T) {
	service, _, _ := setupTestService(t)
	ctx := createAuthContext("user-test", "test@example.com")

	req := connect.NewRequest(&api.CreateRegionFromAddressRequest{
		AdministrativeArea:  "Ontario",
		PreferredRegionType: "state",
	})

	resp, err := service.CreateRegionFromAddress(ctx, req)
	if err != nil {
		t.Fatalf("CreateRegionFromAddress failed for non-US state: %v", err)
	}
	if resp.Msg.RegionType != "state" {
		t.Errorf("region_type = %q, want %q", resp.Msg.RegionType, "state")
	}
}
