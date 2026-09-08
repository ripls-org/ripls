// Package experience: bulk-paste location-candidate extraction.
//
// ExtractLocationCandidates takes a free-form message ("maybe Avery,
// Chautauqua, or that bagel place on West Pearl?") and returns geocoded
// candidates the client can drop into the propose modal.
//
// The current implementation is a focused heuristic splitter + Mapbox
// per-segment lookup. The RPC contract is stable so swapping in an LLM
// extractor later is a server-only change.
package experience

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/location"
	"go.ripls.org/ripls/server/logging"
)

// maxBulkCandidates caps how many candidates one call can return. Beyond
// this we'd be making too many Mapbox round-trips for one user gesture.
const maxBulkCandidates = 8

// minSegmentLen rejects fragments too short to be a real place name
// (e.g. stray "or", "the", whitespace) after stop-word stripping.
const minSegmentLen = 3

// ExtractLocationCandidates parses the request text into candidate names,
// geocodes each one, and returns the successful matches.
func (s *Service) ExtractLocationCandidates(
	ctx context.Context,
	req *connect.Request[api.ExtractLocationCandidatesRequest],
) (*connect.Response[api.ExtractLocationCandidatesResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ExtractLocationCandidates",
		"user_id", authInfo.UserID,
		"text_len", len(req.Msg.Text),
	)

	text := strings.TrimSpace(req.Msg.Text)
	if text == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("text is required"))
	}
	if len(text) > 4000 {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("text is too long (max 4000 chars)"))
	}

	if s.locationProvider == nil {
		logger.WarnContext(ctx, "location provider unavailable; returning empty candidates")
		return connect.NewResponse(&api.ExtractLocationCandidatesResponse{}), nil
	}

	segments := extractCandidateSegments(text)
	if len(segments) > maxBulkCandidates {
		segments = segments[:maxBulkCandidates]
	}
	logger = logger.With("candidate_segment_count", len(segments))

	// Bulk-paste candidates are extracted from the user's own text. Bias
	// toward the user's GPS when supplied, but use a soft Hint rather than
	// a hard Bound: pasted segments often include explicit named-but-distant
	// POIs ("Zilker Park") and dropping them is worse UX than admitting an
	// out-of-area "park" → "Parker, CO" mis-resolve.
	proximityReq := location.ProximityRequest{Semantics: location.ProximityNone}
	if req.Msg.LatitudeDeg != nil && req.Msg.LongitudeDeg != nil {
		proximityReq = location.ProximityRequest{
			Coord: &models.Geolocation{
				LatitudeDeg:  *req.Msg.LatitudeDeg,
				LongitudeDeg: *req.Msg.LongitudeDeg,
			},
			Semantics: location.ProximityHint,
		}
	}

	candidates := make([]*api.GeocodedLocation, 0, len(segments))
	seen := make(map[string]struct{}, len(segments))
	for _, segment := range segments {
		places, err := s.locationProvider.SearchPlaces(ctx, segment, proximityReq, 1)
		if err != nil {
			logger.WarnContext(ctx, "geocode failed for candidate segment",
				"segment", segment, "error", err)
			continue
		}
		if len(places) == 0 {
			continue
		}
		best := places[0]
		displayName := best.Name
		if best.Type == "Address" {
			displayName = best.FullAddress
		}
		geocoded := best.ToGeocodedLocation(displayName)
		if geocoded == nil {
			continue
		}
		// Dedupe by external place ID (or, when missing, by display name).
		key := geocoded.ExternalPlaceId
		if key == "" {
			key = strings.ToLower(geocoded.Name)
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		candidates = append(candidates, geocoded)
	}

	logger.InfoContext(ctx, "location candidates extracted",
		"resolved_count", len(candidates))

	if len(candidates) == 0 && len(segments) > 0 {
		// All segments failed to geocode. Surface a soft error so the
		// client can show a friendly hint instead of an empty list.
		return nil, connecterr.Internal(ctx, "ExtractLocationCandidates",
			fmt.Errorf("no candidates resolved"),
			"segment_count", len(segments))
	}

	return connect.NewResponse(&api.ExtractLocationCandidatesResponse{
		Candidates: candidates,
	}), nil
}

// extractCandidateSegments splits a free-form message into candidate place
// names. Heuristic:
//
//  1. Lowercase the text and split on common conjunctions / list separators
//     ("," / ";" / " or " / " and " / " plus " / " then ").
//  2. Strip filler clauses ("maybe", "that", "the", question marks).
//  3. Discard fragments shorter than minSegmentLen after stripping.
//
// Returns trimmed segments in input order with duplicates removed.
func extractCandidateSegments(text string) []string {
	// Drop trailing punctuation that confuses the splitter.
	text = strings.TrimRight(text, ".?! ")

	// Splitter regex matches the conjunctions / separators between segments.
	// Word boundaries protect names like "Theodore" from matching " the ".
	sep := regexp.MustCompile(`(?i)\s*(?:,|;|\bor\b|\band\b|\bplus\b|\bthen\b|/|\|)\s*`)
	rawSegments := sep.Split(text, -1)

	results := make([]string, 0, len(rawSegments))
	seen := make(map[string]struct{}, len(rawSegments))
	for _, raw := range rawSegments {
		cleaned := cleanCandidateSegment(raw)
		if len(cleaned) < minSegmentLen {
			continue
		}
		key := strings.ToLower(cleaned)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		results = append(results, cleaned)
	}
	return results
}

// cleanCandidateSegment trims whitespace and filler words from a fragment.
func cleanCandidateSegment(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	// Drop trailing punctuation.
	s = strings.TrimRight(s, ".?! ,;")

	// Strip leading filler. The list is short on purpose: aggressive
	// stripping risks losing real place names that begin with "the".
	lower := strings.ToLower(s)
	for _, prefix := range []string{
		"maybe ", "perhaps ", "how about ", "what about ",
		"that ", "that one ", "the place ", "the spot ", "a place ",
		"e.g. ", "eg ",
	} {
		if strings.HasPrefix(lower, prefix) {
			s = s[len(prefix):]
			lower = strings.ToLower(s)
		}
	}

	return strings.TrimSpace(s)
}
