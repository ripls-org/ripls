package webfetch

import (
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func parseTestURL(s string) (*url.URL, error) {
	return url.Parse(s)
}

func TestExtractAmazonProductImage(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{
			name: "data-old-hires wins (primary signal)",
			html: `<html><body>
				<img id="landingImage" data-old-hires="https://m.media-amazon.com/images/I/71bUaHLqV5L._AC_SL1500_.jpg">
			</body></html>`,
			want: "https://m.media-amazon.com/images/I/71bUaHLqV5L._AC_SL1500_.jpg",
		},
		{
			name: "data-a-dynamic-image fallback picks largest width",
			html: `<html><body>
				<img id="landingImage" data-a-dynamic-image='{"https://m.media-amazon.com/images/I/abc._AC_SX342_.jpg":[342,342],"https://m.media-amazon.com/images/I/abc._AC_SX679_.jpg":[679,679],"https://m.media-amazon.com/images/I/abc._AC_SX466_.jpg":[466,466]}'>
			</body></html>`,
			want: "https://m.media-amazon.com/images/I/abc._AC_SX679_.jpg",
		},
		{
			name: "data-old-hires preferred over data-a-dynamic-image",
			html: `<html><body>
				<img id="landingImage"
				     data-old-hires="https://m.media-amazon.com/images/I/abc._AC_SL1500_.jpg"
				     data-a-dynamic-image='{"https://m.media-amazon.com/images/I/abc._AC_SX342_.jpg":[342,342]}'>
			</body></html>`,
			want: "https://m.media-amazon.com/images/I/abc._AC_SL1500_.jpg",
		},
		{
			name: "no Amazon attributes present",
			html: `<html><body><img src="https://example.com/normal.jpg"></body></html>`,
			want: "",
		},
		{
			name: "empty data-old-hires falls through to data-a-dynamic-image",
			html: `<html><body>
				<img id="landingImage" data-old-hires=""
				     data-a-dynamic-image='{"https://m.media-amazon.com/images/I/abc._AC_SX679_.jpg":[679,679]}'>
			</body></html>`,
			want: "https://m.media-amazon.com/images/I/abc._AC_SX679_.jpg",
		},
		{
			name: "malformed data-a-dynamic-image — no panic, returns empty",
			html: `<html><body><img data-a-dynamic-image='{ this is not json }'></body></html>`,
			want: "",
		},
		{
			name: "data-a-dynamic-image with zero width entries ignored",
			html: `<html><body>
				<img data-a-dynamic-image='{"https://m.media-amazon.com/images/I/bad.jpg":[0,0],"https://m.media-amazon.com/images/I/good.jpg":[800,800]}'>
			</body></html>`,
			want: "https://m.media-amazon.com/images/I/good.jpg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(tt.html))
			if err != nil {
				t.Fatalf("parse html: %v", err)
			}
			got := extractAmazonProductImage(doc)
			if got != tt.want {
				t.Errorf("extractAmazonProductImage()\n  got:  %q\n  want: %q", got, tt.want)
			}
		})
	}
}

func TestExtractImageURL_AmazonBeatsContentImg(t *testing.T) {
	// Amazon page shape: no og:image, no JSON-LD, but data-old-hires on
	// the landing image. The Amazon-specific extractor should pick it up
	// instead of falling through to whatever <img> happens to come first.
	// Also exercises Phase 1: the _AC_SL1500_ token gets stripped to the
	// master.
	html := `<html><body>
		<img height="1" width="1" src="https://amazon.com/tracking-pixel.gif">
		<div role="main">
			<img id="landingImage"
			     src="https://m.media-amazon.com/images/I/71bUaHLqV5L._AC_SY300_.jpg"
			     data-old-hires="https://m.media-amazon.com/images/I/71bUaHLqV5L._AC_SL1500_.jpg">
		</div>
	</body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse html: %v", err)
	}
	f := &HTTPFetcher{}
	baseURL, err := parseTestURL("https://www.amazon.com/dp/B0DN1NQ5ZB")
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}
	got := f.extractImageURL(doc, baseURL)
	// Expected: Amazon path picks data-old-hires (._AC_SL1500_.jpg), then
	// Phase 1 strips the size token → master.
	want := "https://m.media-amazon.com/images/I/71bUaHLqV5L.jpg"
	if got != want {
		t.Errorf("expected Amazon data-old-hires to win + Phase 1 to strip token.\n  got:  %q\n  want: %q", got, want)
	}
}
