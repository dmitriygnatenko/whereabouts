package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"
)

//go:embed web/*
var webFiles embed.FS

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loadDBConfig() dbConfig {
	return dbConfig{
		Host:     getenv("DB_HOST", "127.0.0.1"),
		Port:     getenv("DB_PORT", "3306"),
		User:     getenv("DB_USER", "root"),
		Password: getenv("DB_PASSWORD", ""),
		Name:     getenv("DB_NAME", "wherewhat"),
	}
}

// withCORS разрешает фронтенду ходить на API с другого origin (например,
// при локальной разработке фронтенда отдельным сервером). Если фронтенд
// отдаётся этим же Go-сервером (см. ниже), CORS фактически не нужен,
// но не мешает и оставлен для гибкости.
// ВАЖНО: авторизация теперь на httpOnly-куках. Если фронтенд когда-нибудь
// будет обслуживаться с другого origin, "*" в Allow-Origin работать не
// будет — браузер не отправляет credentialed-запросы на wildcard-origin.
// В этом случае замените "*" на конкретный origin фронтенда и добавьте
// Access-Control-Allow-Credentials: true.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// withLogging пишет короткую строку лога на каждый запрос — этого достаточно
// для локальной разработки; для продакшена лучше structured-логирование.
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s", r.Method, r.URL.Path, time.Since(start))
	})
}

func main() {
	cfg := loadDBConfig()

	log.Printf("проверяю базу данных %q на %s:%s...", cfg.Name, cfg.Host, cfg.Port)
	if err := ensureDatabase(cfg); err != nil {
		log.Printf("предупреждение: не удалось автоматически создать базу данных: %v", err)
		log.Printf("если база %q уже существует — можно игнорировать это сообщение", cfg.Name)
	}

	db, err := openDB(cfg)
	if err != nil {
		log.Fatalf("не удалось подключиться к MariaDB: %v", err)
	}
	defer db.Close()

	if err := migrate(db); err != nil {
		log.Fatalf("не удалось выполнить миграции: %v", err)
	}
	if err := seedIfEmpty(db); err != nil {
		log.Fatalf("не удалось наполнить базу демо-данными: %v", err)
	}
	if err := seedDemoUser(db); err != nil {
		log.Fatalf("не удалось создать демо-пользователя: %v", err)
	}
	log.Println("база данных готова")

	cookieSecure := getenv("COOKIE_SECURE", "false") == "true"
	srv := &server{db: db, cookieSecure: cookieSecure}

	mux := http.NewServeMux()
	registerRoutes(mux, srv)

	webRoot, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatalf("не удалось встроить фронтенд: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(webRoot)))

	handler := withLogging(withCORS(mux))

	addr := ":" + getenv("PORT", "8080")
	log.Printf("слушаю на %s (открой http://localhost%s в браузере)", addr, addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("сервер остановлен: %v", err)
	}
}
