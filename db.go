package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"modernc.org/sqlite"
)

// Поддерживаемые драйверы БД, выбираются переменной окружения DB_DRIVER.
const (
	driverMariaDB  = "mariadb"
	driverPostgres = "postgres"
	driverSQLite   = "sqlite"
)

// dbConfig собирает параметры подключения к БД из окружения. Часть полей
// используется не всеми драйверами: Host/Port/User/Password/Name — только
// MariaDB и Postgres, SQLitePath — только SQLite.
type dbConfig struct {
	Driver     string
	Host       string
	Port       string
	User       string
	Password   string
	Name       string
	SQLitePath string
}

type demoUserConfig struct {
	Username string
	Password string
}

var identifierRE = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// appDB оборачивает *sql.DB, чтобы код хендлеров мог оставаться написанным
// на "родном" для MariaDB/SQLite диалекте (плейсхолдеры "?", LastInsertId)
// независимо от того, какой драйвер выбран: для Postgres запросы на лету
// переписываются в "$1, $2, ...", а получение id новой записи идёт через
// INSERT ... RETURNING id вместо LastInsertId (который lib/драйверы Postgres
// не поддерживают).
type appDB struct {
	*sql.DB
	driver string
}

// rebind переводит плейсхолдеры "?" в "$1", "$2", ... для Postgres; для
// остальных драйверов запрос не трогает. Ни один из запросов в проекте не
// содержит литерального "?" внутри строковых констант, так что наивная
// замена по одному символу безопасна.
func (d *appDB) rebind(query string) string {
	if d.driver != driverPostgres {
		return query
	}
	var b strings.Builder
	n := 0
	for _, r := range query {
		if r == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (d *appDB) Exec(query string, args ...any) (sql.Result, error) {
	return d.DB.Exec(d.rebind(query), args...)
}

func (d *appDB) Query(query string, args ...any) (*sql.Rows, error) {
	return d.DB.Query(d.rebind(query), args...)
}

func (d *appDB) QueryRow(query string, args ...any) *sql.Row {
	return d.DB.QueryRow(d.rebind(query), args...)
}

// insertReturningID выполняет INSERT (написанный с плейсхолдерами "?", без
// завершающей точки с запятой) и возвращает id новой записи. Для Postgres
// это INSERT ... RETURNING id, для MariaDB/SQLite — обычный Exec +
// LastInsertId.
func (d *appDB) insertReturningID(query string, args ...any) (int64, error) {
	if d.driver == driverPostgres {
		var id int64
		err := d.QueryRow(query+" RETURNING id", args...).Scan(&id)
		return id, err
	}
	res, err := d.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// sqliteConstraintUnique — расширенный код результата SQLITE_CONSTRAINT_UNIQUE
// (см. sqlite3.h); modernc.org/sqlite не экспортирует эту константу из
// публичного пакета, поэтому продублирован здесь как известное литеральное
// значение (не меняется между версиями SQLite).
const sqliteConstraintUnique = 2067

// isUniqueViolation проверяет, что ошибка БД — нарушение UNIQUE-ограничения
// (например, попытка занять уже существующий username), независимо от
// драйвера.
func isUniqueViolation(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	var sqliteErr *sqlite.Error
	if errors.As(err, &sqliteErr) {
		return sqliteErr.Code() == sqliteConstraintUnique
	}
	return false
}

// dsn возвращает строку подключения к конкретной базе данных.
func (c dbConfig) dsn() string {
	switch c.Driver {
	case driverPostgres:
		return fmt.Sprintf(
			"postgres://%s:%s@%s:%s/%s?sslmode=disable",
			c.User, c.Password, c.Host, c.Port, c.Name,
		)
	case driverSQLite:
		// _pragma=foreign_keys(1) включает проверку внешних ключей — в SQLite
		// она по умолчанию выключена и должна быть включена на каждом
		// соединении, поэтому пробрасываем это через DSN, а не отдельным Exec
		// после Open (пул может открывать новые соединения когда угодно).
		return fmt.Sprintf("file:%s?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)", c.SQLitePath)
	default:
		return fmt.Sprintf(
			"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=UTC&time_zone=%%27%%2B00%%3A00%%27&timeout=5s",
			c.User, c.Password, c.Host, c.Port, c.Name,
		)
	}
}

// serverDSN — подключение к самому серверу БД без конкретной базы, нужно
// только для того, чтобы при первом запуске создать базу, если её ещё нет.
// Для SQLite не используется (файл создаётся драйвером сам).
func (c dbConfig) serverDSN() string {
	switch c.Driver {
	case driverPostgres:
		// Postgres требует подключения к какой-то существующей базе, чтобы
		// оттуда создать другую; используем служебную БД "postgres".
		return fmt.Sprintf(
			"postgres://%s:%s@%s:%s/postgres?sslmode=disable",
			c.User, c.Password, c.Host, c.Port,
		)
	default:
		return fmt.Sprintf(
			"%s:%s@tcp(%s:%s)/?charset=utf8mb4&parseTime=true&loc=UTC&timeout=5s",
			c.User, c.Password, c.Host, c.Port,
		)
	}
}

func (c dbConfig) sqlDriverName() string {
	switch c.Driver {
	case driverPostgres:
		return "pgx"
	case driverSQLite:
		return "sqlite"
	default:
		return "mysql"
	}
}

// ensureDatabase создаёт базу данных, если она ещё не существует.
// Для MariaDB и Postgres требует прав на создание базы; если их нет, просто
// убедитесь, что база создана заранее, и эта функция молча пройдёт мимо.
// Для SQLite — no-op, файл базы создаёт сам драйвер при первом подключении.
func ensureDatabase(cfg dbConfig) error {
	if cfg.Driver == driverSQLite {
		if dir := filepath.Dir(cfg.SQLitePath); dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return fmt.Errorf("создание каталога для файла БД %q: %w", dir, err)
			}
		}
		return nil
	}
	if !identifierRE.MatchString(cfg.Name) {
		return fmt.Errorf("недопустимое имя базы данных: %q", cfg.Name)
	}

	root, err := sql.Open(cfg.sqlDriverName(), cfg.serverDSN())
	if err != nil {
		return fmt.Errorf("подключение к серверу БД: %w", err)
	}
	defer root.Close()

	if err := root.Ping(); err != nil {
		return fmt.Errorf("не удалось достучаться до сервера БД: %w", err)
	}

	switch cfg.Driver {
	case driverPostgres:
		var exists bool
		if err := root.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, cfg.Name).Scan(&exists); err != nil {
			return fmt.Errorf("проверка существования базы данных %q: %w", cfg.Name, err)
		}
		if exists {
			return nil
		}
		// CREATE DATABASE не поддерживает IF NOT EXISTS и не принимает
		// параметризованное имя базы — но оно уже проверено identifierRE.
		if _, err := root.Exec(fmt.Sprintf(`CREATE DATABASE "%s"`, cfg.Name)); err != nil {
			return fmt.Errorf("создание базы данных %q: %w", cfg.Name, err)
		}
	default:
		stmt := fmt.Sprintf(
			"CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci",
			cfg.Name,
		)
		if _, err := root.Exec(stmt); err != nil {
			return fmt.Errorf("создание базы данных %q: %w", cfg.Name, err)
		}
	}
	return nil
}

