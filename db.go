package main

import (
	"database/sql"
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
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=UTC&timeout=5s",
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
	if err := renameEmailColumnToUsername(db); err != nil {
		return fmt.Errorf("миграция email -> username: %w", err)
	}
	if err := renameDataURLColumnToURL(db); err != nil {
		return fmt.Errorf("миграция data_url -> url: %w", err)
	}

	stmts := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id            VARCHAR(64) NOT NULL PRIMARY KEY,
			name          VARCHAR(255) NOT NULL,
			username      VARCHAR(255) NOT NULL,
			password_hash VARCHAR(255) NOT NULL,
			language      VARCHAR(8) NOT NULL DEFAULT 'en',
			location_filter_depth INT NOT NULL DEFAULT 0,
			created_at    VARCHAR(40) NOT NULL,
			UNIQUE KEY uniq_users_username (username)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,

		// Для баз, созданных до появления колонки language — добавляем её отдельно
		// (IF NOT EXISTS поддерживается MariaDB/MySQL 8+, идемпотентно при рестартах).
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS language VARCHAR(8) NOT NULL DEFAULT 'en';`,

		// location_filter_depth — сколько уровней вложенности мест показывать
		// в чипах-фильтрах на вкладке "Вещи". 0 = без ограничения (показывать все).
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS location_filter_depth INT NOT NULL DEFAULT 0;`,

		`CREATE TABLE IF NOT EXISTS sessions (
			token      VARCHAR(64) NOT NULL PRIMARY KEY,
			user_id    VARCHAR(64) NOT NULL,
			created_at VARCHAR(40) NOT NULL,
			expires_at VARCHAR(40) NOT NULL,
			KEY idx_sessions_user (user_id),
			CONSTRAINT fk_sessions_user FOREIGN KEY (user_id)
				REFERENCES users (id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,

		`CREATE TABLE IF NOT EXISTS locations (
			id         VARCHAR(64) NOT NULL PRIMARY KEY,
			name       VARCHAR(255) NOT NULL,
			color      VARCHAR(16) NOT NULL,
			parent_id  VARCHAR(64) NULL,
			created_at VARCHAR(40) NOT NULL,
			KEY idx_locations_parent (parent_id),
			CONSTRAINT fk_locations_parent FOREIGN KEY (parent_id)
				REFERENCES locations (id) ON DELETE RESTRICT
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,

		`CREATE TABLE IF NOT EXISTS items (
			id          VARCHAR(64) NOT NULL PRIMARY KEY,
			name        VARCHAR(255) NOT NULL,
			location_id VARCHAR(64) NOT NULL,
			notes       TEXT NOT NULL,
			created_at  VARCHAR(40) NOT NULL,
			updated_at  VARCHAR(40) NOT NULL,
			KEY idx_items_location (location_id),
			KEY idx_items_updated (updated_at),
			CONSTRAINT fk_items_location FOREIGN KEY (location_id)
				REFERENCES locations (id) ON DELETE RESTRICT
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,

		// url — ссылка на файл фотографии (см. imagestore.go), не сами байты:
		// файлы лежат на диске в web/files, в БД только путь к ним.
		`CREATE TABLE IF NOT EXISTS item_images (
			id       BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
			item_id  VARCHAR(64) NOT NULL,
			url      MEDIUMTEXT NOT NULL,
			position INT NOT NULL DEFAULT 0,
			KEY idx_item_images_item (item_id),
			CONSTRAINT fk_item_images_item FOREIGN KEY (item_id)
				REFERENCES items (id) ON DELETE CASCADE
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("миграция (%q): %w", stmt, err)
		}
	}
	return nil
}

