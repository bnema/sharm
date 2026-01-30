package port

type MediaConverter interface {
	Convert(inputPath, outputDir, id string) (outputPath string, codec string, err error)
	Thumbnail(inputPath, outputPath string) error
	Probe(inputPath string) (width int, height int, err error)
}
