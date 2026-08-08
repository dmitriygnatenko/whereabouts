package app

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"wherewhat/internal/config"
)

// levelQuiet is above every level these tests log at, so a case that only cares about the log file
// can keep its console output out of the test runner's own.
const levelQuiet = slog.LevelError + 1

// TestMultiHandler_RoutesByPerDestinationLevel is the property the whole two-destination setup rests
// on: one record, two handlers, each admitting it or not by its own threshold.
func TestMultiHandler_RoutesByPerDestinationLevel(t *testing.T) {
	var console, file bytes.Buffer

	logger := slog.New(multiHandler{
		handlers: []slog.Handler{
			slog.NewJSONHandler(&console, &slog.HandlerOptions{Level: slog.LevelWarn}),
			slog.NewJSONHandler(&file, &slog.HandlerOptions{Level: slog.LevelInfo}),
		},
	})

	logger.Info("quiet")
	logger.Warn("loud")

	require.NotContains(t, console.String(), "quiet", "want the info record suppressed at warn")
	require.Contains(t, console.String(), "loud")

	for _, want := range []string{
		"quiet",
		"loud",
	} {
		require.Contains(t, file.String(), want)
	}
}

// TestMultiHandler_Enabled checks the fast path: a level no destination wants must report disabled,
// so callers can skip building the record, while a level any destination wants reports enabled.
func TestMultiHandler_Enabled(t *testing.T) {
	handler := multiHandler{
		handlers: []slog.Handler{
			slog.NewJSONHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelError}),
			slog.NewJSONHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelWarn}),
		},
	}

	tests := map[slog.Level]bool{
		slog.LevelInfo:  false,
		slog.LevelWarn:  true,
		slog.LevelError: true,
	}

	for level, want := range tests {
		require.Equal(t, want, handler.Enabled(context.Background(), level))
	}
}

// TestMultiHandler_WithAttrsReachesEveryDestination guards the derive path: attributes added to the
// logger have to show up in every destination, not just the first.
func TestMultiHandler_WithAttrsReachesEveryDestination(t *testing.T) {
	var first, second bytes.Buffer

	logger := slog.New(multiHandler{
		handlers: []slog.Handler{
			slog.NewJSONHandler(&first, nil),
			slog.NewJSONHandler(&second, nil),
		},
	}).With("request_id", "abc123").WithGroup("user")

	logger.Error("boom", "id", 7)

	for name, buf := range map[string]*bytes.Buffer{
		"first":  &first,
		"second": &second,
	} {
		got := buf.String()
		for _, want := range []string{
			`"request_id":"abc123"`,
			`"user":{"id":7}`,
		} {
			require.Contains(t, got, want, "%s destination", name)
		}
	}
}

// TestMultiHandler_NoDestinations covers both destinations being switched off: logging must stay
// safe and simply go nowhere.
func TestMultiHandler_NoDestinations(t *testing.T) {
	handler := multiHandler{}

	require.False(t, handler.Enabled(context.Background(), slog.LevelError))

	slog.New(handler).Error("boom") // must not panic
}

// keepDefaultLogger restores slog's global default when the test ends, so a test that installs a
// logger writing to a file it then closes can't leave later tests logging into a closed handle.
func keepDefaultLogger(t *testing.T) {
	t.Helper()

	previous := slog.Default()

	t.Cleanup(func() { slog.SetDefault(previous) })
}

// captureStdout redirects os.Stdout for the duration of fn and returns what was written to it, which
// is the only way to see what the console destination actually produced — initLogger writes there
// directly rather than through an injected writer.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	read, write, err := os.Pipe()
	require.NoError(t, err)

	previous := os.Stdout
	os.Stdout = write

	defer func() { os.Stdout = previous }()

	fn()

	require.NoError(t, write.Close())

	var out bytes.Buffer
	_, err = out.ReadFrom(read)
	require.NoError(t, err)

	return out.String()
}

