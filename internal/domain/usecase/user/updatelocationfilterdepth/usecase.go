// Package updatelocationfilterdepth is the UpdateLocationFilterDepth use case: it saves how deep
// the location filter chips on the "Items" tab should go.
package updatelocationfilterdepth

import (
	"context"
	"errors"
	"wherewhat/internal/domain/entity"

	validation "github.com/go-ozzo/ozzo-validation/v4"

	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/port"
)

// UseCase implements UpdateLocationFilterDepth.
type UseCase struct {
	Users port.UserRepository
}

// New builds a UseCase from its dependencies.
func New(users port.UserRepository) *UseCase {
	return &UseCase{Users: users}
}

// Execute validates and saves the signed-in user's location-filter-depth preference.
func (uc *UseCase) Execute(ctx context.Context, in Input) (Output, error) {
	// Min(0): 0 itself is the zero value, so ozzo treats it as "empty" and skips the check — which is
	// fine, 0 ("no limit") is valid anyway. Any negative depth is not empty and gets rejected.
	if err := validation.Validate(in.Depth,
		validation.Min(0).Error("Location filter depth must be 0 (show all) or at least 1"),
	); err != nil {
		return entity.PublicUser{}, &domainerror.ValidationError{Message: err.Error()}
	}

	if err := uc.Users.UpdateLocationFilterDepth(ctx, in.User.ID, in.Depth); err != nil {
		return entity.PublicUser{}, errors.New("Failed to save location filter depth")
	}

	updated := in.User
	updated.LocationFilterDepth = in.Depth

	return updated, nil
}
