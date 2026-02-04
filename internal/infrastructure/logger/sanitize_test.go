package logger

import "testing"

func TestSanitizeForLog(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Normal strings pass through unchanged
		{
			name:     "normal string unchanged",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "filename unchanged",
			input:    "my-file.mp4",
			expected: "my-file.mp4",
		},
		{
			name:     "path unchanged",
			input:    "/var/data/uploads/file.txt",
			expected: "/var/data/uploads/file.txt",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},

		// Newlines escaped
		{
			name:     "newline escaped",
			input:    "line1\nline2",
			expected: "line1\\nline2",
		},
		{
			name:     "carriage return escaped",
			input:    "line1\rline2",
			expected: "line1\\rline2",
		},
		{
			name:     "CRLF escaped",
			input:    "line1\r\nline2",
			expected: "line1\\r\\nline2",
		},

		// Tab escaped
		{
			name:     "tab escaped",
			input:    "col1\tcol2",
			expected: "col1\\tcol2",
		},

		// Null byte escaped
		{
			name:     "null byte escaped",
			input:    "before\x00after",
			expected: "before\\x00after",
		},

		// ANSI escape codes escaped
		{
			name:     "ANSI escape code escaped",
			input:    "text\x1b[31mred\x1b[0mnormal",
			expected: "text\\x1b[31mred\\x1b[0mnormal",
		},
		{
			name:     "ANSI bold escaped",
			input:    "\x1b[1mBOLD\x1b[0m",
			expected: "\\x1b[1mBOLD\\x1b[0m",
		},

		// Control characters escaped as hex
		{
			name:     "bell character escaped",
			input:    "alert\x07bell",
			expected: "alert\\x07bell",
		},
		{
			name:     "backspace escaped",
			input:    "back\x08space",
			expected: "back\\x08space",
		},
		{
			name:     "form feed escaped",
			input:    "form\x0cfeed",
			expected: "form\\x0cfeed",
		},
		{
			name:     "vertical tab escaped",
			input:    "vert\x0btab",
			expected: "vert\\x0btab",
		},
		{
			name:     "DEL character escaped",
			input:    "delete\x7fchar",
			expected: "delete\\x7fchar",
		},

		// Unicode preserved
		{
			name:     "unicode accented chars preserved",
			input:    "café résumé naïve",
			expected: "café résumé naïve",
		},
		{
			name:     "unicode emoji preserved",
			input:    "hello 👋 world 🌍",
			expected: "hello 👋 world 🌍",
		},
		{
			name:     "unicode chinese preserved",
			input:    "中文文件名.mp4",
			expected: "中文文件名.mp4",
		},
		{
			name:     "unicode japanese preserved",
			input:    "日本語ファイル.txt",
			expected: "日本語ファイル.txt",
		},
		{
			name:     "unicode arabic preserved",
			input:    "ملف عربي.pdf",
			expected: "ملف عربي.pdf",
		},

		// Log injection attack patterns
		{
			name:     "fake log entry injection",
			input:    "file.mp4\nERROR: fake log entry",
			expected: "file.mp4\\nERROR: fake log entry",
		},
		{
			name:     "multiple fake entries",
			input:    "upload\n2024-01-01 12:00:00 INFO: fake\n2024-01-01 12:00:01 ERROR: attack",
			expected: "upload\\n2024-01-01 12:00:00 INFO: fake\\n2024-01-01 12:00:01 ERROR: attack",
		},
		{
			name:     "terminal clear attempt",
			input:    "\x1b[2Jcleared",
			expected: "\\x1b[2Jcleared",
		},
		{
			name:     "cursor movement attempt",
			input:    "\x1b[10;20Hmoved",
			expected: "\\x1b[10;20Hmoved",
		},

		// Mixed content
		{
			name:     "mixed unicode and control chars",
			input:    "файл\nновая строка",
			expected: "файл\\nновая строка",
		},
		{
			name:     "filename with special chars",
			input:    "my file (1).mp4",
			expected: "my file (1).mp4",
		},
		{
			name:     "filename with quotes",
			input:    `file "name".txt`,
			expected: `file "name".txt`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeForLog(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeForLog(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeForLog_AllControlChars(t *testing.T) {
	// Test all control characters (0-31 and 127) are escaped
	for i := 0; i < 32; i++ {
		input := string(rune(i))
		result := SanitizeForLog(input)

		// Should not contain the raw control character
		if result == input && i != 0 { // null has special handling
			t.Errorf("Control char %d (0x%02x) was not escaped", i, i)
		}

		// Result should contain escape sequence
		if len(result) < 2 {
			// Special cases for \n, \r, \t, \x00 which become 2-char sequences
			switch i {
			case 0: // \x00
				if result != "\\x00" {
					t.Errorf("Null byte not properly escaped: got %q", result)
				}
			case 9: // \t
				if result != "\\t" {
					t.Errorf("Tab not properly escaped: got %q", result)
				}
			case 10: // \n
				if result != "\\n" {
					t.Errorf("Newline not properly escaped: got %q", result)
				}
			case 13: // \r
				if result != "\\r" {
					t.Errorf("Carriage return not properly escaped: got %q", result)
				}
			}
		}
	}

	// Test DEL character (127)
	result := SanitizeForLog(string(rune(127)))
	if result != "\\x7f" {
		t.Errorf("DEL char (127) not properly escaped: got %q, want %q", result, "\\x7f")
	}
}

func BenchmarkSanitizeForLog(b *testing.B) {
	testCases := []struct {
		name  string
		input string
	}{
		{"short_clean", "file.mp4"},
		{"long_clean", "this is a longer filename with spaces and numbers 12345.mp4"},
		{"with_newlines", "file\nwith\nnewlines.txt"},
		{"unicode", "中文文件名_日本語_emoji_👋.mp4"},
		{"mixed_attack", "file.mp4\nERROR: fake\x1b[31mred\x1b[0m"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				_ = SanitizeForLog(tc.input)
			}
		})
	}
}
