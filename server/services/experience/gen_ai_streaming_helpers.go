package experience

import (
	"context"
	"time"

	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/streaming"
)

// resolveRegion looks up the user's primary location and returns a region
// label for the AI prompt plus the geocoding-proximity geolocation.
func (s *Service) resolveRegion(ctx context.Context, locationID string) (string, *models.Geolocation) {
	region := "Unknown region"
	var proximity *models.Geolocation
	if locationID == "" {
		return region, nil
	}
	location := &models.Location{}
	if err := s.storage.GetByID(ctx, locationID, location); err != nil {
		return region, nil
	}
	if location.Address != nil && location.Address.Locality != "" {
		region = location.Address.Locality
	}
	if location.Geolocation != nil &&
		(location.Geolocation.LatitudeDeg != 0 || location.Geolocation.LongitudeDeg != 0) {
		proximity = location.Geolocation
	}
	return region, proximity
}

// resolveTimezone returns the parsed Location and the effective IANA name.
// Date math falls back to UTC on empty or invalid input, but the returned
// name stays empty in that case: it is persisted on the suggested time, and
// a stored "UTC" literal is indistinguishable from a real UTC-timezone
// event (#2621).
func (s *Service) resolveTimezone(logger *logging.Logger, tz string) (string, *time.Location) {
	if tz == "" {
		return "", time.UTC
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		logger.Warn("invalid timezone, falling back to UTC", "timezone", tz, "error", err)
		return "", time.UTC
	}
	return tz, loc
}

// formatCurrentTime returns an RFC3339-formatted current time in the user's
// timezone. When unixSec is zero, uses the server's wall clock.
func formatCurrentTime(unixSec int64, loc *time.Location) string {
	if unixSec > 0 {
		return time.Unix(unixSec, 0).In(loc).Format(time.RFC3339)
	}
	return time.Now().In(loc).Format(time.RFC3339)
}

// buildExperienceTimeExtraction parses the AI's raw date / time /
// time_confidence into the structured ExperienceTimeExtraction the streaming
// `time` event and the terminal `final` event both need. Centralised so the
// mid-stream and final paths can't drift.
func buildExperienceTimeExtraction(
	rawDate string,
	rawTime string,
	rawConfidence string,
	userTimezone string,
	loc *time.Location,
) *api.ExperienceTimeExtraction {
	confidence := api.TimeConfidence_TIME_CONFIDENCE_UNKNOWN
	switch rawConfidence {
	case ai.TimeConfidenceExplicit:
		confidence = api.TimeConfidence_TIME_CONFIDENCE_EXPLICIT
	case ai.TimeConfidenceInferred:
		confidence = api.TimeConfidence_TIME_CONFIDENCE_INFERRED
	}

	suggested := &api.ExperienceTime{
		TimeType: &api.ExperienceTime_Tbd{Tbd: &api.TimeTBD{}},
	}
	var extractedTimeUnixSec int64
	if rawDate != "" {
		dateTimeStr := rawDate
		if rawTime != "" {
			dateTimeStr += "T" + rawTime + ":00"
		} else {
			dateTimeStr += "T00:00:00"
		}
		if parsed, err := time.ParseInLocation("2006-01-02T15:04:05", dateTimeStr, loc); err == nil {
			extractedTimeUnixSec = parsed.Unix()
		}
	}
	if extractedTimeUnixSec != 0 &&
		(confidence == api.TimeConfidence_TIME_CONFIDENCE_EXPLICIT ||
			confidence == api.TimeConfidence_TIME_CONFIDENCE_INFERRED) {
		suggested = &api.ExperienceTime{
			TimeType: &api.ExperienceTime_Specific{
				Specific: &api.SpecificTime{
					UnixTimestampSec: extractedTimeUnixSec,
					Timezone:         userTimezone,
				},
			},
		}
	}
	return &api.ExperienceTimeExtraction{
		SuggestedTime:        suggested,
		Confidence:           confidence,
		ExtractedTimeUnixSec: extractedTimeUnixSec,
	}
}

