package mysql

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/brianvoe/gofakeit/v7"

	"wherewhat/internal/storage/model"
)

// TestCreateSession covers the insert behind every login, plus the two rows the schema won't accept:
// a session for a user that doesn't exist, and a token already in use — both simulated as driver
// errors rather than enforced by a real server.
func TestCreateSession(t *testing.T) {
	query := `INSERT INTO sessions (token, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`

	token := fakeToken()
	userID := fakeID()
	createdAt := fakeTime()
	expiresAt := fakeTime()

	tests := []struct {
		name    string
		mock    func(mock sqlmock.Sqlmock)
		wantErr bool
	}{
		{
			name: "stores the session",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(query).WithArgs(token, userID, createdAt, expiresAt).WillReturnResult(sqlmock.NewResult(0, 1))
			},
		},
		{
			name: "an unknown user surfaces the driver's foreign key error",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(query).WithArgs(token, userID, createdAt, expiresAt).WillReturnError(mysqlErr(mysqlNoReferencedRow))
			},
			wantErr: true,
		},
		{
			name: "a token already in use surfaces the driver's duplicate entry error",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(query).WithArgs(token, userID, createdAt, expiresAt).WillReturnError(mysqlErr(mysqlDuplicateEntry))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, mock := newMock(t)
			tt.mock(mock)

			session := model.Session{Token: token, UserID: userID, ExpiresAt: model.NewTime(expiresAt)}

			err := s.CreateSession(context.Background(), session, createdAt)
			if tt.wantErr {
				if err == nil {
					t.Fatal("CreateSession() error = nil, want the driver's error propagated")
				}

				return
			}

			if err != nil {
				t.Fatalf("CreateSession() error = %v", err)
			}
		})
	}
}

// TestFindSessionByToken covers the lookup every authenticated request starts with. The expiry has to
// survive the round-trip intact — the session is checked against it, not against a stored flag.
func TestFindSessionByToken(t *testing.T) {
	query := `SELECT user_id, expires_at FROM sessions WHERE token = ?`
	token := fakeToken()
	userID := fakeID()
	expiresAt := fakeTime()

	tests := []struct {
		name    string
		mock    func(mock sqlmock.Sqlmock)
		wantErr error
	}{
		{
			name: "finds the session",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery(query).WithArgs(token).WillReturnRows(
					sqlmock.NewRows([]string{"user_id", "expires_at"}).AddRow(userID, expiresAt),
				)
			},
		},
		{
			name:    "an unknown token is sql.ErrNoRows",
			mock:    func(mock sqlmock.Sqlmock) { mock.ExpectQuery(query).WithArgs(token).WillReturnError(sql.ErrNoRows) },
			wantErr: sql.ErrNoRows,
		},
		{
			name:    "a driver error is propagated",
			mock:    func(mock sqlmock.Sqlmock) { mock.ExpectQuery(query).WithArgs(token).WillReturnError(errStub) },
			wantErr: errStub,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, mock := newMock(t)
			tt.mock(mock)

			got, err := s.FindSessionByToken(context.Background(), token)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("FindSessionByToken() error = %v, want %v", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("FindSessionByToken() error = %v", err)
			}

			if got.Token != token || got.UserID != userID || !got.ExpiresAt.Equal(expiresAt) {
				t.Fatalf("FindSessionByToken() = %+v, want user %d expiring at %v", got, userID, expiresAt)
			}
		})
	}
}

// TestDeleteSession covers logout, where a token that isn't there is the same outcome as one that
// was: the session is gone either way, so it isn't reported as an error.
func TestDeleteSession(t *testing.T) {
	query := `DELETE FROM sessions WHERE token = ?`
	token := fakeToken()

	tests := []struct {
		name    string
		mock    func(mock sqlmock.Sqlmock)
		wantErr bool
	}{
		{
			name: "removes the session",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(query).WithArgs(token).WillReturnResult(sqlmock.NewResult(0, 1))
			},
		},
		{
			name: "an unknown token is not an error",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(query).WithArgs(token).WillReturnResult(sqlmock.NewResult(0, 0))
			},
		},
		{
			name:    "a driver error is propagated",
			mock:    func(mock sqlmock.Sqlmock) { mock.ExpectExec(query).WithArgs(token).WillReturnError(errStub) },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, mock := newMock(t)
			tt.mock(mock)

			err := s.DeleteSession(context.Background(), token)
			if tt.wantErr {
				if !errors.Is(err, errStub) {
					t.Fatalf("DeleteSession() error = %v, want %v", err, errStub)
				}

				return
			}

			if err != nil {
				t.Fatalf("DeleteSession() error = %v", err)
			}
		})
	}
}

// TestDeleteExpiredSessions covers the cleanup sweep: the row count RowsAffected reports is what gets
// logged. Whether the "<" comparison itself is strict against a session expiring exactly now is the
// schema/server's job to get right, not something a mock can verify — the query text this pins down
// (WHERE expires_at < ?) is the part that belongs to this adapter.
func TestDeleteExpiredSessions(t *testing.T) {
	query := `DELETE FROM sessions WHERE expires_at < ?`
	now := fakeTime()
	wantDeleted := int64(gofakeit.Number(0, 100))

	tests := []struct {
		name    string
		mock    func(mock sqlmock.Sqlmock)
		want    int64
		wantErr bool
	}{
		{
			name: "reports how many sessions were swept",
			mock: func(mock sqlmock.Sqlmock) {
				mock.ExpectExec(query).WithArgs(now).WillReturnResult(sqlmock.NewResult(0, wantDeleted))
			},
			want: wantDeleted,
		},
		{
			name:    "a driver error is propagated",
			mock:    func(mock sqlmock.Sqlmock) { mock.ExpectExec(query).WithArgs(now).WillReturnError(errStub) },
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, mock := newMock(t)
			tt.mock(mock)

			got, err := s.DeleteExpiredSessions(context.Background(), now)
			if tt.wantErr {
				if !errors.Is(err, errStub) {
					t.Fatalf("DeleteExpiredSessions() error = %v, want %v", err, errStub)
				}

				return
			}

			if err != nil {
				t.Fatalf("DeleteExpiredSessions() error = %v", err)
			}

			if got != tt.want {
				t.Fatalf("DeleteExpiredSessions() = %d, want %d", got, tt.want)
			}
		})
	}
}
