package request

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

// TestRequest_FieldRoundTrip verifies that user-provided fields on
// SubmitRequestRequest survive the submit → fetch cycle through GetRequest.
// See docs/proto_conventions.md for the round-trip convention and issue
// #1144 for the motivation.
func TestRequest_FieldRoundTrip(t *testing.T) {
	svc, testStorage, _, _, _ := setupTestServiceWithNotifications(t)

	requesterID := setupTestUser(t, testStorage, "Requester", "rt@example.com")
	communityID := setupCommunityWithMembers(t, testStorage, requesterID)
	ctx := createAuthenticatedContext(requesterID, "rt@example.com", models.Role_ROLE_USER)

	// Location that the request will refer to.
	location := &models.Location{
		Geolocation: &models.Geolocation{LatitudeDeg: 40.7128, LongitudeDeg: -74.006},
		Address:     &models.Address{Locality: "New York", AddressLines: []string{"123 Main St"}},
	}
	locationID, err := testStorage.Insert(context.Background(), location)
	if err != nil {
		t.Fatalf("failed to insert location: %v", err)
	}

	input := &api.SubmitRequestRequest{
		Title:       "Power Drill",
		Description: "Need a cordless drill for the weekend",
		LocationId:  locationID,
		MediaIds:    []string{"media-req-1"},
	}

	var requestID string
	save := func() error {
		resp, err := svc.SubmitRequest(ctx, connect.NewRequest(input))
		if err != nil {
			return err
		}
		requestID = resp.Msg.RequestId
		return nil
	}
	if err := save(); err != nil {
		t.Fatalf("SubmitRequest failed: %v", err)
	}
	shareRequestInto(t, ctx, svc, requestID, communityID)
	// No WaitForStockImagery: the request supplies its own media_ids, so
	// the server skips the stock imagery fetch entirely.

	noopSave := func() error { return nil }
	get := func() (*api.Request, error) {
		resp, err := svc.GetRequest(ctx, connect.NewRequest(&api.GetRequestRequest{
			RequestId:   requestID,
			CommunityId: communityID,
		}))
		if err != nil {
			return nil, err
		}
		return resp.Msg.GetRequest(), nil
	}

	services.AssertFieldRoundTrip(t, "title", input.Title, noopSave,
		func() (string, error) { r, err := get(); return r.GetTitle(), err })
	services.AssertFieldRoundTrip(t, "description", input.Description, noopSave,
		func() (string, error) { r, err := get(); return r.GetDescription(), err })
	services.AssertFieldRoundTrip(t, "location_id", input.LocationId, noopSave,
		func() (string, error) { r, err := get(); return r.GetLocationId(), err })
	services.AssertFieldRoundTrip(t, "media_ids", input.MediaIds, noopSave,
		func() ([]string, error) { r, err := get(); return r.GetMediaIds(), err })
}
