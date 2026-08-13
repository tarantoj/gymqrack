package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/coreos/go-systemd/v22/journal"
)

// journalSend is journal.Send's signature, injectable for tests.
type journalSend func(message string, priority journal.Priority, vars map[string]string) error

// journaldHandler implements slog.Handler, writing each record to the systemd
// journal over its native unix socket as structured fields (METHOD, PATH,
// TRACE_ID, ...) with the correct priority. It never adds a timestamp field —
// journald stamps entries itself.
type journaldHandler struct {
	send       journalSend
	identifier string
	bound      []boundAttrs // WithAttrs entries, bound to the group active then
	group      string
}

// boundAttrs pairs attrs with the group prefix in effect when they were added.
type boundAttrs struct {
	group string
	attrs []slog.Attr
}

func newJournaldHandler(send journalSend) *journaldHandler {
	return &journaldHandler{
		send:       send,
		identifier: "vivagym-wallet",
	}
}

func (h *journaldHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *journaldHandler) Handle(ctx context.Context, r slog.Record) error {
	vars := map[string]string{"SYSLOG_IDENTIFIER": h.identifier}
	for _, b := range h.bound {
		for _, a := range b.attrs {
			addJournalAttr(vars, b.group, a)
		}
	}
	r.Attrs(func(a slog.Attr) bool {
		addJournalAttr(vars, h.group, a)
		return true
	})
	return h.send(r.Message, priorityForLevel(r.Level), vars)
}

func (h *journaldHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	nh := *h
	nh.bound = append(append([]boundAttrs{}, h.bound...), boundAttrs{group: h.group, attrs: attrs})
	return &nh
}

func (h *journaldHandler) WithGroup(name string) slog.Handler {
	nh := *h
	nh.group = joinJournalKey(h.group, name)
	return &nh
}

// priorityForLevel maps slog levels onto journal priorities so that
// journalctl -p filters behave.
func priorityForLevel(l slog.Level) journal.Priority {
	switch {
	case l >= slog.LevelError:
		return journal.PriErr
	case l >= slog.LevelWarn:
		return journal.PriWarning
	case l >= slog.LevelInfo:
		return journal.PriInfo
	default:
		return journal.PriDebug
	}
}

func addJournalAttr(vars map[string]string, prefix string, a slog.Attr) {
	if a.Equal(slog.Attr{}) {
		return
	}
	v := a.Value.Resolve()
	if v.Kind() == slog.KindGroup {
		for _, child := range v.Group() {
			addJournalAttr(vars, joinJournalKey(prefix, a.Key), child)
		}
		return
	}
	key := normalizeJournalField(joinJournalKey(prefix, a.Key))
	if key == "" {
		return
	}
	val := fmt.Sprint(v.Any())
	if val == "" {
		return
	}
	vars[key] = val
}

func joinJournalKey(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}

// normalizeJournalField converts a slog attribute key into a valid journal
// field name: uppercase letters, numbers and underscores, not starting with an
// underscore, capped at 64 bytes. Reserved names are dropped.
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
	case "MESSAGE", "PRIORITY", "SYSLOG_IDENTIFIER":
		return ""
	}
	return s
}
