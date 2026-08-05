package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

// PublicUser — то, что отдаём наружу в JSON. Хеш пароля сюда никогда не попадает.
type PublicUser struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Language string `json:"language"`
}

type registerInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type updateLanguageInput struct {
	Language string `json:"language"`
}

// supportedLanguages — языки интерфейса, которые понимает фронтенд.
var supportedLanguages = map[string]bool{"en": true, "ru": true}

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

func normalizeEmail(e string) string {
	return strings.ToLower(strings.TrimSpace(e))
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
func (s *server) createSession(w http.ResponseWriter, userID string) error {
	token, err := newToken()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	expires := now.Add(sessionDuration)

	if _, err := s.db.Exec(
		`INSERT INTO sessions (token, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		token, userID, now.Format(time.RFC3339), expires.Format(time.RFC3339),
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
		return nil, errors.New("нет сессии")
	}

	var user PublicUser
	var expiresAt string
	err = s.db.QueryRow(
		`SELECT u.id, u.name, u.email, u.language, s.expires_at
		 FROM sessions s
		 JOIN users u ON u.id = s.user_id
		 WHERE s.token = ?`,
		cookie.Value,
	).Scan(&user.ID, &user.Name, &user.Email, &user.Language, &expiresAt)
	if err != nil {
		return nil, errors.New("сессия не найдена")
	}

	if expiresAt < time.Now().UTC().Format(time.RFC3339) {
		_, _ = s.db.Exec(`DELETE FROM sessions WHERE token = ?`, cookie.Value)
		return nil, errors.New("сессия истекла")
	}

	return &user, nil
}

// requireAuth — миддлварь для ручек данных: без валидной сессии дальше не пускает.
func (s *server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, err := s.userFromSession(r)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "необходима авторизация")
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
		writeError(w, http.StatusBadRequest, "некорректное тело запроса")
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = "Без имени"
	}
	email := normalizeEmail(in.Email)
	if email == "" {
		writeError(w, http.StatusUnprocessableEntity, "укажите email")
		return
	}
	if len(in.Password) < 4 {
		writeError(w, http.StatusUnprocessableEntity, "пароль должен быть не короче 4 символов")
		return
	}

	hash, err := hashPassword(in.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось обработать пароль")
		return
	}

	id := newID("user-")
	now := time.Now().UTC().Format(time.RFC3339)
	// language не передаётся при регистрации — используется дефолт колонки ('en').
	_, err = s.db.Exec(
		`INSERT INTO users (id, name, email, password_hash, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, name, email, hash, now,
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			writeError(w, http.StatusConflict, "пользователь с таким email уже зарегистрирован")
			return
		}
		writeError(w, http.StatusInternalServerError, "не удалось создать пользователя")
		return
	}

	if err := s.createSession(w, id); err != nil {
		writeError(w, http.StatusInternalServerError, "пользователь создан, но не удалось начать сессию")
		return
	}
	writeJSON(w, http.StatusCreated, PublicUser{ID: id, Name: name, Email: email, Language: "en"})
}

func (s *server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in loginInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "некорректное тело запроса")
		return
	}
	email := normalizeEmail(in.Email)

	var user PublicUser
	var hash string
	err := s.db.QueryRow(
		`SELECT id, name, email, language, password_hash FROM users WHERE email = ?`, email,
	).Scan(&user.ID, &user.Name, &user.Email, &user.Language, &hash)
	if err != nil || !checkPassword(hash, in.Password) {
		writeError(w, http.StatusUnauthorized, "неверный email или пароль")
		return
	}

	if err := s.createSession(w, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось начать сессию")
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
		writeError(w, http.StatusUnauthorized, "не авторизован")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// handleUpdateLanguage сохраняет выбранный пользователем язык интерфейса —
// используется вместо localStorage, чтобы настройка не терялась между устройствами.
func (s *server) handleUpdateLanguage(w http.ResponseWriter, r *http.Request) {
	user := userFromContext(r)
	if user == nil {
		writeError(w, http.StatusUnauthorized, "не авторизован")
		return
	}

	var in updateLanguageInput
	if err := decodeJSON(r, &in); err != nil {
		writeError(w, http.StatusBadRequest, "некорректное тело запроса")
		return
	}
	lang := strings.ToLower(strings.TrimSpace(in.Language))
	if !supportedLanguages[lang] {
		writeError(w, http.StatusUnprocessableEntity, "недопустимый язык")
		return
	}

	if _, err := s.db.Exec(`UPDATE users SET language = ? WHERE id = ?`, lang, user.ID); err != nil {
		writeError(w, http.StatusInternalServerError, "не удалось сохранить язык")
		return
	}
	user.Language = lang
	writeJSON(w, http.StatusOK, user)
}
