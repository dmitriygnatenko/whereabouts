package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookieName = "session_token"
	sessionDuration   = 30 * 24 * time.Hour
)

type ctxKey string

const userContextKey ctxKey = "currentUser"

// UserSettings — пользовательские настройки, хранятся в колонке users.settings
// одним JSON-полем вместо отдельных колонок.
type UserSettings struct {
	Language            string `json:"language,omitempty"`
	LocationFilterDepth int    `json:"locationFilterDepth,omitempty"`
}

// defaultUserSettings — настройки нового пользователя. Язык оставляем пустым:
// он проставится при первом успешном логине из языка, который передаёт фронтенд
// (см. handleLogin), а не жёстко захардкожен на английский.
func defaultUserSettings() UserSettings {
	return UserSettings{Language: "", LocationFilterDepth: 0}
}

// scanUserSettings разбирает JSON из колонки users.settings; пустое значение
// (старые/битые строки) откатывается на настройки по умолчанию.
func scanUserSettings(raw []byte) (UserSettings, error) {
	settings := defaultUserSettings()
	if len(raw) == 0 {
		return settings, nil
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return UserSettings{}, err
	}
	return settings, nil
}

// PublicUser — то, что отдаём наружу в JSON. Хеш пароля сюда никогда не попадает.
// UserSettings встроена анонимно, чтобы её поля (language, locationFilterDepth)
// разворачивались в JSON-ответе на верхнем уровне — так фронтенд как читал
// currentUser.language напрямую, так и продолжает.
type PublicUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	UserSettings
}

type registerInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
	// Language — язык интерфейса фронтенда на момент логина (детект по
	// браузеру или ранее сохранённое в localStorage значение). Используется
	// только чтобы проставить язык пользователю, если он ещё не задан.
	Language string `json:"language"`
}

type updateLanguageInput struct {
	Language string `json:"language"`
}

type updateLocationFilterDepthInput struct {
	LocationFilterDepth int `json:"locationFilterDepth"`
}

type updateUsernameInput struct {
	Username        string `json:"username"`
	CurrentPassword string `json:"currentPassword"`
}

type changePasswordInput struct {
	CurrentPassword string `json:"currentPassword"`
	NewPassword     string `json:"newPassword"`
}

// supportedLanguages — языки интерфейса, которые понимает фронтенд.
// Держите в синхроне с SUPPORTED_LOCALES в web/i18n.js ("en" там не нужен —
// это язык по умолчанию, на который фронтенд откатывается сам).
var supportedLanguages = map[string]bool{"en": true, "ru": true, "de": true, "es": true, "fr": true}

// userFromContext достаёт пользователя, положенного в контекст миддлварью requireAuth.
func userFromContext(r *http.Request) *PublicUser {
	u, _ := r.Context().Value(userContextKey).(*PublicUser)
	return u
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

const minUsernameLength = 3

func normalizeUsername(u string) string {
	return strings.ToLower(strings.TrimSpace(u))
}

// ---------- пароли ----------

func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func checkPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// ---------- сессии ----------

// createSession создаёт запись сессии в БД и выставляет httpOnly-куку.
func (s *server) createSession(w http.ResponseWriter, userID int64) error {
	token, err := newToken()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	expires := now.Add(sessionDuration)

	if _, err := s.db.Exec(
		`INSERT INTO sessions (token, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		token, userID, now, expires,
	); err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cookieSecure,
		Expires:  expires,
		MaxAge:   int(sessionDuration.Seconds()),
	})
	return nil
}

// destroySession удаляет сессию из БД (если кука была) и стирает куку в браузере.
func (s *server) destroySession(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE token = ?`, cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cookieSecure,
		MaxAge:   -1,
	})
}

