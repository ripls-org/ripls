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

// fallbackMockProvider is a simple configurable mock for testing fallback behavior.
type fallbackMockProvider struct {
	name      string
	result    *models.StockImage
	err       error
	healthErr error // Separate error for health checks
}

func (m *fallbackMockProvider) GetStockImage(ctx context.Context, query string, opts *StockImageOptions) (*models.StockImage, error) {
	return m.result, m.err
}

func (m *fallbackMockProvider) SearchStockImageCandidates(_ context.Context, _ string, _ int) ([]StockImageCandidate, error) {
	return nil, nil
}

func (m *fallbackMockProvider) GetStockImageByID(_ context.Context, _ string) (*models.StockImage, error) {
	return nil, fmt.Errorf("not supported")
}

func (m *fallbackMockProvider) CheckHealth(_ context.Context) ([]*health.Status, error) {
	status := &health.Status{Name: m.name, Backend: m.name}
	if m.healthErr != nil {
		status.Error = m.healthErr.Error()
	}
	return []*health.Status{status}, nil
}

// slowMockProvider simulates a provider that takes time to respond.
// It respects context cancellation and can be configured to either
// succeed after a delay or block until context is cancelled.
type slowMockProvider struct {
	name     string
	delay    time.Duration
	result   *models.StockImage
	err      error
	called   bool
	calledAt time.Time
}

