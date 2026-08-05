package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ---------- утилиты ----------

func newID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

// decodeJSONLimited — как decodeJSON, но дополнительно ограничивает размер
// тела запроса. Используется там, куда прилетают фотографии, чтобы не
// раздувать память сервера чрезмерно большими запросами.
func decodeJSONLimited(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		if err.Error() == "http: request body too large" {
			return fmt.Errorf("request too large (max %d MB)", maxRequestBytes/1024/1024)
		}
		return err
	}
	return nil
}

// processImages прогоняет каждую фотографию через серверное сжатие и
// сохраняет результат в файл на диске (см. images.go и imagestore.go) —
// в БД попадает только ссылка на файл. Фото, которые уже являются ссылкой
// на ранее сохранённый файл (не изменились с прошлого сохранения вещи),
// не пересохраняются.
func (s *server) processImages(images []string) ([]string, error) {
	processed := make([]string, len(images))
	for i, img := range images {
		if isStoredImageURL(img) {
			processed[i] = img
			continue
		}
		data, ext, err := decodeAndCompressImage(img)
		if err != nil {
			return nil, fmt.Errorf("photo #%d: failed to process (%w)", i+1, err)
		}
		url, err := saveImageFile(data, ext)
		if err != nil {
			return nil, fmt.Errorf("photo #%d: failed to save (%w)", i+1, err)
		}
		processed[i] = url
	}
	return processed, nil
}

// server держит зависимость от БД, чтобы обработчикам не нужно было
// тащить *sql.DB через глобальные переменные.
type server struct {
	db           *sql.DB
	cookieSecure bool
}

func registerRoutes(mux *http.ServeMux, s *server) {
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
	mux.HandleFunc("DELETE /api/locations/{id}", s.requireAuth(s.handleDeleteLocation))

	// Item photo files — stored on disk (see imagestore.go), item_images.url
	// in the DB just points here.
	mux.Handle("/files/", http.StripPrefix("/files/", http.FileServer(http.Dir(filesDir))))
}

func (s *server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// ---------- вещи ----------

// loadImages подтягивает фотографии для набора вещей одним запросом,
// чтобы не делать N+1 обращений к БД при выдаче списка.
func (s *server) loadImages(itemIDs []string) (map[string][]string, error) {
	result := make(map[string][]string, len(itemIDs))
	if len(itemIDs) == 0 {
		return result, nil
	}

	placeholders := strings.Repeat("?,", len(itemIDs))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]any, len(itemIDs))
	for i, id := range itemIDs {
		args[i] = id
	}

	rows, err := s.db.Query(
		`SELECT item_id, url FROM item_images WHERE item_id IN (`+placeholders+`) ORDER BY item_id, position`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var itemID, url string
		if err := rows.Scan(&itemID, &url); err != nil {
			return nil, err
		}
		result[itemID] = append(result[itemID], url)
	}
	return result, rows.Err()
}

