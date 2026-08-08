package app

import (
	"context"
	"errors"
	"flag"
	"fmt"

	"wherewhat/internal/config"
	domainerror "wherewhat/internal/domain/error"
	"wherewhat/internal/domain/service/passwordhasher"
	"wherewhat/internal/domain/usecase/user/create"
	userrepo "wherewhat/internal/repository/user"
)

// createUserCommand parses create-user's own flags and creates a single account with them. It's a
// separate flag.FlagSet, not the top-level flag.CommandLine, so its -username/-password don't leak
// into (or get confused with) whatever flags the server itself grows in the future.
func createUserCommand(args []string) error {
	flags := flag.NewFlagSet("create-user", flag.ExitOnError)
	username := flags.String("username", "", "username for the new account")
	password := flags.String("password", "", "password for the new account")

	if err := flags.Parse(args); err != nil {
		return err
	}

	if *username == "" || *password == "" {
		return errors.New("create-user requires -username and -password")
	}

	return createUser(*username, *password)
}

// createUser opens storage using the normal environment-based configuration and creates a single
// account with the given credentials, then closes the connection. It's the entry point for
// `-create-user`.
func createUser(username, password string) error {
	config.LoadEnv()

	// Only logging and storage are loaded here: this command never serves HTTP, so with the groups
	// split it no longer has to satisfy PORT or the other server settings to create an account.
	logCfg, err := config.LoadLog()
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	closeLog, err := initLogger(logCfg)
	if err != nil {
		return err
	}

	defer closeLog()

	dbCfg, err := config.LoadDB()
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	ctx := context.Background()

	store, err := openStorage(ctx, dbCfg)
	if err != nil {
		return err
	}

	defer store.Close()

	userRepo := userrepo.New(store)
	uc := create.New(userRepo, passwordhasher.New())

	user, err := uc.Execute(ctx, create.Input{
		Username: username,
		Password: password,
	})
	if err != nil {
		var conflict *domainerror.ConflictError
		if errors.As(err, &conflict) {
			return fmt.Errorf("user %q already exists", username)
		}

		return err
	}

	fmt.Printf("created user %q (id=%d)\n", user.User.Username, user.User.ID)

	return nil
}
