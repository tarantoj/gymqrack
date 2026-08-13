package main

import (
	"log/slog"
	"os"
)

// newLogHandler returns the process's structured logger: JSON on stderr.
func newLogHandler() slog.Handler {
	return slog.NewJSONHandler(os.Stderr, nil)
}
