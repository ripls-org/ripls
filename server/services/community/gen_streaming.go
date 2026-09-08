// Streaming variant of GenCommunity. Text mode only: opens the streaming AI
// call, fires stock-image fan-out as the driving JSON `search_keywords` field
// closes, emits a `media_ready` event when an image is found, and ends with a
// terminal `final` or `error` event.
//
// Community creation no longer AI-generates a name or description — the user
// supplies those directly. The LLM only extracts image search keywords from
// the user's name/description text. Image-upload ("bring your own background")
// is handled client-side via direct media upload, not through this RPC.

package community

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/auth"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/genai/mediacand"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/streaming"
)

// maxImageCandidates caps how many alternate URLs ride on MediaReady.
const maxImageCandidates = 4

// fetchStockCandidates returns image candidate alternates for the
// keyword query. Best-effort: any provider failure logs and returns
// nil. Communities have no stock-video fan-out, so only images here.
func (s *Service) fetchStockCandidates(ctx context.Context, keywords []string, limit int, logger *logging.Logger) []*api.MediaCandidate {
	if limit <= 0 || len(keywords) == 0 || s.stockImageryProvider == nil {
		return nil
	}
	query := strings.Join(keywords, " ")
	stockCands, err := s.stockImageryProvider.SearchStockImageCandidates(ctx, query, limit)
	if err != nil {
		logger.WarnContext(ctx, "stock image candidate search failed", "error", err)
		return nil
	}
	return mediacand.StockImageCandidatesToAPI(stockCands)
}

// StreamGenCommunity streams the background image found for a community,
// keyed on the community's name/description text.
func (s *Service) StreamGenCommunity(
	ctx context.Context,
	req *connect.Request[api.StreamGenCommunityRequest],
	stream *connect.ServerStream[api.StreamGenCommunityResponse],
) error {
	streamStart := time.Now()

	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return err
	}

	logger := logging.LoggerWithContext(ctx).With(
		"user_id", authInfo.UserID,
		"user_email", logging.MaskEmail(authInfo.Email),
		"operation", "StreamGenCommunity",
	)
	logger.InfoContext(ctx, "stream started")

	sender := newCommunityEventSender(stream)
	defer func() {
		if cerr := sender.close(); cerr != nil {
			logger.WarnContext(ctx, "stream send failed",
				"error", cerr,
				"duration_ms", time.Since(streamStart).Milliseconds())
		}
	}()

	return s.streamGenCommunityCore(ctx, logger, streamStart, req.Msg, sender)
}

// streamGenCommunityCore holds the core streaming logic. Separated so unit
// tests can exercise it with a capture sender and without needing a real
// Connect ServerStream.
func (s *Service) streamGenCommunityCore(
	ctx context.Context,
	logger *logging.Logger,
	streamStart time.Time,
	msg *api.StreamGenCommunityRequest,
	sender *communityEventSender,
) error {
	if msg.Prompt == "" {
		sender.emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INVALID_ARGUMENT,
			"prompt must be provided")
		return nil
	}
	if s.aiProvider == nil {
		sender.emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_INTERNAL,
			"AI provider not configured")
		return nil
	}

	return s.streamGenCommunityFromText(ctx, logger, streamStart, msg, sender)
}

