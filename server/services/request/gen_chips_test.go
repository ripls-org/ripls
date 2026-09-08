package request

import (
	"context"
	"errors"
	"testing"

	"go.ripls.org/ripls/server/ai"
	"go.ripls.org/ripls/server/logging"
)

// chipMockAIProvider extends ai.MockProvider with a configurable
// GenerateRequestSuggestions override scoped to a single test, so the
// chip-plumbing helper can be exercised without touching real AI APIs.
func chipMockAIProvider(suggestions *ai.RequestSuggestionResult, suggestionsErr error) *ai.MockProvider {
	m := ai.NewMockProvider()
	m.GenerateRequestSuggestionsFunc = func(_ context.Context, _, _, _ string) (*ai.RequestSuggestionResult, error) {
		return suggestions, suggestionsErr
	}
	return m
}

func TestGenRequestSuggestionsBestEffort_ReturnsChipsOnSuccess(t *testing.T) {
	s := &Service{
		aiProvider: chipMockAIProvider(&ai.RequestSuggestionResult{
			AdditionalAsks:  []string{"Cargo straps"},
			BreakdownPieces: []string{"Stump grinder", "Haul debris", "Provide lunch"},
			OfferIdeas:      []string{"Drive truck", "Watch kids"},
			SeedNeeds:       []string{"Yard cleanup help", "Wheelbarrow"},
		}, nil),
	}
	logger := logging.Default()

	asks, pieces, ideas, seedNeeds := s.genRequestSuggestionsBestEffort(
		context.Background(), "Yard cleanup", "Need help after storm", logger,
	)

	if got, want := seedNeeds, []string{"Yard cleanup help", "Wheelbarrow"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("seed_needs: got %v, want %v", got, want)
	}
	if got, want := len(pieces), 3; got != want {
		t.Errorf("breakdown_pieces: got %d, want %d", got, want)
	}
	if got, want := len(asks), 1; got != want {
		t.Errorf("additional_asks: got %d, want %d", got, want)
	}
	if got, want := len(ideas), 2; got != want {
		t.Errorf("offer_ideas: got %d, want %d", got, want)
	}
}

func TestGenRequestSuggestionsBestEffort_ReturnsEmptyOnEmptyTitle(t *testing.T) {
	s := &Service{
		aiProvider: chipMockAIProvider(&ai.RequestSuggestionResult{
			BreakdownPieces: []string{"x"},
		}, nil),
	}
	asks, pieces, ideas, seedNeeds := s.genRequestSuggestionsBestEffort(
		context.Background(), "", "ignored", logging.Default(),
	)
	if asks != nil || pieces != nil || ideas != nil || seedNeeds != nil {
		t.Errorf("expected all-empty results for empty title, got %v / %v / %v / %v", asks, pieces, ideas, seedNeeds)
	}
}

func TestGenRequestSuggestionsBestEffort_ReturnsEmptyOnAIError(t *testing.T) {
	s := &Service{
		aiProvider: chipMockAIProvider(nil, errors.New("LLM unavailable")),
	}
	asks, pieces, ideas, seedNeeds := s.genRequestSuggestionsBestEffort(
		context.Background(), "Yard cleanup", "After storm", logging.Default(),
	)
	if asks != nil || pieces != nil || ideas != nil || seedNeeds != nil {
		t.Errorf("expected all-empty results on AI error, got %v / %v / %v / %v", asks, pieces, ideas, seedNeeds)
	}
}

func TestGenRequestSuggestionsBestEffort_ReturnsEmptyWhenAIProviderNil(t *testing.T) {
	s := &Service{aiProvider: nil}
	asks, pieces, ideas, seedNeeds := s.genRequestSuggestionsBestEffort(
		context.Background(), "Yard cleanup", "After storm", logging.Default(),
	)
	if asks != nil || pieces != nil || ideas != nil || seedNeeds != nil {
		t.Errorf("expected all-empty results when provider unconfigured, got %v / %v / %v / %v", asks, pieces, ideas, seedNeeds)
	}
}
