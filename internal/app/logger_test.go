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

	"wherewhat/internal/config"
)

// levelQuiet is above every level these tests log at, so a case that only cares about the log file
// can keep its console output out of the test runner's own.
const levelQuiet = slog.LevelError + 1

// TestMultiHandler_RoutesByPerDestinationLevel is the property the whole two-destination setup rests
// on: one record, two handlers, each admitting it or not by its own threshold.
func TestMultiHandler_RoutesByPerDestinationLevel(t *testing.T) {
	var console, file bytes.Buffer

	logger := slog.New(multiHandler{handlers: []slog.Handler{
		slog.NewJSONHandler(&console, &slog.HandlerOptions{Level: slog.LevelWarn}),
		slog.NewJSONHandler(&file, &slog.HandlerOptions{Level: slog.LevelInfo}),
	}})

	logger.Info("quiet")
	logger.Warn("loud")

	if got := console.String(); strings.Contains(got, "quiet") {
		t.Errorf("console got %q, want the info record suppressed at warn", got)
	}

	if got := console.String(); !strings.Contains(got, "loud") {
		t.Errorf("console got %q, want it to contain the warn record", got)
	}

	for _, want := range []string{"quiet", "loud"} {
		if got := file.String(); !strings.Contains(got, want) {
			t.Errorf("file got %q, want it to contain %q", got, want)
		}
	}
}

// TestMultiHandler_Enabled checks the fast path: a level no destination wants must report disabled,
// so callers can skip building the record, while a level any destination wants reports enabled.
func TestMultiHandler_Enabled(t *testing.T) {
	handler := multiHandler{handlers: []slog.Handler{
		slog.NewJSONHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelError}),
		slog.NewJSONHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelWarn}),
	}}

	tests := map[slog.Level]bool{
		slog.LevelInfo:  false,
		slog.LevelWarn:  true,
		slog.LevelError: true,
	}

	for level, want := range tests {
		if got := handler.Enabled(context.Background(), level); got != want {
			t.Errorf("Enabled(%v) = %v, want %v", level, got, want)
		}
	}
}

// TestMultiHandler_WithAttrsReachesEveryDestination guards the derive path: attributes added to the
// logger have to show up in every destination, not just the first.
func TestMultiHandler_WithAttrsReachesEveryDestination(t *testing.T) {
	var first, second bytes.Buffer

	logger := slog.New(multiHandler{handlers: []slog.Handler{
		slog.NewJSONHandler(&first, nil),
		slog.NewJSONHandler(&second, nil),
	}}).With("request_id", "abc123").WithGroup("user")

	logger.Error("boom", "id", 7)

	for name, buf := range map[string]*bytes.Buffer{"first": &first, "second": &second} {
		got := buf.String()
		for _, want := range []string{`"request_id":"abc123"`, `"user":{"id":7}`} {
			if !strings.Contains(got, want) {
				t.Errorf("%s destination got %q, want it to contain %s", name, got, want)
			}
		}
	}
}

// TestMultiHandler_NoDestinations covers both destinations being switched off: logging must stay
// safe and simply go nowhere.
func TestMultiHandler_NoDestinations(t *testing.T) {
	handler := multiHandler{}

	if handler.Enabled(context.Background(), slog.LevelError) {
		t.Error("Enabled() = true with no destinations, want false")
	}

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
	if err != nil {
		t.Fatalf("Pipe() error = %v", err)
	}

	previous := os.Stdout
	os.Stdout = write

	defer func() { os.Stdout = previous }()

	fn()

	if err := write.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	var out bytes.Buffer
	if _, err := out.ReadFrom(read); err != nil {
		t.Fatalf("ReadFrom() error = %v", err)
	}

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
		if err != nil {
			t.Fatalf("initLogger() error = %v, want nil", err)
		}

		slog.Info("hello", "user_id", 7)
		closeLog()
	})

	if !strings.Contains(console, `level=INFO msg=hello user_id=7`) {
		t.Errorf("console = %q, want plain text with level=INFO msg=hello user_id=7", console)
	}

	if strings.HasPrefix(strings.TrimSpace(console), "{") {
		t.Errorf("console = %q, want text rather than JSON", console)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}

	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(contents), &record); err != nil {
		t.Fatalf("log file is not JSON (%v): %q", err, contents)
	}

	if record["msg"] != "hello" || record["level"] != "INFO" || record["user_id"] != float64(7) {
		t.Errorf("log file record = %v, want msg=hello level=INFO user_id=7", record)
	}
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
		if err != nil {
			t.Fatalf("initLogger() error = %v, want nil", err)
		}

		slog.Info("routine")
		slog.Error("broken")
		closeLog()
	})

	if strings.Contains(console, "routine") || !strings.Contains(console, "broken") {
		t.Errorf("console = %q, want the error only", console)
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}

	for _, want := range []string{"routine", "broken"} {
		if !strings.Contains(string(contents), want) {
			t.Errorf("log file = %q, want it to contain %q", contents, want)
		}
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
	if err != nil {
		t.Fatalf("initLogger() error = %v, want nil", err)
	}

	slog.Info("recorded")
	slog.Debug("skipped")
	closeLog()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}

	if got := string(contents); !strings.Contains(got, "recorded") || strings.Contains(got, "skipped") {
		t.Errorf("log file = %q, want the info record only", got)
	}
}

// TestInitLogger_AppendsAcrossRuns checks that a restart adds to the log rather than truncating it —
// the reason the file is opened with O_APPEND.
func TestInitLogger_AppendsAcrossRuns(t *testing.T) {
	keepDefaultLogger(t)

	path := filepath.Join(t.TempDir(), "app.log")

	for _, msg := range []string{"first run", "second run"} {
		closeLog, err := initLogger(config.LogConfig{
			ConsoleLevel: levelQuiet,
			FilePath:     path,
			FileLevel:    slog.LevelInfo,
		})
		if err != nil {
			t.Fatalf("initLogger() error = %v, want nil", err)
		}

		slog.Info(msg)
		closeLog()
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}

	for _, want := range []string{"first run", "second run"} {
		if !strings.Contains(string(contents), want) {
			t.Errorf("log file = %q, want it to contain %q", contents, want)
		}
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
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}

	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	if _, err := initLogger(config.LogConfig{
		ConsoleLevel: levelQuiet,
		FilePath:     filepath.Join(dir, "app.log"),
		FileLevel:    slog.LevelInfo,
	}); err == nil {
		t.Error("initLogger() error = nil, want an error for an unwritable log path")
	}
}
