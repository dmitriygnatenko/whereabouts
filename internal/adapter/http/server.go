// Package http is the driving adapter: it exposes the application's use cases over HTTP,
// translating requests into use case Input values and use case Output/errors back into JSON
// responses. It owns nothing that looks like business logic — validation, persistence and side
// effects all live behind the use cases it calls.
package http

import (
	"net/http"

	"wherewhat/internal/adapter/filesystem"
	"wherewhat/internal/domain/usecase/auth/authenticatesession"
	"wherewhat/internal/domain/usecase/auth/login"
	"wherewhat/internal/domain/usecase/auth/logoutuser"
	"wherewhat/internal/domain/usecase/auth/registeruser"
	"wherewhat/internal/domain/usecase/item/createitem"
	"wherewhat/internal/domain/usecase/item/deleteitem"
	"wherewhat/internal/domain/usecase/item/listitems"
	"wherewhat/internal/domain/usecase/item/updateitem"
	"wherewhat/internal/domain/usecase/location/createlocation"
	"wherewhat/internal/domain/usecase/location/deletelocation"
	"wherewhat/internal/domain/usecase/location/listlocations"
	"wherewhat/internal/domain/usecase/location/updatelocation"
	"wherewhat/internal/domain/usecase/user/changepassword"
	"wherewhat/internal/domain/usecase/user/updatelanguage"
	"wherewhat/internal/domain/usecase/user/updatelocationfilterdepth"
	"wherewhat/internal/domain/usecase/user/updateusername"
)

// ItemUseCases collects the use cases behind the /api/items routes.
type ItemUseCases struct {
	Create *createitem.UseCase
	Update *updateitem.UseCase
	Delete *deleteitem.UseCase
	List   *listitems.UseCase
}

// LocationUseCases collects the use cases behind the /api/locations routes.
type LocationUseCases struct {
	Create *createlocation.UseCase
	Update *updatelocation.UseCase
	Delete *deletelocation.UseCase
	List   *listlocations.UseCase
}

// AuthUseCases collects the use cases behind the /api/auth routes.
type AuthUseCases struct {
	Register     *registeruser.UseCase
	Login        *login.UseCase
	Logout       *logoutuser.UseCase
	Authenticate *authenticatesession.UseCase
}

// UserUseCases collects the use cases behind the /api/user routes.
type UserUseCases struct {
	UpdateLanguage            *updatelanguage.UseCase
	UpdateLocationFilterDepth *updatelocationfilterdepth.UseCase
	UpdateUsername            *updateusername.UseCase
	ChangePassword            *changepassword.UseCase
}

// Server holds every use case the API surfaces, plus the handful of settings the HTTP layer itself
// is responsible for (cookie flags).
type Server struct {
	Items     ItemUseCases
	Locations LocationUseCases
	Auth      AuthUseCases
	Users     UserUseCases

	// CookieSecure sets the session cookie's Secure flag — true once the app is served over HTTPS.
	CookieSecure bool
}

// RegisterRoutes wires every endpoint onto mux, matching the original API exactly (paths, methods,
// auth requirements).
func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health", s.handleHealth)

	mux.HandleFunc("POST /api/auth/register", s.handleRegister)
	mux.HandleFunc("POST /api/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleLogout)
	mux.HandleFunc("GET /api/auth/me", s.handleMe)

	mux.HandleFunc("PATCH /api/user/language", s.requireAuth(s.handleUpdateLanguage))
	mux.HandleFunc("PATCH /api/user/location-filter-depth", s.requireAuth(s.handleUpdateLocationFilterDepth))
	mux.HandleFunc("PATCH /api/user/username", s.requireAuth(s.handleUpdateUsername))
	mux.HandleFunc("PATCH /api/user/password", s.requireAuth(s.handleChangePassword))

	mux.HandleFunc("GET /api/items", s.requireAuth(s.handleListItems))
	mux.HandleFunc("POST /api/items", s.requireAuth(s.handleCreateItem))
	mux.HandleFunc("PUT /api/items/{id}", s.requireAuth(s.handleUpdateItem))
	mux.HandleFunc("DELETE /api/items/{id}", s.requireAuth(s.handleDeleteItem))

	mux.HandleFunc("GET /api/locations", s.requireAuth(s.handleListLocations))
	mux.HandleFunc("POST /api/locations", s.requireAuth(s.handleCreateLocation))
	mux.HandleFunc("PUT /api/locations/{id}", s.requireAuth(s.handleUpdateLocation))
	mux.HandleFunc("DELETE /api/locations/{id}", s.requireAuth(s.handleDeleteLocation))

	// Item photo files — stored on disk (see the storage adapter); item_images.url in the DB just
	// points here.
	mux.Handle(
		filesystem.URLPrefix,
		http.StripPrefix(filesystem.URLPrefix, http.FileServer(http.Dir(filesystem.Dir))),
	)
}

// handleHealth handles GET /api/health — a trivial liveness check.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
