package experience

import (
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/genai/experiencegen"
)

// Type aliases keep the experience package's existing identifiers
// working while routing the actual definitions through the shared
// library. Service-isolation (see server/services/service_isolation_test.go)
// forbids services from importing each other; unified_create depends on
// experiencegen, not on experience, and *experience.Service satisfies
// experiencegen.Generator implicitly.
type (
	GenExperienceContext = experiencegen.Context
	StreamSender         = experiencegen.StreamSender
)

// EmitTitle / EmitDescription / EmitGeocoded / EmitMediaReady / EmitTime
// / EmitFinal / EmitError satisfy [experiencegen.StreamSender] by
// delegating to the package-internal lowercase methods on
// experienceEventSender.
func (s *experienceEventSender) EmitTitle(t string)                   { s.emitTitle(t) }
func (s *experienceEventSender) EmitDescription(d string)             { s.emitDescription(d) }
func (s *experienceEventSender) EmitGeocoded(g *api.GeocodedLocation) { s.emitGeocoded(g) }
func (s *experienceEventSender) EmitMediaReady(ids []string)          { s.emitMediaReady(ids) }
func (s *experienceEventSender) EmitMediaReadyWithCandidates(ids []string, c []*api.MediaCandidate) {
	s.emitMediaReadyWithCandidates(ids, c)
}
func (s *experienceEventSender) EmitTime(t *api.ExperienceTimeExtraction) { s.emitTime(t) }
func (s *experienceEventSender) EmitFinal(r *api.GenExperienceResponse)   { s.emitFinal(r) }
func (s *experienceEventSender) EmitError(c api.GenStreamErrorCode, m string) {
	s.emitError(c, m)
}
