package unified_create

import (
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/streaming"
)

// The three per-type adapters below let unified-create delegate to the
// streaming generators in server/services/{experience,gear,request}
// without duplicating their fan-out, fallback, time-extraction, or
// final-assembly logic. Each adapter satisfies the corresponding
// `*StreamSender` interface in its sibling service and translates each
// emit into the unified-envelope event shape
// (api.StreamGenUnifiedCreateResponse).
//
// EmitFinal is special: instead of forwarding the per-type final on the
// wire, the adapter captures it onto the outer StreamGenUnifiedCreateFinal
// payload (Type + ClassifierConfidence are stamped by the dispatcher).
// The unified Final event is sent by the dispatcher *after* the per-type
// generator returns. EmitError IS forwarded immediately so the client
// learns about failures as they happen.

// experienceAdapter satisfies experiencegen.StreamSender.
type experienceAdapter struct {
	target streaming.SendTarget[api.StreamGenUnifiedCreateResponse]
	out    *api.StreamGenUnifiedCreateFinal
}

func (a *experienceAdapter) EmitTitle(title string) {
	_ = a.target.Send(&api.StreamGenUnifiedCreateResponse{
		Event: &api.StreamGenUnifiedCreateResponse_Title{Title: title},
	})
}

func (a *experienceAdapter) EmitDescription(description string) {
	_ = a.target.Send(&api.StreamGenUnifiedCreateResponse{
		Event: &api.StreamGenUnifiedCreateResponse_Description{Description: description},
	})
}

func (a *experienceAdapter) EmitGeocoded(geo *api.GeocodedLocation) {
	_ = a.target.Send(&api.StreamGenUnifiedCreateResponse{
		Event: &api.StreamGenUnifiedCreateResponse_Geocoded{Geocoded: geo},
	})
}

func (a *experienceAdapter) EmitMediaReady(mediaIDs []string) {
	a.EmitMediaReadyWithCandidates(mediaIDs, nil)
}

func (a *experienceAdapter) EmitMediaReadyWithCandidates(mediaIDs []string, candidates []*api.MediaCandidate) {
	_ = a.target.Send(&api.StreamGenUnifiedCreateResponse{
		Event: &api.StreamGenUnifiedCreateResponse_MediaReady{
			MediaReady: &api.MediaReady{
				MediaIds:   mediaIDs,
				Candidates: candidates,
			},
		},
	})
}

func (a *experienceAdapter) EmitTime(t *api.ExperienceTimeExtraction) {
	_ = a.target.Send(&api.StreamGenUnifiedCreateResponse{
		Event: &api.StreamGenUnifiedCreateResponse_Time{Time: t},
	})
}

func (a *experienceAdapter) EmitFinal(resp *api.GenExperienceResponse) {
	a.out.Payload = &api.StreamGenUnifiedCreateFinal_Experience{Experience: resp}
}

func (a *experienceAdapter) EmitError(code api.GenStreamErrorCode, message string) {
	_ = a.target.Send(&api.StreamGenUnifiedCreateResponse{
		Event: &api.StreamGenUnifiedCreateResponse_Error{
			Error: &api.GenStreamError{Code: code, Message: message},
		},
	})
}

// gearAdapter satisfies geargen.StreamSender. Gear has no EmitTime
// (no event date/time).
type gearAdapter struct {
	target streaming.SendTarget[api.StreamGenUnifiedCreateResponse]
	out    *api.StreamGenUnifiedCreateFinal
}

func (a *gearAdapter) EmitTitle(title string) {
	_ = a.target.Send(&api.StreamGenUnifiedCreateResponse{
		Event: &api.StreamGenUnifiedCreateResponse_Title{Title: title},
	})
}

func (a *gearAdapter) EmitDescription(description string) {
	_ = a.target.Send(&api.StreamGenUnifiedCreateResponse{
		Event: &api.StreamGenUnifiedCreateResponse_Description{Description: description},
	})
}

func (a *gearAdapter) EmitGeocoded(geo *api.GeocodedLocation) {
	_ = a.target.Send(&api.StreamGenUnifiedCreateResponse{
		Event: &api.StreamGenUnifiedCreateResponse_Geocoded{Geocoded: geo},
	})
}

func (a *gearAdapter) EmitMediaReady(mediaIDs []string) {
	a.EmitMediaReadyWithCandidates(mediaIDs, nil)
}

func (a *gearAdapter) EmitMediaReadyWithCandidates(mediaIDs []string, candidates []*api.MediaCandidate) {
	_ = a.target.Send(&api.StreamGenUnifiedCreateResponse{
		Event: &api.StreamGenUnifiedCreateResponse_MediaReady{
			MediaReady: &api.MediaReady{
				MediaIds:   mediaIDs,
				Candidates: candidates,
			},
		},
	})
}

func (a *gearAdapter) EmitFinal(resp *api.GenGearResponse) {
	a.out.Payload = &api.StreamGenUnifiedCreateFinal_Gear{Gear: resp}
}

func (a *gearAdapter) EmitError(code api.GenStreamErrorCode, message string) {
	_ = a.target.Send(&api.StreamGenUnifiedCreateResponse{
		Event: &api.StreamGenUnifiedCreateResponse_Error{
			Error: &api.GenStreamError{Code: code, Message: message},
		},
	})
}

// requestAdapter satisfies requestgen.StreamSender. Request has no
// EmitTime (no event date/time).
type requestAdapter struct {
	target streaming.SendTarget[api.StreamGenUnifiedCreateResponse]
	out    *api.StreamGenUnifiedCreateFinal
}

func (a *requestAdapter) EmitTitle(title string) {
	_ = a.target.Send(&api.StreamGenUnifiedCreateResponse{
		Event: &api.StreamGenUnifiedCreateResponse_Title{Title: title},
	})
}

func (a *requestAdapter) EmitDescription(description string) {
	_ = a.target.Send(&api.StreamGenUnifiedCreateResponse{
		Event: &api.StreamGenUnifiedCreateResponse_Description{Description: description},
	})
}

func (a *requestAdapter) EmitGeocoded(geo *api.GeocodedLocation) {
	_ = a.target.Send(&api.StreamGenUnifiedCreateResponse{
		Event: &api.StreamGenUnifiedCreateResponse_Geocoded{Geocoded: geo},
	})
}

func (a *requestAdapter) EmitMediaReady(mediaIDs []string) {
	a.EmitMediaReadyWithCandidates(mediaIDs, nil)
}

func (a *requestAdapter) EmitMediaReadyWithCandidates(mediaIDs []string, candidates []*api.MediaCandidate) {
	_ = a.target.Send(&api.StreamGenUnifiedCreateResponse{
		Event: &api.StreamGenUnifiedCreateResponse_MediaReady{
			MediaReady: &api.MediaReady{
				MediaIds:   mediaIDs,
				Candidates: candidates,
			},
		},
	})
}

func (a *requestAdapter) EmitFinal(resp *api.GenRequestResponse) {
	a.out.Payload = &api.StreamGenUnifiedCreateFinal_Request{Request: resp}
}

func (a *requestAdapter) EmitError(code api.GenStreamErrorCode, message string) {
	_ = a.target.Send(&api.StreamGenUnifiedCreateResponse{
		Event: &api.StreamGenUnifiedCreateResponse_Error{
			Error: &api.GenStreamError{Code: code, Message: message},
		},
	})
}