// buildGenExperienceResponse assembles the terminal `final` response from
// the AI generation, fan-out results, and post-AI derived fields. Mirrors
// the shape built by GenExperience's unary handler.
func buildGenExperienceResponse(
	aiResponse *ai.ExperienceGeneration,
	mediaIds []string,
	locationID string,
	geocodedLocation *api.GeocodedLocation,
	sourceURL string,
	userTimezone string,
	loc *time.Location,
) *api.GenExperienceResponse {
	timeExtraction := buildExperienceTimeExtraction(
		aiResponse.Date, aiResponse.Time, aiResponse.TimeConfidence,
		userTimezone, loc,
	)
	suggestedTime := timeExtraction.GetSuggestedTime()
	timeConfidence := timeExtraction.GetConfidence()
	extractedTimeUnixSec := timeExtraction.GetExtractedTimeUnixSec()

	var metadata *api.ExperienceMetadata
	if aiResponse.ValueEstimate != nil {
		metadata = &api.ExperienceMetadata{
			ValueEstimate: &api.ValueEstimate{
				EstimatedValueUsd: aiResponse.ValueEstimate.EstimatedValueUSD,
				Provenance: &api.Provenance{
					Source:     api.ProvenanceSource_PROVENANCE_SOURCE_LLM,
					Name:       "genai_value_estimate",
					Confidence: proto.Float32(float32(aiResponse.ValueEstimate.Confidence)),
					Sources:    aiResponse.ValueEstimate.Sources,
				},
			},
		}
	}

	return &api.GenExperienceResponse{
		Name:                 aiResponse.Title,
		Description:          aiResponse.Description,
		MediaIds:             mediaIds,
		LocationId:           locationID,
		SuggestedTime:        suggestedTime,
		Tags:                 []string{},
		TimeConfidence:       timeConfidence,
		ExtractedTimeUnixSec: extractedTimeUnixSec,
		LocationQuery:        aiResponse.LocationQuery,
		GeocodedLocation:     geocodedLocation,
		SourceUrl:            sourceURL,
		Metadata:             metadata,
		MentionedNames:       aiResponse.MentionedNames,
	}
}

// experienceEventSender wraps the shared streaming.Sender with typed emit
// helpers that construct the StreamGenExperienceResponse oneof wrapper.
// The serialization + drainer goroutine lives in the shared package.
type experienceEventSender struct {
	*streaming.Sender[api.StreamGenExperienceResponse]
}

func newExperienceEventSender(target streaming.SendTarget[api.StreamGenExperienceResponse]) *experienceEventSender {
	return &experienceEventSender{Sender: streaming.NewSender[api.StreamGenExperienceResponse](target)}
}

func (s *experienceEventSender) emitTitle(title string) {
	s.Emit(&api.StreamGenExperienceResponse{
		Event: &api.StreamGenExperienceResponse_Title{Title: title},
	})
}

func (s *experienceEventSender) emitGeocoded(g *api.GeocodedLocation) {
	s.Emit(&api.StreamGenExperienceResponse{
		Event: &api.StreamGenExperienceResponse_Geocoded{Geocoded: g},
	})
}

func (s *experienceEventSender) emitMediaReady(mediaIDs []string) {
	s.emitMediaReadyWithCandidates(mediaIDs, nil)
}

func (s *experienceEventSender) emitMediaReadyWithCandidates(mediaIDs []string, candidates []*api.MediaCandidate) {
	s.Emit(&api.StreamGenExperienceResponse{
		Event: &api.StreamGenExperienceResponse_MediaReady{
			MediaReady: &api.MediaReady{
				MediaIds:   mediaIDs,
				Candidates: candidates,
			},
		},
	})
}

func (s *experienceEventSender) emitDescription(description string) {
	s.Emit(&api.StreamGenExperienceResponse{
		Event: &api.StreamGenExperienceResponse_Description{Description: description},
	})
}

func (s *experienceEventSender) emitTime(t *api.ExperienceTimeExtraction) {
	s.Emit(&api.StreamGenExperienceResponse{
		Event: &api.StreamGenExperienceResponse_Time{Time: t},
	})
}

func (s *experienceEventSender) emitFinal(resp *api.GenExperienceResponse) {
	s.Emit(&api.StreamGenExperienceResponse{
		Event: &api.StreamGenExperienceResponse_Final{Final: resp},
	})
}

func (s *experienceEventSender) emitError(code api.GenStreamErrorCode, message string) {
	s.Emit(&api.StreamGenExperienceResponse{
		Event: &api.StreamGenExperienceResponse_Error{
			Error: &api.GenStreamError{Code: code, Message: message},
		},
	})
}

func (s *experienceEventSender) close() error { return s.Close() }
