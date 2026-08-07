package config

import (
	"maps"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// dbEnv is the full set of variables LoadDB reads.
var dbEnv = []string{
	"DB_DRIVER",
	"DB_HOST",
	"DB_PORT",
	"DB_USER",
	"DB_PASSWORD",
	"DB_NAME",
	"DB_SQLITE_PATH",
	"DB_MAX_OPEN_CONNS",
	"DB_MAX_IDLE_CONNS",
	"DB_CONN_MAX_LIFETIME",
	"DB_CONN_TIMEOUT",
}

// mysqlEnv is a complete, valid mysql environment — the base most cases below start from, so a case
// only has to say how it differs.
func mysqlEnv(overrides map[string]string) map[string]string {
	env := map[string]string{
		"DB_DRIVER":   "mysql",
		"DB_HOST":     "127.0.0.1",
		"DB_PORT":     "3306",
		"DB_USER":     "test",
		"DB_PASSWORD": "test",
		"DB_NAME":     "wherewhat",
	}
	maps.Copy(env, overrides)

	return env
}

// setDBEnv puts the process into a known state for a LoadDB case.
func setDBEnv(t *testing.T, env map[string]string) {
	t.Helper()
	setEnv(t, dbEnv, env)
}

// TestLoadDB_Valid checks the two shapes of a working configuration — a networked driver with
// credentials, and sqlite with only a path — plus the pool settings, which are the one part of
// DBConfig that has defaults.
func TestLoadDB_Valid(t *testing.T) {
	tests := map[string]struct {
		env  map[string]string
		want DBConfig
	}{
		"mysql with pool defaults": {
			env: mysqlEnv(nil),
			want: DBConfig{
				Driver:          "mysql",
				Host:            "127.0.0.1",
				Port:            "3306",
				User:            "test",
				Password:        "test",
				Name:            "wherewhat",
				MaxOpenConns:    defaultMaxOpenConns,
				MaxIdleConns:    defaultMaxIdleConns,
				ConnMaxLifetime: defaultConnMaxLifetime,
				ConnTimeout:     defaultConnTimeout,
			},
		},
		"pool settings override the defaults": {
			env: mysqlEnv(map[string]string{
				"DB_MAX_OPEN_CONNS":    "25",
				"DB_MAX_IDLE_CONNS":    "0",
				"DB_CONN_MAX_LIFETIME": "90s",
				"DB_CONN_TIMEOUT":      "2s",
			}),
			want: DBConfig{
				Driver:          "mysql",
				Host:            "127.0.0.1",
				Port:            "3306",
				User:            "test",
				Password:        "test",
				Name:            "wherewhat",
				MaxOpenConns:    25,
				MaxIdleConns:    0,
				ConnMaxLifetime: 90 * time.Second,
				ConnTimeout:     2 * time.Second,
			},
		},
		// sqlite needs none of the connection fields the networked drivers require.
		"sqlite needs only a path": {
			env: map[string]string{
				"DB_DRIVER":      "sqlite",
				"DB_SQLITE_PATH": "./data/app.db",
			},
			want: DBConfig{
				Driver:          "sqlite",
				SQLitePath:      "./data/app.db",
				MaxOpenConns:    defaultMaxOpenConns,
				MaxIdleConns:    defaultMaxIdleConns,
				ConnMaxLifetime: defaultConnMaxLifetime,
				ConnTimeout:     defaultConnTimeout,
			},
		},
		// Surrounding whitespace in a copied-in host or driver would otherwise fail validation with a
		// message that looks nothing like the actual problem.
		"connection fields are trimmed": {
			env: mysqlEnv(map[string]string{
				"DB_DRIVER": " mysql ",
				"DB_HOST":   " 127.0.0.1 ",
				"DB_NAME":   " wherewhat ",
			}),
			want: DBConfig{
				Driver:          "mysql",
				Host:            "127.0.0.1",
				Port:            "3306",
				User:            "test",
				Password:        "test",
				Name:            "wherewhat",
				MaxOpenConns:    defaultMaxOpenConns,
				MaxIdleConns:    defaultMaxIdleConns,
				ConnMaxLifetime: defaultConnMaxLifetime,
				ConnTimeout:     defaultConnTimeout,
			},
		},
		// The password is the one field read raw: a space in it may well be part of the secret.
		"password keeps its whitespace": {
			env: mysqlEnv(map[string]string{"DB_PASSWORD": " s3 cret "}),
			want: DBConfig{
				Driver:          "mysql",
				Host:            "127.0.0.1",
				Port:            "3306",
				User:            "test",
				Password:        " s3 cret ",
				Name:            "wherewhat",
				MaxOpenConns:    defaultMaxOpenConns,
				MaxIdleConns:    defaultMaxIdleConns,
				ConnMaxLifetime: defaultConnMaxLifetime,
				ConnTimeout:     defaultConnTimeout,
			},
		},
		// An empty password is a legitimate local setup, not a missing setting.
		"empty password is allowed": {
			env: mysqlEnv(map[string]string{"DB_PASSWORD": ""}),
			want: DBConfig{
				Driver:          "mysql",
				Host:            "127.0.0.1",
				Port:            "3306",
				User:            "test",
				Name:            "wherewhat",
				MaxOpenConns:    defaultMaxOpenConns,
				MaxIdleConns:    defaultMaxIdleConns,
				ConnMaxLifetime: defaultConnMaxLifetime,
				ConnTimeout:     defaultConnTimeout,
			},
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			setDBEnv(t, tt.env)

			cfg, err := LoadDB()
			require.NoError(t, err)
			require.Equal(t, tt.want, cfg)
		})
	}
}

