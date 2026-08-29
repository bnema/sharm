package ffmpeg

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/port"
)

// Path validation errors
var (
	ErrEmptyPath        = errors.New("empty path")
	ErrInvalidPath      = errors.New("invalid path: contains null bytes")
	ErrProbeOutputLimit = errors.New("ffprobe output exceeds limit")
)

const (
	defaultProbeTimeout = 30 * time.Second
	maxProbeOutputBytes = 4 * 1024 * 1024
)

// validatePath checks for empty paths and null byte injection attacks
func validatePath(path string) error {
	if path == "" {
		return ErrEmptyPath
	}
	if strings.ContainsRune(path, 0) {
		return ErrInvalidPath
	}
	return nil
}

const convertTimeout = 30 * time.Minute

type Converter struct{}

func NewConverter() *Converter {
	return &Converter{}
}

func (c *Converter) Convert(inputPath, outputDir, id string) (outputPath, codec string, err error) {
	if validateErr := validatePath(inputPath); validateErr != nil {
		return "", "", fmt.Errorf("invalid input path: %w", validateErr)
	}
	if validateErr := validatePath(outputDir); validateErr != nil {
		return "", "", fmt.Errorf("invalid output dir: %w", validateErr)
	}
	basePath := filepath.Join(outputDir, id)

	webmPath := basePath + ".webm"
	mp4Path := basePath + ".mp4"

	err = c.convertAV1(inputPath, webmPath, 0)
	if err != nil {
		err = c.convertH264(inputPath, mp4Path, 0)
		if err != nil {
			return "", "", fmt.Errorf("both AV1 and H264 conversion failed: %w", err)
		}
		return mp4Path, string(domain.CodecH264), nil
	}

	return webmPath, string(domain.CodecAV1), nil
}

func (c *Converter) ConvertCodec(inputPath, outputDir, id string, codec domain.Codec, fps int) (outputPath string, err error) {
	if validateErr := validatePath(inputPath); validateErr != nil {
		return "", fmt.Errorf("invalid input path: %w", validateErr)
	}
	if validateErr := validatePath(outputDir); validateErr != nil {
		return "", fmt.Errorf("invalid output dir: %w", validateErr)
	}
	basePath := filepath.Join(outputDir, id)

	switch codec {
	case domain.CodecAV1:
		outputPath = basePath + "_av1.webm"
		err = c.convertAV1(inputPath, outputPath, fps)
	case domain.CodecH264:
		outputPath = basePath + "_h264.mp4"
		err = c.convertH264(inputPath, outputPath, fps)
	case domain.CodecOpus:
		outputPath = basePath + "_opus.ogg"
		err = c.convertOpus(inputPath, outputPath)
	default:
		return "", fmt.Errorf("unsupported codec: %s", codec)
	}

	if err != nil {
		return "", fmt.Errorf("convert to %s: %w", codec, err)
	}
	return outputPath, nil
}

