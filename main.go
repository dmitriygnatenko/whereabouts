package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
)

// Явный список файлов вместо "web/*" — web/files/ туда намеренно не входит:
// это runtime-каталог для загруженных фото (см. imagestore.go), он отдаётся
// напрямую с диска через отдельный маршрут "/files/" (см. registerRoutes),
// а не из вшитого в бинарник снапшота. Если бы мы embed'или его целиком,
// сборка ломалась бы в момент, когда в этом каталоге на диске нет ни одного
// файла (go:embed не умеет embed'ить пустую директорию).
//
//go:embed web/index.html web/styles.css web/api.js web/i18n.js web/app.js
//go:embed web/favicon.ico web/favicon.svg web/apple-touch-icon.png
var webFiles embed.FS

// loadEnvFile populates process env vars from a local .env file, if present —
// convenient for local development so you don't have to export DB_HOST etc.
// by hand. Real environment variables always win: godotenv.Load never
// overwrites a variable that's already set, so this is a no-op in
// production/Docker where config comes from the real environment.
func loadEnvFile() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Printf("warning: failed to read .env: %v", err)
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// defaultPortForDriver — порт по умолчанию для DB_PORT, если он не задан
// явно: у Postgres и MariaDB он разный, а угадать он неважно для SQLite.
func defaultPortForDriver(driver string) string {
	if driver == driverPostgres {
		return "5432"
	}
	return "3306"
}

func loadDBConfig() dbConfig {
	driver := getenv("DB_DRIVER", driverMariaDB)
	return dbConfig{
		Driver:     driver,
		Host:       getenv("DB_HOST", "127.0.0.1"),
		Port:       getenv("DB_PORT", defaultPortForDriver(driver)),
		User:       getenv("DB_USER", "root"),
		Password:   getenv("DB_PASSWORD", ""),
		Name:       getenv("DB_NAME", "wherewhat"),
		SQLitePath: getenv("DB_SQLITE_PATH", "./data/wherewhat.db"),
	}
}

func loadDemoUserConfig() demoUserConfig {
	return demoUserConfig{
		Username: getenv("DEMO_USERNAME", "user"),
		Password: getenv("DEMO_PASSWORD", "pass"),
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
	// Часовой пояс приложения по умолчанию — UTC: все time.Now() без явного
	// .UTC() (логи, таймауты и т.п.) и соединение с БД (см. dsn() в db.go)
	// работают в одном часовом поясе независимо от часового пояса хоста.
	time.Local = time.UTC

	loadEnvFile()
	cfg := loadDBConfig()

	if cfg.Driver == driverSQLite {
		log.Printf("использую SQLite (%s)...", cfg.SQLitePath)
	} else {
		log.Printf("проверяю базу данных %q на %s:%s (%s)...", cfg.Name, cfg.Host, cfg.Port, cfg.Driver)
	}
	if err := ensureDatabase(cfg); err != nil {
		log.Printf("предупреждение: не удалось автоматически создать базу данных: %v", err)
		log.Printf("если база %q уже существует — можно игнорировать это сообщение", cfg.Name)
	}

	db, err := openDB(cfg)
	if err != nil {
		log.Fatalf("не удалось подключиться к БД: %v", err)
	}
	defer db.Close()

	if err := migrate(db); err != nil {
		log.Fatalf("не удалось выполнить миграции: %v", err)
	}
	if err := ensureFilesDir(); err != nil {
		log.Fatalf("не удалось создать каталог для файлов фотографий: %v", err)
	}
	if err := seedDemoUser(db, loadDemoUserConfig()); err != nil {
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
