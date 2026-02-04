package ffmpeg

import (
	"errors"
	"testing"
)

func TestValidatePath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr error
	}{
		{
			name:    "valid path",
			path:    "/tmp/video.mp4",
			wantErr: nil,
		},
		{
			name:    "valid path with spaces",
			path:    "/tmp/my video.mp4",
			wantErr: nil,
		},
		{
			name:    "valid relative path",
			path:    "video.mp4",
			wantErr: nil,
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: ErrEmptyPath,
		},
		{
			name:    "path with null byte at start",
			path:    "\x00/tmp/video.mp4",
			wantErr: ErrInvalidPath,
		},
		{
			name:    "path with null byte in middle",
			path:    "/tmp/\x00video.mp4",
			wantErr: ErrInvalidPath,
		},
		{
			name:    "path with null byte at end",
			path:    "/tmp/video.mp4\x00",
			wantErr: ErrInvalidPath,
		},
		{
			name:    "path with multiple null bytes",
			path:    "/tmp/\x00video\x00.mp4",
			wantErr: ErrInvalidPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePath(tt.path)
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("validatePath(%q) = %v, want %v", tt.path, err, tt.wantErr)
			}
		})
	}
}

func TestConverter_Convert_PathValidation(t *testing.T) {
	c := &Converter{}

	tests := []struct {
		name      string
		inputPath string
		outputDir string
		id        string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "empty input path",
			inputPath: "",
			outputDir: "/tmp",
			id:        "test",
			wantErr:   true,
			errMsg:    "invalid input path",
		},
		{
			name:      "empty output dir",
			inputPath: "/tmp/video.mp4",
			outputDir: "",
			id:        "test",
			wantErr:   true,
			errMsg:    "invalid output dir",
		},
		{
			name:      "null byte in input path",
			inputPath: "/tmp/\x00video.mp4",
			outputDir: "/tmp",
			id:        "test",
			wantErr:   true,
			errMsg:    "invalid input path",
		},
		{
			name:      "null byte in output dir",
			inputPath: "/tmp/video.mp4",
			outputDir: "/tmp/\x00output",
			id:        "test",
			wantErr:   true,
			errMsg:    "invalid output dir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := c.Convert(tt.inputPath, tt.outputDir, tt.id)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Convert() expected error containing %q, got nil", tt.errMsg)
					return
				}
				if !containsString(err.Error(), tt.errMsg) {
					t.Errorf("Convert() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestConverter_Thumbnail_PathValidation(t *testing.T) {
	c := &Converter{}

	tests := []struct {
		name       string
		inputPath  string
		outputPath string
		wantErr    bool
		errMsg     string
	}{
		{
			name:       "empty input path",
			inputPath:  "",
			outputPath: "/tmp/thumb.jpg",
			wantErr:    true,
			errMsg:     "invalid input path",
		},
		{
			name:       "empty output path",
			inputPath:  "/tmp/video.mp4",
			outputPath: "",
			wantErr:    true,
			errMsg:     "invalid output path",
		},
		{
			name:       "null byte in input",
			inputPath:  "/tmp/\x00video.mp4",
			outputPath: "/tmp/thumb.jpg",
			wantErr:    true,
			errMsg:     "invalid input path",
		},
		{
			name:       "null byte in output",
			inputPath:  "/tmp/video.mp4",
			outputPath: "/tmp/\x00thumb.jpg",
			wantErr:    true,
			errMsg:     "invalid output path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := c.Thumbnail(tt.inputPath, tt.outputPath)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Thumbnail() expected error containing %q, got nil", tt.errMsg)
					return
				}
				if !containsString(err.Error(), tt.errMsg) {
					t.Errorf("Thumbnail() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func TestConverter_Probe_PathValidation(t *testing.T) {
	c := &Converter{}

	tests := []struct {
		name      string
		inputPath string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "empty input path",
			inputPath: "",
			wantErr:   true,
			errMsg:    "invalid input path",
		},
		{
			name:      "null byte in input path",
			inputPath: "/tmp/\x00video.mp4",
			wantErr:   true,
			errMsg:    "invalid input path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := c.Probe(tt.inputPath)
			if tt.wantErr {
				if err == nil {
					t.Errorf("Probe() expected error containing %q, got nil", tt.errMsg)
					return
				}
				if !containsString(err.Error(), tt.errMsg) {
					t.Errorf("Probe() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
