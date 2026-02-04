package validation

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test data: magic bytes for various file types
var (
	// Allowed types
	jpegMagic = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46}
	pngMagic  = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	gifMagic  = []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61} // GIF89a
	webpMagic = []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50}
	mp4Magic  = []byte{0x00, 0x00, 0x00, 0x18, 0x66, 0x74, 0x79, 0x70, 0x69, 0x73, 0x6F, 0x6D}
	webmMagic = []byte{0x1A, 0x45, 0xDF, 0xA3}             // EBML header
	mp3Magic  = []byte{0xFF, 0xFB, 0x90, 0x00}             // MP3 without ID3
	mp3ID3    = []byte{0x49, 0x44, 0x33, 0x04, 0x00, 0x00} // ID3 tag
	oggMagic  = []byte{0x4F, 0x67, 0x67, 0x53, 0x00, 0x02} // OggS
	wavMagic  = []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x41, 0x56, 0x45}
	flacMagic = []byte{0x66, 0x4C, 0x61, 0x43} // fLaC

	// Disallowed types
	phpMagic   = []byte("<?php echo 'hello'; ?>")
	htmlMagic  = []byte("<!DOCTYPE html><html><body></body></html>")
	jsMagic    = []byte("function malicious() { alert('xss'); }")
	exeMagic   = []byte{0x4D, 0x5A, 0x90, 0x00, 0x03, 0x00, 0x00, 0x00} // MZ header
	emptyMagic = []byte{}
)

// padBytes pads the magic bytes to ensure enough data for detection
func padBytes(magic []byte, size int) []byte {
	if len(magic) >= size {
		return magic
	}
	result := make([]byte, size)
	copy(result, magic)
	return result
}

// --- Tests for allowed file types ---

func TestValidateMagicBytes_JPEG_Allowed(t *testing.T) {
	reader := bytes.NewReader(padBytes(jpegMagic, 512))
	mime, allowed, err := ValidateMagicBytes(reader)

	require.NoError(t, err)
	assert.True(t, allowed, "JPEG should be allowed")
	assert.Equal(t, "image/jpeg", mime)
}

func TestValidateMagicBytes_PNG_Allowed(t *testing.T) {
	reader := bytes.NewReader(padBytes(pngMagic, 512))
	mime, allowed, err := ValidateMagicBytes(reader)

	require.NoError(t, err)
	assert.True(t, allowed, "PNG should be allowed")
	assert.Equal(t, "image/png", mime)
}

func TestValidateMagicBytes_GIF_Allowed(t *testing.T) {
	reader := bytes.NewReader(padBytes(gifMagic, 512))
	mime, allowed, err := ValidateMagicBytes(reader)

	require.NoError(t, err)
	assert.True(t, allowed, "GIF should be allowed")
	assert.Equal(t, "image/gif", mime)
}

func TestValidateMagicBytes_WebP_Allowed(t *testing.T) {
	reader := bytes.NewReader(padBytes(webpMagic, 512))
	mime, allowed, err := ValidateMagicBytes(reader)

	require.NoError(t, err)
	assert.True(t, allowed, "WebP should be allowed")
	assert.Equal(t, "image/webp", mime)
}

func TestValidateMagicBytes_MP4_Allowed(t *testing.T) {
	reader := bytes.NewReader(padBytes(mp4Magic, 512))
	mime, allowed, err := ValidateMagicBytes(reader)

	require.NoError(t, err)
	assert.True(t, allowed, "MP4 should be allowed")
	assert.Equal(t, "video/mp4", mime)
}

func TestValidateMagicBytes_WebM_Allowed(t *testing.T) {
	reader := bytes.NewReader(padBytes(webmMagic, 512))
	mime, allowed, err := ValidateMagicBytes(reader)

	require.NoError(t, err)
	assert.True(t, allowed, "WebM should be allowed")
	assert.Equal(t, "video/webm", mime)
}

