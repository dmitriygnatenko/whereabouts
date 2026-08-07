package http

import (
	"net/http"

	"wherewhat/internal/domain/usecase/location/create"
	"wherewhat/internal/domain/usecase/location/delete"
	"wherewhat/internal/domain/usecase/location/update"
)

type locationRequest struct {
	Name     string  `json:"name"`
	Color    string  `json:"color"`
	ParentID *uint64 `json:"parentId"`
}

// handleListLocations handles GET /api/locations.
func (s *Server) handleListLocations(w http.ResponseWriter, r *http.Request) {
	locations, err := s.Locations.List.Execute(r.Context())
	if err != nil {
		writeUseCaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, locations.Locations)
}

// handleCreateLocation handles POST /api/locations.
func (s *Server) handleCreateLocation(w http.ResponseWriter, r *http.Request) {
	var input locationRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	created, err := s.Locations.Create.Execute(
		r.Context(),
		create.Input{
			Name:     input.Name,
			Color:    input.Color,
			ParentID: input.ParentID,
		},
	)
	if err != nil {
		writeUseCaseError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, created.Location)
}

// handleUpdateLocation handles PUT /api/locations/{id}.
func (s *Server) handleUpdateLocation(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r)
	if !ok {
		return
	}

	var input locationRequest
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	updated, err := s.Locations.Update.Execute(
		r.Context(),
		update.Input{
			ID:    id,
			Name:  input.Name,
			Color: input.Color,
		},
	)
	if err != nil {
		writeUseCaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, updated.Location)
}

// handleDeleteLocation handles DELETE /api/locations/{id}.
func (s *Server) handleDeleteLocation(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r)
	if !ok {
		return
	}

	if err := s.Locations.Delete.Execute(
		r.Context(),
		delete.Input{
			ID: id,
		},
	); err != nil {
		writeUseCaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
