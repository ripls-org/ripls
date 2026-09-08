package ratelimit

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"golang.org/x/time/rate"
)

// TestDefaultLoginRules_Count verifies the expected number of rules are defined.
func TestDefaultLoginRules_Count(t *testing.T) {
	rules := DefaultLoginRules()
	if len(rules) == 0 {
		t.Fatal("expected non-empty DefaultLoginRules")
	}
	// All rules must have a non-empty procedure and at least one limit.
	for _, r := range rules {
		if r.Procedure == "" {
			t.Errorf("rule has empty Procedure: %+v", r)
		}
		if r.IPLimit == nil && r.EmailLimit == nil && r.UserIDLimit == nil {
			t.Errorf("rule %q has no limits set", r.Procedure)
		}
	}
}

// TestDefaultMediaRules_AddMediaFromURL verifies the AddMediaFromURL rule
// exists, uses the user-id dimension (the right key for an authenticated
// procedure), and has a sane budget.
func TestDefaultMediaRules_AddMediaFromURL(t *testing.T) {
	rules := DefaultMediaRules()
	if len(rules) == 0 {
		t.Fatal("expected non-empty DefaultMediaRules")
	}
	var found *Rule
	for i := range rules {
		if strings.HasSuffix(rules[i].Procedure, "/AddMediaFromURL") {
			found = &rules[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected DefaultMediaRules to include AddMediaFromURL, got %+v", rules)
	}
	if found.UserIDLimit == nil {
		t.Error("AddMediaFromURL rule must set UserIDLimit (authenticated procedure)")
	}
	if found.IPLimit != nil {
		t.Error("AddMediaFromURL rule should not set IPLimit — user id is the right key")
	}
	if found.UserIDLimit != nil && found.UserIDLimit.Burst < 1 {
		t.Errorf("AddMediaFromURL UserIDLimit.Burst must be >= 1, got %d", found.UserIDLimit.Burst)
	}
}

// TestAllRules_IncludesEveryDefaultSet verifies AllRules concatenates all
// the Default*Rules constructors. This catches the case where a new
// constructor is added and the author forgets to append it to AllRules.
func TestAllRules_IncludesEveryDefaultSet(t *testing.T) {
	all := AllRules()
	procs := make(map[string]bool, len(all))
	for _, r := range all {
		procs[r.Procedure] = true
	}
	for _, r := range DefaultLoginRules() {
		if !procs[r.Procedure] {
			t.Errorf("AllRules missing login rule for %q", r.Procedure)
		}
	}
	for _, r := range DefaultMediaRules() {
		if !procs[r.Procedure] {
			t.Errorf("AllRules missing media rule for %q", r.Procedure)
		}
	}
}

// TestAllRules_AddMediaFromURL_EndToEnd exercises the full interceptor
// path against the real AllRules registry: an authenticated client can
// burn through the burst, then is rejected with CodeResourceExhausted +
// Retry-After. Catches both Phase 2 wiring (the rule loaded via
// AllRules) and Phase 3 (the rule itself).
func TestAllRules_AddMediaFromURL_EndToEnd(t *testing.T) {
	clk := newFakeClock(time.Now())
	s := makeState(clk)
	fn := buildInterceptorWithState(s, AllRules()...)

	// Find the AddMediaFromURL procedure name from the rule set.
	var proc string
	for _, r := range AllRules() {
		if strings.HasSuffix(r.Procedure, "/AddMediaFromURL") {
			proc = r.Procedure
			break
		}
	}
	if proc == "" {
		t.Fatal("AllRules has no AddMediaFromURL entry")
	}

	req := &fakeRequest{spec: connect.Spec{Procedure: proc}, anyMsg: &msgNoEmail{}}
	ctx := ctxWithUserID("e2e-user")

	// Find the budget so we know how many requests should succeed.
	var burst int
	for _, r := range AllRules() {
		if r.Procedure == proc && r.UserIDLimit != nil {
			burst = r.UserIDLimit.Burst
			break
		}
	}
	if burst < 1 {
		t.Fatal("could not determine burst for AddMediaFromURL")
	}

	for i := 0; i < burst; i++ {
		if _, err := fn(ctx, req); err != nil {
			t.Fatalf("request %d should be allowed, got: %v", i+1, err)
		}
	}

	_, err := fn(ctx, req)
	if err == nil {
		t.Fatal("expected rate-limit rejection after burst exhausted")
	}
	if connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Errorf("expected CodeResourceExhausted, got %v", connect.CodeOf(err))
	}
	var connectErr *connect.Error
	if !connectAs(err, &connectErr) {
		t.Fatalf("expected *connect.Error, got %T", err)
	}
	if connectErr.Meta().Get("Retry-After") == "" {
		t.Error("expected Retry-After header on AddMediaFromURL rejection")
	}
}

// TestMetricsOnly_DoesNotRejectButLogs verifies the soak-mode contract:
// state advances and the "rate_limited" warn log fires with
// mode="metrics_only", but the request is allowed through.
func TestMetricsOnly_DoesNotRejectButLogs(t *testing.T) {
	clk := newFakeClock(time.Now())
	s := makeState(clk)

	const burst = 1
	fn := buildInterceptorWithState(s, Rule{
		Procedure:   testProc,
		UserIDLimit: &Budget{Rate: rate.Every(time.Hour), Burst: burst, MetricsOnly: true},
	})

	req := &fakeRequest{spec: connect.Spec{Procedure: testProc}, anyMsg: &msgNoEmail{}}
	var buf bytes.Buffer
	ctx := ctxWithUserIDAndLogger("soak-user", &buf)

	// Burn the burst (these should pass).
	for i := 0; i < burst; i++ {
		if _, err := fn(ctx, req); err != nil {
			t.Fatalf("under-burst request %d should be allowed: %v", i+1, err)
		}
	}

	// Next request would have been rejected in enforce mode; metrics-only
	// must let it through with no error.
	if _, err := fn(ctx, req); err != nil {
		t.Fatalf("metrics-only must not return an error, got: %v", err)
	}

	// The log must still record what would have been rejected — with the
	// soak mode, never enforce (conflated modes would poison log queries
	// comparing "soaking" vs "enforcing" populations).
	entries := rateLimitedLogs(&buf)
	if len(entries) != 1 {
		t.Fatalf("expected exactly one rate_limited log, got %d", len(entries))
	}
	if got := entries[0]["mode"]; got != "metrics_only" {
		t.Errorf("mode = %v, want metrics_only", got)
	}
	if got := entries[0]["key_type"]; got != "user_id" {
		t.Errorf("key_type = %v, want user_id", got)
	}
}

// TestDefaultGenAIRules_AllShipMetricsOnly verifies the gen-AI rules
// ship with MetricsOnly = true so they can soak in production before
// being flipped to enforce. Catches the regression where someone tunes
// a budget without remembering to keep the soak gate.
func TestDefaultGenAIRules_AllShipMetricsOnly(t *testing.T) {
	rules := DefaultGenAIRules()
	if len(rules) == 0 {
		t.Fatal("expected non-empty DefaultGenAIRules")
	}
	for _, r := range rules {
		if r.UserIDLimit == nil {
			t.Errorf("rule %q must set UserIDLimit", r.Procedure)
			continue
		}
		if !r.UserIDLimit.MetricsOnly {
			t.Errorf("rule %q must ship with MetricsOnly=true (soak before enforce)", r.Procedure)
		}
	}
}

// TestDefaultGenAIRules_CoversAllStreamGenProcedures verifies all five
// wire-exposed StreamGen* procedures have a rule. The internal
// *FromText/*FromImage/*FromWebpage methods on each service are not
// Connect RPCs; they're called as direct Go method invocations by
// StreamGenUnifiedCreate after classification, so they don't need
// (and can't have) interceptor-level limits.
func TestDefaultGenAIRules_CoversAllStreamGenProcedures(t *testing.T) {
	want := []string{
		"/ripls.api.UnifiedCreateService/StreamGenUnifiedCreate",
		"/ripls.api.ExperienceService/StreamGenExperience",
		"/ripls.api.GearService/StreamGenGear",
		"/ripls.api.RequestService/StreamGenRequest",
		"/ripls.api.CommunityService/StreamGenCommunity",
	}
	got := make(map[string]bool)
	for _, r := range DefaultGenAIRules() {
		got[r.Procedure] = true
	}
	for _, p := range want {
		if !got[p] {
			t.Errorf("DefaultGenAIRules missing %q", p)
		}
	}
}

// TestAllRules_IncludesGenAI verifies AllRules concatenates DefaultGenAIRules.
func TestAllRules_IncludesGenAI(t *testing.T) {
	all := AllRules()
	procs := make(map[string]bool, len(all))
	for _, r := range all {
		procs[r.Procedure] = true
	}
	for _, r := range DefaultGenAIRules() {
		if !procs[r.Procedure] {
			t.Errorf("AllRules missing gen-AI rule for %q", r.Procedure)
		}
	}
}

// TestAllRulesAsMetricsOnly_ForcesEveryBudgetToMetricsOnly verifies the
// dev transform: every non-nil budget on every rule has MetricsOnly = true.
// Catches the regression where a new dimension is added but the
// transformer isn't taught to shadow it.
func TestAllRulesAsMetricsOnly_ForcesEveryBudgetToMetricsOnly(t *testing.T) {
	devRules := AllRulesAsMetricsOnly()
	if len(devRules) == 0 {
		t.Fatal("expected AllRulesAsMetricsOnly to be non-empty")
	}
	for _, r := range devRules {
		if r.IPLimit != nil && !r.IPLimit.MetricsOnly {
			t.Errorf("rule %q IPLimit must be MetricsOnly after shadowing", r.Procedure)
		}
		if r.EmailLimit != nil && !r.EmailLimit.MetricsOnly {
			t.Errorf("rule %q EmailLimit must be MetricsOnly after shadowing", r.Procedure)
		}
		if r.UserIDLimit != nil && !r.UserIDLimit.MetricsOnly {
			t.Errorf("rule %q UserIDLimit must be MetricsOnly after shadowing", r.Procedure)
		}
	}
}

// TestAllRulesAsMetricsOnly_CoversEveryAllRulesProcedure verifies the
// shadow transformer doesn't drop any rule. Dev must see every
// procedure prod sees.
func TestAllRulesAsMetricsOnly_CoversEveryAllRulesProcedure(t *testing.T) {
	all := AllRules()
	dev := AllRulesAsMetricsOnly()
	if len(dev) != len(all) {
		t.Fatalf("expected len(AllRulesAsMetricsOnly)=%d to equal len(AllRules)=%d", len(dev), len(all))
	}
	devProcs := make(map[string]bool, len(dev))
	for _, r := range dev {
		devProcs[r.Procedure] = true
	}
	for _, r := range all {
		if !devProcs[r.Procedure] {
			t.Errorf("AllRulesAsMetricsOnly missing procedure %q present in AllRules", r.Procedure)
		}
	}
}

// TestAllRulesAsMetricsOnly_DoesNotMutateSource verifies the transform
// returns a deep copy: mutating the dev set must not alter AllRules.
func TestAllRulesAsMetricsOnly_DoesNotMutateSource(t *testing.T) {
	before := AllRules()
	_ = AllRulesAsMetricsOnly()
	after := AllRules()

	// Compare a representative enforce-mode budget from the source set
	// to verify it wasn't flipped to MetricsOnly by the shadow call.
	var foundEnforce bool
	for i, r := range before {
		if r.IPLimit != nil && !r.IPLimit.MetricsOnly {
			foundEnforce = true
			if after[i].IPLimit.MetricsOnly {
				t.Errorf("AllRulesAsMetricsOnly mutated source: %q IPLimit promoted to MetricsOnly", r.Procedure)
			}
		}
		if r.UserIDLimit != nil && !r.UserIDLimit.MetricsOnly {
			foundEnforce = true
			if after[i].UserIDLimit.MetricsOnly {
				t.Errorf("AllRulesAsMetricsOnly mutated source: %q UserIDLimit promoted to MetricsOnly", r.Procedure)
			}
		}
	}
	if !foundEnforce {
		t.Skip("no enforce-mode budgets in AllRules; can't verify shadow non-mutation")
	}
}

// TestShadowBudget_TagsSourceEnforceAsDevShadow verifies an enforce-
// mode source budget produces a shadow whose mode label is "dev_shadow",
// while a MetricsOnly source budget produces a shadow whose mode label
// stays "metrics_only".
func TestShadowBudget_TagsSourceEnforceAsDevShadow(t *testing.T) {
	enforce := &Budget{Rate: rate.Every(time.Minute), Burst: 1}
	soaking := &Budget{Rate: rate.Every(time.Minute), Burst: 1, MetricsOnly: true}

	es := shadowBudget(enforce)
	if !es.MetricsOnly {
		t.Error("shadowBudget(enforce).MetricsOnly must be true")
	}
	if got := budgetMode(es); got != "dev_shadow" {
		t.Errorf("shadowed enforce budget mode = %q, want dev_shadow", got)
	}

	ss := shadowBudget(soaking)
	if !ss.MetricsOnly {
		t.Error("shadowBudget(soaking).MetricsOnly must be true")
	}
	if got := budgetMode(ss); got != "metrics_only" {
		t.Errorf("shadowed MetricsOnly budget mode = %q, want metrics_only", got)
	}
}

// TestRejection_DevShadowMode_DoesNotRejectAndLogsDevShadow verifies
// the full path through a dev-shadowed rule: state advances, the
// "rate_limited" warn log fires with mode="dev_shadow", and the request
// is NOT rejected (dev never sees 429s from rate-limiting).
func TestRejection_DevShadowMode_DoesNotRejectAndLogsDevShadow(t *testing.T) {
	clk := newFakeClock(time.Now())
	s := makeState(clk)

	// Source budget is enforce-mode; shadow it for dev.
	source := Rule{
		Procedure:   testProc,
		UserIDLimit: &Budget{Rate: rate.Every(time.Hour), Burst: 0}, // reject every request
	}
	dev := Rule{
		Procedure:   source.Procedure,
		UserIDLimit: shadowBudget(source.UserIDLimit),
	}
	fn := buildInterceptorWithState(s, dev)

	req := &fakeRequest{spec: connect.Spec{Procedure: testProc}, anyMsg: &msgNoEmail{}}
	var buf bytes.Buffer
	ctx := ctxWithUserIDAndLogger("provisional-user", &buf)

	// Dev shadow must not reject.
	if _, err := fn(ctx, req); err != nil {
		t.Fatalf("dev shadow must not return an error, got: %v", err)
	}

	// Exactly one log, and its mode must be dev_shadow — not enforce or
	// metrics_only, which would conflate populations in log queries.
	entries := rateLimitedLogs(&buf)
	if len(entries) != 1 {
		t.Fatalf("expected exactly one rate_limited log, got %d", len(entries))
	}
	if got := entries[0]["mode"]; got != "dev_shadow" {
		t.Errorf("mode = %v, want dev_shadow", got)
	}
	if got := entries[0]["key_type"]; got != "user_id" {
		t.Errorf("key_type = %v, want user_id", got)
	}
}
