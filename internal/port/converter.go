package port

import "github.com/bnema/sharm/internal/domain"

type MediaConverter interface {
	Convert(inputPath, outputDir, id string) (outputPath string, codec string, err error)
	ConvertCodec(inputPath, outputDir, id string, codec domain.Codec, fps int) (outputPath string, err error)
	Thumbnail(inputPath, outputPath string) error
	Probe(inputPath string) (width int, height int, err error)
}
