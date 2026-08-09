package port

//go:generate go tool mockgen -source=image_processor.go -destination=mocks/image_processor_mock.go -package=mocks

// ImageProcessor compresses a freshly-uploaded photo and returns ready-to-store bytes plus the file
// extension to save it under.
type ImageProcessor interface {
	Compress(data []byte, mimeType string) (out []byte, ext string, err error)

	// CompressThumbnail produces a small preview of a freshly-uploaded photo, for list views.
	// Returns an error if the source can't be decoded — callers should treat that as
	// "no thumbnail available" rather than a fatal failure.
	CompressThumbnail(data []byte, mimeType string) (out []byte, ext string, err error)
}
