package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// dbConfig собирает параметры подключения к MariaDB из окружения.
type dbConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	Name     string
}

var identifierRE = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// dsn возвращает строку подключения к конкретной базе данных.
func (c dbConfig) dsn() string {
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=UTC&time_zone=%%27%%2B00%%3A00%%27&timeout=5s",
		c.User, c.Password, c.Host, c.Port, c.Name,
	)
}

// serverDSN — подключение к самому серверу MariaDB без конкретной базы,
// нужно только для того, чтобы при первом запуске создать БД, если её ещё нет.
func (c dbConfig) serverDSN() string {
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/?charset=utf8mb4&parseTime=true&loc=UTC&timeout=5s",
		c.User, c.Password, c.Host, c.Port,
	)
}

// ensureDatabase создаёт базу данных, если она ещё не существует.
// Требует, чтобы у пользователя было право CREATE DATABASE; если его нет,
// просто убедитесь, что база создана заранее, и эта функция молча пройдёт мимо.
func ensureDatabase(cfg dbConfig) error {
	if !identifierRE.MatchString(cfg.Name) {
		return fmt.Errorf("недопустимое имя базы данных: %q", cfg.Name)
	}

	root, err := sql.Open("mysql", cfg.serverDSN())
	if err != nil {
		return fmt.Errorf("подключение к серверу MariaDB: %w", err)
	}
	defer root.Close()

	if err := root.Ping(); err != nil {
		return fmt.Errorf("не удалось достучаться до MariaDB: %w", err)
	}

	stmt := fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci",
		cfg.Name,
	)
	if _, err := root.Exec(stmt); err != nil {
		return fmt.Errorf("создание базы данных %q: %w", cfg.Name, err)
	}
	return nil
}

// openDB открывает пул соединений с конкретной базой данных MariaDB.
func openDB(cfg dbConfig) (*sql.DB, error) {
	db, err := sql.Open("mysql", cfg.dsn())
	if err != nil {
		return nil, fmt.Errorf("открытие БД: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("проверка соединения с БД: %w", err)
	}
	return db, nil
}

// migrate создаёт таблицы, если их ещё нет. Простая идемпотентная миграция —
// для проекта такого размера отдельный инструмент миграций избыточен.
// Индексы объявлены прямо внутри CREATE TABLE (а не отдельным CREATE INDEX),
// чтобы не зависеть от версии MariaDB, где "IF NOT EXISTS" для индексов
// поддерживается не везде.
func migrate(db *sql.DB) error {
	stmts := []string{
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
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("миграция (%q): %w", stmt, err)
		}
	}
	return nil
}

// seedDemoUser создаёт демо-пользователя (demo / demo1234),
// если таблица users ещё пуста — чтобы сразу было чем войти.
func seedDemoUser(db *sql.DB) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	hash, err := hashPassword("pass")
	if err != nil {
		return err
	}
	settings, err := json.Marshal(defaultUserSettings())
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO users (username, password_hash, settings) VALUES (?, ?, ?)`,
		"user", hash, settings,
	)
	return err
}
