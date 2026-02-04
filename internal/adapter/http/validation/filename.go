package validation

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// maxFilenameLength is the maximum allowed filename length (common filesystem limit).
const maxFilenameLength = 255

// dangerousChars contains characters that must be replaced in filenames.
// These characters can cause HTTP header injection or path traversal attacks.
var dangerousChars = map[rune]bool{
	'"':  true, // Can break Content-Disposition header quotes
	'\\': true, // Path separator on Windows, escape char
	'/':  true, // Path separator
	':':  true, // Windows drive separator, URI scheme
	'\n': true, // HTTP header injection
	'\r': true, // HTTP header injection
}

// SanitizeFilename sanitizes a filename for safe use in Content-Disposition headers
// and file paths. It:
//   - Replaces dangerous characters (quotes, backslash, newlines, control chars, path separators) with underscore
//   - Preserves Unicode characters (accented letters, CJK, emoji)
//   - Truncates to 255 characters while preserving the file extension
//   - Returns "file" for empty or whitespace-only input
func SanitizeFilename(name string) string {
	// Build sanitized filename
	var sb strings.Builder
	sb.Grow(len(name))

	for _, r := range name {
		if shouldReplace(r) {
			sb.WriteRune('_')
		} else {
			sb.WriteRune(r)
		}
	}

	result := sb.String()

	// Trim whitespace
	result = strings.TrimSpace(result)

	// Check if result is empty or only underscores
	if result == "" || isOnlyUnderscores(result) {
		return "file"
	}

	// Truncate if too long, preserving extension
	if len(result) > maxFilenameLength {
		result = truncatePreservingExtension(result)
	}

	return result
}

// shouldReplace returns true if the character should be replaced with underscore.
func shouldReplace(r rune) bool {
	// Replace control characters (< 32 and DEL 127)
	if r < 32 || r == 127 {
		return true
	}

	// Replace dangerous characters
	return dangerousChars[r]
}

// isOnlyUnderscores returns true if the string contains only underscores.
func isOnlyUnderscores(s string) bool {
	for _, r := range s {
		if r != '_' {
			return false
		}
	}
	return true
}

// truncatePreservingExtension truncates a filename to maxFilenameLength while
// preserving the file extension if possible.
func truncatePreservingExtension(name string) string {
	ext := filepath.Ext(name)
	extLen := len(ext)

	// If extension is too long or there's no extension, just truncate
	if extLen == 0 || extLen >= maxFilenameLength {
		return truncateToBytes(name, maxFilenameLength)
	}

	// Calculate max length for base name
	maxBaseLen := maxFilenameLength - extLen
	baseName := name[:len(name)-extLen]

	// Truncate base name and add extension
	truncatedBase := truncateToBytes(baseName, maxBaseLen)
	return truncatedBase + ext
}

// truncateToBytes truncates a UTF-8 string to at most maxBytes bytes,
// ensuring we don't cut in the middle of a multi-byte character.
func truncateToBytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}

	// Find the last valid UTF-8 character boundary at or before maxBytes
	for maxBytes > 0 && !utf8.ValidString(s[:maxBytes]) {
		maxBytes--
	}

	// Also ensure we don't cut in the middle of a rune
	for maxBytes > 0 {
		r, _ := utf8.DecodeLastRuneInString(s[:maxBytes])
		if r != utf8.RuneError {
			break
		}
		maxBytes--
	}

	return s[:maxBytes]
}

// ContentDisposition returns a safe Content-Disposition header value.
// It sanitizes the filename and formats it properly for HTTP headers.
//
// If inline is true, returns "inline; filename=\"...\""
// If inline is false, returns "attachment; filename=\"...\""
func ContentDisposition(filename string, inline bool) string {
	sanitized := SanitizeFilename(filename)

	disposition := "attachment"
	if inline {
		disposition = "inline"
	}

	return fmt.Sprintf("%s; filename=%q", disposition, sanitized)
}
