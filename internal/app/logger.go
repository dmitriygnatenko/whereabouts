package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"wherewhat/internal/config"
)

// logFilePerm/logDirPerm are what a freshly created log file and its parent directory get: readable
// by anyone who can reach them, writable only by the user running the process.
const (
	logFilePerm = 0o644
	logDirPerm  = 0o755
)

// initLogger installs the process-wide structured logger (via slog.SetDefault) that use cases call
// through the package-level slog.ErrorContext etc. to record operational errors — e.g. a repository
// failure a use case turns into a generic message for the caller. A global logger, rather than one
// threaded through every use case constructor, keeps use cases from having to take a logging
// dependency just to report the occasional internal error.
//
// Records fan out to up to two destinations, each with its own threshold and its own format. The
// console goes to stdout (so logs land in the stream a container runtime or process supervisor
// already collects) as plain text, which is what someone actually reading a terminal wants; the file
// gets JSON, which is what a log aggregator wants to ingest and query. The returned function closes
// the log file and must be called before the process exits; it is safe to call even when no file was
// opened.
//
// Note that slog.SetDefault also redirects the standard log package through this handler, at INFO —
// so anything written with log.Printf is subject to these thresholds too, and output meant to be
// unconditional (the startup banner, fatal errors) must not go through log.
func initLogger(cfg config.LogConfig) (closeLog func(), err error) {
	// The console is always one of the destinations — LOG_CONSOLE_LEVEL only decides how much of the
	// stream reaches it — so unlike the file it needs no presence check.
	handlers := []slog.Handler{
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.ConsoleLevel}),
	}

	closeLog = func() {}

	if cfg.FilePath != "" {
		file, err := openLogFile(cfg.FilePath)
		if err != nil {
			return nil, err
		}

		closeLog = func() { _ = file.Close() }

		handlers = append(handlers, slog.NewJSONHandler(file, &slog.HandlerOptions{Level: cfg.FileLevel}))
	}

	slog.SetDefault(slog.New(multiHandler{handlers: handlers}))

	return closeLog, nil
}

// openLogFile opens the log file for appending, creating it and any missing parent directories.
// Appending (rather than truncating) keeps history across restarts; nothing here rotates the file,
// so a long-lived deployment should point LOG_FILE_PATH somewhere logrotate or the equivalent can
// see it.
func openLogFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), logDirPerm); err != nil {
		return nil, fmt.Errorf("failed to create the log directory for %s: %w", path, err)
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, logFilePerm)
	if err != nil {
		return nil, fmt.Errorf("failed to open the log file %s: %w", path, err)
	}

	return file, nil
}

// multiHandler fans one record out to several handlers, which is what lets the console and the file
// hold different levels: each destination keeps its own slog.Handler with its own threshold, and
// this handler asks each one whether it wants the record. slog has no built-in equivalent — a single
// Logger has exactly one Handler.
//
// A multiHandler with no handlers discards everything, which is the natural result of switching both
// destinations off.
type multiHandler struct {
	handlers []slog.Handler
}

// Enabled reports whether any destination wants records at this level, so a suppressed level still
// costs nothing to log.
func (h multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, sub := range h.handlers {
		if sub.Enabled(ctx, level) {
			return true
		}
	}

	return false
}

// Handle passes the record to every destination whose own threshold admits it. Each gets a clone:
// slog.Record shares its backing array between copies, so handing the same one to two handlers
// risks them clobbering each other's attributes.
func (h multiHandler) Handle(ctx context.Context, record slog.Record) error {
	var errs []error

	for _, sub := range h.handlers {
		if !sub.Enabled(ctx, record.Level) {
			continue
		}

		if err := sub.Handle(ctx, record.Clone()); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

// WithAttrs returns a handler whose destinations have all been given the attributes.
func (h multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return h.derive(func(sub slog.Handler) slog.Handler { return sub.WithAttrs(attrs) })
}

// WithGroup returns a handler whose destinations have all opened the group.
func (h multiHandler) WithGroup(name string) slog.Handler {
	return h.derive(func(sub slog.Handler) slog.Handler { return sub.WithGroup(name) })
}

// derive builds a new multiHandler by applying with to each destination, leaving the receiver — which
// slog may still be using — untouched.
func (h multiHandler) derive(with func(slog.Handler) slog.Handler) slog.Handler {
	subs := make([]slog.Handler, len(h.handlers))
	for i, sub := range h.handlers {
		subs[i] = with(sub)
	}

	return multiHandler{handlers: subs}
}
