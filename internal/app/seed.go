package app

import (
	"context"
	"wherewhat/internal/domain/entity"

	"wherewhat/internal/port"
)

// seedDemoUser creates a demo account (default user/pass, configurable via
// DEMO_USERNAME/DEMO_PASSWORD) if the users table is still empty, so there's always something to
// sign in with on a fresh install.
func seedDemoUser(
	ctx context.Context, users port.UserRepository, hasher port.PasswordHasher, username, password string,
) error {
	count, err := users.Count(ctx)
	if err != nil {
		return err
	}

	if count > 0 {
		return nil
	}

	hash, err := hasher.Hash(password)
	if err != nil {
		return err
	}

	_, err = users.Create(ctx, username, hash, entity.UserSettings{})

	return err
}
