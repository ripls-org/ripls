package simulation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/api/apiconnect"
)

// authRecorder captures the Authorization header observed on each request,
// keyed by RPC procedure path.
type authRecorder struct {
	mu   sync.Mutex
	byID map[string][]string
}

func (a *authRecorder) record(path, auth string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.byID == nil {
		a.byID = map[string][]string{}
	}
	a.byID[path] = append(a.byID[path], auth)
}

func (a *authRecorder) get(path string) []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, len(a.byID[path]))
	copy(out, a.byID[path])
	return out
}

// fakeCommunityService implements enough of the CommunityService to let a
// SetGearAvailability call succeed and record the caller's auth header.
type fakeCommunityService struct {
	apiconnect.UnimplementedCommunityServiceHandler
	rec *authRecorder
}

func (s *fakeCommunityService) SetGearAvailability(
	_ context.Context,
	req *connect.Request[api.SetGearAvailabilityRequest],
) (*connect.Response[api.SetGearAvailabilityResponse], error) {
	s.rec.record(apiconnect.CommunityServiceSetGearAvailabilityProcedure, req.Header().Get("Authorization"))
	return connect.NewResponse(&api.SetGearAvailabilityResponse{}), nil
}

// fakeTransferService lets an ExpressInterest call succeed and records the
// caller's auth header.
type fakeTransferService struct {
	apiconnect.UnimplementedTransferServiceHandler
	rec *authRecorder
}

func (s *fakeTransferService) ExpressInterest(
	_ context.Context,
	req *connect.Request[api.ExpressInterestRequest],
) (*connect.Response[api.ExpressInterestResponse], error) {
	s.rec.record(apiconnect.TransferServiceExpressInterestProcedure, req.Header().Get("Authorization"))
	return connect.NewResponse(&api.ExpressInterestResponse{
		Transfer: &api.Transfer{Id: "transfer-1"},
	}), nil
}

// TestExecuteExpressInterest_GiveawayUsesActorToken is the regression test
// for issue #1061. It verifies that the giveaway flow flips the gear to
// FOR_GIVEAWAY as the gear owner and calls ExpressInterest as the interested
// actor, with no token crossover between the two RPCs.
//
// Before the pool refactor: getClient(owner) mutated a shared client's auth
// token during the owner-side availability change and never restored it, so the
// subsequent ExpressInterest call ran as the owner and the server rejected
// "cannot express interest in your own gear." Under the pool model each
// user has an independent client; the same assertion still guards against
// any future code path that reintroduces token sharing.
func TestExecuteExpressInterest_GiveawayUsesActorToken(t *testing.T) {
	rec := &authRecorder{}

	mux := http.NewServeMux()
	commPath, commHandler := apiconnect.NewCommunityServiceHandler(&fakeCommunityService{rec: rec})
	transPath, transHandler := apiconnect.NewTransferServiceHandler(&fakeTransferService{rec: rec})
	mux.Handle(commPath, commHandler)
	mux.Handle(transPath, transHandler)

	ts := httptest.NewUnstartedServer(mux)
	ts.EnableHTTP2 = true
	ts.StartTLS()
	defer ts.Close()

	const (
		actorEmail  = "alice@example.test"
		ownerEmail  = "bob@example.test"
		actorToken  = "actor-token-alice"
		ownerToken  = "owner-token-bob"
		gearRef     = "gear-test-1"
		giveawayRef = "gear-test-1-giveaway-42"
		gearID      = "gear-id-1"
		communityID = "community-id-1"
	)

	// Seed state with the two users, gear metadata, and community mapping.
	state := NewState()
	state.UserTokens[actorEmail] = actorToken
	state.UserTokens[ownerEmail] = ownerToken
	state.GearIDs[gearRef] = gearID
	state.GearOwners[gearRef] = ownerEmail
	state.CommunityIDs["Test Community"] = communityID

	pool := NewClientPool(ts.URL, state.UserTokens)
	// Swap each pool client's transport to accept the httptest TLS cert.
	for _, c := range pool.clients {
		c.transport.base = ts.Client().Transport
	}
	pool.SetTimestamp(time.Unix(1_700_000_000, 0))

	client := pool.For(actorEmail)
	getClient := pool.For

	step := ActivityStep{
		Time:          time.Unix(1_700_000_000, 0),
		Actor:         actorEmail,
		CommunityName: "Test Community",
		Action:        ActionExpressInterestGiveaway,
		Ref:           giveawayRef,
	}

	if err := executeExpressInterest(context.Background(), client, state, step, true, getClient); err != nil {
		t.Fatalf("executeExpressInterest returned error: %v", err)
	}

	shareAuths := rec.get(apiconnect.CommunityServiceSetGearAvailabilityProcedure)
	interestAuths := rec.get(apiconnect.TransferServiceExpressInterestProcedure)

	wantShare := "Bearer " + ownerToken
	wantInterest := "Bearer " + actorToken

	if len(shareAuths) != 1 || shareAuths[0] != wantShare {
		t.Errorf("SetGearAvailability auth = %v, want %q", shareAuths, wantShare)
	}
	if len(interestAuths) != 1 || interestAuths[0] != wantInterest {
		t.Errorf("ExpressInterest auth = %v, want %q — this is the #1061 regression guard",
			interestAuths, wantInterest)
	}
}
