package webfetch

import (
	"strconv"
	"strings"
)

// maxSrcsetCandidateWidth caps the width-descriptor we'll honor when picking
// a srcset candidate. Pages can in principle declare arbitrarily large w
// values; without a guard a malicious or buggy page could point us at a
// download bomb, since the picker biases toward the largest stated width.
// 4096px is comfortably larger than any product image we'd actually want
// (typical e-commerce masters are 1500–2500px) but well below the realm
// where someone is deliberately attacking the fetcher.
//
// downloadAndStoreWebpageImage applies its own body-size cap, so this is
// the first of two defenses, not the only one.
const maxSrcsetCandidateWidth = 4096

// pickBestSrcsetURL parses an HTML img srcset value and returns the URL of
// the highest-resolution candidate, respecting maxSrcsetCandidateWidth.
//
// srcset syntax: comma-separated entries of "<url>[ <descriptor>]" where
// descriptor is either "<W>w" (pixel width hint) or "<density>x" (pixel
// density). We prefer w-described candidates (more reliable on real
// e-commerce pages) and fall back to the highest x-described one when no
// usable w-descriptor exists.
//
// Returns the empty string when srcset is empty, malformed, or every
// candidate exceeds the width cap. The caller should fall back to src.
func pickBestSrcsetURL(srcset string) string {
	type cand struct {
		url string
		w   int
		x   float64
	}
	var bestW cand
	var bestX cand
	for _, entry := range strings.Split(srcset, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		fields := strings.Fields(entry)
		if len(fields) == 0 {
			continue
		}
		u := fields[0]
		if u == "" {
			continue
		}
		var w int
		var x float64
		if len(fields) >= 2 {
			d := fields[1]
			switch {
			case strings.HasSuffix(d, "w"):
				if n, err := strconv.Atoi(strings.TrimSuffix(d, "w")); err == nil && n > 0 {
					w = n
				}
			case strings.HasSuffix(d, "x"):
				if f, err := strconv.ParseFloat(strings.TrimSuffix(d, "x"), 64); err == nil && f > 0 {
					x = f
				}
			}
		}
		if w > 0 && w <= maxSrcsetCandidateWidth && w > bestW.w {
			bestW = cand{url: u, w: w}
		}
		if x > bestX.x {
			bestX = cand{url: u, x: x}
		}
	}
	if bestW.url != "" {
		return bestW.url
	}
	return bestX.url
}
