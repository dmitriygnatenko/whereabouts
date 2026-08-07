package port

import (
	"context"
	"wherewhat/internal/domain/entity"
)

//go:generate go tool mockgen -source=user_repository.go -destination=mocks/user_repository_mock.go -package=mocks

// UserRepository persists user accounts. Create and UpdateUsername return a
// *domainerror.ConflictError when the username is already taken — the adapter is expected to
// inspect the underlying driver error (unique constraint violation) and translate it, so use cases
// never need to know which SQL driver is in play.
// UserCreateRequest bundles the UserRepository.Create parameters that ride along with the context.
type UserCreateRequest struct {
	Username     string
	PasswordHash string
	Settings     entity.UserSettings
}

type UserRepository interface {
	FindByUsername(ctx context.Context, username string) (entity.User, error)
	FindByID(ctx context.Context, id uint64) (entity.User, error)
	Create(ctx context.Context, req UserCreateRequest) (id uint64, err error)
	UpdateUsername(ctx context.Context, id uint64, username string) error
	UpdatePasswordHash(ctx context.Context, id uint64, hash string) error
	UpdateLanguage(ctx context.Context, id uint64, lang string) error
	UpdateLocationFilterDepth(ctx context.Context, id uint64, depth int) error
	Count(ctx context.Context) (int, error)
}
