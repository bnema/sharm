package validation

import (
	"strings"
	"testing"
)

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Normal filenames pass through unchanged
		{
			name:     "simple filename",
			input:    "video.mp4",
			expected: "video.mp4",
		},
		{
			name:     "filename with spaces",
			input:    "my video file.mp4",
			expected: "my video file.mp4",
		},
		{
			name:     "filename with multiple dots",
			input:    "file.name.with.dots.mp4",
			expected: "file.name.with.dots.mp4",
		},
		{
			name:     "filename with numbers",
			input:    "video123.mp4",
			expected: "video123.mp4",
		},
		{
			name:     "filename with dashes and underscores",
			input:    "my-video_file.mp4",
			expected: "my-video_file.mp4",
		},

		// Unicode filenames are preserved
		{
			name:     "unicode french",
			input:    "vidéo.mp4",
			expected: "vidéo.mp4",
		},
		{
			name:     "unicode japanese",
			input:    "動画.mp4",
			expected: "動画.mp4",
		},
		{
			name:     "unicode chinese",
			input:    "视频文件.mp4",
			expected: "视频文件.mp4",
		},
		{
			name:     "unicode emoji",
			input:    "my🎬video.mp4",
			expected: "my🎬video.mp4",
		},

		// Dangerous characters replaced with underscore
		{
			name:     "double quote",
			input:    `file"name.mp4`,
			expected: "file_name.mp4",
		},
		{
			name:     "backslash",
			input:    `file\name.mp4`,
			expected: "file_name.mp4",
		},
		{
			name:     "newline LF",
			input:    "file\nname.mp4",
			expected: "file_name.mp4",
		},
		{
			name:     "newline CR",
			input:    "file\rname.mp4",
			expected: "file_name.mp4",
		},
		{
			name:     "newline CRLF",
			input:    "file\r\nname.mp4",
			expected: "file__name.mp4",
		},
		{
			name:     "control character NUL",
			input:    "file\x00name.mp4",
			expected: "file_name.mp4",
		},
		{
			name:     "control character BEL",
			input:    "file\x07name.mp4",
			expected: "file_name.mp4",
		},
		{
			name:     "control character DEL",
			input:    "file\x7Fname.mp4",
			expected: "file_name.mp4",
		},
		{
			name:     "forward slash",
			input:    "file/name.mp4",
			expected: "file_name.mp4",
		},
		{
			name:     "colon",
			input:    "file:name.mp4",
			expected: "file_name.mp4",
		},

		// Path traversal attempts sanitized
		{
			name:     "path traversal parent directory",
			input:    "../../../etc/passwd",
			expected: ".._.._.._etc_passwd",
		},
		{
			name:     "path traversal hidden file",
			input:    "..secret.mp4",
			expected: "..secret.mp4",
		},
		{
			name:     "path traversal with backslash",
			input:    `..\..\secret.mp4`,
			expected: ".._.._secret.mp4",
		},

		// Empty filename returns "file"
		{
			name:     "empty string",
			input:    "",
			expected: "file",
		},
		{
			name:     "only whitespace",
			input:    "   ",
			expected: "file",
		},
		{
			name:     "only dangerous chars",
			input:    `"/\:`,
			expected: "file",
		},

		// Multiple dangerous chars
		{
			name:     "multiple dangerous chars",
			input:    `"file\with:bad/chars"`,
			expected: "_file_with_bad_chars_",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeFilename(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeFilename_LongFilenames(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		maxLen     int
		wantLen    int
		wantHasExt bool
		wantExt    string
	}{
		{
			name:       "filename at limit",
			input:      strings.Repeat("a", 255),
			wantLen:    255,
			wantHasExt: false,
		},
		{
			name:       "filename over limit no extension",
			input:      strings.Repeat("a", 300),
			wantLen:    255,
			wantHasExt: false,
		},
		{
			name:       "long filename preserves extension",
			input:      strings.Repeat("a", 300) + ".mp4",
			wantLen:    255,
			wantHasExt: true,
			wantExt:    ".mp4",
		},
		{
			name:       "long filename preserves long extension",
			input:      strings.Repeat("a", 300) + ".jpeg",
			wantLen:    255,
			wantHasExt: true,
			wantExt:    ".jpeg",
		},
		{
			name:       "filename exactly 255 with extension",
			input:      strings.Repeat("a", 251) + ".mp4",
			wantLen:    255,
			wantHasExt: true,
			wantExt:    ".mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeFilename(tt.input)
			if len(result) > 255 {
				t.Errorf("SanitizeFilename(%q) len = %d, want <= 255", tt.input[:50]+"...", len(result))
			}
			if len(result) != tt.wantLen {
				t.Errorf("SanitizeFilename(%q) len = %d, want %d", tt.input[:50]+"...", len(result), tt.wantLen)
			}
			if tt.wantHasExt {
				if !strings.HasSuffix(result, tt.wantExt) {
					t.Errorf("SanitizeFilename(%q) = %q, want suffix %q", tt.input[:50]+"...", result, tt.wantExt)
				}
			}
		})
	}
}

