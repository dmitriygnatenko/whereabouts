// Package shared holds validation and photo-processing logic used by more than one item use case
// (createitem and updateitem) — exported so those sibling packages can call it, now that each use
// case lives in its own package instead of sharing one.
package shared

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"

	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/port"
)

// inputFields is a validation-only view of the item fields that need form-shaped (per-field)
// feedback. ozzo-validation names fields in its error map from the "json" struct tag, so the tags
// here exist purely to make it report "name"/"locationId", matching the API's existing field keys —
// they don't leak into any public Input type. The names also decide the order ValidationError.Error
// renders several violations in, since it sorts by field name.
type inputFields struct {
	Name       string `json:"name"`
	LocationID uint64 `json:"locationId"`
}

// ValidateInput mirrors the original handler-level validation: name must be non-blank, locationID
// must be a positive id. Returns nil when input is valid.
func ValidateInput(name string, locationID uint64) *domainerror.ValidationError {
	subject := inputFields{Name: strings.TrimSpace(name), LocationID: locationID}

	err := validation.ValidateStruct(&subject,
		validation.Field(&subject.Name, validation.Required.Error("Enter the item name")),
		validation.Field(&subject.LocationID, validation.Min(uint64(1)).Error("Choose a location")),
	)
	if err == nil {
		return nil
	}

	return domainerror.ToValidationError(err)
}

// ProcessImages runs each photo through compression + storage, unless it's already a URL to a
// previously-stored file (unchanged since the item was last saved).
func ProcessImages(store port.ImageStorage, proc port.ImageProcessor, images []string) ([]string, error) {
	processed := make([]string, len(images))

	for i, img := range images {
		if store.IsStoredURL(img) {
			processed[i] = img
			continue
		}

		mimeType, raw, err := parseDataURL(img)
		if err != nil {
			return nil, fmt.Errorf("photo #%d: %w", i+1, err)
		}

		data, ext, err := proc.Compress(raw, mimeType)
		if err != nil {
			return nil, fmt.Errorf("photo #%d: failed to process (%w)", i+1, err)
		}

		savedURL, err := store.Save(data, ext)
		if err != nil {
			return nil, fmt.Errorf("photo #%d: failed to save (%w)", i+1, err)
		}

		processed[i] = savedURL
	}

	return processed, nil
}

// parseDataURL splits "data:<mime>;base64,<data>" — the format a freshly uploaded photo arrives in
// from the frontend — into a MIME type and raw bytes.
func parseDataURL(s string) (mimeType string, data []byte, err error) {
	if !strings.HasPrefix(s, "data:") {
		return "", nil, fmt.Errorf("does not look like a data URL")
	}

	comma := strings.IndexByte(s, ',')
	if comma == -1 {
		return "", nil, fmt.Errorf("invalid data URL: missing comma separator")
	}

	header := s[len("data:"):comma]
	body := s[comma+1:]

	isBase64 := strings.HasSuffix(header, ";base64")

	mimeType = strings.TrimSuffix(header, ";base64")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	if !isBase64 {
		decodedStr, uerr := url.QueryUnescape(body)
		if uerr != nil {
			return "", nil, fmt.Errorf("invalid data URL encoding: %w", uerr)
		}

		return mimeType, []byte(decodedStr), nil
	}

	raw, derr := base64.StdEncoding.DecodeString(body)
	if derr != nil {
		return "", nil, fmt.Errorf("invalid base64 in data URL: %w", derr)
	}

	return mimeType, raw, nil
}
