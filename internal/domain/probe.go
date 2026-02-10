package domain

import (
	"fmt"
	"math"
	"strconv"
)

type ProbeFormat struct {
	FormatName string            `json:"format_name"`
	FormatLong string            `json:"format_long_name"`
	Duration   string            `json:"duration"`
	Size       string            `json:"size"`
	BitRate    string            `json:"bit_rate"`
	NbStreams  int               `json:"nb_streams"`
	Tags       map[string]string `json:"tags"`
}

type ProbeStream struct {
	Index         int               `json:"index"`
	CodecType     string            `json:"codec_type"`
	CodecName     string            `json:"codec_name"`
	CodecLong     string            `json:"codec_long_name"`
	Width         int               `json:"width"`
	Height        int               `json:"height"`
	PixFmt        string            `json:"pix_fmt"`
	ColorSpace    string            `json:"color_space"`
	ColorRange    string            `json:"color_range"`
	RFrameRate    string            `json:"r_frame_rate"`
	AvgFrameRate  string            `json:"avg_frame_rate"`
	Duration      string            `json:"duration"`
	BitRate       string            `json:"bit_rate"`
	SampleRate    string            `json:"sample_rate"`
	Channels      int               `json:"channels"`
	ChannelLayout string            `json:"channel_layout"`
	BitsPerSample int               `json:"bits_per_sample"`
	Tags          map[string]string `json:"tags"`
}

type ProbeResult struct {
	Format  ProbeFormat   `json:"format"`
	Streams []ProbeStream `json:"streams"`
	RawJSON string        `json:"-"`
}

const (
	oneKilobyte      = 1024
	oneMegabyte      = oneKilobyte * 1024
	oneGigabyte      = oneMegabyte * 1024
	oneMegabitPerSec = 1000000
	oneKilobitPerSec = 1000
)

func (p *ProbeResult) VideoStream() *ProbeStream {
	for i := range p.Streams {
		if p.Streams[i].CodecType == "video" {
			return &p.Streams[i]
		}
	}
	return nil
}

func (p *ProbeResult) AudioStream() *ProbeStream {
	for i := range p.Streams {
		if p.Streams[i].CodecType == "audio" {
			return &p.Streams[i]
		}
	}
	return nil
}

func (p *ProbeResult) Dimensions() (width, height int) {
	vs := p.VideoStream()
	if vs != nil {
		return vs.Width, vs.Height
	}
	return 0, 0
}

func ParseFrameRate(fraction string) float64 {
	if fraction == "" || fraction == "0/0" {
		return 0
	}
	var num, den int
	if _, err := fmt.Sscanf(fraction, "%d/%d", &num, &den); err == nil && den > 0 {
		return float64(num) / float64(den)
	}
	return 0
}

func FormatDuration(seconds float64) string {
	if seconds <= 0 {
		return "00:00"
	}
	hours := int(seconds) / 3600
	minutes := (int(seconds) % 3600) / 60
	secs := int(seconds) % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, secs)
	}
	return fmt.Sprintf("%d:%02d", minutes, secs)
}

func FormatBitrate(bitrateStr string) string {
	if bitrateStr == "" {
		return ""
	}
	bitrate, err := strconv.ParseFloat(bitrateStr, 64)
	if err != nil {
		return bitrateStr
	}
	if bitrate >= oneMegabitPerSec {
		return fmt.Sprintf("%.1f Mbps", bitrate/oneMegabitPerSec)
	}
	if bitrate >= oneKilobitPerSec {
		return fmt.Sprintf("%.1f Kbps", bitrate/oneKilobitPerSec)
	}
	return fmt.Sprintf("%.0f bps", bitrate)
}

func FormatFrameRate(fraction string) string {
	fps := ParseFrameRate(fraction)
	if fps == 0 {
		return ""
	}
	if fps == math.Floor(fps) {
		return fmt.Sprintf("%.0f FPS", fps)
	}
	return fmt.Sprintf("%.2f FPS", fps)
}

func FormatSampleRate(sampleRateStr string) string {
	if sampleRateStr == "" {
		return ""
	}
	sampleRate, err := strconv.ParseFloat(sampleRateStr, 64)
	if err != nil {
		return sampleRateStr
	}
	return fmt.Sprintf("%.0f Hz", sampleRate)
}

func ParseSize(sizeStr string) int64 {
	if sizeStr == "" {
		return 0
	}
	var size int64
	if _, err := fmt.Sscanf(sizeStr, "%d", &size); err == nil {
		return size
	}
	return 0
}

func ParseDuration(durationStr string) float64 {
	if durationStr == "" || durationStr == "N/A" {
		return 0
	}
	duration, err := strconv.ParseFloat(durationStr, 64)
	if err != nil {
		return 0
	}
	return duration
}

func FormatSize(bytes int64) string {
	if bytes < oneKilobyte {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < oneMegabyte {
		return fmt.Sprintf("%.1f KB", float64(bytes)/oneKilobyte)
	}
	if bytes < oneGigabyte {
		return fmt.Sprintf("%.1f MB", float64(bytes)/oneMegabyte)
	}
	return fmt.Sprintf("%.1f GB", float64(bytes)/oneGigabyte)
}
