package entity

// UserSettings holds the user's app preferences, stored as one JSON blob rather than separate
// columns. omitempty on both fields matches the original API: a language that hasn't been set yet,
// or a filter depth of 0 ("no limit"), are simply absent from the response rather than sent as zero
// values.
type UserSettings struct {
	Language            string `json:"language,omitempty"`
	LocationFilterDepth int    `json:"locationFilterDepth,omitempty"`
}

// User is the full account record, including the password hash. It never leaves the use case layer
// as-is — API responses use PublicUser instead.
type User struct {
	ID           uint64
	Username     string
	PasswordHash string
	Settings     UserSettings
}

// PublicUser is what use cases hand back to callers: everything about a user except the password
// hash. UserSettings is embedded anonymously so its fields (language, locationFilterDepth)
// serialize at the top level — the frontend reads currentUser.language directly.
type PublicUser struct {
	ID       uint64 `json:"id"`
	Username string `json:"username"`
	UserSettings
}

// Public strips the password hash, producing the PublicUser safe to hand back to API callers.
func (u User) Public() PublicUser {
	return PublicUser{
		ID:           u.ID,
		Username:     u.Username,
		UserSettings: u.Settings,
	}
}

// SupportedLanguages — interface languages the frontend understands. Keep in sync with
// SUPPORTED_LOCALES in web/i18n.js ("en" isn't listed there — it's the default the frontend falls
// back to on its own).
var SupportedLanguages = map[string]struct{}{
	"en": {},
	"ru": {},
	"de": {},
	"es": {},
	"fr": {},
}

// MinUsernameLength is the minimum accepted length for a username.
const MinUsernameLength = 3

// MinPasswordLength is the minimum accepted length for a password.
const MinPasswordLength = 4
