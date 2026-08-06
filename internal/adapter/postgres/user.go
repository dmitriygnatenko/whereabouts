package postgres

import (
	"context"
	"database/sql"

	"wherewhat/internal/storage/model"
)

// FindUserByUsername looks up a user by their username.
func (s *Storage) FindUserByUsername(ctx context.Context, username string) (model.User, error) {
	return scanUser(s.DB.QueryRowContext(ctx,
		`SELECT id, username, settings, password_hash FROM users WHERE username = $1`, username,
	))
}

// FindUserByID looks up a user by id.
func (s *Storage) FindUserByID(ctx context.Context, id uint64) (model.User, error) {
	return scanUser(s.DB.QueryRowContext(ctx,
		`SELECT id, username, settings, password_hash FROM users WHERE id = $1`, id,
	))
}

// CreateUser inserts a user row and returns its new id.
func (s *Storage) CreateUser(
	ctx context.Context, username, passwordHash string, settings model.UserSettings,
) (uint64, error) {
	id, err := s.insertReturningID(ctx,
		`INSERT INTO users (username, password_hash, settings) VALUES ($1, $2, $3) RETURNING id`,
		username, passwordHash, settings,
	)
	if err != nil {
		return 0, wrapUnique(err)
	}

	return id, nil
}

// UpdateUsername renames a user.
func (s *Storage) UpdateUsername(ctx context.Context, id uint64, username string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET username = $1 WHERE id = $2`, username, id)

	return wrapUnique(err)
}

// UpdateUserPasswordHash overwrites a user's stored password hash.
func (s *Storage) UpdateUserPasswordHash(ctx context.Context, id uint64, hash string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE users SET password_hash = $1 WHERE id = $2`, hash, id)

	return err
}

// UpdateUserLanguage sets the language field inside the settings JSON column.
func (s *Storage) UpdateUserLanguage(ctx context.Context, id uint64, lang string) error {
	_, err := s.DB.ExecContext(ctx,
		`UPDATE users SET settings = jsonb_set(settings, '{language}', to_jsonb($1::text)) WHERE id = $2`,
		lang, id,
	)

	return err
}

// UpdateUserLocationFilterDepth sets the locationFilterDepth field inside the settings JSON column.
func (s *Storage) UpdateUserLocationFilterDepth(ctx context.Context, id uint64, depth int) error {
	_, err := s.DB.ExecContext(ctx,
		`UPDATE users SET settings = jsonb_set(settings, '{locationFilterDepth}', to_jsonb($1::int)) WHERE id = $2`,
		depth, id,
	)

	return err
}

// CountUsers returns the total number of users.
func (s *Storage) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)

	return n, err
}

// scanUser reads the user column list, in the order every user query above selects it.
func scanUser(row *sql.Row) (model.User, error) {
	var m model.User
	if err := row.Scan(&m.ID, &m.Username, &m.Settings, &m.PasswordHash); err != nil {
		return model.User{}, err
	}

	return m, nil
}
