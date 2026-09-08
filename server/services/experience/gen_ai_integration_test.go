//go:build integration

package experience

import (
	"context"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/ai/aitest"
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/webfetch"
)

// genExperienceCallTimeout bounds each live-provider GenExperience call so a
// stuck LLM cannot consume the full go-test deadline. Without it a single
// stalled provider takes the whole package down: the `go test -timeout 20m` in
// test_go.yaml is per test binary, so one hung call fails every test in the
// package with a timeout panic rather than skipping one subtest.
//
// Sized off observed healthy latency — the three providers answer this prompt in
// 3s (Anthropic), 6s (Gemini) and 14s (OpenAI) — with headroom for a slow day.
const genExperienceCallTimeout = 90 * time.Second

// fakeEventPageContent returns mock webpage content for a fictional event.
// This simulates fetching an event page like from Eventbrite, Meetup, etc.
func fakeEventPageContent() *webfetch.PageContent {
	return &webfetch.PageContent{
		URL:         "https://example.com/events/summer-jazz-festival-2025",
		Title:       "Summer Jazz Festival 2025 - Downtown Arts District",
		Description: "Join us for three nights of amazing jazz music featuring local and international artists at Main Street Plaza.",
		BodyText: `Summer Jazz Festival 2025
Presented by the Downtown Arts District

Experience three unforgettable nights of world-class jazz music in the heart of downtown!

EVENT DETAILS:
Date: July 18-20, 2025
Gates Open: 5:00 PM
Music Starts: 6:00 PM each night

LOCATION:
Main Street Plaza
123 Main Street
Downtown Arts District, Boulder, CO 80302

LINEUP:

FRIDAY, JULY 18
- 6:00 PM: Boulder Jazz Collective (Local Opener)
- 8:00 PM: The Maria Santos Quartet

SATURDAY, JULY 19
- 6:00 PM: High Plains Jazz Band
- 8:00 PM: Grammy-nominated saxophonist Marcus Williams

SUNDAY, JULY 20
- 4:00 PM: Community Jazz Jam (open to all musicians!)
- 6:00 PM: Festival All-Stars Finale

TICKETS:
- Single Day Pass: $45
- Weekend Pass: $99 (BEST VALUE!)
- VIP Experience: $175 (includes meet & greet, premium seating, complimentary drinks)

WHAT TO BRING:
- Blankets and lawn chairs welcome
- Outside food/drinks NOT permitted
- Cash and cards accepted at food vendors

FOOD & BEVERAGES:
Over 15 local food trucks will be on site! Craft beer garden featuring Colorado breweries.

FAMILY FRIENDLY:
Kids 12 and under FREE with paying adult. Family zone with activities and early bedtime options.

WEATHER:
Rain or shine event. In case of severe weather, check social media for updates.

PARKING:
Free street parking available. Paid garage parking at 200 Main Street ($10/day).

MORE INFO:
Email: info@summerjazzfest.com
Phone: (303) 555-JAZZ
Website: www.summerjazzfest.com

Follow us: @SummerJazzFest

See you there!`,
	}
}