// openDB открывает пул соединений с конкретной базой данных и оборачивает
// его в appDB (см. выше).
func openDB(cfg dbConfig) (*appDB, error) {
	raw, err := sql.Open(cfg.sqlDriverName(), cfg.dsn())
	if err != nil {
		return nil, fmt.Errorf("открытие БД: %w", err)
	}
	raw.SetMaxOpenConns(10)
	raw.SetMaxIdleConns(5)
	raw.SetConnMaxLifetime(5 * time.Minute)
	if cfg.Driver == driverSQLite {
		// SQLite не терпит параллельных писателей — несколько одновременных
		// соединений только приводят к "database is locked" под нагрузкой.
		raw.SetMaxOpenConns(1)
	}

	if err := raw.Ping(); err != nil {
		return nil, fmt.Errorf("проверка соединения с БД: %w", err)
	}
	return &appDB{DB: raw, driver: cfg.Driver}, nil
}

// migrate создаёт таблицы, если их ещё нет. Простая идемпотентная миграция —
// для проекта такого размера отдельный инструмент миграций избыточен.
func migrate(db *appDB) error {
	switch db.driver {
	case driverPostgres:
		return migratePostgres(db)
	case driverSQLite:
		return migrateSQLite(db)
	default:
		return migrateMariaDB(db)
	}
}

