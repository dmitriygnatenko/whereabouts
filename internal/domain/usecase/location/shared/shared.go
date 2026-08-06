// Package shared holds validation logic used by more than one location use case (createlocation and
// updatelocation) — exported so those sibling packages can call it, now that each use case lives in
// its own package.
package shared

import (
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"

	domainerror "wherewhat/internal/domain/error"
)

// DefaultColor is used whenever the caller doesn't supply one.
const DefaultColor = "#3D6B63"

// ValidateName checks a (already-trimmed) location name is present.
func ValidateName(name string) *domainerror.ValidationError {
	if err := validation.Validate(name, validation.Required.Error("Please enter a location name")); err != nil {
		return &domainerror.ValidationError{Message: err.Error()}
	}

	return nil
}

// ResolveColor trims the given color, falling back to DefaultColor when blank.
func ResolveColor(color string) string {
	color = strings.TrimSpace(color)
	if color == "" {
		return DefaultColor
	}

	return color
}
