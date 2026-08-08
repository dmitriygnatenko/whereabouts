package app

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"wherewhat/internal/config"
	"wherewhat/internal/port"
	"wherewhat/internal/port/mocks"
	userrepo "wherewhat/internal/repository/user"
)

// captureSlog redirects the process-wide slog default for the duration of fn and returns everything
// written to it, restoring the previous default afterwards — the only way to observe what
// openStorage/seedDemoUser log, since they go through the package-level slog functions rather than an
// injected logger.
func captureSlog(t *testing.T, fn func()) string {
	t.Helper()

	var buf bytes.Buffer

	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))

	t.Cleanup(func() { slog.SetDefault(previous) })

	fn()

	return buf.String()
}

// unreachableAddr is a loopback host:port nothing listens on, so dialing it fails immediately with
// "connection refused" instead of hanging — exactly what a real DB server being unreachable looks
// like, without needing one running.
const unreachableAddr = "127.0.0.1"

func unreachablePort(t *testing.T) string {
	t.Helper()

	// Bind port 0 to get one the OS guarantees is currently free, then close it immediately: nothing
	// is listening there again by the time the test dials it.
	l, err := net.Listen("tcp", unreachableAddr+":0")
	require.NoError(t, err)

	_, port, err := net.SplitHostPort(l.Addr().String())
	require.NoError(t, err)
	require.NoError(t, l.Close())

	return port
}

// TestOpenStorage_UnsupportedDriver checks the fallback branch: an unrecognized driver is rejected
// outright rather than falling through to one of the three real drivers.
func TestOpenStorage_UnsupportedDriver(t *testing.T) {
	_, err := openStorage(context.Background(), config.DBConfig{Driver: "oracle"})
	require.ErrorContains(t, err, `unsupported DB_DRIVER "oracle"`)
}

// TestOpenStorage_SQLite exercises the real sqlite branch end to end — ensure, open, and migrate all
// run for real against a temp file, which is cheap enough that there's no reason to fake it.
func TestOpenStorage_SQLite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "nested", "test.db")

	var (
		store storage
		err   error
	)

	logs := captureSlog(t, func() {
		store, err = openStorage(context.Background(), config.DBConfig{
			Driver:     config.DriverSQLite,
			SQLitePath: dbPath,
		})
	})

	require.NoError(t, err)
	require.NotNil(t, store)
	t.Cleanup(func() { _ = store.Close() })

	require.Contains(t, logs, "opening database")
	require.Contains(t, logs, "driver=sqlite")
	require.Contains(t, logs, dbPath)

	// The migration actually ran: the users table exists and is queryable through the repository,
	// rather than just "no error was returned".
	users := userrepo.New(store)
	count, err := users.Count(context.Background())
	require.NoError(t, err)
	require.Zero(t, count)
}

// TestOpenStorage_MySQL_ConnectionFailure and TestOpenStorage_Postgres_ConnectionFailure check that a
// networked driver which can't reach its server reports a clear, wrapped connection error — and, for
// MySQL, that EnsureDatabase failing first is only a warning, not fatal (Open still runs and produces
// the real error).
func TestOpenStorage_MySQL_ConnectionFailure(t *testing.T) {
	logs := captureSlog(t, func() {
		_, err := openStorage(context.Background(), config.DBConfig{
			Driver:      config.DriverMySQL,
			Host:        unreachableAddr,
			Port:        unreachablePort(t),
			User:        "user",
			Name:        "testdb",
			ConnTimeout: time.Second,
		})
		require.ErrorContains(t, err, "failed to connect to the database")
	})

	require.Contains(t, logs, "opening database")
	require.Contains(t, logs, "failed to auto-create the database")
}

func TestOpenStorage_Postgres_ConnectionFailure(t *testing.T) {
	logs := captureSlog(t, func() {
		_, err := openStorage(context.Background(), config.DBConfig{
			Driver: config.DriverPostgres,
			Host:   unreachableAddr,
			Port:   unreachablePort(t),
			User:   "user",
			Name:   "testdb",
		})
		require.ErrorContains(t, err, "failed to connect to the database")
	})

	require.Contains(t, logs, "opening database")
	require.Contains(t, logs, "failed to auto-create the database")
}