// streamGenCommunityFromText runs the streaming AI call (keyword extraction)
// with concurrent stock-image fan-out (keyed on search_keywords) for
// background-image discovery.
func (s *Service) streamGenCommunityFromText(
	ctx context.Context,
	logger *logging.Logger,
	streamStart time.Time,
	msg *api.StreamGenCommunityRequest,
	sender *communityEventSender,
) error {
	streamFields, streamFinal, err := s.aiProvider.GenerateCommunityContentStreaming(ctx, msg.Prompt, msg.Region)
	if err != nil {
		logger.ErrorContext(ctx, "failed to open AI stream", "error", err)
		sender.emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED, err.Error())
		return nil
	}

	var (
		stockMediaID string
		branchWG     sync.WaitGroup
	)
	mediaFired := false
	skipStockMedia := s.stockImageryProvider == nil

	for ev := range streamFields {
		//nolint:gocritic // singleCaseSwitch: deliberate dispatch over the
		// StreamField key space. The sibling gen_streaming.go files in gear,
		// request and experience switch over several keys each; keeping the
		// same shape here means adding a watched key is a new `case`, not a
		// restructure.
		switch ev.Key {
		case ai.StreamFieldSearchKeywords:
			if skipStockMedia {
				continue
			}
			var keywords []string
			if err := json.Unmarshal(ev.Value, &keywords); err != nil || len(keywords) == 0 {
				continue
			}
			mediaFired = true
			branchWG.Add(1)
			keywordsCopy := keywords
			logging.GoSafe(ctx, "stream-gen-community-media", func() {
				defer branchWG.Done()
				id, ferr := s.findAndStoreStockImage(ctx, keywordsCopy)
				if ferr != nil {
					// Stock-image lookup is best-effort; mirror unary behavior.
					logger.WarnContext(ctx, "failed to get stock image", "error", ferr)
					return
				}
				if id == "" {
					return
				}
				stockMediaID = id
				cands := s.fetchStockCandidates(ctx, keywordsCopy, maxImageCandidates, logger)
				if len(cands) > 0 {
					sender.emitMediaReadyWithCandidates([]string{id}, cands)
				} else {
					sender.emitMediaReady([]string{id})
				}
			})
		}
	}

	branchWG.Wait()
	final := <-streamFinal
	if final.Err != nil {
		ai.LogAIError(ctx, logger, "AI streaming generation failed", final.Err)
		sender.emitError(api.GenStreamErrorCode_GEN_STREAM_ERROR_CODE_AI_PROVIDER_FAILED, final.Err.Error())
		return nil
	}

	logger.InfoContext(ctx, "streaming community image fan-out completed",
		"media_fired", mediaFired,
		"media_hit", stockMediaID != "",
		"stream_duration_ms", time.Since(streamStart).Milliseconds())

	var mediaIDs []string
	if stockMediaID != "" {
		mediaIDs = []string{stockMediaID}
	}

	sender.emitFinal(&api.GenCommunityResponse{MediaIds: mediaIDs})
	return nil
}

// communityEventSender wraps streaming.Sender with typed emit helpers for
// the StreamGenCommunityResponse oneof.
type communityEventSender struct {
	*streaming.Sender[api.StreamGenCommunityResponse]
}

func newCommunityEventSender(target streaming.SendTarget[api.StreamGenCommunityResponse]) *communityEventSender {
	return &communityEventSender{Sender: streaming.NewSender[api.StreamGenCommunityResponse](target)}
}

func (s *communityEventSender) emitMediaReady(mediaIDs []string) {
	s.emitMediaReadyWithCandidates(mediaIDs, nil)
}

func (s *communityEventSender) emitMediaReadyWithCandidates(mediaIDs []string, candidates []*api.MediaCandidate) {
	s.Emit(&api.StreamGenCommunityResponse{
		Event: &api.StreamGenCommunityResponse_MediaReady{
			MediaReady: &api.MediaReady{
				MediaIds:   mediaIDs,
				Candidates: candidates,
			},
		},
	})
}

func (s *communityEventSender) emitFinal(resp *api.GenCommunityResponse) {
	s.Emit(&api.StreamGenCommunityResponse{
		Event: &api.StreamGenCommunityResponse_Final{Final: resp},
	})
}

func (s *communityEventSender) emitError(code api.GenStreamErrorCode, message string) {
	s.Emit(&api.StreamGenCommunityResponse{
		Event: &api.StreamGenCommunityResponse_Error{
			Error: &api.GenStreamError{Code: code, Message: message},
		},
	})
}

func (s *communityEventSender) close() error { return s.Close() }
