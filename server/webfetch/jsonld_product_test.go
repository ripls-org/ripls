package webfetch

import (
	"net/url"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestExtractJSONLDProductImage(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{
			name: "string image",
			html: `<html><head><script type="application/ld+json">{
				"@context": "https://schema.org",
				"@type": "Product",
				"name": "Widget",
				"image": "https://example.com/widget.jpg"
			}</script></head><body></body></html>`,
			want: "https://example.com/widget.jpg",
		},
		{
			name: "array of strings — picks last (ascending-size convention)",
			html: `<html><head><script type="application/ld+json">{
				"@type": "Product",
				"image": [
					"https://example.com/small.jpg",
					"https://example.com/medium.jpg",
					"https://example.com/large.jpg"
				]
			}</script></head><body></body></html>`,
			want: "https://example.com/large.jpg",
		},
		{
			name: "ImageObject with url",
			html: `<html><head><script type="application/ld+json">{
				"@type": "Product",
				"image": {
					"@type": "ImageObject",
					"url": "https://example.com/widget.jpg",
					"width": 2000,
					"height": 2000
				}
			}</script></head><body></body></html>`,
			want: "https://example.com/widget.jpg",
		},
		{
			name: "ImageObject with contentUrl fallback",
			html: `<html><head><script type="application/ld+json">{
				"@type": "Product",
				"image": {
					"@type": "ImageObject",
					"contentUrl": "https://example.com/widget.jpg"
				}
			}</script></head><body></body></html>`,
			want: "https://example.com/widget.jpg",
		},
		{
			name: "@graph wrapper",
			html: `<html><head><script type="application/ld+json">{
				"@context": "https://schema.org",
				"@graph": [
					{ "@type": "Organization", "name": "Acme" },
					{ "@type": "Product", "image": "https://example.com/widget.jpg" }
				]
			}</script></head><body></body></html>`,
			want: "https://example.com/widget.jpg",
		},
		{
			name: "Product nested in CollectionPage",
			html: `<html><head><script type="application/ld+json">{
				"@type": "CollectionPage",
				"mainEntity": {
					"@type": "Product",
					"image": "https://example.com/widget.jpg"
				}
			}</script></head><body></body></html>`,
			want: "https://example.com/widget.jpg",
		},
		{
			name: "multi-typed Product node",
			html: `<html><head><script type="application/ld+json">{
				"@type": ["Product", "IndividualProduct"],
				"image": "https://example.com/widget.jpg"
			}</script></head><body></body></html>`,
			want: "https://example.com/widget.jpg",
		},
		{
			name: "ProductGroup type accepted",
			html: `<html><head><script type="application/ld+json">{
				"@type": "ProductGroup",
				"image": "https://example.com/widget.jpg"
			}</script></head><body></body></html>`,
			want: "https://example.com/widget.jpg",
		},
		{
			name: "ProductGroup beats nested Product variant (top-level wins)",
			html: `<html><head><script type="application/ld+json">{
				"@type": "ProductGroup",
				"image": "https://example.com/group.jpg",
				"hasVariant": [
					{ "@type": "Product", "image": "https://example.com/variant.jpg" }
				]
			}</script></head><body></body></html>`,
			want: "https://example.com/group.jpg",
		},
		{
			name: "IndividualProduct accepted",
			html: `<html><head><script type="application/ld+json">{
				"@type": "IndividualProduct",
				"image": "https://example.com/widget.jpg"
			}</script></head><body></body></html>`,
			want: "https://example.com/widget.jpg",
		},
		{
			name: "image array picks LAST entry (ascending-size convention)",
			html: `<html><head><script type="application/ld+json">{
				"@type": "Product",
				"image": [
					"https://example.com/widget.jpg?width=100",
					"https://example.com/widget.jpg?width=300",
					"https://example.com/widget.jpg?width=900"
				]
			}</script></head><body></body></html>`,
			want: "https://example.com/widget.jpg?width=900",
		},
		{
			name: "ignores Event JSON-LD (no Product)",
			html: `<html><head><script type="application/ld+json">{
				"@type": "Event",
				"name": "Concert",
				"image": "https://example.com/concert.jpg"
			}</script></head><body></body></html>`,
			want: "",
		},
		{
			name: "ignores malformed JSON",
			html: `<html><head>
				<script type="application/ld+json">{ this is not json }</script>
				<script type="application/ld+json">{ "@type": "Product", "image": "https://example.com/widget.jpg" }</script>
			</head><body></body></html>`,
			want: "https://example.com/widget.jpg",
		},
		{
			name: "no JSON-LD blocks",
			html: `<html><head></head><body><p>nothing</p></body></html>`,
			want: "",
		},
		{
			name: "Product with empty image",
			html: `<html><head><script type="application/ld+json">{
				"@type": "Product",
				"image": ""
			}</script></head><body></body></html>`,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(tt.html))
			if err != nil {
				t.Fatalf("parse html: %v", err)
			}
			got := extractJSONLDProductImage(doc)
			if got != tt.want {
				t.Errorf("extractJSONLDProductImage()\n  got:  %q\n  want: %q", got, tt.want)
			}
		})
	}
}

func TestExtractImageURL_JSONLDBeatsOGImage(t *testing.T) {
	// E-commerce reality check: when both signals are present, JSON-LD's
	// high-res master should win over og:image's link-preview thumbnail.
	html := `<html><head>
		<meta property="og:image" content="https://example.com/widget._SY300_.jpg">
		<script type="application/ld+json">{
			"@type": "Product",
			"image": "https://example.com/widget.jpg"
		}</script>
	</head><body></body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatalf("parse html: %v", err)
	}
	f := &HTTPFetcher{}
	baseURL, err := url.Parse("https://example.com/products/widget")
	if err != nil {
		t.Fatalf("parse base url: %v", err)
	}
	got := f.extractImageURL(doc, baseURL)
	want := "https://example.com/widget.jpg"
	if got != want {
		t.Errorf("expected JSON-LD to win over og:image.\n  got:  %q\n  want: %q", got, want)
	}
}
