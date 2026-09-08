package experience

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/services"
)

// TestExperience_FieldRoundTrip verifies that user-provided fields on
// SaveExperienceRequest survive the save → fetch cycle through
// GetExperience. See docs/proto_conventions.md for the round-trip
// convention and issue #1144 for the motivation.
//
// This test covers the straightforward scalar and repeated-string fields.
// GenExperience — the path that regressed in #1142 — runs through the AI
// provider and Mapbox client and is tested separately in
// gen_ai_integration_test.go.
func TestExperience_FieldRoundTrip(t *testing.T) {
	svc, testStorage, _ := setupTestService(t)

	const (
		ownerID    = "user-exp-roundtrip"
		ownerEmail = "exprt@example.com"
	)
	createTestUser(t, testStorage, ownerID, ownerEmail, "Exp RT User")
	ctx := createAuthenticatedContext(ownerID, ownerEmail, models.Role_ROLE_USER)

	// Seed a location so location_id is a valid reference (not strictly
	// required, but keeps the test faithful to real-world state).
	location := &models.Location{
		Geolocation: &models.Geolocation{LatitudeDeg: 39.7392, LongitudeDeg: -104.9903},
		Address:     &models.Address{Locality: "Denver", RegionCode: "US"},
	}
	locationID, err := testStorage.Insert(ctx, location)
	if err != nil {
		t.Fatalf("failed to insert location: %v", err)
	}

	sourceURL := "https://example.com/weekend-ski-trip"
	// A future range-timed value with a timezone: range.timezone is the
	// wire contract the SSR/off-app renderers depend on (#2621). Future so
	// the past-experience detection doesn't flip the initial state.
	rangeStart := time.Now().Add(48 * time.Hour).Unix()
	rangeEnd := time.Now().Add(52 * time.Hour).Unix()
	input := &api.SaveExperienceRequest{
		Name:            "Weekend ski trip",
		Description:     "Carpool to A-Basin Saturday",
		MediaIds:        []string{"media-exp-1", "media-exp-2"},
		LocationId:      locationID,
		MaxParticipants: 6,
		SourceUrl:       &sourceURL,
		Time: &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Range{
				Range: &api.TimeRange{
					Description:  "Saturday afternoon",
					StartUnixSec: &rangeStart,
					EndUnixSec:   &rangeEnd,
					Timezone:     "America/Denver",
				},
			},
		},
	}

	var savedID string
	save := func() error {
		resp, err := svc.SaveExperience(ctx, connect.NewRequest(input))
		if err != nil {
			return err
		}
		savedID = resp.Msg.GetExperience().GetId()
		return nil
	}
	if err := save(); err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}

	noopSave := func() error { return nil }
	get := func() (*api.Experience, error) {
		resp, err := svc.GetExperience(ctx, connect.NewRequest(&api.GetExperienceRequest{Id: savedID}))
		if err != nil {
			return nil, err
		}
		return resp.Msg.GetExperience(), nil
	}

	services.AssertFieldRoundTrip(t, "name", input.Name, noopSave,
		func() (string, error) { r, err := get(); return r.GetName(), err })
	services.AssertFieldRoundTrip(t, "description", input.Description, noopSave,
		func() (string, error) { r, err := get(); return r.GetDescription(), err })
	services.AssertFieldRoundTrip(t, "source_url", sourceURL, noopSave,
		func() (string, error) { r, err := get(); return r.GetSourceUrl(), err })
	services.AssertFieldRoundTrip(t, "media_ids", input.MediaIds, noopSave,
		func() ([]string, error) { r, err := get(); return r.GetMediaIds(), err })
	services.AssertFieldRoundTrip(t, "location_id", input.LocationId, noopSave,
		func() (string, error) { r, err := get(); return r.GetLocationId(), err })
	services.AssertFieldRoundTrip(t, "max_participants", input.MaxParticipants, noopSave,
		func() (int32, error) { r, err := get(); return r.GetMaxParticipants(), err })

	// Verify that lat/lng from the linked location record are surfaced.
	services.AssertFieldRoundTrip(t, "latitude_deg", location.Geolocation.LatitudeDeg, noopSave,
		func() (float64, error) { r, err := get(); return r.GetLatitudeDeg(), err })
	services.AssertFieldRoundTrip(t, "longitude_deg", location.Geolocation.LongitudeDeg, noopSave,
		func() (float64, error) { r, err := get(); return r.GetLongitudeDeg(), err })

	// Range-timed fields, including the timezone (#2621).
	services.AssertFieldRoundTrip(t, "time.range.timezone", "America/Denver", noopSave,
		func() (string, error) { r, err := get(); return r.GetTime().GetRange().GetTimezone(), err })
	services.AssertFieldRoundTrip(t, "time.range.start_unix_sec", rangeStart, noopSave,
		func() (int64, error) { r, err := get(); return r.GetTime().GetRange().GetStartUnixSec(), err })
	services.AssertFieldRoundTrip(t, "time.range.end_unix_sec", rangeEnd, noopSave,
		func() (int64, error) { r, err := get(); return r.GetTime().GetRange().GetEndUnixSec(), err })
}

