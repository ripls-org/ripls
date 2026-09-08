// Package experiencegen holds the streaming-generator surface for
// experience generation: the [Context] struct, the [StreamSender]
// interface, and the [Generator] interface. Lives outside
// server/services/ so multiple service packages can depend on these
// types without violating service isolation (see
// docs/server/architecture.md §"Service Independence" and
// server/services/service_isolation_test.go).
//
// The implementation of [Generator] is *experience.Service in
// server/services/experience/. The unified_create service depends on
// the interface here, not on the experience service directly.
package experiencegen

import (
	"context"
	"time"

	"go.ripls.org/ripls/server/ai"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/logging"
)

// StreamSender is the abstract surface the streaming experience
// generators emit through. Implementations should emit synchronously —
// buffering breaks the "live as the LLM resolves" guarantee the
// streaming generators rely on.
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
	EmitTime(t *api.ExperienceTimeExtraction)
	EmitFinal(resp *api.GenExperienceResponse)
	EmitError(code api.GenStreamErrorCode, message string)
}

// Context bundles the request-level inputs the streaming experience
// generators read. Constructed by the StreamGenExperience handler from
// req.Msg, or by external callers (unified_create) from their own
// request shape.
//
// Image is optional. When set on an image-mode call
// (StreamGenExperienceFromImage), the per-type service uses it directly
// instead of re-resolving the media ID — see Context in geargen/types.go
// for the full rationale.
type Context struct {
	UserID             string
	LocationID         string
	Timezone           string
	CurrentTimeUnixSec int64
	LatitudeDeg        float64
	LongitudeDeg       float64
	Image              *ai.DetectionImage
}

// Generator is the full streaming surface — text, image, and webpage
// modes. *experience.Service satisfies this implicitly.
type Generator interface {
	StreamGenExperienceFromText(ctx context.Context, logger *logging.Logger, streamStart time.Time,
		args Context, prompt string, sender StreamSender) error
	StreamGenExperienceFromImage(ctx context.Context, logger *logging.Logger, streamStart time.Time,
		args Context, mediaID string, sender StreamSender) error
	StreamGenExperienceFromWebpage(ctx context.Context, logger *logging.Logger, streamStart time.Time,
		args Context, websiteURL string, sender StreamSender) error
}
