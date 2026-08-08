package app

import (
	"wherewhat/internal/adapter/filesystem"
	"wherewhat/internal/domain/service/imageprocessor"
	"wherewhat/internal/domain/service/passwordhasher"
	"wherewhat/internal/domain/service/tokengenerator"
)

// services bundles the domain services that are shared across multiple use cases (e.g. hasher is
// needed by login, demo-user seeding, and the user use cases alike), so each is constructed once
// here rather than once per use case.
type services struct {
	Hasher   *passwordhasher.BcryptHasher
	Tokens   *tokengenerator.RandomTokenGenerator
	ImgStore *filesystem.Storage
	ImgProc  *imageprocessor.Compressor
}

// newServices constructs the shared domain services. They're all stateless/self-contained, so this
// never fails.
func newServices() services {
	return services{
		Hasher:   passwordhasher.New(),
		Tokens:   tokengenerator.New(),
		ImgStore: filesystem.NewStorage(),
		ImgProc:  imageprocessor.New(),
	}
}