// renameEmailColumnToUsername переименовывает колонку email в username (вместе
// с её уникальным индексом) на базах, поднятых до перехода на логин по имени
// пользователя. На новых базах колонки email не существует, и функция ничего
// не делает — CREATE TABLE ниже сразу создаёт таблицу с колонкой username.
func renameEmailColumnToUsername(db *sql.DB) error {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM information_schema.COLUMNS
		 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'users' AND COLUMN_NAME = 'email'`,
	).Scan(&count)
	if err != nil || count == 0 {
		return err
	}
	if _, err := db.Exec(`ALTER TABLE users CHANGE COLUMN email username VARCHAR(255) NOT NULL`); err != nil {
		return err
	}
	if _, err := db.Exec(`ALTER TABLE users DROP INDEX uniq_users_email, ADD UNIQUE KEY uniq_users_username (username)`); err != nil {
		return err
	}
	return nil
}

// renameDataURLColumnToURL переименовывает item_images.data_url в url на
// базах, поднятых до перехода на файловое хранилище фотографий (см.
// imagestore.go) — раньше в этой колонке лежал целиком data:-URL, теперь
// только ссылка на файл. На новых базах колонки data_url не существует, и
// функция ничего не делает — CREATE TABLE выше сразу создаёт колонку url.
func renameDataURLColumnToURL(db *sql.DB) error {
	var count int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM information_schema.COLUMNS
		 WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'item_images' AND COLUMN_NAME = 'data_url'`,
	).Scan(&count)
	if err != nil || count == 0 {
		return err
	}
	_, err = db.Exec(`ALTER TABLE item_images CHANGE COLUMN data_url url MEDIUMTEXT NOT NULL`)
	return err
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
	_, err = db.Exec(
		`INSERT INTO users (id, name, username, password_hash, created_at) VALUES (?, ?, ?, ?, ?)`,
		newID("user-"), "Демо Пользователь", "user", hash, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

// seedIfEmpty наполняет пустую базу демо-данными, чтобы приложение сразу
// было с чем показать — та же иерархия мест и вещей, что раньше жила
// в мок-бэкенде фронтенда.
func seedIfEmpty(db *sql.DB) error {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM locations`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	ago := func(days int) string { return now.AddDate(0, 0, -days).Format(time.RFC3339) }
	strPtr := func(s string) *string { return &s }

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	locations := []Location{
		{ID: "loc-hall", Name: "Прихожая", Color: "#3D6B63", ParentID: nil},
		{ID: "loc-kitchen", Name: "Кухня", Color: "#D98E2B", ParentID: nil},
		{ID: "loc-bedroom", Name: "Спальня", Color: "#8E5A9E", ParentID: nil},
		{ID: "loc-storage", Name: "Кладовая", Color: "#B5453A", ParentID: nil},
		{ID: "loc-storage-top", Name: "Верхняя полка", Color: "#B5453A", ParentID: strPtr("loc-storage")},
		{ID: "loc-garage", Name: "Гараж", Color: "#4A6FA5", ParentID: nil},
		{ID: "loc-garage-shelf", Name: "Полка 2", Color: "#4A6FA5", ParentID: strPtr("loc-garage")},
		{ID: "loc-garage-box", Name: "Коробка с проводами", Color: "#4A6FA5", ParentID: strPtr("loc-garage-shelf")},
	}
	for _, l := range locations {
		if _, err := tx.Exec(
			`INSERT INTO locations (id, name, color, parent_id, created_at) VALUES (?, ?, ?, ?, ?)`,
			l.ID, l.Name, l.Color, l.ParentID, nowStr,
		); err != nil {
			return err
		}
	}

	type seedItem struct {
		id, name, locationID, notes string
		updatedAt                   string
	}
	items := []seedItem{
		{newID(""), "Паспорт", "loc-hall", "В верхнем ящике комода, синяя папка", ago(1)},
		{newID(""), "Зарядка для ноутбука", "loc-bedroom", "В тумбе у кровати", ago(3)},
		{newID(""), "Зимние шины", "loc-garage-shelf", "", ago(40)},
		{newID(""), "Изолента", "loc-garage-box", "Синяя и чёрная катушки", ago(12)},
		{newID(""), "Аптечка", "loc-storage-top", "Справа, рядом с фонариком", ago(7)},
		{newID(""), "Запасные ключи", "loc-kitchen", "Крючок у холодильника", ago(0)},
	}
	for _, it := range items {
		if _, err := tx.Exec(
			`INSERT INTO items (id, name, location_id, notes, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			it.id, it.name, it.locationID, it.notes, it.updatedAt, it.updatedAt,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}
