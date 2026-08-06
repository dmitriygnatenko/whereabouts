package filesystem

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Dir is the on-disk directory photo files are saved under.
const Dir = "web/files"

// URLPrefix is the path photo files are served back out at (see Server.RegisterRoutes, which mounts
// Dir under this prefix).
const URLPrefix = "/files/"

// EnsureDir creates the photo storage directory if it doesn't exist yet.
func EnsureDir() error {
	return os.MkdirAll(Dir, 0o755)
}

// Storage implements port.ImageStorage against the local filesystem.
type Storage struct{}

// NewStorage builds a Storage.
func NewStorage() *Storage {
	return &Storage{}
}

// Save writes image bytes under a random filename and returns the URL it will be served back out at.
func (s *Storage) Save(data []byte, ext string) (string, error) {
	if err := EnsureDir(); err != nil {
		return "", fmt.Errorf("failed to create files directory: %w", err)
	}

	name, err := randomFileName(ext)
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(filepath.Join(Dir, name), data, 0o600); err != nil {
		return "", fmt.Errorf("failed to write image file: %w", err)
	}

	return URLPrefix + name, nil
}

// Delete removes the file a URL points to, if it's one of ours. A missing file isn't an error — it
// may have already been removed.
func (s *Storage) Delete(url string) {
	if !s.IsStoredURL(url) {
		return
	}

	name := strings.TrimPrefix(url, URLPrefix)
	// Guard against path traversal — the file name must not contain path separators.
	if name == "" || strings.ContainsAny(name, `/\`) {
		return
	}

	_ = os.Remove(filepath.Join(Dir, name))
}

// IsStoredURL reports whether a string already points at a file this store previously saved, as
// opposed to a fresh data: URL still awaiting processing.
func (s *Storage) IsStoredURL(url string) bool {
	return strings.HasPrefix(url, URLPrefix)
}

// randomFileName generates a random 16-byte hex filename with ext, so saved photos never collide
// and never leak anything about their upload order.
func randomFileName(ext string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate file name: %w", err)
	}

	return hex.EncodeToString(b) + ext, nil
}