func TestValidateMagicBytes_MP3_WithoutID3_Allowed(t *testing.T) {
	reader := bytes.NewReader(padBytes(mp3Magic, 512))
	mime, allowed, err := ValidateMagicBytes(reader)

	require.NoError(t, err)
	assert.True(t, allowed, "MP3 without ID3 should be allowed")
	assert.Equal(t, "audio/mpeg", mime)
}

func TestValidateMagicBytes_MP3_WithID3_Allowed(t *testing.T) {
	reader := bytes.NewReader(padBytes(mp3ID3, 512))
	mime, allowed, err := ValidateMagicBytes(reader)

	require.NoError(t, err)
	assert.True(t, allowed, "MP3 with ID3 should be allowed")
	assert.Equal(t, "audio/mpeg", mime)
}

func TestValidateMagicBytes_OGG_Allowed(t *testing.T) {
	reader := bytes.NewReader(padBytes(oggMagic, 512))
	mime, allowed, err := ValidateMagicBytes(reader)

	require.NoError(t, err)
	assert.True(t, allowed, "OGG should be allowed")
	assert.Contains(t, []string{"audio/ogg", "application/ogg"}, mime)
}

func TestValidateMagicBytes_WAV_Allowed(t *testing.T) {
	reader := bytes.NewReader(padBytes(wavMagic, 512))
	mime, allowed, err := ValidateMagicBytes(reader)

	require.NoError(t, err)
	assert.True(t, allowed, "WAV should be allowed")
	assert.Contains(t, []string{"audio/wav", "audio/wave", "audio/x-wav"}, mime)
}

func TestValidateMagicBytes_FLAC_Allowed(t *testing.T) {
	reader := bytes.NewReader(padBytes(flacMagic, 512))
	mime, allowed, err := ValidateMagicBytes(reader)

	require.NoError(t, err)
	assert.True(t, allowed, "FLAC should be allowed")
	assert.Contains(t, []string{"audio/flac", "audio/x-flac"}, mime)
}

// --- Tests for disallowed file types ---

func TestValidateMagicBytes_PHP_Rejected(t *testing.T) {
	reader := bytes.NewReader(padBytes(phpMagic, 512))
	mime, allowed, err := ValidateMagicBytes(reader)

	require.NoError(t, err)
	assert.False(t, allowed, "PHP should be rejected")
	assert.NotEmpty(t, mime)
}

func TestValidateMagicBytes_HTML_Rejected(t *testing.T) {
	reader := bytes.NewReader(padBytes(htmlMagic, 512))
	mime, allowed, err := ValidateMagicBytes(reader)

	require.NoError(t, err)
	assert.False(t, allowed, "HTML should be rejected")
	assert.NotEmpty(t, mime)
}

func TestValidateMagicBytes_JavaScript_Rejected(t *testing.T) {
	reader := bytes.NewReader(padBytes(jsMagic, 512))
	mime, allowed, err := ValidateMagicBytes(reader)

	require.NoError(t, err)
	assert.False(t, allowed, "JavaScript should be rejected")
	assert.NotEmpty(t, mime)
}

func TestValidateMagicBytes_EXE_Rejected(t *testing.T) {
	reader := bytes.NewReader(padBytes(exeMagic, 512))
	mime, allowed, err := ValidateMagicBytes(reader)

	require.NoError(t, err)
	assert.False(t, allowed, "EXE should be rejected")
	assert.NotEmpty(t, mime)
}

func TestValidateMagicBytes_Empty_Rejected(t *testing.T) {
	reader := bytes.NewReader(emptyMagic)
	mime, allowed, err := ValidateMagicBytes(reader)

	require.NoError(t, err)
	assert.False(t, allowed, "Empty file should be rejected")
	assert.NotEmpty(t, mime)
}

// --- Tests for reader position reset ---

