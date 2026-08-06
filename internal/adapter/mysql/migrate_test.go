package mysql

import (
	"context"
	"errors"
	"strings"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/brianvoe/gofakeit/v7"
)

// The statements below are copied verbatim from Migrate in migrate.go — not test fixtures, but the
// actual production contract this file pins down. sqlmock's QueryMatcherEqual strips whitespace
// before comparing (see query.go in DATA-DOG/go-sqlmock), so the indentation here doesn't have to
// match the source exactly, only the SQL content.
const (
	usersTableStmt = `CREATE TABLE IF NOT EXISTS users (
		id            INTEGER UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
		username      VARCHAR(255) NOT NULL,
		password_hash VARCHAR(255) NOT NULL,
		settings      JSON NOT NULL CHECK(JSON_VALID(settings)),
		created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		UNIQUE KEY uniq_users_username (username)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`

	sessionsTableStmt = `CREATE TABLE IF NOT EXISTS sessions (
		token      VARCHAR(64) NOT NULL PRIMARY KEY,
		user_id    INTEGER UNSIGNED NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		KEY idx_sessions_user (user_id),
		CONSTRAINT fk_sessions_user FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`

	locationsTableStmt = `CREATE TABLE IF NOT EXISTS locations (
		id         INTEGER UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
		title      VARCHAR(255) NOT NULL,
		color      VARCHAR(16) NOT NULL,
		parent_id  INTEGER UNSIGNED NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		KEY idx_locations_parent (parent_id),
		CONSTRAINT fk_locations_parent FOREIGN KEY (parent_id) REFERENCES locations (id) ON DELETE RESTRICT
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`

	itemsTableStmt = `CREATE TABLE IF NOT EXISTS items (
		id          INTEGER UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
		title       VARCHAR(255) NOT NULL,
		location_id INTEGER UNSIGNED NOT NULL,
		notes       TEXT NOT NULL,
		created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		KEY idx_items_location (location_id),
		KEY idx_items_updated (updated_at),
		CONSTRAINT fk_items_location FOREIGN KEY (location_id) REFERENCES locations (id) ON DELETE RESTRICT
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`

	itemImagesTableStmt = `CREATE TABLE IF NOT EXISTS item_images (
		id          INTEGER UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
		item_id  INTEGER UNSIGNED NOT NULL,
		url      VARCHAR(255) NOT NULL,
		position INTEGER NOT NULL DEFAULT 0,
		KEY idx_item_images_item (item_id),
		CONSTRAINT fk_item_images_item FOREIGN KEY (item_id) REFERENCES items (id) ON DELETE CASCADE
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`
)

// TestMigrateExecutesTheSchemaStatements checks that Migrate issues exactly the five CREATE TABLE
// statements the schema is built from, in order. Without a real server to inspect afterward (see
// storage_test.go), this is what "creates the schema" can mean under a mock: it can't confirm the SQL
// is valid or that the resulting tables behave as declared — that's what an integration test against a
// real MySQL/MariaDB would be for — only that the adapter attempted to create the right objects.
func TestMigrateExecutesTheSchemaStatements(t *testing.T) {
	s, mock := newMock(t)

	for _, stmt := range []string{
		usersTableStmt, sessionsTableStmt, locationsTableStmt, itemsTableStmt, itemImagesTableStmt,
	} {
		mock.ExpectExec(stmt).WillReturnResult(sqlmock.NewResult(0, 0))
	}

	if err := Migrate(context.Background(), s); err != nil {
		t.Fatalf("Migrate() error = %v", err)
	}
}

// TestExecAll covers the loop the migration is built on: statements run in order, and the first
// failure stops the run without attempting the rest. The statement text itself is opaque random data
// here — execAll never inspects it, only passes it to ExecContext — so gofakeit stands in for real SQL
// without weakening the test.
func TestExecAll(t *testing.T) {
	tests := []struct {
		name    string
		stmts   []string
		mock    func(mock sqlmock.Sqlmock, stmts []string)
		wantErr bool
	}{
		{
			name: "no statements is not an error",
			mock: func(mock sqlmock.Sqlmock, stmts []string) {},
		},
		{
			name:  "statements run in order",
			stmts: []string{gofakeit.Sentence(), gofakeit.Sentence(), gofakeit.Sentence()},
			mock: func(mock sqlmock.Sqlmock, stmts []string) {
				for _, stmt := range stmts {
					mock.ExpectExec(stmt).WillReturnResult(sqlmock.NewResult(0, 0))
				}
			},
		},
		{
			name:  "the first failure stops the run",
			stmts: []string{gofakeit.Sentence(), gofakeit.Sentence(), gofakeit.Sentence()},
			mock: func(mock sqlmock.Sqlmock, stmts []string) {
				mock.ExpectExec(stmts[0]).WillReturnResult(sqlmock.NewResult(0, 0))
				mock.ExpectExec(stmts[1]).WillReturnError(errStub)
				// stmts[2] is deliberately never registered: if execAll called it anyway, sqlmock would
				// fail the call as unexpected.
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, mock := newMock(t)
			tt.mock(mock, tt.stmts)

			err := execAll(context.Background(), s, tt.stmts)
			if tt.wantErr {
				if !errors.Is(err, errStub) {
					t.Fatalf("execAll() error = %v, want %v", err, errStub)
				}

				if !strings.Contains(err.Error(), "migration") {
					t.Fatalf("execAll() error = %v, want it to mention the failing migration", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("execAll() error = %v", err)
			}
		})
	}
}
