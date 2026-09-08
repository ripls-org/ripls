package request

import (
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/genai/requestgen"
)

// Type aliases keep request's existing identifiers working while
// routing the actual definitions through the shared library.
// Service-isolation (see server/services/service_isolation_test.go)
// forbids services from importing each other; unified_create depends on
// requestgen, not on request, and *request.Service satisfies
// requestgen.Generator implicitly.
type (
	GenRequestContext = requestgen.Context
	StreamSender      = requestgen.StreamSender
)

// EmitTitle / EmitDescription / EmitGeocoded / EmitMediaReady /
// EmitFinal / EmitError satisfy [requestgen.StreamSender] by delegating
// to the package-internal lowercase methods on requestEventSender.
func (s *requestEventSender) EmitTitle(t string)                   { s.emitTitle(t) }
func (s *requestEventSender) EmitDescription(d string)             { s.emitDescription(d) }
func (s *requestEventSender) EmitGeocoded(g *api.GeocodedLocation) { s.emitGeocoded(g) }
func (s *requestEventSender) EmitMediaReady(ids []string)          { s.emitMediaReady(ids) }
func (s *requestEventSender) EmitMediaReadyWithCandidates(ids []string, c []*api.MediaCandidate) {
	s.emitMediaReadyWithCandidates(ids, c)
}
func (s *requestEventSender) EmitFinal(r *api.GenRequestResponse)          { s.emitFinal(r) }
func (s *requestEventSender) EmitError(c api.GenStreamErrorCode, m string) { s.emitError(c, m) }
