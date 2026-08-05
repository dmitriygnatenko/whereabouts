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
			return fmt.Errorf("слишком большой запрос (максимум %d МБ)", maxRequestBytes/1024/1024)
		}
		return err
	}
	return nil
}

// processImages прогоняет каждую фотографию через серверное сжатие:
// маленькие изображения возвращаются как есть, большие — уменьшаются
// и пережимаются в JPEG (см. images.go). Так фронтенд-сжатие остаётся
// быстрой оптимизацией для UX, а бэкенд — источником истины по размеру.
func (s *server) processImages(images []string) ([]string, error) {
	processed := make([]string, len(images))
	for i, img := range images {
		out, err := processImageDataURL(img)
		if err != nil {
			return nil, fmt.Errorf("фото №%d: не удалось обработать (%w)", i+1, err)
		}
		processed[i] = out
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
	mux.HandleFunc("PATCH /api/auth/language", s.requireAuth(s.handleUpdateLanguage))

	mux.HandleFunc("GET /api/items", s.requireAuth(s.handleListItems))
	mux.HandleFunc("POST /api/items", s.requireAuth(s.handleCreateItem))
	mux.HandleFunc("PUT /api/items/{id}", s.requireAuth(s.handleUpdateItem))
	mux.HandleFunc("DELETE /api/items/{id}", s.requireAuth(s.handleDeleteItem))

	mux.HandleFunc("GET /api/locations", s.requireAuth(s.handleListLocations))
	mux.HandleFunc("POST /api/locations", s.requireAuth(s.handleCreateLocation))
	mux.HandleFunc("DELETE /api/locations/{id}", s.requireAuth(s.handleDeleteLocation))
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
		`SELECT item_id, data_url FROM item_images WHERE item_id IN (`+placeholders+`) ORDER BY item_id, position`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var itemID, dataURL string
		if err := rows.Scan(&itemID, &dataURL); err != nil {
			return nil, err
		}
		result[itemID] = append(result[itemID], dataURL)
	}
	return result, rows.Err()
}

func (s *server) handleListItems(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(
		`SELECT id, name, location_id, notes, updated_at FROM items ORDER BY updated_at DESC`,
	)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить вещи")
		return
	}
	defer rows.Close()

	var items []Item
	var ids []string
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.Name, &it.LocationID, &it.Notes, &it.UpdatedAt); err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось прочитать вещь")
			return
		}
		it.Images = []string{}
		items = append(items, it)
		ids = append(ids, it.ID)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить вещи")
		return
	}

	imagesByItem, err := s.loadImages(ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить фотографии")
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
		errs["name"] = "Введите название вещи"
	}
	if strings.TrimSpace(in.LocationID) == "" {
		errs["locationId"] = "Выберите место хранения"
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

func (s *server) replaceItemImages(itemID string, images []string) error {
	if _, err := s.db.Exec(`DELETE FROM item_images WHERE item_id = ?`, itemID); err != nil {
		return err
	}
	for pos, dataURL := range images {
		if _, err := s.db.Exec(
			`INSERT INTO item_images (item_id, data_url, position) VALUES (?, ?, ?)`,
			itemID, dataURL, pos,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *server) handleCreateItem(w http.ResponseWriter, r *http.Request) {
	var in itemInput
	if err := decodeJSONLimited(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "некорректное тело запроса: "+err.Error())
		return
	}
	if errs := validateItemInput(in); len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "проверьте поля формы", "fields": errs})
		return
	}
	exists, err := s.locationExists(in.LocationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось проверить место")
		return
	}
	if !exists {
		writeError(w, http.StatusUnprocessableEntity, "указанное место хранения не найдено")
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
		writeError(w, http.StatusInternalServerError, "не удалось сохранить вещь")
		return
	}
	if err := s.replaceItemImages(id, images); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сохранить фотографии")
		return
	}

	item, err := s.getItemByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "вещь сохранена, но не удалось её прочитать")
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *server) handleUpdateItem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var in itemInput
	if err := decodeJSONLimited(w, r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "некорректное тело запроса: "+err.Error())
		return
	}
	if errs := validateItemInput(in); len(errs) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "проверьте поля формы", "fields": errs})
		return
	}
	exists, err := s.locationExists(in.LocationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось проверить место")
		return
	}
	if !exists {
		writeError(w, http.StatusUnprocessableEntity, "указанное место хранения не найдено")
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
		writeError(w, http.StatusInternalServerError, "не удалось обновить вещь")
		return
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, "запись не найдена")
		return
	}
	if err := s.replaceItemImages(id, images); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось обновить фотографии")
		return
	}

	item, err := s.getItemByID(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "вещь обновлена, но не удалось её прочитать")
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *server) handleDeleteItem(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	res, err := s.db.Exec(`DELETE FROM items WHERE id = ?`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось удалить вещь")
		return
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, "запись не найдена")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ---------- места ----------

func (s *server) handleListLocations(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query(`SELECT id, name, color, parent_id FROM locations ORDER BY created_at`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить места")
		return
	}
	defer rows.Close()

	var locations []Location
	for rows.Next() {
		var l Location
		if err := rows.Scan(&l.ID, &l.Name, &l.Color, &l.ParentID); err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось прочитать место")
			return
		}
		locations = append(locations, l)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось получить места")
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
		writeError(w, http.StatusBadRequest, "некорректное тело запроса")
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		writeError(w, http.StatusUnprocessableEntity, "введите название места")
		return
	}
	color := strings.TrimSpace(in.Color)
	if color == "" {
		color = "#3D6B63"
	}

	if in.ParentID != nil && strings.TrimSpace(*in.ParentID) != "" {
		exists, err := s.locationExists(*in.ParentID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "не удалось проверить родительское место")
			return
		}
		if !exists {
			writeError(w, http.StatusUnprocessableEntity, "родительское место не найдено")
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
		writeError(w, http.StatusInternalServerError, "не удалось сохранить место")
		return
	}

	writeJSON(w, http.StatusCreated, Location{ID: id, Name: name, Color: color, ParentID: in.ParentID})
}

func (s *server) handleDeleteLocation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var childCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM locations WHERE parent_id = ?`, id).Scan(&childCount); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось проверить вложенные места")
		return
	}
	if childCount > 0 {
		writeError(w, http.StatusConflict, "у этого места есть вложенные места — сначала удалите или перенесите их")
		return
	}

	var itemCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM items WHERE location_id = ?`, id).Scan(&itemCount); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось проверить вещи в этом месте")
		return
	}
	if itemCount > 0 {
		writeError(w, http.StatusConflict, "в этом месте есть вещи — сначала перенесите их в другое место")
		return
	}

	res, err := s.db.Exec(`DELETE FROM locations WHERE id = ?`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось удалить место")
		return
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		writeError(w, http.StatusNotFound, "место не найдено")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