// imageURLsForItem returns the currently stored photo URLs for one item —
// used before replacing/deleting an item's photos so orphaned files on disk
// can be cleaned up afterwards.
func (s *server) imageURLsForItem(itemID string) ([]string, error) {
	rows, err := s.db.Query(`SELECT url FROM item_images WHERE item_id = ?`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var urls []string
	for rows.Next() {
		var url string
		if err := rows.Scan(&url); err != nil {
			return nil, err
		}
		urls = append(urls, url)
	}
	return urls, rows.Err()
}

func (s *server) handleListItems(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(
		`SELECT id, name, location_id, notes, updated_at FROM items ORDER BY updated_at DESC`,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to load items")
		return
	}
	defer rows.Close()

	var items []Item
	var ids []string
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.Name, &it.LocationID, &it.Notes, &it.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to read item")
			return
		}
		it.Images = []string{}
		items = append(items, it)
		ids = append(ids, it.ID)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to load items")
		return
	}

	imagesByItem, err := s.loadImages(ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to load photos")
		return
	}
	for i := range items {
		if imgs, ok := imagesByItem[items[i].ID]; ok {
			items[i].Images = imgs
		}
	}

	if items == nil {
		items = []Item{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (s *server) getItemByID(id string) (*Item, error) {
	var it Item
	err := s.db.QueryRow(
		`SELECT id, name, location_id, notes, updated_at FROM items WHERE id = ?`, id,
	).Scan(&it.ID, &it.Name, &it.LocationID, &it.Notes, &it.UpdatedAt)
	if err != nil {
		return nil, err
	}
	imagesByItem, err := s.loadImages([]string{id})
	if err != nil {
		return nil, err
	}
	it.Images = imagesByItem[id]
	if it.Images == nil {
		it.Images = []string{}
	}
	return &it, nil
}

func validateItemInput(in itemInput) map[string]string {
	errs := map[string]string{}
	if strings.TrimSpace(in.Name) == "" {
		errs["name"] = "Enter the item name"
	}
	if strings.TrimSpace(in.LocationID) == "" {
		errs["locationId"] = "Choose a location"
	}
	return errs
}

func (s *server) locationExists(id string) (bool, error) {
	var exists int
	err := s.db.QueryRow(`SELECT 1 FROM locations WHERE id = ?`, id).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// replaceItemImages swaps an item's photo rows for the given list of URLs,
// then deletes the files of any old photo that isn't in the new list — e.g.
// the user removed it from the item, or replaced it with a new upload.
func (s *server) replaceItemImages(itemID string, images []string) error {
	oldURLs, err := s.imageURLsForItem(itemID)
	if err != nil {
		return err
	}

	if _, err := s.db.Exec(`DELETE FROM item_images WHERE item_id = ?`, itemID); err != nil {
		return err
	}
	for pos, url := range images {
		if _, err := s.db.Exec(
			`INSERT INTO item_images (item_id, url, position) VALUES (?, ?, ?)`,
			itemID, url, pos,
		); err != nil {
			return err
		}
	}

	kept := make(map[string]bool, len(images))
	for _, url := range images {
		kept[url] = true
	}
	for _, url := range oldURLs {
		if !kept[url] {
			deleteImageFile(url)
		}
	}
	return nil
}

func (s *server) handleCreateItem(w http.ResponseWriter, r *http.Request) {
	var in itemInput
	if err := decodeJSONLimited(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}
	if errs := validateItemInput(in); len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "Please check the form fields", "fields": errs})
		return
	}
	exists, err := s.locationExists(in.LocationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to verify location")
		return
	}
	if !exists {
		writeError(w, http.StatusUnprocessableEntity, "The specified location was not found")
		return
	}
	images, err := s.processImages(in.Images)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	id := newID("")
	now := time.Now().UTC().Format(time.RFC3339)

	if _, err := s.db.Exec(
		`INSERT INTO items (id, name, location_id, notes, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, strings.TrimSpace(in.Name), in.LocationID, strings.TrimSpace(in.Notes), now, now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save item")
		return
	}
	if err := s.replaceItemImages(id, images); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save photos")
		return
	}

	item, err := s.getItemByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Item saved, but failed to read it back")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *server) handleUpdateItem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var in itemInput
	if err := decodeJSONLimited(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}
	if errs := validateItemInput(in); len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "Please check the form fields", "fields": errs})
		return
	}
	exists, err := s.locationExists(in.LocationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to verify location")
		return
	}
	if !exists {
		writeError(w, http.StatusUnprocessableEntity, "The specified location was not found")
		return
	}
	images, err := s.processImages(in.Images)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.Exec(
		`UPDATE items SET name = ?, location_id = ?, notes = ?, updated_at = ? WHERE id = ?`,
		strings.TrimSpace(in.Name), in.LocationID, strings.TrimSpace(in.Notes), now, id,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update item")
		return
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, "Record not found")
		return
	}
	if err := s.replaceItemImages(id, images); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update photos")
		return
	}

	item, err := s.getItemByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Item updated, but failed to read it back")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *server) handleDeleteItem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	// Best-effort: read the photo URLs before the DB delete (item_images rows
	// cascade-delete with the item) so we can also remove their files on disk.
	urls, _ := s.imageURLsForItem(id)

	res, err := s.db.Exec(`DELETE FROM items WHERE id = ?`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete item")
		return
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, "Record not found")
		return
	}
	for _, url := range urls {
		deleteImageFile(url)
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------- места ----------

func (s *server) handleListLocations(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`SELECT id, name, color, parent_id FROM locations ORDER BY created_at`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to load locations")
		return
	}
	defer rows.Close()

	var locations []Location
	for rows.Next() {
		var l Location
		if err := rows.Scan(&l.ID, &l.Name, &l.Color, &l.ParentID); err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to read location")
			return
		}
		locations = append(locations, l)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to load locations")
		return
	}
	if locations == nil {
		locations = []Location{}
	}
	writeJSON(w, http.StatusOK, locations)
}

func (s *server) handleCreateLocation(w http.ResponseWriter, r *http.Request) {
	var in locationInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		writeError(w, http.StatusUnprocessableEntity, "Please enter a location name")
		return
	}
	color := strings.TrimSpace(in.Color)
	if color == "" {
		color = "#3D6B63"
	}

	if in.ParentID != nil && strings.TrimSpace(*in.ParentID) != "" {
		exists, err := s.locationExists(*in.ParentID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to verify parent location")
			return
		}
		if !exists {
			writeError(w, http.StatusUnprocessableEntity, "Parent location not found")
			return
		}
	} else {
		in.ParentID = nil
	}

	id := newID("loc-")
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.Exec(
		`INSERT INTO locations (id, name, color, parent_id, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, name, color, in.ParentID, now,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save location")
		return
	}

	writeJSON(w, http.StatusCreated, Location{ID: id, Name: name, Color: color, ParentID: in.ParentID})
}

func (s *server) handleDeleteLocation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var childCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM locations WHERE parent_id = ?`, id).Scan(&childCount); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to check nested locations")
		return
	}
	if childCount > 0 {
		writeError(w, http.StatusConflict, "This location has nested locations — delete or move them first")
		return
	}

	var itemCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM items WHERE location_id = ?`, id).Scan(&itemCount); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to check items in this location")
		return
	}
	if itemCount > 0 {
		writeError(w, http.StatusConflict, "This location has items in it — move them elsewhere first")
		return
	}

	res, err := s.db.Exec(`DELETE FROM locations WHERE id = ?`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to delete location")
		return
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, "Location not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
