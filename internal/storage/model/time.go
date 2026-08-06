package model

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// timeLayouts covers every shape the three supported drivers hand back for a TIMESTAMP column when
// scanned as text (MySQL/SQLite don't always round-trip through driver-native time.Time the way
// Postgres/pgx does).
var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05",
}

// Time is a time.Time that can be scanned from a TIMESTAMP column no matter which of the three
// supported drivers produced it: some decode the column natively, others hand back text in one of
// several layouts. Values are always normalized to UTC.
type Time struct {
	time.Time
}

// NewTime wraps t for storage.
func NewTime(t time.Time) Time {
	return Time{
		Time: t,
	}
}

// Scan implements sql.Scanner, accepting either a driver-decoded time.Time or the textual forms
// listed in timeLayouts.
func (t *Time) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		t.Time = time.Time{}

		return nil
	case time.Time:
		t.Time = v.UTC()

		return nil
	case []byte:
		return t.parse(string(v))
	case string:
		return t.parse(v)
	default:
		return fmt.Errorf("unsupported time value of type %T", src)
	}
}

// Value implements driver.Valuer, binding the wrapped time.Time as every driver already understands
// it natively. A value receiver, unlike Scan: every adapter binds a Time by value into query args
// (e.g. session.ExpiresAt), and only the value type — not *Time — satisfies driver.Valuer that way.
func (t Time) Value() (driver.Value, error) {
	return t.Time, nil
}

// parse tries each of timeLayouts in turn, keeping the first one that parses s.
func (t *Time) parse(s string) error {
	for _, layout := range timeLayouts {
		if parsed, err := time.Parse(layout, s); err == nil {
			t.Time = parsed.UTC()

			return nil
		}
	}

	return fmt.Errorf("unrecognized time format: %q", s)
}
