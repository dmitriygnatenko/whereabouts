// Package shared holds password-verification logic used by more than one user profile use case
// (updateusername and changepassword) — exported so those sibling packages can call it, now that
// each use case lives in its own package.
package shared

import (
	"context"

	"wherewhat/internal/port"
)

// CheckCurrentPassword re-fetches the user's password hash and verifies it — required before
// letting a profile change alter the username or password, since the session cookie alone shouldn't
// be enough.
func CheckCurrentPassword(
	ctx context.Context, users port.UserRepository, hasher port.PasswordHasher, userID uint64, password string,
) (bool, error) {
	u, err := users.FindByID(ctx, userID)
	if err != nil {
		return false, err
	}

	return hasher.Compare(u.PasswordHash, password), nil
}
