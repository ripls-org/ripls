package gear

import (
	"context"

	"go.ripls.org/ripls/server/gen/ripls/models"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/impact_metrics/estimator"
	"go.ripls.org/ripls/server/media"
	"go.ripls.org/ripls/server/product"
	"go.ripls.org/ripls/server/storage"
	"go.ripls.org/ripls/server/webfetch"
)

// StockImageryProvider defines the interface for stock imagery operations.
// This is a copy of media.StockImageryProvider to avoid import cycles.
type StockImageryProvider interface {
	GetStockImage(ctx context.Context, query string, opts *media.StockImageOptions) (*models.StockImage, error)
	SearchStockImageCandidates(ctx context.Context, query string, limit int) ([]media.StockImageCandidate, error)
}

// Service implements the GearService RPC interface.
type Service struct {
	storage              *storage.ProtoSQLStorage
	bucket               storage.BucketStorage
	aiProvider           ai.Provider
	stockImageryProvider StockImageryProvider // Optional: can be nil
	webFetcher           webfetch.Fetcher     // Optional: can be nil (enables URL-based generation)
	productLookup        *product.Lookup      // Optional: can be nil (enables product spec lookup)
	estimatorCfg         *estimator.Config    // Optional: can be nil (enables embodied carbon computation)

	// provisionItemCommunity provisions the gear's per-item ad-hoc community at
	// creation and shares the gear into it with the given availability
	// (lend vs give away — the create flow's choice, #2687), returning the
	// community id (#2492). Injected from main.go so the gear service doesn't
	// depend on the community service or library directly (gear sharing lives
	// on the community service). Optional: nil in unit tests that don't
	// exercise per-item provisioning.
	provisionItemCommunity func(ctx context.Context, hostUserID, gearID string, availability models.Availability) (string, error)
}

// New creates a new gear service.
func New(sqlStorage *storage.ProtoSQLStorage, bucketStorage storage.BucketStorage) *Service {
	return &Service{
		storage: sqlStorage,
		bucket:  bucketStorage,
	}
}

// SetAIProvider sets the AI provider for this service.
func (s *Service) SetAIProvider(provider ai.Provider) {
	s.aiProvider = provider
}

// SetStockImageryProvider sets the stock imagery provider for this service (optional).
func (s *Service) SetStockImageryProvider(provider StockImageryProvider) {
	s.stockImageryProvider = provider
}

// SetWebFetcher sets the web fetcher for this service (optional).
// When set, enables URL-based gear generation from product pages.
func (s *Service) SetWebFetcher(fetcher webfetch.Fetcher) {
	s.webFetcher = fetcher
}

// SetProductLookup sets the product lookup for this service (optional).
// When set, enables automatic product specification lookup from manufacturer/retailer websites
// when AI detects a known product brand with high confidence.
func (s *Service) SetProductLookup(lookup *product.Lookup) {
	s.productLookup = lookup
}

// SetEstimatorConfig sets the impact estimator config for this service (optional).
// When set, enables embodied carbon computation on gear save.
func (s *Service) SetEstimatorConfig(cfg *estimator.Config) {
	s.estimatorCfg = cfg
}

// SetItemCommunityProvisioner wires the per-item community provisioner (#2492)
// so a newly created gear is born with its own nameless ad-hoc community (gear
// shared in with the creation-time availability, host the sole member),
// mirroring SaveExperience / SubmitRequest. The func composes
// community.ProvisionPerItemCommunity + the community service's gear share in
// main.go, keeping the gear service decoupled.
func (s *Service) SetItemCommunityProvisioner(fn func(ctx context.Context, hostUserID, gearID string, availability models.Availability) (string, error)) {
	s.provisionItemCommunity = fn
}

// isTerminalTransferState returns true if the transfer state is a terminal state.
func isTerminalTransferState(state models.TransferState) bool {
	return state == models.TransferState_TRANSFER_STATE_COMPLETED ||
		state == models.TransferState_TRANSFER_STATE_CANCELLED
}

// Verify that Service implements the GearServiceHandler interface.
var _ apiconnect.GearServiceHandler = (*Service)(nil)
