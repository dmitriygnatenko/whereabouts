package validate_test

import (
	"testing"

	"wherewhat/internal/domain/usecase/validate"
)

// TestUsername checks Username's required/min-length rules and their messages.
func TestUsername(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantMsg string
	}{
		{"empty", "", "Please enter a username"},
		{"too short", "ab", "Username must be at least 3 characters"},
		{"valid", "abc", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validate.Username(tt.input)
			if tt.wantMsg == "" {
				if got != nil {
					t.Fatalf("Username(%q) = %v, want nil", tt.input, got)
				}

				return
			}

			if got == nil || got.Message != tt.wantMsg {
				t.Fatalf("Username(%q) = %v, want message %q", tt.input, got, tt.wantMsg)
			}
		})
	}
}

// TestPassword checks Password's required/min-length rules and their messages.
func TestPassword(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantMsg string
	}{
		{"empty", "", "Password must be at least 4 characters"},
		{"too short", "abc", "Password must be at least 4 characters"},
		{"valid", "abcd", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validate.Password(tt.input)
			if tt.wantMsg == "" {
				if got != nil {
					t.Fatalf("Password(%q) = %v, want nil", tt.input, got)
				}

				return
			}

			if got == nil || got.Message != tt.wantMsg {
				t.Fatalf("Password(%q) = %v, want message %q", tt.input, got, tt.wantMsg)
			}
		})
	}
}
