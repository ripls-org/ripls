package webfetch

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// extractAmazonProductImage extracts the high-resolution product image from
// an Amazon product page. Amazon strips the standard og:image/twitter:image
// /JSON-LD signals from its SSR'd product HTML; the high-res image lives on
// the main <img id="landingImage"> via two Amazon-specific attributes:
//
//   - data-old-hires="https://m.media-amazon.com/images/I/<id>._AC_SL1500_.jpg"
//     A direct URL to the 1500-px master variant. Always present on the
//     landing image when JS is disabled (i.e. exactly the case we're in).
//
//   - data-a-dynamic-image="{<url>:[w,h],<url>:[w,h],…}"
//     A JSON-encoded dimension map used by the page's image switcher.
//     Used as a fallback when data-old-hires is absent — pick the URL with
//     the largest stated width.
//
// Returns "" when neither signal is found. The per-domain rewrite in
// resolveImageURL further strips any remaining ._XX_. token, but the
// URLs here are typically already the master.
func extractAmazonProductImage(doc *goquery.Document) string {
	if u := strings.TrimSpace(doc.Find(`img[data-old-hires]`).First().AttrOr("data-old-hires", "")); u != "" {
		return u
	}
	if raw := strings.TrimSpace(doc.Find(`img[data-a-dynamic-image]`).First().AttrOr("data-a-dynamic-image", "")); raw != "" {
		if u := largestAmazonDynamicImage(raw); u != "" {
			return u
		}
	}
	return ""
}

// largestAmazonDynamicImage parses Amazon's data-a-dynamic-image attribute
// (a JSON object mapping URL → [width, height]) and returns the URL with the
// largest stated width. Returns "" when the input can't be parsed or holds
// no usable entries.
func largestAmazonDynamicImage(raw string) string {
	urls := amazonDynamicImagesByWidth(raw)
	if len(urls) == 0 {
		return ""
	}
	return urls[0]
}

// amazonDynamicImagesByWidth parses Amazon's data-a-dynamic-image attribute
// and returns all URLs in descending width order. Returns nil when the input
// can't be parsed or holds no usable entries.
func amazonDynamicImagesByWidth(raw string) []string {
	// data-a-dynamic-image is HTML-attribute-encoded; the surrounding HTML
	// parser has already decoded &quot; to " for us by the time goquery
	// hands us the value, so a direct json.Unmarshal works.
	var m map[string][]int
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil
	}
	type entry struct {
		url string
		w   int
	}
	candidates := make([]entry, 0, len(m))
	for u, dims := range m {
		if len(dims) == 0 || dims[0] <= 0 {
			continue
		}
		candidates = append(candidates, entry{url: u, w: dims[0]})
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].w > candidates[j].w })
	out := make([]string, len(candidates))
	for i, c := range candidates {
		out[i] = c.url
	}
	return out
}

// extractAmazonProductImages returns the Amazon-specific image URLs from a
// product page, in priority order: data-old-hires first (the master 1500-px
// variant), then every URL in the data-a-dynamic-image dimension map sorted
// largest-width first. Duplicates are not removed by this function — the
// caller should dedupe across all extractor sources.
func extractAmazonProductImages(doc *goquery.Document) []string {
	var out []string
	if u := strings.TrimSpace(doc.Find(`img[data-old-hires]`).First().AttrOr("data-old-hires", "")); u != "" {
		out = append(out, u)
	}
	if raw := strings.TrimSpace(doc.Find(`img[data-a-dynamic-image]`).First().AttrOr("data-a-dynamic-image", "")); raw != "" {
		out = append(out, amazonDynamicImagesByWidth(raw)...)
	}
	return out
}
