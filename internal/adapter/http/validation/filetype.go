// Package validation provides file type validation utilities for upload security.
package validation

import (
	"errors"
	"io"
	"net/http"
)

// ErrDisallowedFileType is returned when a file type is not in the allowlist.
var ErrDisallowedFileType = errors.New("file type not allowed")

// allowedMIMETypes defines the allowlist of media MIME types accepted for upload.
var allowedMIMETypes = map[string]bool{
	// Images
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
	// Videos
	"video/mp4":       true,
	"video/webm":      true,
	"video/quicktime": true,
	// Audio
	"audio/mpeg":    true,
	"audio/ogg":     true,
	"application/ogg": true,
	"audio/wav":     true,
	"audio/wave":    true,
	"audio/x-wav":   true,
	"audio/flac":    true,
	"audio/x-flac":  true,
}

// magicBytesBufferSize is the number of bytes to read for content type detection.
const magicBytesBufferSize = 512

// ValidateMagicBytes validates a file's content type by reading its magic bytes.
// It uses http.DetectContentType for standard detection and includes custom
// detection for formats not well-supported by the standard library.
//
// The function reads up to 512 bytes from the reader, detects the MIME type,
// and resets the reader position to the beginning.
//
// Returns:
//   - mime: the detected MIME type
//   - allowed: whether the file type is in the allowlist
//   - err: any error encountered during reading or seeking
func ValidateMagicBytes(reader io.ReadSeeker) (mime string, allowed bool, err error) {
	// Read up to 512 bytes for content type detection
	buf := make([]byte, magicBytesBufferSize)
	n, err := reader.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", false, err
	}

	// Reset reader position to beginning
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return "", false, err
	}

	// Handle empty files
	if n == 0 {
		return "application/octet-stream", false, nil
	}

	// Trim buffer to actual read size
	buf = buf[:n]

	// Check for custom magic bytes that http.DetectContentType may not handle well
	mime = detectCustomMagicBytes(buf)
	if mime == "" {
		// Fall back to standard detection
		mime = http.DetectContentType(buf)
	}

	// Check if MIME type is allowed
	allowed = allowedMIMETypes[mime]

	return mime, allowed, nil
}

// detectCustomMagicBytes handles detection of file types that http.DetectContentType
// may not recognize correctly.
func detectCustomMagicBytes(buf []byte) string {
	if len(buf) < 4 {
		return ""
	}

	// WebM/Matroska: EBML header (0x1A 0x45 0xDF 0xA3)
	if buf[0] == 0x1A && buf[1] == 0x45 && buf[2] == 0xDF && buf[3] == 0xA3 {
		return "video/webm"
	}

	// FLAC: starts with "fLaC"
	if buf[0] == 'f' && buf[1] == 'L' && buf[2] == 'a' && buf[3] == 'C' {
		return "audio/flac"
	}

	// MP3 without ID3: Frame sync (0xFF 0xFB, 0xFF 0xFA, 0xFF 0xF3, 0xFF 0xF2)
	// These are MPEG Audio Layer III frame headers
	if len(buf) >= 2 && buf[0] == 0xFF {
		switch buf[1] & 0xFE {
		case 0xFA, 0xF2: // MPEG1/2 Layer 3
			return "audio/mpeg"
		}
	}

	// ID3 tag (common for MP3): starts with "ID3"
	if buf[0] == 'I' && buf[1] == 'D' && buf[2] == '3' {
		return "audio/mpeg"
	}

	// WebP: RIFF....WEBP (bytes 0-3: RIFF, bytes 8-11: WEBP)
	if len(buf) >= 12 {
		if buf[0] == 'R' && buf[1] == 'I' && buf[2] == 'F' && buf[3] == 'F' &&
			buf[8] == 'W' && buf[9] == 'E' && buf[10] == 'B' && buf[11] == 'P' {
			return "image/webp"
		}
	}

	// MP4/QuickTime: ftyp box at offset 4 (bytes 4-7: "ftyp")
	// The format is: [4 bytes size][4 bytes "ftyp"][brand...]
	if len(buf) >= 12 {
		if buf[4] == 'f' && buf[5] == 't' && buf[6] == 'y' && buf[7] == 'p' {
			// Check brand to distinguish MP4 variants
			brand := string(buf[8:12])
			switch brand {
			case "isom", "iso2", "iso3", "iso4", "iso5", "iso6", "mp41", "mp42", "avc1", "M4V ", "M4A ":
				return "video/mp4"
			case "qt  ":
				return "video/quicktime"
			default:
				// Default to MP4 for unknown ftyp brands
				return "video/mp4"
			}
		}
	}

	return ""
}