func TestValidateMagicBytes_ReaderPositionReset(t *testing.T) {
	originalData := padBytes(jpegMagic, 512)
	reader := bytes.NewReader(originalData)

	// Validate should read and reset
	_, _, err := ValidateMagicBytes(reader)
	require.NoError(t, err)

	// Verify reader is at position 0
	pos, err := reader.Seek(0, io.SeekCurrent)
	require.NoError(t, err)
	assert.Equal(t, int64(0), pos, "Reader position should be reset to 0")

	// Verify we can read the same data again
	readData := make([]byte, len(originalData))
	n, err := reader.Read(readData)
	require.NoError(t, err)
	assert.Equal(t, len(originalData), n)
	assert.Equal(t, originalData, readData)
}

func TestValidateMagicBytes_ReaderPositionResetAfterPartialRead(t *testing.T) {
	originalData := padBytes(pngMagic, 1024)
	reader := bytes.NewReader(originalData)

	// Read some bytes first to offset the position
	tmpBuf := make([]byte, 100)
	_, err := reader.Read(tmpBuf)
	require.NoError(t, err)

	// Reset manually for the test setup
	_, err = reader.Seek(0, io.SeekStart)
	require.NoError(t, err)

	// Now validate
	_, _, err = ValidateMagicBytes(reader)
	require.NoError(t, err)

	// Verify reader is at position 0
	pos, err := reader.Seek(0, io.SeekCurrent)
	require.NoError(t, err)
	assert.Equal(t, int64(0), pos, "Reader position should be reset to 0")
}

// --- Tests for error handling ---

func TestValidateMagicBytes_SmallFile_NoError(t *testing.T) {
	// File smaller than 512 bytes should still work
	smallData := []byte{0xFF, 0xD8, 0xFF, 0xE0}
	reader := bytes.NewReader(smallData)

	mime, allowed, err := ValidateMagicBytes(reader)
	require.NoError(t, err)
	assert.True(t, allowed, "Small JPEG-like file should still be validated")
	assert.NotEmpty(t, mime)
}

// --- Table-driven tests for comprehensive coverage ---

func TestValidateMagicBytes_AllowedTypes_TableDriven(t *testing.T) {
	tests := []struct {
		name         string
		magic        []byte
		expectedMIME string
	}{
		{"JPEG", jpegMagic, "image/jpeg"},
		{"PNG", pngMagic, "image/png"},
		{"GIF", gifMagic, "image/gif"},
		{"WebP", webpMagic, "image/webp"},
		{"MP4", mp4Magic, "video/mp4"},
		{"WebM", webmMagic, "video/webm"},
		{"MP3 no ID3", mp3Magic, "audio/mpeg"},
		{"MP3 ID3", mp3ID3, "audio/mpeg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bytes.NewReader(padBytes(tt.magic, 512))
			mime, allowed, err := ValidateMagicBytes(reader)

			require.NoError(t, err)
			assert.True(t, allowed, "%s should be allowed", tt.name)
			assert.Equal(t, tt.expectedMIME, mime)
		})
	}
}

func TestValidateMagicBytes_RejectedTypes_TableDriven(t *testing.T) {
	tests := []struct {
		name  string
		magic []byte
	}{
		{"PHP script", phpMagic},
		{"HTML document", htmlMagic},
		{"JavaScript", jsMagic},
		{"Windows EXE", exeMagic},
		{"Empty file", emptyMagic},
		{"Random binary", []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}},
		{"Text file", []byte("Hello, this is plain text content.")},
		{"JSON", []byte(`{"key": "value"}`)},
		{"XML", []byte(`<?xml version="1.0"?><root></root>`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bytes.NewReader(padBytes(tt.magic, 512))
			_, allowed, err := ValidateMagicBytes(reader)

			require.NoError(t, err)
			assert.False(t, allowed, "%s should be rejected", tt.name)
		})
	}
}

// --- Test ErrDisallowedFileType ---

func TestErrDisallowedFileType_Defined(t *testing.T) {
	assert.NotNil(t, ErrDisallowedFileType)
	assert.Equal(t, "file type not allowed", ErrDisallowedFileType.Error())
}
