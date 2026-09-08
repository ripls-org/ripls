package workshop

import (
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
	"go.ripls.org/ripls/server/impact_metrics"
	"go.ripls.org/ripls/server/storage"
)

// Service implements the WorkshopService RPC interface.
type Service struct {
	storage    *storage.ProtoSQLStorage
	calculator *impact_metrics.Calculator
}

// New creates a new Workshop service.
//
// The `calculator` is optional — when nil, the synthesis surface
// (`GetWorkshopSynthesis`) returns no panels rather than crashing. v1
// callers without an estimator config can pass nil; production wires
// the same calculator the portfolio service uses.
func New(sqlStorage *storage.ProtoSQLStorage, calculator *impact_metrics.Calculator) *Service {
	return &Service{storage: sqlStorage, calculator: calculator}
}

// Ensure Service implements WorkshopServiceHandler.
var _ apiconnect.WorkshopServiceHandler = (*Service)(nil)
