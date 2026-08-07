// Package create is the CreateLocation use case.
package create

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	domainError "wherewhat/internal/domain/error"
	"wherewhat/internal/domain/usecase"
	"wherewhat/internal/port"
)

// UseCase implements CreateLocation.
type UseCase struct {
	locationRepository port.LocationRepository
}

// New builds a UseCase from its dependencies.
func New(
	locationRepository port.LocationRepository,
) *UseCase {
	return &UseCase{
		locationRepository: locationRepository,
	}
}

// Execute validates and creates a new location, verifying its parent (if any) exists first.
func (uc *UseCase) Execute(
	ctx context.Context,
	input Input,
) (Output, error) {
	if err := input.Validate(); err != nil {
		slog.InfoContext(ctx, "create location: validation", "error", err)

		return Output{}, domainError.ToValidationError(err)
	}

	name := strings.TrimSpace(input.Name)
	color := usecase.ResolveColor(input.Color)

	parentID := input.ParentID
	if parentID != nil && *parentID > 0 {
		exists, err := uc.locationRepository.Exists(ctx, *parentID)
		if err != nil {
			slog.ErrorContext(ctx, "create location: verify parent", "error", err)

			return Output{}, errors.New("Failed to verify parent location")
		}

		if !exists {
			slog.InfoContext(ctx, "create location: parent not found", "parent_id", *parentID)

			return Output{}, &domainError.ValidationError{Message: "Parent location not found"}
		}
	} else {
		parentID = nil
	}

	now := time.Now().UTC()

	loc, err := uc.locationRepository.Create(ctx, port.LocationCreateRequest{
		Name:     name,
		Color:    color,
		ParentID: parentID,
		Now:      now,
	})
	if err != nil {
		slog.ErrorContext(ctx, "create location: save", "error", err)

		return Output{}, errors.New("Failed to save location")
	}

	return Output{Location: loc}, nil
}
