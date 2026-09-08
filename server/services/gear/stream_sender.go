package gear

import (
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/genai/geargen"
)

// Type aliases keep gear's existing identifiers working while routing
// the actual definitions through the shared library. Service-isolation
// (see server/services/service_isolation_test.go) forbids services from
// importing each other; unified_create depends on geargen, not on gear,
// and *gear.Service satisfies geargen.Generator implicitly.
type (
	GenGearContext = geargen.Context
	StreamSender   = geargen.StreamSender
)

// EmitTitle / EmitDescription / EmitGeocoded / EmitMediaReady /
// EmitFinal / EmitError satisfy [geargen.StreamSender] by delegating to
// the package-internal lowercase methods on gearEventSender.
func (s *gearEventSender) EmitTitle(t string)                   { s.emitTitle(t) }
func (s *gearEventSender) EmitDescription(d string)             { s.emitDescription(d) }
func (s *gearEventSender) EmitGeocoded(g *api.GeocodedLocation) { s.emitGeocoded(g) }
func (s *gearEventSender) EmitMediaReady(ids []string)          { s.emitMediaReady(ids) }
func (s *gearEventSender) EmitMediaReadyWithCandidates(ids []string, c []*api.MediaCandidate) {
	s.emitMediaReadyWithCandidates(ids, c)
}
func (s *gearEventSender) EmitFinal(r *api.GenGearResponse)             { s.emitFinal(r) }
func (s *gearEventSender) EmitError(c api.GenStreamErrorCode, m string) { s.emitError(c, m) }
