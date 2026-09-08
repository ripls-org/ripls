//go:build benchmark

// Real end-to-end benchmark measuring GenExperience wall-clock (AI call +
// post-AI Mapbox geocoding + stock-media fetch) against live external
// providers. Three test variants — one per AI provider — share an
// identical body so numbers are directly comparable across providers.
// Gated by the `benchmark` build tag so it never runs in CI or default
// test suites — invoke explicitly.
//
// IMPORTANT: use the PROD Mapbox and Pexels keys so the measurement
// reflects real end-user quota / cache behavior. The AI provider keys can
// stay on the dev project (just metered LLM usage).
//
// DEV and PROD below are your two project ids; scripts/gcp_project.sh
// resolves them from .env.local, so `DEV=$(scripts/gcp_project.sh dev)`.
//
//	export ANTHROPIC_API_KEY="$(gcloud secrets versions access latest --secret=anthropic-api-key --project=$DEV)"
//	export OPENAI_API_KEY="$(gcloud secrets versions access latest --secret=openai-api-key --project=$DEV)"
//	export VERTEX_AI_PROJECT="$DEV"
//	export VERTEX_AI_LOCATION="us-central1"
//	export MAPBOX_ACCESS_TOKEN="$(gcloud secrets versions access latest --secret=mapbox-access-token --project=$PROD)"
//	export PEXELS_API_KEY="$(gcloud secrets versions access latest --secret=pexels-api-key --project=$PROD)"
//
//	go test -tags=benchmark -v -count=1 -timeout 5m -run TestGenExperienceRealFanoutBenchmark ./server/services/experience/
//
// Use -run TestGenExperienceRealFanoutBenchmark_Anthropic etc. to run a
// single provider. The -count=1 flag bypasses Go's test-result cache.
//
// To capture pre-refactor baselines, run this same test on a branch that
// predates the relevant refactor — `service.GenExperience` has the same
// public API pre- and post-refactor, so the test compiles unchanged.

package experience

import (
	"context"
	"os"
	"sort"
	"testing"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	cebus "go.ripls.org/ripls/server/community_event_bus"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/location"
	"go.ripls.org/ripls/server/media"
	"go.ripls.org/ripls/server/notifications"
	"go.ripls.org/ripls/server/pubsub"
)

// TestGenExperienceRealFanoutBenchmark_Anthropic measures end-to-end
// wall-clock through service.GenExperience with the Anthropic provider
// (claude-haiku-4-5).
func TestGenExperienceRealFanoutBenchmark_Anthropic(t *testing.T) {
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	if anthropicKey == "" {
		t.Skip("Requires ANTHROPIC_API_KEY")
	}
	provider, err := ai.NewAnthropicProvider(anthropicKey, "claude-haiku-4-5", ai.DefaultTemperature)
	if err != nil {
		t.Fatalf("NewAnthropicProvider: %v", err)
	}
	runFanoutBenchmark(t, "anthropic/claude-haiku-4-5", provider)
}

// TestGenExperienceRealFanoutBenchmark_OpenAI measures end-to-end
// wall-clock through service.GenExperience with the OpenAI provider.
// Defaults to gpt-5-mini; override via OPENAI_MODEL.
func TestGenExperienceRealFanoutBenchmark_OpenAI(t *testing.T) {
	openaiKey := os.Getenv("OPENAI_API_KEY")
	if openaiKey == "" {
		t.Skip("Requires OPENAI_API_KEY")
	}
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-5-mini"
	}
	provider, err := ai.NewOpenAIProvider(openaiKey, model, ai.DefaultTemperature)
	if err != nil {
		t.Fatalf("NewOpenAIProvider: %v", err)
	}
	runFanoutBenchmark(t, "openai/"+model, provider)
}

// TestGenExperienceRealFanoutBenchmark_Gemini measures end-to-end
// wall-clock through service.GenExperience with the Gemini provider via
// Vertex AI. Defaults to vertexai/gemini-2.5-flash; override via
// GEMINI_MODEL. Requires Application Default Credentials
// (`gcloud auth application-default login`) for Vertex AI access.
func TestGenExperienceRealFanoutBenchmark_Gemini(t *testing.T) {
	project := os.Getenv("VERTEX_AI_PROJECT")
	if project == "" {
		project = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}
	if project == "" {
		t.Skip("set VERTEX_AI_PROJECT (or GOOGLE_CLOUD_PROJECT) to the project that hosts Vertex AI")
	}
	location := os.Getenv("VERTEX_AI_LOCATION")
	if location == "" {
		location = "us-central1"
	}
	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "vertexai/gemini-2.5-flash"
	}

	provider, err := ai.NewGeminiProvider(context.Background(), model, project, location, ai.DefaultTemperature)
	if err != nil {
		t.Skipf("NewGeminiProvider: %v (run `gcloud auth application-default login` if ADC is expired)", err)
	}
	runFanoutBenchmark(t, "gemini/"+model, provider)
}

