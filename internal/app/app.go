// Package app wires up the application's dependencies (config, storage, use cases, HTTP server) and
// runs it. It's the composition root, kept separate from cmd/whereabouts/main.go so main can stay a
// thin entry point.
package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"wherewhat"
	"wherewhat/internal/adapter/filesystem"
	httpAPI "wherewhat/internal/adapter/http"
	"wherewhat/internal/config"
	"wherewhat/internal/domain/service/imageprocessor"
	"wherewhat/internal/domain/service/passwordhasher"
	"wherewhat/internal/domain/service/tokengenerator"
	"wherewhat/internal/domain/usecase/auth/authenticate"
	"wherewhat/internal/domain/usecase/auth/login"
	"wherewhat/internal/domain/usecase/auth/logout"
	itemCreate "wherewhat/internal/domain/usecase/item/create"
	itemDelete "wherewhat/internal/domain/usecase/item/delete"
	itemList "wherewhat/internal/domain/usecase/item/list"
	itemUpdate "wherewhat/internal/domain/usecase/item/update"
	locationCreate "wherewhat/internal/domain/usecase/location/create"
	locationDelete "wherewhat/internal/domain/usecase/location/delete"
	locationList "wherewhat/internal/domain/usecase/location/list"
	locationUpdate "wherewhat/internal/domain/usecase/location/update"
	"wherewhat/internal/domain/usecase/user/changepassword"
	"wherewhat/internal/domain/usecase/user/updatelanguage"
	"wherewhat/internal/domain/usecase/user/updatelocationfilterdepth"
	"wherewhat/internal/domain/usecase/user/updateusername"
	itemrepo "wherewhat/internal/repository/item"
	locationrepo "wherewhat/internal/repository/location"
	sessionrepo "wherewhat/internal/repository/session"
	userrepo "wherewhat/internal/repository/user"
)

// Main is the CLI entry point: it parses flags and either creates a user (-create-user) and exits,
// or starts the server — so cmd/whereabouts/main.go can stay a thin wrapper around it.
func Main() error {
	createUser := flag.Bool("create-user", false, "create a new user with -username/-password and exit")
	username := flag.String("username", "", "username for -create-user")
	password := flag.String("password", "", "password for -create-user")
	flag.Parse()

	if *createUser {
		if *username == "" || *password == "" {
			return errors.New("-create-user requires -username and -password")
		}

		return CreateUser(*username, *password)
	}

	return Run()
}

// Run loads configuration, opens storage, wires up the use cases and HTTP server, and blocks
// serving requests until the server stops.
func Run() error {
	time.Local = time.UTC

	config.LoadEnv()

	// Logging is loaded and installed first so anything below has somewhere to report to. Returning
	// these errors rather than log.Fatal'ing them matters once the logger is installed: slog.SetDefault
	// routes the log package through the handler at INFO, so a log.Fatalf below a configured threshold
	// would exit(1) having printed nothing at all.
	logCfg, err := config.LoadLog()
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	closeLog, err := initLogger(logCfg)
	if err != nil {
		return err
	}

	defer closeLog()

	appCfg, err := config.LoadApp()
	if err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

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

	if err := filesystem.EnsureDir(); err != nil {
		return fmt.Errorf("failed to create the photo storage directory: %w", err)
	}

	itemRepo := itemrepo.New(store)
	locationRepo := locationrepo.New(store)
	userRepo := userrepo.New(store)
	sessionRepo := sessionrepo.New(store)
	startSessionCleanup(ctx, sessionRepo, sessionCleanupInterval)

	hasher := passwordhasher.New()
	tokens := tokengenerator.New()
	imgStore := filesystem.NewStorage()
	imgProc := imageprocessor.New()

	if err := seedDemoUser(ctx, userRepo, hasher, appCfg.DemoUsername, appCfg.DemoPassword); err != nil {
		return fmt.Errorf("failed to seed the demo user: %w", err)
	}

	// Printed rather than logged: this and the banner below are the CLI telling its operator it came
	// up, so they shouldn't disappear when someone raises LOG_CONSOLE_LEVEL.
	fmt.Println("database ready")

	srv := &httpAPI.Server{
		Items: httpAPI.ItemUseCases{
			Create: itemCreate.New(itemRepo, locationRepo, imgStore, imgProc),
			Update: itemUpdate.New(itemRepo, locationRepo, imgStore, imgProc),
			Delete: itemDelete.New(itemRepo, imgStore),
			List:   itemList.New(itemRepo),
		},
		Locations: httpAPI.LocationUseCases{
			Create: locationCreate.New(locationRepo),
			Update: locationUpdate.New(locationRepo),
			Delete: locationDelete.New(locationRepo, itemRepo),
			List:   locationList.New(locationRepo),
		},
		Auth: httpAPI.AuthUseCases{
			Login:        login.New(userRepo, sessionRepo, hasher, tokens),
			Logout:       logout.New(sessionRepo),
			Authenticate: authenticate.New(sessionRepo, userRepo),
		},
		Users: httpAPI.UserUseCases{
			UpdateLanguage:            updatelanguage.New(userRepo),
			UpdateLocationFilterDepth: updatelocationfilterdepth.New(userRepo),
			UpdateUsername:            updateusername.New(userRepo, hasher),
			ChangePassword:            changepassword.New(userRepo, hasher),
		},
		CookieSecure: appCfg.CookieSecure,
	}

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	webRoot, err := fs.Sub(wherewhat.WebFiles, "web")
	if err != nil {
		return fmt.Errorf("failed to embed the frontend: %w", err)
	}

	mux.Handle("/", http.FileServer(http.FS(webRoot)))

	handler := httpAPI.WithLogging(httpAPI.WithCORS(mux))

	addr := ":" + appCfg.Port
	httpServer := &http.Server{
		Addr:    addr,
		Handler: handler,
		// ReadHeaderTimeout guards against slow-header attacks (slowloris); ReadTimeout/WriteTimeout stay
		// generous enough to cover a slow connection uploading/downloading a full batch of item photos
		// (bounded by imageprocessor.MaxRequestBytes) without cutting it off.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	fmt.Printf("listening on %s (open http://localhost%s in your browser)\n", addr, addr)

	return httpServer.ListenAndServe()
}
