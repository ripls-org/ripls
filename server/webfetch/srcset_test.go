package webfetch

import (
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestPickBestSrcsetURL(t *testing.T) {
	tests := []struct {
		name   string
		srcset string
		want   string
	}{
		{
			name:   "empty",
			srcset: "",
			want:   "",
		},
		{
			name:   "no descriptors — picks first as-is is wrong, returns empty",
			srcset: "https://example.com/a.jpg, https://example.com/b.jpg",
			want:   "",
		},
		{
			name:   "single w descriptor",
			srcset: "https://example.com/widget.jpg 600w",
			want:   "https://example.com/widget.jpg",
		},
		{
			name:   "ladder of w descriptors — picks largest",
			srcset: "https://example.com/300.jpg 300w, https://example.com/600.jpg 600w, https://example.com/1200.jpg 1200w",
			want:   "https://example.com/1200.jpg",
		},
		{
			name:   "ladder out of order — still picks largest",
			srcset: "https://example.com/1200.jpg 1200w, https://example.com/300.jpg 300w, https://example.com/600.jpg 600w",
			want:   "https://example.com/1200.jpg",
		},
		{
			name:   "single x descriptor",
			srcset: "https://example.com/widget.jpg 2x",
			want:   "https://example.com/widget.jpg",
		},
		{
			name:   "ladder of x descriptors — picks highest density",
			srcset: "https://example.com/1x.jpg 1x, https://example.com/2x.jpg 2x, https://example.com/3x.jpg 3x",
			want:   "https://example.com/3x.jpg",
		},
		{
			name:   "mixed w + x — prefers w",
			srcset: "https://example.com/big.jpg 1200w, https://example.com/2x.jpg 2x",
			want:   "https://example.com/big.jpg",
		},
		{
			name:   "absurd w over cap is rejected, falls back",
			srcset: "https://example.com/normal.jpg 1200w, https://example.com/bomb.jpg 50000w",
			want:   "https://example.com/normal.jpg",
		},
		{
			name:   "all candidates over cap — returns empty",
			srcset: "https://example.com/huge1.jpg 8000w, https://example.com/huge2.jpg 12000w",
			want:   "",
		},
		{
			name:   "right at the cap — accepted",
			srcset: "https://example.com/at-cap.jpg 4096w, https://example.com/below-cap.jpg 2048w",
			want:   "https://example.com/at-cap.jpg",
		},
		{
			name:   "extra whitespace tolerated",
			srcset: "   https://example.com/a.jpg 300w  ,   https://example.com/b.jpg 600w  ",
			want:   "https://example.com/b.jpg",
		},
		{
			name:   "malformed descriptor ignored, next candidate wins",
			srcset: "https://example.com/bad.jpg ?, https://example.com/good.jpg 800w",
			want:   "https://example.com/good.jpg",
		},
		{
			name:   "zero or negative w ignored",
			srcset: "https://example.com/zero.jpg 0w, https://example.com/good.jpg 600w",
			want:   "https://example.com/good.jpg",
		},
		{
			name:   "fractional x descriptor honored",
			srcset: "https://example.com/1x.jpg 1x, https://example.com/1_5x.jpg 1.5x",
			want:   "https://example.com/1_5x.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pickBestSrcsetURL(tt.srcset)
			if got != tt.want {
				t.Errorf("pickBestSrcsetURL(%q)\n  got:  %q\n  want: %q", tt.srcset, got, tt.want)
			}
		})
	}
}

func TestExtractImageURL_SrcsetPreferredOverSrc(t *testing.T) {
	// Fallback-branch behavior: when the chosen <img> has both src (small)
	// and a srcset ladder, the largest in-cap srcset entry wins. og:image is
	// absent so the extractor falls through to the content <img> branch.
	html := `<html><head></head><body><main>
		<img src="https://example.com/small.jpg"
		     srcset="https://example.com/small.jpg 300w, https://example.com/medium.jpg 800w, https://example.com/large.jpg 1600w">
	</main></body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse html: %v", err)
	}
	f := &HTTPFetcher{}
	baseURL, err := url.Parse("https://example.com/")
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}
	got := f.extractImageURL(doc, baseURL)
	want := "https://example.com/large.jpg"
	if got != want {
		t.Errorf("expected srcset ladder to win over src.\n  got:  %q\n  want: %q", got, want)
	}
}

func TestExtractImageURL_SrcsetCapForcesFallbackToSrc(t *testing.T) {
	// When every srcset candidate exceeds the width cap, fall back to src
	// rather than picking an absurd value.
	html := `<html><head></head><body><main>
		<img src="https://example.com/normal.jpg"
		     srcset="https://example.com/huge1.jpg 8000w, https://example.com/huge2.jpg 12000w">
	</main></body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse html: %v", err)
	}
	f := &HTTPFetcher{}
	baseURL, err := url.Parse("https://example.com/")
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}
	got := f.extractImageURL(doc, baseURL)
	want := "https://example.com/normal.jpg"
	if got != want {
		t.Errorf("expected fall-back to src when srcset over cap.\n  got:  %q\n  want: %q", got, want)
	}
}
