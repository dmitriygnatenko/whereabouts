package postgres

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
// This mirrors the MySQL schema: SERIAL instead of AUTO_INCREMENT, JSONB instead of
// JSON+CHECK(JSON_VALID), indexes as separate CREATE INDEX statements (Postgres doesn't understand
// "KEY name (col)" inside CREATE TABLE), no ENGINE/CHARSET (not applicable), and no ON UPDATE
// CURRENT_TIMESTAMP (Postgres would need a trigger for that, and updated_at is always set
// explicitly from the application anyway — see the item repository).
func Migrate(ctx context.Context, db *Storage) error {
	return execAll(ctx, db, []string{
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

		`CREATE TABLE IF NOT EXISTS item_images (
			id       SERIAL PRIMARY KEY,
			item_id  INTEGER NOT NULL REFERENCES items (id) ON DELETE CASCADE,
			url      VARCHAR(255) NOT NULL,
			position INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE INDEX IF NOT EXISTS idx_item_images_item ON item_images (item_id);`,
	})
}
