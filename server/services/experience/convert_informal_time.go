package experience

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/logging"
)

// timeParseResponse represents the JSON structure returned by the LLM.
// timezoneUTC is the fallback IANA zone used for date math when the client
// sent no usable timezone. It is deliberately never persisted as the event's
// timezone — a stored "UTC" is indistinguishable from a genuinely
// UTC-scheduled event (#2621).
const timezoneUTC = "UTC"

type timeParseResponse struct {
	TimeType         string `json:"time_type"`          // "specific", "range", or "tbd"
	UnixTimestampSec int64  `json:"unix_timestamp_sec"` // For specific times
	Timezone         string `json:"timezone"`           // IANA timezone
	DurationMinutes  int32  `json:"duration_minutes"`   // Duration in minutes
	StartUnixSec     int64  `json:"start_unix_sec"`     // For ranges
	EndUnixSec       int64  `json:"end_unix_sec"`       // For ranges
	Description      string `json:"description"`        // For ranges
	Confidence       string `json:"confidence"`         // "EXPLICIT", "INFERRED", or "UNKNOWN"
}

// ConvertInformalTime converts a natural language time description to structured ExperienceTime.
// This RPC is used by the calendar-chips time picker modal for real-time parsing.
func (s *Service) ConvertInformalTime(
	ctx context.Context,
	req *connect.Request[api.ConvertInformalTimeRequest],
) (*connect.Response[api.ConvertInformalTimeResponse], error) {
	logger := logging.LoggerWithContext(ctx).With(
		"informal_description", req.Msg.InformalDescription,
	)

	// Validate input
	if req.Msg.InformalDescription == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("informal_description is required"))
	}

	// Check if AI provider is configured
	if s.aiProvider == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("AI provider not configured"))
	}

	// Get current time for relative date parsing
	var currentTime string
	if req.Msg.CurrentTimeUnixSec > 0 {
		// Convert Unix timestamp to RFC3339 format
		currentTime = time.Unix(req.Msg.CurrentTimeUnixSec, 0).Format(time.RFC3339)
	} else {
		// Default to current time if not provided
		currentTime = time.Now().Format(time.RFC3339)
	}

	// Default timezone if not provided
	timezone := req.Msg.Timezone
	if timezone == "" {
		timezone = timezoneUTC
	}

	// Call LLM to parse the informal time description
	aiResponse, err := s.aiProvider.ParseInformalTime(ctx, req.Msg.InformalDescription, currentTime, timezone)
	if err != nil {
		logger.Warn("LLM time parsing failed", "error", err)
		return connect.NewResponse(&api.ConvertInformalTimeResponse{
			Time:         nil,
			Confidence:   api.TimeConfidence_TIME_CONFIDENCE_UNKNOWN,
			ErrorMessage: fmt.Sprintf("Failed to parse time: %v", err),
		}), nil // Return success with error message in response
	}

	logger.Debug("received LLM response", "response", aiResponse)

	// Strip markdown code blocks if present
	aiResponse = stripMarkdownCodeBlocks(aiResponse)

	// Parse LLM JSON response
	var parsed timeParseResponse
	if err := json.Unmarshal([]byte(aiResponse), &parsed); err != nil {
		logger.Error("failed to parse LLM response", "error", err, "raw_response", aiResponse)
		return connect.NewResponse(&api.ConvertInformalTimeResponse{
			Time:         nil,
			Confidence:   api.TimeConfidence_TIME_CONFIDENCE_UNKNOWN,
			ErrorMessage: "Failed to parse LLM response",
		}), nil
	}

	// When the request carried no timezone, the LLM was prompted with UTC and
	// usually just echoes it back. Don't persist that echo — a stored "UTC"
	// literal is indistinguishable from a real UTC-timezone event (#2621). A
	// non-UTC timezone the LLM inferred from the text itself is kept.
	if req.Msg.Timezone == "" && parsed.Timezone == timezoneUTC {
		parsed.Timezone = ""
	}

	// Convert to ExperienceTime proto
	experienceTime, confidence := convertToExperienceTime(&parsed, req.Msg.InformalDescription)

	return connect.NewResponse(&api.ConvertInformalTimeResponse{
		Time:         experienceTime,
		Confidence:   confidence,
		ErrorMessage: "",
	}), nil
}

// convertToExperienceTime converts the parsed LLM response to an ExperienceTime proto.
func convertToExperienceTime(parsed *timeParseResponse, informalDescription string) (*api.ExperienceTime, api.TimeConfidence) {
	// Parse confidence
	confidence := parseConfidence(parsed.Confidence)

	// Create ExperienceTime based on type
	switch parsed.TimeType {
	case "specific":
		return &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{
				Specific: &api.SpecificTime{
					UnixTimestampSec: parsed.UnixTimestampSec,
					Timezone:         parsed.Timezone,
					DurationMinutes:  parsed.DurationMinutes,
				},
			},
			InformalDescription: informalDescription,
			Confidence:          confidence,
		}, confidence

	case "range":
		return &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Range{
				Range: &api.TimeRange{
					Description:     parsed.Description,
					StartUnixSec:    &parsed.StartUnixSec,
					EndUnixSec:      &parsed.EndUnixSec,
					Timezone:        parsed.Timezone,
					DurationMinutes: parsed.DurationMinutes,
				},
			},
			InformalDescription: informalDescription,
			Confidence:          confidence,
		}, confidence

	case "tbd":
		fallthrough
	default:
		return &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Tbd{
				Tbd: &api.TimeTBD{},
			},
			InformalDescription: informalDescription,
			Confidence:          api.TimeConfidence_TIME_CONFIDENCE_UNKNOWN,
		}, api.TimeConfidence_TIME_CONFIDENCE_UNKNOWN
	}
}

// parseConfidence converts string confidence to enum.
func parseConfidence(confidenceStr string) api.TimeConfidence {
	switch confidenceStr {
	case ai.TimeConfidenceExplicit:
		return api.TimeConfidence_TIME_CONFIDENCE_EXPLICIT
	case ai.TimeConfidenceInferred:
		return api.TimeConfidence_TIME_CONFIDENCE_INFERRED
	case ai.TimeConfidenceUnknown:
		fallthrough
	default:
		return api.TimeConfidence_TIME_CONFIDENCE_UNKNOWN
	}
}

// stripMarkdownCodeBlocks removes markdown code block formatting from LLM responses.
// Some LLMs wrap JSON in ```json ... ``` despite instructions not to.
func stripMarkdownCodeBlocks(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		// Find first newline after opening ```
		firstNewline := strings.Index(s, "\n")
		if firstNewline > 0 {
			s = s[firstNewline+1:]
		}
		// Remove trailing ```
		s = strings.TrimSuffix(strings.TrimSpace(s), "```")
	}
	return strings.TrimSpace(s)
}
