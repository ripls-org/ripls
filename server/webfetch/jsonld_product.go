package webfetch

import (
	"encoding/json"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// extractJSONLDProductImage scans the document for
// <script type="application/ld+json"> blocks, walks each block looking for an
// object with @type "Product" (including objects nested under @graph or
// other container shapes), and returns the first image URL found on a
// Product object. Empty if no Product is present or no image is set.
//
// Schema.org Product.image is polymorphic: it can be a URL string, an
// ImageObject {url: "..."} / {contentUrl: "..."}, or an array of either.
// We accept all three.
//
// server/cmd/eval-seed-from-urls/main.go has a separate walker that fills
// many Product fields (name, brand, price, …) for goldens harvesting; the
// extractor here is intentionally narrower — webfetch's caller only needs
// the image. The two walkers can be unified in a follow-up once eval-seed
// is refactored to depend on this package.
func extractJSONLDProductImage(doc *goquery.Document) string {
	var found string
	doc.Find(`script[type="application/ld+json"]`).EachWithBreak(func(_ int, sel *goquery.Selection) bool {
		raw := strings.TrimSpace(sel.Text())
		if raw == "" {
			return true
		}
		var generic any
		if err := json.Unmarshal([]byte(raw), &generic); err != nil {
			return true // skip malformed blocks
		}
		if u := walkForProductImage(generic); u != "" {
			found = u
			return false
		}
		return true
	})
	return found
}

// walkForProductImage recursively descends decoded JSON-LD looking for a
// Product-like object and returns its image URL when found. Returns the
// empty string when no Product-like node is reachable from v.
func walkForProductImage(v any) string {
	switch n := v.(type) {
	case map[string]any:
		if isJSONLDProductLike(n["@type"]) {
			if u := jsonLDImageOf(n["image"]); u != "" {
				return u
			}
		}
		for _, child := range n {
			if u := walkForProductImage(child); u != "" {
				return u
			}
		}
	case []any:
		for _, child := range n {
			if u := walkForProductImage(child); u != "" {
				return u
			}
		}
	}
	return ""
}

// extractJSONLDProductImages collects every Product.image URL from every
// JSON-LD block on the page, preserving the canonical schema.org array
// ordering (smallest → largest, see jsonLDImagesOf). When the same Product
// node carries an image array, all entries are returned. Duplicates are
// not removed — the caller should dedupe across all extractor sources.
func extractJSONLDProductImages(doc *goquery.Document) []string {
	var out []string
	doc.Find(`script[type="application/ld+json"]`).Each(func(_ int, sel *goquery.Selection) {
		raw := strings.TrimSpace(sel.Text())
		if raw == "" {
			return
		}
		var generic any
		if err := json.Unmarshal([]byte(raw), &generic); err != nil {
			return
		}
		out = append(out, walkForAllProductImages(generic)...)
	})
	return out
}

// walkForAllProductImages recursively descends decoded JSON-LD collecting
// every image URL from every Product-like node. Mirrors walkForProductImage
// but accumulates instead of short-circuiting.
func walkForAllProductImages(v any) []string {
	var out []string
	switch n := v.(type) {
	case map[string]any:
		if isJSONLDProductLike(n["@type"]) {
			out = append(out, jsonLDImagesOf(n["image"])...)
		}
		for _, child := range n {
			out = append(out, walkForAllProductImages(child)...)
		}
	case []any:
		for _, child := range n {
			out = append(out, walkForAllProductImages(child)...)
		}
	}
	return out
}

// jsonLDImagesOf coerces a polymorphic image field into a slice of URL
// strings, preserving schema.org's largest-last array convention so the
// caller can reverse if it wants largest-first ranking.
func jsonLDImagesOf(v any) []string {
	switch n := v.(type) {
	case string:
		if s := strings.TrimSpace(n); s != "" {
			return []string{s}
		}
	case map[string]any:
		if u := jsonLDImageObjectURL(n); u != "" {
			return []string{u}
		}
	case []any:
		// Walk in reverse so largest (per schema.org convention) comes first.
		out := make([]string, 0, len(n))
		for i := len(n) - 1; i >= 0; i-- {
			out = append(out, jsonLDImagesOf(n[i])...)
		}
		return out
	}
	return nil
}

// jsonLDProductTypes is the set of schema.org Product subtypes whose image
// field carries the canonical product photo. ProductGroup is widely used
// by retailers that ship multiple variants under one listing (Allbirds,
// Patagonia, etc.); IndividualProduct/ProductModel are alternate forms
// used by some catalogs; Vehicle inherits from Product per schema.org.
var jsonLDProductTypes = map[string]bool{
	"Product":           true,
	"ProductGroup":      true,
	"IndividualProduct": true,
	"ProductModel":      true,
	"Vehicle":           true,
}

// isJSONLDProductLike reports whether the given @type value identifies a
// Product or Product subtype. @type can be a single string or an array of
// strings (a node can declare multiple types).
func isJSONLDProductLike(t any) bool {
	switch v := t.(type) {
	case string:
		return jsonLDProductTypes[v]
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && jsonLDProductTypes[s] {
				return true
			}
		}
	}
	return false
}

// jsonLDImageOf coerces a polymorphic image field into a URL string. Accepts
// a bare URL string, an ImageObject (preferring .url over .contentUrl), or
// an array of either.
//
// Arrays: schema.org's documented convention is to list images in ascending
// size order (the largest comes last). Most modern e-commerce platforms
// (Shopify, BigCommerce) follow this convention; some legacy CMSes list
// largest first. We pick the last non-empty entry — the more common case —
// and rely on the per-CDN rewrite in resolveImageURL to upgrade either
// way when the pattern is known. If the last entry is empty, we walk
// backwards until we find a non-empty one.
func jsonLDImageOf(v any) string {
	switch n := v.(type) {
	case string:
		return strings.TrimSpace(n)
	case map[string]any:
		if u := jsonLDImageObjectURL(n); u != "" {
			return u
		}
	case []any:
		for i := len(n) - 1; i >= 0; i-- {
			if u := jsonLDImageOf(n[i]); u != "" {
				return u
			}
		}
	}
	return ""
}

// jsonLDImageObjectURL extracts the URL from a schema.org ImageObject. The
// canonical field is `url`; `contentUrl` is a documented alternate.
func jsonLDImageObjectURL(obj map[string]any) string {
	if s, ok := obj["url"].(string); ok {
		if v := strings.TrimSpace(s); v != "" {
			return v
		}
	}
	if s, ok := obj["contentUrl"].(string); ok {
		if v := strings.TrimSpace(s); v != "" {
			return v
		}
	}
	return ""
}
