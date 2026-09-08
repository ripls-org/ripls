package mediacand

import (
	"testing"

	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/media"
)

func TestImageURLsToCandidates(t *testing.T) {
	t.Run("empty input returns nil", func(t *testing.T) {
		if got := ImageURLsToCandidates(nil); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
	t.Run("skips empty entries", func(t *testing.T) {
		got := ImageURLsToCandidates([]string{"https://a", "", "https://b"})
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		if got[0].GetUrl() != "https://a" || got[1].GetUrl() != "https://b" {
			t.Errorf("got urls %v", []string{got[0].GetUrl(), got[1].GetUrl()})
		}
		for _, c := range got {
			if c.GetContentType() != "image/jpeg" {
				t.Errorf("content_type = %q", c.GetContentType())
			}
		}
	})
}

func TestStockImageCandidatesToAPI(t *testing.T) {
	t.Run("empty input returns nil", func(t *testing.T) {
		if got := StockImageCandidatesToAPI(nil); got != nil {
			t.Errorf("got %v, want nil", got)
		}
	})
	t.Run("maps Pexels candidate with attribution metadata", func(t *testing.T) {
		got := StockImageCandidatesToAPI([]media.StockImageCandidate{{
			URL:             "https://images.pexels.com/photos/123/medium.jpg",
			ContentType:     "image/jpeg",
			WidthPx:         800,
			HeightPx:        600,
			Provider:        models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_PEXELS,
			ProviderPhotoID: "123",
		}})
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1", len(got))
		}
		c := got[0]
		if c.GetUrl() != "https://images.pexels.com/photos/123/medium.jpg" {
			t.Errorf("url = %q", c.GetUrl())
		}
		if c.GetContentType() != "image/jpeg" {
			t.Errorf("content_type = %q", c.GetContentType())
		}
		if c.GetWidthPx() != 800 || c.GetHeightPx() != 600 {
			t.Errorf("dims = %dx%d", c.GetWidthPx(), c.GetHeightPx())
		}
		if c.GetProvider() != api.StockImageProvider_STOCK_IMAGE_PROVIDER_PEXELS {
			t.Errorf("provider = %v", c.GetProvider())
		}
		if c.GetProviderPhotoId() != "123" {
			t.Errorf("provider_photo_id = %q", c.GetProviderPhotoId())
		}
	})
	t.Run("Unsplash maps to Unsplash API enum", func(t *testing.T) {
		got := StockImageCandidatesToAPI([]media.StockImageCandidate{{
			URL:      "https://images.unsplash.com/x",
			Provider: models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_UNSPLASH,
		}})
		if got[0].GetProvider() != api.StockImageProvider_STOCK_IMAGE_PROVIDER_UNSPLASH {
			t.Errorf("provider = %v", got[0].GetProvider())
		}
	})
	t.Run("skips empty URL entries", func(t *testing.T) {
		got := StockImageCandidatesToAPI([]media.StockImageCandidate{
			{URL: "https://a"},
			{URL: ""},
			{URL: "https://b"},
		})
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
	})
}
