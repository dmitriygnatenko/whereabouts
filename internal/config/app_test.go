package config

import (
	"strings"
	"testing"
)

// appEnv is the full set of variables LoadApp reads.
var appEnv = []string{"PORT", "COOKIE_SECURE", "DEMO_USERNAME", "DEMO_PASSWORD"}

// setAppEnv puts the process into a known state for a LoadApp case.
func setAppEnv(t *testing.T, env map[string]string) {
	t.Helper()
	setEnv(t, appEnv, env)
}

// withAppDefaults returns the configuration LoadApp produces from an empty environment, with
// override applied on top (pass nil for none), so each case below only spells out what it changes.
func withAppDefaults(override func(*AppConfig)) AppConfig {
	cfg := AppConfig{
		Port:         defaultPort,
		CookieSecure: false,
		DemoUsername: defaultDemoUsername,
		DemoPassword: defaultDemoPassword,
	}
	if override != nil {
		override(&cfg)
	}

	return cfg
}

// TestLoadApp_Defaults checks that an empty environment yields a fully usable configuration — the
// app is meant to start with nothing set — and, in particular, that cookies default to insecure,
// which is what plain-HTTP local development needs.
func TestLoadApp_Defaults(t *testing.T) {
	setAppEnv(t, nil)

	cfg, err := LoadApp()
	if err != nil {
		t.Fatalf("LoadApp() error = %v, want nil", err)
	}

	want := withAppDefaults(nil)
	if cfg != want {
		t.Errorf("LoadApp() = %+v, want %+v", cfg, want)
	}
}

// TestLoadApp_Valid covers the spellings each setting accepts — notably COOKIE_SECURE, which follows
// strconv.ParseBool rather than an exact "true" match.
func TestLoadApp_Valid(t *testing.T) {
	tests := map[string]struct {
		env  map[string]string
		want AppConfig
	}{
		"all set": {
			env: map[string]string{
				"PORT": "9000", "COOKIE_SECURE": "true",
				"DEMO_USERNAME": "alice", "DEMO_PASSWORD": "s3cret",
			},
			want: AppConfig{
				Port: "9000", CookieSecure: true,
				DemoUsername: "alice", DemoPassword: "s3cret",
			},
		},
		"cookie secure accepts 1": {
			env:  map[string]string{"COOKIE_SECURE": "1"},
			want: withAppDefaults(func(c *AppConfig) { c.CookieSecure = true }),
		},
		"cookie secure accepts FALSE": {
			env:  map[string]string{"COOKIE_SECURE": "FALSE"},
			want: withAppDefaults(nil),
		},
		"strings are trimmed": {
			env: map[string]string{"PORT": " 9000 ", "DEMO_USERNAME": " alice "},
			want: withAppDefaults(func(c *AppConfig) {
				c.Port, c.DemoUsername = "9000", "alice"
			}),
		},
		// A whitespace-only value is treated as unset rather than seeding an account named " ".
		"blank strings fall back to defaults": {
			env:  map[string]string{"PORT": " ", "DEMO_USERNAME": " ", "DEMO_PASSWORD": "  "},
			want: withAppDefaults(nil),
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			setAppEnv(t, tt.env)

			cfg, err := LoadApp()
			if err != nil {
				t.Fatalf("LoadApp() error = %v, want nil", err)
			}

			if cfg != tt.want {
				t.Errorf("LoadApp() = %+v, want %+v", cfg, tt.want)
			}
		})
	}
}

// TestLoadApp_Invalid checks that a malformed value is rejected instead of silently falling back to
// the default, and that the message names the offending variable so the fix is obvious.
func TestLoadApp_Invalid(t *testing.T) {
	tests := map[string]struct {
		env     map[string]string
		wantErr string
	}{
		"non-bool cookie flag": {env: map[string]string{"COOKIE_SECURE": "yes"}, wantErr: "COOKIE_SECURE"},
		"non-numeric port":     {env: map[string]string{"PORT": "http"}, wantErr: "PORT"},
		"port out of range":    {env: map[string]string{"PORT": "70000"}, wantErr: "PORT"},
		"port zero":            {env: map[string]string{"PORT": "0"}, wantErr: "PORT"},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			setAppEnv(t, tt.env)

			cfg, err := LoadApp()
			if err == nil {
				t.Fatalf("LoadApp() = %+v, want an error", cfg)
			}

			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("LoadApp() error = %q, want it to mention %s", err, tt.wantErr)
			}
		})
	}
}