// TestGenExperienceFromWebpage_RealLLMs tests experience generation from webpage content
// using real LLM backends. This is an integration test that requires API keys.
func TestGenExperienceFromWebpage_RealLLMs(t *testing.T) {
	ctx := context.Background()

	// Skip if no API keys are available
	vertexProject := os.Getenv("VERTEX_AI_PROJECT")
	anthropicKey := os.Getenv("ANTHROPIC_API_KEY")
	openaiKey := os.Getenv("OPENAI_API_KEY")

	if vertexProject == "" && anthropicKey == "" && openaiKey == "" {
		t.Skip("Skipping integration test: no AI API keys available (VERTEX_AI_PROJECT, ANTHROPIC_API_KEY, or OPENAI_API_KEY)")
	}

	service, testStorage, _ := setupTestService(t)
	authCtx := createAuthenticatedContext("user123", "test@example.com", models.Role_ROLE_USER)

	// Create the test user in the database
	createTestUser(t, testStorage, "user123", "test@example.com", "Test User")

	// Setup mock web fetcher with fake event page
	mockFetcher := &webfetch.MockFetcher{
		FetchPageContentFunc: func(ctx context.Context, url string) (*webfetch.PageContent, error) {
			return fakeEventPageContent(), nil
		},
	}
	service.SetWebFetcher(mockFetcher)

	// Test with each available provider
	providers := []struct {
		name    string
		envKey  string
		envVal  string
		creator func() (ai.Provider, error)
	}{
		{
			name:   "Gemini",
			envKey: "VERTEX_AI_PROJECT",
			envVal: vertexProject,
			creator: func() (ai.Provider, error) {
				return ai.NewGeminiProvider(ctx, "vertexai/gemini-2.5-flash", vertexProject, "us-central1", ai.DefaultTemperature)
			},
		},
		{
			name:   "Anthropic",
			envKey: "ANTHROPIC_API_KEY",
			envVal: anthropicKey,
			creator: func() (ai.Provider, error) {
				return ai.NewAnthropicProvider(anthropicKey, "claude-haiku-4-5", ai.DefaultTemperature)
			},
		},
		{
			name:   "OpenAI",
			envKey: "OPENAI_API_KEY",
			envVal: openaiKey,
			creator: func() (ai.Provider, error) {
				return ai.NewOpenAIProvider(openaiKey, "gpt-5-mini", ai.DefaultTemperature)
			},
		},
	}

	for _, p := range providers {
		t.Run(p.name, func(t *testing.T) {
			if p.envVal == "" {
				t.Skipf("Skipping %s: %s not set", p.name, p.envKey)
			}

			provider, err := p.creator()
			if err != nil {
				t.Fatalf("Failed to create %s provider: %v", p.name, err)
			}
			service.SetAIProvider(provider)

			// Create request for webpage-based generation
			req := connect.NewRequest(&api.GenExperienceRequest{
				Prompt:             &api.GenExperienceRequest_WebsiteUrl{WebsiteUrl: "https://example.com/events/summer-jazz-festival-2025"},
				CurrentTimeUnixSec: time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC).Unix(), // June 1, 2025
				LatitudeDeg:        40.0150,                                             // Boulder, CO
				LongitudeDeg:       -105.2705,
			})

			t.Logf("\n========================================")
			t.Logf("Testing %s Provider", p.name)
			t.Logf("========================================")

			bounded, cancel := context.WithTimeout(authCtx, genExperienceCallTimeout)
			defer cancel()

			resp, err := service.GenExperience(bounded, req)
			if err != nil {
				// A deadline miss reads as transient ("context deadline
				// exceeded" matches errs.IsTransient), so a stalled provider
				// skips this subtest instead of failing the package.
				if aitest.IsTransientError(err) {
					t.Skipf("%s provider transient failure, skipping: %v", p.name, err)
				}
				t.Fatalf("%s GenExperience failed: %v", p.name, err)
			}

			// Log the results
			t.Logf("\n--- %s Results ---", p.name)
			t.Logf("Name: %s", resp.Msg.Name)
			t.Logf("Description: %s", resp.Msg.Description)
			t.Logf("LocationQuery: %s", resp.Msg.LocationQuery)
			t.Logf("TimeConfidence: %s", resp.Msg.TimeConfidence.String())

			if resp.Msg.ExtractedTimeUnixSec > 0 {
				extractedTime := time.Unix(resp.Msg.ExtractedTimeUnixSec, 0)
				t.Logf("ExtractedTime: %s", extractedTime.Format("2006-01-02 15:04:05"))
			} else {
				t.Logf("ExtractedTime: (not extracted)")
			}

			if resp.Msg.GeocodedLocation != nil {
				t.Logf("GeocodedLocation: %s (%.4f, %.4f)",
					resp.Msg.GeocodedLocation.Name,
					resp.Msg.GeocodedLocation.LatitudeDeg,
					resp.Msg.GeocodedLocation.LongitudeDeg)
			}

			// Basic validation
			if resp.Msg.Name == "" {
				t.Errorf("%s: Expected non-empty name", p.name)
			}
			if resp.Msg.Description == "" {
				t.Errorf("%s: Expected non-empty description", p.name)
			}
		})
	}
}
