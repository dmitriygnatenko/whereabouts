package app

import (
	itemRepo "wherewhat/internal/repository/item"
	locationRepo "wherewhat/internal/repository/location"
	sessionRepo "wherewhat/internal/repository/session"
	userRepo "wherewhat/internal/repository/user"
)

// repositories bundles the four table-group repositories built on top of storage. It exists so
// session cleanup, demo-user seeding and use-case wiring can each take just the repositories they
// need without Run growing a long, error-prone parameter list of its own.
type repositories struct {
	Items     *itemRepo.Repository
	Locations *locationRepo.Repository
	Users     *userRepo.Repository
	Sessions  *sessionRepo.Repository
}

// newRepositories builds every repository on top of the same storage connection.
func newRepositories(store storage) repositories {
	return repositories{
		Items:     itemRepo.New(store),
		Locations: locationRepo.New(store),
		Users:     userRepo.New(store),
		Sessions:  sessionRepo.New(store),
	}
}
