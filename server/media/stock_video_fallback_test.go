package media

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/health"
)

// videoFallbackMockProvider is a simple configurable mock for testing video fallback behavior.
type videoFallbackMockProvider struct {
	name      string
	result    *models.StockImage
	err       error
	healthErr error
}

func (m *videoFallbackMockProvider) GetStockVideo(_ context.Context, _ string) (*models.StockImage, error) {
	return m.result, m.err
}

func (m *videoFallbackMockProvider) SearchStockVideoCandidates(_ context.Context, _ string, _ int) ([]StockImageCandidate, error) {
	return nil, nil
}

func (m *videoFallbackMockProvider) GetStockVideoByID(_ context.Context, _ string) (*models.StockImage, error) {
	return nil, fmt.Errorf("not supported")
}

func (m *videoFallbackMockProvider) CheckHealth(_ context.Context) ([]*health.Status, error) {
	status := &health.Status{Name: m.name, Backend: m.name}
	if m.healthErr != nil {
		status.Error = m.healthErr.Error()
	}
	return []*health.Status{status}, nil
}

// slowVideoMockProvider simulates a video provider that takes time to respond.
type slowVideoMockProvider struct {
	name     string
	delay    time.Duration
	result   *models.StockImage
	err      error
	called   bool
	calledAt time.Time
}

