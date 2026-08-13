package main

import (
	"log/slog"
	"os"
	"strings"

	"github.com/coreos/go-systemd/v22/journal"
	slogjournal "github.com/systemd/slog-journal"
)

// newLogHandler returns the process's structured logger. Under a systemd
// service it writes to the journal over its native socket via
// slogjournal (structured fields + correct priority + CODE_* fields);
// everywhere else it falls back to JSON on stderr. VIVAGYM_LOG_FORMAT=json
// forces JSON even under systemd.
func newLogHandler() slog.Handler {
	jsonHandler := slog.NewJSONHandler(os.Stderr, nil)
	if os.Getenv("VIVAGYM_LOG_FORMAT") == "json" {
		return jsonHandler
	}
	// journal.Enabled() checks the native socket or the JOURNAL_STREAM
	// transport; without a journal, NewHandler would succeed but silently
	// drop records (writes fail with ENOENT), so we gate explicitly.
	if !journal.Enabled() {
		return jsonHandler
	}
	h, err := slogjournal.NewHandler(&slogjournal.Options{
		// The journal only accepts keys of the form ^[A-Z_][A-Z0-9_]*$.
		// Our attrs are lowercase (method, path, trace_id, ...), so they are
		// normalized here; without this they would be silently dropped.
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			a.Key = normalizeJournalField(a.Key)
			return a
		},
		ReplaceGroup: normalizeJournalField,
	})
	if err != nil {
		return jsonHandler
	}
	return h
}

// normalizeJournalField converts an slog key/group name into a valid journal
// field name: uppercase letters, numbers and underscores, not starting with an
// underscore, capped at 64 bytes. Reserved names yield an empty string, which
// causes the attribute to be dropped.
func normalizeJournalField(key string) string {
	var sb strings.Builder
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z':
			sb.WriteRune(r - 32)
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			sb.WriteRune(r)
		default:
			sb.WriteByte('_')
		}
	}
	s := strings.Trim(sb.String(), "_")
	if len(s) > 64 {
		s = s[:64]
	}
	switch s {
	case "MESSAGE", "PRIORITY", "SYSLOG_IDENTIFIER", "CODE_FILE", "CODE_FUNC", "CODE_LINE":
		return ""
	}
	return s
}
