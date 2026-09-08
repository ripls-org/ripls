package storage

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"strings"
	"testing"
)

// pngHeaderWithDimensions builds a minimal PNG (signature + a valid IHDR chunk)
// declaring the given width/height. image.DecodeConfig reads dimensions from
// IHDR without needing pixel data, so this is enough to exercise the
// decompression-bomb guard without allocating a giant buffer.
func pngHeaderWithDimensions(width, height uint32) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A})

	ihdr := make([]byte, 0, 17)
	ihdr = append(ihdr, 'I', 'H', 'D', 'R')
	var wh [8]byte
	binary.BigEndian.PutUint32(wh[0:4], width)
	binary.BigEndian.PutUint32(wh[4:8], height)
	ihdr = append(ihdr, wh[:]...)
	ihdr = append(ihdr, 8, 6, 0, 0, 0) // bit depth 8, color type 6 (RGBA), no compression/filter/interlace

	var length [4]byte
	binary.BigEndian.PutUint32(length[:], 13)
	buf.Write(length[:])
	buf.Write(ihdr)
	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.ChecksumIEEE(ihdr))
	buf.Write(crc[:])
	return buf.Bytes()
}

func TestValidateMediaBytes_DimensionBomb(t *testing.T) {
	// A small PNG declaring 100000x100000 (10 billion px) is rejected before any
	// full decode would allocate.
	bomb := pngHeaderWithDimensions(100000, 100000)
	if err := ValidateMediaBytes(bomb, "image/png"); !errors.Is(err, ErrImageDimensionsExceeded) {
		t.Fatalf("expected ErrImageDimensionsExceeded, got %v", err)
	}

	// A normal-sized PNG header passes the dimension guard.
	ok := pngHeaderWithDimensions(1024, 768)
	if err := ValidateMediaBytes(ok, "image/png"); err != nil {
		t.Fatalf("expected accept for 1024x768, got %v", err)
	}
}

// Minimal byte prefixes sufficient for http.DetectContentType; ValidateMediaBytes
// only sniffs, it does not fully decode.
var (
	jpegBytes = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
	pngBytes  = []byte("\x89PNG\r\n\x1a\n0000000000000000")
	// "GIF89a" + logical screen descriptor declaring 16x16, no global color table.
	gifBytes  = []byte{'G', 'I', 'F', '8', '9', 'a', 0x10, 0x00, 0x10, 0x00, 0x00, 0x00, 0x00}
	htmlBytes = []byte("<!DOCTYPE html><html><body>hi</body></html>")
	svgBytes  = []byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)
	// HEIC/MP4 share the ISOBMFF "....ftyp<brand>" shape; http.DetectContentType
	// reports application/octet-stream (or video/mp4) for these.
	heicBytes = []byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c', 0, 0, 0, 0}
	mp4Bytes  = []byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 0, 0, 0}
	pdfBytes  = []byte("%PDF-1.7\n0000000000")
)

func TestValidateMediaBytes(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		data        []byte
		wantErr     error // nil = accept
	}{
		{"jpeg ok", "image/jpeg", jpegBytes, nil},
		{"jpeg charset param ok", "image/jpeg; charset=binary", jpegBytes, nil},
		{"jpg alias ok", "image/jpg", jpegBytes, nil},
		{"png ok", "image/png", pngBytes, nil},
		{"gif ok", "image/gif", gifBytes, nil},
		{"heic octet-stream ok", "image/heic", heicBytes, nil},
		{"mp4 ok", "video/mp4", mp4Bytes, nil},
		{"benign image format mismatch ok", "image/heic", jpegBytes, nil}, // both image, allow-listed

		{"svg rejected by allow-list", "image/svg+xml", svgBytes, ErrUnsupportedMediaType},
		{"pdf rejected by allow-list", "application/pdf", pdfBytes, ErrUnsupportedMediaType},
		{"unknown type rejected", "application/x-msdownload", []byte("MZ00000000"), ErrUnsupportedMediaType},

		{"empty payload rejected", "image/jpeg", nil, ErrMediaContentMismatch},
		{"image declaring html rejected", "image/jpeg", htmlBytes, ErrMediaContentMismatch},
		{"image declaring svg-text rejected", "image/png", svgBytes, ErrMediaContentMismatch},
		{"video declaring html rejected", "video/mp4", htmlBytes, ErrMediaContentMismatch},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateMediaBytes(tc.data, tc.contentType)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("expected accept, got %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidateMediaBytes_SizeCaps(t *testing.T) {
	// Image over the image cap is rejected (the size check precedes the sniff,
	// so a benign jpeg prefix on an oversized buffer still trips ErrMediaTooLarge).
	oversizeImage := make([]byte, MaxImageUploadBytes+1)
	copy(oversizeImage, jpegBytes)
	if err := ValidateMediaBytes(oversizeImage, "image/jpeg"); !errors.Is(err, ErrMediaTooLarge) {
		t.Fatalf("oversize image: expected ErrMediaTooLarge, got %v", err)
	}

	// An image at exactly the image cap is accepted (boundary).
	atImageCap := make([]byte, MaxImageUploadBytes)
	copy(atImageCap, jpegBytes)
	if err := ValidateMediaBytes(atImageCap, "image/jpeg"); err != nil {
		t.Fatalf("image at cap: expected accept, got %v", err)
	}

	// A video the size of an image+1 is still well under the video cap → accepted.
	smallVideo := make([]byte, MaxImageUploadBytes+1)
	copy(smallVideo, mp4Bytes)
	if err := ValidateMediaBytes(smallVideo, "video/mp4"); err != nil {
		t.Fatalf("video under video cap: expected accept, got %v", err)
	}
}

// TestValidateMediaBytes_RejectionMessagesAreSafe guards that the error text
// names the type/size but never echoes payload bytes.
func TestValidateMediaBytes_RejectionMessagesAreSafe(t *testing.T) {
	err := ValidateMediaBytes(htmlBytes, "image/jpeg")
	if err == nil {
		t.Fatal("expected rejection")
	}
	if strings.Contains(err.Error(), "<html>") || strings.Contains(err.Error(), "DOCTYPE") {
		t.Errorf("error message leaked payload bytes: %q", err)
	}
}
