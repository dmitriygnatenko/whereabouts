package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"

	"wherewhat/internal/domain/entity"
)

// User is the shape of a row in the users table.
type User struct {
	ID           uint64
	Username     string
	PasswordHash string
	Settings     UserSettings
}

// ToEntity converts a row-shaped User into the domain entity.
func (u User) ToEntity() entity.User {
	return entity.User{
		ID:           u.ID,
		Username:     u.Username,
		PasswordHash: u.PasswordHash,
		Settings:     u.Settings.ToEntity(),
	}
}

// UserSettings is the on-disk shape of the users.settings JSON column. Field names/tags must stay
// exactly as they are — they're the storage format for every row already written.
type UserSettings struct {
	Language            string `json:"language,omitempty"`
	LocationFilterDepth int    `json:"locationFilterDepth,omitempty"`
}

// UserSettingsFromEntity converts a domain UserSettings into its row shape, ready to be bound to the
// settings column.
func UserSettingsFromEntity(s entity.UserSettings) UserSettings {
	return UserSettings{
		Language:            s.Language,
		LocationFilterDepth: s.LocationFilterDepth,
	}
}

// ToEntity converts a row-shaped UserSettings into the domain entity.
func (s *UserSettings) ToEntity() entity.UserSettings {
	return entity.UserSettings{
		Language:            s.Language,
		LocationFilterDepth: s.LocationFilterDepth,
	}
}

// Scan implements sql.Scanner, decoding the settings JSON column. An empty or NULL column yields the
// zero value rather than an error — that's what a row written before a settings field existed looks
// like.
func (s *UserSettings) Scan(src any) error {
	var raw []byte

	switch v := src.(type) {
	case nil:
		*s = UserSettings{}

		return nil
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("unsupported settings value of type %T", src)
	}

	if len(raw) == 0 {
		*s = UserSettings{}

		return nil
	}

	var decoded UserSettings
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}

	*s = decoded

	return nil
}

// Value implements driver.Valuer, encoding the settings for the JSON column. A value receiver,
// unlike Scan: every adapter binds a UserSettings by value into query args, and only the value type
// — not *UserSettings — satisfies driver.Valuer that way.
func (s UserSettings) Value() (driver.Value, error) {
	return json.Marshal(s)
}
