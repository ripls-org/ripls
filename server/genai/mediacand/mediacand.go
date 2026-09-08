// Package mediacand builds api.MediaCandidate slices for the streaming
// Gen* RPCs. Candidates ride on MediaReady events as alternates the
// client may surface as one-tap replacements for the chosen image.
package mediacand

import (
	api "go.ripls.org/ripls/server/gen/ripls/api"
	"go.ripls.org/ripls/server/gen/ripls/models"
	"go.ripls.org/ripls/server/media"
)

// ImageURLsToCandidates converts a slice of image URLs into MediaCandidate
// entries with content_type "image/jpeg" — the safe default; the
// AddMediaFromURL import flow validates the real content type at fetch
// time. Empty input returns nil so the caller can pass straight through
// to MediaReady.
func ImageURLsToCandidates(urls []string) []*api.MediaCandidate {
	if len(urls) == 0 {
		return nil
	}
	out := make([]*api.MediaCandidate, 0, len(urls))
	for _, u := range urls {
		if u == "" {
			continue
		}
		out = append(out, &api.MediaCandidate{
			Url:         u,
			ContentType: "image/jpeg",
		})
	}
	return out
}

// StockImageCandidatesToAPI converts media.StockImageCandidate entries
// (returned by stock-imagery providers) into the wire-format
// api.MediaCandidate slice. Preserves dimensions and provider metadata
// so the import-time attribution path on the server has what it needs.
func StockImageCandidatesToAPI(cs []media.StockImageCandidate) []*api.MediaCandidate {
	if len(cs) == 0 {
		return nil
	}
	out := make([]*api.MediaCandidate, 0, len(cs))
	for _, c := range cs {
		if c.URL == "" {
			continue
		}
		mc := &api.MediaCandidate{
			Url:         c.URL,
			ContentType: c.ContentType,
		}
		if c.WidthPx > 0 {
			mc.WidthPx = &c.WidthPx
		}
		if c.HeightPx > 0 {
			mc.HeightPx = &c.HeightPx
		}
		if provider := stockProviderToAPI(c.Provider); provider != api.StockImageProvider_STOCK_IMAGE_PROVIDER_UNSPECIFIED {
			mc.Provider = &provider
		}
		if c.ProviderPhotoID != "" {
			id := c.ProviderPhotoID
			mc.ProviderPhotoId = &id
		}
		if c.ThumbnailURL != "" {
			t := c.ThumbnailURL
			mc.ThumbnailUrl = &t
		}
		out = append(out, mc)
	}
	return out
}

// stockProviderToAPI maps the models-level provider enum into the api
// (wire-facing) enum. The two enums are intentionally separate so the
// storage schema and the API surface can evolve independently.
func stockProviderToAPI(p models.StockImageryProvider) api.StockImageProvider {
	switch p {
	case models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_UNSPLASH:
		return api.StockImageProvider_STOCK_IMAGE_PROVIDER_UNSPLASH
	case models.StockImageryProvider_STOCK_IMAGERY_PROVIDER_PEXELS:
		return api.StockImageProvider_STOCK_IMAGE_PROVIDER_PEXELS
	default:
		return api.StockImageProvider_STOCK_IMAGE_PROVIDER_UNSPECIFIED
	}
}