func execAll(db *appDB, stmts []string) error {
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("миграция (%q): %w", stmt, err)
		}
	}
	return nil
}

// migrateMariaDB — оригинальная схема. Индексы объявлены прямо внутри
// CREATE TABLE (а не отдельным CREATE INDEX), чтобы не зависеть от версии
// MariaDB, где "IF NOT EXISTS" для индексов поддерживается не везде.
func migrateMariaDB(db *appDB) error {
	return execAll(db, []string{
		`CREATE TABLE IF NOT EXISTS users (
			id            INTEGER UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			username      VARCHAR(255) NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			settings      JSON NOT NULL CHECK(JSON_VALID(settings)),
			created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			UNIQUE KEY uniq_users_username (username)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,

		`CREATE TABLE IF NOT EXISTS sessions (
			token      VARCHAR(64) NOT NULL PRIMARY KEY,
			user_id    INTEGER UNSIGNED NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			KEY idx_sessions_user (user_id),
			CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,

		`CREATE TABLE IF NOT EXISTS locations (
			id         INTEGER UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			title      VARCHAR(255) NOT NULL,
			color      VARCHAR(16) NOT NULL,
			parent_id  INTEGER UNSIGNED NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			KEY idx_locations_parent (parent_id),
			CONSTRAINT fk_locations_parent FOREIGN KEY (parent_id) REFERENCES locations (id) ON DELETE RESTRICT
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,

		`CREATE TABLE IF NOT EXISTS items (
			id          INTEGER UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			title       VARCHAR(255) NOT NULL,
			location_id INTEGER UNSIGNED NOT NULL,
			notes       TEXT NOT NULL,
			created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			KEY idx_items_location (location_id),
			KEY idx_items_updated (updated_at),
			CONSTRAINT fk_items_location FOREIGN KEY (location_id) REFERENCES locations (id) ON DELETE RESTRICT
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,

		// url — ссылка на файл фотографии (см. imagestore.go), не сами байты:
		// файлы лежат на диске в web/files, в БД только путь к ним.
		`CREATE TABLE IF NOT EXISTS item_images (
			id          INTEGER UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
			item_id  INTEGER UNSIGNED NOT NULL,
			url      VARCHAR(255) NOT NULL,
			position INTEGER NOT NULL DEFAULT 0,
			KEY idx_item_images_item (item_id),
			CONSTRAINT fk_item_images_item FOREIGN KEY (item_id) REFERENCES items (id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,
	})
}

// migratePostgres — та же схема для Postgres: SERIAL вместо AUTO_INCREMENT,
// JSONB вместо JSON+CHECK(JSON_VALID), индексы отдельными CREATE INDEX
// (Postgres не понимает "KEY name (col)" внутри CREATE TABLE), без
// ENGINE/CHARSET (не применимо) и без ON UPDATE CURRENT_TIMESTAMP (в
// Postgres это делается триггером, а колонка updated_at в этом проекте и
// так везде проставляется явно из приложения — см. handlers.go).
func migratePostgres(db *appDB) error {
	return execAll(db, []string{
		`CREATE TABLE IF NOT EXISTS users (
			id            SERIAL PRIMARY KEY,
			username      VARCHAR(255) NOT NULL UNIQUE,
			password_hash VARCHAR(255) NOT NULL,
			settings      JSONB NOT NULL,
			created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,

		`CREATE TABLE IF NOT EXISTS sessions (
			token      VARCHAR(64) PRIMARY KEY,
			user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions (user_id);`,

		`CREATE TABLE IF NOT EXISTS locations (
			id         SERIAL PRIMARY KEY,
			title      VARCHAR(255) NOT NULL,
			color      VARCHAR(16) NOT NULL,
			parent_id  INTEGER NULL REFERENCES locations (id) ON DELETE RESTRICT,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_locations_parent ON locations (parent_id);`,

		`CREATE TABLE IF NOT EXISTS items (
			id          SERIAL PRIMARY KEY,
			title       VARCHAR(255) NOT NULL,
			location_id INTEGER NOT NULL REFERENCES locations (id) ON DELETE RESTRICT,
			notes       TEXT NOT NULL,
			created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_items_location ON items (location_id);`,
		`CREATE INDEX IF NOT EXISTS idx_items_updated ON items (updated_at);`,

		// url — ссылка на файл фотографии (см. imagestore.go), не сами байты:
		// файлы лежат на диске в web/files, в БД только путь к ним.
		`CREATE TABLE IF NOT EXISTS item_images (
			id       SERIAL PRIMARY KEY,
			item_id  INTEGER NOT NULL REFERENCES items (id) ON DELETE CASCADE,
			url      VARCHAR(255) NOT NULL,
			position INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE INDEX IF NOT EXISTS idx_item_images_item ON item_images (item_id);`,
	})
}

// migrateSQLite — та же схема для SQLite: INTEGER PRIMARY KEY AUTOINCREMENT
// вместо AUTO_INCREMENT, json_valid() вместо JSON_VALID(), индексы отдельными
// CREATE INDEX (SQLite не понимает "KEY name (col)" внутри CREATE TABLE).
// Включение проверки внешних ключей — через DSN, см. dsn().
func migrateSQLite(db *appDB) error {
	return execAll(db, []string{
		`CREATE TABLE IF NOT EXISTS users (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			username      VARCHAR(255) NOT NULL UNIQUE,
			password_hash VARCHAR(255) NOT NULL,
			settings      TEXT NOT NULL CHECK(json_valid(settings)),
			created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,

		`CREATE TABLE IF NOT EXISTS sessions (
			token      VARCHAR(64) PRIMARY KEY,
			user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			expires_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions (user_id);`,

		`CREATE TABLE IF NOT EXISTS locations (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			title      VARCHAR(255) NOT NULL,
			color      VARCHAR(16) NOT NULL,
			parent_id  INTEGER NULL REFERENCES locations (id) ON DELETE RESTRICT,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_locations_parent ON locations (parent_id);`,

		`CREATE TABLE IF NOT EXISTS items (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			title       VARCHAR(255) NOT NULL,
			location_id INTEGER NOT NULL REFERENCES locations (id) ON DELETE RESTRICT,
			notes       TEXT NOT NULL,
			created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_items_location ON items (location_id);`,
		`CREATE INDEX IF NOT EXISTS idx_items_updated ON items (updated_at);`,

		// url — ссылка на файл фотографии (см. imagestore.go), не сами байты:
		// файлы лежат на диске в web/files, в БД только путь к ним.
		`CREATE TABLE IF NOT EXISTS item_images (
			id       INTEGER PRIMARY KEY AUTOINCREMENT,
			item_id  INTEGER NOT NULL REFERENCES items (id) ON DELETE CASCADE,
			url      VARCHAR(255) NOT NULL,
			position INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE INDEX IF NOT EXISTS idx_item_images_item ON item_images (item_id);`,
	})
}

// seedDemoUser создаёт демо-пользователя (по умолчанию user / pass,
// настраивается через DEMO_USERNAME / DEMO_PASSWORD),
// если таблица users ещё пуста — чтобы сразу было чем войти.
func seedDemoUser(db *appDB, cfg demoUserConfig) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	hash, err := hashPassword(cfg.Password)
	if err != nil {
		return err
	}
	settings, err := json.Marshal(defaultUserSettings())
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO users (username, password_hash, settings) VALUES (?, ?, ?)`,
		cfg.Username, hash, settings,
	)
	return err
}
