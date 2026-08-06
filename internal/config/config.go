// Package config loads and validates the application's configuration from the environment. It lives
// under internal/ (not cmd/whereabouts) so any adapter or composition root could depend on it
// without reaching into the binary's own package. It deliberately knows nothing about any concrete
// DB adapter (internal/adapter/sqlite, mysql, postgres) — only the composition root
// (cmd/whereabouts/main.go) picks one, based on Driver.
//
// The configuration is split into three independent groups, one per file: app.go (how the server
// runs), db.go (how it reaches storage) and log.go (where it writes records). Each has its own
// Load function, so a caller takes only the group it needs — `-create-user`, for instance, loads
// logging and the database but has no use for the HTTP port. This file holds what they share:
// reading the .env file, and the typed env readers each group parses its own variables with.
package config

import (
	"cmp"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// LoadEnv populates process env vars from a local .env file, if present — convenient for local
// development so you don't have to export DB_HOST etc. by hand. Real environment variables always
// win: godotenv.Load never overwrites a variable that's already set, so this is a no-op in
// production/Docker where config comes from the real environment.
func LoadEnv() {
	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		log.Printf("warning: failed to read .env: %v", err)
	}
}

// stringEnv reads name, falling back to def when it's unset or holds nothing but whitespace. The
// value is trimmed: surrounding spaces in a port or a username are a .env typo rather than intent
// (godotenv itself strips them from unquoted values), and leaving them in would seed a demo account
// whose name is a blank. Every reader below goes through it, so "what counts as a value" is decided
// in exactly one place — the one exception is DB_PASSWORD, which LoadDB reads raw because spaces in
// a password may well be deliberate.
func stringEnv(name, def string) string {
	return cmp.Or(strings.TrimSpace(os.Getenv(name)), def)
}

// portEnv reads name as a TCP port a listener can actually bind, falling back to def when it's
// unset. 0 is rejected along with non-numeric values: it's valid to bind but would pick an arbitrary
// free port, which is never what a PORT setting means. The port stays a string because that's how
// it's used — as the ":8080" half of a listen address.
func portEnv(name, def string) (string, error) {
	v := stringEnv(name, def)

	n, err := strconv.Atoi(v)
	if err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("invalid %s %q: must be a number between 1 and 65535", name, v)
	}

	return v, nil
}

// boolEnv reads name as a boolean — strconv.ParseBool's spelling, so "1", "t", "true" and "TRUE" all
// count as true — falling back to def when it's unset.
func boolEnv(name string, def bool) (bool, error) {
	v := stringEnv(name, "")
	if v == "" {
		return def, nil
	}

	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("invalid %s %q: must be true or false", name, v)
	}

	return b, nil
}

// intEnv reads name as an integer, falling back to def when it's unset.
func intEnv(name string, def int) (int, error) {
	v := stringEnv(name, "")
	if v == "" {
		return def, nil
	}

	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: must be an integer", name, v)
	}

	return n, nil
}

// durationEnv reads name as a Go duration string (e.g. "5m", "30s"), falling back to def when it's unset.
func durationEnv(name string, def time.Duration) (time.Duration, error) {
	v := stringEnv(name, "")
	if v == "" {
		return def, nil
	}

	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", name, v, err)
	}

	return d, nil
}
