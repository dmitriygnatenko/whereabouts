package port

//go:generate go tool mockgen -source=image_processor.go -destination=mocks/image_processor_mock.go -package=mocks

// ImageProcessor compresses a freshly-uploaded photo and returns ready-to-store bytes plus the file
// extension to save it under.
type ImageProcessor interface {
	Compress(data []byte, mimeType string) (out []byte, ext string, err error)
}