func (c *Converter) convertAV1(inputPath, outputPath string, fps int) error {
	if validateErr := validatePath(inputPath); validateErr != nil {
		return fmt.Errorf("invalid input path: %w", validateErr)
	}
	if validateErr := validatePath(outputPath); validateErr != nil {
		return fmt.Errorf("invalid output path: %w", validateErr)
	}
	args := []string{
		"-nostdin", // Security: prevent stdin-based attacks
		"-i", inputPath,
		"-c:v", "libsvtav1",
		"-crf", "30",
		"-preset", "6",
		"-c:a", "libopus",
		"-b:a", "128k",
	}
	if fps > 0 {
		args = append(args, "-r", fmt.Sprintf("%d", fps))
	}
	args = append(args, "-y", outputPath)
	ctx, cancel := context.WithTimeout(context.Background(), convertTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	return cmd.Run()
}

func (c *Converter) convertH264(inputPath, outputPath string, fps int) error {
	if err := validatePath(inputPath); err != nil {
		return fmt.Errorf("invalid input path: %w", err)
	}
	if err := validatePath(outputPath); err != nil {
		return fmt.Errorf("invalid output path: %w", err)
	}
	args := []string{
		"-nostdin", // Security: prevent stdin-based attacks
		"-i", inputPath,
		"-c:v", "libx264",
		"-crf", "23",
		"-preset", "medium",
		"-c:a", "aac",
		"-b:a", "128k",
		"-movflags", "+faststart",
	}
	if fps > 0 {
		args = append(args, "-r", fmt.Sprintf("%d", fps))
	}
	args = append(args, "-y", outputPath)
	ctx, cancel := context.WithTimeout(context.Background(), convertTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	return cmd.Run()
}

func (c *Converter) convertOpus(inputPath, outputPath string) error {
	if err := validatePath(inputPath); err != nil {
		return fmt.Errorf("invalid input path: %w", err)
	}
	if err := validatePath(outputPath); err != nil {
		return fmt.Errorf("invalid output path: %w", err)
	}
	args := []string{
		"-nostdin", // Security: prevent stdin-based attacks
		"-i", inputPath,
		"-c:a", "libopus",
		"-b:a", "128k",
		"-vn",
		"-y",
		outputPath,
	}
	ctx, cancel := context.WithTimeout(context.Background(), convertTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	return cmd.Run()
}

func (c *Converter) Thumbnail(inputPath, outputPath string) error {
	if err := validatePath(inputPath); err != nil {
		return fmt.Errorf("invalid input path: %w", err)
	}
	if err := validatePath(outputPath); err != nil {
		return fmt.Errorf("invalid output path: %w", err)
	}
	args := []string{
		"-nostdin", // Security: prevent stdin-based attacks
		"-i", inputPath,
		"-vframes", "1",
		"-ss", "00:00:01",
		"-f", "image2",
		"-y",
		outputPath,
	}
	cmd := exec.Command("ffmpeg", args...)
	return cmd.Run()
}

func (c *Converter) Probe(inputPath string) (*domain.ProbeResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), defaultProbeTimeout)
	defer cancel()
	return c.ProbeContext(ctx, inputPath)
}

func (*Converter) ProbeContext(ctx context.Context, inputPath string) (*domain.ProbeResult, error) {
	if err := validatePath(inputPath); err != nil {
		return nil, fmt.Errorf("invalid input path: %w", err)
	}
	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		inputPath,
	}
	probeCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	//nolint:gosec // Executable is fixed; the input path is validated.
	cmd := exec.CommandContext(probeCtx, "ffprobe", args...)
	output := &cappedBuffer{max: maxProbeOutputBytes, onLimit: cancel}
	cmd.Stdout = output
	if err := cmd.Run(); err != nil {
		if output.limitExceeded.Load() || errors.Is(err, ErrProbeOutputLimit) {
			return nil, ErrProbeOutputLimit
		}
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	rawJSON := output.String()
	var result domain.ProbeResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}
	result.RawJSON = rawJSON
	return &result, nil
}

type cappedBuffer struct {
	buffer        bytes.Buffer
	max           int
	onLimit       func()
	limitExceeded atomic.Bool
}

func (b *cappedBuffer) Write(data []byte) (int, error) {
	if len(data) > b.max-b.buffer.Len() {
		b.limitExceeded.Store(true)
		if b.onLimit != nil {
			b.onLimit()
		}
		return 0, ErrProbeOutputLimit
	}
	return b.buffer.Write(data)
}

func (b *cappedBuffer) ReadFrom(reader io.Reader) (int64, error) {
	return io.Copy(writerOnly{Writer: b}, reader)
}

func (b *cappedBuffer) Bytes() []byte {
	return b.buffer.Bytes()
}

func (b *cappedBuffer) String() string {
	return b.buffer.String()
}

type writerOnly struct {
	io.Writer
}

var _ port.MediaConverter = (*Converter)(nil)
