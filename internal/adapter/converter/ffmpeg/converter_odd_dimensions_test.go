package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bnema/sharm/internal/domain"
)

func requireFFmpeg(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping ffmpeg integration test in short mode")
	}
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not available")
	}
	return path
}

func requireFFprobe(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not available")
	}
}

func requireEncoder(t *testing.T, ffmpeg, encoder string) {
	t.Helper()
	out, err := exec.Command(ffmpeg, "-hide_banner", "-encoders").Output()
	require.NoError(t, err)
	if !strings.Contains(string(out), encoder) {
		t.Skipf("encoder %s not available", encoder)
	}
}

// generateOddDimensionSource creates a small video-only file with the given
// dimensions using the lavfi testsrc filter. mpeg4 accepts odd dimensions,
// which makes it a good source for reproducing the encoder failure.
func generateOddDimensionSource(t *testing.T, ffmpeg, dir, name string, width, height int) string {
	t.Helper()
	out := filepath.Join(dir, name)
	cmd := exec.Command(ffmpeg,
		"-nostdin",
		"-f", "lavfi", "-i", fmt.Sprintf("testsrc=size=%dx%d:rate=1:duration=1", width, height),
		"-c:v", "mpeg4", "-pix_fmt", "yuv420p",
		"-y", out,
	)
	outBytes, err := cmd.CombinedOutput()
	require.NoError(t, err, "generate test source: %s", strings.TrimSpace(string(outBytes)))
	return out
}

func TestConvertCodec_OddDimensions(t *testing.T) {
	ffmpeg := requireFFmpeg(t)
	requireFFprobe(t)

	tests := []struct {
		name    string
		codec   domain.Codec
		encoder string
		width   int
		height  int
	}{
		{name: "h264 odd width landscape", codec: domain.CodecH264, encoder: "libx264", width: 15, height: 12},
		{name: "h264 odd portrait", codec: domain.CodecH264, encoder: "libx264", width: 13, height: 21},
		{name: "av1 odd portrait", codec: domain.CodecAV1, encoder: "libsvtav1", width: 13, height: 21},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireEncoder(t, ffmpeg, tt.encoder)
			dir := t.TempDir()
			source := generateOddDimensionSource(t, ffmpeg, dir, "source.mp4", tt.width, tt.height)

			c := &Converter{}
			outputPath, err := c.ConvertCodec(source, dir, "odd", tt.codec, 0)
			require.NoError(t, err)
			info, statErr := os.Stat(outputPath)
			require.NoError(t, statErr)
			assert.Greater(t, info.Size(), int64(0))

			probe, probeErr := c.Probe(outputPath)
			require.NoError(t, probeErr)
			gotWidth, gotHeight := probe.Dimensions()
			assert.Zero(t, gotWidth%2, "output width should be even, got %d", gotWidth)
			assert.Zero(t, gotHeight%2, "output height should be even, got %d", gotHeight)
		})
	}
}

func TestRunFFmpeg_IncludesStderr(t *testing.T) {
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not available")
	}
	cmd := exec.Command(shell, "-c", "echo boom >&2; exit 3")
	runErr := runFFmpeg(context.Background(), cmd)
	require.Error(t, runErr)
	assert.Contains(t, runErr.Error(), "boom")
}

func TestRunFFmpeg_Timeout(t *testing.T) {
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, sleep, "30")
	runErr := runFFmpeg(ctx, cmd)
	require.Error(t, runErr)
	assert.Contains(t, runErr.Error(), "timed out")
}
