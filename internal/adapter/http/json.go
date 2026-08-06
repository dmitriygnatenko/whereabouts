package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"wherewhat/internal/domain/service/imageprocessor"
)

// maxJSONBodyBytes caps the request body of plain JSON endpoints (no photos) — generous for any
// legitimate payload here (credentials, item/ location names, settings...) while still bounding how
// much an unauthenticated client (e.g. hitting /api/auth/register or /api/auth/login) can force the
// server to buffer.
const maxJSONBodyBytes = 64 * 1024

// writeJSON writes v as a JSON response body with the given status code.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a {"error": message} JSON response with the given status code.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// decodeJSON decodes a JSON request body capped at maxJSONBodyBytes.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONBodyBytes)
	defer r.Body.Close()

	return json.NewDecoder(r.Body).Decode(dst)
}

// decodeJSONLimited is like decodeJSON but also caps the request body size — used on endpoints that
// receive photos, so the server can't be pushed into swapping by an oversized request.
func decodeJSONLimited(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, imageprocessor.MaxRequestBytes)
	defer r.Body.Close()

	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		if err.Error() == "http: request body too large" {
			return fmt.Errorf("request too large (max %d MB)", imageprocessor.MaxRequestBytes/1024/1024)
		}

		return err
	}

	return nil
}

// parseIDParam parses a numeric id out of the request path (e.g. {id} in /api/items/{id}), writing
// a 400 and returning ok=false if it doesn't look like a positive integer.
func parseIDParam(w http.ResponseWriter, r *http.Request) (id uint64, ok bool) {
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 64)
	if err != nil || id == 0 {
		writeError(w, http.StatusBadRequest, "Invalid id")
		return 0, false
	}

	return id, true
}
