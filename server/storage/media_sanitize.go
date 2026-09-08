package storage

import (
	"bytes"
	"fmt"
	"image"

	"github.com/disintegration/imaging"
)

// sanitizeJPEGQuality is the JPEG quality used for every re-encode and
// transcode. A single value keeps behavior consistent across input formats.
// 90 is visually near-lossless. HEIC carries no comparable "quality" we could
// match — HEVC QP doesn't map to JPEG quality and isn't exposed by the decoder
// — so a fixed high quality is the standard, predictable choice.
const sanitizeJPEGQuality = 90

// Image content types this package branches on. JPEG is both an input the
// orientation pass handles and the default output every non-PNG format is
// transcoded to, which is why it appears on both sides below.
const (
	contentTypeJPEG = "image/jpeg"
	contentTypeJPG  = "image/jpg"
	contentTypePNG  = "image/png"
	contentTypeTIFF = "image/tiff"
	contentTypeHEIC = "image/heic"
	contentTypeHEIF = "image/heif"
	contentTypeWebP = "image/webp"
	contentTypeGIF  = "image/gif"
)

// SanitizeImageForStorage strips all metadata from a user-supplied image and
// returns web-renderable bytes plus the (possibly changed) content type.
//
// It decodes the image, bakes any EXIF/ISOBMFF orientation into the pixels, and
// re-encodes — which drops GPS coordinates, device identifiers, capture
// timestamps, and every other metadata block as a side effect (#1953). Two
// goals are served at once:
//
//   - Privacy: the served object and any AI re-fetch no longer carry the
//     uploader's location or device.
//   - Web compatibility: HEIC and TIFF do not render in non-Safari browsers
//     (Chrome/Firefox/Edge show a broken image), so they are transcoded to
//     JPEG. PNG stays PNG (lossless, preserves alpha); every other input is
//     emitted as JPEG.
//
// GIF is passed through untouched: the format cannot carry EXIF, it renders
// everywhere, and re-encoding would flatten animation.
//
// Fail-closed contract: on any decode/encode failure the function returns an
// error and NO bytes, so the caller must reject the upload rather than store
// the unsanitized original. Callers gate this on user-originated uploads
// (stock-provider images are curated and exempt).
func SanitizeImageForStorage(data []byte, contentType string) ([]byte, string, error) {
	ct := normalizeContentType(contentType)

	// GIF: no EXIF, universally renderable, possibly animated — leave as-is.
	if ct == contentTypeGIF {
		return data, contentTypeGIF, nil
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("decode %s for sanitization: %w", ct, err)
	}

	// Bake orientation into the pixels so dropping the metadata can't leave the
	// image displayed sideways. JPEG/TIFF carry it in EXIF; HEIC in ISOBMFF.
	switch ct {
	case contentTypeJPEG, contentTypeJPG, contentTypeTIFF:
		img = applyEXIFOrientation(img, getEXIFOrientation(data))
	case contentTypeHEIC, contentTypeHEIF:
		img = applyHEICRotations(img, getHEICRotations(data))
	}

	// PNG round-trips to PNG (lossless, keeps alpha); all others become JPEG.
	outContentType := contentTypeJPEG
	format := imaging.JPEG
	encodeOpts := []imaging.EncodeOption{imaging.JPEGQuality(sanitizeJPEGQuality)}
	if ct == contentTypePNG {
		outContentType = contentTypePNG
		format = imaging.PNG
		encodeOpts = nil
	}

	var buf bytes.Buffer
	if err := imaging.Encode(&buf, img, format, encodeOpts...); err != nil {
		return nil, "", fmt.Errorf("re-encode %s for sanitization: %w", ct, err)
	}
	return buf.Bytes(), outContentType, nil
}

// webRenderableContentTypes lists the image types every target browser displays
// natively. They are stored as-is (bytes and metadata preserved); only the
// non-web formats below are transcoded.
var webRenderableContentTypes = map[string]struct{}{
	contentTypeJPEG: {},
	contentTypeJPG:  {},
	contentTypePNG:  {},
	contentTypeWebP: {},
	contentTypeGIF:  {},
}

// TranscodeForWebIfNeeded guarantees the stored bytes are renderable by the web
// client, independent of the opt-in metadata stripping (#2210).
//
// Web-native formats (jpeg/png/webp/gif) are returned UNCHANGED — bytes and
// EXIF/metadata preserved, honoring the #1953 retention decision. HEIC/HEIF and
// TIFF do not render outside Safari, so they are decoded, their orientation
// baked into the pixels, and re-encoded to JPEG.
//
// Unlike SanitizeImageForStorage, this does not re-encode web-native formats and
// so does not strip their metadata. The transcoded formats (HEIC/TIFF) do lose
// their metadata as a side effect of the re-encode — accepted here because they
// must be re-encoded to render at all; the libvips pipeline in #2210 will
// preserve it.
//
// Fail-closed: a decode/encode failure returns an error and NO bytes, so the
// caller rejects the upload rather than store an unrenderable original.
func TranscodeForWebIfNeeded(data []byte, contentType string) ([]byte, string, error) {
	ct := normalizeContentType(contentType)
	if _, ok := webRenderableContentTypes[ct]; ok {
		return data, ct, nil
	}

	// Non-web-renderable image (heic/heif/tiff). Decode and re-encode to JPEG.
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", fmt.Errorf("decode %s for web transcode: %w", ct, err)
	}
	switch ct {
	case contentTypeTIFF:
		img = applyEXIFOrientation(img, getEXIFOrientation(data))
	case contentTypeHEIC, contentTypeHEIF:
		img = applyHEICRotations(img, getHEICRotations(data))
	}

	var buf bytes.Buffer
	if err := imaging.Encode(&buf, img, imaging.JPEG, imaging.JPEGQuality(sanitizeJPEGQuality)); err != nil {
		return nil, "", fmt.Errorf("re-encode %s for web transcode: %w", ct, err)
	}
	return buf.Bytes(), contentTypeJPEG, nil
}
