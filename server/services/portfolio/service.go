package portfolio

import (
	"context"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/storage"
	"go.ripls.org/ripls/server/weather"
)

// NudgeProvider supplies an optional contextual nudge for the Home view,
// implemented by the feed service. Declared here (consumer-side) so the
// portfolio service depends on a behavior, not the feed service's concrete
// type — nudge generation/storage stays owned by the feed package.
type NudgeProvider interface {
	// InboxNudge returns one nudge for the inbox, searching across the viewer's
	// communities, or nil when none is available. Best-effort.
	InboxNudge(ctx context.Context, userID string, communityIDs []string) (*api.NudgePayload, error)
}

// Service implements the PortfolioService RPC interface.
type Service struct {
	storage         *storage.ProtoSQLStorage
	calculator      *impact_metrics.MetricDetailCalculator
	watchStorage    *storage.WatchStorage
	nudgeProvider   NudgeProvider
	weatherProvider weather.Provider
}

// New creates a new portfolio service.
func New(store *storage.ProtoSQLStorage, calculator *impact_metrics.MetricDetailCalculator) *Service {
	return &Service{
		storage:      store,
		calculator:   calculator,
		watchStorage: storage.NewWatchStorage(store),
	}
}

// SetNudgeProvider injects the feed-backed nudge provider used to surface a
// contextual nudge in the Home view. Optional — when unset, the view carries
// no nudge and the client falls back to its static prompt.
func (s *Service) SetNudgeProvider(p NudgeProvider) {
	s.nudgeProvider = p
}

// SetWeatherProvider injects the weather provider used to paint the calendar's
// per-day forecast. Optional — when unset, the calendar carries no weather and
// the client renders cells without a glyph. See docs/weather.md.
func (s *Service) SetWeatherProvider(p weather.Provider) {
	s.weatherProvider = p
}

// Ensure Service implements PortfolioServiceHandler.
var _ apiconnect.PortfolioServiceHandler = (*Service)(nil)
