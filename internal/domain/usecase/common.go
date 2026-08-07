package usecase

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"

	"wherewhat/internal/domain/entity"
	"wherewhat/internal/port"
)

func NormalizeUsername(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

// UsernameRules is the ozzo-validation rule set for a username field: required, and at least
// entity.MinUsernameLength characters. The two checks stay separate, each with its own message, so
// an empty username reports "please enter one" rather than "too short".
func UsernameRules() []validation.Rule {
	return []validation.Rule{
		validation.Required.Error("Please enter a username"),
		validation.Length(entity.MinUsernameLength, 0).
			Error(fmt.Sprintf("Username must be at least %d characters", entity.MinUsernameLength)),
	}
}

// PasswordRules is the ozzo-validation rule set for a password field: required, and at least
// entity.MinPasswordLength characters. Required is chained in front of Length with the same
// message: Length alone treats an empty value as valid, which would let a blank password slip
// through silently.
func PasswordRules() []validation.Rule {
	return []validation.Rule{
		validation.Required.Error("Please enter a password"),
		validation.Length(entity.MinPasswordLength, 0).
			Error(fmt.Sprintf("Password must be at least %d characters", entity.MinPasswordLength)),
	}
}

// ItemNameRules is the ozzo-validation rule set for an item name field: required.
func ItemNameRules() []validation.Rule {
	return []validation.Rule{
		validation.Required.Error("Enter the item name"),
	}
}

// LocationIDRules is the ozzo-validation rule set for a field referencing the location an item
// belongs to: it must be a positive id.
func LocationIDRules() []validation.Rule {
	return []validation.Rule{
		validation.Min(uint64(1)).Error("Choose a location"),
	}
}

// LocationNameRules is the ozzo-validation rule set for a location name field: required.
func LocationNameRules() []validation.Rule {
	return []validation.Rule{
		validation.Required.Error("Please enter a location name"),
	}
}

// DefaultColor is used whenever the caller doesn't supply a location color.
const DefaultColor = "#3D6B63"

// ResolveColor trims the given color, falling back to DefaultColor when blank.
func ResolveColor(color string) string {
	color = strings.TrimSpace(color)
	if color == "" {
		return DefaultColor
	}

	return color
}

// supportedLanguageValues is entity.SupportedLanguages' key set, in the []any shape validation.In
// wants — built once at package init so the map stays the single source of truth for what's
// supported.
var supportedLanguageValues = func() []any {
	vals := make([]any, 0, len(entity.SupportedLanguages))
	for lang := range entity.SupportedLanguages {
		vals = append(vals, lang)
	}
	return vals
}()

// LanguageRules is the ozzo-validation rule set for an interface-language field: required, and one
// of entity.SupportedLanguages. Required is chained in front of In with the same message: In alone
// treats an empty value as valid (see its doc comment), which would let a blank language slip
// through instead of being rejected like any other unsupported value.
func LanguageRules() []validation.Rule {
	const msg = "Unsupported language"

	return []validation.Rule{
		validation.Required.Error(msg),
		validation.In(supportedLanguageValues...).Error(msg),
	}
}

// LocationFilterDepthRules is the ozzo-validation rule set for a location-filter-depth field: 0
// itself is the zero value, so ozzo treats it as "empty" and skips the check — which is fine, 0
// ("no limit") is valid anyway. Any negative depth is not empty and gets rejected.
func LocationFilterDepthRules() []validation.Rule {
	return []validation.Rule{
		validation.Min(0).Error("Location filter depth must be 0 (show all) or at least 1"),
	}
}

// CheckCurrentPasswordRequest bundles the CheckCurrentPassword parameters that ride along with the
// context.
type CheckCurrentPasswordRequest struct {
	Users    port.UserRepository
	Hasher   port.PasswordHasher
	UserID   uint64
	Password string
}

// CheckCurrentPassword re-fetches the user's password hash and verifies it — required before
// letting a profile change alter the username or password, since the session cookie alone shouldn't
// be enough.
func CheckCurrentPassword(ctx context.Context, req CheckCurrentPasswordRequest) (bool, error) {
	u, err := req.Users.FindByID(ctx, req.UserID)
	if err != nil {
		return false, err
	}

	return req.Hasher.Compare(u.PasswordHash, req.Password), nil
}

// ProcessImages runs each photo through compression + storage, unless it's already a URL to a
// previously-stored file (unchanged since the item was last saved).
func ProcessImages(
	store port.ImageStorage,
	proc port.ImageProcessor,
	images []string,
) ([]string, error) {
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
