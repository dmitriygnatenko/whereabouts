package app

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"wherewhat"
	httpAPI "wherewhat/internal/adapter/http"
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
)

// newServer wires every use case the HTTP API depends on, grouped the same way httpAPI.Server groups
// its routes (items, locations, auth, users). Keeping this assembly in one function makes it the
// single place that shows, for any given use case, exactly which repositories and services feed it.
func newServer(repos repositories, svcs services, cookieSecure bool) *httpAPI.Server {
	return &httpAPI.Server{
		Items: httpAPI.ItemUseCases{
			Create: itemCreate.New(repos.Items, repos.Locations, svcs.ImgStore, svcs.ImgProc),
			Update: itemUpdate.New(repos.Items, repos.Locations, svcs.ImgStore, svcs.ImgProc),
			Delete: itemDelete.New(repos.Items, svcs.ImgStore),
			List:   itemList.New(repos.Items),
		},
		Locations: httpAPI.LocationUseCases{
			Create: locationCreate.New(repos.Locations),
			Update: locationUpdate.New(repos.Locations),
			Delete: locationDelete.New(repos.Locations, repos.Items),
			List:   locationList.New(repos.Locations),
		},
		Auth: httpAPI.AuthUseCases{
			Login:        login.New(repos.Users, repos.Sessions, svcs.Hasher, svcs.Tokens),
			Logout:       logout.New(repos.Sessions),
			Authenticate: authenticate.New(repos.Sessions, repos.Users),
		},
		Users: httpAPI.UserUseCases{
			UpdateLanguage:            updatelanguage.New(repos.Users),
			UpdateLocationFilterDepth: updatelocationfilterdepth.New(repos.Users),
			UpdateUsername:            updateusername.New(repos.Users, svcs.Hasher),
			ChangePassword:            changepassword.New(repos.Users, svcs.Hasher),
		},
		CookieSecure: cookieSecure,
	}
}

// newHandler registers the API routes and the embedded frontend on one mux, then wraps it with the
// CORS and logging middleware that every request — API or static file — should go through.
func newHandler(srv *httpAPI.Server) (http.Handler, error) {
	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

	webRoot, err := fs.Sub(wherewhat.WebFiles, "web")
	if err != nil {
		return nil, fmt.Errorf("failed to embed the frontend: %w", err)
	}

	mux.Handle("/", http.FileServer(http.FS(webRoot)))

	return httpAPI.WithLogging(httpAPI.WithCORS(mux)), nil
}

// newHTTPServer builds the server with timeouts tuned for this app rather than net/http's defaults.
func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
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
}

// shutdownTimeout bounds how long graceful shutdown waits for in-flight requests to finish before
// giving up and forcibly closing whatever connections are still open.
const shutdownTimeout = 10 * time.Second

// runServer serves on httpServer until ctx is canceled, then drains in-flight requests via Shutdown
// instead of cutting them off — so a deploy or restart doesn't truncate a response mid-upload. A
// fresh, un-canceled context is used for Shutdown itself since ctx just fired as the reason to stop.
func runServer(ctx context.Context, httpServer *http.Server) error {
	serveErr := make(chan error, 1)

	go func() {
		serveErr <- httpServer.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("server stopped unexpectedly: %w", err)

	case <-ctx.Done():
		slog.Info("shutting down")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("failed to shut down cleanly: %w", err)
		}

		return nil
	}
}
