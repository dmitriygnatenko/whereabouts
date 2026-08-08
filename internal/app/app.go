// Package app wires up the application's dependencies (config, storage, use cases, HTTP server) and
// runs it. It's the composition root, kept separate from cmd/whereabouts/main.go so main can stay a
// thin entry point.
package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"wherewhat/internal/adapter/filesystem"
	"wherewhat/internal/config"
)

// Main is the CLI entry point: "create-user" is a subcommand that creates an account and exits,
// without starting the server; anything else starts the server — so cmd/whereabouts/main.go can stay
// a thin wrapper around it.
func Main() error {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "create-user":
			return createUserCommand(os.Args[2:])
		case "-h", "-help", "--help":
			printUsage()

			return nil
		}
	}

	return Run()
}

// printUsage documents the CLI's one subcommand; everything else just starts the server.
func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  whereabouts                                        start the server")
	fmt.Println("  whereabouts create-user -username=U -password=P    create an account and exit")
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
		return fmt.Errorf("invalid log configuration: %w", err)
	}

	closeLog, err := initLogger(logCfg)
	if err != nil {
		return fmt.Errorf("init log error: %w", err)
	}

	defer closeLog()

	appCfg, err := config.LoadApp()
	if err != nil {
		return fmt.Errorf("invalid app configuration: %w", err)
	}

	dbCfg, err := config.LoadDB()
	if err != nil {
		return fmt.Errorf("invalid DB configuration: %w", err)
	}

	// ctx is canceled the moment the process is asked to stop (Ctrl+C, or SIGTERM from e.g. Docker/
	// systemd), which is what lets runServer below drain in-flight requests instead of dropping them —
	// and, incidentally, is also what stops the session-cleanup goroutine started further down.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := openStorage(ctx, dbCfg)
	if err != nil {
		return err
	}

	defer store.Close()

	if err = filesystem.EnsureDir(); err != nil {
		return fmt.Errorf("failed to create the photo storage directory: %w", err)
	}

	repos := newRepositories(store)
	svcs := newServices()

	startSessionCleanup(ctx, repos.Sessions, sessionCleanupInterval)

	if err = seedDemoUser(ctx, seedDemoUserRequest{
		Users:    repos.Users,
		Hasher:   svcs.Hasher,
		Username: appCfg.DemoUsername,
		Password: appCfg.DemoPassword,
	}); err != nil {
		return fmt.Errorf("failed to seed the demo user: %w", err)
	}

	srv := newServer(repos, svcs, appCfg.CookieSecure)

	handler, err := newHandler(srv)
	if err != nil {
		return err
	}

	addr := ":" + appCfg.Port
	httpServer := newHTTPServer(addr, handler)

	slog.InfoContext(ctx, fmt.Sprintf("listening on %s", addr))

	return runServer(ctx, httpServer)
}
