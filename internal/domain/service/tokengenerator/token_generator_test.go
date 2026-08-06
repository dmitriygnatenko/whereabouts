package tokengenerator

import (
	"encoding/hex"
	"testing"

	"github.com/brianvoe/gofakeit/v7"
)

// TestNewToken checks that NewToken returns a 32-byte token hex-encoded to 64
// characters, with no error.
func TestNewToken(t *testing.T) {
	g := New()

	token, err := g.NewToken()
	if err != nil {
		t.Fatalf("NewToken() error = %v, want nil", err)
	}

	const wantLen = 64 // 32 bytes, hex-encoded
	if len(token) != wantLen {
		t.Fatalf("NewToken() length = %d, want %d", len(token), wantLen)
	}

	if _, err := hex.DecodeString(token); err != nil {
		t.Fatalf("NewToken() = %q is not valid hex: %v", token, err)
	}
}

// TestNewTokenUnique checks that repeated calls don't repeat tokens — the
// CSPRNG backing NewToken should never produce the same 32-byte value twice in a small sample.
func TestNewTokenUnique(t *testing.T) {
	g := New()

	n := gofakeit.Number(500, 1500)
	seen := make(map[string]bool, n)

	for range n {
		token, err := g.NewToken()
		if err != nil {
			t.Fatalf("NewToken() error = %v, want nil", err)
		}

		if seen[token] {
			t.Fatalf("NewToken() produced duplicate token %q", token)
		}
		seen[token] = true
	}
}
