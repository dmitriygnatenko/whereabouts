package config

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

// logEnv is the full set of variables LoadLog reads.
var logEnv = []string{
	"LOG_CONSOLE_LEVEL",
	"LOG_FILE_PATH",
	"LOG_FILE_LEVEL",
}

// setLogEnv puts the process into a known state for a LoadLog case.
func setLogEnv(t *testing.T, env map[string]string) {
	t.Helper()
	setEnv(t, logEnv, env)
}

// withLogDefaults returns the configuration LoadLog produces from an empty environment, with
// override applied on top (pass nil for none). Overriding through a function rather than merging
// zero fields matters here: slog.LevelInfo is 0, so a zero level is a value a case might legitimately
// expect, not "unset".
func withLogDefaults(override func(*LogConfig)) LogConfig {
	cfg := LogConfig{
		ConsoleLevel: defaultConsoleLogLevel,
		FilePath:     "",
		FileLevel:    defaultFileLogLevel,
	}
	if override != nil {
		override(&cfg)
	}

	return cfg
}

// TestLoadLog_Defaults pins the out-of-the-box behaviour: the console is on at warn, and there is no
// log file until one is asked for.
func TestLoadLog_Defaults(t *testing.T) {
	setLogEnv(t, nil)

	cfg, err := LoadLog()
	require.NoError(t, err)
	require.Equal(t, withLogDefaults(nil), cfg)
}

// TestLoadLog_Valid covers the spellings a level accepts and, more importantly, that the two
// destinations are configured independently of each other.
func TestLoadLog_Valid(t *testing.T) {
	tests := map[string]struct {
		env  map[string]string
		want LogConfig
	}{
		"both destinations set": {
			env: map[string]string{
				"LOG_CONSOLE_LEVEL": "debug",
				"LOG_FILE_PATH":     "/var/log/app.log",
				"LOG_FILE_LEVEL":    "error",
			},
			want: LogConfig{
				ConsoleLevel: slog.LevelDebug,
				FilePath:     "/var/log/app.log",
				FileLevel:    slog.LevelError,
			},
		},
		"level is case insensitive and trimmed": {
			env:  map[string]string{"LOG_CONSOLE_LEVEL": " Info "},
			want: withLogDefaults(func(c *LogConfig) { c.ConsoleLevel = slog.LevelInfo }),
		},
		"level takes an offset": {
			env:  map[string]string{"LOG_CONSOLE_LEVEL": "info+2"},
			want: withLogDefaults(func(c *LogConfig) { c.ConsoleLevel = slog.LevelInfo + 2 }),
		},
		// A level without a path is harmless: there's simply no file for it to apply to.
		"file level without a path": {
			env:  map[string]string{"LOG_FILE_LEVEL": "debug"},
			want: withLogDefaults(func(c *LogConfig) { c.FileLevel = slog.LevelDebug }),
		},
		"file path is trimmed": {
			env:  map[string]string{"LOG_FILE_PATH": "  ./logs/app.log  "},
			want: withLogDefaults(func(c *LogConfig) { c.FilePath = "./logs/app.log" }),
		},
		// A blank path is treated as no path at all, rather than a file named " ".
		"blank file path means no file": {
			env:  map[string]string{"LOG_FILE_PATH": "   "},
			want: withLogDefaults(nil),
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			setLogEnv(t, tt.env)

			cfg, err := LoadLog()
			require.NoError(t, err)
			require.Equal(t, tt.want, cfg)
		})
	}
}

// TestLoadLog_Invalid checks that an unrecognized level is rejected rather than silently falling back
// to the default, which would leave someone wondering where their debug output went.
func TestLoadLog_Invalid(t *testing.T) {
	tests := map[string]struct {
		env     map[string]string
		wantErr string
	}{
		"unknown console level": {
			env:     map[string]string{"LOG_CONSOLE_LEVEL": "verbose"},
			wantErr: "LOG_CONSOLE_LEVEL",
		},
		"unknown file level": {
			env:     map[string]string{"LOG_FILE_LEVEL": "loud"},
			wantErr: "LOG_FILE_LEVEL",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			setLogEnv(t, tt.env)

			_, err := LoadLog()
			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}
