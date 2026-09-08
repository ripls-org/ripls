package experience

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
)

// GenerateCompletionSummary is retired (#2936). It used to write the
// host's post-event recap for them, in first person and in their voice.
//
// Experience.completion_summary survives and is still posted to the
// event conversation on confirm — it is now whatever the host actually
// typed, and stays empty when they type nothing. The handler stays as an
// Unimplemented stub only because the generated ExperienceServiceHandler
// interface still declares the method; TODO(#2938): delete this file and
// the RPC once the release carrying #2936 has soaked, per
// docs/proto_conventions.md.
func (s *Service) GenerateCompletionSummary(
	_ context.Context,
	_ *connect.Request[api.GenerateCompletionSummaryRequest],
) (*connect.Response[api.GenerateCompletionSummaryResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented,
		fmt.Errorf("completion summaries are no longer generated"))
}
