package profile

import (
	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/storage"
)

// Service implements the ProfileService RPC interface.
type Service struct {
	storage    *storage.ProtoSQLStorage
	calculator *impact_metrics.Calculator
	aiProvider ai.Provider // optional; nil disables narrative generation
}

// New creates a new Profile service.
//
// The calculator is required: GetUserProfileForViewer aggregates the
// target user's activity across their communities and depends on the
// shared impact_metrics library to produce hero/ticker numbers.
//
// aiProvider is optional. When nil, per-user impact-row narrative
// generation is disabled — the response carries no narrative fields
// and the client falls back to localized static strings. When set,
// the handler kicks off a background goroutine that ensures the
// narratives row is fresh for each viewed target.
func New(sqlStorage *storage.ProtoSQLStorage, calculator *impact_metrics.Calculator, aiProvider ai.Provider) *Service {
	return &Service{storage: sqlStorage, calculator: calculator, aiProvider: aiProvider}
}

// Ensure Service implements ProfileServiceHandler.
var _ apiconnect.ProfileServiceHandler = (*Service)(nil)