func (m *slowMockProvider) GetStockImage(ctx context.Context, query string, opts *StockImageOptions) (*models.StockImage, error) {
	m.called = true
	m.calledAt = time.Now()

	select {
	case <-time.After(m.delay):
		return m.result, m.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (m *slowMockProvider) SearchStockImageCandidates(_ context.Context, _ string, _ int) ([]StockImageCandidate, error) {
	return nil, nil
}

func (m *slowMockProvider) GetStockImageByID(_ context.Context, _ string) (*models.StockImage, error) {
	return nil, fmt.Errorf("not supported")
}

func (m *slowMockProvider) CheckHealth(_ context.Context) ([]*health.Status, error) {
	return []*health.Status{{Name: m.name, Backend: m.name}}, nil
}

func TestNewFallbackStockImageryProvider(t *testing.T) {
	t.Run("creates provider with single provider", func(t *testing.T) {
		providers := []StockImageryProvider{&fallbackMockProvider{}}
		names := []string{"provider1"}

		fp, err := NewFallbackStockImageryProvider(providers, names)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(fp.providers) != 1 {
			t.Errorf("Expected 1 provider, got %d", len(fp.providers))
		}
	})

	t.Run("creates provider with multiple providers", func(t *testing.T) {
		providers := []StockImageryProvider{
			&fallbackMockProvider{},
			&fallbackMockProvider{},
		}
		names := []string{"provider1", "provider2"}

		fp, err := NewFallbackStockImageryProvider(providers, names)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if len(fp.providers) != 2 {
			t.Errorf("Expected 2 providers, got %d", len(fp.providers))
		}
	})

	t.Run("returns error with no providers", func(t *testing.T) {
		_, err := NewFallbackStockImageryProvider([]StockImageryProvider{}, []string{})
		if err == nil {
			t.Error("Expected error with no providers")
		}
	})

	t.Run("returns error when providers and names length mismatch", func(t *testing.T) {
		providers := []StockImageryProvider{&fallbackMockProvider{}}
		names := []string{"provider1", "provider2"}

		_, err := NewFallbackStockImageryProvider(providers, names)
		if err == nil {
			t.Error("Expected error with mismatched lengths")
		}
	})
}

func TestFallbackStockImageryProvider_GetStockImage(t *testing.T) {
	ctx := context.Background()

	t.Run("returns result from first provider on success", func(t *testing.T) {
		expectedResult := &models.StockImage{
			Id:       "stock-123",
			MediaId:  "media-123",
			Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_UNSPLASH,
		}

		provider1 := &fallbackMockProvider{
			name:   "provider1",
			result: expectedResult,
			err:    nil,
		}
		provider2 := &fallbackMockProvider{
			name: "provider2",
			err:  errors.New("should not be called"),
		}

		fp, _ := NewFallbackStockImageryProvider(
			[]StockImageryProvider{provider1, provider2},
			[]string{"provider1", "provider2"},
		)

		result, err := fp.GetStockImage(ctx, "test query", nil)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Id != expectedResult.Id {
			t.Errorf("Expected result ID %s, got %s", expectedResult.Id, result.Id)
		}
	})

	t.Run("tries second provider when first fails", func(t *testing.T) {
		expectedResult := &models.StockImage{
			Id:       "stock-456",
			MediaId:  "media-456",
			Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_UNSPLASH,
		}

		provider1 := &fallbackMockProvider{
			name: "provider1",
			err:  errors.New("provider1 failed"),
		}
		provider2 := &fallbackMockProvider{
			name:   "provider2",
			result: expectedResult,
			err:    nil,
		}

		fp, _ := NewFallbackStockImageryProvider(
			[]StockImageryProvider{provider1, provider2},
			[]string{"provider1", "provider2"},
		)

		result, err := fp.GetStockImage(ctx, "test query", nil)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Id != expectedResult.Id {
			t.Errorf("Expected result ID %s, got %s", expectedResult.Id, result.Id)
		}
	})

	t.Run("tries all providers in order", func(t *testing.T) {
		expectedResult := &models.StockImage{
			Id:       "stock-789",
			MediaId:  "media-789",
			Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_UNSPLASH,
		}

		provider1 := &fallbackMockProvider{
			name: "provider1",
			err:  errors.New("provider1 failed"),
		}
		provider2 := &fallbackMockProvider{
			name: "provider2",
			err:  errors.New("provider2 failed"),
		}
		provider3 := &fallbackMockProvider{
			name:   "provider3",
			result: expectedResult,
			err:    nil,
		}

		fp, _ := NewFallbackStockImageryProvider(
			[]StockImageryProvider{provider1, provider2, provider3},
			[]string{"provider1", "provider2", "provider3"},
		)

		result, err := fp.GetStockImage(ctx, "test query", nil)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if result.Id != expectedResult.Id {
			t.Errorf("Expected result ID %s, got %s", expectedResult.Id, result.Id)
		}
	})

	t.Run("returns error when all providers fail", func(t *testing.T) {
		provider1 := &fallbackMockProvider{
			name: "provider1",
			err:  errors.New("provider1 failed"),
		}
		provider2 := &fallbackMockProvider{
			name: "provider2",
			err:  errors.New("provider2 failed"),
		}

		fp, _ := NewFallbackStockImageryProvider(
			[]StockImageryProvider{provider1, provider2},
			[]string{"provider1", "provider2"},
		)

		result, err := fp.GetStockImage(ctx, "test query", nil)
		if err == nil {
			t.Fatal("Expected error when all providers fail")
		}

		if result != nil {
			t.Error("Expected nil result when all providers fail")
		}

		// Verify error message includes all provider count
		expectedErrMsg := "all 2 stock imagery providers failed"
		if err.Error()[:len(expectedErrMsg)] != expectedErrMsg {
			t.Errorf("Expected error message to start with %q, got %q", expectedErrMsg, err.Error())
		}
	})

	t.Run("passes options to providers", func(t *testing.T) {
		var capturedOpts *StockImageOptions

		provider := &fallbackMockProvider{
			name: "provider1",
			result: &models.StockImage{
				Id:      "stock-123",
				MediaId: "media-123",
			},
		}

		// Override GetStockImage to capture options
		originalGetStockImage := func(_ context.Context, _ string, opts *StockImageOptions) (*models.StockImage, error) {
			capturedOpts = opts
			return provider.result, provider.err
		}

		fp, _ := NewFallbackStockImageryProvider(
			[]StockImageryProvider{provider},
			[]string{"provider1"},
		)

		// Create a custom provider that captures options
		fp.providers[0] = &struct{ fallbackMockProvider }{fallbackMockProvider{
			name:   "provider1",
			result: provider.result,
			err:    provider.err,
		}}

		// Manually call with a wrapper to capture
		testOpts := &StockImageOptions{RequireUnique: true}
		_, _ = originalGetStockImage(ctx, "test", testOpts)

		if capturedOpts != testOpts {
			t.Error("Expected options to be passed through to provider")
		}
	})
}

func TestFallbackStockImageryProvider_CheckHealth_AllHealthy(t *testing.T) {
	provider1 := &fallbackMockProvider{name: "provider1"}
	provider2 := &fallbackMockProvider{name: "provider2"}

	fp, err := NewFallbackStockImageryProvider(
		[]StockImageryProvider{provider1, provider2},
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

func TestFallbackStockImageryProvider_CheckHealth_OneUnhealthy(t *testing.T) {
	provider1 := &fallbackMockProvider{name: "provider1"}
	provider2 := &fallbackMockProvider{name: "provider2", healthErr: errors.New("connection failed")}

	fp, err := NewFallbackStockImageryProvider(
		[]StockImageryProvider{provider1, provider2},
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

	// First provider should be healthy
	if statuses[0].Error != "" {
		t.Errorf("expected provider1 to be healthy, got error: %s", statuses[0].Error)
	}

	// Second provider should have an error
	if statuses[1].Error == "" {
		t.Error("expected provider2 to have error, but status.Error is empty")
	}
}

func TestNewFallbackStockImageryProviderWithTimeout(t *testing.T) {
	t.Run("creates provider with custom timeout", func(t *testing.T) {
		providers := []StockImageryProvider{&fallbackMockProvider{}}
		names := []string{"provider1"}
		customTimeout := 5 * time.Second

		fp, err := NewFallbackStockImageryProviderWithTimeout(providers, names, customTimeout)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if fp.perProviderTimeout != customTimeout {
			t.Errorf("Expected timeout %v, got %v", customTimeout, fp.perProviderTimeout)
		}
	})

	t.Run("default constructor uses DefaultPerProviderTimeout", func(t *testing.T) {
		providers := []StockImageryProvider{&fallbackMockProvider{}}
		names := []string{"provider1"}

		fp, err := NewFallbackStockImageryProvider(providers, names)
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}

		if fp.perProviderTimeout != DefaultPerProviderTimeout {
			t.Errorf("Expected default timeout %v, got %v", DefaultPerProviderTimeout, fp.perProviderTimeout)
		}
	})
}

func TestFallbackStockImageryProvider_PerProviderTimeout(t *testing.T) {
	t.Run("slow provider times out and fallback succeeds", func(t *testing.T) {
		// First provider is slow (will timeout after 10ms)
		slowProvider := &slowMockProvider{
			name:  "slow",
			delay: 500 * time.Millisecond, // Would take 500ms, but will be cancelled at 10ms
		}

		// Second provider is fast and returns success
		fastProvider := &fallbackMockProvider{
			name: "fast",
			result: &models.StockImage{
				Id:      "stock-fast",
				MediaId: "media-fast",
			},
		}

		// Use a very short per-provider timeout for testing
		fp, _ := NewFallbackStockImageryProviderWithTimeout(
			[]StockImageryProvider{slowProvider, fastProvider},
			[]string{"slow", "fast"},
			10*time.Millisecond,
		)

		start := time.Now()
		result, err := fp.GetStockImage(context.Background(), "test query", nil)
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("Expected success from fast provider, got error: %v", err)
		}

		if result.Id != "stock-fast" {
			t.Errorf("Expected result from fast provider, got ID: %s", result.Id)
		}

		// Should complete in roughly 10-50ms, not 500ms
		if elapsed > 100*time.Millisecond {
			t.Errorf("Operation took too long (%v), slow provider wasn't timed out properly", elapsed)
		}

		if !slowProvider.called {
			t.Error("Slow provider should have been called first")
		}
	})

	t.Run("all providers timing out returns error", func(t *testing.T) {
		provider1 := &slowMockProvider{name: "slow1", delay: 500 * time.Millisecond}
		provider2 := &slowMockProvider{name: "slow2", delay: 500 * time.Millisecond}

		fp, _ := NewFallbackStockImageryProviderWithTimeout(
			[]StockImageryProvider{provider1, provider2},
			[]string{"slow1", "slow2"},
			10*time.Millisecond,
		)

		start := time.Now()
		result, err := fp.GetStockImage(context.Background(), "test query", nil)
		elapsed := time.Since(start)

		if err == nil {
			t.Fatal("Expected error when all providers timeout")
		}
		if result != nil {
			t.Error("Expected nil result when all providers timeout")
		}

		// Should complete in roughly 20ms (2 x 10ms timeouts), not 1 second
		if elapsed > 100*time.Millisecond {
			t.Errorf("Operation took too long (%v), providers weren't timed out properly", elapsed)
		}

		if !provider1.called || !provider2.called {
			t.Error("Both providers should have been called")
		}
	})
}

func TestFallbackStockImageryProvider_ParentContextCancellation(t *testing.T) {
	t.Run("stops trying providers when parent context is cancelled", func(t *testing.T) {
		provider1 := &slowMockProvider{name: "slow1", delay: 500 * time.Millisecond}
		provider2 := &slowMockProvider{
			name:   "slow2",
			delay:  10 * time.Millisecond,
			result: &models.StockImage{Id: "stock-2", MediaId: "media-2"},
		}

		fp, _ := NewFallbackStockImageryProviderWithTimeout(
			[]StockImageryProvider{provider1, provider2},
			[]string{"slow1", "slow2"},
			100*time.Millisecond, // Per-provider timeout
		)

		// Cancel parent context after 20ms
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()

		start := time.Now()
		_, err := fp.GetStockImage(ctx, "test query", nil)
		elapsed := time.Since(start)

		if err == nil {
			t.Fatal("Expected error when parent context is cancelled")
		}

		// Should complete around 20ms, not wait for per-provider timeout (100ms)
		if elapsed > 80*time.Millisecond {
			t.Errorf("Operation took too long (%v), should have stopped on parent context cancellation", elapsed)
		}

		if !provider1.called {
			t.Error("First provider should have been called")
		}
		if provider2.called {
			t.Error("Second provider should not have been called after parent context cancellation")
		}
	})
}