// userFromSession проверяет куку и возвращает пользователя, если сессия жива.
func (s *server) userFromSession(r *http.Request) (*PublicUser, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, errors.New("no session")
	}

	var user PublicUser
	var settingsRaw []byte
	var expiresAt string
	err = s.db.QueryRow(
		`SELECT u.id, u.username, u.settings, s.expires_at
		 FROM sessions s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.token = ?`,
		cookie.Value,
	).Scan(&user.ID, &user.Username, &settingsRaw, &expiresAt)
	if err != nil {
		return nil, errors.New("session not found")
	}
	if user.UserSettings, err = scanUserSettings(settingsRaw); err != nil {
		return nil, err
	}

	if expiresAt < time.Now().UTC().Format(time.RFC3339) {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE token = ?`, cookie.Value)
		return nil, errors.New("session expired")
	}

	return &user, nil
}

// requireAuth — миддлварь для ручек данных: без валидной сессии дальше не пускает.
func (s *server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := s.userFromSession(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "Authentication required")
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// ---------- хендлеры ----------

func (s *server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var in registerInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	username := normalizeUsername(in.Username)
	if username == "" {
		writeError(w, http.StatusUnprocessableEntity, "Please enter a username")
		return
	}
	if len(username) < minUsernameLength {
		writeError(w, http.StatusUnprocessableEntity, "Username must be at least 3 characters")
		return
	}
	if len(in.Password) < 4 {
		writeError(w, http.StatusUnprocessableEntity, "Password must be at least 4 characters")
		return
	}

	hash, err := hashPassword(in.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to process password")
		return
	}

	settings := defaultUserSettings()
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to process settings")
		return
	}

	res, err := s.db.Exec(
		`INSERT INTO users (username, password_hash, settings) VALUES (?, ?, ?)`,
		username, hash, settingsJSON,
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			writeError(w, http.StatusConflict, "A user with this username is already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to create user")
		return
	}
	id, err := res.LastInsertId()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create user")
		return
	}

	if err := s.createSession(w, id); err != nil {
		writeError(w, http.StatusInternalServerError, "User created, but failed to start a session")
		return
	}
	writeJSON(w, http.StatusCreated, PublicUser{ID: id, Username: username, UserSettings: settings})
}

func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in loginInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	username := normalizeUsername(in.Username)

	var user PublicUser
	var settingsRaw []byte
	var hash string
	err := s.db.QueryRow(
		`SELECT id, username, settings, password_hash FROM users WHERE username = ?`, username,
	).Scan(&user.ID, &user.Username, &settingsRaw, &hash)
	if err != nil || !checkPassword(hash, in.Password) {
		writeError(w, http.StatusUnauthorized, "Incorrect username or password")
		return
	}
	if user.UserSettings, err = scanUserSettings(settingsRaw); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to read user settings")
		return
	}

	// First login without a saved language yet — adopt whatever the frontend
	// detected (browser locale or its own cached value) and persist it, so
	// subsequent sessions on any device start in that language.
	if user.Language == "" {
		if lang := strings.ToLower(strings.TrimSpace(in.Language)); supportedLanguages[lang] {
			if err := s.saveUserLanguage(user.ID, lang); err != nil {
				writeError(w, http.StatusInternalServerError, "Failed to save language preference")
				return
			}
			user.Language = lang
		}
	}

	if err := s.createSession(w, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to start a session")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.destroySession(w, r)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, err := s.userFromSession(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// handleUpdateLanguage сохраняет выбранный пользователем язык интерфейса —
// используется вместо localStorage, чтобы настройка не терялась между устройствами.
func (s *server) handleUpdateLanguage(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r)
	if user == nil {
		writeError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	var in updateLanguageInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	lang := strings.ToLower(strings.TrimSpace(in.Language))
	if !supportedLanguages[lang] {
		writeError(w, http.StatusUnprocessableEntity, "Unsupported language")
		return
	}

	if err := s.saveUserLanguage(user.ID, lang); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save language preference")
		return
	}
	user.Language = lang
	writeJSON(w, http.StatusOK, user)
}

// handleUpdateLocationFilterDepth сохраняет глубину вложенности мест, которую
// показывать в чипах-фильтрах на вкладке "Вещи". 0 означает "без ограничения".
func (s *server) handleUpdateLocationFilterDepth(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r)
	if user == nil {
		writeError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	var in updateLocationFilterDepthInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if in.LocationFilterDepth != 0 && in.LocationFilterDepth < 1 {
		writeError(w, http.StatusUnprocessableEntity, "Location filter depth must be 0 (show all) or at least 1")
		return
	}

	if _, err := s.db.Exec(
		`UPDATE users SET settings = JSON_SET(settings, '$.locationFilterDepth', ?) WHERE id = ?`,
		in.LocationFilterDepth, user.ID,
	); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to save location filter depth")
		return
	}
	user.LocationFilterDepth = in.LocationFilterDepth
	writeJSON(w, http.StatusOK, user)
}

// saveUserLanguage persists the interface language to the settings JSON column.
func (s *server) saveUserLanguage(userID int64, lang string) error {
	_, err := s.db.Exec(
		`UPDATE users SET settings = JSON_SET(settings, '$.language', ?) WHERE id = ?`,
		lang, userID,
	)
	return err
}

// checkCurrentPassword re-fetches the user's password hash and verifies it
// against the given plaintext — used before letting the profile page change
// the username or password, since the session cookie alone shouldn't be enough.
func (s *server) checkCurrentPassword(userID int64, password string) (bool, error) {
	var hash string
	if err := s.db.QueryRow(`SELECT password_hash FROM users WHERE id = ?`, userID).Scan(&hash); err != nil {
		return false, err
	}
	return checkPassword(hash, password), nil
}

// handleUpdateUsername lets a signed-in user change their login username,
// after confirming their current password.
func (s *server) handleUpdateUsername(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r)
	if user == nil {
		writeError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	var in updateUsernameInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	username := normalizeUsername(in.Username)
	if username == "" {
		writeError(w, http.StatusUnprocessableEntity, "Please enter a username")
		return
	}
	if len(username) < minUsernameLength {
		writeError(w, http.StatusUnprocessableEntity, "Username must be at least 3 characters")
		return
	}

	ok, err := s.checkCurrentPassword(user.ID, in.CurrentPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to verify current password")
		return
	}
	if !ok {
		// 403, not 401: the session itself is still valid — only the
		// re-entered password was wrong. A 401 here would trip the
		// frontend's global "session expired" handler and log the user out.
		writeError(w, http.StatusForbidden, "Incorrect current password")
		return
	}

	if username == user.Username {
		writeJSON(w, http.StatusOK, user)
		return
	}

	if _, err := s.db.Exec(`UPDATE users SET username = ? WHERE id = ?`, username, user.ID); err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			writeError(w, http.StatusConflict, "A user with this username is already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to update username")
		return
	}
	user.Username = username
	writeJSON(w, http.StatusOK, user)
}

// handleChangePassword lets a signed-in user change their password, after
// confirming their current one.
func (s *server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r)
	if user == nil {
		writeError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	var in changePasswordInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(in.NewPassword) < 4 {
		writeError(w, http.StatusUnprocessableEntity, "Password must be at least 4 characters")
		return
	}

	ok, err := s.checkCurrentPassword(user.ID, in.CurrentPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to verify current password")
		return
	}
	if !ok {
		// 403, not 401: the session itself is still valid — only the
		// re-entered password was wrong. A 401 here would trip the
		// frontend's global "session expired" handler and log the user out.
		writeError(w, http.StatusForbidden, "Incorrect current password")
		return
	}

	newHash, err := hashPassword(in.NewPassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to process password")
		return
	}
	if _, err := s.db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, newHash, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to update password")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