// runFanoutBenchmark is the shared body for all three provider variants.
// Identical setup, prompt, sample count, and reporting so numbers are
// directly comparable across providers.
func runFanoutBenchmark(t *testing.T, label string, provider ai.Provider) {
	t.Helper()

	mapboxKey := os.Getenv("MAPBOX_ACCESS_TOKEN")
	pexelsKey := os.Getenv("PEXELS_API_KEY")
	if mapboxKey == "" || pexelsKey == "" {
		t.Skip("Requires MAPBOX_ACCESS_TOKEN and PEXELS_API_KEY")
	}

	testStorage := setupTestStorage(t)
	testBucket := setupTestBucket(t)
	topic := pubsub.NewMemTopic[*cebus.PublishedEvent](cebus.TopicName)
	bus := cebus.NewInProcessBus(testStorage, topic)
	service := New(testStorage, testBucket, notifications.NewMockService(), bus)
	service.SetAIProvider(provider)
	service.SetLocationProvider(location.NewMapboxClient(mapboxKey))
	pexelsClient := media.NewPexelsClient(pexelsKey)
	pexelsProvider := media.NewPexelsProvider(pexelsClient, testStorage, testBucket)
	service.SetStockImageryProvider(pexelsProvider)
	service.SetStockVideoProvider(pexelsProvider)

	createTestUser(t, testStorage, "benchuser", "bench@test.com", "Bench User")
	authCtx := createAuthenticatedContext("benchuser", "bench@test.com", models.Role_ROLE_USER)

	// Prompt chosen to trigger both fan-out branches: non-USER_PRIMARY_LOCATION
	// location (Mapbox hits) + search_keywords (stock-media hits).
	prompt := "hiking tomorrow morning at Chautauqua"
	req := connect.NewRequest(&api.GenExperienceRequest{
		Prompt:             &api.GenExperienceRequest_Text{Text: prompt},
		CurrentTimeUnixSec: time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC).Unix(),
		LatitudeDeg:        40.015, // Boulder, CO — biases Mapbox to local results
		LongitudeDeg:       -105.27,
	})

	// Samples chosen empirically: 3 is enough to see the median cleanly while
	// keeping cost bounded. Each call is ~2-4 s and writes one stock-media
	// copy to the test bucket.
	const samples = 3
	durations := make([]time.Duration, 0, samples)

	for i := 0; i < samples; i++ {
		start := time.Now()
		resp, err := service.GenExperience(authCtx, req)
		elapsed := time.Since(start)
		if err != nil {
			t.Fatalf("[%s] sample %d: GenExperience failed: %v", label, i+1, err)
		}
		durations = append(durations, elapsed)

		hasGeo := resp.Msg.GeocodedLocation != nil
		mediaCount := len(resp.Msg.MediaIds)
		t.Logf("[%s] sample %d/%d: elapsed=%v title=%q has_geocoded=%v media_ids=%d",
			label, i+1, samples, elapsed.Round(time.Millisecond), resp.Msg.Name, hasGeo, mediaCount)

		// Sanity: both fan-out branches must hit for the measurement to be
		// comparable across runs. If either dropped, the median below would
		// reflect serial + one branch, not serial + both.
		if !hasGeo {
			t.Errorf("[%s] sample %d: expected Mapbox geocoding to succeed", label, i+1)
		}
		if mediaCount == 0 {
			t.Errorf("[%s] sample %d: expected stock-media fetch to produce at least one media_id", label, i+1)
		}
	}

	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	var sum time.Duration
	for _, d := range durations {
		sum += d
	}
	median := durations[len(durations)/2]
	mean := sum / time.Duration(len(durations))

	t.Log("============================================")
	t.Logf("GenExperience steps 7+8+9 real wall-clock — %s", label)
	t.Logf("  samples: %d", samples)
	t.Logf("  prompt:  %q", prompt)
	t.Logf("  median:  %v", median.Round(time.Millisecond))
	t.Logf("  mean:    %v", mean.Round(time.Millisecond))
	t.Logf("  min:     %v", durations[0].Round(time.Millisecond))
	t.Logf("  max:     %v", durations[len(durations)-1].Round(time.Millisecond))
	t.Log("============================================")
}
