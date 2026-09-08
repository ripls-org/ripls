package unified_create

import (
	"context"
	"fmt"

	"go.ripls.org/ripls/server/ai"
)

// ClassifierInput is a thin alias for ai.UnifiedCreateClassifierInput
// so call sites in this package can stay terse.
type ClassifierInput = ai.UnifiedCreateClassifierInput

// Classifier is a thin alias for ai.UnifiedCreateClassifier so the
// service signature stays terse.
type Classifier = ai.UnifiedCreateClassifier

// providerClassifier adapts ai.Provider to the narrow Classifier
// interface so the unified-create service can stay decoupled from
// the full Provider surface. Production wiring runs the classifier
// through the same provider chain (Gemini primary, Anthropic
// fallback, ...) as every other AI surface.
type providerClassifier struct{ p ai.Provider }

// NewProviderClassifier returns a Classifier backed by ai.Provider.
// The Classify call routes to Provider.ClassifyUnifiedCreate, which
// inherits the provider's fallback and load-balancing behavior.
func NewProviderClassifier(p ai.Provider) Classifier {
	return &providerClassifier{p: p}
}

func (c *providerClassifier) Classify(ctx context.Context, in ClassifierInput) (*ai.UnifiedCreateClassification, error) {
	return c.p.ClassifyUnifiedCreate(ctx, in)
}

// stubClassifier returns a fixed low-confidence "gear" result; useful
// in dev / tests when no real classifier has been wired. The real
// production wiring uses NewProviderClassifier with the configured
// ai.Provider.
type stubClassifier struct{}

// NewStubClassifier returns a placeholder classifier suitable for dev
// and tests. Production callers should pass a real
// ai.UnifiedCreateClassifier.
func NewStubClassifier() Classifier {
	return &stubClassifier{}
}

func (c *stubClassifier) Classify(_ context.Context, in ClassifierInput) (*ai.UnifiedCreateClassification, error) {
	if in.Text == "" && in.Image == nil && in.WebsiteURL == "" {
		return nil, fmt.Errorf("classifier: empty input")
	}
	return &ai.UnifiedCreateClassification{
		Type: ai.UnifiedCreateContentTypeGear,
	}, nil
}
