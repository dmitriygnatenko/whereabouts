package main

import (
	"log/slog"
	"os"

	"wherewhat/internal/app"
)

func main() {
	// Reported through slog rather than log.Fatal: by the time Main returns, slog.SetDefault has
	// pointed the log package at the application's handler, which routes it at INFO — below the
	// default console threshold, so log.Fatal would exit(1) in silence. At ERROR this clears every
	// threshold the app configures by default, and reaches the log file too. Before the logger is
	// installed, slog's own default still puts it on stderr.
	if err := app.Main(); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}
