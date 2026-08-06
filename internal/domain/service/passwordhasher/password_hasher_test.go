package passwordhasher

import (
	"strings"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
)

// TestHash checks that Hash returns a non-empty bcrypt hash distinct from the
// plaintext, and that hashing the same password twice yields different hashes (bcrypt salts each
// call).
func TestHash(t *testing.T) {
	h := New()

	pass := gofakeit.Word()

	hash1, err := h.Hash(pass)
	if err != nil {
		t.Fatalf("Hash() error = %v, want nil", err)
	}

	if hash1 == "" {
		t.Fatal("Hash() = \"\", want a non-empty hash")
	}

	if hash1 == pass {
		t.Fatal("Hash() returned the plaintext password unchanged")
	}

	hash2, err := h.Hash(pass)
	if err != nil {
		t.Fatalf("Hash() error = %v, want nil", err)
	}

	if hash1 == hash2 {
		t.Fatal("Hash() returned the same hash for two calls with the same password, want different salts")
	}
}

// TestHashTooLong checks that Hash surfaces bcrypt's error for passwords over
// its 72-byte limit instead of silently truncating them.
func TestHashTooLong(t *testing.T) {
	h := New()

	long := strings.Repeat("a", 73)
	if _, err := h.Hash(long); err == nil {
		t.Fatal("Hash() error = nil, want an error for a password over 72 bytes")
	}
}

// TestCompare checks that Compare accepts the matching password, and rejects
// a wrong password or a malformed hash.
func TestCompare(t *testing.T) {
	h := New()

	pass := gofakeit.Password(true, true, true, true, false, 16)
	wrongPass := gofakeit.Password(true, true, true, true, false, 16)

	hash, err := h.Hash(pass)
	if err != nil {
		t.Fatalf("Hash() error = %v, want nil", err)
	}

	tests := []struct {
		name     string
		hash     string
		password string
		want     bool
	}{
		{
			"matching password",
			hash,
			pass,
			true,
		},
		{
			"wrong password",
			hash,
			wrongPass,
			false,
		},
		{
			"empty password",
			hash,
			"",
			false,
		},
		{
			"malformed hash",
			"not-a-bcrypt-hash",
			pass,
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := h.Compare(tt.hash, tt.password); got != tt.want {
				t.Fatalf("Compare(%q, %q) = %v, want %v", tt.hash, tt.password, got, tt.want)
			}
		})
	}
}