func TestContentDisposition(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		inline   bool
		expected string
	}{
		{
			name:     "inline simple filename",
			filename: "video.mp4",
			inline:   true,
			expected: `inline; filename="video.mp4"`,
		},
		{
			name:     "attachment simple filename",
			filename: "video.mp4",
			inline:   false,
			expected: `attachment; filename="video.mp4"`,
		},
		{
			name:     "inline with unicode",
			filename: "vidéo.mp4",
			inline:   true,
			expected: `inline; filename="vidéo.mp4"`,
		},
		{
			name:     "sanitizes dangerous chars",
			filename: `bad"file\name.mp4`,
			inline:   true,
			expected: `inline; filename="bad_file_name.mp4"`,
		},
		{
			name:     "sanitizes newlines",
			filename: "file\r\nname.mp4",
			inline:   true,
			expected: `inline; filename="file__name.mp4"`,
		},
		{
			name:     "empty filename returns file",
			filename: "",
			inline:   true,
			expected: `inline; filename="file"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ContentDisposition(tt.filename, tt.inline)
			if result != tt.expected {
				t.Errorf("ContentDisposition(%q, %v) = %q, want %q", tt.filename, tt.inline, result, tt.expected)
			}
		})
	}
}

func TestContentDisposition_NoUnescapedQuotes(t *testing.T) {
	// Ensure the result never contains unescaped quotes in the filename part
	maliciousFilenames := []string{
		`file"name.mp4`,
		`"start.mp4`,
		`end".mp4`,
		`"both".mp4`,
		`file""double.mp4`,
		`injection"; evil=header`,
		"header\r\nX-Injected: value",
	}

	for _, filename := range maliciousFilenames {
		t.Run(filename, func(t *testing.T) {
			result := ContentDisposition(filename, true)

			// Extract filename value between quotes
			// Format is: inline; filename="FILENAME"
			prefix := `inline; filename="`
			suffix := `"`
			if !strings.HasPrefix(result, prefix) || !strings.HasSuffix(result, suffix) {
				t.Errorf("ContentDisposition(%q) = %q, unexpected format", filename, result)
				return
			}

			// Get the filename part
			filenameValue := result[len(prefix) : len(result)-len(suffix)]

			// Check for unescaped quotes
			if strings.Contains(filenameValue, `"`) {
				t.Errorf("ContentDisposition(%q) contains unescaped quotes in filename: %q", filename, result)
			}

			// Check for newlines (HTTP header injection)
			if strings.Contains(filenameValue, "\n") || strings.Contains(filenameValue, "\r") {
				t.Errorf("ContentDisposition(%q) contains newlines in filename: %q", filename, result)
			}
		})
	}
}
