package ai

import (
	"context"
)

// DefaultAnthropicModel is the model used by the Anthropic provider
// when no override is supplied. Exported so glue code in main can use
// the same constant.
const DefaultAnthropicModel = "claude-haiku-4-5"

// UnifiedCreateClassifierInput is the input to a unified-create
// classifier call. Exactly one of Text / Image / WebsiteURL is set.
// When WebsiteURL is set, callers may also populate WebsiteTitle and
// WebsiteDescription so the classifier can decide on the page metadata
// without a second fetch. When Image is set, the bytes (or presigned
// URL) are sent to the model as an image content block — the model
// classifies based on actual visual content rather than a text-only
// instruction.
type UnifiedCreateClassifierInput struct {
	Text               string
	Image              *DetectionImage
	WebsiteURL         string
	WebsiteTitle       string
	WebsiteDescription string
}

// UnifiedCreateClassifier is a narrow interface around the classifier
// method on ai.Provider, kept for use by the eval runner and any
// future consumer that wants the classifier surface without the full
// Provider interface.
type UnifiedCreateClassifier interface {
	Classify(ctx context.Context, in UnifiedCreateClassifierInput) (*UnifiedCreateClassification, error)
}
