package port

//go:generate go tool mockgen -source=image_storage.go -destination=mocks/image_storage_mock.go -package=mocks

// ImageStorage saves/deletes photo files and recognizes URLs it has already issued (as opposed to a
// fresh data: URL still awaiting processing).
type ImageStorage interface {
	Save(data []byte, ext string) (url string, err error)
	Delete(url string)
	IsStoredURL(url string) bool
}
