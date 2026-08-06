package sqlite

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
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		username      VARCHAR(255) NOT NULL UNIQUE,
		password_hash VARCHAR(255) NOT NULL,
		settings      TEXT NOT NULL CHECK(json_valid(settings)),
		created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`

	sessionsTableStmt = `CREATE TABLE IF NOT EXISTS sessions (
		token      VARCHAR(64) PRIMARY KEY,
		user_id    INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		expires_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`
	sessionsUserIndexStmt = `CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions (user_id);`

	locationsTableStmt = `CREATE TABLE IF NOT EXISTS locations (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		title      VARCHAR(255) NOT NULL,
		color      VARCHAR(16) NOT NULL,
		parent_id  INTEGER NULL REFERENCES locations (id) ON DELETE RESTRICT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`
	locationsParentIndexStmt = `CREATE INDEX IF NOT EXISTS idx_locations_parent ON locations (parent_id);`

	itemsTableStmt = `CREATE TABLE IF NOT EXISTS items (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		title       VARCHAR(255) NOT NULL,
		location_id INTEGER NOT NULL REFERENCES locations (id) ON DELETE RESTRICT,
		notes       TEXT NOT NULL,
		created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);`
	itemsLocationIndexStmt = `CREATE INDEX IF NOT EXISTS idx_items_location ON items (location_id);`
	itemsUpdatedIndexStmt  = `CREATE INDEX IF NOT EXISTS idx_items_updated ON items (updated_at);`

	itemImagesTableStmt = `CREATE TABLE IF NOT EXISTS item_images (
		id       INTEGER PRIMARY KEY AUTOINCREMENT,
		item_id  INTEGER NOT NULL REFERENCES items (id) ON DELETE CASCADE,
		url      VARCHAR(255) NOT NULL,
		position INTEGER NOT NULL DEFAULT 0
	);`
	itemImagesItemIndexStmt = `CREATE INDEX IF NOT EXISTS idx_item_images_item ON item_images (item_id);`
)

// TestMigrateExecutesTheSchemaStatements checks that Migrate issues exactly the CREATE TABLE and
// CREATE INDEX statements the schema is built from, in order — SQLite, unlike MySQL, can't declare an
// index inline inside CREATE TABLE, so each one is its own statement. Without a real database file to
// inspect afterward, this is what "creates the schema" can mean under a mock: it can't confirm the SQL
// is valid or that the resulting tables behave as declared — that's what an integration test against a
// real SQLite file would be for — only that the adapter attempted to create the right objects.
func TestMigrateExecutesTheSchemaStatements(t *testing.T) {
	s, mock := newMock(t)

	for _, stmt := range []string{
		usersTableStmt,
		sessionsTableStmt, sessionsUserIndexStmt,
		locationsTableStmt, locationsParentIndexStmt,
		itemsTableStmt, itemsLocationIndexStmt, itemsUpdatedIndexStmt,
		itemImagesTableStmt, itemImagesItemIndexStmt,
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
