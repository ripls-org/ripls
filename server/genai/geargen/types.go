// Package geargen holds the streaming-generator surface for gear
// generation. Same shape as experiencegen — see its doc comment for
// rationale. Gear has no EmitTime (no event date/time) so the
// StreamSender interface is narrower than experience.
package geargen

import (
	"context"
	"time"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/logging"
)

// StreamSender is the abstract surface the streaming gear generators
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
	EmitFinal(resp *api.GenGearResponse)
	EmitError(code api.GenStreamErrorCode, message string)
}

// Context bundles the request-level inputs the streaming gear generators
// read. Today gear only carries UserID — it doesn't take lat/lng or
// timezone from the client. The struct exists for symmetry with
// experience/request and so future fields can be added without
// changing the function signature.
//
// Image is optional. When set on an image-mode call (StreamGenGearFromImage),
// the per-type service uses it directly instead of re-resolving the media
// ID — this lets unified-create thread the classifier's resolved
// *DetectionImage forward to the per-type call so both calls use the
// same signed URL (enabling Anthropic prompt-cache hits when paired
// with cache_control markers). See #1952 / #1939.
type Context struct {
	UserID string
	Image  *ai.DetectionImage
}

// Generator is the full streaming surface. *gear.Service satisfies
// this implicitly.
type Generator interface {
	StreamGenGearFromText(ctx context.Context, logger *logging.Logger, streamStart time.Time,
		args Context, prompt string, sender StreamSender) error
	StreamGenGearFromImage(ctx context.Context, logger *logging.Logger, streamStart time.Time,
		args Context, mediaID string, sender StreamSender) error
	StreamGenGearFromWebpage(ctx context.Context, logger *logging.Logger, streamStart time.Time,
		args Context, websiteURL string, sender StreamSender) error
}
