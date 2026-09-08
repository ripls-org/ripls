package storage

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"net/http"
	"strings"
)

// Upload size caps, shared across every media path so the limit is consistent
// codebase-wide (see #1953). Image-bearing flows (direct upload, webpage-image,
// avatar fetch, stock-image fetch) bound on MaxImageUploadBytes; video bounds on
// MaxVideoUploadBytes.
const (
	// MaxImageUploadBytes is the largest accepted image payload (60 MB).
	MaxImageUploadBytes int64 = 60 * 1024 * 1024
	// MaxVideoUploadBytes is the largest accepted video payload (200 MB).
	MaxVideoUploadBytes int64 = 200 * 1024 * 1024

	// maxImagePixels caps an image's declared width × height. image.Decode
	// allocates a buffer sized to the header dimensions before reading pixels,
	// so a small file declaring enormous dimensions (a decompression bomb, e.g.
	// a 1 MB PNG claiming 100000×100000) can OOM the server. We read only the
	// header (image.DecodeConfig — no pixel allocation) and reject above this
	// cap. 100 MP comfortably clears mainstream phone photos (12–64 MP) and most
	// high-MP modes while rejecting bombs by orders of magnitude. Tunable.
	maxImagePixels int64 = 100 * 1000 * 1000
)

// Sentinel errors returned by ValidateMediaBytes. Callers at an RPC boundary
// map these to connect.CodeInvalidArgument; they are user input errors, not
// server failures.
var (
	// ErrUnsupportedMediaType is returned when the declared content type is not
	// on the allow-list (e.g. image/svg+xml, application/pdf).
	ErrUnsupportedMediaType = errors.New("unsupported media type")
	// ErrMediaTooLarge is returned when the payload exceeds the per-kind cap.
	ErrMediaTooLarge = errors.New("media exceeds size limit")
	// ErrMediaContentMismatch is returned when the sniffed bytes contradict the
	// declared content type (e.g. an image/jpeg declaration wrapping HTML/SVG).
	ErrMediaContentMismatch = errors.New("media content does not match declared type")
	// ErrImageDimensionsExceeded is returned when an image's declared pixel
	// dimensions exceed maxImagePixels (a decompression-bomb guard).
	ErrImageDimensionsExceeded = errors.New("image dimensions exceed limit")
)

type mediaKind int

const (
	mediaKindImage mediaKind = iota
	mediaKindVideo
)

// allowedMediaTypes is the upload allow-list. SVG is deliberately excluded —
// it can carry inline <script> that executes in any browser preview of the
// served object. gif/tiff and quicktime/webm are included so existing avatar
// and iOS-video uploads are not regressed.
var allowedMediaTypes = map[string]mediaKind{
	"image/jpeg":      mediaKindImage,
	"image/jpg":       mediaKindImage, // some clients send the non-canonical spelling
	"image/png":       mediaKindImage,
	"image/webp":      mediaKindImage,
	"image/gif":       mediaKindImage,
	"image/heic":      mediaKindImage,
	"image/heif":      mediaKindImage,
	"image/tiff":      mediaKindImage,
	"video/mp4":       mediaKindVideo,
	"video/quicktime": mediaKindVideo,
	"video/webm":      mediaKindVideo,
}

// normalizeContentType lowercases a content type and drops any parameters
// (e.g. "; charset=utf-8").
func normalizeContentType(ct string) string {
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	return strings.ToLower(strings.TrimSpace(ct))
}

// ValidateMediaBytes enforces the upload allow-list, per-kind size cap, and a
// content-type sniff that rejects payloads whose bytes contradict the declared
// type. It returns one of the sentinel errors above (wrapped) on rejection.
//
// The sniff intentionally accepts application/octet-stream: http.DetectContentType
// cannot name HEIC/TIFF/MP4/MOV/WEBM and reports them as generic binary. It
// rejects any text/* sniff for a media declaration, which is what HTML and SVG
// payloads resolve to — closing the script-injection vector without a magic-byte
// table for every container format.
func ValidateMediaBytes(data []byte, declaredContentType string) error {
	ct := normalizeContentType(declaredContentType)

	kind, ok := allowedMediaTypes[ct]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnsupportedMediaType, declaredContentType)
	}

	if len(data) == 0 {
		return fmt.Errorf("%w: empty payload declared as %q", ErrMediaContentMismatch, declaredContentType)
	}

	limit := MaxImageUploadBytes
	if kind == mediaKindVideo {
		limit = MaxVideoUploadBytes
	}
	if int64(len(data)) > limit {
		return fmt.Errorf("%w: %d bytes declared %q exceeds %d", ErrMediaTooLarge, len(data), declaredContentType, limit)
	}

	if !sniffMatchesDeclared(ct, data) {
		sniffed := normalizeContentType(http.DetectContentType(data))
		return fmt.Errorf("%w: declared %q but content sniffed as %q", ErrMediaContentMismatch, declaredContentType, sniffed)
	}

	// Decompression-bomb guard: for images, read only the header (no pixel
	// allocation) and reject absurd declared dimensions before any image.Decode
	// downstream. Formats whose header DecodeConfig can't parse — and partial
	// fixtures — are skipped; the byte cap and sniff still bound them.
	if kind == mediaKindImage {
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
			if int64(cfg.Width)*int64(cfg.Height) > maxImagePixels {
				return fmt.Errorf("%w: %dx%d exceeds %d pixels",
					ErrImageDimensionsExceeded, cfg.Width, cfg.Height, maxImagePixels)
			}
		}
	}

	return nil
}

// sniffMatchesDeclared reports whether the payload's sniffed content type is
// compatible with the (already allow-listed) declared type.
func sniffMatchesDeclared(declared string, data []byte) bool {
	sniffed := normalizeContentType(http.DetectContentType(data))

	switch {
	case sniffed == "application/octet-stream":
		// Generic binary — expected for HEIC/TIFF/MP4/MOV/WEBM, which the
		// sniffer cannot name. Accept; these are not script-executable.
		return true
	case strings.HasPrefix(sniffed, "text/"):
		// A media declaration whose bytes are text (HTML, SVG, plain) is a
		// spoof — reject. This is the SVG / HTML script-injection guard.
		return false
	default:
		// The sniffer named a concrete type. It must be allow-listed and of the
		// same media kind as the declaration (image vs video). A benign
		// image-format mismatch (e.g. declared heic, actually jpeg) is allowed;
		// a cross-kind or disallowed sniff (application/pdf, etc.) is not.
		k, ok := allowedMediaTypes[sniffed]
		return ok && k == allowedMediaTypes[declared]
	}
}
