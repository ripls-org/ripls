package chat

import (
	"context"
	"fmt"

	"connectrpc.com/connect"

	api "go.ripls.org/ripls/server/gen/ripls/api"
)

// GenConversationSummary is retired (#2936). It used to return an
// LLM-written recap of a conversation.
//
// The RPC had no production caller on either platform: the only client
// path reached it through GearSharingGiveawayMixin.loadGiveawaySummary,
// which nothing invoked, and the summary it produced had no widget
// reading it. The handler stays as an Unimplemented stub only because
// the generated ChatServiceHandler interface still declares the method;
// TODO(#2938): delete this file and the RPC once the release carrying
// #2936 has soaked, per docs/proto_conventions.md.
func (s *Service) GenConversationSummary(
	_ context.Context,
	_ *connect.Request[api.GenConversationSummaryRequest],
) (*connect.Response[api.GenConversationSummaryResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented,
		fmt.Errorf("conversation summaries are no longer generated"))
}
