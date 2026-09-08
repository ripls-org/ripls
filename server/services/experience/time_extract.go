// Package experience: bulk-paste time-candidate extraction.
//
// ExtractTimeCandidates takes a free-form message ("tonight, tomorrow
// night, or Monday afternoon") and returns parsed candidates the client
// can drop into the time-poll propose modal. Mirrors
// ExtractLocationCandidates: server splits the text on conjunctions and
// runs each segment through the AI provider's informal-time parser in
// parallel, then dedupes by canonical instant.
package experience

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/logging"
)

// maxBulkTimeCandidates caps how many candidates one call can return.
// Beyond this we'd make too many parallel AI calls for one user
// gesture. Matches maxBulkCandidates on the location side.
const maxBulkTimeCandidates = 8

// ExtractTimeCandidates parses [req.Msg.Text] into candidate time
// segments, runs each segment through the AI provider in parallel, and
// returns the parsed candidates. Empty or unparsable segments are
// silently dropped; the response is empty when nothing usable came back.
func (s *Service) ExtractTimeCandidates(
	ctx context.Context,
	req *connect.Request[api.ExtractTimeCandidatesRequest],
) (*connect.Response[api.ExtractTimeCandidatesResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "ExtractTimeCandidates",
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

	if s.aiProvider == nil {
		logger.WarnContext(ctx, "AI provider unavailable; returning empty candidates")
		return connect.NewResponse(&api.ExtractTimeCandidatesResponse{}), nil
	}

	// Reuse the location-side segmenter — same conjunction set works for
	// time phrases ("tonight, tomorrow, or Monday").
	segments := extractCandidateSegments(text)
	if len(segments) > maxBulkTimeCandidates {
		segments = segments[:maxBulkTimeCandidates]
	}
	logger = logger.With("candidate_segment_count", len(segments))

	// Resolve current-time context once. The AI parser uses this to
	// anchor relative phrases ("tomorrow") against a consistent now.
	currentTime := time.Now().Format(time.RFC3339)
	if req.Msg.CurrentTimeUnixSec > 0 {
		currentTime = time.Unix(req.Msg.CurrentTimeUnixSec, 0).Format(time.RFC3339)
	}
	timezone := req.Msg.Timezone
	if timezone == "" {
		timezone = timezoneUTC
	}

	// Fan out one AI call per segment. Cap of [maxBulkTimeCandidates]
	// keeps the parallelism bounded; the AI provider's per-call timeout
	// limits the worst-case overall latency.
	type segmentResult struct {
		index   int
		time    *api.ExperienceTime
		segment string
	}
	results := make([]segmentResult, len(segments))
	var wg sync.WaitGroup
	for i, segment := range segments {
		wg.Add(1)
		idx, seg := i, segment
		logging.GoSafe(ctx, "extract-time-candidates-segment", func() {
			defer wg.Done()
			parsed := s.parseSegmentToTime(ctx, seg, currentTime, timezone, logger)
			results[idx] = segmentResult{index: idx, time: parsed, segment: seg}
		})
	}
	wg.Wait()

	// Dedupe by canonical instant so two segments parsing to the same
	// minute don't both stage. Mirrors the location dedupe by
	// external_place_id / display name.
	candidates := make([]*api.ExperienceTime, 0, len(results))
	seen := make(map[string]struct{}, len(results))
	for _, r := range results {
		if r.time == nil {
			continue
		}
		key := candidateKey(r.time)
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		candidates = append(candidates, r.time)
	}

	logger.InfoContext(ctx, "time candidates extracted",
		"resolved_count", len(candidates))

	return connect.NewResponse(&api.ExtractTimeCandidatesResponse{
		Candidates: candidates,
	}), nil
}

// parseSegmentToTime runs one informal-time string through the AI
// provider and returns the parsed [ExperienceTime] when the result is a
// usable specific or range time. Returns nil for TBD, empty, or
// parse-failed segments — the caller drops them silently.
func (s *Service) parseSegmentToTime(
	ctx context.Context,
	segment, currentTime, timezone string,
	logger *logging.Logger,
) *api.ExperienceTime {
	aiResponse, err := s.aiProvider.ParseInformalTime(ctx, segment, currentTime, timezone)
	if err != nil {
		logger.WarnContext(ctx, "AI parse failed for segment",
			"segment", segment, "error", err)
		return nil
	}
	aiResponse = stripMarkdownCodeBlocks(aiResponse)

	var parsed timeParseResponse
	if err := json.Unmarshal([]byte(aiResponse), &parsed); err != nil {
		logger.WarnContext(ctx, "AI response JSON unparseable for segment",
			"segment", segment, "error", err)
		return nil
	}

	t, _ := convertToExperienceTime(&parsed, segment)
	if t == nil {
		return nil
	}
	// Drop TBD results — the caller wants concrete times to stage.
	switch t.TimeType.(type) {
	case *api.ExperienceTime_Specific, *api.ExperienceTime_Range:
		return t
	default:
		return nil
	}
}

// candidateKey returns a canonical-instant string for a parsed
// [ExperienceTime] so duplicates collapse. Specific times key on their
// unix-second; range times key on their start-second.
func candidateKey(t *api.ExperienceTime) string {
	if specific := t.GetSpecific(); specific != nil {
		return fmt.Sprintf("ts:%d", specific.GetUnixTimestampSec())
	}
	if rng := t.GetRange(); rng != nil {
		return fmt.Sprintf("ts:%d", rng.GetStartUnixSec())
	}
	return ""
}
