package gear

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/auth"
	"go.ripls.org/ripls/server/connecterr"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/logging"
	"go.ripls.org/ripls/server/webfetch"
)

// DetectGearFromURL extracts product metadata from a product page URL.
//
// It fetches the webpage and uses AI to extract structured metadata fields only
// (brand, category, material, weight, value). Unlike GenGear, it does not
// generate a title, description, images, or location. It returns a clear error
// if the site cannot be fetched — there is no fallback to text generation.
func (s *Service) DetectGearFromURL(
	ctx context.Context,
	req *connect.Request[api.DetectGearFromURLRequest],
) (*connect.Response[api.DetectGearFromURLResponse], error) {
	authInfo, err := auth.RequireAuth(ctx)
	if err != nil {
		return nil, err
	}

	rawURL := strings.TrimSpace(req.Msg.Url)
	if rawURL == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("url is required"))
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("url must start with http:// or https://"))
	}

	if s.aiProvider == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("AI provider not configured"))
	}
	if s.webFetcher == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("web fetcher not configured"))
	}

	logger := logging.LoggerWithContext(ctx).With(
		"operation", "DetectGearFromURL",
		"user_id", authInfo.UserID,
		"url", rawURL,
	)
	logger.Info("fetching product page for gear metadata extraction")

	pageContent, err := s.webFetcher.FetchPageContent(ctx, rawURL)
	if err != nil {
		logger.Warn("failed to fetch product page", "error", err)
		return nil, connectErrorFromFetchError(ctx, err)
	}

	logger.Debug("fetched page content",
		"title", pageContent.Title,
		"body_length", len(pageContent.BodyText),
	)

	generation, err := s.aiProvider.GenerateGearFromWebpage(
		ctx,
		pageContent.Title,
		pageContent.Description,
		pageContent.BodyText,
		"",
	)
	if err != nil {
		logger.Error("AI metadata extraction failed", "error", err)
		return nil, connecterr.Internal(ctx, "DetectGearFromURL", err, "detail", "failed to extract product details")
	}

	ai.SanitizeGearGeneration(generation)

	item := &api.DetectedGearItem{
		Brand:            generation.Brand,
		Category:         generation.Category,
		MaterialCategory: ai.MaterialCategoryFromJSON(generation.MaterialCategory),
	}
	if generation.WeightGrams > 0 {
		item.WeightGrams = &api.Estimate{
			Mean:   generation.WeightGrams,
			Stddev: generation.WeightGrams * 0.3,
		}
	}
	if generation.ValueEstimate != nil {
		prov := &api.Provenance{
			Source:     api.ProvenanceSource_PROVENANCE_SOURCE_LLM,
			Name:       "genai_value_estimate",
			Confidence: proto.Float32(float32(generation.ValueEstimate.Confidence)),
			Sources:    generation.ValueEstimate.Sources,
		}
		if s.estimatorCfg != nil {
			prov.Version = s.estimatorCfg.ProvenanceVersion("genai_value_estimate")
		}
		item.ValueEstimate = &api.ValueEstimate{
			EstimatedValueUsd: generation.ValueEstimate.EstimatedValueUSD,
			Provenance:        prov,
		}
	}

	logger.Info("extracted product metadata",
		"brand", item.Brand,
		"category", item.Category,
		"has_value", item.ValueEstimate != nil,
	)

	return connect.NewResponse(&api.DetectGearFromURLResponse{
		DetectedGear: item,
	}), nil
}

// connectErrorFromFetchError maps a webfetch.FetchError to an appropriate Connect error code.
// CodeInternal cases route through connecterr.Internal so the underlying err
// is logged server-side and not serialized to the client; the other cases
// surface user-actionable messages.
func connectErrorFromFetchError(ctx context.Context, err error) error {
	var fetchErr *webfetch.FetchError
	if !errors.As(err, &fetchErr) {
		return connecterr.Internal(ctx, "DetectGearFromURL.fetch", err)
	}
	switch fetchErr.Kind {
	case webfetch.FetchErrorBlocked:
		return connect.NewError(connect.CodePermissionDenied, err)
	case webfetch.FetchErrorNotFound:
		return connect.NewError(connect.CodeNotFound, err)
	case webfetch.FetchErrorTimeout:
		return connect.NewError(connect.CodeDeadlineExceeded, err)
	case webfetch.FetchErrorConnection:
		return connect.NewError(connect.CodeUnavailable, err)
	case webfetch.FetchErrorInvalidURL:
		return connect.NewError(connect.CodeInvalidArgument, err)
	case webfetch.FetchErrorParse:
		return connecterr.Internal(ctx, "DetectGearFromURL.fetch.parse", err)
	default:
		return connecterr.Internal(ctx, "DetectGearFromURL.fetch.unknown", err)
	}
}
