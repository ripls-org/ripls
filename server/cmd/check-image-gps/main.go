// Command check-image-gps fails if a tracked image carries EXIF GPS
// coordinates, except for the fixtures that are supposed to.
//
// # Why this exists
//
// The open-source export (#2953) is gated by two text scanners:
// scripts/oss_export.js decides what ships and scripts/oss_scan.js greps the
// result for org identifiers. Neither can see inside a JPEG — oss_scan.js
// skips binaries by extension, on purpose, because scanning them produces
// nothing but noise.
//
// That leaves a gap with real consequence. A photo taken on a phone carries
// the coordinates it was taken at, and this repo's image fixtures were
// extracted from production. When this checker was written, eight tracked
// images carried GPS: two avatars pinned to where real people live, and
// server/test_data/books_exif.jpg, which shipped in the export and pointed at
// a real address. Publishing those would have leaked something no amount of
// grepping would have caught.
//
// Uploaded media is already safe — storage.SanitizeImageForStorage re-encodes
// and drops EXIF entirely. Checked-in fixtures never go through that path,
// which is exactly why they need their own gate.
//
// # Coverage
//
// JPEG and TIFF, via github.com/rwcarlsen/goexif/exif — the same library the
// server itself uses to read orientation and location, so this agrees with
// what the product would see. That is where EXIF GPS overwhelmingly lives and
// where all eight original findings were.
//
// It does NOT read HEIC/HEIF, which store metadata in ISO-BMFF boxes that
// goexif cannot parse. The tracked .heic fixtures are clean today (verified
// with exiftool), but a future one carrying GPS would pass this check. Closing
// that needs a HEIC metadata reader; until then, run
//
//	exiftool -gps:all -q $(git ls-files '*.heic' '*.heif')
//
// by hand when adding one.
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rwcarlsen/goexif/exif"
)

// allowed lists fixtures that carry GPS deliberately, with the reason.
//
// Keep this as short as it can be. Every entry is an image whose coordinates
// must be synthetic — a public landmark or an obviously invented point, never
// a place connected to a person.
var allowed = map[string]string{
	"server/test_data/books_exif.jpg": "proves SanitizeImageForStorage strips GPS; " +
		"without GPS the test would pass by having nothing to strip. " +
		"Coordinates are the Royal Observatory, Greenwich.",
}

// scanned extensions. goexif reads the EXIF segment of JPEG and TIFF.
var scanned = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".tif":  true,
	".tiff": true,
}

func trackedImages(ctx context.Context) ([]string, error) {
	out, err := exec.CommandContext(ctx, "git", "ls-files", "-z").Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-files: %w", err)
	}
	var paths []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p == "" {
			continue
		}
		if scanned[strings.ToLower(filepath.Ext(p))] {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return paths, nil
}

// gpsOf returns the coordinates in path, or ok=false when it has none.
// An unreadable or EXIF-less file is not a failure: most images legitimately
// carry no EXIF at all.
func gpsOf(path string) (lat, lon float64, ok bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, false
	}
	defer f.Close()

	x, err := exif.Decode(f)
	if err != nil {
		return 0, 0, false
	}
	lat, lon, err = x.LatLong()
	if err != nil {
		return 0, 0, false
	}
	return lat, lon, true
}

func main() {
	paths, err := trackedImages(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "check-image-gps: %v\n", err)
		os.Exit(2)
	}

	var violations []string
	usedAllowlist := map[string]bool{}

	for _, p := range paths {
		lat, lon, ok := gpsOf(p)
		if !ok {
			continue
		}
		if _, permitted := allowed[p]; permitted {
			usedAllowlist[p] = true
			continue
		}
		violations = append(violations, fmt.Sprintf("  %s  (%.6f, %.6f)", p, lat, lon))
	}

	// A stale allowlist entry is a silent hole: the file it names may have been
	// deleted or renamed, and the next fixture added at a similar path would
	// inherit the exemption. Report it, but don't fail the build over it.
	for p := range allowed {
		if !usedAllowlist[p] {
			fmt.Fprintf(os.Stderr,
				"check-image-gps: note: allowlisted %s no longer carries GPS; drop the entry\n", p)
		}
	}

	if len(violations) > 0 {
		fmt.Fprintf(os.Stderr,
			"check-image-gps: %d tracked image(s) carry EXIF GPS coordinates:\n%s\n\n"+
				"Coordinates in a checked-in image are a location someone was at. Strip them:\n\n"+
				"    exiftool -overwrite_original -GPS:all= <file>\n\n"+
				"If a fixture needs GPS to be meaningful, give it synthetic coordinates\n"+
				"(a public landmark) and add it to `allowed` in this file with the reason.\n",
			len(violations), strings.Join(violations, "\n"))
		os.Exit(1)
	}

	fmt.Printf("check-image-gps: %d image(s) scanned, no unexpected GPS coordinates\n", len(paths))
}
