// Package updatelocationfilterdepth is the UpdateLocationFilterDepth use case: it saves how deep
// the location filter chips on the "Items" tab should go.
package updatelocationfilterdepth

import (
	"context"
	"errors"
	"log/slog"

	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/port"
)

// UseCase implements UpdateLocationFilterDepth.
type UseCase struct {
	userRepository port.UserRepository
}

// New builds a UseCase from its dependencies.
func New(
	userRepository port.UserRepository,
) *UseCase {
	return &UseCase{
		userRepository: userRepository,
	}
}

// Execute validates and saves the signed-in user's location-filter-depth preference.
func (uc *UseCase) Execute(
	ctx context.Context,
	input Input,
) (Output, error) {
	if err := input.Validate(); err != nil {
		slog.InfoContext(ctx, "update location filter depth: validation", "error", err)

		return Output{}, domainerror.ToValidationError(err)
	}

	if err := uc.userRepository.UpdateLocationFilterDepth(
		ctx,
		input.User.ID,
		input.Depth,
	); err != nil {
		slog.ErrorContext(
			ctx,
			"update location filter depth: save",
			"user_id", input.User.ID, "error", err,
		)

		return Output{}, errors.New("Failed to save location filter depth")
	}

	updated := input.User
	updated.LocationFilterDepth = input.Depth

	return Output{
		User: updated,
	}, nil
}