func (m *slowVideoMockProvider) GetStockVideo(ctx context.Context, _ string) (*models.StockImage, error) {
	m.called = true
	m.calledAt = time.Now()

	select {
	case <-time.After(m.delay):
		return m.result, m.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (m *slowVideoMockProvider) SearchStockVideoCandidates(_ context.Context, _ string, _ int) ([]StockImageCandidate, error) {
	return nil, nil
}

func (m *slowVideoMockProvider) GetStockVideoByID(_ context.Context, _ string) (*models.StockImage, error) {
	return nil, fmt.Errorf("not supported")
}

func (m *slowVideoMockProvider) CheckHealth(_ context.Context) ([]*health.Status, error) {
	return []*health.Status{{Name: m.name, Backend: m.name}}, nil
}

func TestNewFallbackStockVideoProvider(t *testing.T) {
	t.Run("creates provider with single provider", func(t *testing.T) {
		providers := []StockVideoProvider{&videoFallbackMockProvider{}}
		names := []string{"provider1"}

		fp, err := NewFallbackStockVideoProvider(providers, names)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(fp.providers) != 1 {
			t.Errorf("Expected 1 provider, got %d", len(fp.providers))
		}
	})

	t.Run("creates provider with multiple providers", func(t *testing.T) {
		providers := []StockVideoProvider{
			&videoFallbackMockProvider{},
			&videoFallbackMockProvider{},
		}
		names := []string{"provider1", "provider2"}

		fp, err := NewFallbackStockVideoProvider(providers, names)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if len(fp.providers) != 2 {
			t.Errorf("Expected 2 providers, got %d", len(fp.providers))
		}
	})

	t.Run("returns error with no providers", func(t *testing.T) {
		_, err := NewFallbackStockVideoProvider([]StockVideoProvider{}, []string{})
		if err == nil {
			t.Error("Expected error with no providers")
		}
	})

	t.Run("returns error when providers and names length mismatch", func(t *testing.T) {
		providers := []StockVideoProvider{&videoFallbackMockProvider{}}
		names := []string{"provider1", "provider2"}

		_, err := NewFallbackStockVideoProvider(providers, names)
		if err == nil {
			t.Error("Expected error with mismatched lengths")
		}
	})
}

func TestFallbackStockVideoProvider_GetStockVideo(t *testing.T) {
	ctx := context.Background()

	t.Run("returns result from first provider on success", func(t *testing.T) {
		expected := &models.StockImage{Id: "stock-123", MediaId: "media-123"}

		provider1 := &videoFallbackMockProvider{name: "provider1", result: expected}
		provider2 := &videoFallbackMockProvider{name: "provider2", err: errors.New("should not be called")}

		fp, _ := NewFallbackStockVideoProvider(
			[]StockVideoProvider{provider1, provider2},
			[]string{"provider1", "provider2"},
		)

		result, err := fp.GetStockVideo(ctx, "test query")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result.Id != expected.Id {
			t.Errorf("Expected result ID %s, got %s", expected.Id, result.Id)
		}
	})

	t.Run("tries second provider when first fails", func(t *testing.T) {
		expected := &models.StockImage{Id: "stock-456", MediaId: "media-456"}

		provider1 := &videoFallbackMockProvider{name: "provider1", err: errors.New("provider1 failed")}
		provider2 := &videoFallbackMockProvider{name: "provider2", result: expected}

		fp, _ := NewFallbackStockVideoProvider(
			[]StockVideoProvider{provider1, provider2},
			[]string{"provider1", "provider2"},
		)

		result, err := fp.GetStockVideo(ctx, "test query")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result.Id != expected.Id {
			t.Errorf("Expected result ID %s, got %s", expected.Id, result.Id)
		}
	})

	t.Run("tries second provider when first is rate limited", func(t *testing.T) {
		// Unlike per-provider search loops, the fallback CHAIN should try the next
		// provider even on rate limit — Pexels and Pixabay have independent quotas.
		expected := &models.StockImage{Id: "stock-pixabay-789", MediaId: "media-789"}

		provider1 := &videoFallbackMockProvider{
			name: "pexels",
			err:  errors.New("pexels video API: rate limited"),
		}
		provider2 := &videoFallbackMockProvider{name: "pixabay", result: expected}

		fp, _ := NewFallbackStockVideoProvider(
			[]StockVideoProvider{provider1, provider2},
			[]string{"pexels", "pixabay"},
		)

		result, err := fp.GetStockVideo(ctx, "test query")
		if err != nil {
			t.Fatalf("Expected Pixabay to succeed after Pexels rate limit, got: %v", err)
		}
		if result.Id != expected.Id {
			t.Errorf("Expected result ID %s, got %s", expected.Id, result.Id)
		}
	})

	t.Run("tries all providers in order", func(t *testing.T) {
		expected := &models.StockImage{Id: "stock-789", MediaId: "media-789"}

		provider1 := &videoFallbackMockProvider{name: "provider1", err: errors.New("provider1 failed")}
		provider2 := &videoFallbackMockProvider{name: "provider2", err: errors.New("provider2 failed")}
		provider3 := &videoFallbackMockProvider{name: "provider3", result: expected}

		fp, _ := NewFallbackStockVideoProvider(
			[]StockVideoProvider{provider1, provider2, provider3},
			[]string{"provider1", "provider2", "provider3"},
		)

		result, err := fp.GetStockVideo(ctx, "test query")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if result.Id != expected.Id {
			t.Errorf("Expected result ID %s, got %s", expected.Id, result.Id)
		}
	})

	t.Run("returns error when all providers fail", func(t *testing.T) {
		provider1 := &videoFallbackMockProvider{name: "provider1", err: errors.New("provider1 failed")}
		provider2 := &videoFallbackMockProvider{name: "provider2", err: errors.New("provider2 failed")}

		fp, _ := NewFallbackStockVideoProvider(
			[]StockVideoProvider{provider1, provider2},
			[]string{"provider1", "provider2"},
		)

		result, err := fp.GetStockVideo(ctx, "test query")
		if err == nil {
			t.Fatal("Expected error when all providers fail")
		}
		if result != nil {
			t.Error("Expected nil result when all providers fail")
		}

		expectedPrefix := "all 2 stock video providers failed"
		if len(err.Error()) < len(expectedPrefix) || err.Error()[:len(expectedPrefix)] != expectedPrefix {
			t.Errorf("Expected error message to start with %q, got %q", expectedPrefix, err.Error())
		}
	})
}

func TestFallbackStockVideoProvider_CheckHealth_AllHealthy(t *testing.T) {
	provider1 := &videoFallbackMockProvider{name: "provider1"}
	provider2 := &videoFallbackMockProvider{name: "provider2"}

	fp, err := NewFallbackStockVideoProvider(
		[]StockVideoProvider{provider1, provider2},
		[]string{"provider1", "provider2"},
	)
	if err != nil {
		t.Fatalf("Failed to create fallback provider: %v", err)
	}

	statuses, err := fp.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("CheckHealth returned error: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}
	if statuses[0].Name != "provider1" {
		t.Errorf("expected first status name=provider1, got %s", statuses[0].Name)
	}
	if statuses[1].Name != "provider2" {
		t.Errorf("expected second status name=provider2, got %s", statuses[1].Name)
	}
}

func TestFallbackStockVideoProvider_CheckHealth_OneUnhealthy(t *testing.T) {
	provider1 := &videoFallbackMockProvider{name: "provider1"}
	provider2 := &videoFallbackMockProvider{name: "provider2", healthErr: errors.New("connection failed")}

	fp, err := NewFallbackStockVideoProvider(
		[]StockVideoProvider{provider1, provider2},
		[]string{"provider1", "provider2"},
	)
	if err != nil {
		t.Fatalf("Failed to create fallback provider: %v", err)
	}

	statuses, err := fp.CheckHealth(context.Background())
	if err != nil {
		t.Fatalf("CheckHealth returned unexpected Go error: %v", err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}
	if statuses[0].Error != "" {
		t.Errorf("expected provider1 to be healthy, got error: %s", statuses[0].Error)
	}
	if statuses[1].Error == "" {
		t.Error("expected provider2 to have error, but status.Error is empty")
	}
}

func TestNewFallbackStockVideoProviderWithTimeout(t *testing.T) {
	t.Run("creates provider with custom timeout", func(t *testing.T) {
		providers := []StockVideoProvider{&videoFallbackMockProvider{}}
		names := []string{"provider1"}
		customTimeout := 5 * time.Second

		fp, err := NewFallbackStockVideoProviderWithTimeout(providers, names, customTimeout)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if fp.perProviderTimeout != customTimeout {
			t.Errorf("Expected timeout %v, got %v", customTimeout, fp.perProviderTimeout)
		}
	})

	t.Run("default constructor uses DefaultPerProviderTimeout", func(t *testing.T) {
		providers := []StockVideoProvider{&videoFallbackMockProvider{}}
		names := []string{"provider1"}

		fp, err := NewFallbackStockVideoProvider(providers, names)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if fp.perProviderTimeout != DefaultPerProviderTimeout {
			t.Errorf("Expected timeout %v, got %v", DefaultPerProviderTimeout, fp.perProviderTimeout)
		}
	})
}

func TestFallbackStockVideoProvider_PerProviderTimeout(t *testing.T) {
	t.Run("slow provider times out and fallback succeeds", func(t *testing.T) {
		slowProvider := &slowVideoMockProvider{
			name:  "slow",
			delay: 500 * time.Millisecond,
		}
		fastProvider := &videoFallbackMockProvider{
			name:   "fast",
			result: &models.StockImage{Id: "stock-fast", MediaId: "media-fast"},
		}

		fp, _ := NewFallbackStockVideoProviderWithTimeout(
			[]StockVideoProvider{slowProvider, fastProvider},
			[]string{"slow", "fast"},
			10*time.Millisecond,
		)

		start := time.Now()
		result, err := fp.GetStockVideo(context.Background(), "test query")
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("Expected success from fast provider, got error: %v", err)
		}
		if result.Id != "stock-fast" {
			t.Errorf("Expected result from fast provider, got ID: %s", result.Id)
		}
		if elapsed > 100*time.Millisecond {
			t.Errorf("Operation took too long (%v), slow provider wasn't timed out properly", elapsed)
		}
		if !slowProvider.called {
			t.Error("Slow provider should have been called first")
		}
	})

	t.Run("all providers timing out returns error", func(t *testing.T) {
		provider1 := &slowVideoMockProvider{name: "slow1", delay: 500 * time.Millisecond}
		provider2 := &slowVideoMockProvider{name: "slow2", delay: 500 * time.Millisecond}

		fp, _ := NewFallbackStockVideoProviderWithTimeout(
			[]StockVideoProvider{provider1, provider2},
			[]string{"slow1", "slow2"},
			10*time.Millisecond,
		)

		start := time.Now()
		result, err := fp.GetStockVideo(context.Background(), "test query")
		elapsed := time.Since(start)

		if err == nil {
			t.Fatal("Expected error when all providers timeout")
		}
		if result != nil {
			t.Error("Expected nil result when all providers timeout")
		}
		if elapsed > 100*time.Millisecond {
			t.Errorf("Operation took too long (%v), providers weren't timed out properly", elapsed)
		}
		if !provider1.called || !provider2.called {
			t.Error("Both providers should have been called")
		}
	})
}

func TestFallbackStockVideoProvider_ParentContextCancellation(t *testing.T) {
	provider1 := &slowVideoMockProvider{name: "slow1", delay: 500 * time.Millisecond}
	provider2 := &slowVideoMockProvider{
		name:   "slow2",
		delay:  10 * time.Millisecond,
		result: &models.StockImage{Id: "stock-2", MediaId: "media-2"},
	}

	fp, _ := NewFallbackStockVideoProviderWithTimeout(
		[]StockVideoProvider{provider1, provider2},
		[]string{"slow1", "slow2"},
		100*time.Millisecond,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := fp.GetStockVideo(ctx, "test query")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("Expected error when parent context is cancelled")
	}
	if elapsed > 80*time.Millisecond {
		t.Errorf("Operation took too long (%v), should have stopped on parent context cancellation", elapsed)
	}
	if !provider1.called {
		t.Error("First provider should have been called")
	}
	if provider2.called {
		t.Error("Second provider should not have been called after parent context cancellation")
	}
}