// TestBuildAPIExperience_RSVPCounts verifies that rsvp_yes_count and
// rsvp_maybe_count are populated from ExperienceRSVP records (not hardcoded 0).
func TestBuildAPIExperience_RSVPCounts(t *testing.T) {
	svc, testStorage, _ := setupTestService(t)

	const (
		ownerID    = "user-rsvp-count-owner"
		ownerEmail = "rsvpowner@example.com"
		userYes1   = "user-rsvp-yes-1"
		userYes2   = "user-rsvp-yes-2"
		userMaybe1 = "user-rsvp-maybe-1"
	)
	createTestUser(t, testStorage, ownerID, ownerEmail, "RSVP Owner")
	createTestUser(t, testStorage, userYes1, "yes1@example.com", "Yes User 1")
	createTestUser(t, testStorage, userYes2, "yes2@example.com", "Yes User 2")
	createTestUser(t, testStorage, userMaybe1, "maybe1@example.com", "Maybe User 1")

	ownerCtx := createAuthenticatedContext(ownerID, ownerEmail, models.Role_ROLE_USER)
	communityID := createTestCommunity(t, testStorage, "RSVP Test Community", ownerID)
	createTestCommunityMembership(t, testStorage, communityID, ownerID)
	createTestCommunityMembership(t, testStorage, communityID, userYes1)
	createTestCommunityMembership(t, testStorage, communityID, userYes2)
	createTestCommunityMembership(t, testStorage, communityID, userMaybe1)

	saveResp, err := svc.SaveExperience(ownerCtx, connect.NewRequest(&api.SaveExperienceRequest{
		Name: "RSVP Count Test Experience",
	}))
	if err != nil {
		t.Fatalf("SaveExperience failed: %v", err)
	}
	expID := saveResp.Msg.GetExperience().GetId()

	shareExperienceForTest(t, svc, ownerCtx, expID, communityID)

	// Insert RSVPs directly to avoid going through the full RSVP RPC flow.
	for _, tc := range []struct {
		userID    string
		intention models.RSVPIntention
	}{
		{userYes1, models.RSVPIntention_RSVP_INTENTION_YES},
		{userYes2, models.RSVPIntention_RSVP_INTENTION_YES},
		{userMaybe1, models.RSVPIntention_RSVP_INTENTION_MAYBE},
	} {
		_, err := testStorage.Insert(context.Background(), &models.ExperienceRSVP{
			ExperienceId:       expID,
			UserId:             tc.userID,
			CommunityId:        communityID,
			Intention:          tc.intention,
			Attended:           models.AttendedStatus_ATTENDED_STATUS_UNKNOWN,
			RsvpedAtUnixSec:    1000,
			LastUpdatedUnixSec: 1000,
		})
		if err != nil {
			t.Fatalf("failed to insert RSVP for %s: %v", tc.userID, err)
		}
	}

	// Query without CommunityId so RSVP counts are cross-community: the owner's
	// auto-RSVP lives in the experience's per-item community (#2492), while the
	// explicit RSVPs were inserted into communityID. Per-community scoping would
	// only see the RSVPs in that one community.
	resp, err := svc.GetExperience(ownerCtx, connect.NewRequest(&api.GetExperienceRequest{
		Id: expID,
	}))
	if err != nil {
		t.Fatalf("GetExperience failed: %v", err)
	}

	got := resp.Msg.GetExperience()
	// Owner is auto-RSVPed YES when sharing; plus 2 explicit YES users = 3 total YES.
	if got.GetRsvpYesCount() != 3 {
		t.Errorf("rsvp_yes_count: want 3 (owner auto-rsvp + 2 explicit), got %d", got.GetRsvpYesCount())
	}
	if got.GetRsvpMaybeCount() != 1 {
		t.Errorf("rsvp_maybe_count: want 1, got %d", got.GetRsvpMaybeCount())
	}
}