// TestInitLogger_Formats pins the two destinations to different formats: plain text on the console
// for a human reading a terminal, JSON in the file for a log aggregator.
func TestInitLogger_Formats(t *testing.T) {
	keepDefaultLogger(t)

	path := filepath.Join(t.TempDir(), "app.log")

	console := captureStdout(t, func() {
		closeLog, err := initLogger(config.LogConfig{
			ConsoleLevel: slog.LevelInfo,
			FilePath:     path,
			FileLevel:    slog.LevelInfo,
		})
		require.NoError(t, err)

		slog.Info("hello", "user_id", 7)
		closeLog()
	})

	require.Contains(t, console, `level=INFO msg=hello user_id=7`)
	require.False(t, strings.HasPrefix(strings.TrimSpace(console), "{"), "want text rather than JSON")

	contents, err := os.ReadFile(path)
	require.NoError(t, err)

	var record map[string]any

	require.NoError(t, json.Unmarshal(bytes.TrimSpace(contents), &record), "log file is not JSON")

	require.Equal(t, "hello", record["msg"])
	require.Equal(t, "INFO", record["level"])
	require.InDelta(t, float64(7), record["user_id"], 0)
}

// TestInitLogger_PerDestinationLevels is the point of the whole arrangement, checked through the
// real wiring rather than the handler alone: a record can be too quiet for the console and still be
// worth recording in the file.
func TestInitLogger_PerDestinationLevels(t *testing.T) {
	keepDefaultLogger(t)

	path := filepath.Join(t.TempDir(), "app.log")

	console := captureStdout(t, func() {
		closeLog, err := initLogger(config.LogConfig{
			ConsoleLevel: slog.LevelError,
			FilePath:     path,
			FileLevel:    slog.LevelInfo,
		})
		require.NoError(t, err)

		slog.Info("routine")
		slog.Error("broken")
		closeLog()
	})

	require.NotContains(t, console, "routine")
	require.Contains(t, console, "broken")

	contents, err := os.ReadFile(path)
	require.NoError(t, err)

	for _, want := range []string{
		"routine",
		"broken",
	} {
		require.Contains(t, string(contents), want)
	}
}

// TestInitLogger_WritesToFile exercises the real wiring end to end: initLogger must create the
// file's parent directory, honour the file's own level, and hand back a closer.
func TestInitLogger_WritesToFile(t *testing.T) {
	keepDefaultLogger(t)

	path := filepath.Join(t.TempDir(), "nested", "app.log")

	closeLog, err := initLogger(config.LogConfig{
		ConsoleLevel: levelQuiet,
		FilePath:     path,
		FileLevel:    slog.LevelInfo,
	})
	require.NoError(t, err)

	slog.Info("recorded")
	slog.Debug("skipped")
	closeLog()

	contents, err := os.ReadFile(path)
	require.NoError(t, err)

	got := string(contents)
	require.Contains(t, got, "recorded")
	require.NotContains(t, got, "skipped")
}

// TestInitLogger_AppendsAcrossRuns checks that a restart adds to the log rather than truncating it —
// the reason the file is opened with O_APPEND.
func TestInitLogger_AppendsAcrossRuns(t *testing.T) {
	keepDefaultLogger(t)

	path := filepath.Join(t.TempDir(), "app.log")

	for _, msg := range []string{
		"first run",
		"second run",
	} {
		closeLog, err := initLogger(config.LogConfig{
			ConsoleLevel: levelQuiet,
			FilePath:     path,
			FileLevel:    slog.LevelInfo,
		})
		require.NoError(t, err)

		slog.Info(msg)
		closeLog()
	}

	contents, err := os.ReadFile(path)
	require.NoError(t, err)

	for _, want := range []string{
		"first run",
		"second run",
	} {
		require.Contains(t, string(contents), want)
	}
}

// TestInitLogger_UnwritableFile checks that a log destination the process can't open is reported as
// an error rather than leaving the app running with logs silently going nowhere.
func TestInitLogger_UnwritableFile(t *testing.T) {
	// Directory permissions don't stop root, which is how tests often run inside a container.
	if os.Geteuid() == 0 {
		t.Skip("running as root: an unwritable directory can't be simulated")
	}

	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0o500))

	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	_, err := initLogger(config.LogConfig{
		ConsoleLevel: levelQuiet,
		FilePath:     filepath.Join(dir, "app.log"),
		FileLevel:    slog.LevelInfo,
	})
	require.Error(t, err, "want an error for an unwritable log path")
}
