package mysql

import (
	"context"
	"fmt"
)

// execAll runs each statement in order, stopping at the first error.
func execAll(ctx context.Context, db *Storage, stmts []string) error {
	for _, stmt := range stmts {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migration (%q): %w", stmt, err)
		}
	}

	return nil
}

// Migrate creates tables if they don't already exist — a simple idempotent migration, which is all
// a project this size needs; no separate migration tool.
//
// Indexes are declared inline inside CREATE TABLE (rather than a separate CREATE INDEX) to avoid
// depending on the MySQL/MariaDB version, where "IF NOT EXISTS" for indexes isn't universally
// supported.
func Migrate(ctx context.Context, db *Storage) error {
	return execAll(ctx, db, []string{
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
