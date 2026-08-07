package http

import (
	"net/http"

	"wherewhat/internal/domain/usecase/item/create"
	"wherewhat/internal/domain/usecase/item/delete"
	"wherewhat/internal/domain/usecase/item/update"
)

type itemRequest struct {
	Name       string   `json:"name"`
	LocationID uint64   `json:"locationId"`
	Notes      string   `json:"notes"`
	Images     []string `json:"images"`
}

// handleListItems handles GET /api/items.
func (s *Server) handleListItems(w http.ResponseWriter, r *http.Request) {
	items, err := s.Items.List.Execute(r.Context())
	if err != nil {
		writeUseCaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, items.Items)
}

// handleCreateItem handles POST /api/items.
func (s *Server) handleCreateItem(w http.ResponseWriter, r *http.Request) {
	var input itemRequest
	if err := decodeJSONLimited(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	created, err := s.Items.Create.Execute(
		r.Context(),
		create.Input{
			Name:       input.Name,
			LocationID: input.LocationID,
			Notes:      input.Notes,
			Images:     input.Images,
		},
	)
	if err != nil {
		writeUseCaseError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, created.Item)
}

// handleUpdateItem handles PUT /api/items/{id}.
func (s *Server) handleUpdateItem(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r)
	if !ok {
		return
	}

	var input itemRequest
	if err := decodeJSONLimited(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	updated, err := s.Items.Update.Execute(
		r.Context(),
		update.Input{
			ID:         id,
			Name:       input.Name,
			LocationID: input.LocationID,
			Notes:      input.Notes,
			Images:     input.Images,
		},
	)
	if err != nil {
		writeUseCaseError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, updated.Item)
}

// handleDeleteItem handles DELETE /api/items/{id}.
func (s *Server) handleDeleteItem(w http.ResponseWriter, r *http.Request) {
	id, ok := parseIDParam(w, r)
	if !ok {
		return
	}

	if err := s.Items.Delete.Execute(
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
