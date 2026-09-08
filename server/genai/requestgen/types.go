// Package requestgen holds the streaming-generator surface for request
// generation. Same shape as experiencegen — see its doc comment for
// rationale. Request has no EmitTime (no event date/time) so the
// StreamSender interface is narrower than experience.
package requestgen

import (
	"context"
	"time"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/logging"
)

// StreamSender is the abstract surface the streaming request generators
// emit through.
type StreamSender interface {
	EmitTitle(title string)
	EmitDescription(description string)
	EmitGeocoded(geo *api.GeocodedLocation)
	EmitMediaReady(mediaIDs []string)
	// EmitMediaReadyWithCandidates emits a MediaReady event carrying
	// both the chosen media (mediaIDs) and alternate candidate URLs the
	// client may surface as one-tap replacements. Pass nil candidates
	// to fall through to the same wire shape as EmitMediaReady.
	EmitMediaReadyWithCandidates(mediaIDs []string, candidates []*api.MediaCandidate)
	EmitFinal(resp *api.GenRequestResponse)
	EmitError(code api.GenStreamErrorCode, message string)
}

// Context bundles the request-level inputs the streaming request
// generators read.
//
// Image is optional. When set on an image-mode call
// (StreamGenRequestFromImage), the per-type service uses it directly
// instead of re-resolving the media ID — see Context in geargen/types.go
// for the full rationale.
type Context struct {
	UserID       string
	LocationID   string
	LatitudeDeg  float64
	LongitudeDeg float64
	Image        *ai.DetectionImage
}

// Generator is the full streaming surface. *request.Service satisfies
// this implicitly. Request has no webpage variant — unified_create
// synthesises a prompt from the page text and routes through
// StreamGenRequestFromText.
type Generator interface {
	StreamGenRequestFromText(ctx context.Context, logger *logging.Logger, streamStart time.Time,
		args Context, prompt string, sender StreamSender) error
	StreamGenRequestFromImage(ctx context.Context, logger *logging.Logger, streamStart time.Time,
		args Context, mediaID string, sender StreamSender) error
}