// TestOpenAndMigrate covers the shared driver dance in isolation, with fake steps standing in for a
// real driver — every branch (ensure/open/migrate each failing, and the happy path) is exercised
// without touching a real database.
func TestOpenAndMigrate(t *testing.T) {
	t.Run("runs ensure, open and migrate in order and returns the opened store", func(t *testing.T) {
		var calls []string

		got, err := openAndMigrate(context.Background(),
			func() error {
				calls = append(calls, "ensure")

				return nil
			},
			func() (string, error) {
				calls = append(calls, "open")

				return "the-store", nil
			},
			func(context.Context, string) error {
				calls = append(calls, "migrate")

				return nil
			},
		)

		require.NoError(t, err)
		require.Equal(t, "the-store", got)
		require.Equal(t, []string{
			"ensure",
			"open",
			"migrate",
		}, calls)
	})

	t.Run("a failed ensure is only logged, open and migrate still run", func(t *testing.T) {
		var opened, migrated bool

		logs := captureSlog(t, func() {
			got, err := openAndMigrate(context.Background(),
				func() error { return errStub },
				func() (string, error) { opened = true; return "the-store", nil },
				func(context.Context, string) error { migrated = true; return nil },
			)

			require.NoError(t, err)
			require.Equal(t, "the-store", got)
		})

		require.True(t, opened, "expected open to still run after ensure failed")
		require.True(t, migrated, "expected migrate to still run after ensure failed")
		require.Contains(t, logs, "failed to auto-create the database")
		require.Contains(t, logs, errStub.Error())
	})

	t.Run("an open failure is wrapped and migrate never runs", func(t *testing.T) {
		migrated := false

		got, err := openAndMigrate(context.Background(),
			func() error { return nil },
			func() (string, error) { return "", errStub },
			func(context.Context, string) error { migrated = true; return nil },
		)

		require.ErrorIs(t, err, errStub)
		require.ErrorContains(t, err, "failed to connect to the database")
		require.Zero(t, got)
		require.False(t, migrated, "migrate must not run once open has failed")
	})

	t.Run("a migrate failure is wrapped and the zero value is returned", func(t *testing.T) {
		got, err := openAndMigrate(context.Background(),
			func() error { return nil },
			func() (string, error) { return "the-store", nil },
			func(context.Context, string) error { return errStub },
		)

		require.ErrorIs(t, err, errStub)
		require.ErrorContains(t, err, "failed to run migrations")
		require.Zero(t, got)
	})
}

// seedDeps bundles the two mocks seedDemoUser depends on.
type seedDeps struct {
	users  *mocks.MockUserRepository
	hasher *mocks.MockPasswordHasher
}

func newSeedDeps(t *testing.T) *seedDeps {
	t.Helper()

	mc := gomock.NewController(t)
	t.Cleanup(mc.Finish)

	return &seedDeps{
		users:  mocks.NewMockUserRepository(mc),
		hasher: mocks.NewMockPasswordHasher(mc),
	}
}

// TestSeedDemoUser covers the empty-table happy path, the already-seeded no-op, and every step's
// failure being propagated.
func TestSeedDemoUser(t *testing.T) {
	t.Run("does nothing when the users table is not empty", func(t *testing.T) {
		d := newSeedDeps(t)
		d.users.EXPECT().Count(gomock.Any()).Return(1, nil)

		err := seedDemoUser(context.Background(), seedDemoUserRequest{
			Users: d.users, Hasher: d.hasher, Username: "demo", Password: "password",
		})
		require.NoError(t, err)
	})

	t.Run("a Count error is propagated and nothing is created", func(t *testing.T) {
		d := newSeedDeps(t)
		d.users.EXPECT().Count(gomock.Any()).Return(0, errStub)

		err := seedDemoUser(context.Background(), seedDemoUserRequest{
			Users: d.users, Hasher: d.hasher, Username: "demo", Password: "password",
		})
		require.ErrorIs(t, err, errStub)
	})

	t.Run("a hashing error is propagated and nothing is created", func(t *testing.T) {
		d := newSeedDeps(t)
		d.users.EXPECT().Count(gomock.Any()).Return(0, nil)
		d.hasher.EXPECT().Hash("password").Return("", errStub)

		err := seedDemoUser(context.Background(), seedDemoUserRequest{
			Users: d.users, Hasher: d.hasher, Username: "demo", Password: "password",
		})
		require.ErrorIs(t, err, errStub)
	})

	t.Run("a Create error is propagated", func(t *testing.T) {
		d := newSeedDeps(t)
		d.users.EXPECT().Count(gomock.Any()).Return(0, nil)
		d.hasher.EXPECT().Hash("password").Return("hashed", nil)
		d.users.EXPECT().
			Create(gomock.Any(), port.UserCreateRequest{
				Username:     "demo",
				PasswordHash: "hashed",
			}).
			Return(uint64(0), errStub)

		err := seedDemoUser(context.Background(), seedDemoUserRequest{
			Users: d.users, Hasher: d.hasher, Username: "demo", Password: "password",
		})
		require.ErrorIs(t, err, errStub)
	})

	t.Run("creates the demo account and logs it when the table is empty", func(t *testing.T) {
		d := newSeedDeps(t)
		d.users.EXPECT().Count(gomock.Any()).Return(0, nil)
		d.hasher.EXPECT().Hash("password").Return("hashed", nil)
		d.users.EXPECT().
			Create(gomock.Any(), port.UserCreateRequest{
				Username:     "demo",
				PasswordHash: "hashed",
			}).
			Return(uint64(1), nil)

		var err error

		logs := captureSlog(t, func() {
			err = seedDemoUser(context.Background(), seedDemoUserRequest{
				Users: d.users, Hasher: d.hasher, Username: "demo", Password: "password",
			})
		})

		require.NoError(t, err)
		require.Contains(t, logs, "seeded the demo user")
		require.Contains(t, logs, "username=demo")
	})
}
