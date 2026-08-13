package main

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/coreos/go-systemd/v22/journal"
)

// captureJournald returns a handler whose send records its arguments.
func captureJournald(t *testing.T) (*journaldHandler, *string, *journal.Priority, *map[string]string) {
	t.Helper()
	var msg string
	var prio journal.Priority
	var vars map[string]string
	h := newJournaldHandler(func(message string, priority journal.Priority, v map[string]string) error {
		msg, prio, vars = message, priority, v
		return nil
	})
	return h, &msg, &prio, &vars
}

func TestJournaldHandlerFields(t *testing.T) {
	h, msg, _, vars := captureJournald(t)
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "hello", 0)
	r.AddAttrs(
		slog.String("trace_id", "abc123"),
		slog.String("path", "/health"),
		slog.Int("status", 200),
		slog.Int64("duration_ms", 432),
		slog.Group("user", slog.String("email", "u@e.com")),
	)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if *msg != "hello" {
		t.Fatalf("message = %q", *msg)
	}
	want := map[string]string{
		"SYSLOG_IDENTIFIER": "vivagym-wallet",
		"TRACE_ID":          "abc123",
		"PATH":              "/health",
		"STATUS":            "200",
		"DURATION_MS":       "432",
		"USER_EMAIL":        "u@e.com",
	}
	for k, v := range want {
		if got := (*vars)[k]; got != v {
			t.Errorf("%s = %q, want %q (vars=%v)", k, got, v, *vars)
		}
	}
}

func TestJournaldHandlerPriority(t *testing.T) {
	cases := []struct {
		level slog.Level
		want  journal.Priority
	}{
		{slog.LevelError, journal.PriErr},
		{slog.LevelWarn, journal.PriWarning},
		{slog.LevelInfo, journal.PriInfo},
		{slog.LevelDebug, journal.PriDebug},
	}
	for _, c := range cases {
		h, _, prio, _ := captureJournald(t)
		r := slog.NewRecord(time.Now(), c.level, "m", 0)
		if err := h.Handle(context.Background(), r); err != nil {
			t.Fatal(err)
		}
		if *prio != c.want {
			t.Errorf("level %v -> priority %d, want %d", c.level, *prio, c.want)
		}
	}
}

func TestJournaldHandlerNormalization(t *testing.T) {
	h, _, _, vars := captureJournald(t)
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "m", 0)
	r.AddAttrs(
		slog.String("Content-Type", "text/html"),
		slog.String("trace id", "t1"),
		slog.String("message", "reserved"),
		slog.String("empty", ""),
	)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if got := (*vars)["CONTENT_TYPE"]; got != "text/html" {
		t.Errorf("CONTENT_TYPE = %q", got)
	}
	if got := (*vars)["TRACE_ID"]; got != "t1" {
		t.Errorf("TRACE_ID = %q", got)
	}
	if _, ok := (*vars)["MESSAGE"]; ok {
		t.Error("reserved MESSAGE field leaked into vars")
	}
	if _, ok := (*vars)["EMPTY"]; ok {
		t.Error("empty value should be dropped")
	}
}

func TestJournaldHandlerWithGroup(t *testing.T) {
	h, _, _, vars := captureJournald(t)
	h = h.WithGroup("req").WithAttrs([]slog.Attr{slog.String("id", "7")}).(*journaldHandler)
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "m", 0)
	r.AddAttrs(slog.String("path", "/health"))
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if got := (*vars)["REQ_ID"]; got != "7" {
		t.Errorf("REQ_ID = %q (attrs=%v)", got, *vars)
	}
	if got := (*vars)["REQ_PATH"]; got != "/health" {
		t.Errorf("REQ_PATH = %q (attrs=%v)", got, *vars)
	}
}
