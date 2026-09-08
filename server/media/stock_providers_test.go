package media

import (
	"testing"

	"go.ripls.org/ripls/server/logging"
)

func TestNewStockProviders(t *testing.T) {
	logger := logging.NewLogger(logging.Options{Level: "error", Format: "json"})

	tests := []struct {
		name        string
		keys        StockProviderKeys
		wantImagery bool
		wantVideo   bool
	}{
		{
			name:        "no keys disables stock media",
			keys:        StockProviderKeys{},
			wantImagery: false,
			wantVideo:   false,
		},
		{
			name:        "unsplash only enables images without video",
			keys:        StockProviderKeys{UnsplashAccessKey: "u-key"},
			wantImagery: true,
			wantVideo:   false,
		},
		{
			name:        "pexels only enables images and video",
			keys:        StockProviderKeys{PexelsAPIKey: "p-key"},
			wantImagery: true,
			wantVideo:   true,
		},
		{
			name:        "pixabay alone enables nothing",
			keys:        StockProviderKeys{PixabayAPIKey: "x-key"},
			wantImagery: false,
			wantVideo:   false,
		},
		{
			name:        "all keys enable full chains",
			keys:        StockProviderKeys{UnsplashAccessKey: "u-key", PexelsAPIKey: "p-key", PixabayAPIKey: "x-key"},
			wantImagery: true,
			wantVideo:   true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			imagery, video, err := NewStockProviders(tc.keys, nil, nil, logger)
			if err != nil {
				t.Fatalf("NewStockProviders: %v", err)
			}
			if got := imagery != nil; got != tc.wantImagery {
				t.Errorf("imagery provider present = %v; want %v", got, tc.wantImagery)
			}
			if got := video != nil; got != tc.wantVideo {
				t.Errorf("video provider present = %v; want %v", got, tc.wantVideo)
			}
		})
	}
}