// TestLoadDB_Invalid covers the driver-conditional rules: a setting one driver insists on is exactly
// the setting another one ignores, so each case checks the error names the variable actually missing.
func TestLoadDB_Invalid(t *testing.T) {
	tests := map[string]struct {
		env     map[string]string
		wantErr string
	}{
		"no driver": {
			env:     nil,
			wantErr: "DB_DRIVER",
		},
		"unknown driver": {
			env:     map[string]string{"DB_DRIVER": "oracle"},
			wantErr: "DB_DRIVER",
		},
		"mysql without host": {
			env:     mysqlEnv(map[string]string{"DB_HOST": ""}),
			wantErr: "DB_HOST",
		},
		"mysql without port": {
			env:     mysqlEnv(map[string]string{"DB_PORT": ""}),
			wantErr: "DB_PORT",
		},
		"mysql without user": {
			env:     mysqlEnv(map[string]string{"DB_USER": ""}),
			wantErr: "DB_USER",
		},
		"mysql without name": {
			env:     mysqlEnv(map[string]string{"DB_NAME": ""}),
			wantErr: "DB_NAME",
		},
		"sqlite without path": {
			env:     map[string]string{"DB_DRIVER": "sqlite"},
			wantErr: "DB_SQLITE_PATH",
		},
		"non-numeric max open": {
			env:     mysqlEnv(map[string]string{"DB_MAX_OPEN_CONNS": "lots"}),
			wantErr: "DB_MAX_OPEN_CONNS",
		},
		"max open below one": {
			env:     mysqlEnv(map[string]string{"DB_MAX_OPEN_CONNS": "0"}),
			wantErr: "DB_MAX_OPEN_CONNS",
		},
		"negative max idle": {
			env:     mysqlEnv(map[string]string{"DB_MAX_IDLE_CONNS": "-1"}),
			wantErr: "DB_MAX_IDLE_CONNS",
		},
		"unparsable lifetime": {
			env:     mysqlEnv(map[string]string{"DB_CONN_MAX_LIFETIME": "5"}),
			wantErr: "DB_CONN_MAX_LIFETIME",
		},
		"negative connect timeout": {
			env:     mysqlEnv(map[string]string{"DB_CONN_TIMEOUT": "-3s"}),
			wantErr: "DB_CONN_TIMEOUT",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			setDBEnv(t, tt.env)

			_, err := LoadDB()
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}
